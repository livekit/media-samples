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

// The flash drawbox is enabled for the first 50ms of each integer second.
// At 24/25 fps that maps to exactly one frame at PTS = N seconds, so the
// analyzer should report flashes at integer-second PTS with no detection
// delay. Tolerance is set just above 1/24fps (≈41.7ms) frame quantization.
const flashPTSTolerance = 1 * time.Millisecond

type videoFile struct {
	path string
	part string // expected participant for every flash
}

var allVideoFiles = []videoFile{
	{"../livekit_avsync_p0_video_red_1080p25.h264", "p0"},
	{"../livekit_avsync_p1_video_green_1080p25.h264", "p1"},
	{"../livekit_avsync_p2_video_blue_1080p25.h264", "p2"},
	{"../livekit_avsync_p0_video_red_1080p24.vp8.ivf", "p0"},
	{"../livekit_avsync_p0_video_red_1080p24.vp9.ivf", "p0"},
}

func TestAnalyzeVideo(t *testing.T) {
	for _, vf := range allVideoFiles {
		t.Run(vf.part+"/"+vf.path, func(t *testing.T) {
			flashes, err := analyzeVideo(Config{
				FilePath: vf.path,
				Regions: []Region{
					{Name: "full", Rect: image.Rect(0, 0, 1920, 1080)},
				},
				Participants: AllParticipants,
				Timeout:      60 * time.Second,
			})
			if err != nil {
				t.Fatalf("analyzeVideo: %v", err)
			}
			checkFlashes(t, flashes, vf.part)
		})
	}
}

// checkFlashes verifies exactly 120 flashes attributed to wantPart, each
// at the expected integer-second PTS with a 1s cadence.
func checkFlashes(t *testing.T, flashes []Flash, wantPart string) {
	t.Helper()

	if len(flashes) != 120 {
		t.Errorf("flashes: got %d, want exactly 120", len(flashes))
	}

	for i, f := range flashes {
		if f.Region != "full" {
			t.Errorf("flash %d: region=%q, want full", i, f.Region)
		}
		if f.Participant != wantPart {
			t.Errorf("flash %d (PTS=%s): participant=%q, want %s", i, f.PTS, f.Participant, wantPart)
			if i > 5 {
				t.Fatalf("stopping after %d mismatches", i)
			}
		}
		want := time.Duration(i) * time.Second
		if diff := absDuration(f.PTS - want); diff > flashPTSTolerance {
			t.Errorf("flash %d: PTS=%s, want %s ±%s (off by %s)", i, f.PTS, want, flashPTSTolerance, diff)
		}
	}

	for i := 1; i < len(flashes); i++ {
		gap := flashes[i].PTS - flashes[i-1].PTS
		if diff := absDuration(gap - time.Second); diff > flashPTSTolerance {
			t.Errorf("flash gap [%d]: %s, want 1s ±%s", i, gap, flashPTSTolerance)
		}
	}
}
