package main

import (
	"context"
	"math/big"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestTUIScreens(t *testing.T) {
	o, _ := parseArgs(nil)
	m := &tui{ctx: context.Background(), cancel: func() {}, o: o, send: func(tea.Msg) {}}
	if !strings.Contains(m.View(), "Folder (or file) to upscale") {
		t.Fatal("folder screen missing")
	}
	jobs := []*Job{
		{Source: "/a/clip.gif", Status: "todo", Info: &Info{Width: 400, Height: 300, Rate: big.NewRat(20, 1), Duration: 3}},
		{Source: "/a/done.mp4", Status: "exists", Info: &Info{Width: 640, Height: 360, Rate: big.NewRat(30000, 1001), Duration: 5, Audio: 1}},
		{Source: "/a/bad.mov", Status: "unreadable", Error: "no video stream"},
	}
	m.Update(scannedMsg{jobs: jobs})
	v := m.View()
	for _, want := range []string{"3 file(s), 1 selected", "[x] clip.gif", "1600x1200", "29.97", "(done already)", "no video stream"} {
		if !strings.Contains(v, want) {
			t.Fatalf("plan screen lacks %q:\n%s", want, v)
		}
	}
	m.Update(key("down"))
	m.Update(key(" ")) // pick the one already done, to redo it
	m.Update(key("down"))
	m.Update(key(" ")) // the unreadable one cannot be picked
	if !m.pick[1] || m.pick[2] {
		t.Fatalf("picks %v", m.pick)
	}
	m.Update(key("a")) // all -> none
	if m.pick[0] || m.pick[1] {
		t.Fatalf("a should clear: %v", m.pick)
	}
	m.Update(key("enter")) // nothing picked: stays on the plan
	if m.sc != scPlan {
		t.Fatal("started with nothing picked")
	}
	m.Update(key("esc"))
	if m.sc != scFolder {
		t.Fatal("esc should go back to the folder")
	}
	m.sc = scRun
	m.onEvent(Event{"start", map[string]any{"file": "/a/clip.gif", "index": 1, "total": 1}})
	m.onEvent(Event{"progress", map[string]any{"stage": "upscale", "upscaled": 30, "upscale_total": 60, "encoded": 10, "encode_total": 60, "elapsed": 4.0}})
	m.onEvent(Event{"done", map[string]any{"output": "/a/clip_x4.mov", "width": 1600, "height": 1200, "fps": "20", "duration_output": 3.0, "seconds": 12.0}})
	v = m.View()
	for _, want := range []string{"[1/1] clip.gif", "upscale", "30/60", "clip_x4.mov", "ctrl+c cancel"} {
		if !strings.Contains(v, want) {
			t.Fatalf("run screen lacks %q:\n%s", want, v)
		}
	}
}
