package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func names(dir string, fmtStr string, nums ...int) []string {
	var out []string
	for _, n := range nums {
		out = append(out, filepath.Join(dir, fmt.Sprintf(fmtStr, n)))
	}
	return out
}

func TestFindSequences(t *testing.T) {
	d := filepath.Join("x", "shot")
	files := names(d, "render_%04d.exr", 1, 2, 3, 5)                          // one hole
	files = append(files, names(d, "img%d.jpg", 8, 9, 10, 11)...)             // unpadded, crosses a width
	files = append(files, names(d, "Shot %d.png", 103045, 104512, 111307)...) // timestamps
	files = append(files, names(d, "logo%d.png", 1, 2)...)                    // too few
	files = append(files, filepath.Join(d, "clip.mp4"))
	seqs, rest := findSequences(files)
	if len(seqs) != 2 {
		t.Fatalf("want 2 sequences, got %d: %+v", len(seqs), seqs)
	}
	r := seqs[0]
	if r.Pattern() != filepath.Join(d, "render_####.exr") || r.Name() != "render" || r.Missing() != 1 {
		t.Errorf("render: %s %s missing %d", r.Pattern(), r.Name(), r.Missing())
	}
	if !reflect.DeepEqual(r.Map(), []int{0, 1, 2, 2, 3}) {
		t.Errorf("render map %v", r.Map())
	}
	if j := seqs[1]; j.Pattern() != filepath.Join(d, "img#.jpg") || j.Pad != 0 || len(j.Map()) != 4 {
		t.Errorf("img: %s pad %d map %v", j.Pattern(), j.Pad, j.Map())
	}
	if len(rest) != 6 {
		t.Errorf("want 6 left as files, got %v", rest)
	}
}

func TestStepAndName(t *testing.T) {
	s := asSequence(seqKey{"d", "", ".png"}, names("d", "%03d.png", 10, 20, 30, 50))
	if s == nil || s.Step != 10 || !reflect.DeepEqual(s.Map(), []int{0, 1, 2, 2, 3}) || s.Name() != "d" {
		t.Fatalf("got %+v", s)
	}
}

func TestOpenPattern(t *testing.T) {
	d := t.TempDir()
	for _, n := range names(d, "a_%04d.png", 1, 2, 3) {
		os.WriteFile(n, nil, 0o644)
	}
	os.WriteFile(filepath.Join(d, "a_00001.png"), nil, 0o644) // other width: not this pattern
	s, err := openPattern(filepath.Join(d, "a_####.png"))
	if err != nil || len(s.Files) != 3 {
		t.Fatalf("got %+v, %v", s, err)
	}
	if _, err := openPattern(filepath.Join(d, "b_####.png")); err == nil {
		t.Error("a pattern with no frames should fail")
	}
}

func TestGatherUpscaled(t *testing.T) {
	d := t.TempDir()
	for _, n := range []string{"clip.mov", "clip_x4.mov"} {
		os.WriteFile(filepath.Join(d, n), nil, 0o644)
	}
	o, _ := parseArgs(nil)
	fs, err := gather([]string{d}, o)
	if err != nil || len(fs) != 2 || fs[0].upscaled || !fs[1].upscaled {
		t.Fatalf("folder: %+v %v", fs, err)
	}
	fs, err = gather([]string{filepath.Join(d, "clip_x4.mov")}, o) // named directly: a normal source
	if err != nil || len(fs) != 1 || fs[0].upscaled {
		t.Fatalf("file: %+v %v", fs, err)
	}
}
