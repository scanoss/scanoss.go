// SPDX-License-Identifier: MIT
/*
 * Copyright (c) 2026, SCANOSS
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in
 * all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
 * THE SOFTWARE.
 */

package filter

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Defaults holds the skip lists and size bounds a DefaultSource turns into
// matchers. Callers normally start from StdDefaults and override fields as
// needed.
type Defaults struct {
	Dirs    []string // directory names skipped wholesale
	DirExts []string // directory-name suffixes skipped (e.g. ".egg-info")
	Files   []string // exact file names skipped
	Exts    []string // file extensions skipped (leading dot)
	Endings []string // non-extension file-name suffixes skipped
}

// StdDefaults returns the built-in default skip lists. Size bounds are not part
// of it: they are caller input, not a built-in, and have their own SizeSource.
func StdDefaults() Defaults {
	return Defaults{
		Dirs:    skippedDirs(ScanOnlySkippedDirs),
		DirExts: DefaultSkippedDirExts,
		Files:   DefaultSkippedFiles,
		Exts:    DefaultSkippedExts,
		Endings: DefaultSkippedFileEndings,
	}
}

// DefaultSource turns the default skip lists and size bounds into matchers.
func DefaultSource(d Defaults) []Matcher {
	var ms []Matcher
	for _, name := range d.Dirs {
		ms = append(ms, newDirNameMatcher(name))
	}
	for _, suffix := range d.DirExts {
		ms = append(ms, newDirSuffixMatcher(suffix))
	}
	for _, name := range d.Files {
		ms = append(ms, newNameMatcher(name))
	}
	// Simple (single-dot) extensions collapse into one map-backed matcher; only
	// compound endings like ".min.js" (where filepath.Ext yields ".js") stay as
	// individual suffix matchers.
	extSet := make(map[string]bool, len(d.Exts))
	extKeys := make([]string, 0, len(d.Exts))
	for _, ext := range d.Exts {
		lower := strings.ToLower(ext)
		if filepath.Ext(lower) == lower {
			if !extSet[lower] {
				extSet[lower] = true
				extKeys = append(extKeys, lower)
			}
			continue
		}
		ms = append(ms, newExtMatcher(ext))
	}
	if len(extSet) > 0 {
		sort.Strings(extKeys)
		ms = append(ms, newExtSetMatcher(extSet, "extset:"+strings.Join(extKeys, ",")))
	}
	for _, ending := range d.Endings {
		ms = append(ms, newEndingMatcher(ending))
	}
	return ms
}

// UnscannableSource skips entries there is no point fingerprinting, whatever the
// other rules say. It is not configurable and applies on every collection,
// independently of the default lists and of the size bounds — these are not
// policy choices but statements about the entry:
//
//   - zero-byte files: no content to match, so the WFP entry carries a zero hash
//     and no lines — bytes on the wire no scan can act on;
//   - symbolic links: the target is collected on its own when it is inside the
//     tree, so following the link would report the same content twice.
func UnscannableSource() []Matcher {
	return []Matcher{emptyFileMatcher{}, symlinkMatcher{}}
}

// HiddenSource skips entries whose name begins with a dot.
//
// Not part of UnscannableSource: a dotfile has perfectly good content, so this
// is a policy choice about what belongs to a project, not a statement about the
// entry. That is why it can be switched off (Options.IncludeHidden) and why it
// is a source like any other rather than a check buried in the walk — a caller
// that cannot walk a tree needs to apply it too.
func HiddenSource() []Matcher {
	return []Matcher{hiddenMatcher{}}
}

// SizeSource turns a [min, max] byte range into a matcher. It is a source of its
// own — not part of DefaultSource — because the bounds come from the caller
// (--min-size/--max-size), not from the built-in lists: switching the defaults
// off must not discard a bound the caller asked for. A min of 0 imposes no
// minimum and a max of 0 no maximum, so 0/0 yields no matcher at all.
func SizeSource(min, max int64) []Matcher {
	if min <= 0 && max <= 0 {
		return nil
	}
	return []Matcher{newSizeMatcher(min, max)}
}

// SizeRule is one scanoss.json skip.sizes entry: files matching any of Patterns
// are skipped when smaller than Min or larger than Max (0 disables a bound).
type SizeRule struct {
	Patterns []string
	Min      int64
	Max      int64
}

// Skip mirrors the scanoss.json settings.skip subset filter consumes, already
// resolved to a single operation (scanning, fingerprinting, or dependencies).
type Skip struct {
	Patterns []string   // gitignore-style globs
	Sizes    []SizeRule // per-pattern size limits
}

// Settings is the local, dependency-free mirror of the scanoss.json bits filter
// needs. Callers map settings.Settings into this.
type Settings struct {
	Skip Skip
}

// SettingsSource turns scanoss.json skip rules into matchers. Returns nil when s
// is nil.
func SettingsSource(s *Settings) []Matcher {
	if s == nil {
		return nil
	}
	var ms []Matcher
	if len(s.Skip.Patterns) > 0 {
		ms = append(ms, newGlobMatcher(s.Skip.Patterns, "scanoss-skip"))
	}
	for _, rule := range s.Skip.Sizes {
		if len(rule.Patterns) == 0 {
			continue
		}
		ms = append(ms, newScopedSizeMatcher(rule.Patterns, rule.Min, rule.Max))
	}
	return ms
}

// GitIgnoreSource reads the .gitignore at the root of the tree (if present) and
// returns a matcher for its patterns. If root is a file rather than a directory,
// its parent directory is used. Missing file is not an error.
func GitIgnoreSource(root string) ([]Matcher, error) {
	dir := root
	if info, err := os.Stat(root); err == nil && !info.IsDir() {
		dir = filepath.Dir(root)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	return []Matcher{newGlobMatcher(lines, "gitignore")}, nil
}
