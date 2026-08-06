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

	"github.com/scanoss/scanoss.go/internal/logging"
)

// defaults holds the skip lists and size bounds a defaultSource turns into
// matchers. Callers normally start from stdDefaults and override fields as
// needed.
type defaults struct {
	Dirs    []string // directory names skipped wholesale
	DirExts []string // directory-name suffixes skipped (e.g. ".egg-info")
	Files   []string // exact file names skipped
	Exts    []string // file extensions skipped (leading dot)
	Endings []string // non-extension file-name suffixes skipped
}

// stdDefaults returns the built-in default skip lists. Size bounds are not part
// of it: they are caller input, not a built-in, and have their own sizeSource.
func stdDefaults() defaults {
	return defaults{
		Dirs:    skippedDirs(scanOnlySkippedDirs),
		DirExts: defaultSkippedDirExts,
		Files:   defaultSkippedFiles,
		Exts:    defaultSkippedExts,
		Endings: defaultSkippedFileEndings,
	}
}

// defaultSource turns every default skip list into matchers: the directory rules and the file
// rules together. Callers that want one half without the other use folderDefaultSource and
// fileDefaultSource, which is what the two --all-folders / --all-extensions switches select.
func defaultSource(d defaults) []matcher {
	return append(folderDefaultSource(d), fileDefaultSource(d)...)
}

// folderDefaultSource turns the default directory skip lists into matchers: whole directory names
// and directory-name suffixes. Skipping a directory prunes everything under it.
func folderDefaultSource(d defaults) []matcher {
	var ms []matcher
	for _, name := range d.Dirs {
		ms = append(ms, newDirNameMatcher(name))
	}
	for _, suffix := range d.DirExts {
		ms = append(ms, newDirSuffixMatcher(suffix))
	}
	return ms
}

// fileDefaultSource turns the default file skip lists into matchers: extensions, non-extension
// name endings, and exact names.
func fileDefaultSource(d defaults) []matcher {
	var ms []matcher
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

// unscannableSource skips entries there is no point fingerprinting
func unscannableSource() []matcher {
	return []matcher{emptyFileMatcher{}, symlinkMatcher{}}
}

// hiddenSource skips entries whose name begins with a dot.
//
// Not part of unscannableSource: a dotfile has perfectly good content, so this
// is a policy choice about what belongs to a project, not a statement about the
// entry. That is why it can be switched off (Options.IncludeHidden) and why it
// is a source like any other rather than a check buried in the walk — a caller
// that cannot walk a tree needs to apply it too.
func hiddenSource() []matcher {
	return []matcher{hiddenMatcher{}}
}

// sizeSource turns a [min, max] byte range into a matcher. It is a source of its
// own — not part of defaultSource — because the bounds come from the caller
// (Options.MinSize/MaxSize), not from the built-in lists: switching the defaults
// off must not discard a bound the caller asked for. A min of 0 imposes no
// minimum and a max of 0 no maximum, so 0/0 yields no matcher at all.
// An inverted range (min > max, both set) can match nothing; applying it would
// silently exclude every file, so it is warned about and ignored instead.
func sizeSource(min, max int64) []matcher {
	if min <= 0 && max <= 0 {
		return nil
	}
	if max > 0 && min > max {
		logging.Warn("ignoring size bounds: min exceeds max, no file could match",
			"min", min, "max", max)
		return nil
	}
	return []matcher{newSizeMatcher(min, max)}
}

// SizeRule is one scanoss.json skip.sizes entry: files matching any of Patterns
// are skipped when smaller than Min or larger than Max (0 disables a bound).
type SizeRule struct {
	Patterns []string
	Min      int64
	Max      int64
}

// patternSource turns the glob rules into matchers. The profiles have already read the
// project's scanoss.json into these fields, so there is one path here whether a rule
// came from the file or from the caller.
func patternSource(o Options) []matcher {
	var ms []matcher
	if len(o.SkipPatterns) > 0 {
		ms = append(ms, newGlobMatcher(o.SkipPatterns, "skip-patterns"))
	}
	for _, rule := range o.SizeRules {
		if len(rule.Patterns) == 0 {
			continue
		}
		// Same guard as sizeSource: an inverted range would exclude every matching
		// file, and a typo in scanoss.json must not silently lose data.
		if rule.Max > 0 && rule.Min > rule.Max {
			logging.Warn("ignoring scanoss.json skip.sizes rule: min exceeds max, no file could match",
				"patterns", strings.Join(rule.Patterns, ","), "min", rule.Min, "max", rule.Max)
			continue
		}
		ms = append(ms, newScopedSizeMatcher(rule.Patterns, rule.Min, rule.Max))
	}
	return ms
}

// gitIgnoreSource reads the .gitignore at the root of the tree (if present) and
// returns a matcher for its patterns. If root is a file rather than a directory,
// its parent directory is used. Missing file is not an error.
func gitIgnoreSource(root string) ([]matcher, error) {
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
	return []matcher{newGlobMatcher(lines, "gitignore")}, nil
}
