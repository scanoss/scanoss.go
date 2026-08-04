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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gitignore "github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// matcher decides whether a single path should be skipped. It is the leaf of the
// composite pattern; composite implements it too, so a set of matchers is itself
// a matcher. Match returns true when the path should be skipped. Skipped files
// are not tracked, so a boolean (no reason) is all that is needed.
type matcher interface {
	// Match reports whether the path (rel, relative to the scan root) should be
	// skipped. info describes the entry.
	Match(rel string, info os.FileInfo) bool
	// Key is a stable identity used to deduplicate matchers built from different
	// sources (e.g. "ext:.png", "dir:vendor").
	Key() string
}

// composite holds many matchers and skips a path if any of them does. It is
// itself a matcher, so composites can nest.
type composite struct {
	matchers []matcher
}

// Match reports whether any contained matcher skips the path.
func (c *composite) Match(rel string, info os.FileInfo) bool {
	for _, m := range c.matchers {
		if m.Match(rel, info) {
			return true
		}
	}
	return false
}

// Key identifies the composite.
func (c *composite) Key() string { return "composite" }

// suffixMatcher skips files whose name ends with a given suffix (matched
// case-insensitively). It backs both extension matching (suffix includes the
// leading dot, e.g. ".png", ".min.js") and "file ending" matching (no dot, e.g.
// "readme"). Directories are never matched.
type suffixMatcher struct {
	suffix string
	key    string
}

func (m suffixMatcher) Match(rel string, info os.FileInfo) bool {
	if info.IsDir() {
		return false
	}
	return strings.HasSuffix(strings.ToLower(info.Name()), m.suffix)
}

func (m suffixMatcher) Key() string { return m.key }

// newExtMatcher matches files by extension suffix (the value must include the
// leading dot, e.g. ".png").
func newExtMatcher(ext string) suffixMatcher {
	ext = strings.ToLower(ext)
	return suffixMatcher{suffix: ext, key: "ext:" + ext}
}

// newEndingMatcher matches files whose name ends with a non-extension suffix
// (e.g. "readme", "changelog").
func newEndingMatcher(ending string) suffixMatcher {
	ending = strings.ToLower(ending)
	return suffixMatcher{suffix: ending, key: "ending:" + ending}
}

// extSetMatcher skips files whose extension is in a set, in one O(1) map lookup.
// It replaces a long run of individual suffixMatchers for simple (single-dot)
// extensions: for such an extension, strings.HasSuffix(name, ".x") is equivalent
// to filepath.Ext(name) == ".x" (the trailing ".x" holds the last dot), so the
// filtering result is unchanged. Compound endings like ".min.js" — where
// filepath.Ext yields ".js" — are not simple and stay as suffixMatchers.
type extSetMatcher struct {
	exts map[string]bool
	key  string
}

// newExtSetMatcher builds a set matcher from simple extensions (each including
// the leading dot, e.g. ".png"). key must be stable so build can deduplicate.
func newExtSetMatcher(exts map[string]bool, key string) extSetMatcher {
	return extSetMatcher{exts: exts, key: key}
}

func (m extSetMatcher) Match(rel string, info os.FileInfo) bool {
	if info.IsDir() {
		return false
	}
	return m.exts[strings.ToLower(filepath.Ext(info.Name()))]
}

func (m extSetMatcher) Key() string { return m.key }

// nameMatcher skips files whose name exactly equals a given name (case-insensitive).
type nameMatcher struct {
	name string
}

func newNameMatcher(name string) nameMatcher {
	return nameMatcher{name: strings.ToLower(name)}
}

func (m nameMatcher) Match(rel string, info os.FileInfo) bool {
	if info.IsDir() {
		return false
	}
	return strings.ToLower(info.Name()) == m.name
}

func (m nameMatcher) Key() string { return "name:" + m.name }

// dirNameMatcher skips directories whose name exactly equals a given name.
type dirNameMatcher struct {
	name string
}

func newDirNameMatcher(name string) dirNameMatcher {
	return dirNameMatcher{name: strings.ToLower(name)}
}

func (m dirNameMatcher) Match(rel string, info os.FileInfo) bool {
	return info.IsDir() && strings.ToLower(info.Name()) == m.name
}

func (m dirNameMatcher) Key() string { return "dir:" + m.name }

// dirSuffixMatcher skips directories whose name ends with a given suffix
// (e.g. ".egg-info").
type dirSuffixMatcher struct {
	suffix string
}

func newDirSuffixMatcher(suffix string) dirSuffixMatcher {
	return dirSuffixMatcher{suffix: strings.ToLower(suffix)}
}

func (m dirSuffixMatcher) Match(rel string, info os.FileInfo) bool {
	return info.IsDir() && strings.HasSuffix(strings.ToLower(info.Name()), m.suffix)
}

func (m dirSuffixMatcher) Key() string { return "dirsuffix:" + m.suffix }

// symlinkMatcher skips symbolic links. The link's target, when it is inside the
// tree, is collected on its own, so following the link would fingerprint the
// same content twice under two names; when the target is outside the tree, or
// broken, it is not ours to report. Requires an os.FileInfo from Lstat (which is
// what filepath.Walk supplies); a Stat-derived one describes the target and
// never reports a link.
type symlinkMatcher struct{}

func (symlinkMatcher) Match(rel string, info os.FileInfo) bool {
	return info.Mode()&os.ModeSymlink != 0
}

func (symlinkMatcher) Key() string { return "symlink" }

// hiddenMatcher skips entries whose name begins with a dot. It matches
// directories too, so a walk can prune the whole subtree.
type hiddenMatcher struct{}

func (hiddenMatcher) Match(rel string, info os.FileInfo) bool {
	return strings.HasPrefix(info.Name(), ".")
}

func (hiddenMatcher) Key() string { return "hidden" }

// emptyFileMatcher skips zero-byte files. Unlike sizeMatcher it carries no
// configuration: there is no byte count to compare against, only the absence of
// content. Directories are never matched.
type emptyFileMatcher struct{}

func (emptyFileMatcher) Match(rel string, info os.FileInfo) bool {
	return !info.IsDir() && info.Size() == 0
}

func (emptyFileMatcher) Key() string { return "empty" }

// sizeMatcher skips files outside a [min, max] byte range. A min of 0 disables
// the lower bound; a max of 0 disables the upper bound (unlimited).
type sizeMatcher struct {
	min int64
	max int64
}

func newSizeMatcher(min, max int64) sizeMatcher {
	return sizeMatcher{min: min, max: max}
}

func (m sizeMatcher) Match(rel string, info os.FileInfo) bool {
	if info.IsDir() {
		return false
	}
	size := info.Size()
	if m.min > 0 && size < m.min {
		return true
	}
	if m.max > 0 && size > m.max {
		return true
	}
	return false
}

func (m sizeMatcher) Key() string { return fmt.Sprintf("size:%d:%d", m.min, m.max) }

// globMatcher skips paths matching a set of gitignore-style patterns. The
// patterns are compiled together into one go-git matcher (not one matcher per
// pattern) so gitignore semantics that span patterns — negation ("!") and
// anchoring — are preserved. Used for both .gitignore files and scanoss.json
// skip.patterns. Paths are matched relative to the scan root.
type globMatcher struct {
	matcher gitignore.Matcher
	key     string
}

// newGlobMatcher parses the given patterns into a single matcher (blank lines
// and comments are skipped). keyPrefix distinguishes the pattern group's origin
// (e.g. "gitignore", "scanoss") in the dedup key.
func newGlobMatcher(patterns []string, keyPrefix string) globMatcher {
	ps := make([]gitignore.Pattern, 0, len(patterns))
	for _, p := range patterns {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		ps = append(ps, gitignore.ParsePattern(p, nil))
	}
	return globMatcher{
		matcher: gitignore.NewMatcher(ps),
		key:     keyPrefix + ":" + strings.Join(patterns, "\n"),
	}
}

func (m globMatcher) Match(rel string, info os.FileInfo) bool {
	if m.matcher == nil {
		return false
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	return m.matcher.Match(parts, info.IsDir())
}

func (m globMatcher) Key() string { return m.key }

// scopedSizeMatcher skips files that match a glob set AND fall outside [min, max].
// It backs scanoss.json skip.sizes rules (each rule is scoped to its patterns),
// unlike sizeMatcher which applies a global bound to every file.
type scopedSizeMatcher struct {
	matcher gitignore.Matcher
	min     int64
	max     int64
	key     string
}

func newScopedSizeMatcher(patterns []string, min, max int64) scopedSizeMatcher {
	ps := make([]gitignore.Pattern, 0, len(patterns))
	for _, p := range patterns {
		if strings.TrimSpace(p) == "" {
			continue
		}
		ps = append(ps, gitignore.ParsePattern(p, nil))
	}
	return scopedSizeMatcher{
		matcher: gitignore.NewMatcher(ps),
		min:     min,
		max:     max,
		key:     fmt.Sprintf("scoped-size:%s:%d:%d", strings.Join(patterns, ","), min, max),
	}
}

func (m scopedSizeMatcher) Match(rel string, info os.FileInfo) bool {
	if info.IsDir() {
		return false
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if !m.matcher.Match(parts, false) {
		return false
	}
	size := info.Size()
	if m.min > 0 && size < m.min {
		return true
	}
	if m.max > 0 && size > m.max {
		return true
	}
	return false
}

func (m scopedSizeMatcher) Key() string { return m.key }

// matchKey reports which contained rule skips the path, or "" when none does. It is
// Match with the reason attached: the keys already exist to deduplicate matchers, and
// they happen to be exactly what answers "why was this file not scanned?".
func (c *composite) matchKey(rel string, info os.FileInfo) string {
	for _, m := range c.matchers {
		if m.Match(rel, info) {
			return m.Key()
		}
	}
	return ""
}
