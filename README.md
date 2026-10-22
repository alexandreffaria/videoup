# videoup

AI-upscales videos and GIFs 4x with Real-ESRGAN (`upscayl-bin`, model **High Fidelity**) and
returns **one finished file per source**: `<name>_x4.mov`, ProRes 4444, beside the source. The
output has the same duration and frame rate as the source, the audio stays in sync, and alpha is
kept. Frames only ever exist in the system temp folder and are removed when the run ends.
Images in the folder (PNG, JPG, WebP, BMP, TIFF), and GIFs with a single frame, become an
upscaled `<name>_x4.png` instead of a one-frame movie.

It ships as **one self-contained binary per OS**. Each binary carries its own upscaler and model,
so the only thing the machine needs is **ffmpeg** on PATH.

| Binary (in `dist/`) | For |
|---|---|
| `videoup-windows-amd64.exe` | Windows x64 |
| `videoup-macos-arm64` / `videoup-macos-amd64` | Apple Silicon / Intel Macs |
| `videoup-linux-amd64` | Linux x64 (needs a Vulkan GPU driver) |

On macOS an unsigned download is quarantined; clear it once with
`xattr -d com.apple.quarantine videoup-macos-arm64`, then `chmod +x` it.

## Run

```
videoup                                    interactive: pick a folder, review, run
videoup "D:\footage\gifs"                  review screen for everything under it
videoup gifs --yes                         straight to work, no review
videoup clip.mp4 --codec h264              one file, H.264 review copy (.mp4)
videoup --dry-run gifs                     the plan only
videoup --list-models                      every model each found upscaler has
```

The interactive screens: type or drop a folder and press Enter. The review screen shows every file
with its size, its output size, fps, duration and audio. Space toggles a file, `a` toggles all,
Enter starts. A file already done shows as "done already" and is not picked; toggle it to redo it.
The run screen shows upscale and encode bars per file. **Ctrl+C cancels cleanly**: the encoder and
upscaler stop and temp is removed.

| Option | Default | What it does |
|---|---|---|
| `--model` | `high-fidelity-4x` | another model needs Upscayl installed (or `--upscaler`); see `--list-models` |
| `--scale` | `4` | 2 or 3 resample the 4x result down |
| `--codec` | `prores4444` | `prores422hq` (no alpha), `h264` (review copy, `.mp4`) |
| `--out-dir` | beside source | mirror the folder tree somewhere else |
| `--vfr-fps` | nearest standard | rate for variable-rate sources, e.g. `30` or `30000/1001` |
| `--force` | off | redo sources whose output exists (overwrites it) |
| `--tile` | auto | lower it if the GPU runs out of memory |
| `--upscaler` | built in | another `upscayl-bin`, or an Upscayl `resources` folder |

Inputs: video and animation `.gif .mov .mp4 .m4v .mkv .webm .avi .mpg .mpeg .mxf .wmv .flv .ts
.apng .3gp`; images `.png .jpg .jpeg .webp .bmp .tif .tiff`.
Folders named `to_delete` and files already ending in `_x4` are never picked up, so a second run
over a folder only does what is new. When `clip.gif` and `clip.mp4` sit side by side, the two
outputs are named `clip_gif_x4.mov` and `clip_mp4_x4.mov`. Options may come before or after the
paths.

## For scripts and LLMs

When stdout is not a terminal (or with `--json`), stdout carries **JSON lines** and nothing else.
Without `--json`, a run on a terminal gets the TUI, or a plain progress line with `--yes`.

| event | when | key fields |
|---|---|---|
| `plan` | one per source | `source`, `output`, `status` (`todo`/`exists`/`unreadable`), `width`, `out_width`, `fps`, `duration`, `audio_streams`, `alpha` |
| `start` | a file begins | `file`, `output`, `index`, `total` |
| `progress` | about once a second | `stage`, `upscaled`/`upscale_total`, `encoded`/`encode_total`, `elapsed` |
| `done` | a file is finished | `output`, `width`, `height`, `fps`, `frames`, `duration_source`, `duration_output`, `rate_note`, `seconds` |
| `warning` / `error` | as they occur | `file`, `message` |
| `interrupted` | Ctrl+C | `file` |
| `summary` | last line | `done`, `skipped`, `failed`, `outputs` |

`-` (or no paths with a piped stdin) reads paths, one per line, **or videoup's own JSON lines**
(it takes `source`), so a plan can be filtered and fed back:

```
videoup --dry-run gifs      | jq -c "select(.status==\"todo\" and .duration < 10)" | videoup -
```

Exit codes: `0` done or skipped, `1` a file failed (the others still ran), `2` usage error or
missing dependency, `130` interrupted.

## How it works

Four stages per file. The **timing map** and the **pipe** carry the mechanism.

1. **Frame times** (`probe.go`). ffprobe decodes the video once and lists when each frame is
   shown. Duration, rate and sync all derive from that list, never from the container's headline
   rate, which for a GIF is a guess (`david-goggins.gif` holds frames for either 30 or 70 ms).

2. **The timing map** (`timing.go`). This decides the output rate and which source frame fills
   each output frame.
   - **Constant-rate source:** keeps its exact rate as a fraction, so 29.97 is written as
     30000/1001 and never rounds to 29.97 and drifts. The map is 1:1.
   - **Variable-rate source:** mixed GIF delays or phone footage have no single rate, and a
     timeline needs one. Each slot of a constant grid shows the frame on screen at that moment,
     the rule of ffmpeg's `fps` filter. The grid is the nearest standard rate to the average, or
     `--vfr-fps`. Duration is unchanged. The `done` event's `rate_note` says when this happened.
   - **Stills:** images, and GIFs or APNGs whose frames counted at plan time come to one, skip
     all of this. The single frame is upscaled straight to a PNG, alpha included.

   Only frames the map uses are upscaled, each once; repeats reuse the upscaled frame.

3. **The pipe** (`job.go`). The encoder starts first and reads PNGs from a pipe in output order.
   A goroutine runs the upscaler on batches of 120 frames, one batch ahead. Each batch is deleted
   once it has been fed in, so the 4x frames on disk never exceed two batches, however long the
   clip. The upscaler's output goes to a log file and never to an unread pipe: it prints for every
   tile, and a full pipe would stall it.

4. **Audio and hand-over.** Audio streams come straight from the source. The video is offset by
   the source's own gap between container start and first frame, which preserves sync. ProRes
   gets 24-bit PCM audio. The file is encoded in temp, probed, copied beside the source as
   `.part`, then renamed, so a half-written output never appears in an edit folder.

**Codec.** ProRes 4444 at quality 5 (`-qscale:v 5`; round-trip error is a fraction of one
8-bit step), vendor `apl0`, tagged Rec.709. After Effects and Premiere decode it natively on
Windows, and it carries a real alpha channel, which DNxHD/DNxHR do not. The upscaler upscales the
alpha channel too, so GIF transparency survives.

**The built-in upscaler** (`upscaler.go`, `assets_*.go`). Each binary embeds that OS's
`upscayl-bin` and the model. On first run it unpacks them to the user cache folder
(`%LOCALAPPDATA%\videoup\<hash>`, `~/Library/Caches/videoup/…`, `~/.cache/videoup/…`), once per
version. Upscalers are tried in this order: `--upscaler` / `VIDEOUP_UPSCALER`, the built-in one,
an Upscayl install, then `upscayl-bin` or `realesrgan-ncnn-vulkan` on PATH. The first one that
has the requested model wins.

**Speed.** About 0.6 s per upscaled frame for a 500 px source on the RTX 3090 (a 32-frame GIF
takes about 20 s). The work scales with output pixels.

## Build

```
cd videoup
go run ./build                    every target into dist/
go run ./build windows/amd64      just one
go test -tags noembed ./...       unit tests (timing map, args, TUI screens)
```

`build/main.go` downloads the pinned `upscayl-bin` release for each OS and the model from
Upscayl's repository at a pinned commit, checks every download against its SHA-256, and caches
them in `assets/`. It then cross-compiles with `CGO_ENABLED=0`, so one machine builds all four
binaries. On Windows it also grants Everyone read+execute on each binary, because a file written
to a network share by a tool can arrive without Execute.

**Releases.** Pushing a tag such as `v2026.10.22` makes GitHub Actions build all four binaries
and attach them to a GitHub Release. That Release is the download link to share. `assets/` and `dist/` are git-ignored. To move to a newer upscaler, change the release
URL and checksums at the top of `build/main.go`. `-tags noembed` builds a small binary without
the upscaler, which then uses an installed Upscayl.

**Licences.** `upscayl-bin` is AGPL-3.0. Its licence travels inside each binary and is unpacked
beside it. The model comes from the Upscayl repository; check its terms before shipping outside
the team.
