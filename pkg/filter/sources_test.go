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
	"strings"
	"testing"
)

func keySet(ms []Matcher) map[string]bool {
	s := make(map[string]bool, len(ms))
	for _, m := range ms {
		s[m.Key()] = true
	}
	return s
}

func TestDefaultSource(t *testing.T) {
	d := StdDefaults()
	ms := DefaultSource(d)

	// Simple (single-dot) extensions fold into one map-backed matcher; compound
	// endings (e.g. ".min.js") stay as individual suffix matchers. Everything
	// else is one matcher per list entry, plus a single size matcher.
	var compound int
	for _, ext := range d.Exts {
		lower := strings.ToLower(ext)
		if filepath.Ext(lower) != lower {
			compound++
		}
	}
	want := len(d.Dirs) + len(d.DirExts) + len(d.Files) + len(d.Endings) + compound + 1 /*extset*/ + 1 /*size*/
	if len(ms) != want {
		t.Fatalf("DefaultSource count = %d, want %d", len(ms), want)
	}

	keys := keySet(ms)
	for _, k := range []string{
		"dir:vendor", "dir:node_modules", "dirsuffix:.egg-info",
		"name:makefile", "ending:readme", "size:100:0",
	} {
		if !keys[k] {
			t.Errorf("DefaultSource missing matcher %q", k)
		}
	}

	// Simple extensions are matched via the set; the compound ".min.js" via its
	// own suffix matcher. Both must still be skipped.
	c := Build(ms)
	if !c.Match("a.png", aFile("a.png", 200)) {
		t.Error("a.png should be skipped (extset)")
	}
	if !c.Match("app.min.js", aFile("app.min.js", 200)) {
		t.Error("app.min.js should be skipped (compound suffix)")
	}
}

func TestDefaultSourceNoSizeWhenUnset(t *testing.T) {
	ms := DefaultSource(Defaults{Dirs: []string{"build"}})
	if len(ms) != 1 {
		t.Fatalf("count = %d, want 1", len(ms))
	}
	if ms[0].Key() != "dir:build" {
		t.Fatalf("key = %q, want dir:build", ms[0].Key())
	}
}

func TestDefaultSourceCustomSize(t *testing.T) {
	ms := DefaultSource(Defaults{MaxSize: 1000})
	if len(ms) != 1 || ms[0].Key() != "size:0:1000" {
		t.Fatalf("matchers = %v, want one size:0:1000", keySet(ms))
	}
	if !ms[0].Match("big", aFile("big", 2000)) {
		t.Error("file above max should be skipped")
	}
	if ms[0].Match("ok", aFile("ok", 500)) {
		t.Error("file within max should not be skipped")
	}
}

func TestSettingsSourceNil(t *testing.T) {
	if ms := SettingsSource(nil); ms != nil {
		t.Fatalf("SettingsSource(nil) = %v, want nil", ms)
	}
}

func TestSettingsSourcePatterns(t *testing.T) {
	ms := SettingsSource(&Settings{Skip: Skip{Patterns: []string{"*.log", "build/**"}}})
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
	ms := SettingsSource(&Settings{Skip: Skip{
		Sizes: []SizeRule{{Patterns: []string{"*.bin"}, Max: 100}},
	}})
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
	ms := SettingsSource(&Settings{Skip: Skip{
		Sizes: []SizeRule{{Patterns: nil, Max: 100}},
	}})
	if len(ms) != 0 {
		t.Fatalf("count = %d, want 0 (size rule without patterns is ignored)", len(ms))
	}
}

func TestSettingsSourcePatternsAndSizes(t *testing.T) {
	ms := SettingsSource(&Settings{Skip: Skip{
		Patterns: []string{"*.log"},
		Sizes:    []SizeRule{{Patterns: []string{"*.bin"}, Max: 100}},
	}})
	if len(ms) != 2 {
		t.Fatalf("count = %d, want 2", len(ms))
	}
}

func TestGitIgnoreSourceMissing(t *testing.T) {
	ms, err := GitIgnoreSource(t.TempDir())
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
	ms, err := GitIgnoreSource(root)
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
