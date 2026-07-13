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

package parsers

import (
	"testing"
)

// TestParseRequirementsTxtSourceLocation verifies Line and DeclaredText for requirements.txt.
func TestParseRequirementsTxtSourceLocation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    []LocalPurl
	}{
		{
			name: "blank lines above first dep",
			// line 1: # comment
			// line 2: (blank)
			// line 3: requests==2.28.0
			// line 4: flask==2.3.0
			content: `# comment

requests==2.28.0
flask==2.3.0
`,
			want: []LocalPurl{
				{
					Purl:         "pkg:pypi/requests@2.28.0",
					Requirement:  "2.28.0",
					Line:         3,
					DeclaredText: "requests==2.28.0",
				},
				{
					Purl:         "pkg:pypi/flask@2.3.0",
					Requirement:  "2.3.0",
					Line:         4,
					DeclaredText: "flask==2.3.0",
				},
			},
		},
		{
			name: "unmappable edge case — no match gives no entry",
			// Only a comment and blank lines; no deps should be emitted.
			content: `# just a comment
`,
			want: []LocalPurl{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := ParseRequirementsTxt([]byte(tc.content), "requirements.txt")
			if err != nil {
				t.Fatalf("ParseRequirementsTxt error: %v", err)
			}
			if len(result.Purls) != len(tc.want) {
				t.Fatalf("got %d purls, want %d; purls: %+v", len(result.Purls), len(tc.want), result.Purls)
			}
			for i, got := range result.Purls {
				w := tc.want[i]
				if got.Purl != w.Purl {
					t.Errorf("[%d] Purl: got %q, want %q", i, got.Purl, w.Purl)
				}
				if got.Line != w.Line {
					t.Errorf("[%d] Line: got %d, want %d", i, got.Line, w.Line)
				}
				if got.DeclaredText != w.DeclaredText {
					t.Errorf("[%d] DeclaredText: got %q, want %q", i, got.DeclaredText, w.DeclaredText)
				}
			}
		})
	}
}

// TestParsePyprojectTomlSourceLocation verifies Line and DeclaredText for pyproject.toml.
// For pyproject.toml, multiple quoted deps on a single line share the same Line and DeclaredText.
func TestParsePyprojectTomlSourceLocation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		content   string
		wantCount int
		wantLine  int
		wantLines []int // per-purl exact lines; checked when len > 0
		wantPurls []string
	}{
		{
			name: "single dep per line under dependencies section (PEP 508 style)",
			// line 1: [tool.poetry]
			// line 2: name = "myapp"
			// line 3: (blank)
			// line 4: [tool.poetry.dependencies]
			// line 5: dep1 = "requests>=2.28.0"
			// line 6: dep2 = "flask>=2.3.0"
			content: `[tool.poetry]
name = "myapp"

[tool.poetry.dependencies]
dep1 = "requests>=2.28.0"
dep2 = "flask>=2.3.0"
`,
			wantCount: 2,
			wantLines: []int{5, 6},
		},
		{
			name: "multiple quoted deps on single line share same Line",
			// line 1: [build-system]
			// line 2: requires = ["setuptools>=40.8.0", "wheel"]
			// line 3: (blank)
			// line 4: [tool.poetry.dependencies]
			// line 5: extras = ["requests>=2.28.0", "flask>=2.3.0"]
			content: `[build-system]
requires = ["setuptools>=40.8.0", "wheel"]

[tool.poetry.dependencies]
extras = ["requests>=2.28.0", "flask>=2.3.0"]
`,
			wantCount: 2,
			wantLine:  5,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := ParsePyprojectToml([]byte(tc.content), "pyproject.toml")
			if err != nil {
				t.Fatalf("ParsePyprojectToml error: %v", err)
			}
			if len(result.Purls) != tc.wantCount {
				t.Fatalf("got %d purls, want %d; purls: %+v", len(result.Purls), tc.wantCount, result.Purls)
			}
			// All purls must have non-zero Line and non-empty DeclaredText
			for i, p := range result.Purls {
				if p.Line == 0 {
					t.Errorf("[%d] Line is 0, want non-zero", i)
				}
				if p.DeclaredText == "" {
					t.Errorf("[%d] DeclaredText is empty", i)
				}
				if tc.wantLine != 0 && p.Line != tc.wantLine {
					t.Errorf("[%d] Line: got %d, want %d", i, p.Line, tc.wantLine)
				}
				if i < len(tc.wantLines) && p.Line != tc.wantLines[i] {
					t.Errorf("[%d] Line: got %d, want %d", i, p.Line, tc.wantLines[i])
				}
			}
			// For multi-dep line case: all deps on same line share same DeclaredText
			if tc.wantLine != 0 && len(result.Purls) == 2 {
				if result.Purls[0].DeclaredText != result.Purls[1].DeclaredText {
					t.Errorf("multi-dep: expected same DeclaredText for both; got %q vs %q",
						result.Purls[0].DeclaredText, result.Purls[1].DeclaredText)
				}
			}
		})
	}
}
