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

// Characterisation tests for the filter unification (see specs/unify-filters).
//
// These record what each operation collects TODAY, before any rule moves. They
// are not a statement that the current behaviour is right — only that the
// refactor must not change it. Every expectation below is a literal file list,
// on purpose: a test that recomputed the answer from the same lists the code
// uses would agree with any bug those lists contain.
//
// If one of these fails during the refactor, the refactor changed behaviour.
// Fix the code, not the expectation — unless the SDD says that task is the one
// that deliberately changes it.

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/scanoss/scanoss.go/pkg/dependencies"
	"github.com/scanoss/scanoss.go/pkg/filter"
	"github.com/scanoss/scanoss.go/pkg/manifests"
	"github.com/scanoss/scanoss.go/pkg/scanner"
	"github.com/scanoss/scanoss.go/pkg/settings"
)

// baselineTree builds a fixture that exercises every axis the refactor touches:
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
	t.Errorf("%s changed.\n got: %v\nwant: %v\n\nIf this is the task that is meant to change it, update the expectation and say so in the commit; otherwise the refactor altered behaviour.",
		what, got, want)
}

// What `scan` and `wfp` collect today.
func TestBaselineScanCollection(t *testing.T) {
	root := baselineTree(t)
	st, err := settings.Resolve("", root)
	if err != nil {
		t.Fatal(err)
	}

	res, err := scanner.CollectFilesWithOptions(root, filter.Options{
		Defaults:  true,
		GitIgnore: true,
		Settings:  st.ScanFilter(),
	})
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
// nothing in the collection excludes it — pkg/fingerprint/wfp drops it later.
// That is the bug the refactor fixes; recorded here so the fix is visible.
func TestBaselineScanCollectionNoDefaults(t *testing.T) {
	root := baselineTree(t)
	res, err := scanner.CollectFilesWithOptions(root, filter.Options{
		Defaults:  false,
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

// What `dependencies` collects, now through the shared filter.
//
// This expectation was updated by T007 — the one task allowed to change it.
// The candidate list is smaller because the default extension, name and ending
// lists now apply: Makefile, README, a.png and scanoss.json are gone. None of
// them was ever a manifest, so the parser's output is unchanged; only the
// candidate set it is handed shrank.
//
// What did NOT change is the part that matters: every manifest is still here,
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

// The parser's output is what users see, and it must be identical to what the
// old walk produced. Asserting on the manifests rather than on the candidate
// list is what proves T007 changed the route and not the result.
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

// scanoss.json's dependencies section now takes effect. It is part of the
// published schema and did nothing before.
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

// What the extraction pre-filter keeps today — the shape an external SDK
// consumer relies on when it filters archive entries before writing them out.
func TestBaselineExtractionMatcher(t *testing.T) {
	root := baselineTree(t)
	// Composed the way an external consumer composes it, from the exported
	// pieces: the default sources, plus manifests.Is for the exemption that
	// Options.PreserveDependencyManifests applies inside Collect. scanoss.go no
	// longer ships a ready-made matcher for callers that do not walk a tree.
	base := filter.Build(filter.DefaultSource(filter.StdDefaults()))
	skip := func(rel string, info os.FileInfo) bool {
		return base.Match(rel, info) && !manifests.Is(rel)
	}

	var kept []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || strings.HasPrefix(info.Name(), ".") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if !skip(filepath.ToSlash(rel), info) {
			kept = append(kept, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Manifests survive the extension list thanks to the exemption. Note that
	// node_modules/, venv/ and examples/ entries are kept: this matcher decides
	// about the entry, not the path, so pruning directories is the caller's job
	// (a consumer does it with its own traversal), using the exported lists.
	assertSet(t, "extraction matcher", relNames(t, root, kept), []string{
		"Gemfile",
		"dist/package.json",
		"examples/go.mod",
		"examples/main.go",
		"go.mod",
		"ignored/gen.go",
		"main.go",
		"node_modules/y.js",
		"pom.xml",
		"venv/x.go",
	})
}
