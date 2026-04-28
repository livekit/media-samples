// Package avsync analyzes composited audio/video files for A/V sync markers.
package avsync

import (
	"image"
	"image/color"
	"time"
)

// Colors used by the test content generator for participant identification.
var (
	ColorWhite  = color.White
	ColorCyan   = color.RGBA{0, 255, 255, 255}
	ColorYellow = color.RGBA{255, 255, 0, 255}
)

// Pre-defined participants matching the test content generator output.
var (
	P0 = Participant{Name: "p0", Color: ColorWhite, BeepFreq: 523}
	P1 = Participant{Name: "p1", Color: ColorCyan, BeepFreq: 659}
	P2 = Participant{Name: "p2", Color: ColorYellow, BeepFreq: 784}

	AllParticipants = []Participant{P0, P1, P2}
)

type Config struct {
	FilePath     string
	Regions      []Region
	Participants []Participant
	Timeout      time.Duration // 0 = 5 minute default
}

type Region struct {
	Name string
	Rect image.Rectangle
}

type Participant struct {
	Name     string
	Color    color.Color
	BeepFreq float64
}

type Result struct {
	Video VideoResult
	Audio AudioResult
}

type VideoResult struct {
	Regions map[string][]RegionFrame
	Flashes map[string][]time.Duration
}

type RegionFrame struct {
	PTS         time.Duration
	Participant string
}

type AudioResult struct {
	Beeps   []Beep
	Silence []SilenceRange
}

type Beep struct {
	PTS         time.Duration
	Participant string
}

type SilenceRange struct {
	Start    time.Duration
	End      time.Duration
	Duration time.Duration
}
