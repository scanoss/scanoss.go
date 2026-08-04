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

package wfp

import "testing"

// naiveCombine is the original O(n²) implementation, kept here as the reference
// oracle: combineFingerprints must produce byte-identical output.
func naiveCombine(fps []*FileFingerprint) string {
	var result string
	for _, fp := range fps {
		result += fp.Fingerprint //nolint // intentional: reference oracle for the O(n) builder
		if len(fp.Fingerprint) > 0 && fp.Fingerprint[len(fp.Fingerprint)-1] != '\n' {
			result += "\n"
		}
		result += "\n"
	}
	return result
}

func TestCombineFingerprintsParity(t *testing.T) {
	cases := map[string][]*FileFingerprint{
		"nil":                   nil,
		"empty slice":           {},
		"no trailing newline":   {{Fingerprint: "file=a.c,10,abc\n1=deadbeef"}},
		"with trailing newline": {{Fingerprint: "file=a.c,10,abc\n1=deadbeef\n"}},
		"empty fingerprint":     {{Fingerprint: ""}},
		"multiple files": {
			{Fingerprint: "file=a.c,10,abc\n1=deadbeef"},
			{Fingerprint: "file=b.c,20,def\n2=cafef00d\n"},
			{Fingerprint: ""},
		},
	}

	for name, fps := range cases {
		t.Run(name, func(t *testing.T) {
			got := combineFingerprints(fps)
			want := naiveCombine(fps)
			if got != want {
				t.Errorf("output differs from reference\n got: %q\nwant: %q", got, want)
			}
		})
	}
}
