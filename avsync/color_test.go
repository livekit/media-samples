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
		{name: "white/p0", stats: regionStats{ymax: 235, umin: 128, vmin: 128, satmax: 0}, expected: "p0"},
		{name: "cyan/p1", stats: regionStats{ymax: 200, umin: 122, vmin: 11, satmax: 121}, expected: "p1"},
		{name: "yellow/p2", stats: regionStats{ymax: 210, umin: 9, vmin: 126, satmax: 119}, expected: "p2"},
		{name: "black/empty", stats: regionStats{ymax: 16, umin: 128, vmin: 128, satmax: 0}, expected: ""},
		{name: "dark/no content", stats: regionStats{ymax: 30, umin: 128, vmin: 128, satmax: 2}, expected: ""},
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
