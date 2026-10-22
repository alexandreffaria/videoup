// videoup AI-upscales videos and GIFs with Real-ESRGAN (upscayl-bin) and returns one
// edit-ready file per source. See README.md for the design; `videoup -h` for usage.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/term"
)

var version = "dev" // set by the build: -ldflags "-X main.version=..."

func main() { os.Exit(run()) }

// Event is one line of the JSON output, or one update to the TUI.
type Event struct {
	Kind string
	Data map[string]any
}

func run() int {
	o, err := parseArgs(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintln(os.Stderr, "error:", err, "(videoup -h for usage)")
		return 2
	}
	if o.Version {
		fmt.Println("videoup", version)
		return 0
	}
	stdoutTTY := term.IsTerminal(int(os.Stdout.Fd()))
	stdinTTY := term.IsTerminal(int(os.Stdin.Fd()))
	jsonMode := o.JSON || (!o.Text && !stdoutTTY)
	emit := newEmitter(jsonMode)

	fail := func(err error) int {
		emit(Event{"error", map[string]any{"message": err.Error()}})
		return 2
	}
	if err := checkFFmpeg(); err != nil {
		return fail(err)
	}
	ups := findUpscalers(o.Upscaler)
	if o.ListModels {
		var list []map[string]any
		for _, u := range ups {
			list = append(list, map[string]any{"source": u.Source, "bin": u.Bin, "models": u.models()})
		}
		emit(Event{"models", map[string]any{"upscalers": list, "default": "high-fidelity-4x"}})
		return 0
	}
	up, err := pickUpscaler(ups, o.Model)
	if err != nil {
		return fail(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	sweepTemp(o.Tmp)

	readStdin := len(o.Paths) == 1 && o.Paths[0] == "-" || (len(o.Paths) == 0 && !stdinTTY)
	if readStdin {
		o.Paths = nil
		sc := bufio.NewScanner(os.Stdin)
		sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
		for sc.Scan() {
			// Windows PowerShell set to UTF-8 starts a pipe with a byte-order mark
			line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\ufeff"))
			if strings.HasPrefix(line, "{") { // our own JSON lines: take the source
				var ev map[string]any
				if json.Unmarshal([]byte(line), &ev) == nil {
					line, _ = ev["source"].(string)
					if line == "" {
						line, _ = ev["file"].(string)
					}
				}
			}
			if line != "" {
				o.Paths = append(o.Paths, line)
			}
		}
	}

	if !jsonMode && stdinTTY && stdoutTTY && !o.DryRun && !(len(o.Paths) > 0 && o.Yes) {
		return runTUI(ctx, o, up)
	}
	if len(o.Paths) == 0 {
		fmt.Fprintln(os.Stderr, "error: no paths given (videoup -h for usage)")
		return 2
	}
	fs, err := gather(o.Paths, o)
	if err != nil {
		return fail(err)
	}
	jobs := plan(ctx, fs, o)
	for i, j := range jobs {
		ev := j.row(o)
		ev["index"] = i + 1
		emit(Event{"plan", ev})
	}
	if !jsonMode {
		printPlan(os.Stderr, jobs, o)
	}
	var todo []*Job
	for _, j := range jobs {
		if j.Status == "todo" {
			todo = append(todo, j)
		}
	}
	if o.DryRun {
		emit(Event{"summary", map[string]any{"dry_run": true, "todo": len(todo), "total": len(jobs)}})
		return 0
	}
	return runJobs(ctx, todo, len(jobs)-len(todo), o, up, emit)
}

// runJobs processes the jobs in order and reports through emit. Returns the exit code.
func runJobs(ctx context.Context, todo []*Job, skipped int, o *Options, up Upscaler, emit func(Event)) int {
	t0 := time.Now()
	outputs := []string{}
	failed := 0
	for n, j := range todo {
		if ctx.Err() != nil {
			break
		}
		emit(Event{"start", map[string]any{"file": j.Source, "output": j.Output, "index": n + 1, "total": len(todo)}})
		var last time.Time
		res, err := process(ctx, j, o, up, func(p Progress) {
			if time.Since(last) < time.Second && p.Encoded < p.EncodeTotal {
				return
			}
			last = time.Now()
			emit(Event{"progress", map[string]any{"file": p.File, "stage": p.Stage, "upscaled": p.Upscaled,
				"upscale_total": p.UpscaleTotal, "encoded": p.Encoded, "encode_total": p.EncodeTotal,
				"elapsed": round3(p.Elapsed.Seconds())}})
		})
		for _, w := range j.Warnings {
			emit(Event{"warning", map[string]any{"file": j.Source, "message": w}})
		}
		switch {
		case ctx.Err() != nil:
			emit(Event{"interrupted", map[string]any{"file": j.Source}})
			return 130
		case err != nil:
			failed++
			emit(Event{"error", map[string]any{"file": j.Source, "message": err.Error()}})
		default:
			outputs = append(outputs, res.Output)
			emit(Event{"done", toMap(res)})
		}
	}
	emit(Event{"summary", map[string]any{"done": len(outputs), "skipped": skipped, "failed": failed,
		"seconds": round3(time.Since(t0).Seconds()), "outputs": outputs}})
	if failed > 0 {
		return 1
	}
	return 0
}

// newEmitter writes JSON lines to stdout, or short human lines to stderr.
func newEmitter(jsonMode bool) func(Event) {
	var mu sync.Mutex
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	tty := term.IsTerminal(int(os.Stderr.Fd()))
	return func(e Event) {
		mu.Lock()
		defer mu.Unlock()
		if jsonMode {
			m := map[string]any{"event": e.Kind}
			for k, v := range e.Data {
				m[k] = v
			}
			enc.Encode(m)
			return
		}
		d := e.Data
		clear := ""
		if tty {
			clear = "\r\x1b[2K"
		}
		switch e.Kind {
		case "progress":
			if tty {
				fmt.Fprintf(os.Stderr, "\r  %-9s upscaled %v/%v  encoded %v/%v  %4.0fs", d["stage"], d["upscaled"],
					d["upscale_total"], d["encoded"], d["encode_total"], d["elapsed"])
			}
		case "start":
			fmt.Fprintf(os.Stderr, "%s[%v/%v] %s\n", clear, d["index"], d["total"], d["file"])
		case "done":
			if d["still"] == true {
				fmt.Fprintf(os.Stderr, "%s  -> %s  %vx%v  still image  in %.0fs\n", clear, d["output"], d["width"],
					d["height"], d["seconds"])
				break
			}
			fmt.Fprintf(os.Stderr, "%s  -> %s  %vx%v  %v fps  %.2fs (source %.2fs)  in %.0fs\n", clear, d["output"],
				d["width"], d["height"], d["fps"], d["duration_output"], d["duration_source"], d["seconds"])
		case "error", "warning", "interrupted":
			fmt.Fprintf(os.Stderr, "%s  %s: %v %v\n", clear, e.Kind, valueOr(d["file"], ""), valueOr(d["message"], ""))
		case "summary":
			if _, dry := d["dry_run"]; dry {
				fmt.Fprintf(os.Stderr, "%v to do of %v (dry run)\n", d["todo"], d["total"])
			} else {
				fmt.Fprintf(os.Stderr, "%s%v done, %v skipped, %v failed in %.0fs\n", clear, d["done"], d["skipped"],
					d["failed"], d["seconds"])
			}
		case "models":
			for _, u := range d["upscalers"].([]map[string]any) {
				fmt.Printf("%s  %s\n", u["source"], u["bin"])
				for _, m := range u["models"].([]string) {
					fmt.Printf("    %s\n", m)
				}
			}
		}
	}
}

func printPlan(w *os.File, jobs []*Job, o *Options) {
	fmt.Fprintf(w, "\n  %3s  %-44s %23s %8s %7s  %s\n", "#", "file", "size", "fps", "sec", "status")
	for i, j := range jobs {
		size, fps, dur := "", "", ""
		if in := j.Info; in != nil {
			size = fmt.Sprintf("%dx%d -> %dx%d", in.Width, in.Height, in.Width*o.Scale, in.Height*o.Scale)
			if in.Rate != nil {
				fps = fmt.Sprintf("%.4g", f(in.Rate))
			}
			if in.Still {
				fps, size, dur = "still", size+" png", ""
			}
			dur = fmt.Sprintf("%.2f", in.Duration)
		}
		status := j.Status
		if j.Error != "" {
			status += " (" + j.Error + ")"
		}
		fmt.Fprintf(w, "  %3d  %-44s %23s %8s %7s  %s\n", i+1, clip(base(j.Source), 44), size, fps, dur, status)
	}
	fmt.Fprintln(w)
}

// sweepTemp removes scratch folders a killed run left behind (a clean exit removes its
// own). Only folders in the scratch location, named videoup_*, older than half a day.
func sweepTemp(dir string) {
	if dir == "" {
		dir = os.TempDir()
	}
	old, _ := filepath.Glob(filepath.Join(dir, "videoup_*"))
	for _, d := range old {
		if st, err := os.Stat(d); err == nil && st.IsDir() && time.Since(st.ModTime()) > 12*time.Hour {
			os.RemoveAll(d)
		}
	}
}

func checkFFmpeg() error {
	hint := map[string]string{"windows": "winget install Gyan.FFmpeg", "darwin": "brew install ffmpeg"}[runtime.GOOS]
	if hint == "" {
		hint = "sudo apt install ffmpeg (or your distro's package)"
	}
	for _, t := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(t); err != nil {
			return fmt.Errorf("%s is not on PATH; install it: %s", t, hint)
		}
	}
	return nil
}

func toMap(v any) map[string]any {
	b, _ := json.Marshal(v)
	var m map[string]any
	json.Unmarshal(b, &m)
	return m
}

func valueOr(v any, d string) any {
	if v == nil {
		return d
	}
	return v
}

func base(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n-1]) + "~"
	}
	return s
}
