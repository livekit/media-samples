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
