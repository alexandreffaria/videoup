package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"os/exec"
	"strconv"
	"strings"
)

// Info is what the plan needs to know about a source, from one cheap ffprobe call.
type Info struct {
	Width, Height int
	Rate          *big.Rat // r_frame_rate, nil when ffprobe has no sane value
	AvgRate       *big.Rat
	Duration      float64
	Start         float64 // container start time; the audio offset is measured from it
	Audio         int     // number of audio streams
	Alpha         bool
	Still         bool // one frame only: the output is a PNG
}

var alphaFmts = map[string]bool{"rgba": true, "bgra": true, "argb": true, "abgr": true, "pal8": true,
	"rgba64le": true, "rgba64be": true, "bgra64le": true, "bgra64be": true, "ya8": true, "ya16le": true, "ya16be": true}

func probe(ctx context.Context, src string) (*Info, error) {
	out, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_streams", "-show_format",
		"-of", "json", src).Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe could not read it: %w", exitText(err))
	}
	var d struct {
		Streams []struct {
			CodecType   string            `json:"codec_type"`
			Width       int               `json:"width"`
			Height      int               `json:"height"`
			PixFmt      string            `json:"pix_fmt"`
			RFrameRate  string            `json:"r_frame_rate"`
			AvgRate     string            `json:"avg_frame_rate"`
			Duration    string            `json:"duration"`
			Disposition map[string]int    `json:"disposition"`
			SideData    []json.RawMessage `json:"side_data_list"`
		} `json:"streams"`
		Format struct {
			Duration  string `json:"duration"`
			StartTime string `json:"start_time"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &d); err != nil {
		return nil, fmt.Errorf("ffprobe output: %w", err)
	}
	info := &Info{}
	found := false
	for _, s := range d.Streams {
		switch {
		case s.CodecType == "audio":
			info.Audio++
		case s.CodecType == "video" && !found && s.Disposition["attached_pic"] == 0:
			found = true
			info.Width, info.Height = s.Width, s.Height
			info.Rate, info.AvgRate = rate(s.RFrameRate), rate(s.AvgRate)
			info.Alpha = alphaFmts[s.PixFmt] || strings.HasPrefix(s.PixFmt, "yuva") || strings.HasPrefix(s.PixFmt, "gbrap")
			info.Duration = num(s.Duration)
			for _, raw := range s.SideData { // phone footage: ffmpeg rotates on decode
				var sd struct{ Rotation float64 }
				if json.Unmarshal(raw, &sd) == nil && math.Mod(math.Abs(sd.Rotation), 180) == 90 {
					info.Width, info.Height = info.Height, info.Width
				}
			}
		}
	}
	if !found {
		return nil, fmt.Errorf("no video stream")
	}
	if f := num(d.Format.Duration); f > 0 {
		info.Duration = f
	}
	info.Start = num(d.Format.StartTime)
	return info, nil
}

// frameTimes decodes the video once and returns when each frame is shown, plus how
// long the last one stays up (0 when unknown). Every timing decision derives from this
// list, never from the container's headline frame rate, which for a GIF is a guess.
func frameTimes(ctx context.Context, src string) ([]float64, float64, error) {
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-select_streams", "v:0", "-show_entries",
		"frame=best_effort_timestamp_time,duration_time", "-of", "compact=p=0", src)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, 0, err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, 0, err
	}
	var pts []float64
	last := 0.0
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		kv := map[string]string{}
		for _, p := range strings.Split(strings.TrimSpace(sc.Text()), "|") {
			if k, v, ok := strings.Cut(p, "="); ok {
				kv[k] = v
			}
		}
		t, err := strconv.ParseFloat(kv["best_effort_timestamp_time"], 64)
		if err != nil { // no timestamp: assume it follows the previous frame
			t = 0
			if n := len(pts); n > 0 {
				t = pts[n-1] + last
			}
		}
		pts = append(pts, t)
		last, _ = strconv.ParseFloat(kv["duration_time"], 64)
	}
	if err := cmd.Wait(); err != nil {
		return nil, 0, fmt.Errorf("ffprobe frame scan failed: %s", strings.TrimSpace(stderr.String()))
	}
	if len(pts) == 0 {
		return nil, 0, fmt.Errorf("no frames decoded")
	}
	return pts, last, nil
}

func rate(s string) *big.Rat {
	r, ok := new(big.Rat).SetString(s)
	if !ok || r.Sign() <= 0 || r.Cmp(big.NewRat(1000, 1)) >= 0 {
		return nil
	}
	return r
}

func num(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func exitText(err error) error {
	if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
		return fmt.Errorf("%s", tail(string(ee.Stderr), 400))
	}
	return err
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return "..." + s[len(s)-n:]
	}
	return s
}
