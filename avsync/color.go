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
	"image/color"
	"math"
)

type regionStats struct {
	uavg float64
	vavg float64
}

type colorTarget struct {
	name string
	u    float64
	v    float64
}

type colorClassifier struct {
	targets []colorTarget
}

func newColorClassifier(participants []Participant) *colorClassifier {
	cc := &colorClassifier{}
	for _, p := range participants {
		r, g, b, _ := p.Color.RGBA()
		_, cb, cr := color.RGBToYCbCr(uint8(r>>8), uint8(g>>8), uint8(b>>8))
		cc.targets = append(cc.targets, colorTarget{
			name: p.Name,
			u:    float64(cb),
			v:    float64(cr),
		})
	}
	return cc
}

// Per-participant backgrounds are solid colors; the per-frame average
// U/V over the sampled region tracks the background closely (text and
// flash overlays add small amounts of neutral pixels but the
// participant chroma dominates). Nearest-neighbor in (U,V), with a max
// distance to reject neutral / out-of-gamut samples (e.g. a pure-white
// flash frame, an off-region black bar).
const maxClassifyDist = 40

func (cc *colorClassifier) classify(s regionStats) string {
	bestName := ""
	bestDist := math.MaxFloat64

	for _, t := range cc.targets {
		dist := math.Abs(s.uavg-t.u) + math.Abs(s.vavg-t.v)
		if dist < bestDist {
			bestDist = dist
			bestName = t.name
		}
	}

	if bestDist > maxClassifyDist {
		return ""
	}

	return bestName
}
