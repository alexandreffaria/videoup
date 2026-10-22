package main

import (
	"math/big"
	"testing"
)

func times(start float64, delays ...float64) []float64 {
	out := []float64{start}
	for _, d := range delays[:len(delays)-1] {
		out = append(out, out[len(out)-1]+d)
	}
	return out
}

func TestConstantRateKeepsRateAndMaps1to1(t *testing.T) {
	r := big.NewRat(30000, 1001)
	pts := make([]float64, 90)
	for i := range pts {
		pts[i] = float64(i) * 1001 / 30000
	}
	tm := planTiming(&Info{Rate: r}, pts, 1001.0/30000, nil)
	if !tm.CFR || tm.FPS.Cmp(r) != 0 || len(tm.Map) != 90 {
		t.Fatalf("cfr=%v fps=%v n=%d", tm.CFR, tm.FPS, len(tm.Map))
	}
	for i, v := range tm.Map {
		if v != i {
			t.Fatalf("map[%d]=%d", i, v)
		}
	}
}

func TestMixedGifDelaysLandOnNearestStandardRateWithSameDuration(t *testing.T) {
	var delays []float64
	for i := 0; i < 84; i++ {
		if i%2 == 0 {
			delays = append(delays, 0.03)
		} else {
			delays = append(delays, 0.07)
		}
	}
	pts := times(0, delays...)
	tm := planTiming(&Info{Rate: big.NewRat(30, 1), AvgRate: big.NewRat(25, 1)}, pts, 0.07, nil)
	if tm.CFR || tm.FPS.Cmp(big.NewRat(25, 1)) != 0 {
		t.Fatalf("cfr=%v fps=%v", tm.CFR, tm.FPS)
	}
	if got := float64(len(tm.Map)) / 25; got < 4.19 || got > 4.21 { // 42 * 0.1 s
		t.Fatalf("duration %.3f", got)
	}
	for k := 1; k < len(tm.Map); k++ {
		if tm.Map[k] < tm.Map[k-1] {
			t.Fatal("map goes backwards")
		}
	}
}

func TestStillIsHeldItsOwnLength(t *testing.T) {
	tm := planTiming(&Info{Rate: big.NewRat(100, 1)}, []float64{0}, 0.1, nil)
	if len(tm.Map) != 3 || tm.FPS.Cmp(big.NewRat(30, 1)) != 0 {
		t.Fatalf("n=%d fps=%v", len(tm.Map), tm.FPS)
	}
}

func TestNeededHasEachUsedFrameOnce(t *testing.T) {
	tm := Timing{Map: []int{0, 0, 1, 3, 3, 4}}
	got := tm.needed()
	want := []int{0, 1, 3, 4}
	if len(got) != len(want) {
		t.Fatalf("%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%v", got)
		}
	}
}

func TestArgsMayFollowPaths(t *testing.T) {
	o, err := parseArgs([]string{"a", "--codec", "h264", "b", "-", "--dry-run"})
	if err != nil {
		t.Fatal(err)
	}
	if o.Codec != "h264" || !o.DryRun || len(o.Paths) != 3 {
		t.Fatalf("%+v", o)
	}
}
