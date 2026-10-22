// Build every shippable videoup binary:  go run ./build   (from the repo root)
//
// It fetches the pinned upscaler for each OS and the model into assets/ (checked by
// SHA-256, downloaded once), then cross-compiles one self-contained binary per target
// into dist/. Pass target names (windows/amd64 ...) to build only those.
package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const ncnn = "https://github.com/upscayl/upscayl-ncnn/releases/download/20251207-174704/upscayl-bin-20251207-174704-"
const models = "https://raw.githubusercontent.com/upscayl/upscayl/e1aeaf1217b6061b4212b87c9fd421984f73e437/resources/models/"

type asset struct {
	url, sha, dest string
	member         string // file to take out of the zip, "" when the download is the file
}

var assets = []asset{
	{ncnn + "windows.zip", "1f0f65c5d2ade866555e2ac467d35952c35a47080a4060fe56a1ab028f67258a", "assets/windows/upscayl-bin.exe", "upscayl-bin.exe"},
	{ncnn + "windows.zip", "1f0f65c5d2ade866555e2ac467d35952c35a47080a4060fe56a1ab028f67258a", "assets/windows/LICENSE-upscayl-bin", "LICENSE"},
	{ncnn + "linux.zip", "a9fab3c770b62f2b7a35d8d6d61eb4e8b3aef79128b665c919d080b85a2292f2", "assets/linux/upscayl-bin", "upscayl-bin"},
	{ncnn + "linux.zip", "a9fab3c770b62f2b7a35d8d6d61eb4e8b3aef79128b665c919d080b85a2292f2", "assets/linux/LICENSE-upscayl-bin", "LICENSE"},
	{ncnn + "macos.zip", "277419791281a56eae0c739c70120b974d7267cf7c2de8e86dc09798d4b314db", "assets/darwin/upscayl-bin", "upscayl-bin"},
	{ncnn + "macos.zip", "277419791281a56eae0c739c70120b974d7267cf7c2de8e86dc09798d4b314db", "assets/darwin/LICENSE-upscayl-bin", "LICENSE"},
	{models + "high-fidelity-4x.param", "4576ed5c2fc5fa250d3c3d585ef02248f26abdfc1867088078f501fe71e5d61e", "assets/models/high-fidelity-4x.param", ""},
	{models + "high-fidelity-4x.bin", "8a135402b4f39286121b76abb47601a6b7b7e8d4f3e999a5aaa45ed277824fb4", "assets/models/high-fidelity-4x.bin", ""},
}

// darwin/amd64 and darwin/arm64 share the universal macOS upscaler.
var targets = []string{"windows/amd64", "linux/amd64", "darwin/arm64", "darwin/amd64"}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "build failed:", err)
		os.Exit(1)
	}
}

func run(only []string) error {
	cache := map[string][]byte{}
	for _, a := range assets {
		if _, err := os.Stat(a.dest); err == nil {
			continue
		}
		data, ok := cache[a.url]
		if !ok {
			fmt.Println("fetch", a.url)
			var err error
			if data, err = fetch(a.url); err != nil {
				return err
			}
			if got := sum(data); got != a.sha {
				return fmt.Errorf("%s: sha256 %s, pinned %s", a.url, got, a.sha)
			}
			cache[a.url] = data
		}
		if a.member != "" {
			var err error
			if data, err = unzip(data, a.member); err != nil {
				return fmt.Errorf("%s: %w", a.url, err)
			}
		}
		os.MkdirAll(filepath.Dir(a.dest), 0o755)
		if err := os.WriteFile(a.dest, data, 0o755); err != nil {
			return err
		}
	}

	ver := time.Now().Format("2006.01.02")
	if out, err := exec.Command("git", "log", "-1", "--format=%h", "--", ".").Output(); err == nil && len(out) > 0 {
		ver += "-" + strings.TrimSpace(string(out))
	}
	if v := os.Getenv("VIDEOUP_VERSION"); v != "" { // the release workflow passes the tag
		ver = v
	}
	if len(only) > 0 {
		targets = only
	}
	for _, t := range targets {
		goos, goarch, _ := strings.Cut(t, "/")
		label := goos
		if goos == "darwin" {
			label = "macos"
		}
		name := fmt.Sprintf("dist/videoup-%s-%s", label, goarch)
		if goos == "windows" {
			name += ".exe"
		}
		fmt.Println("build", name)
		cmd := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w -X main.version="+ver, "-o", name, ".")
		cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s: %w", t, err)
		}
		if runtime.GOOS == "windows" { // a file a tool writes on a network share can arrive without Execute
			exec.Command("icacls", filepath.FromSlash(name), "/grant", "Everyone:(RX)").Run()
		}
	}
	return nil
}

func fetch(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func unzip(data []byte, member string) ([]byte, error) {
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	for _, f := range z.File {
		if filepath.Base(f.Name) == member && !f.FileInfo().IsDir() {
			r, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer r.Close()
			return io.ReadAll(r)
		}
	}
	return nil, fmt.Errorf("%s not in the zip", member)
}

func sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
