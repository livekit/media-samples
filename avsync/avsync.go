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
	"image/color"
	"time"
)

// Colors used by the test content generator for participant identification.
var (
	ColorWhite  = color.White
	ColorCyan   = color.RGBA{R: 0, G: 255, B: 255, A: 255}
	ColorYellow = color.RGBA{R: 255, G: 255, B: 0, A: 255}
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
