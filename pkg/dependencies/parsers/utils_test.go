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

// TestGetScopedPackage verifies scope handling. The leading "@" MUST be kept
// on the namespace so PURLs stay canonical (pkg:npm/@scope/name) — the SCANOSS
// decoration services do not resolve scope-stripped PURLs.
func TestGetScopedPackage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		pkg           string
		wantNamespace string
		wantName      string
	}{
		{"scoped", "@angular/core", "@angular", "core"},
		{"scoped babel", "@babel/traverse", "@babel", "traverse"},
		{"unscoped", "lodash", "", "lodash"},
		{"lone at prefix no slash", "@weird", "", "@weird"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ns, name := GetScopedPackage(tc.pkg)
			if ns != tc.wantNamespace || name != tc.wantName {
				t.Errorf("GetScopedPackage(%q) = (%q, %q), want (%q, %q)",
					tc.pkg, ns, name, tc.wantNamespace, tc.wantName)
			}
		})
	}
}

// TestOffsetToLine verifies the shared byte-offset-to-line-number helper.
func TestOffsetToLine(t *testing.T) {
	t.Parallel()

	// "line1\nline2\nline3"
	//  01234 5 67890 1 23456
	content := []byte("line1\nline2\nline3")

	tests := []struct {
		name    string
		content []byte
		off     int
		want    int
	}{
		{"off=0 is line 1", content, 0, 1},
		{"mid-first-line is line 1", content, 3, 1},
		{"just after first newline is line 2", content, 6, 2},
		{"mid-second-line is line 2", content, 8, 2},
		{"just after second newline is line 3", content, 12, 3},
		{"last byte is line 3", content, len(content), 3},
		{"off negative clamps to line 1", content, -1, 1},
		{"off > len clamps to last line", content, len(content) + 100, 3},
		{"empty content returns line 1", []byte{}, 0, 1},
		{"content with no newlines", []byte("nodeps"), 3, 1},
		{"content with no newlines off=0", []byte("nodeps"), 0, 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := offsetToLine(tc.content, tc.off)
			if got != tc.want {
				t.Errorf("offsetToLine(content, %d) = %d, want %d", tc.off, got, tc.want)
			}
		})
	}
}

// TestLineTextAt verifies the helper that extracts the physical line at a given offset.
func TestLineTextAt(t *testing.T) {
	t.Parallel()

	// "first line\nsecond line\nthird"
	content := []byte("first line\nsecond line\nthird")

	tests := []struct {
		name    string
		content []byte
		off     int
		want    string
	}{
		{"offset on first line (no leading newline)", content, 3, "first line"},
		{"offset on middle line", content, 14, "second line"},
		{"offset on last line (no trailing newline)", content, 24, "third"},
		{"offset at first newline byte", content, 10, "first line"},
		{"offset=0", content, 0, "first line"},
		{"empty content", []byte{}, 0, ""},
		{"single line no newline", []byte("  hello  "), 4, "hello"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := lineTextAt(tc.content, tc.off)
			if got != tc.want {
				t.Errorf("lineTextAt(content, %d) = %q, want %q", tc.off, got, tc.want)
			}
		})
	}
}
