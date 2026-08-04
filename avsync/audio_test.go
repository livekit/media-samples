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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Observed jitter across all sample files is <1µs
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

// logFrame is one synthetic astats analysis frame for parseBeepLog tests.
// RMS values are written verbatim; "" omits the channel line entirely
// (mono files never emit channel 2).
type logFrame struct {
	pts time.Duration
	ch1 string
	ch2 string
}

func writeBeepLog(t *testing.T, frames []logFrame) string {
	t.Helper()

	var sb strings.Builder
	for i, f := range frames {
		fmt.Fprintf(&sb, "frame:%d pts:%d pts_time:%.6f\n", i, i*480, f.pts.Seconds())
		if f.ch1 != "" {
			fmt.Fprintf(&sb, "lavfi.astats.1.RMS_level=%s\n", f.ch1)
		}
		if f.ch2 != "" {
			fmt.Fprintf(&sb, "lavfi.astats.2.RMS_level=%s\n", f.ch2)
		}
	}

	path := filepath.Join(t.TempDir(), "beep.log")
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestParseBeepLog covers run grouping, peak-frame channel classification,
// and bleed rejection on synthetic frame sequences. The bleed cases
// reproduce routed-channel recordings where a neighboring participant's
// beep leaks through the bandpass on the opposite channel, offset from the
// true beep by playout skew.
func TestParseBeepLog(t *testing.T) {
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }

	type want struct {
		pts     time.Duration
		channel BeepChannel
	}

	cases := []struct {
		name   string
		frames []logFrame
		want   []want
	}{
		{
			name: "bleed before true beep is rejected",
			frames: []logFrame{
				{ms(1000), "-57.0", "-30.0"},
				{ms(1010), "-57.0", "-31.0"},
				{ms(1360), "-18.0", "-53.0"},
				{ms(1370), "-17.5", "-52.0"},
				{ms(1380), "-19.0", "-53.0"},
			},
			want: []want{{ms(1350), BeepChannelLeft}},
		},
		{
			name: "bleed after true beep is rejected",
			frames: []logFrame{
				{ms(1000), "-53.0", "-18.0"},
				{ms(1010), "-52.0", "-17.5"},
				{ms(1360), "-30.0", "-57.0"},
				{ms(1370), "-31.0", "-57.0"},
			},
			want: []want{{ms(990), BeepChannelRight}},
		},
		{
			name: "bleed at onset of the same run classifies at peak",
			frames: []logFrame{
				{ms(1000), "-57.0", "-30.0"},
				{ms(1010), "-18.0", "-52.0"},
				{ms(1020), "-18.5", "-53.0"},
			},
			want: []want{{ms(990), BeepChannelLeft}},
		},
		{
			name: "equal-level runs are debounced, not rejected",
			frames: []logFrame{
				{ms(1000), "-18.0", "-53.0"},
				{ms(1150), "-18.5", "-53.0"},
			},
			want: []want{{ms(990), BeepChannelLeft}},
		},
		{
			name: "consecutive beeps at 1s cadence are both kept",
			frames: []logFrame{
				{ms(1000), "-18.0", "-53.0"},
				{ms(2000), "-18.5", "-53.0"},
			},
			want: []want{
				{ms(990), BeepChannelLeft},
				{ms(1990), BeepChannelLeft},
			},
		},
		{
			name: "quiet run with no louder neighbor is still a beep",
			frames: []logFrame{
				{ms(1000), "-33.0", "-80.0"},
			},
			want: []want{{ms(990), BeepChannelLeft}},
		},
		{
			name: "dip below threshold shorter than runMergeGap does not split the run",
			frames: []logFrame{
				{ms(1000), "-20.0", "-60.0"},
				{ms(1010), "-40.0", "-60.0"},
				{ms(1020), "-18.0", "-60.0"},
			},
			want: []want{{ms(990), BeepChannelLeft}},
		},
		{
			name: "mono beep reports both channels",
			frames: []logFrame{
				{ms(1000), "-18.0", ""},
				{ms(1010), "-18.5", ""},
			},
			want: []want{{ms(990), BeepChannelBoth}},
		},
		{
			name: "digitally silent right channel reports left",
			frames: []logFrame{
				{ms(1000), "-18.0", "-inf"},
			},
			want: []want{{ms(990), BeepChannelLeft}},
		},
		{
			name: "both channels loud without dominance reports both",
			frames: []logFrame{
				{ms(1000), "-18.0", "-19.0"},
			},
			want: []want{{ms(990), BeepChannelBoth}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			beeps, err := parseBeepLog(writeBeepLog(t, tc.frames), "p0")
			if err != nil {
				t.Fatalf("parseBeepLog: %v", err)
			}
			if len(beeps) != len(tc.want) {
				t.Fatalf("got %d beeps (%+v), want %d", len(beeps), beeps, len(tc.want))
			}
			for i, w := range tc.want {
				if diff := absDuration(beeps[i].PTS - w.pts); diff > beepPTSTolerance {
					t.Errorf("beep %d: PTS=%s, want %s ±%s", i, beeps[i].PTS, w.pts, beepPTSTolerance)
				}
				if beeps[i].Channel != w.channel {
					t.Errorf("beep %d: channel=%d, want %d", i, beeps[i].Channel, w.channel)
				}
			}
		})
	}
}
