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
	"testing"
	"time"
)

// Observed jitter across all sample files is <1µs (just float rounding
// in pts_time parsing). 1ms is 1000x tighter than the prior ±100ms
// bound while still tolerating ffmpeg-version drift.
const beepPTSTolerance = 1 * time.Millisecond

// audioFile describes a generated sample we expect the analyzer to read.
type audioFile struct {
	path string
	part string // expected participant for every detected beep
}

var allAudioFiles = []audioFile{
	{"../livekit_avsync_p0_audio_523hz_48k.ogg", "p0"},
	{"../livekit_avsync_p1_audio_659hz_48k.ogg", "p1"},
	{"../livekit_avsync_p2_audio_784hz_48k.ogg", "p2"},
	{"../livekit_avsync_p0_audio_523hz_48k.wav", "p0"},
	{"../livekit_avsync_p0_audio_523hz_8k.pcma.wav", "p0"},
	{"../livekit_avsync_p0_audio_523hz_8k.pcmu.wav", "p0"},
}

func TestAnalyzeAudio(t *testing.T) {
	for _, af := range allAudioFiles {
		t.Run(af.part+"/"+af.path, func(t *testing.T) {
			beeps, err := analyzeAudio(Config{
				FilePath:     af.path,
				Participants: AllParticipants,
				Timeout:      30 * time.Second,
			})
			if err != nil {
				t.Fatalf("analyzeAudio: %v", err)
			}
			checkBeeps(t, beeps, af.part)
		})
	}
}

// checkBeeps verifies the analyzer found exactly 120 beeps for wantPart
// at the expected per-second cadence, with no cross-talk into the other
// two participants' bandpass filters.
func checkBeeps(t *testing.T, beeps []Beep, wantPart string) {
	t.Helper()

	var got []Beep
	counts := map[string]int{}
	for _, b := range beeps {
		counts[b.Participant]++
		if b.Participant == wantPart {
			got = append(got, b)
		}
	}

	if len(got) != 120 {
		t.Errorf("%s beeps: got %d, want exactly 120", wantPart, len(got))
	}
	for _, p := range []string{"p0", "p1", "p2"} {
		if p == wantPart {
			continue
		}
		if counts[p] != 0 {
			t.Errorf("%s beeps: got %d, want 0 (bandpass at participant freq should reject the source tone)", p, counts[p])
		}
	}

	for i, b := range got {
		want := time.Duration(i) * time.Second
		if diff := absDuration(b.PTS - want); diff > beepPTSTolerance {
			t.Errorf("%s beep %d: PTS=%s, want %s ±%s (off by %s)", wantPart, i, b.PTS, want, beepPTSTolerance, diff)
		}
	}

	for i := 1; i < len(got); i++ {
		gap := got[i].PTS - got[i-1].PTS
		if diff := absDuration(gap - time.Second); diff > beepPTSTolerance {
			t.Errorf("%s beep gap [%d]: %s, want 1s ±%s", wantPart, i, gap, beepPTSTolerance)
		}
	}
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
