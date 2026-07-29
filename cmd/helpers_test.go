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

import (
	"strings"
	"testing"
)

func TestValidateSizeBounds(t *testing.T) {
	tests := []struct {
		name    string
		min     int64
		max     int64
		wantErr string // substring the message must name; empty means no error
	}{
		{"both unset", 0, 0, ""},
		{"minimum only", 100, 0, ""},
		{"maximum only", 0, 1024, ""},
		{"minimum below maximum", 100, 1024, ""},
		{"minimum equal to maximum", 1024, 1024, ""},
		{"negative minimum", -1, 0, "--min-size"},
		{"negative maximum", 0, -1, "--max-size"},
		{"minimum above maximum", 2048, 1024, "--min-size"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSizeBounds(tc.min, tc.max)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("validateSizeBounds(%d, %d) = %v, want nil", tc.min, tc.max, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateSizeBounds(%d, %d) = nil, want an error", tc.min, tc.max)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not name %q", err, tc.wantErr)
			}
		})
	}
}

// A minimum above an unlimited maximum (0) is not a contradiction: 0 means "no
// upper bound", so the pair is valid however large the minimum is.
func TestValidateSizeBoundsUnlimitedMax(t *testing.T) {
	if err := validateSizeBounds(1<<40, 0); err != nil {
		t.Fatalf("validateSizeBounds(large, 0) = %v, want nil", err)
	}
}
