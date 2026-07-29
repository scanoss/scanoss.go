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
	"os"
	"path/filepath"

	"github.com/scanoss/scanoss.go/pkg/manifests"
)

// absPath returns the absolute path for p, falling back to p on error.
func absPath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

// Options configures a Collect call. Set the Skip* fields to replace the
// corresponding default list. Use DefaultOptions for the common case where the
// built-in defaults and .gitignore are applied.
type Options struct {
	// Skip* replace the matching built-in default list when non-nil.
	SkipDirs       []string
	SkipFiles      []string
	SkipExtensions []string

	// Size bounds. The built-in values are DefaultMinFileSize/DefaultMaxFileSize,
	// set by DefaultOptions/ScanOptions/DependencyOptions; assign these fields to
	// override them. They are applied as their own source, so turning Defaults
	// off keeps a bound the caller asked for. A zero-valued Options (built
	// literally, without a constructor) means no bound on either side.
	MinSize int64 // minimum file size in bytes; 0 imposes no minimum
	MaxSize int64 // maximum file size in bytes; 0 imposes no maximum (unlimited)

	Defaults  bool // apply the built-in default skip lists
	GitIgnore bool // honor .gitignore

	// IncludeHidden collects entries whose name begins with a dot. They are
	// excluded by default: a scan wants the project's source, not its tooling.
	// Version-control metadata (.git and friends) stays excluded either way — see
	// UnscannableSource, which no option can switch off.
	IncludeHidden bool

	// Settings is the scanoss.json skip/folders rules, already resolved to a
	// single operation. Nil when there is no scanoss.json.
	Settings *Settings

	// PreserveDependencyManifests keeps dependency manifest files
	// (package.json, go.mod, pom.xml, … — see pkg/manifests) even when a skip
	// rule would otherwise drop them. Use it for stages that consume manifests
	// (extraction/upload feeding the dependency parser) while still pruning
	// everything else. Fingerprint scanning leaves this false — manifests are
	// not useful for matching. Default false → unchanged behavior.
	PreserveDependencyManifests bool

	HFH bool // reserved: high-file-hashing (folder hashing) variants; not yet used
}

// DefaultOptions returns the common-case Options: the built-in default skip
// lists and .gitignore are applied, with no scanoss.json.
func DefaultOptions() Options {
	return Options{
		Defaults:  true,
		GitIgnore: true,
		MinSize:   DefaultMinFileSize,
		MaxSize:   DefaultMaxFileSize,
	}
}

// ScanOptions returns the options for fingerprint scanning: the built-in
// defaults and .gitignore, with dependency manifests skipped (they are not
// useful for matching). Alias of DefaultOptions, named for intent.
func ScanOptions() Options {
	return Options{
		Defaults:  true,
		GitIgnore: true,
		MinSize:   DefaultMinFileSize,
		MaxSize:   DefaultMaxFileSize,
	}
}

// FingerprintOptions returns the options for the fingerprint-only path (the wfp
// command). Identical to ScanOptions today — the two differ only in which
// scanoss.json section the caller supplies — but named separately so each layer
// states which profile it uses, and so the two can diverge without a caller
// silently inheriting the wrong one.
func FingerprintOptions() Options {
	return ScanOptions()
}

// DependencyOptions returns the options for collecting dependency manifests.
// Three things differ from ScanOptions, and all three are deliberate:
//
//   - the directory list adds DependencyOnlySkippedDirs instead of the scanning ones;
//   - manifests are preserved, since they live behind skipped extensions;
//   - .gitignore is NOT applied. It answers "should this be versioned", not "is
//     this a dependency": a lock file excluded from git still declares what the
//     project uses, and losing a declaration is worse than analysing one extra.
func DependencyOptions() Options {
	return Options{
		Defaults:                    true,
		GitIgnore:                   false,
		MinSize:                     DefaultMinFileSize,
		MaxSize:                     DefaultMaxFileSize,
		SkipDirs:                    skippedDirs(DependencyOnlySkippedDirs),
		PreserveDependencyManifests: true,
	}
}

// keepMatcher wraps a base skip matcher with the PreserveDependencyManifests
// exemption: a file that a base rule would skip is kept when it is a dependency
// manifest. The exemption applies to files only — directories are matched by the
// base matcher unchanged (so skipped dirs like node_modules are not descended).
type keepMatcher struct {
	base              Matcher // built-in rules: the exemption may override these
	userRules         Matcher // scanoss.json: the exemption may not
	preserveManifests bool
}

func (m *keepMatcher) Match(rel string, info os.FileInfo) bool {
	// A rule the project wrote in scanoss.json is checked first and is final.
	// The exemption exists so our own extension list does not swallow the
	// manifests a dependency scan needs — it is not there to overrule someone
	// who said, explicitly, not to look at a given file.
	if m.userRules != nil && m.userRules.Match(rel, info) {
		return true
	}
	if !m.base.Match(rel, info) {
		return false
	}
	if m.preserveManifests && !info.IsDir() && manifests.Is(rel) {
		return false // kept despite the built-in rule
	}
	return true
}

func (m *keepMatcher) Key() string { return "keep:" + m.base.Key() }

// CollectResult is the outcome of a Collect: the absolute paths to scan and how
// many files were skipped. The skipped files themselves are not retained.
type CollectResult struct {
	Files        []string
	SkippedCount int
}

// defaults builds the Defaults to use, applying any overrides from o.
func (o Options) defaults() Defaults {
	d := StdDefaults()
	if o.SkipDirs != nil {
		d.Dirs = o.SkipDirs
	}
	if o.SkipFiles != nil {
		d.Files = o.SkipFiles
	}
	if o.SkipExtensions != nil {
		d.Exts = o.SkipExtensions
	}
	return d
}

// Collect walks root once, returning the files to scan and a count of those
// skipped. Rules are loaded from the enabled sources (defaults, scanoss.json,
// .gitignore), deduplicated, and applied as a single composite — including the
// hidden-entry rule, unless Options.IncludeHidden says otherwise. Zero-byte
// files and symbolic links are always skipped (see UnscannableSource).
// Symlinked directories are not followed. Returned paths are absolute.
func Collect(root string, o Options) (*CollectResult, error) {
	var sources [][]Matcher
	sources = append(sources, UnscannableSource())
	if !o.IncludeHidden {
		sources = append(sources, HiddenSource())
	}
	if o.Defaults {
		sources = append(sources, DefaultSource(o.defaults()))
	}
	if sz := SizeSource(o.MinSize, o.MaxSize); sz != nil {
		sources = append(sources, sz)
	}
	if o.GitIgnore {
		gi, err := GitIgnoreSource(root)
		if err != nil {
			return nil, err
		}
		if gi != nil {
			sources = append(sources, gi)
		}
	}
	var userRules Matcher
	if o.Settings != nil {
		if ms := SettingsSource(o.Settings); len(ms) > 0 {
			userRules = Build(ms)
		}
	}
	skip := &keepMatcher{
		base:              Build(sources...),
		userRules:         userRules,
		preserveManifests: o.PreserveDependencyManifests,
	}

	res := &CollectResult{}
	walkErr := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}

		if info.IsDir() {
			if path == root {
				return nil
			}
			if skip.Match(rel, info) {
				return filepath.SkipDir
			}
			return nil
		}

		if skip.Match(rel, info) {
			res.SkippedCount++
			return nil
		}

		res.Files = append(res.Files, absPath(path))
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return res, nil
}
