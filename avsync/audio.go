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
	beepRMSThreshold   = -35.0 // dB: after bandpass filter, beep is detected above this
	beepMinGap         = 200 * time.Millisecond
	beepDetectionDelay = 10 * time.Millisecond
	bandpassWidth      = 50.0 // Hz
	// channelDominance: when both channels register above beepRMSThreshold,
	// the louder one wins if it's at least this many dB louder. Bandpass
	// bleed-through from neighboring participant frequencies puts a small
	// fraction of a routed beep on the opposite channel; this margin keeps
	// the detector from misclassifying that bleed as a true "both channel"
	// signal.
	channelDominance = 4.0
	// runMergeGap: an above-threshold frame this close to the end of the
	// current run extends it — a beep briefly dipping below threshold
	// mid-tone must not split into two runs.
	runMergeGap = 30 * time.Millisecond
	// Bandpass bleed from a neighboring participant's beep can itself rise
	// above beepRMSThreshold. When playout skew separates that bleed from
	// the participant's own beep by more than a frame, it forms its own run
	// — on whatever channel the neighbor is routed to. A run at least
	// leakMargin quieter than another run within leakProximity is bleed,
	// not a beep. Measured bleed peaks 9dB+ below the true beep, while
	// consecutive true beeps stay within ~1dB of each other; proximity is
	// capped at half the 1Hz beep cadence so true beeps never suppress
	// each other.
	leakMargin    = 6.0 // dB
	leakProximity = 500 * time.Millisecond
)

var (
	// reChannelRMS matches per-channel RMS levels emitted by astats, e.g.:
	//   lavfi.astats.1.RMS_level=-31.596143
	//   lavfi.astats.2.RMS_level=-inf       (digitally silent channel)
	// Channel index 1 = left, 2 = right (mono inputs only emit channel 1).
	// We deliberately do NOT match `Overall.RMS_level` (averages channels).
	reChannelRMS = regexp.MustCompile(`lavfi\.astats\.(\d+)\.RMS_level=(\S+)`)
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
	//
	// Resample to 48 kHz then chunk into fixed 480-sample (10 ms) frames
	// before bandpass+astats. Without this, the analysis frame size is set
	// by the codec (Opus ≈21 ms, raw PCM up to ~128 ms at 8 kHz), and the
	// beep onset can land anywhere within that frame, giving codec-specific
	// PTS skew up to ±60 ms. With a uniform 10 ms window every codec reports
	// the same beep onset within ~1 µs.
	filter := fmt.Sprintf(
		"aresample=48000,asetnsamples=n=480:p=0,"+
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

// beepFrame is a single astats analysis frame (10 ms). Channel 1 = left,
// 2 = right; mono inputs only report channel 1.
type beepFrame struct {
	pts    time.Duration
	ch1    float64
	ch2    float64
	hasCh2 bool
}

// level returns the loudest channel's RMS.
func (f beepFrame) level() float64 {
	if f.hasCh2 && f.ch2 > f.ch1 {
		return f.ch2
	}
	return f.ch1
}

// beepRun is a group of consecutive above-threshold frames — one beep
// candidate. start is the onset frame's PTS; peak is the loudest frame.
type beepRun struct {
	start time.Duration
	end   time.Duration
	peak  beepFrame
}

// parseBeepLog reads the metadata log file and extracts debounced beep
// timestamps. Frames above beepRMSThreshold are grouped into runs, bleed
// runs from neighboring participant frequencies are rejected, and each
// surviving run becomes one beep: PTS from the run's onset frame, channel
// from its loudest frame:
//
//   - only channel 1 above threshold                → BeepChannelLeft
//   - only channel 2 above threshold                → BeepChannelRight
//   - both above, one dominant by channelDominance  → that channel
//   - both above, neither dominant                  → BeepChannelBoth
//   - mono input, channel 1 above                   → BeepChannelBoth
//
// Classifying at the loudest frame (rather than the onset frame) matters
// for routed-channel recordings: the onset frame can be bandpass bleed
// from a neighboring frequency on the opposite channel, arriving slightly
// ahead of the true beep.
func parseBeepLog(logFile, participantName string) ([]Beep, error) {
	frames, err := parseBeepFrames(logFile)
	if err != nil {
		return nil, err
	}

	runs := groupBeepRuns(frames)

	var beeps []Beep
	var lastBeepPTS time.Duration = -1
	for i, r := range runs {
		if isLeakRun(runs, i) {
			continue
		}
		// Debounce: only emit if we're at least beepMinGap past last beep.
		// Subtract beepDetectionDelay so the reported PTS matches the
		// true beep onset rather than the analysis frame that detected it.
		// Debounce stays on the raw onset PTS so it's independent of the
		// calibration constant.
		if lastBeepPTS >= 0 && r.start-lastBeepPTS < beepMinGap {
			continue
		}
		beeps = append(beeps, Beep{
			PTS:         r.start - beepDetectionDelay,
			Participant: participantName,
			Channel:     classifyChannel(r.peak),
		})
		lastBeepPTS = r.start
	}

	return beeps, nil
}

// parseBeepFrames reads per-channel RMS values for each analysis frame in
// the astats metadata log.
func parseBeepFrames(logFile string) ([]beepFrame, error) {
	f, err := os.Open(logFile)
	if err != nil {
		return nil, fmt.Errorf("open beep log %s: %w", logFile, err)
	}
	defer f.Close()

	var frames []beepFrame

	var currentPTS time.Duration = -1
	channelRMS := map[int]float64{}
	hasFrame := false

	flushFrame := func() {
		if !hasFrame {
			return
		}
		ch1, has1 := channelRMS[1]
		if !has1 {
			return
		}
		ch2, has2 := channelRMS[2]
		frames = append(frames, beepFrame{
			pts:    currentPTS,
			ch1:    ch1,
			ch2:    ch2,
			hasCh2: has2,
		})
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
			if math.IsNaN(rms) {
				continue
			}
			// -inf means the channel is digital silence, not absent — record it
			// as a very low finite value so classification can tell "stereo with
			// one silent channel" apart from "mono file (no channel 2)".
			if math.IsInf(rms, -1) {
				rms = -200
			}
			channelRMS[ch] = rms
		}
	}
	flushFrame() // last frame in the log

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan beep log: %w", err)
	}

	return frames, nil
}

// groupBeepRuns groups above-threshold frames into runs, tolerating dips
// below threshold shorter than runMergeGap.
func groupBeepRuns(frames []beepFrame) []beepRun {
	var runs []beepRun
	var cur *beepRun
	for _, f := range frames {
		if f.level() <= beepRMSThreshold {
			continue
		}
		if cur != nil && f.pts-cur.end <= runMergeGap {
			cur.end = f.pts
			if f.level() > cur.peak.level() {
				cur.peak = f
			}
			continue
		}
		if cur != nil {
			runs = append(runs, *cur)
		}
		cur = &beepRun{start: f.pts, end: f.pts, peak: f}
	}
	if cur != nil {
		runs = append(runs, *cur)
	}
	return runs
}

// isLeakRun reports whether runs[i] is bandpass bleed: another run within
// leakProximity peaks at least leakMargin louder.
func isLeakRun(runs []beepRun, i int) bool {
	level := runs[i].peak.level()
	for j := i - 1; j >= 0 && runs[i].start-runs[j].start <= leakProximity; j-- {
		if runs[j].peak.level()-level >= leakMargin {
			return true
		}
	}
	for j := i + 1; j < len(runs) && runs[j].start-runs[i].start <= leakProximity; j++ {
		if runs[j].peak.level()-level >= leakMargin {
			return true
		}
	}
	return false
}

// classifyChannel decides which channel(s) a beep landed on from its
// loudest analysis frame. The frame is above threshold on at least one
// channel by construction.
func classifyChannel(f beepFrame) BeepChannel {
	if !f.hasCh2 {
		// Mono input: only channel 1 reported.
		return BeepChannelBoth
	}
	ch1Above := f.ch1 > beepRMSThreshold
	ch2Above := f.ch2 > beepRMSThreshold
	switch {
	case ch1Above && ch2Above:
		// Both above threshold — the louder channel wins if it
		// dominates by at least channelDominance dB; otherwise
		// the signal is genuinely on both channels.
		switch {
		case f.ch1-f.ch2 >= channelDominance:
			return BeepChannelLeft
		case f.ch2-f.ch1 >= channelDominance:
			return BeepChannelRight
		default:
			return BeepChannelBoth
		}
	case ch1Above:
		return BeepChannelLeft
	default:
		return BeepChannelRight
	}
}
