package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The interactive mode: folder -> plan (review, toggle) -> run (live bars) -> done.
// It drives the same runJobs as the JSON and text modes; events arrive as messages.

type screen int

const (
	scFolder screen = iota
	scScanning
	scPlan
	scRun
	scDone
)

var (
	stTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FAFAFA")).Background(lipgloss.Color("#7D56F4")).Padding(0, 1)
	stDim   = lipgloss.NewStyle().Foreground(lipgloss.Color("#808080"))
	stOK    = lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575"))
	stErr   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5F5F"))
	stCur   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD75F")).Bold(true)
)

type scannedMsg struct {
	jobs []*Job
	err  error
}
type eventMsg Event
type finishedMsg int

type tui struct {
	ctx    context.Context
	cancel context.CancelFunc
	o      *Options
	up     Upscaler
	send   func(tea.Msg)

	sc       screen
	input    textinput.Model
	err      string
	jobs     []*Job
	pick     []bool
	cursor   int
	barUp    progress.Model
	barEnc   progress.Model
	current  map[string]any
	progress map[string]any
	log      []string
	summary  map[string]any
	code     int
	started  time.Time
	width    int
}

func runTUI(ctx context.Context, o *Options, up Upscaler) int {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	in := textinput.New()
	in.Placeholder = "drop a folder here, or type a path"
	if wd, err := os.Getwd(); err == nil {
		in.SetValue(wd)
	}
	in.Focus()
	in.Width = 80
	m := &tui{ctx: ctx, cancel: cancel, o: o, up: up, input: in,
		barUp: progress.New(progress.WithDefaultGradient()), barEnc: progress.New(progress.WithDefaultGradient())}
	if len(o.Paths) > 0 {
		m.sc = scScanning
	}
	p := tea.NewProgram(m)
	m.send = p.Send
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 2
	}
	return m.code
}

func (m *tui) Init() tea.Cmd {
	if m.sc == scScanning {
		return m.scan(m.o.Paths)
	}
	return textinput.Blink
}

func (m *tui) scan(paths []string) tea.Cmd {
	return func() tea.Msg {
		fs, err := gather(paths, m.o)
		if err != nil {
			return scannedMsg{err: err}
		}
		return scannedMsg{jobs: plan(m.ctx, fs, m.o)}
	}
}

func (m *tui) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		w := min(60, max(20, msg.Width-30))
		m.barUp.Width, m.barEnc.Width = w, w
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			if m.sc == scRun {
				m.cancel() // runJobs stops, temp is cleaned, then finishedMsg quits
				m.log = append(m.log, stErr.Render("cancelling..."))
				return m, nil
			}
			m.code = 130
			return m, tea.Quit
		}
	case scannedMsg:
		if msg.err != nil {
			m.sc, m.err = scFolder, msg.err.Error()
			return m, nil
		}
		m.jobs, m.pick, m.cursor, m.err = msg.jobs, make([]bool, len(msg.jobs)), 0, ""
		for i, j := range m.jobs {
			m.pick[i] = j.Status == "todo"
		}
		m.sc = scPlan
		if len(m.jobs) == 0 {
			m.sc, m.err = scFolder, "no gif or video files in there"
		}
		return m, nil
	case eventMsg:
		m.onEvent(Event(msg))
		return m, nil
	case finishedMsg:
		m.code, m.sc = int(msg), scDone
		return m, tea.Quit
	}

	switch m.sc {
	case scFolder:
		if k, ok := msg.(tea.KeyMsg); ok {
			switch k.String() {
			case "enter":
				path := strings.Trim(strings.TrimSpace(m.input.Value()), `"'`)
				if path == "" {
					return m, nil
				}
				m.sc = scScanning
				return m, m.scan([]string{path})
			case "esc":
				return m, tea.Quit
			}
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	case scPlan:
		if k, ok := msg.(tea.KeyMsg); ok {
			switch k.String() {
			case "up", "k":
				m.cursor = max(0, m.cursor-1)
			case "down", "j":
				m.cursor = min(len(m.jobs)-1, m.cursor+1)
			case " ", "x":
				if m.jobs[m.cursor].Info != nil {
					m.pick[m.cursor] = !m.pick[m.cursor]
				}
			case "a":
				all := true
				for i, j := range m.jobs {
					all = all && (m.pick[i] || j.Info == nil)
				}
				for i, j := range m.jobs {
					m.pick[i] = !all && j.Info != nil
				}
			case "esc", "backspace":
				m.sc = scFolder
			case "q":
				return m, tea.Quit
			case "enter":
				var todo []*Job
				for i, j := range m.jobs {
					if m.pick[i] {
						todo = append(todo, j)
					}
				}
				if len(todo) == 0 {
					return m, nil
				}
				m.sc, m.started = scRun, time.Now()
				skipped := len(m.jobs) - len(todo)
				go func() {
					code := runJobs(m.ctx, todo, skipped, m.o, m.up, func(e Event) { m.send(eventMsg(e)) })
					m.send(finishedMsg(code))
				}()
			}
		}
	case scDone:
		if _, ok := msg.(tea.KeyMsg); ok {
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *tui) onEvent(e Event) {
	d := e.Data
	switch e.Kind {
	case "start":
		m.current, m.progress = d, nil
	case "progress":
		m.progress = d
	case "done":
		if d["still"] == true {
			m.log = append(m.log, stOK.Render("done ")+fmt.Sprintf("%s  %vx%v  still image  (%.0fs)",
				base(fmt.Sprint(d["output"])), d["width"], d["height"], d["seconds"]))
			break
		}
		m.log = append(m.log, stOK.Render("done ")+fmt.Sprintf("%s  %vx%v  %v fps  %.2fs  (%.0fs)",
			base(fmt.Sprint(d["output"])), d["width"], d["height"], d["fps"], d["duration_output"], d["seconds"]))
	case "error":
		m.log = append(m.log, stErr.Render("FAIL ")+base(fmt.Sprint(d["file"]))+": "+fmt.Sprint(d["message"]))
	case "warning":
		m.log = append(m.log, stCur.Render("warn ")+base(fmt.Sprint(d["file"]))+": "+fmt.Sprint(d["message"]))
	case "summary":
		m.summary = d
	}
}

func (m *tui) View() string {
	var b strings.Builder
	head := fmt.Sprintf(" videoup %s ", version)
	b.WriteString(stTitle.Render(head) + stDim.Render(fmt.Sprintf("  %s x%d -> %s   upscaler: %s",
		m.o.Model, m.o.Scale, m.o.Codec, m.up.Source)) + "\n\n")
	switch m.sc {
	case scFolder:
		b.WriteString("Folder (or file) to upscale:\n\n" + m.input.View() + "\n\n")
		if m.err != "" {
			b.WriteString(stErr.Render(m.err) + "\n\n")
		}
		b.WriteString(stDim.Render("enter scan   esc quit"))
	case scScanning:
		b.WriteString("Reading the files...")
	case scPlan:
		n := 0
		for _, p := range m.pick {
			if p {
				n++
			}
		}
		b.WriteString(fmt.Sprintf("%d file(s), %d selected\n\n", len(m.jobs), n))
		lo := max(0, min(m.cursor-10, len(m.jobs)-20))
		for i := lo; i < min(len(m.jobs), lo+20); i++ {
			j := m.jobs[i]
			box := "[ ]"
			if m.pick[i] {
				box = "[x]"
			}
			line := fmt.Sprintf("%s %-40s", box, clip(base(j.Source), 40))
			if in := j.Info; in != nil {
				fps := "?"
				if in.Rate != nil {
					fps = fmt.Sprintf("%.4g", f(in.Rate))
				}
				if in.Still {
					fps = "still"
				}
				line += fmt.Sprintf(" %4dx%-4d -> %5dx%-5d %6s fps %7.2fs", in.Width, in.Height,
					in.Width*m.o.Scale, in.Height*m.o.Scale, fps, in.Duration)
				if in.Audio > 0 {
					line += "  audio"
				}
			}
			switch j.Status {
			case "exists":
				line += stDim.Render("  (done already)")
			case "unreadable":
				line += stErr.Render("  " + j.Error)
			}
			if i == m.cursor {
				line = stCur.Render(">") + line
			} else {
				line = " " + line
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n" + stDim.Render("space toggle   a all/none   enter start   esc back   q quit"))
	case scRun, scDone:
		if m.current != nil && m.sc == scRun {
			b.WriteString(fmt.Sprintf("[%v/%v] %s\n\n", m.current["index"], m.current["total"], base(fmt.Sprint(m.current["file"]))))
			if p := m.progress; p != nil {
				ratio := func(a, t any) float64 {
					x, y := toF(a), toF(t)
					if y == 0 {
						return 0
					}
					return x / y
				}
				b.WriteString(fmt.Sprintf("  upscale %s %v/%v\n", m.barUp.ViewAs(ratio(p["upscaled"], p["upscale_total"])), p["upscaled"], p["upscale_total"]))
				b.WriteString(fmt.Sprintf("  encode  %s %v/%v\n", m.barEnc.ViewAs(ratio(p["encoded"], p["encode_total"])), p["encoded"], p["encode_total"]))
				b.WriteString(stDim.Render(fmt.Sprintf("  %s, %.0fs on this file, %s total", p["stage"], toF(p["elapsed"]),
					time.Since(m.started).Round(time.Second))) + "\n")
			} else {
				b.WriteString(stDim.Render("  starting...") + "\n")
			}
			b.WriteString("\n")
		}
		for _, l := range m.log[max(0, len(m.log)-12):] {
			b.WriteString(l + "\n")
		}
		if s := m.summary; s != nil {
			b.WriteString(fmt.Sprintf("\n%v done, %v skipped, %v failed in %s\n", s["done"], s["skipped"], s["failed"],
				time.Since(m.started).Round(time.Second)))
		}
		if m.sc == scRun {
			b.WriteString("\n" + stDim.Render("ctrl+c cancel (cleans up)"))
		}
	}
	return b.String() + "\n"
}

func toF(v any) float64 {
	switch x := v.(type) {
	case int:
		return float64(x)
	case float64:
		return x
	}
	return 0
}
