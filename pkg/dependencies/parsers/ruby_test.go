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

// TestParseGemfileSourceLocation verifies Line and DeclaredText for Gemfile.
func TestParseGemfileSourceLocation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    []LocalPurl
	}{
		{
			name: "source line and blank lines before gem declarations",
			// line 1: source 'https://rubygems.org'
			// line 2: (blank)
			// line 3: # comment
			// line 4: (blank)
			// line 5: gem 'rails'
			// line 6: gem 'devise'
			content: `source 'https://rubygems.org'

# comment

gem 'rails'
gem 'devise'
`,
			want: []LocalPurl{
				{
					Purl:         "pkg:gem/rails",
					Line:         5,
					DeclaredText: "gem 'rails'",
				},
				{
					Purl:         "pkg:gem/devise",
					Line:         6,
					DeclaredText: "gem 'devise'",
				},
			},
		},
		{
			name: "zero-value case — no gem declarations",
			content: `source 'https://rubygems.org'
# just a comment
`,
			want: []LocalPurl{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := ParseGemfile([]byte(tc.content), "Gemfile")
			if err != nil {
				t.Fatalf("ParseGemfile error: %v", err)
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

// TestParseGemfileLockSourceLocation verifies Line and DeclaredText for Gemfile.lock.
func TestParseGemfileLockSourceLocation(t *testing.T) {
	t.Parallel()

	// line 1:  GEM
	// line 2:    remote: https://rubygems.org/
	// line 3:    specs:
	// line 4:      rails (7.0.0)
	// line 5:        actioncable (= 7.0.0)
	content := `GEM
  remote: https://rubygems.org/
  specs:
    rails (7.0.0)
`
	result, err := ParseGemfileLock([]byte(content), "Gemfile.lock")
	if err != nil {
		t.Fatalf("ParseGemfileLock error: %v", err)
	}
	if len(result.Purls) != 1 {
		t.Fatalf("got %d purls, want 1; purls: %+v", len(result.Purls), result.Purls)
	}
	if result.Purls[0].Line != 4 {
		t.Errorf("Line: got %d, want 4", result.Purls[0].Line)
	}
	wantText := "rails (7.0.0)"
	if result.Purls[0].DeclaredText != wantText {
		t.Errorf("DeclaredText: got %q, want %q", result.Purls[0].DeclaredText, wantText)
	}
}
