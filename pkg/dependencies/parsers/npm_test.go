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

// TestParseYarnLockSourceLocation verifies Line and DeclaredText for yarn.lock.
// Line should point at the pkg@range: entry line, not the version line.
func TestParseYarnLockSourceLocation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    []LocalPurl
	}{
		{
			name: "Line points at entry line not version line",
			// line 1: # yarn lockfile v1
			// line 2: (blank)
			// line 3: react@^18.0.0:
			// line 4:   version "18.2.0"
			content: `# yarn lockfile v1

react@^18.0.0:
  version "18.2.0"
`,
			want: []LocalPurl{
				{
					Purl:         "pkg:npm/react@18.2.0",
					Requirement:  "18.2.0",
					Line:         3,
					DeclaredText: "react@^18.0.0:",
				},
			},
		},
		{
			name: "multiple packages",
			// line 1: lodash@^4.0.0:
			// line 2:   version "4.17.21"
			// line 3: (blank)
			// line 4: express@^4.18.0:
			// line 5:   version "4.18.2"
			content: `lodash@^4.0.0:
  version "4.17.21"

express@^4.18.0:
  version "4.18.2"
`,
			want: []LocalPurl{
				{
					Purl:         "pkg:npm/lodash@4.17.21",
					Requirement:  "4.17.21",
					Line:         1,
					DeclaredText: "lodash@^4.0.0:",
				},
				{
					Purl:         "pkg:npm/express@4.18.2",
					Requirement:  "4.18.2",
					Line:         4,
					DeclaredText: "express@^4.18.0:",
				},
			},
		},
		{
			name: "zero-value case — only metadata, no packages",
			content: `# yarn lockfile v1

`,
			want: []LocalPurl{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := ParseYarnLock([]byte(tc.content), "yarn.lock")
			if err != nil {
				t.Fatalf("ParseYarnLock error: %v", err)
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
				if got.DeclaredText != w.DeclaredText {
					t.Errorf("[%d] DeclaredText: got %q, want %q", i, got.DeclaredText, w.DeclaredText)
				}
			}
		})
	}
}

// TestParsePackageJSONSourceLocation verifies Line and DeclaredText for package.json.
func TestParsePackageJSONSourceLocation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		wantMap map[string]int // purl -> expected line number
	}{
		{
			name: "first key, last key, scoped key in dependencies",
			content: `{
  "name": "my-app",
  "dependencies": {
    "react": "^18.0.0",
    "lodash": "^4.17.21",
    "@angular/core": "^15.0.0"
  }
}
`,
			wantMap: map[string]int{
				"pkg:npm/react@^18.0.0":         4,
				"pkg:npm/lodash@^4.17.21":       5,
				"pkg:npm/@angular/core@^15.0.0": 6,
			},
		},
		{
			name: "devDependencies key",
			content: `{
  "name": "my-app",
  "devDependencies": {
    "jest": "^29.0.0"
  }
}
`,
			wantMap: map[string]int{
				"pkg:npm/jest@^29.0.0": 4,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := ParsePackageJSON([]byte(tc.content), "package.json")
			if err != nil {
				t.Fatalf("ParsePackageJSON error: %v", err)
			}
			for _, p := range result.Purls {
				if wantLine, ok := tc.wantMap[p.Purl]; ok {
					if p.Line != wantLine {
						t.Errorf("Purl %q: Line got %d, want %d", p.Purl, p.Line, wantLine)
					}
					if p.DeclaredText == "" {
						t.Errorf("Purl %q: DeclaredText is empty", p.Purl)
					}
				}
			}
		})
	}
}

// TestParsePackageLockV1SourceLocation verifies Line and DeclaredText for package-lock.json v1.
func TestParsePackageLockV1SourceLocation(t *testing.T) {
	t.Parallel()

	// A minimal v1 package-lock.json — no "packages" key so it's detected as v1.
	// line 1: {
	// line 2:   "lockfileVersion": 1,
	// line 3:   "dependencies": {
	// line 4:     "react": {
	// line 5:       "version": "18.2.0"
	// line 6:     },
	// line 7:     "lodash": {
	// line 8:       "version": "4.17.21"
	// line 9:     }
	// line 10:  }
	// line 11: }
	content := `{
  "lockfileVersion": 1,
  "dependencies": {
    "react": {
      "version": "18.2.0"
    },
    "lodash": {
      "version": "4.17.21"
    }
  }
}
`
	result, err := ParsePackageLock([]byte(content), "package-lock.json")
	if err != nil {
		t.Fatalf("ParsePackageLock v1 error: %v", err)
	}
	if len(result.Purls) != 2 {
		t.Fatalf("got %d purls, want 2; purls: %+v", len(result.Purls), result.Purls)
	}

	// Build a lookup map — v1 dep ordering is deterministic (JSON object walk order).
	v1Map := make(map[string]LocalPurl)
	for _, p := range result.Purls {
		v1Map[p.Purl] = p
	}

	// react: declared at line 4 ("react": {)
	reactV1 := v1Map["pkg:npm/react@18.2.0"]
	if reactV1.Line != 4 {
		t.Errorf("v1 react Line: got %d, want 4", reactV1.Line)
	}
	if reactV1.DeclaredText != `"react": {` {
		t.Errorf("v1 react DeclaredText: got %q, want %q", reactV1.DeclaredText, `"react": {`)
	}

	// lodash: declared at line 7 ("lodash": {)
	lodashV1 := v1Map["pkg:npm/lodash@4.17.21"]
	if lodashV1.Line != 7 {
		t.Errorf("v1 lodash Line: got %d, want 7", lodashV1.Line)
	}
	if lodashV1.DeclaredText != `"lodash": {` {
		t.Errorf("v1 lodash DeclaredText: got %q, want %q", lodashV1.DeclaredText, `"lodash": {`)
	}
}

// TestParsePackageLockV2SourceLocation verifies Line and DeclaredText for package-lock.json v2.
func TestParsePackageLockV2SourceLocation(t *testing.T) {
	t.Parallel()

	// A minimal v2 package-lock.json with "packages" key.
	// The root "" key should be skipped.
	// A nested "node_modules/foo/node_modules/bar" key should be skipped.
	// line 1:  {
	// line 2:    "lockfileVersion": 2,
	// line 3:    "packages": {
	// line 4:      "": {
	// line 5:        "name": "my-app"
	// line 6:      },
	// line 7:      "node_modules/react": {
	// line 8:        "version": "18.2.0"
	// line 9:      },
	// line 10:     "node_modules/lodash": {
	// line 11:       "version": "4.17.21"
	// line 12:     },
	// line 13:     "node_modules/foo/node_modules/bar": {
	// line 14:       "version": "1.0.0"
	// line 15:     }
	// line 16:   }
	// line 17: }
	content := `{
  "lockfileVersion": 2,
  "packages": {
    "": {
      "name": "my-app"
    },
    "node_modules/react": {
      "version": "18.2.0"
    },
    "node_modules/lodash": {
      "version": "4.17.21"
    },
    "node_modules/foo/node_modules/bar": {
      "version": "1.0.0"
    }
  }
}
`
	result, err := ParsePackageLock([]byte(content), "package-lock.json")
	if err != nil {
		t.Fatalf("ParsePackageLock v2 error: %v", err)
	}
	// Should have react and lodash only (root "" and nested skipped)
	if len(result.Purls) != 2 {
		t.Fatalf("got %d purls, want 2; purls: %+v", len(result.Purls), result.Purls)
	}

	// Build a lookup map — v2 results come from a Go map iteration so order is non-deterministic.
	v2Map := make(map[string]LocalPurl)
	for _, p := range result.Purls {
		v2Map[p.Purl] = p
	}

	// react: "node_modules/react" key at line 7
	reactV2 := v2Map["pkg:npm/react@18.2.0"]
	if reactV2.Line != 7 {
		t.Errorf("v2 react Line: got %d, want 7", reactV2.Line)
	}
	if reactV2.DeclaredText != `"node_modules/react": {` {
		t.Errorf("v2 react DeclaredText: got %q, want %q", reactV2.DeclaredText, `"node_modules/react": {`)
	}

	// lodash: "node_modules/lodash" key at line 10
	lodashV2 := v2Map["pkg:npm/lodash@4.17.21"]
	if lodashV2.Line != 10 {
		t.Errorf("v2 lodash Line: got %d, want 10", lodashV2.Line)
	}
	if lodashV2.DeclaredText != `"node_modules/lodash": {` {
		t.Errorf("v2 lodash DeclaredText: got %q, want %q", lodashV2.DeclaredText, `"node_modules/lodash": {`)
	}
}
