package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type Codec struct {
	Ext       string
	Alpha     bool
	Video     []string
	Pix, PixA string
	Audio     []string
}

// ProRes at quality 5, vendor apl0: lossless to the eye, decoded natively by Adobe apps.
var codecs = map[string]Codec{
	"prores4444": {".mov", true, []string{"-c:v", "prores_ks", "-profile:v", "4444", "-qscale:v", "5", "-vendor", "apl0"},
		"yuv444p10le", "yuva444p10le", []string{"-c:a", "pcm_s24le"}},
	"prores422hq": {".mov", false, []string{"-c:v", "prores_ks", "-profile:v", "3", "-qscale:v", "5", "-vendor", "apl0"},
		"yuv422p10le", "", []string{"-c:a", "pcm_s24le"}},
	"h264": {".mp4", false, []string{"-c:v", "libx264", "-preset", "slow", "-crf", "14", "-movflags", "+faststart"},
		"yuv420p", "", []string{"-c:a", "aac", "-b:a", "320k"}},
}

// Progress is reported while a file runs.
type Progress struct {
	File                   string
	Stage                  string // probe, extract, upscale
	Upscaled, UpscaleTotal int
	Encoded, EncodeTotal   int
	Elapsed                time.Duration
}

type Result struct {
	File           string  `json:"file"`
	Output         string  `json:"output"`
	Width          int     `json:"width"`
	Height         int     `json:"height"`
	FPS            string  `json:"fps,omitempty"`
	Frames         int     `json:"frames"`
	Still          bool    `json:"still"`
	CFRSource      bool    `json:"cfr_source"`
	RateNote       string  `json:"rate_note,omitempty"`
	DurationSource float64 `json:"duration_source"`
	DurationOutput float64 `json:"duration_output"`
	AudioStreams   int     `json:"audio_streams"`
	Alpha          bool    `json:"alpha"`
	Seconds        float64 `json:"seconds"`
}

type batch struct {
	idx map[int]bool
	dir string
	err error
}

// process turns one source into one finished file:
//
//  1. ffprobe lists when every frame is shown; planTiming turns that into the output
//     rate and a frame map.
//  2. ffmpeg writes each source frame the map uses, once, as a PNG in the temp folder.
//  3. A goroutine upscales those PNGs in batches while the main loop feeds the finished
//     ones, in output order and repeated where the map repeats them, through a pipe into
//     the encoder. Each batch is deleted as soon as it is fed, so the 4x frames on disk
//     never exceed two batches however long the clip.
//  4. The audio comes straight from the source, offset by the source's own gap between
//     container start and first video frame. The file is encoded in temp, verified, and
//     copied beside the source as .part, then renamed: a half-written output never shows.
func process(ctx context.Context, j *Job, o *Options, up Upscaler, report func(Progress)) (res *Result, err error) {
	start := time.Now()
	codec := codecs[o.Codec]
	alpha := j.Info.Alpha && codec.Alpha
	tmp, err := os.MkdirTemp(o.Tmp, "videoup_")
	if err != nil {
		return nil, err
	}
	if !o.KeepTemp {
		defer os.RemoveAll(tmp) // the system temp folder, never the source's folder
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	prog := Progress{File: j.Source}
	tick := func(stage string) {
		prog.Stage, prog.Elapsed = stage, time.Since(start)
		report(prog)
	}

	if j.Info.Still {
		return processStill(ctx, j, o, up, tmp, start)
	}

	tick("probe")
	pts, lastDur, err := frameTimes(ctx, j.Source)
	if err != nil {
		return nil, err
	}
	tm := planTiming(j.Info, pts, lastDur, o.VFRFPS)

	tick("extract")
	lo := filepath.Join(tmp, "src")
	os.Mkdir(lo, 0o755)
	pix := "rgb24"
	if j.Info.Alpha {
		pix = "rgba"
	}
	if out, err := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-v", "error", "-i", j.Source, "-map", "0:v:0",
		passthroughFlag(), "passthrough", "-pix_fmt", pix, "-start_number", "0",
		filepath.Join(lo, "%08d.png")).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("frame extraction failed: %s", tail(string(out), 400))
	}
	pngs, _ := filepath.Glob(filepath.Join(lo, "*.png"))
	if len(pngs) == 0 {
		return nil, fmt.Errorf("no frames extracted")
	}
	if len(pngs) != len(pts) {
		j.Warnings = append(j.Warnings, fmt.Sprintf("decoded %d frames, timestamps for %d", len(pngs), len(pts)))
		for k, v := range tm.Map {
			tm.Map[k] = min(v, len(pngs)-1)
		}
	}
	needed := tm.needed()
	prog.UpscaleTotal, prog.EncodeTotal = len(needed), len(tm.Map)

	// encoder, reading PNGs from a pipe in output order
	part := filepath.Join(tmp, "out"+codec.Ext)
	args := []string{"-nostdin", "-v", "error", "-y", "-f", "image2pipe", "-c:v", "png", "-framerate", tm.FPS.RatString()}
	if j.Info.Audio > 0 {
		args = append(args, "-itsoffset", strconv.FormatFloat(tm.VStart-j.Info.Start, 'f', 6, 64))
	}
	args = append(args, "-i", "pipe:0")
	if j.Info.Audio > 0 {
		args = append(args, "-i", j.Source, "-map", "0:v:0", "-map", "1:a")
	}
	pf := codec.Pix
	if alpha {
		pf = codec.PixA
	}
	args = append(args, "-vf", "scale=out_color_matrix=bt709:out_range=tv,format="+pf)
	args = append(args, codec.Video...)
	args = append(args, "-colorspace", "bt709", "-color_primaries", "bt709", "-color_trc", "bt709")
	if alpha {
		args = append(args, "-alpha_bits", "16")
	}
	if j.Info.Audio > 0 {
		args = append(args, codec.Audio...)
	}
	args = append(args, part)
	encLog, _ := os.Create(filepath.Join(tmp, "ffmpeg.log"))
	defer encLog.Close()
	enc := exec.CommandContext(ctx, "ffmpeg", args...)
	enc.Stderr = encLog
	pipe, err := enc.StdinPipe()
	if err != nil {
		return nil, err
	}
	if err := enc.Start(); err != nil {
		return nil, err
	}
	encErr := func() error { return fmt.Errorf("encoder failed: %s", tail(readFile(encLog.Name()), 400)) }
	defer func() { // on any failure: stop the encoder and the upscaler before temp is removed
		if err != nil {
			cancel()
			pipe.Close()
			enc.Wait()
		}
	}()

	// upscaler, one batch ahead of the encoder
	var upscaled atomic.Int64
	ready := make(chan batch, 1)
	workerDone := make(chan struct{})
	defer func() { cancel(); <-workerDone }() // temp is only removed once the worker is out of it
	go func() {
		defer close(workerDone)
		defer close(ready)
		for b := 0; b*o.Batch < len(needed); b++ {
			idxs := needed[b*o.Batch : min((b+1)*o.Batch, len(needed))]
			bin, bout := filepath.Join(tmp, fmt.Sprintf("b%d_in", b)), filepath.Join(tmp, fmt.Sprintf("b%d_out", b))
			os.Mkdir(bin, 0o755)
			os.Mkdir(bout, 0o755)
			set := map[int]bool{}
			for _, i := range idxs {
				name := fmt.Sprintf("%08d.png", i)
				os.Rename(filepath.Join(lo, name), filepath.Join(bin, name))
				set[i] = true
			}
			if err := runUpscaler(ctx, up, o, bin, bout, tmp, func(n int) { upscaled.Store(int64(b*o.Batch + n)) }); err != nil {
				send(ctx, ready, batch{err: err})
				return
			}
			var missing int
			for _, i := range idxs {
				if _, err := os.Stat(filepath.Join(bout, fmt.Sprintf("%08d.png", i))); err != nil {
					missing++
				}
			}
			if missing > 0 {
				send(ctx, ready, batch{err: fmt.Errorf("upscaler produced %d/%d frames: %s", len(idxs)-missing,
					len(idxs), tail(readFile(filepath.Join(tmp, "upscaler.log")), 400))})
				return
			}
			os.RemoveAll(bin)
			upscaled.Store(int64(b*o.Batch + len(idxs)))
			if !send(ctx, ready, batch{idx: set, dir: bout}) {
				return
			}
		}
	}()

	var cur batch
	tickUp := time.NewTicker(300 * time.Millisecond)
	defer tickUp.Stop()
	for k, i := range tm.Map {
		for !cur.idx[i] { // the map only moves forward: the next batch holds it
			if cur.dir != "" {
				os.RemoveAll(cur.dir)
			}
		wait:
			for {
				select {
				case b, ok := <-ready:
					if !ok {
						return nil, errors.Join(ctx.Err(), fmt.Errorf("upscaler stopped early"))
					}
					if b.err != nil {
						return nil, b.err
					}
					cur = b
					break wait
				case <-tickUp.C:
					prog.Upscaled = int(upscaled.Load())
					tick("upscale")
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
		}
		data, err := os.ReadFile(filepath.Join(cur.dir, fmt.Sprintf("%08d.png", i)))
		if err != nil {
			return nil, err
		}
		if _, err := pipe.Write(data); err != nil {
			return nil, encErr()
		}
		prog.Encoded, prog.Upscaled = k+1, int(upscaled.Load())
		select {
		case <-tickUp.C:
			tick("upscale")
		default:
		}
	}
	pipe.Close()
	if err := enc.Wait(); err != nil {
		return nil, encErr()
	}
	prog.Upscaled = len(needed)
	tick("upscale")

	got, err := probe(ctx, part)
	if err != nil {
		return nil, fmt.Errorf("output did not verify: %w", err)
	}
	if err := deliver(part, j.Output); err != nil {
		return nil, err
	}
	return &Result{File: j.Source, Output: j.Output, Width: got.Width, Height: got.Height,
		FPS: tm.FPS.RatString(), Frames: len(tm.Map), CFRSource: tm.CFR, RateNote: tm.Changed,
		DurationSource: round3(j.Info.Duration), DurationOutput: round3(got.Duration),
		AudioStreams: got.Audio, Alpha: alpha, Seconds: round3(time.Since(start).Seconds())}, nil
}

// processStill handles a one-frame source: the frame is upscaled and delivered as a PNG,
// alpha included, with no movie around it.
func processStill(ctx context.Context, j *Job, o *Options, up Upscaler, tmp string, start time.Time) (*Result, error) {
	in, out := filepath.Join(tmp, "in"), filepath.Join(tmp, "out")
	os.Mkdir(in, 0o755)
	os.Mkdir(out, 0o755)
	pix := "rgb24"
	if j.Info.Alpha {
		pix = "rgba"
	}
	if b, err := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-v", "error", "-i", j.Source, "-map", "0:v:0",
		"-frames:v", "1", "-pix_fmt", pix, filepath.Join(in, "frame.png")).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("frame extraction failed: %s", tail(string(b), 400))
	}
	if err := runUpscaler(ctx, up, o, in, out, tmp, func(int) {}); err != nil {
		return nil, err
	}
	png := filepath.Join(out, "frame.png")
	if _, err := os.Stat(png); err != nil {
		return nil, fmt.Errorf("upscaler produced nothing: %s", tail(readFile(filepath.Join(tmp, "upscaler.log")), 400))
	}
	got, err := probe(ctx, png)
	if err != nil {
		return nil, fmt.Errorf("output did not verify: %w", err)
	}
	if err := deliver(png, j.Output); err != nil {
		return nil, err
	}
	return &Result{File: j.Source, Output: j.Output, Width: got.Width, Height: got.Height, Frames: 1,
		Still: true, Alpha: j.Info.Alpha, Seconds: round3(time.Since(start).Seconds())}, nil
}

func send(ctx context.Context, ch chan<- batch, b batch) bool {
	select {
	case ch <- b:
		return true
	case <-ctx.Done():
		return false
	}
}

// runUpscaler runs one batch. Its output goes to a log file, never an unread pipe: the
// upscaler prints per tile, and a full pipe stalls it.
func runUpscaler(ctx context.Context, up Upscaler, o *Options, in, out, tmp string, done func(int)) error {
	args := []string{"-i", in, "-o", out, "-m", up.Models, "-n", o.Model, "-s", strconv.Itoa(o.Scale), "-f", "png"}
	if s := modelScale(o.Model); s != 0 && filepath.Base(up.Bin) != "realesrgan-ncnn-vulkan" {
		args = append(args, "-z", strconv.Itoa(s))
	}
	if o.GPU >= 0 {
		args = append(args, "-g", strconv.Itoa(o.GPU))
	}
	if o.Tile > 0 {
		args = append(args, "-t", strconv.Itoa(o.Tile))
	}
	log, _ := os.Create(filepath.Join(tmp, "upscaler.log"))
	defer log.Close()
	cmd := exec.CommandContext(ctx, up.Bin, args...)
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("upscaler would not start: %w", err)
	}
	finished := make(chan error, 1)
	go func() { finished <- cmd.Wait() }()
	t := time.NewTicker(300 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-finished:
			return ctx.Err() // a non-zero exit is judged by the frames it left, not its code
		case <-t.C:
			if m, _ := filepath.Glob(filepath.Join(out, "*.png")); m != nil {
				done(len(m))
			}
		}
	}
}

// modelScale reads the native scale from a name like high-fidelity-4x or realesrgan-x4plus.
func modelScale(model string) int {
	for _, s := range []int{2, 3, 4, 8} {
		n := strconv.Itoa(s)
		if strings.Contains(model, "-"+n+"x") || strings.Contains(model, "x"+n) || strings.HasSuffix(model, n+"x") {
			return s
		}
	}
	return 0
}

// deliver copies the finished file next to the source as .part, then renames it over
// the final name, so an interrupted copy never leaves a playable-looking output.
func deliver(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	staged := dst + ".part"
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(staged)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(staged, dst)
}

var passthrough string

// passthroughFlag is -fps_mode on ffmpeg 5.1+, -vsync on older builds.
func passthroughFlag() string {
	if passthrough == "" {
		passthrough = "-vsync"
		if out, _ := exec.Command("ffmpeg", "-hide_banner", "-h", "long").Output(); strings.Contains(string(out), "-fps_mode") {
			passthrough = "-fps_mode"
		}
	}
	return passthrough
}

func readFile(p string) string { b, _ := os.ReadFile(p); return string(b) }

func round3(x float64) float64 { return float64(round(x*1000)) / 1000 }
