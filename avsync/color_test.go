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
)

func TestClassifyColor(t *testing.T) {
	participants := AllParticipants

	tests := []struct {
		name     string
		stats    regionStats
		expected string
	}{
		// Target U/V points (see avsync.go RGB values):
		//   red:   U=112, V=176
		//   green: U=96,  V=88
		//   blue:  U=176, V=120
		{name: "red/p0 exact", stats: regionStats{uavg: 112, vavg: 176}, expected: "p0"},
		{name: "green/p1 exact", stats: regionStats{uavg: 96, vavg: 88}, expected: "p1"},
		{name: "blue/p2 exact", stats: regionStats{uavg: 176, vavg: 120}, expected: "p2"},
		{name: "red/p0 with white-text pull", stats: regionStats{uavg: 118, vavg: 168}, expected: "p0"},
		{name: "green/p1 with white-text pull", stats: regionStats{uavg: 104, vavg: 96}, expected: "p1"},
		{name: "blue/p2 with white-text pull", stats: regionStats{uavg: 168, vavg: 122}, expected: "p2"},
		{name: "neutral/white flash", stats: regionStats{uavg: 128, vavg: 128}, expected: ""},
		{name: "near-neutral/no content", stats: regionStats{uavg: 130, vavg: 126}, expected: ""},
	}

	cc := newColorClassifier(participants)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cc.classify(tt.stats)
			if got != tt.expected {
				t.Errorf("classify(%v) = %q, want %q", tt.stats, got, tt.expected)
			}
		})
	}
}
