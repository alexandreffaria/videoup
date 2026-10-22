package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Sequence is a numbered run of still frames (Blender's render_0001.exr ...) that becomes
// one movie. Files[i] is frame Nums[i]; Step is the spacing of the numbering.
type Sequence struct {
	Dir, Prefix, Ext string
	Pad              int // digits when zero-padded, 0 when the numbers are not padded
	Nums             []int
	Files            []string
	Step             int
}

// minSeqFrames: fewer numbered stills than this stay separate stills.
const minSeqFrames = 3

var trailingNum = regexp.MustCompile(`^(.*?)(\d+)$`)

// Pattern names the sequence as one path, the frame number written as #s: render_####.exr.
// It is the sequence's source in the plan, and videoup accepts it back as an input.
func (s *Sequence) Pattern() string {
	return filepath.Join(s.Dir, s.Prefix+strings.Repeat("#", max(1, s.Pad))+s.Ext)
}

// Name is the movie's stem: the prefix without its separator, else the folder's name.
func (s *Sequence) Name() string {
	if n := strings.TrimRight(s.Prefix, "_-. "); n != "" {
		return n
	}
	return filepath.Base(s.Dir)
}

// Map gives each output frame its file: a missing frame holds the one before it.
func (s *Sequence) Map() []int {
	n := (s.Nums[len(s.Nums)-1]-s.Nums[0])/s.Step + 1
	m := make([]int, n)
	i := 0
	for k := range m {
		for i+1 < len(s.Nums) && (s.Nums[i+1]-s.Nums[0])/s.Step <= k {
			i++
		}
		m[k] = i
	}
	return m
}

func (s *Sequence) Missing() int { return len(s.Map()) - len(s.Nums) }

type seqKey struct{ dir, prefix, ext string }

// findSequences splits image files into numbered sequences and the stills left over.
// Files form a sequence when they share a folder, a name before the number and an
// extension, number at least minSeqFrames, and their numbers step evenly with at most as
// many holes as frames: a render with a dropped frame still counts, screenshots named
// by the time they were taken do not.
func findSequences(files []string) (seqs []*Sequence, rest []string) {
	groups := map[seqKey][]string{}
	var order []seqKey
	for _, f := range files {
		ext := filepath.Ext(f)
		m := trailingNum.FindStringSubmatch(strings.TrimSuffix(filepath.Base(f), ext))
		if m == nil || !images[strings.ToLower(ext)] {
			rest = append(rest, f)
			continue
		}
		k := seqKey{filepath.Dir(f), m[1], strings.ToLower(ext)}
		if groups[k] == nil {
			order = append(order, k)
		}
		groups[k] = append(groups[k], f)
	}
	for _, k := range order {
		if s := asSequence(k, groups[k]); s != nil {
			seqs = append(seqs, s)
		} else {
			rest = append(rest, groups[k]...)
		}
	}
	sort.Strings(rest)
	return seqs, rest
}

func asSequence(k seqKey, files []string) *Sequence {
	if len(files) < minSeqFrames {
		return nil
	}
	type fr struct {
		n    int
		f, d string
	}
	var frs []fr
	for _, f := range files {
		d := trailingNum.FindStringSubmatch(strings.TrimSuffix(filepath.Base(f), filepath.Ext(f)))[2]
		n, err := strconv.Atoi(d)
		if err != nil {
			return nil
		}
		frs = append(frs, fr{n, f, d})
	}
	sort.Slice(frs, func(a, b int) bool { return frs[a].n < frs[b].n })
	s := &Sequence{Dir: k.dir, Prefix: k.prefix, Ext: filepath.Ext(files[0])}
	pad := len(frs[0].d)
	for i, x := range frs {
		if len(x.d) != pad {
			pad = 0
		}
		if i > 0 {
			d := x.n - frs[i-1].n
			if d == 0 {
				return nil // 01 and 1 side by side: two numberings, not one
			}
			if s.Step == 0 || d < s.Step {
				s.Step = d
			}
		}
		s.Nums, s.Files = append(s.Nums, x.n), append(s.Files, x.f)
	}
	if pad > 0 && frs[0].d[0] != '0' {
		pad = 0 // 100..250: same width, but nothing says it is padded
	}
	s.Pad = pad
	for i := 1; i < len(s.Nums); i++ {
		if (s.Nums[i]-s.Nums[i-1])%s.Step != 0 {
			return nil
		}
	}
	if s.Missing() > len(s.Nums) {
		return nil
	}
	return s
}

// openPattern reads a path like X:\shot\render_####.exr back into its sequence. A single
// # matches a number of any width; several match exactly that many digits.
func openPattern(p string) (*Sequence, error) {
	dir, base := filepath.Split(p)
	i, j := strings.Index(base, "#"), strings.LastIndex(base, "#")
	if i < 0 || strings.Count(base[i:j+1], "#") != j-i+1 {
		return nil, fmt.Errorf("not a frame pattern: %s", p)
	}
	prefix, ext, width := base[:i], base[j+1:], j-i+1
	digits := fmt.Sprintf(`\d{%d}`, width)
	if width == 1 {
		digits = `\d+`
	}
	re := regexp.MustCompile(`(?i)^` + regexp.QuoteMeta(prefix) + digits + regexp.QuoteMeta(ext) + `$`)
	entries, err := os.ReadDir(filepath.Clean(dir))
	if err != nil {
		return nil, fmt.Errorf("not found: %s", p)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && re.MatchString(e.Name()) {
			files = append(files, filepath.Join(filepath.Clean(dir), e.Name()))
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no frames match %s", p)
	}
	k := seqKey{filepath.Clean(dir), prefix, strings.ToLower(ext)}
	s := asSequence(k, files)
	if s == nil && len(files) < minSeqFrames {
		return nil, fmt.Errorf("%s matches %d frame(s); a sequence needs %d", p, len(files), minSeqFrames)
	}
	if s == nil {
		return nil, fmt.Errorf("%s: the frame numbers do not step evenly", p)
	}
	return s, nil
}
