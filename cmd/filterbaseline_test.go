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

package cmd

// Characterisation tests for file collection: they record what each operation
// collects, so that a change to the shared filter rules cannot alter it unnoticed.
// Every expectation below is a literal file list, on purpose: a test that
// recomputed the answer from the same lists the code uses would agree with any
// bug those lists contain.
//

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/scanoss/scanoss.go/pkg/dependencies"
	"github.com/scanoss/scanoss.go/pkg/filter"
)

// baselineTree builds a fixture that exercises every axis the filters touch:
// directories the two operations disagree on (venv, examples, dist), manifests
// hidden behind skipped extensions (.json, .mod, .xml), a file skipped only by
// extension (a.png), one skipped by name (Makefile), one by ending (README), a
// .gitignore and a scanoss.json.
func baselineTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	files := map[string]string{
		"main.go":           "package main\nfunc main() {}\n",
		"a.png":             "not really a png but long enough to matter\n",
		"README":            "readme body\n",
		"Makefile":          "all:\n\techo hi\n",
		"go.mod":            "module example.com/x\n\ngo 1.25\n",
		"pom.xml":           "<project></project>\n",
		"Gemfile":           "source 'https://rubygems.org'\n",
		"venv/x.go":         "package venv\n",
		"examples/go.mod":   "module example.com/x/examples\n",
		"examples/main.go":  "package main\n",
		"dist/package.json": `{"name":"generated"}` + "\n",
		"node_modules/y.js": "module.exports = {}\n",
		"ignored/gen.go":    "package ignored\n",
		".gitignore":        "ignored/\n",
		"scanoss.json":      `{"settings":{"skip":{"patterns":{"scanning":[]}}}}` + "\n",
	}
	for rel, body := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// relNames returns the paths relative to root, slash-separated and sorted.
func relNames(t *testing.T, root string, paths []string) []string {
	t.Helper()
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		rel, err := filepath.Rel(root, p)
		if err != nil {
			rel = p
		}
		out = append(out, filepath.ToSlash(rel))
	}
	sort.Strings(out)
	return out
}

func assertSet(t *testing.T, what string, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") == strings.Join(want, "\n") {
		return
	}
	t.Errorf("%s changed.\n got: %v\nwant: %v\n\nIf the change is intended, update the expectation and say so in the commit; otherwise a filter rule altered behaviour.",
		what, got, want)
}

// What `scan` and `wfp` collect today.
func TestBaselineScanCollection(t *testing.T) {
	root := baselineTree(t)
	st, err := resolveSettings("", root)
	if err != nil {
		t.Fatal(err)
	}

	res, err := filter.Collect(root, filter.Scanning(st))
	if err != nil {
		t.Fatal(err)
	}

	// go.mod, pom.xml, dist/package.json and scanoss.json are dropped by
	// extension; README by ending; Makefile by name; a.png by extension;
	// venv/, examples/ and node_modules/ by directory; ignored/ by .gitignore.
	assertSet(t, "scan collection", relNames(t, root, res.Files), []string{
		"Gemfile",
		"main.go",
	})
}

// The same, with the built-in lists off. Note a.png is absent even though
// nothing in the collection excludes it — pkg/fingerprint/wfp drops it later,
// so turning the lists off does not reach it.
func TestBaselineScanCollectionNoDefaults(t *testing.T) {
	root := baselineTree(t)
	res, err := filter.Collect(root, filter.Options{
		BuiltinFolderRules: false, BuiltinFileRules: false,
		GitIgnore: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := relNames(t, root, res.Files)
	assertSet(t, "scan collection (--default-filters=false)", got, []string{
		"Gemfile",
		"Makefile",
		"README",
		"a.png",
		"dist/package.json",
		"examples/go.mod",
		"examples/main.go",
		"go.mod",
		"main.go",
		"node_modules/y.js",
		"pom.xml",
		"scanoss.json",
		"venv/x.go",
	})
}

// What `dependencies` collects, through the shared filter.
//
// The candidate list is smaller than the tree because the default extension, name
// and ending lists apply: Makefile, README, a.png and scanoss.json are absent.
// None of them is a manifest, so the parser's output is unaffected; only the
// candidate set it is handed is narrower.
//
// The part that matters is that every manifest is here,
// including the ones behind skipped extensions (go.mod, pom.xml) and the one
// under examples/, which dependency collection deliberately does not prune.
// ignored/gen.go is still present too: .gitignore does not decide what is a
// dependency.
func TestBaselineDependencyCollection(t *testing.T) {
	root := baselineTree(t)
	files, _, err := collectDependencyFiles(root, "")
	if err != nil {
		t.Fatal(err)
	}
	assertSet(t, "dependency collection", relNames(t, root, files), []string{
		"Gemfile",
		"examples/go.mod",
		"examples/main.go",
		"go.mod",
		"ignored/gen.go",
		"main.go",
		"pom.xml",
		"venv/x.go",
	})
}

// The parser's output is what users see. Asserting on the manifests rather than
// on the candidate list is what proves the shared filter narrowed the route
// without changing the result.
func TestDependencyManifestsUnchanged(t *testing.T) {
	root := baselineTree(t)
	files, _, err := collectDependencyFiles(root, "")
	if err != nil {
		t.Fatal(err)
	}
	got := relNames(t, root, dependencies.NewDependencyParser().FilterFiles(files))
	assertSet(t, "parsed manifests", got, []string{
		"Gemfile",
		"examples/go.mod",
		"go.mod",
		"pom.xml",
	})
}

// scanoss.json's dependencies section takes effect: it is part of the published
// schema, so a pattern listed there must prune dependency collection too.
func TestDependencySkipPatternsHonoured(t *testing.T) {
	root := baselineTree(t)
	cfg := `{"settings":{"skip":{"patterns":{"dependencies":["examples/**"]}}}}`
	if err := os.WriteFile(filepath.Join(root, "scanoss.json"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	files, _, err := collectDependencyFiles(root, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range relNames(t, root, files) {
		if strings.HasPrefix(f, "examples/") {
			t.Errorf("skip.patterns.dependencies should have excluded %s", f)
		}
	}
}
