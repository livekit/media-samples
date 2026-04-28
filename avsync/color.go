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
		r8 := float64(r >> 8)
		g8 := float64(g >> 8)
		b8 := float64(b >> 8)
		_, cb, cr := color.RGBToYCbCr(uint8(r8), uint8(g8), uint8(b8))
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
