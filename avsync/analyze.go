package avsync

import (
	"fmt"
	"time"
)

// Analyze orchestrates video and audio analysis for the given Config.
// It returns a Result containing video frame classifications and audio beep
// detections. Audio failures are treated as non-fatal (e.g. video-only files).
func Analyze(cfg Config) (*Result, error) {
	if cfg.FilePath == "" {
		return nil, fmt.Errorf("FilePath is required")
	}

	result := &Result{
		Video: VideoResult{
			Regions: make(map[string][]RegionFrame),
			Flashes: make(map[string][]time.Duration),
		},
	}

	if len(cfg.Regions) > 0 {
		videoResult, err := analyzeVideo(cfg)
		if err != nil {
			return nil, fmt.Errorf("video analysis: %w", err)
		}
		result.Video = videoResult
	}

	audioResult, err := analyzeAudio(cfg)
	if err != nil {
		// Audio may not exist in a video-only file — return empty audio
		result.Audio = AudioResult{}
	} else {
		result.Audio = audioResult
	}

	return result, nil
}
