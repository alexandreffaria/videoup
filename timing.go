package main

import (
	"math"
	"math/big"
	"sort"
)

var commonRates = []*big.Rat{big.NewRat(24000, 1001), big.NewRat(24, 1), big.NewRat(25, 1),
	big.NewRat(30000, 1001), big.NewRat(30, 1), big.NewRat(50, 1), big.NewRat(60000, 1001), big.NewRat(60, 1)}

// Timing is the output frame rate and, for every output frame, which source frame fills it.
type Timing struct {
	FPS     *big.Rat
	CFR     bool  // the source was constant-rate, so the map is 1:1
	Map     []int // output frame k shows source frame Map[k]
	VStart  float64
	Changed string // why the rate differs from the source's, empty when it doesn't
}

// planTiming decides the output rate and the frame map.
//
// A constant-rate source keeps its own rate exactly (29.97 stays 30000/1001) and maps 1:1.
// A variable-rate source -- most GIFs with mixed delays, phone footage -- has no single
// rate, and an edit timeline needs one, so it is laid onto a constant grid: each grid
// slot shows the source frame on screen at that moment (the rule of ffmpeg's fps
// filter). Duration is unchanged; a frame repeats or drops only where the grid demands.
func planTiming(info *Info, pts []float64, lastDur float64, vfrFPS *big.Rat) Timing {
	t0 := pts[0]
	rel := make([]float64, len(pts))
	for i, t := range pts {
		rel[i] = t - t0
	}
	if len(rel) == 1 { // a still saved as a GIF: one frame, held its own length
		fps := orRate(vfrFPS, big.NewRat(30, 1))
		hold := lastDur
		if hold <= 0 {
			hold = info.Duration
		}
		return Timing{FPS: fps, CFR: true, Map: make([]int, max(1, round(hold*f(fps)))), VStart: t0}
	}
	deltas := make([]float64, len(rel)-1)
	for i := range deltas {
		deltas[i] = rel[i+1] - rel[i]
	}
	cfr := info.Rate != nil
	if cfr {
		step := 1 / f(info.Rate)
		for _, d := range deltas {
			if math.Abs(d-step) >= 0.25*step {
				cfr = false
				break
			}
		}
	}
	tm := Timing{CFR: cfr, VStart: t0}
	switch {
	case cfr:
		tm.FPS = info.Rate
	case vfrFPS != nil:
		tm.FPS, tm.Changed = vfrFPS, "variable-rate source, --vfr-fps"
	default:
		avg := 0.0
		if info.AvgRate != nil {
			avg = f(info.AvgRate)
		} else if rel[len(rel)-1] > 0 {
			avg = float64(len(rel)-1) / rel[len(rel)-1]
		}
		tm.FPS = big.NewRat(30, 1)
		best := math.Inf(1)
		for _, c := range commonRates {
			if d := math.Abs(f(c) - avg); d < best {
				best, tm.FPS = d, c
			}
		}
		if avg == 0 || best/f(tm.FPS) > 0.03 {
			tm.FPS = big.NewRat(30, 1)
		}
		tm.Changed = "variable-rate source, nearest standard rate to its average"
	}
	tailDur := lastDur
	if tailDur <= 0 {
		tailDur = deltas[len(deltas)-1]
	}
	fps := f(tm.FPS)
	n := max(1, round((rel[len(rel)-1]+tailDur)*fps))
	slots := make([]int, len(rel))
	for i, t := range rel {
		slots[i] = round(t * fps)
	}
	tm.Map = make([]int, n)
	for k := range tm.Map {
		// last source frame whose slot is <= k
		tm.Map[k] = max(0, sort.Search(len(slots), func(i int) bool { return slots[i] > k })-1)
	}
	return tm
}

// needed is every source frame the map uses, once, in order: only these get upscaled.
func (t Timing) needed() []int {
	var out []int
	for i, v := range t.Map {
		if i == 0 || v != t.Map[i-1] {
			out = append(out, v)
		}
	}
	return out
}

func f(r *big.Rat) float64 { v, _ := r.Float64(); return v }

func round(x float64) int { return int(math.Round(x)) }

func orRate(a, b *big.Rat) *big.Rat {
	if a != nil {
		return a
	}
	return b
}
