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
	"sort"
	"strings"
	"testing"

	"github.com/scanoss/scanoss.go/pkg/settings"
)

func keySet(ms []matcher) map[string]bool {
	s := make(map[string]bool, len(ms))
	for _, m := range ms {
		s[m.Key()] = true
	}
	return s
}

func TestDefaultSource(t *testing.T) {
	d := stdDefaults()
	ms := defaultSource(d)

	// Simple (single-dot) extensions fold into one map-backed matcher; compound
	// endings (e.g. ".min.js") stay as individual suffix matchers. Everything
	// else is one matcher per list entry. The defaults set neither size bound, so
	// there is no size matcher.
	var compound int
	for _, ext := range d.Exts {
		lower := strings.ToLower(ext)
		if filepath.Ext(lower) != lower {
			compound++
		}
	}
	want := len(d.Dirs) + len(d.DirExts) + len(d.Files) + len(d.Endings) + compound + 1 /*extset*/
	if len(ms) != want {
		t.Fatalf("defaultSource count = %d, want %d", len(ms), want)
	}

	keys := keySet(ms)
	for _, k := range []string{
		"dir:vendor", "dir:node_modules", "dirsuffix:.egg-info",
		"name:makefile", "ending:readme",
	} {
		if !keys[k] {
			t.Errorf("defaultSource missing matcher %q", k)
		}
	}

	// Simple extensions are matched via the set; the compound ".min.js" via its
	// own suffix matcher. Both must still be skipped.
	c := build(ms)
	if !c.Match("a.png", aFile("a.png", 200)) {
		t.Error("a.png should be skipped (extset)")
	}
	if !c.Match("app.min.js", aFile("app.min.js", 200)) {
		t.Error("app.min.js should be skipped (compound suffix)")
	}
}

// The built-in defaults carry no size bound at all: size is caller input, built
// by sizeSource, so no default run stat-compares a file for size.
func TestDefaultSourceHasNoSizeMatcher(t *testing.T) {
	for _, m := range defaultSource(stdDefaults()) {
		if strings.HasPrefix(m.Key(), "size:") {
			t.Errorf("defaultSource built a size matcher %q, want none", m.Key())
		}
	}
}

func TestDefaultSourceNoSizeWhenUnset(t *testing.T) {
	ms := defaultSource(defaults{Dirs: []string{"build"}})
	if len(ms) != 1 {
		t.Fatalf("count = %d, want 1", len(ms))
	}
	if ms[0].Key() != "dir:build" {
		t.Fatalf("key = %q, want dir:build", ms[0].Key())
	}
}

func TestSizeSource(t *testing.T) {
	if ms := sizeSource(0, 0); ms != nil {
		t.Fatalf("sizeSource(0, 0) = %v, want nil", keySet(ms))
	}

	ms := sizeSource(0, 1000)
	if len(ms) != 1 || ms[0].Key() != "size:0:1000" {
		t.Fatalf("matchers = %v, want one size:0:1000", keySet(ms))
	}
	if !ms[0].Match("big", aFile("big", 2000)) {
		t.Error("file above max should be skipped")
	}
	if ms[0].Match("ok", aFile("ok", 500)) {
		t.Error("file within max should not be skipped")
	}

	ms = sizeSource(100, 0)
	if len(ms) != 1 || ms[0].Key() != "size:100:0" {
		t.Fatalf("matchers = %v, want one size:100:0", keySet(ms))
	}
	if !ms[0].Match("tiny", aFile("tiny", 40)) {
		t.Error("file below min should be skipped")
	}
}

// scanossSettings builds a parsed scanoss.json carrying rules for one operation only,
// so a test can prove Collect read that section and not another.
func scanossSettings(op string, patterns []string, sizes []settings.SizeRule) *settings.Settings {
	var byOp settings.SkipPatternsByOp
	var sizesByOp settings.SkipSizesByOp
	switch op {
	case settings.OperationScanning:
		byOp.Scanning, sizesByOp.Scanning = patterns, sizes
	case settings.OperationFingerprinting:
		byOp.Fingerprinting, sizesByOp.Fingerprinting = patterns, sizes
	case settings.OperationDependencies:
		byOp.Dependencies, sizesByOp.Dependencies = patterns, sizes
	}
	return &settings.Settings{Settings: settings.Tuning{
		Skip: settings.Skip{Patterns: byOp, Sizes: sizesByOp},
	}}
}

func TestPatternSourceEmpty(t *testing.T) {
	if ms := patternSource(Options{}); ms != nil {
		t.Fatalf("patternSource with no rules = %v, want nil", ms)
	}
}

// Each profile reads its own scanoss.json section. Reading the wrong one is the mistake
// the three per-operation accessors used to invite; now the profile is the section.
func TestProfilesReadTheirOwnSection(t *testing.T) {
	s := scanossSettings(settings.OperationDependencies, []string{"only-deps/**"}, nil)

	if got := Dependencies(s).SkipPatterns; len(got) != 1 || got[0] != "only-deps/**" {
		t.Errorf("Dependencies patterns = %v, want [only-deps/**]", got)
	}
	if got := Scanning(s).SkipPatterns; len(got) != 0 {
		t.Errorf("Scanning must not see the dependencies rules, got %v", got)
	}
	if got := Fingerprinting(s).SkipPatterns; len(got) != 0 {
		t.Errorf("Fingerprinting must not see the dependencies rules, got %v", got)
	}
}

// Scanning and Fingerprinting apply the same rules and differ only in the section they
// read — which is why they are two profiles and not one.
func TestScanningAndFingerprintingReadDistinctSections(t *testing.T) {
	s := &settings.Settings{Settings: settings.Tuning{Skip: settings.Skip{
		Patterns: settings.SkipPatternsByOp{
			Scanning:       []string{"only-scanning/**"},
			Fingerprinting: []string{"only-fingerprinting/**"},
		},
	}}}
	if got := Scanning(s).SkipPatterns; len(got) != 1 || got[0] != "only-scanning/**" {
		t.Errorf("Scanning patterns = %v", got)
	}
	if got := Fingerprinting(s).SkipPatterns; len(got) != 1 || got[0] != "only-fingerprinting/**" {
		t.Errorf("Fingerprinting patterns = %v", got)
	}
}

// A project without a scanoss.json gets the profile and nothing else.
func TestProfilesWithoutSettings(t *testing.T) {
	for name, o := range map[string]Options{
		"Scanning": Scanning(nil), "Fingerprinting": Fingerprinting(nil), "Dependencies": Dependencies(nil),
	} {
		if len(o.SkipPatterns) != 0 || len(o.SizeRules) != 0 {
			t.Errorf("%s(nil) carried project rules: %+v", name, o)
		}
	}
}

// A caller may add its own patterns on top of the project's.
func TestCallerPatternsLayerOnTheProjects(t *testing.T) {
	o := Scanning(scanossSettings(settings.OperationScanning, []string{"from-file/**"}, nil))
	o.SkipPatterns = append(o.SkipPatterns, "from-caller/**")
	m := build(patternSource(o))
	if !m.Match("from-caller/x.go", aFile("x.go", 10)) {
		t.Error("the caller's own pattern must apply")
	}
	if !m.Match("from-file/x.go", aFile("x.go", 10)) {
		t.Error("the project's pattern must apply")
	}
}

func TestSettingsSourcePatterns(t *testing.T) {
	ms := patternSource(Options{SkipPatterns: []string{"*.log", "build/**"}})
	if len(ms) != 1 {
		t.Fatalf("count = %d, want 1 (one glob group)", len(ms))
	}
	m := ms[0]
	if !m.Match("a.log", aFile("a.log", 10)) {
		t.Error("*.log should match a.log")
	}
	if !m.Match("build/x.go", aFile("x.go", 10)) {
		t.Error("build/** should match build/x.go")
	}
	if m.Match("main.go", aFile("main.go", 10)) {
		t.Error("patterns should not match main.go")
	}
}

func TestSettingsSourceSizes(t *testing.T) {
	ms := patternSource(Options{
		SizeRules: []SizeRule{{Patterns: []string{"*.bin"}, Max: 100}},
	})
	if len(ms) != 1 {
		t.Fatalf("count = %d, want 1 (one scoped-size matcher)", len(ms))
	}
	m := ms[0]
	if !m.Match("big.bin", aFile("big.bin", 200)) {
		t.Error("matching pattern over max should be skipped")
	}
	if m.Match("small.bin", aFile("small.bin", 50)) {
		t.Error("matching pattern within size should not be skipped")
	}
	if m.Match("big.txt", aFile("big.txt", 200)) {
		t.Error("non-matching pattern should not be skipped on size")
	}
}

func TestSettingsSourceSizeRuleWithoutPatternsIgnored(t *testing.T) {
	ms := patternSource(Options{
		SizeRules: []SizeRule{{Patterns: nil, Max: 100}},
	})
	if len(ms) != 0 {
		t.Fatalf("count = %d, want 0 (size rule without patterns is ignored)", len(ms))
	}
}

func TestSettingsSourcePatternsAndSizes(t *testing.T) {
	ms := patternSource(Options{
		SkipPatterns: []string{"*.log"},
		SizeRules:    []SizeRule{{Patterns: []string{"*.bin"}, Max: 100}},
	})
	if len(ms) != 2 {
		t.Fatalf("count = %d, want 2", len(ms))
	}
}

func TestGitIgnoreSourceMissing(t *testing.T) {
	ms, err := gitIgnoreSource(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if ms != nil {
		t.Fatalf("missing .gitignore should yield nil, got %v", ms)
	}
}

func TestGitIgnoreSourcePresent(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("*.tmp\nbuild/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ms, err := gitIgnoreSource(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 1 {
		t.Fatalf("count = %d, want 1", len(ms))
	}
	m := ms[0]
	if !m.Match("a.tmp", aFile("a.tmp", 10)) {
		t.Error("*.tmp should match a.tmp")
	}
	if !m.Match("build", aDir("build")) {
		t.Error("build/ should match the build directory")
	}
	if m.Match("main.go", aFile("main.go", 10)) {
		t.Error(".gitignore should not match main.go")
	}
}

// The scanning directory set is the one the CLI has always applied. It is
// asserted against literals rather than recomposed from the same lists the code
// uses, so a mistake in the split shows up here instead of agreeing with itself.
func TestScanningDirSetUnchanged(t *testing.T) {
	got := append([]string(nil), stdDefaults().Dirs...)
	sort.Strings(got)

	// .git is deliberately absent: it is excluded by the hidden rule, like any other
	// dotted entry. Listing it here as well would put the same rule in two places,
	// and the two are not equivalent — this list is what --all-folders switches off,
	// the hidden rule is what --all-hidden switches off.
	want := []string{
		"__pycache__", "__pypackages__", "_yardoc", "eggs", "example",
		"examples", "htmlcov", "nbbuild", "nbdist", "nbproject",
		"node_modules", "vendor", "venv", "wheels",
	}
	if len(got) != len(want) {
		t.Fatalf("scanning dirs = %d entries %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("scanning dirs = %v, want %v", got, want)
		}
	}
}

// Dependency collection keeps the common list and adds only its own.
func TestDependencyDirSet(t *testing.T) {
	got := append([]string(nil), Dependencies(nil).SkipDirs...)
	sort.Strings(got)

	want := []string{"__pycache__", "build", "dist", "node_modules", "target", "vendor"}
	if len(got) != len(want) {
		t.Fatalf("dependency dirs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("dependency dirs = %v, want %v", got, want)
		}
	}

	// Everything except the directory list is shared with scanning: the one
	// difference that would matter (manifests behind skipped extensions) is
	// handled by KeepManifests, not by a separate list.
	// Only the directory list may differ: the dependency profile overrides
	// SkipDirs and nothing else, so every other list stays shared.
	dep := Dependencies(nil)
	if dep.SkipFiles != nil || dep.SkipExts != nil {
		t.Error("only the directory list may differ between the two profiles")
	}
}

// The three lists must not overlap: an entry in two of them would make the
// split meaningless and the effective sets ambiguous.
func TestDirListsAreDisjoint(t *testing.T) {
	seen := map[string]string{}
	for name, list := range map[string][]string{
		"commonSkippedDirs":         commonSkippedDirs,
		"scanOnlySkippedDirs":       scanOnlySkippedDirs,
		"dependencyOnlySkippedDirs": dependencyOnlySkippedDirs,
	} {
		for _, d := range list {
			if prev, dup := seen[d]; dup {
				t.Errorf("%q appears in both %s and %s", d, prev, name)
			}
			seen[d] = name
		}
	}
}

// skippedDirs must not alias or append to the package-level lists.
func TestSkippedDirsDoesNotAliasPackageLists(t *testing.T) {
	before := append([]string(nil), commonSkippedDirs...)
	got := skippedDirs(scanOnlySkippedDirs)
	got[0] = "mutated"
	for i := range before {
		if commonSkippedDirs[i] != before[i] {
			t.Fatalf("commonSkippedDirs was mutated through the returned slice")
		}
	}
}

// .whl moves in from the fingerprint layer's own list, so removing that list
// does not start fingerprinting files it used to skip.
func TestWhlIsSkipped(t *testing.T) {
	c := build(defaultSource(stdDefaults()))
	if !c.Match("pkg-1.0.whl", aFile("pkg-1.0.whl", 2000)) {
		t.Error(".whl should be skipped")
	}
}

// Each layer's profile, asserted on the fields that define it. The point is not
// that the numbers match, but that the three deliberate differences of the
// dependency profile are pinned: its own directory list, manifests preserved,
// and .gitignore off.
func TestLayerProfiles(t *testing.T) {
	scan := Scanning(nil)
	if !scan.BuiltinFolderRules || !scan.BuiltinFileRules || !scan.GitIgnore || scan.KeepManifests {
		t.Errorf("SourceFiles = %+v", scan)
	}
	if scan.SkipDirs != nil {
		t.Error("SourceFiles must not override the directory list; it uses stdDefaults")
	}

	dep := Dependencies(nil)
	// The built-in file lists apply, the folder ones do not: the scanning directory
	// lists prune venv/ and examples/, where manifests legitimately live, so
	// Dependencies states its own directory rules instead.
	if !dep.BuiltinFileRules {
		t.Error("Dependencies must apply the built-in file lists")
	}
	if dep.BuiltinFolderRules {
		t.Error("Dependencies must not apply the built-in folder lists; it carries its own")
	}
	if dep.SkipDirExts == nil {
		t.Error("Dependencies must still prune directory suffixes (foo.egg-info)")
	}
	if dep.GitIgnore {
		t.Error("Dependencies must not apply .gitignore: it does not decide what is a dependency")
	}
	if !dep.KeepManifests {
		t.Error("Dependencies must preserve manifests; they live behind skipped extensions")
	}
	if dep.SkipDirs == nil {
		t.Fatal("Dependencies must carry its own directory list")
	}
	for _, d := range dep.SkipDirs {
		if d == "example" || d == "examples" {
			t.Errorf("dependency collection must look inside %q: a manifest there declares real dependencies", d)
		}
	}
}

// The dependency profile finds a manifest that scanning discards by extension,
// and still prunes what both operations agree on.
func TestDependenciesKeepManifests(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), 200)           // manifest behind .json
	writeFile(t, filepath.Join(root, "examples", "go.mod"), 200)     // manifest scanning would prune
	writeFile(t, filepath.Join(root, "node_modules", "p.json"), 200) // pruned by the shared list
	writeFile(t, filepath.Join(root, "dist", "package.json"), 200)   // pruned by the dependency list
	writeFile(t, filepath.Join(root, "notes.txt"), 200)              // not a manifest

	res, err := Collect(root, Dependencies(nil))
	if err != nil {
		t.Fatal(err)
	}
	got := baseNames(res.Files)
	if !equalStrings(got, []string{"go.mod", "package.json"}) {
		t.Fatalf("kept %v, want [go.mod package.json]", got)
	}
}

// The manifest set must not depend on which entry point asked for it. This is
// asserted by comparing the two profiles against each other rather than against
// a literal list: a literal would let both drift together, which is exactly how
// they came apart in the first place.
func TestDependencyProfileFindsManifestsAnywhere(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), 200)
	writeFile(t, filepath.Join(root, "examples", "go.mod"), 200)
	writeFile(t, filepath.Join(root, "examples", "main.go"), 200)

	res, err := Collect(root, Dependencies(nil))
	if err != nil {
		t.Fatal(err)
	}
	var found int
	for _, f := range res.Files {
		if filepath.Base(f) == "go.mod" {
			found++
		}
	}
	if found != 2 {
		t.Fatalf("found %d go.mod, want 2 — a manifest under examples/ declares real dependencies", found)
	}

	// Scanning, by contrast, still prunes examples/: its code is not the product.
	scanRes, err := Collect(root, Scanning(nil))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range scanRes.Files {
		if strings.Contains(filepath.ToSlash(f), "/examples/") {
			t.Errorf("scanning should not collect %s", f)
		}
	}
}
