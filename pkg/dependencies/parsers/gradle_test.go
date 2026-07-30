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

// TestParseBuildGradleSourceLocation verifies Line and DeclaredText for build.gradle.
func TestParseBuildGradleSourceLocation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    []LocalPurl
	}{
		{
			name: "compact single-line dep",
			// line 1: dependencies {
			// line 2:     implementation 'org.scala-lang:scala-library:2.11.12'
			// line 3: }
			content: `dependencies {
    implementation 'org.scala-lang:scala-library:2.11.12'
}
`,
			want: []LocalPurl{
				{
					Purl:         "pkg:maven/org.scala-lang/scala-library@2.11.12",
					Requirement:  "2.11.12",
					Scope:        "implementation",
					Line:         2,
					DeclaredText: "implementation 'org.scala-lang:scala-library:2.11.12'",
				},
			},
		},
		{
			name: "multi-line extended dep — Line is block-start line, DeclaredText is joined buffer",
			// line 1: dependencies {
			// line 2:     implementation(
			// line 3:         group: 'com.example',
			// line 4:         name: 'mylib',
			// line 5:         version: '1.0.0'
			// line 6:     )
			// line 7: }
			content: `dependencies {
    implementation(
        group: 'com.example',
        name: 'mylib',
        version: '1.0.0'
    )
}
`,
			want: []LocalPurl{
				{
					Purl:         "pkg:maven/com.example/mylib@1.0.0",
					Requirement:  "1.0.0",
					Scope:        "implementation",
					Line:         2,
					DeclaredText: "implementation( group: 'com.example', name: 'mylib', version: '1.0.0' )",
				},
			},
		},
		{
			name: "zero-value case — empty dependencies block",
			content: `dependencies {
}
`,
			want: []LocalPurl{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := ParseBuildGradle([]byte(tc.content), "build.gradle")
			if err != nil {
				t.Fatalf("ParseBuildGradle error: %v", err)
			}
			if len(result.Purls) != len(tc.want) {
				t.Fatalf("got %d purls, want %d; purls: %+v", len(result.Purls), len(tc.want), result.Purls)
			}
			for i, got := range result.Purls {
				w := tc.want[i]
				if got.Purl != w.Purl {
					t.Errorf("[%d] Purl: got %q, want %q", i, got.Purl, w.Purl)
				}
				if got.Requirement != w.Requirement {
					t.Errorf("[%d] Requirement: got %q, want %q", i, got.Requirement, w.Requirement)
				}
				if got.Line != w.Line {
					t.Errorf("[%d] Line: got %d, want %d", i, got.Line, w.Line)
				}
				// Checked exactly, but only when the case bothers to state one
				if w.DeclaredText != "" && got.DeclaredText != w.DeclaredText {
					t.Errorf("[%d] DeclaredText: got %q, want %q", i, got.DeclaredText, w.DeclaredText)
				}
			}
		})
	}
}
