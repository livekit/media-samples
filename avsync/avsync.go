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

// Colors used by the test content generator as the full-frame background
// for each participant. Chosen dark enough that a white flash (Y≈235)
// stands out from the background's luma, and far enough apart in U/V to
// classify reliably from any sampled region.
var (
	ColorRed   = color.RGBA{R: 128, G: 32, B: 32, A: 255}
	ColorGreen = color.RGBA{R: 32, G: 128, B: 32, A: 255}
	ColorBlue  = color.RGBA{R: 32, G: 32, B: 128, A: 255}
)

// Pre-defined participants matching the test content generator output.
var (
	P0 = Participant{Name: "p0", Color: ColorRed, BeepFreq: 523}
	P1 = Participant{Name: "p1", Color: ColorGreen, BeepFreq: 659}
	P2 = Participant{Name: "p2", Color: ColorBlue, BeepFreq: 784}

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
	Beeps   []Beep
	Flashes []Flash
}

// BeepChannel identifies which stereo channel(s) a beep was detected on.
type BeepChannel int

const (
	// BeepChannelBoth indicates the beep was detected on both stereo channels
	// (or on the only channel for mono input). Most tests produce this.
	BeepChannelBoth BeepChannel = 0
	// BeepChannelLeft indicates the beep was detected only on the left channel.
	// Used by audio routing tests where a participant is mapped to one channel.
	BeepChannelLeft BeepChannel = 1
	// BeepChannelRight indicates the beep was detected only on the right channel.
	BeepChannelRight BeepChannel = 2
)

type Beep struct {
	PTS         time.Duration
	Participant string
	Channel     BeepChannel
}

type Flash struct {
	Region      string
	PTS         time.Duration
	Participant string
}
