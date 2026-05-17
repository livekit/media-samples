// Copyright 2026 LiveKit, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package avsync

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	reVideoFrameHeader = regexp.MustCompile(`pts_time:([0-9.]+)`)
	reVideoStatLine    = regexp.MustCompile(`lavfi\.signalstats\.(\w+)=([0-9.]+)`)
)

// frameStats holds per-frame parsed data from a metadata log file.
type frameStats struct {
	pts  time.Duration
	vals map[string]float64
}

// parseMetadataLog reads a metadata=print log file produced by FFmpeg and
// returns one frameStats per frame.
func parseMetadataLog(path string) ([]frameStats, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open metadata log %s: %w", path, err)
	}
	defer f.Close()

	var frames []frameStats
	var cur *frameStats

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()

		if m := reVideoFrameHeader.FindStringSubmatch(line); m != nil {
			// Commit the previous frame if any.
			if cur != nil {
				frames = append(frames, *cur)
			}
			secs, err := strconv.ParseFloat(m[1], 64)
			if err != nil {
				return nil, fmt.Errorf("parse pts_time: %w", err)
			}
			cur = &frameStats{pts: secToDuration(secs), vals: make(map[string]float64)}
			continue
		}

		if cur == nil {
			continue
		}

		if m := reVideoStatLine.FindStringSubmatch(line); m != nil {
			key := strings.ToUpper(m[1])
			val, err := strconv.ParseFloat(m[2], 64)
			if err == nil {
				cur.vals[key] = val
			}
		}
	}
	if cur != nil {
		frames = append(frames, *cur)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan metadata log: %w", err)
	}
	return frames, nil
}

const (
	flashYAVGThreshold = 130.0
	flashDebounce      = 200 * time.Millisecond
)

// analyzeVideo runs FFmpeg on cfg.FilePath and returns the union of flash
// events across all requested regions, each attributed to a participant.
func analyzeVideo(cfg Config) ([]Flash, error) {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}

	cc := newColorClassifier(cfg.Participants)

	var flashes []Flash
	for _, region := range cfg.Regions {
		regionFlashes, err := analyzeRegion(cfg.FilePath, region, cc, timeout)
		if err != nil {
			return nil, fmt.Errorf("region %q: %w", region.Name, err)
		}
		flashes = append(flashes, regionFlashes...)
	}

	return flashes, nil
}

func analyzeRegion(filePath string, region Region, cc *colorClassifier, timeout time.Duration) ([]Flash, error) {
	tmpDir, err := os.MkdirTemp("", "avsync-video-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	flashLog := tmpDir + "/flash.log"
	labelLog := tmpDir + "/label.log"

	r := region.Rect
	x := r.Min.X
	y := r.Min.Y
	w := r.Dx()
	h := r.Dy()

	// Flash stripe: top 8 px of the region.
	// Label area: 60% × 40% centered on the region. With a solid
	// participant-colored background, the average U/V over this area
	// is dominated by the background regardless of which part of the
	// source ends up cropped/scaled into the cell.
	labelW := w * 60 / 100
	labelH := h * 40 / 100
	if labelW < 40 {
		labelW = 40
	}
	if labelH < 30 {
		labelH = 30
	}
	labelX := x + (w-labelW)/2
	labelY := y + (h-labelH)/2

	// Force yuv420p before signalstats so YAVG/UAVG/VAVG are always
	// reported in 0-255 range. Without this, 10/12-bit pixel formats
	// (e.g. yuv444p12le from HDR VP9 sources) emit 0-4095 values and
	// the 8-bit flashYAVGThreshold misfires on every frame.
	//
	// Run flash and label detection as two separate -vf passes instead
	// of a single -filter_complex with split. Some ffmpeg versions
	// (notably 6.x) reset video PTS to 0 inside filter_complex for
	// MPEG-TS input, breaking av-sync measurement. Simple -vf filters
	// preserve the original PTS correctly.
	flashFilter := fmt.Sprintf(
		"format=yuv420p,crop=%d:8:%d:%d,signalstats,metadata=print:file=%s",
		w, x, y, flashLog,
	)
	labelFilter := fmt.Sprintf(
		"format=yuv420p,crop=%d:%d:%d:%d,signalstats,metadata=print:file=%s",
		labelW, labelH, labelX, labelY, labelLog,
	)

	flashArgs := []string{"-i", filePath, "-vf", flashFilter, "-f", "null", "-"}
	if _, err := runFFmpeg(runFFmpegArgs{args: flashArgs, timeout: timeout}); err != nil {
		return nil, fmt.Errorf("ffmpeg flash: %w", err)
	}

	labelArgs := []string{"-i", filePath, "-vf", labelFilter, "-f", "null", "-"}
	if _, err := runFFmpeg(runFFmpegArgs{args: labelArgs, timeout: timeout}); err != nil {
		return nil, fmt.Errorf("ffmpeg label: %w", err)
	}

	flashFrames, err := parseMetadataLog(flashLog)
	if err != nil {
		return nil, fmt.Errorf("parse flash log: %w", err)
	}
	labelFrames, err := parseMetadataLog(labelLog)
	if err != nil {
		return nil, fmt.Errorf("parse label log: %w", err)
	}

	// Per-PTS participant classification from the label stripe — used to
	// attribute each flash to whoever's video was rendered in this cell
	// at that moment.
	labels := make(map[time.Duration]string, len(labelFrames))
	for _, f := range labelFrames {
		labels[f.pts] = cc.classify(regionStats{
			uavg: f.vals["UAVG"],
			vavg: f.vals["VAVG"],
		})
	}

	// Detect flashes: YAVG >= threshold, debounced. Attribute each to
	// the participant rendered at the same PTS.
	var flashes []Flash
	var lastFlash time.Duration = -flashDebounce - 1
	for _, f := range flashFrames {
		if f.vals["YAVG"] < flashYAVGThreshold {
			continue
		}
		if f.pts-lastFlash <= flashDebounce {
			continue
		}
		flashes = append(flashes, Flash{
			Region:      region.Name,
			PTS:         f.pts,
			Participant: labels[f.pts],
		})
		lastFlash = f.pts
	}

	return flashes, nil
}
