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

package postprocess

import (
	scanossapi "github.com/scanoss/scanoss.api-sdk"
	"testing"

	"github.com/scanoss/scanoss.go/pkg/settings"
)

func TestStripVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"pkg:npm/lodash@4.17.21", "pkg:npm/lodash"},
		{"pkg:npm/lodash", "pkg:npm/lodash"},
		{"pkg:maven/org.apache.commons/commons-lang3@3.12.0", "pkg:maven/org.apache.commons/commons-lang3"},
		{"pkg:npm/@scope/package@1.0.0", "pkg:npm/@scope/package"},
		{"pkg:golang/github.com/spf13/cobra@1.0.0", "pkg:golang/github.com/spf13/cobra"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := stripVersion(tt.input); got != tt.expected {
				t.Errorf("stripVersion(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestMatchPath(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		path    string
		want    bool
	}{
		{"recursive match direct child", "src/", "src/file.js", true},
		{"recursive match nested", "src/", "src/components/file.js", true},
		{"recursive match deep nested", "src/", "src/a/b/c/file.js", true},
		{"recursive no match different dir", "src/", "lib/file.js", false},
		{"recursive no match partial prefix", "src/", "srcfile.js", false},
		{"single-level match", "src/*", "src/file.js", true},
		{"single-level no match nested", "src/*", "src/components/file.js", false},
		{"single-level no match different dir", "lib/*", "src/file.js", false},
		{"glob match exact", "src/file.js", "src/file.js", true},
		{"glob match wildcard ext", "src/*.js", "src/file.js", true},
		{"glob no match wrong ext", "src/*.ts", "src/file.js", false},
		{"empty pattern", "", "src/file.js", false},
		{"exact dir match recursive", "src/", "src", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchPath(tt.pattern, tt.path); got != tt.want {
				t.Errorf("matchPath(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}

// fileByPath returns the file result at the given path (the slice is small).
func fileByPath(result *scanossapi.ScanResult, path string) scanossapi.FileResult {
	for _, f := range result.Files {
		if f.Path == path {
			return f
		}
	}
	return scanossapi.FileResult{}
}

func TestApplyRemove(t *testing.T) {
	result := &scanossapi.ScanResult{
		Files: []scanossapi.FileResult{
			{Path: "/keep.c", FileHash: "a", MatchType: "file", Matches: []scanossapi.MatchResult{{UrlHash: "h1"}}},
			{Path: "/drop.c", FileHash: "b", MatchType: "snippet", Matches: []scanossapi.MatchResult{{UrlHash: "h2", MatchPercentage: 80}}},
		},
		Components: map[string]scanossapi.ComponentResult{
			"h1": {Purls: []string{"pkg:github/keep/x"}},
			"h2": {Purls: []string{"pkg:github/drop/y"}},
		},
	}

	bom := &settings.BOM{Remove: []settings.BOMEntry{{Purl: "pkg:github/drop/y"}}}

	applyRemove(result, bom)

	// /keep.c untouched.
	if fileByPath(result, "/keep.c").MatchType != "file" {
		t.Errorf("/keep.c match_type = %q, want file", fileByPath(result, "/keep.c").MatchType)
	}
	// /drop.c neutralized, file hash preserved, matches cleared.
	drop := fileByPath(result, "/drop.c")
	if drop.MatchType != "none" || drop.FileHash != "b" {
		t.Errorf("/drop.c not neutralized: %+v", drop)
	}
	if len(drop.Matches) != 0 {
		t.Errorf("/drop.c matches should be cleared: %+v", drop)
	}
	// Orphaned component h2 pruned, h1 kept.
	if len(result.Components) != 1 {
		t.Errorf("components not pruned correctly: %+v", result.Components)
	}
	if _, ok := result.Components["h1"]; !ok {
		t.Errorf("h1 should be kept: %+v", result.Components)
	}
}

func TestApplyRemove_AnyMatchNeutralizes(t *testing.T) {
	// /multi.c matches two components; one matches a remove rule, so the WHOLE
	// file is neutralized. h1 is also referenced by /other.c, so it survives the
	// prune; h2 (only referenced by the neutralized file) is pruned.
	result := &scanossapi.ScanResult{
		Files: []scanossapi.FileResult{
			{Path: "/multi.c", FileHash: "m", MatchType: "file", Matches: []scanossapi.MatchResult{{UrlHash: "h1"}, {UrlHash: "h2"}}},
			{Path: "/other.c", FileHash: "o", MatchType: "file", Matches: []scanossapi.MatchResult{{UrlHash: "h1"}}},
		},
		Components: map[string]scanossapi.ComponentResult{
			"h1": {Purls: []string{"pkg:a/keep"}},
			"h2": {Purls: []string{"pkg:b/drop"}},
		},
	}
	bom := &settings.BOM{Remove: []settings.BOMEntry{{Purl: "pkg:b/drop"}}}

	applyRemove(result, bom)

	multi := fileByPath(result, "/multi.c")
	if multi.MatchType != "none" || len(multi.Matches) != 0 {
		t.Errorf("/multi.c should be fully neutralized: %+v", multi)
	}
	if fileByPath(result, "/other.c").MatchType != "file" {
		t.Errorf("/other.c should be untouched: %+v", fileByPath(result, "/other.c"))
	}
	if len(result.Components) != 1 {
		t.Errorf("expected only h1 kept: %+v", result.Components)
	}
	if _, ok := result.Components["h1"]; !ok {
		t.Errorf("h1 should be kept: %+v", result.Components)
	}
}

func TestApplyRemove_IncludeProtects(t *testing.T) {
	result := &scanossapi.ScanResult{
		Files: []scanossapi.FileResult{
			{Path: "/f.c", FileHash: "a", MatchType: "file", Matches: []scanossapi.MatchResult{{UrlHash: "h1"}}},
		},
		Components: map[string]scanossapi.ComponentResult{
			"h1": {Purls: []string{"pkg:github/x/y"}},
		},
	}
	bom := &settings.BOM{
		Remove:  []settings.BOMEntry{{Purl: "pkg:github/x/y"}},
		Include: []settings.BOMEntry{{Purl: "pkg:github/x/y"}},
	}
	applyRemove(result, bom)
	// include protects → match stays.
	if fileByPath(result, "/f.c").MatchType != "file" {
		t.Errorf("include should protect from removal; got %+v", fileByPath(result, "/f.c"))
	}
}

func TestApplyRemove_MultiMatchKept(t *testing.T) {
	// A file matching two components, neither matching a remove rule, is left
	// intact (both matches preserved) and both components survive the prune.
	result := &scanossapi.ScanResult{
		Files: []scanossapi.FileResult{
			{Path: "/multi.c", FileHash: "m", MatchType: "snippet", Matches: []scanossapi.MatchResult{{UrlHash: "h1"}, {UrlHash: "h2"}}},
		},
		Components: map[string]scanossapi.ComponentResult{
			"h1": {Purls: []string{"pkg:a/keep"}},
			"h2": {Purls: []string{"pkg:b/keep"}},
		},
	}
	bom := &settings.BOM{Remove: []settings.BOMEntry{{Purl: "pkg:c/other"}}}

	applyRemove(result, bom)

	multi := fileByPath(result, "/multi.c")
	if multi.MatchType != "snippet" || len(multi.Matches) != 2 {
		t.Errorf("/multi.c should be untouched with both matches: %+v", multi)
	}
	if len(result.Components) != 2 {
		t.Errorf("both referenced components should survive: %+v", result.Components)
	}
}
