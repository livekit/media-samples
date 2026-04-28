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
	"strings"
	"time"
)

const (
	beepRMSThreshold = -35.0 // dB: after bandpass filter, beep is detected above this
	beepMinGap       = 200 * time.Millisecond
	bandpassWidth    = 50.0 // Hz
	silenceNoiseDB   = -38
	silenceMinDur    = 0.1 // seconds
)

var (
	reRMSLevel     = regexp.MustCompile(`RMS_level[=:](-?[0-9.]+)`)
	rePTSTime      = regexp.MustCompile(`pts_time:([0-9.]+)`)
	reSilenceStart = regexp.MustCompile(`silence_start:\s*([0-9.]+)`)
	reSilenceEnd   = regexp.MustCompile(`silence_end:\s*([0-9.]+)\s*\|\s*silence_duration:\s*([0-9.]+)`)
)

func secToDuration(s float64) time.Duration {
	return time.Duration(s * float64(time.Second))
}

// analyzeAudio runs per-participant bandpass beep detection and silence detection.
func analyzeAudio(cfg Config) (AudioResult, error) {
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultTimeout
	}

	tmpDir, err := os.MkdirTemp("", "avsync-audio-*")
	if err != nil {
		return AudioResult{}, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	var allBeeps []Beep

	for _, p := range cfg.Participants {
		beeps, err := detectBeeps(cfg, p, tmpDir)
		if err != nil {
			return AudioResult{}, fmt.Errorf("detect beeps for %s: %w", p.Name, err)
		}
		allBeeps = append(allBeeps, beeps...)
	}

	// Sort beeps chronologically.
	sort.Slice(allBeeps, func(i, j int) bool {
		return allBeeps[i].PTS < allBeeps[j].PTS
	})

	silence, err := detectSilence(cfg)
	if err != nil {
		return AudioResult{}, fmt.Errorf("detect silence: %w", err)
	}

	return AudioResult{
		Beeps:   allBeeps,
		Silence: silence,
	}, nil
}

// detectBeeps runs FFmpeg with a bandpass filter centered on p.BeepFreq,
// writes RMS metadata to a log file, and parses it for beep events.
func detectBeeps(cfg Config, p Participant, tmpDir string) ([]Beep, error) {
	logFile := filepath.Join(tmpDir, fmt.Sprintf("beep_%s.log", p.Name))

	filter := fmt.Sprintf(
		"bandpass=f=%.0f:width_type=h:w=%.0f,astats=metadata=1:reset=1,ametadata=print:key=lavfi.astats.Overall.RMS_level:file=%s",
		p.BeepFreq, bandpassWidth, logFile,
	)

	args := []string{
		"-i", cfg.FilePath,
		"-af", filter,
		"-f", "null", "-",
	}

	if err := runFFmpeg(runFFmpegArgs{args: args, timeout: cfg.Timeout}); err != nil {
		return nil, err
	}

	return parseBeepLog(logFile, p.Name)
}

// parseBeepLog reads the ametadata log file and extracts debounced beep timestamps.
func parseBeepLog(logFile, participantName string) ([]Beep, error) {
	f, err := os.Open(logFile)
	if err != nil {
		return nil, fmt.Errorf("open beep log %s: %w", logFile, err)
	}
	defer f.Close()

	var beeps []Beep
	var lastBeepPTS time.Duration = -1

	// The log alternates: pts_time line, then RMS_level line (or vice versa).
	// We accumulate (pts, rms) pairs and emit beeps when rms > threshold.
	var currentPTS time.Duration = -1
	var hasPTS bool

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()

		if m := rePTSTime.FindStringSubmatch(line); m != nil {
			secs, err := strconv.ParseFloat(m[1], 64)
			if err == nil {
				currentPTS = secToDuration(secs)
				hasPTS = true
			}
			continue
		}

		if m := reRMSLevel.FindStringSubmatch(line); m != nil {
			valStr := m[1]
			// Skip non-numeric values like "inf" or "nan"
			if strings.EqualFold(valStr, "inf") || strings.EqualFold(valStr, "nan") || strings.EqualFold(valStr, "-inf") {
				continue
			}
			rms, err := strconv.ParseFloat(valStr, 64)
			if err != nil {
				continue
			}
			if math.IsInf(rms, 0) || math.IsNaN(rms) {
				continue
			}

			if hasPTS && rms > beepRMSThreshold {
				// Debounce: only emit if we're at least beepMinGap past last beep.
				if lastBeepPTS < 0 || currentPTS-lastBeepPTS >= beepMinGap {
					beeps = append(beeps, Beep{
						PTS:         currentPTS,
						Participant: participantName,
					})
					lastBeepPTS = currentPTS
				}
			}
			hasPTS = false
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan beep log: %w", err)
	}

	return beeps, nil
}

// detectSilence runs FFmpeg silencedetect and parses stderr for silence ranges.
func detectSilence(cfg Config) ([]SilenceRange, error) {
	filter := fmt.Sprintf("silencedetect=noise=%ddB:d=%.2f", silenceNoiseDB, silenceMinDur)

	args := []string{
		"-i", cfg.FilePath,
		"-af", filter,
		"-f", "null", "-",
	}

	stderr, err := runFFmpegWithStderr(runFFmpegArgs{args: args, timeout: cfg.Timeout})
	if err != nil {
		return nil, err
	}

	return parseSilenceLog(string(stderr))
}

// parseSilenceLog extracts SilenceRange entries from silencedetect stderr output.
func parseSilenceLog(output string) ([]SilenceRange, error) {
	var ranges []SilenceRange
	var pendingStart time.Duration = -1

	for _, line := range strings.Split(output, "\n") {
		if m := reSilenceStart.FindStringSubmatch(line); m != nil {
			secs, err := strconv.ParseFloat(m[1], 64)
			if err == nil {
				pendingStart = secToDuration(secs)
			}
			continue
		}

		if m := reSilenceEnd.FindStringSubmatch(line); m != nil {
			endSecs, err := strconv.ParseFloat(m[1], 64)
			if err != nil {
				continue
			}
			durSecs, err := strconv.ParseFloat(m[2], 64)
			if err != nil {
				continue
			}
			endPTS := secToDuration(endSecs)
			dur := secToDuration(durSecs)

			start := pendingStart
			if start < 0 {
				// Reconstruct start if we missed a silence_start line.
				start = endPTS - dur
			}

			ranges = append(ranges, SilenceRange{
				Start:    start,
				End:      endPTS,
				Duration: dur,
			})
			pendingStart = -1
		}
	}

	return ranges, nil
}
