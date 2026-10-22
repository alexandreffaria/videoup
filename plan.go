package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Options struct {
	Paths      []string
	Model      string
	Scale      int
	Codec      string
	OutDir     string
	Suffix     string
	VFRFPS     *big.Rat
	Force      bool
	DryRun     bool
	Yes        bool
	NoRecurse  bool
	JSON, Text bool
	ListModels bool
	Upscaler   string
	GPU, Tile  int
	Batch      int
	Tmp        string
	KeepTemp   bool
	Version    bool
}

type Job struct {
	Source, Output, Root string
	Info                 *Info
	Status               string // todo, exists, unreadable
	Error                string
	Warnings             []string
}

var extensions = map[string]bool{".gif": true, ".mov": true, ".mp4": true, ".m4v": true, ".mkv": true,
	".webm": true, ".avi": true, ".mpg": true, ".mpeg": true, ".mxf": true, ".wmv": true, ".flv": true,
	".ts": true, ".apng": true, ".3gp": true}

var skipDirs = map[string]bool{"to_delete": true, "_to_delete": true, ".git": true, "node_modules": true}

const usageText = `videoup -- AI-upscale videos and GIFs (Real-ESRGAN via upscayl-bin) into edit-ready files.

Every source becomes ONE file beside it, <name>_x4.mov (ProRes 4444): same duration and
frame rate, audio in sync, alpha kept. Frames live only in the system temp folder.

usage:
  videoup                                   interactive: pick a folder, review, run
  videoup <folder|file>... [options]        batch (options may come before or after paths)
  videoup --dry-run --json <folder>         the plan as JSON lines, nothing runs
  ... | videoup -                           paths, or videoup's own JSON lines, from stdin

output: JSON lines on stdout when stdout is not a terminal (or --json); progress on stderr.
exit:   0 ok / skipped, 1 a file failed, 2 usage or missing dependency, 130 interrupted.

options:
`

func parseArgs(args []string) (*Options, error) {
	o := &Options{}
	fs := flag.NewFlagSet("videoup", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.Model, "model", "high-fidelity-4x", "upscaling model (see --list-models)")
	fs.IntVar(&o.Scale, "scale", 4, "output scale: 2, 3 or 4")
	fs.StringVar(&o.Codec, "codec", "prores4444", "prores4444 (keeps alpha) | prores422hq | h264 (review copy, .mp4)")
	fs.StringVar(&o.OutDir, "out-dir", "", "write here, mirroring folders, instead of beside each source")
	fs.StringVar(&o.Suffix, "suffix", "", "output name suffix (default _x<scale>)")
	fs.Func("vfr-fps", "frame rate for variable-rate sources, e.g. 30 or 30000/1001 (default: nearest standard)", func(s string) error {
		if o.VFRFPS = rate(s); o.VFRFPS == nil {
			return fmt.Errorf("bad rate %q", s)
		}
		return nil
	})
	fs.BoolVar(&o.Force, "force", false, "redo sources whose output exists (overwrites it)")
	fs.BoolVar(&o.DryRun, "dry-run", false, "show the plan, upscale nothing")
	fs.BoolVar(&o.Yes, "yes", false, "start without the review screen")
	fs.BoolVar(&o.Yes, "y", false, "same as --yes")
	fs.BoolVar(&o.NoRecurse, "no-recurse", false, "only the top level of each folder")
	fs.BoolVar(&o.JSON, "json", false, "JSON lines on stdout (default when piped)")
	fs.BoolVar(&o.Text, "text", false, "plain text even when piped")
	fs.BoolVar(&o.ListModels, "list-models", false, "list the models every found upscaler has, then exit")
	fs.StringVar(&o.Upscaler, "upscaler", "", "upscayl-bin path or Upscayl resources folder (default: built in)")
	fs.IntVar(&o.GPU, "gpu", -1, "GPU id (default: auto)")
	fs.IntVar(&o.Tile, "tile", 0, "tile size; lower it if the GPU runs out of memory (default: auto)")
	fs.IntVar(&o.Batch, "batch", 120, "frames per upscaler call; bounds temp disk use")
	fs.StringVar(&o.Tmp, "tmp", "", "scratch folder (default: system temp)")
	fs.BoolVar(&o.KeepTemp, "keep-temp", false, "keep the scratch frames, for debugging")
	fs.BoolVar(&o.Version, "version", false, "print the version")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, usageText)
		fs.SetOutput(os.Stderr)
		fs.PrintDefaults()
		fs.SetOutput(io.Discard)
	}
	for { // flags may sit anywhere among the paths
		if err := fs.Parse(args); err != nil {
			if err == flag.ErrHelp {
				fs.Usage()
			}
			return nil, err
		}
		if fs.NArg() == 0 {
			break
		}
		o.Paths = append(o.Paths, fs.Arg(0))
		args = fs.Args()[1:]
	}
	if _, ok := codecs[o.Codec]; !ok {
		return nil, fmt.Errorf("unknown --codec %q", o.Codec)
	}
	if o.Scale < 2 || o.Scale > 4 {
		return nil, fmt.Errorf("--scale must be 2, 3 or 4")
	}
	if o.Batch < 1 {
		o.Batch = 1
	}
	if o.Suffix == "" {
		o.Suffix = fmt.Sprintf("_x%d", o.Scale)
	}
	return o, nil
}

type found struct{ path, root string }

// gather lists every media file in the given paths, recursing into folders.
func gather(paths []string, o *Options) ([]found, error) {
	var out []found
	seen := map[string]bool{}
	for _, p := range paths {
		p = strings.Trim(strings.TrimSpace(p), `"'`)
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, err
		}
		st, err := os.Stat(abs)
		if err != nil {
			return nil, fmt.Errorf("not found: %s", p)
		}
		var files []string
		root := ""
		if st.IsDir() {
			root = abs
			filepath.WalkDir(abs, func(q string, d os.DirEntry, err error) error {
				if err != nil {
					return nil
				}
				if d.IsDir() && q != abs && (skipDirs[strings.ToLower(d.Name())] || o.NoRecurse) {
					return filepath.SkipDir
				}
				if !d.IsDir() {
					files = append(files, q)
				}
				return nil
			})
			sort.Strings(files)
		} else {
			files = []string{abs}
		}
		for _, f := range files {
			stem := strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))
			if !extensions[strings.ToLower(filepath.Ext(f))] || strings.HasSuffix(stem, o.Suffix) ||
				strings.HasSuffix(f, ".part") || seen[strings.ToLower(f)] {
				continue
			}
			seen[strings.ToLower(f)] = true
			out = append(out, found{f, root})
		}
	}
	return out, nil
}

func outputPath(f found, o *Options) string {
	stem := strings.TrimSuffix(filepath.Base(f.path), filepath.Ext(f.path))
	dir := filepath.Dir(f.path)
	if o.OutDir != "" {
		dir = o.OutDir
		if f.root != "" {
			if rel, err := filepath.Rel(f.root, filepath.Dir(f.path)); err == nil {
				dir = filepath.Join(o.OutDir, rel)
			}
		}
	}
	return filepath.Join(dir, stem+o.Suffix+codecs[o.Codec].Ext)
}

// imageLike are containers that often hold a single still; only these get their frames
// counted at plan time, since counting means decoding the whole file.
var imageLike = map[string]bool{".gif": true, ".apng": true}

// plan probes every source and decides its output and status. A single-frame source
// (a still saved as a GIF) becomes an upscaled PNG, not a one-frame movie.
func plan(ctx context.Context, fs []found, o *Options) []*Job {
	var jobs []*Job
	for _, f := range fs {
		j := &Job{Source: f.path, Root: f.root, Status: "todo"}
		info, err := probe(ctx, f.path)
		if err != nil {
			j.Status, j.Error = "unreadable", err.Error()
		} else {
			j.Info = info
			if imageLike[strings.ToLower(filepath.Ext(f.path))] {
				if pts, _, err := frameTimes(ctx, f.path); err == nil && len(pts) == 1 {
					info.Still = true
				}
			}
		}
		jobs = append(jobs, j)
	}
	ext := func(j *Job) string {
		if j.Info != nil && j.Info.Still {
			return ".png"
		}
		return codecs[o.Codec].Ext
	}
	count := map[string]int{}
	for i, j := range jobs {
		count[strings.ToLower(swapExt(outputPath(fs[i], o), ext(j)))]++
	}
	for i, j := range jobs {
		out := swapExt(outputPath(fs[i], o), ext(j))
		if count[strings.ToLower(out)] > 1 { // clip.gif and clip.mp4 side by side
			src := strings.ToLower(strings.TrimPrefix(filepath.Ext(j.Source), "."))
			stem := strings.TrimSuffix(filepath.Base(j.Source), filepath.Ext(j.Source))
			out = filepath.Join(filepath.Dir(out), stem+"_"+src+o.Suffix+ext(j))
		}
		j.Output = out
		if j.Info == nil {
			continue
		}
		if _, err := os.Stat(out); err == nil && !o.Force {
			j.Status = "exists"
		}
		if w, h := j.Info.Width*o.Scale, j.Info.Height*o.Scale; max(w, h) > 8192 {
			j.Warnings = append(j.Warnings, fmt.Sprintf("output %dx%d is past 8K; consider --scale 2", w, h))
		}
	}
	return jobs
}

func swapExt(p, ext string) string { return strings.TrimSuffix(p, filepath.Ext(p)) + ext }

func (j *Job) row(o *Options) map[string]any {
	r := map[string]any{"source": j.Source, "output": j.Output, "status": j.Status}
	if i := j.Info; i != nil {
		r["width"], r["height"] = i.Width, i.Height
		r["out_width"], r["out_height"] = i.Width*o.Scale, i.Height*o.Scale
		r["duration"], r["audio_streams"], r["alpha"], r["still"] = round3(i.Duration), i.Audio, i.Alpha, i.Still
		if i.Rate != nil {
			r["fps"] = i.Rate.RatString()
		}
	}
	if j.Error != "" {
		r["error"] = j.Error
	}
	if len(j.Warnings) > 0 {
		r["warnings"] = j.Warnings
	}
	return r
}
