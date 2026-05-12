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
	"image"
	"testing"
	"time"
)

func TestAnalyzeVideo(t *testing.T) {
	cfg := Config{
		FilePath: "../livekit_avsync_p0_video_red_1080p25.h264",
		Regions: []Region{
			{Name: "full", Rect: image.Rect(0, 0, 1920, 1080)},
		},
		Participants: AllParticipants,
		Timeout:      30 * time.Second,
	}

	flashes, err := analyzeVideo(cfg)
	if err != nil {
		t.Fatalf("analyzeVideo failed: %v", err)
	}

	if len(flashes) < 115 || len(flashes) > 125 {
		t.Errorf("expected ~120 flashes, got %d", len(flashes))
	}

	for i, f := range flashes {
		if f.Region != "full" {
			t.Errorf("flash %d: region=%q, want full", i, f.Region)
		}
		if f.Participant != "p0" {
			t.Errorf("flash %d (PTS=%s): got participant %q, want p0", i, f.PTS, f.Participant)
			if i > 5 {
				t.Fatalf("stopping after %d mismatches", i)
			}
		}
	}

	for i := 1; i < len(flashes); i++ {
		gap := flashes[i].PTS - flashes[i-1].PTS
		if gap < 800*time.Millisecond || gap > 1200*time.Millisecond {
			t.Errorf("flash gap [%d]: %s (expected ~1s)", i, gap)
		}
	}
}
