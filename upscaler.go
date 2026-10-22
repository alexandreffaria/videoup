package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// Upscaler is one Real-ESRGAN ncnn binary and the folder of models it reads.
type Upscaler struct {
	Bin, Models, Source string
}

func (u Upscaler) models() []string {
	var out []string
	params, _ := filepath.Glob(filepath.Join(u.Models, "*.param"))
	for _, p := range params {
		name := strings.TrimSuffix(filepath.Base(p), ".param")
		if _, err := os.Stat(filepath.Join(u.Models, name+".bin")); err == nil {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func (u Upscaler) has(model string) bool {
	for _, m := range u.models() {
		if m == model {
			return true
		}
	}
	return false
}

// findUpscalers lists every usable upscaler, best first: the one named on the command
// line or in VIDEOUP_UPSCALER, the copy built into this binary, an installed Upscayl,
// then anything on PATH.
func findUpscalers(explicit string) []Upscaler {
	var out []Upscaler
	add := func(bin, models, source string) {
		if st, err := os.Stat(bin); err == nil && !st.IsDir() {
			out = append(out, Upscaler{Bin: bin, Models: models, Source: source})
		}
	}
	exe := "upscayl-bin"
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	fromDir := func(dir, source string) { // an Upscayl "resources" folder, or a folder holding the binary
		for _, b := range []string{filepath.Join(dir, "bin", exe), filepath.Join(dir, exe),
			filepath.Join(dir, "resources", "bin", exe)} {
			if _, err := os.Stat(b); err == nil {
				models := filepath.Join(filepath.Dir(b), "models")
				if _, err := os.Stat(models); err != nil {
					models = filepath.Join(filepath.Dir(filepath.Dir(b)), "models")
				}
				add(b, models, source)
				return
			}
		}
	}
	for _, e := range []struct{ v, src string }{{explicit, "--upscaler"}, {os.Getenv("VIDEOUP_UPSCALER"), "VIDEOUP_UPSCALER"}} {
		if e.v == "" {
			continue
		}
		if st, err := os.Stat(e.v); err == nil && !st.IsDir() {
			add(e.v, filepath.Join(filepath.Dir(e.v), "models"), e.src)
			if len(out) > 0 && !dirExists(out[len(out)-1].Models) {
				out[len(out)-1].Models = filepath.Join(filepath.Dir(filepath.Dir(e.v)), "models")
			}
		} else {
			fromDir(e.v, e.src)
		}
	}
	if u, err := embedded(); err == nil && u != nil {
		out = append(out, *u)
	}
	for _, d := range upscaylDirs() {
		fromDir(d, "Upscayl install")
	}
	for _, name := range []string{"upscayl-bin", "realesrgan-ncnn-vulkan"} {
		if p, err := exec.LookPath(name); err == nil {
			add(p, filepath.Join(filepath.Dir(p), "models"), "PATH")
		}
	}
	return out
}

func upscaylDirs() []string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		return []string{filepath.Join(os.Getenv("ProgramFiles"), "Upscayl", "resources"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Upscayl", "resources")}
	case "darwin":
		return []string{"/Applications/Upscayl.app/Contents/Resources",
			filepath.Join(home, "Applications", "Upscayl.app", "Contents", "Resources")}
	default:
		return []string{"/opt/Upscayl/resources", "/usr/lib/upscayl/resources", "/usr/share/upscayl/resources"}
	}
}

// pickUpscaler returns the first upscaler that has the model.
func pickUpscaler(all []Upscaler, model string) (Upscaler, error) {
	for _, u := range all {
		if u.has(model) {
			return u, nil
		}
	}
	if len(all) == 0 {
		return Upscaler{}, fmt.Errorf("no upscaler found: this build carries none, and no Upscayl install or " +
			"upscayl-bin is on this machine (install Upscayl, or pass --upscaler <path>)")
	}
	return Upscaler{}, fmt.Errorf("model %q is in none of: %s (see --list-models)", model, describe(all))
}

func describe(all []Upscaler) string {
	var s []string
	for _, u := range all {
		s = append(s, u.Source+" ("+u.Bin+")")
	}
	return strings.Join(s, ", ")
}

// embedded unpacks the upscaler built into this binary into the user cache folder,
// once per asset version, and returns it. Nil when the build carries none.
func embedded() (*Upscaler, error) {
	if assets == nil {
		return nil, nil
	}
	sum := sha256.New()
	var files []string
	fs.WalkDir(assets, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			files = append(files, p)
			fi, _ := d.Info()
			fmt.Fprintf(sum, "%s:%d;", p, fi.Size())
		}
		return nil
	})
	cache, err := os.UserCacheDir()
	if err != nil {
		cache = os.TempDir()
	}
	root := filepath.Join(cache, "videoup", hex.EncodeToString(sum.Sum(nil))[:12])
	exe := "upscayl-bin"
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	u := &Upscaler{Bin: filepath.Join(root, exe), Models: filepath.Join(root, "models"), Source: "built in"}
	if _, err := os.Stat(filepath.Join(root, ".complete")); err == nil {
		return u, nil
	}
	for _, p := range files {
		data, err := fs.ReadFile(assets, p)
		if err != nil {
			return nil, err
		}
		// assets/<os>/upscayl-bin -> root/upscayl-bin, assets/models/x -> root/models/x
		parts := strings.Split(p, "/")
		dst := filepath.Join(root, filepath.Base(p))
		if len(parts) >= 2 && parts[1] == "models" {
			dst = filepath.Join(root, "models", filepath.Base(p))
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, err
		}
		tmp := dst + ".tmp"
		if err := os.WriteFile(tmp, data, 0o755); err != nil {
			return nil, err
		}
		if err := os.Rename(tmp, dst); err != nil {
			return nil, err
		}
	}
	return u, os.WriteFile(filepath.Join(root, ".complete"), nil, 0o644)
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
