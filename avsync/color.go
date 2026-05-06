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
	ymax   float64
	umin   float64
	vmin   float64
	satmax float64
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

const minBrightness = 50

func (cc *colorClassifier) classify(s regionStats) string {
	if s.ymax < minBrightness {
		return ""
	}

	bestName := ""
	bestDist := math.MaxFloat64

	for _, t := range cc.targets {
		var dist float64
		isTargetNeutral := math.Abs(t.u-128) < 5 && math.Abs(t.v-128) < 5
		isObsNeutral := s.satmax < 20

		if isTargetNeutral && isObsNeutral {
			dist = 0
		} else if isTargetNeutral != isObsNeutral {
			dist = 1000
		} else {
			dist = math.Abs(s.umin-t.u) + math.Abs(s.vmin-t.v)
		}

		if dist < bestDist {
			bestDist = dist
			bestName = t.name
		}
	}

	if bestDist > 100 {
		return ""
	}

	return bestName
}
