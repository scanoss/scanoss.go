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
	"encoding/json"
	"testing"
)

// TestLocalPurlJsonRoundTrip verifies that Line and DeclaredText are present in
// marshaled JSON when set, absent (omitempty) when zero, and that existing fields
// (Purl, Requirement, Scope) are unchanged.
func TestLocalPurlJsonRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		input            LocalPurl
		wantLine         bool   // true if "line" key expected in JSON
		wantDeclaredText bool   // true if "declaredText" key expected in JSON
		wantLineValue    int    // expected line value when wantLine is true
		wantTextValue    string // expected declaredText value when wantDeclaredText is true
	}{
		{
			name: "all fields set",
			input: LocalPurl{
				Purl:         "pkg:npm/react@18.0.0",
				Requirement:  "^18.0.0",
				Scope:        "dependencies",
				Line:         7,
				DeclaredText: `"react": "^18.0.0"`,
			},
			wantLine:         true,
			wantDeclaredText: true,
			wantLineValue:    7,
			wantTextValue:    `"react": "^18.0.0"`,
		},
		{
			name: "zero line and empty declaredText are omitted",
			input: LocalPurl{
				Purl:        "pkg:npm/lodash@4.17.21",
				Requirement: "4.17.21",
				Scope:       "dependencies",
			},
			wantLine:         false,
			wantDeclaredText: false,
		},
		{
			name: "existing fields unchanged",
			input: LocalPurl{
				Purl:        "pkg:golang/github.com/spf13/cobra@1.10.2",
				Requirement: "1.10.2",
				Scope:       "compile",
			},
			wantLine:         false,
			wantDeclaredText: false,
		},
		{
			name: "only line set",
			input: LocalPurl{
				Purl: "pkg:pypi/requests@2.28.0",
				Line: 3,
			},
			wantLine:         true,
			wantDeclaredText: false,
			wantLineValue:    3,
		},
		{
			name: "only declaredText set — line still zero, omitted",
			input: LocalPurl{
				Purl:         "pkg:pypi/flask",
				DeclaredText: "flask",
			},
			wantLine:         false,
			wantDeclaredText: true,
			wantTextValue:    "flask",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			data, err := json.Marshal(tc.input)
			if err != nil {
				t.Fatalf("json.Marshal failed: %v", err)
			}

			// Unmarshal into a generic map to inspect keys
			var m map[string]interface{}
			if err := json.Unmarshal(data, &m); err != nil {
				t.Fatalf("json.Unmarshal into map failed: %v", err)
			}

			// Check "line" presence/absence
			lineVal, linePresent := m["line"]
			if tc.wantLine && !linePresent {
				t.Errorf("expected 'line' key in JSON, got: %s", string(data))
			}
			if !tc.wantLine && linePresent {
				t.Errorf("expected 'line' key absent in JSON (omitempty), got: %s", string(data))
			}
			if tc.wantLine && linePresent {
				gotLine := int(lineVal.(float64))
				if gotLine != tc.wantLineValue {
					t.Errorf("line: got %d, want %d", gotLine, tc.wantLineValue)
				}
			}

			// Check "declaredText" presence/absence
			textVal, textPresent := m["declaredText"]
			if tc.wantDeclaredText && !textPresent {
				t.Errorf("expected 'declaredText' key in JSON, got: %s", string(data))
			}
			if !tc.wantDeclaredText && textPresent {
				t.Errorf("expected 'declaredText' key absent in JSON (omitempty), got: %s", string(data))
			}
			if tc.wantDeclaredText && textPresent {
				gotText := textVal.(string)
				if gotText != tc.wantTextValue {
					t.Errorf("declaredText: got %q, want %q", gotText, tc.wantTextValue)
				}
			}

			// Existing fields must always be present/correct
			if tc.input.Purl != "" {
				if gotPurl, ok := m["purl"].(string); !ok || gotPurl != tc.input.Purl {
					t.Errorf("purl: got %q, want %q", m["purl"], tc.input.Purl)
				}
			}

			// Round-trip back to struct
			var out LocalPurl
			if err := json.Unmarshal(data, &out); err != nil {
				t.Fatalf("round-trip Unmarshal failed: %v", err)
			}
			if out.Purl != tc.input.Purl {
				t.Errorf("round-trip Purl: got %q, want %q", out.Purl, tc.input.Purl)
			}
			if out.Requirement != tc.input.Requirement {
				t.Errorf("round-trip Requirement: got %q, want %q", out.Requirement, tc.input.Requirement)
			}
			if out.Scope != tc.input.Scope {
				t.Errorf("round-trip Scope: got %q, want %q", out.Scope, tc.input.Scope)
			}
			if out.Line != tc.input.Line {
				t.Errorf("round-trip Line: got %d, want %d", out.Line, tc.input.Line)
			}
			if out.DeclaredText != tc.input.DeclaredText {
				t.Errorf("round-trip DeclaredText: got %q, want %q", out.DeclaredText, tc.input.DeclaredText)
			}
		})
	}
}
