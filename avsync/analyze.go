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
