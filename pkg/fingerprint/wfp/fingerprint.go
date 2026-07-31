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

package fingerprint

import "strings"

// FileFingerprint is one file's fingerprint: what the scan uploads about it, and what every
// stage between hashing and upload passes around.
//
// It lives here, beside the function that produces it, because it is the vocabulary the
// fingerprinting packages speak to each other in — the worker pool and the scan service both
// name it. Kept out of reach, it would leave those packages with signatures no caller could
// write down.
type FileFingerprint struct {
	Path        string // path of the fingerprinted file, relative to the scan root
	Hash        string // whole-file hash
	Size        int    // file size in bytes
	Fingerprint string // the WFP text itself, "file=..." and its minutiae
}

// CombineFingerprints joins fingerprints into the single WFP stream a scan uploads,
// with a blank line between files as the format requires.
//
// The builder is pre-sized because a naive `result += ...` is O(n²) — each += copies
// the whole accumulated string — and that dominated wall-clock time on large scans:
// ~38s to assemble a 16 MB WFP from ~8900 files.
func CombineFingerprints(fps []*FileFingerprint) string {
	var total int
	for _, fp := range fps {
		// fingerprint + up to one missing trailing newline + one blank line
		total += len(fp.Fingerprint) + 2
	}

	var b strings.Builder
	b.Grow(total)
	for _, fp := range fps {
		b.WriteString(fp.Fingerprint)
		if len(fp.Fingerprint) > 0 && fp.Fingerprint[len(fp.Fingerprint)-1] != '\n' {
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}
	return b.String()
}
