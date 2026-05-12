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
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"
)

const (
	beepRMSThreshold = -35.0 // dB: after bandpass filter, beep is detected above this
	beepMinGap       = 200 * time.Millisecond
	bandpassWidth    = 50.0 // Hz
)

var (
	// reChannelRMS matches per-channel RMS levels emitted by astats, e.g.:
	//   lavfi.astats.1.RMS_level=-31.596143
	// Channel index 1 = left, 2 = right (mono inputs only emit channel 1).
	// We deliberately do NOT match `Overall.RMS_level` (averages channels).
	reChannelRMS = regexp.MustCompile(`lavfi\.astats\.(\d+)\.RMS_level=(-?[0-9.]+)`)
	rePTSTime    = regexp.MustCompile(`pts_time:([0-9.]+)`)
)

func secToDuration(s float64) time.Duration {
	return time.Duration(s * float64(time.Second))
}

// analyzeAudio runs per-participant bandpass beep detection.
func analyzeAudio(cfg Config) ([]Beep, error) {
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultTimeout
	}

	tmpDir, err := os.MkdirTemp("", "avsync-audio-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	var allBeeps []Beep
	for _, p := range cfg.Participants {
		beeps, err := detectBeeps(cfg, p, tmpDir)
		if err != nil {
			return nil, fmt.Errorf("detect beeps for %s: %w", p.Name, err)
		}
		allBeeps = append(allBeeps, beeps...)
	}

	sort.Slice(allBeeps, func(i, j int) bool {
		return allBeeps[i].PTS < allBeeps[j].PTS
	})
	return allBeeps, nil
}

// detectBeeps runs FFmpeg with a bandpass filter centered on p.BeepFreq,
// writes RMS metadata to a log file, and parses it for beep events.
func detectBeeps(cfg Config, p Participant, tmpDir string) ([]Beep, error) {
	logFile := filepath.Join(tmpDir, fmt.Sprintf("beep_%s.log", p.Name))

	// Print all astats metadata (rather than just Overall.RMS_level) so the
	// parser can read per-channel RMS and identify which channel(s) a beep
	// landed on. Used by audio-routing tests.
	filter := fmt.Sprintf(
		"bandpass=f=%.0f:width_type=h:w=%.0f,astats=metadata=1:reset=1,ametadata=print:file=%s",
		p.BeepFreq, bandpassWidth, logFile,
	)

	args := []string{
		"-i", cfg.FilePath,
		"-af", filter,
		"-f", "null", "-",
	}

	if _, err := runFFmpeg(runFFmpegArgs{args: args, timeout: cfg.Timeout}); err != nil {
		return nil, err
	}

	return parseBeepLog(logFile, p.Name)
}

// parseBeepLog reads the metadata log file and extracts debounced beep
// timestamps. Each frame in the log emits per-channel RMS values (channel 1 =
// left, 2 = right; mono inputs emit only channel 1). The parser accumulates
// per-channel RMS for each frame and, when the next pts_time is seen, decides:
//
//   - both channels above threshold      → BeepChannelBoth
//   - only channel 1 above threshold     → BeepChannelLeft
//   - only channel 2 above threshold     → BeepChannelRight
//   - mono input, channel 1 above        → BeepChannelBoth
//   - neither above threshold            → no beep emitted
func parseBeepLog(logFile, participantName string) ([]Beep, error) {
	f, err := os.Open(logFile)
	if err != nil {
		return nil, fmt.Errorf("open beep log %s: %w", logFile, err)
	}
	defer f.Close()

	var beeps []Beep
	var lastBeepPTS time.Duration = -1

	var currentPTS time.Duration = -1
	channelRMS := map[int]float64{}
	hasFrame := false

	flushFrame := func() {
		if !hasFrame {
			return
		}
		ch1, has1 := channelRMS[1]
		ch2, has2 := channelRMS[2]
		var channel BeepChannel
		switch {
		case has1 && has2:
			ch1Above := ch1 > beepRMSThreshold
			ch2Above := ch2 > beepRMSThreshold
			switch {
			case ch1Above && ch2Above:
				channel = BeepChannelBoth
			case ch1Above:
				channel = BeepChannelLeft
			case ch2Above:
				channel = BeepChannelRight
			default:
				return // no beep on either channel
			}
		case has1:
			// Mono input: only channel 1 reported.
			if ch1 <= beepRMSThreshold {
				return
			}
			channel = BeepChannelBoth
		default:
			return
		}

		// Debounce: only emit if we're at least beepMinGap past last beep.
		if lastBeepPTS < 0 || currentPTS-lastBeepPTS >= beepMinGap {
			beeps = append(beeps, Beep{
				PTS:         currentPTS,
				Participant: participantName,
				Channel:     channel,
			})
			lastBeepPTS = currentPTS
		}
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()

		if m := rePTSTime.FindStringSubmatch(line); m != nil {
			// New frame starts: flush the previous frame's accumulated values.
			flushFrame()
			secs, err := strconv.ParseFloat(m[1], 64)
			if err == nil {
				currentPTS = secToDuration(secs)
				channelRMS = map[int]float64{}
				hasFrame = true
			}
			continue
		}

		if !hasFrame {
			continue
		}

		if m := reChannelRMS.FindStringSubmatch(line); m != nil {
			ch, err := strconv.Atoi(m[1])
			if err != nil {
				continue
			}
			rms, err := strconv.ParseFloat(m[2], 64)
			if err != nil {
				continue
			}
			if math.IsInf(rms, 0) || math.IsNaN(rms) {
				continue
			}
			channelRMS[ch] = rms
		}
	}
	flushFrame() // last frame in the log

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan beep log: %w", err)
	}

	return beeps, nil
}
