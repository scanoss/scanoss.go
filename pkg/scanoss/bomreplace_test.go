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

package scanoss

import (
	"testing"

	scanossapi "github.com/scanoss/scanoss.api-sdk"

	"github.com/scanoss/scanoss.go/pkg/settings"
)

// replaceFixture is one matched file against a one-component catalog.
func replaceFixture() *scanossapi.ScanResult {
	return &scanossapi.ScanResult{
		Files: []scanossapi.FileResult{{
			Path:       "vendor/lib.c",
			MatchType:  "file",
			SourceHash: "src-hash",
			FileHash:   "file-hash",
			Matches:    []scanossapi.MatchResult{{UrlHash: "h1", OssFilePath: "lib.c"}},
		}},
		Components: map[string]scanossapi.ComponentResult{
			"h1": {Purls: []string{"pkg:github/wrong/lib"}, Component: "lib", Vendor: "wrong"},
		},
	}
}

// purlOf returns the PURLs the file's matches now resolve to.
func purlOf(t *testing.T, res *scanossapi.ScanResult, file int) []string {
	t.Helper()
	return filePurls(&res.Files[file], res.Components)
}

func TestReplaceRepointsMatchToTheNamedComponent(t *testing.T) {
	res := replaceFixture()
	ApplyBOMReplace(res, &settings.BOM{Replace: []settings.BOMEntry{
		{Purl: "pkg:github/wrong/lib", ReplaceWith: "pkg:github/right/lib@2.1.0"},
	}})

	got := purlOf(t, res, 0)
	if len(got) != 1 || got[0] != "pkg:github/right/lib" {
		t.Fatalf("file should now report the replacement component, got %v", got)
	}

	comp := res.Components[res.Files[0].Matches[0].UrlHash]
	if comp.Version != "2.1.0" {
		t.Errorf("version from the replacement PURL: got %q, want 2.1.0", comp.Version)
	}
	if comp.Component != "lib" || comp.Vendor != "right" {
		t.Errorf("name and vendor come from the PURL: got %q / %q, want lib / right", comp.Component, comp.Vendor)
	}
}

// Replacing states which component the file is, not what the scan saw in it. Anything the scan
// observed has to survive, or the replacement would be laundering evidence.
func TestReplaceKeepsWhatTheScanObserved(t *testing.T) {
	res := replaceFixture()
	ApplyBOMReplace(res, &settings.BOM{Replace: []settings.BOMEntry{
		{Purl: "pkg:github/wrong/lib", ReplaceWith: "pkg:github/right/lib"},
	}})

	f := res.Files[0]
	if f.Path != "vendor/lib.c" || f.MatchType != "file" {
		t.Errorf("path and match type must survive: got %q / %q", f.Path, f.MatchType)
	}
	if f.SourceHash != "src-hash" || f.FileHash != "file-hash" {
		t.Errorf("hashes must survive: got %q / %q", f.SourceHash, f.FileHash)
	}
	if len(f.Matches) != 1 || f.Matches[0].OssFilePath != "lib.c" {
		t.Errorf("match evidence must survive: got %+v", f.Matches)
	}
}

// Two rules can cover one file and name different replacements, so the more specific one has to
// win — otherwise the answer depends on the order the settings file happens to be written in.
func TestReplacePicksTheMostSpecificRule(t *testing.T) {
	rules := []settings.BOMEntry{
		{Purl: "pkg:github/wrong/lib", ReplaceWith: "pkg:github/by-purl/lib"},                  // score 2
		{Path: "vendor/", ReplaceWith: "pkg:github/by-path/lib"},                               // score 1
		{Purl: "pkg:github/wrong/lib", Path: "vendor/", ReplaceWith: "pkg:github/by-both/lib"}, // score 4
	}

	// Same rules, reversed: the outcome must not depend on the order they are listed in.
	for _, order := range [][]settings.BOMEntry{rules, {rules[2], rules[1], rules[0]}} {
		res := replaceFixture()
		ApplyBOMReplace(res, &settings.BOM{Replace: order})
		if got := purlOf(t, res, 0); len(got) != 1 || got[0] != "pkg:github/by-both/lib" {
			t.Errorf("the purl+path rule must win, got %v", got)
		}
	}
}

// Between rules of equal specificity the narrower path is the one that meant this file.
func TestReplacePrefersTheLongerPath(t *testing.T) {
	res := replaceFixture()
	ApplyBOMReplace(res, &settings.BOM{Replace: []settings.BOMEntry{
		{Path: "vendor/", ReplaceWith: "pkg:github/broad/lib"},
		{Path: "vendor/lib.c", ReplaceWith: "pkg:github/narrow/lib"},
	}})
	if got := purlOf(t, res, 0); len(got) != 1 || got[0] != "pkg:github/narrow/lib" {
		t.Errorf("the narrower path must win, got %v", got)
	}
}

// A replacement naming a component the scan already found joins that entry rather than creating a
// second one for the same thing.
func TestReplaceReusesAnExistingComponent(t *testing.T) {
	res := replaceFixture()
	res.Files = append(res.Files, scanossapi.FileResult{
		Path: "src/other.c", MatchType: "file",
		Matches: []scanossapi.MatchResult{{UrlHash: "h2"}},
	})
	res.Components["h2"] = scanossapi.ComponentResult{
		Purls: []string{"pkg:github/right/lib"}, Component: "lib", Vendor: "right", Url: "https://example.test/lib",
	}

	ApplyBOMReplace(res, &settings.BOM{Replace: []settings.BOMEntry{
		{Purl: "pkg:github/wrong/lib", ReplaceWith: "pkg:github/right/lib"},
	}})

	if hash := res.Files[0].Matches[0].UrlHash; hash != "h2" {
		t.Fatalf("should reuse the catalog entry h2, got %q", hash)
	}
	if url := res.Components["h2"].Url; url != "https://example.test/lib" {
		t.Errorf("reusing the entry keeps what the KB knew about it, got url %q", url)
	}
	if len(res.Components) != 1 {
		t.Errorf("the replaced-away component should be pruned, catalog is %v", res.Components)
	}
}

// A file whose match remove already dismissed is not a candidate: replace must not resurrect it.
func TestReplaceSkipsRemovedMatches(t *testing.T) {
	res := replaceFixture()
	bom := &settings.BOM{
		Remove:  []settings.BOMEntry{{Purl: "pkg:github/wrong/lib"}},
		Replace: []settings.BOMEntry{{Path: "vendor/", ReplaceWith: "pkg:github/right/lib"}},
	}
	ApplyBOMRemove(res, bom)
	ApplyBOMReplace(res, bom)

	if mt := res.Files[0].MatchType; mt != "none" {
		t.Errorf("the file stays dismissed, got match type %q", mt)
	}
	if len(res.Components) != 0 {
		t.Errorf("no component should be reported, got %v", res.Components)
	}
}

func TestReplaceNoOps(t *testing.T) {
	cases := map[string]*settings.BOM{
		"nil bom":            nil,
		"no replace rules":   {Remove: []settings.BOMEntry{{Purl: "pkg:github/wrong/lib"}}},
		"rule with no purl":  {Replace: []settings.BOMEntry{{ReplaceWith: "pkg:github/right/lib"}}},
		"rule not replacing": {Replace: []settings.BOMEntry{{Purl: "pkg:github/wrong/lib"}}},
		"purl does not match": {Replace: []settings.BOMEntry{
			{Purl: "pkg:github/other/thing", ReplaceWith: "pkg:github/right/lib"},
		}},
		"path does not match": {Replace: []settings.BOMEntry{
			{Path: "src/", ReplaceWith: "pkg:github/right/lib"},
		}},
	}
	for name, bom := range cases {
		t.Run(name, func(t *testing.T) {
			res := replaceFixture()
			ApplyBOMReplace(res, bom)
			if got := purlOf(t, res, 0); len(got) != 1 || got[0] != "pkg:github/wrong/lib" {
				t.Errorf("result should be untouched, got %v", got)
			}
		})
	}

	ApplyBOMReplace(nil, &settings.BOM{Replace: []settings.BOMEntry{{Purl: "x", ReplaceWith: "y"}}})
}

// A rule written against one release still covers the component, and a scoped npm namespace is
// not mistaken for a version — reading "@babel" as one would leave "pkg:npm/" and match every
// npm package in the result.
func TestReplaceVersionHandling(t *testing.T) {
	t.Run("rule with a version covers the component", func(t *testing.T) {
		res := replaceFixture()
		ApplyBOMReplace(res, &settings.BOM{Replace: []settings.BOMEntry{
			{Purl: "pkg:github/wrong/lib@1.0.0", ReplaceWith: "pkg:github/right/lib"},
		}})
		if got := purlOf(t, res, 0); len(got) != 1 || got[0] != "pkg:github/right/lib" {
			t.Errorf("got %v", got)
		}
	})

	t.Run("scoped npm namespace is not a version", func(t *testing.T) {
		if bare, version := splitPurlVersion("pkg:npm/@babel/core"); bare != "pkg:npm/@babel/core" || version != "" {
			t.Errorf("got %q / %q, want the PURL unchanged and no version", bare, version)
		}
		if bare, version := splitPurlVersion("pkg:npm/@babel/core@7.0.0"); bare != "pkg:npm/@babel/core" || version != "7.0.0" {
			t.Errorf("got %q / %q, want pkg:npm/@babel/core / 7.0.0", bare, version)
		}
	})
}

func TestPurlNameAndVendor(t *testing.T) {
	cases := []struct{ purl, name, vendor string }{
		{"pkg:npm/lodash", "lodash", ""},
		{"pkg:github/scanoss/scanner.c", "scanner.c", "scanoss"},
		{"pkg:golang/github.com/spf13/cobra", "cobra", "github.com/spf13"},
		{"pkg:npm/@babel/core", "core", "@babel"},
		{"pkg:maven/org.springframework/spring-core", "spring-core", "org.springframework"},
	}
	for _, c := range cases {
		name, vendor := purlNameAndVendor(c.purl)
		if name != c.name || vendor != c.vendor {
			t.Errorf("%s: got %q / %q, want %q / %q", c.purl, name, vendor, c.name, c.vendor)
		}
	}
}
