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
	"log/slog"
	"os"
	"path/filepath"
	"slices"

	"github.com/scanoss/scanoss.go/internal/logging"
	"github.com/scanoss/scanoss.go/pkg/manifests"
	"github.com/scanoss/scanoss.go/pkg/settings"
)

// absPath returns the absolute path for p, falling back to p on error.
func absPath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

// Options configures a Collect call. Start from Scanning, Fingerprinting or Dependencies
// or build one literally — a zero Options filters nothing beyond what is never
// scannable (empty files, symlinks) and hidden entries.
type Options struct {
	// BuiltinFolderRules applies the built-in directory lists (node_modules, vendor,
	// build output, …). BuiltinFileRules applies the built-in file lists: exact names,
	// extensions and name endings, which together answer one question — is this file
	// worth fingerprinting.
	//
	// Turning either off does not touch the Skip* fields below: those are the caller's
	// own rules, not part of the built-in policy. "No built-in folder lists" and "no
	// folder rules at all" are different requests, and this flag only makes the first.
	BuiltinFolderRules bool
	BuiltinFileRules   bool

	// The caller's own skip rules, layered on top of whatever the flags above admit.
	//
	// These are names, not paths and not globs: "node_modules", never "/node_modules"
	// or "src/**". A name matches at any depth. For paths and patterns use SkipPatterns.
	SkipDirs    []string // directory names
	SkipDirExts []string // directory-name suffixes, e.g. ".egg-info"
	SkipFiles   []string // exact file names
	SkipExts    []string // file extensions, leading dot: ".png"

	// Size bounds, applied as their own rule so they survive turning the built-in
	// lists off. 0 means no bound on that side; the built-in values are
	// DefaultMinFileSize / DefaultMaxFileSize.
	MinSize int64
	MaxSize int64

	GitIgnore bool // honor .gitignore

	// IncludeHidden collects entries whose name begins with a dot. They are excluded by default: a
	// scan wants the project's source, not its tooling. Setting it reaches version-control
	// metadata too — .git and friends are dotted like anything else — which matches the reference
	// implementation, where the equivalent flag has the same reach.
	IncludeHidden bool

	// SkipPatterns are gitignore-style globs: "src/**", "*.min.js". SizeRules bound the
	// size of files matching a glob. The profiles fill both from the project's
	// scanoss.json; a caller may add its own.
	//
	// Unlike the name-based fields above, these are final — KeepManifests never
	// overrides one, because a caller that named a path meant that path.
	SkipPatterns []string
	SizeRules    []SizeRule

	// KeepManifests keeps dependency manifest files (package.json, go.mod, pom.xml, …
	// — see pkg/manifests) even when a skip rule would otherwise drop them. Use it for
	// stages that consume manifests while still pruning everything else. Fingerprint
	// scanning leaves it false: a manifest is a declaration, not a file worth matching.
	KeepManifests bool
}

// One profile per scanoss.json operation. Each takes the project's parsed file and
// pulls its own section out of it, so the caller never picks a section — the profile it
// asked for already is one. A nil scanossSettings means no project rules.

// Scanning selects the files worth matching: the built-in skip lists apply and
// .gitignore is honoured. Dependency manifests are dropped — a declaration is not a
// file to match against the knowledge base.
func Scanning(scanossSettings *settings.Settings) Options {
	return sourceFiles(scanossSettings, settings.OperationScanning)
}

// Fingerprinting is Scanning's ruleset over the fingerprinting section. The two apply
// the same rules and differ only in which rules the project wrote for them.
func Fingerprinting(scanossSettings *settings.Settings) Options {
	return sourceFiles(scanossSettings, settings.OperationFingerprinting)
}

func sourceFiles(s *settings.Settings, operation string) Options {
	o := Options{
		BuiltinFolderRules: true,
		BuiltinFileRules:   true,
		GitIgnore:          true,
		MinSize:            DefaultMinFileSize,
		MaxSize:            DefaultMaxFileSize,
	}
	o.SkipPatterns, o.SizeRules = projectRules(s, operation)
	return o
}

// projectRules reads one operation's skip rules out of a parsed scanoss.json. Nil
// settings yields nothing, which is what a project without the file should get.
func projectRules(s *settings.Settings, operation string) ([]string, []SizeRule) {
	if s == nil {
		return nil, nil
	}
	sizes := s.Settings.SkipSizes(operation)
	rules := make([]SizeRule, 0, len(sizes))
	for _, r := range sizes {
		rules = append(rules, SizeRule{Patterns: r.Patterns, Min: r.Min, Max: r.Max})
	}
	return s.Settings.SkipPatterns(operation), rules
}

// Dependencies collects the files a dependency stage should see. It keeps the manifests the
// file rules would otherwise drop, but it does not select only manifests: a caller that wants
// just those filters the result by what its parser handles.
//
// Three things differ from Scanning, and all three are deliberate:
//
//   - the directory list prunes generated trees instead of the scanning ones;
//   - manifests are preserved, since they live behind skipped extensions;
//   - .gitignore is NOT applied. It answers "should this be versioned", not "is
//     this a dependency": a lock file excluded from git still declares what the
//     project uses, and losing a declaration is worse than analysing one extra.
//
// The built-in folder lists are off and the directory rules are given explicitly: the scanning
// lists prune venv/ and examples/, where manifests legitimately live. dist/, build/ and target/
// are pruned here instead, since a manifest under one of them is build output.
// The directory-suffix list is the built-in one, which those lists do not disagree on.
func Dependencies(scanossSettings *settings.Settings) Options {
	o := Options{
		BuiltinFolderRules: false,
		BuiltinFileRules:   true,
		GitIgnore:          false,
		MinSize:            DefaultMinFileSize,
		MaxSize:            DefaultMaxFileSize,
		SkipDirs:           skippedDirs(dependencyOnlySkippedDirs),
		SkipDirExts:        slices.Clone(defaultSkippedDirExts),
		KeepManifests:      true,
	}
	o.SkipPatterns, o.SizeRules = projectRules(scanossSettings, settings.OperationDependencies)
	return o
}

// keepMatcher wraps a base skip matcher with the KeepManifests
// exemption: a file that a base rule would skip is kept when it is a dependency
// manifest. The exemption applies to files only — directories are matched by the
// base matcher unchanged (so skipped dirs like node_modules are not descended).
type keepMatcher struct {
	base              *composite // built-in rules: the exemption may override these
	userRules         *composite // the caller's own patterns: the exemption may not
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

// CollectResult is the outcome of a Collect: the absolute paths to scan and how
// many files were skipped. The skipped files themselves are not retained.
type CollectResult struct {
	Files        []string
	SkippedCount int
}

// Collect walks root once, returning the files to scan and a count of those
// skipped. Rules are loaded from the enabled sources (defaults, scanoss.json,
// .gitignore), deduplicated, and applied as a single composite — including the
// hidden-entry rule, unless Options.IncludeHidden says otherwise. Zero-byte
// files and symbolic links are always skipped, unconditionally.
// Symlinked directories are not followed. Returned paths are absolute.
func Collect(root string, o Options) (*CollectResult, error) {
	var sources [][]matcher
	sources = append(sources, unscannableSource())
	if !o.IncludeHidden {
		sources = append(sources, hiddenSource())
	}
	// The built-in lists and the caller's own are separate sources, added independently:
	// that is what lets a caller drop the built-ins and keep its own rules. build
	// deduplicates by key, so a name in both collapses to one matcher.
	if o.BuiltinFolderRules {
		sources = append(sources, folderDefaultSource(stdDefaults()))
	}
	if len(o.SkipDirs) > 0 || len(o.SkipDirExts) > 0 {
		sources = append(sources, folderDefaultSource(defaults{Dirs: o.SkipDirs, DirExts: o.SkipDirExts}))
	}
	if o.BuiltinFileRules {
		sources = append(sources, fileDefaultSource(stdDefaults()))
	}
	if len(o.SkipFiles) > 0 || len(o.SkipExts) > 0 {
		sources = append(sources, fileDefaultSource(defaults{Files: o.SkipFiles, Exts: o.SkipExts}))
	}
	if sz := sizeSource(o.MinSize, o.MaxSize); sz != nil {
		sources = append(sources, sz)
	}
	if o.GitIgnore {
		gi, err := gitIgnoreSource(root)
		if err != nil {
			return nil, err
		}
		if gi != nil {
			sources = append(sources, gi)
		}
	}
	// The caller's patterns are kept apart from everything above: KeepManifests may
	// override a built-in rule, never a path the caller named.
	var userRules *composite
	if ms := patternSource(o); len(ms) > 0 {
		userRules = build(ms)
	}
	skip := &keepMatcher{
		base:              build(sources...),
		userRules:         userRules,
		preserveManifests: o.KeepManifests,
	}

	logging.Debug("filters applied", "root", root,
		"builtinFolderRules", o.BuiltinFolderRules, "builtinFileRules", o.BuiltinFileRules,
		"gitignore", o.GitIgnore, "includeHidden", o.IncludeHidden,
		"minSize", o.MinSize, "maxSize", o.MaxSize,
		"skipDirs", len(o.SkipDirs), "skipFiles", len(o.SkipFiles), "skipExts", len(o.SkipExts),
		"skipPatterns", len(o.SkipPatterns), "sizeRules", len(o.SizeRules),
		"keepManifests", o.KeepManifests, "matchers", len(skip.base.matchers))

	// Resolved once: the reason is only built when someone is listening.
	explain := logging.Enabled(slog.LevelDebug)

	res := &CollectResult{}
	walkErr := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}

		// With Debug off this is Match; with it on, the same walk also reports which rule
		// each exclusion came from — the question a filtered file always raises, and one
		// SkippedCount alone cannot answer.
		if info.IsDir() {
			if path == root {
				return nil
			}
			if reason := skip.matchKey(rel, info); reason != "" {
				if explain {
					logging.Debug("directory pruned", "path", rel, "rule", reason)
				}
				return filepath.SkipDir
			}
			return nil
		}

		if reason := skip.matchKey(rel, info); reason != "" {
			if explain {
				logging.Debug("file excluded", "path", rel, "rule", reason)
			}
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

// matchKey is Match with the reason attached, following the same precedence.
func (m *keepMatcher) matchKey(rel string, info os.FileInfo) string {
	if m.userRules != nil {
		if key := m.userRules.matchKey(rel, info); key != "" {
			return key
		}
	}
	key := m.base.matchKey(rel, info)
	if key == "" {
		return ""
	}
	if m.preserveManifests && !info.IsDir() && manifests.Is(rel) {
		return "" // kept despite the built-in rule
	}
	return key
}
