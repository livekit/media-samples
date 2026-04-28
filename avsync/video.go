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

// analyzeVideo runs FFmpeg on cfg.FilePath and returns a VideoResult
// containing per-region frame classifications and flash timestamps.
func analyzeVideo(cfg Config) (VideoResult, error) {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}

	cc := newColorClassifier(cfg.Participants)

	result := VideoResult{
		Regions: make(map[string][]RegionFrame),
		Flashes: make(map[string][]time.Duration),
	}

	for _, region := range cfg.Regions {
		if err := analyzeRegion(cfg.FilePath, region, cc, timeout, &result); err != nil {
			return VideoResult{}, fmt.Errorf("region %q: %w", region.Name, err)
		}
	}

	return result, nil
}

func analyzeRegion(filePath string, region Region, cc *colorClassifier, timeout time.Duration, result *VideoResult) error {
	tmpDir, err := os.MkdirTemp("", "avsync-video-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	flashLog := tmpDir + "/flash.log"
	labelLog := tmpDir + "/label.log"

	r := region.Rect
	x := r.Min.X
	y := r.Min.Y
	w := r.Dx()

	// Flash stripe: top 8 px of the region.
	// Label area: top-left 200x50 of the region, starting 10px from the top.
	filterComplex := fmt.Sprintf(
		"[0:v]split=2[flash][label];"+
			"[flash]crop=%d:8:%d:%d,signalstats,metadata=print:file=%s[fout];"+
			"[label]crop=200:50:%d:%d,signalstats,metadata=print:file=%s[lout]",
		w, x, y, flashLog,
		x, y+10, labelLog,
	)

	args := []string{
		"-i", filePath,
		"-filter_complex", filterComplex,
		"-map", "[fout]", "-f", "null", "-",
		"-map", "[lout]", "-f", "null", "-",
	}

	if err := runFFmpeg(runFFmpegArgs{args: args, timeout: timeout}); err != nil {
		return fmt.Errorf("ffmpeg: %w", err)
	}

	// Parse flash log.
	flashFrames, err := parseMetadataLog(flashLog)
	if err != nil {
		return fmt.Errorf("parse flash log: %w", err)
	}

	// Parse label log.
	labelFrames, err := parseMetadataLog(labelLog)
	if err != nil {
		return fmt.Errorf("parse label log: %w", err)
	}

	// Detect flashes: YAVG >= threshold, debounced.
	var flashes []time.Duration
	var lastFlash time.Duration = -flashDebounce - 1
	for _, f := range flashFrames {
		yavg := f.vals["YAVG"]
		if yavg >= flashYAVGThreshold {
			if f.pts-lastFlash > flashDebounce {
				flashes = append(flashes, f.pts)
				lastFlash = f.pts
			}
		}
	}

	// Build per-frame participant classification.
	frames := make([]RegionFrame, 0, len(labelFrames))
	for _, f := range labelFrames {
		s := regionStats{
			ymax:   f.vals["YMAX"],
			umin:   f.vals["UMIN"],
			vmin:   f.vals["VMIN"],
			satmax: f.vals["SATMAX"],
		}
		participant := cc.classify(s)
		frames = append(frames, RegionFrame{
			PTS:         f.pts,
			Participant: participant,
		})
	}

	result.Regions[region.Name] = frames
	result.Flashes[region.Name] = flashes

	return nil
}
