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

// Option tunes how fingerprints are generated. Every entry point (Folder, Files, Stream,
// StreamFolder) takes the same set, so a caller does not have to know which one it reached for.
type Option func(*options)

type options struct {
	skipHeaders      bool
	skipHeadersLimit int
}

// WithSkipHeaders drops the leading licence header, documentation comments and imports of each
// file from its fingerprint, so two files sharing nothing but a common licence block do not look
// alike to the matcher.
//
// limit caps how many leading lines may be dropped; 0 or less means no cap. Passing the option
// is what enables the behaviour — there is no separate on/off, because not passing it is off.
//
// Only files whose extension names a language it understands are affected; anything else is
// fingerprinted whole, since dropping lines by guesswork would corrupt the fingerprint.
func WithSkipHeaders(limit int) Option {
	return func(o *options) {
		o.skipHeaders = true
		if limit > 0 {
			o.skipHeadersLimit = limit
		}
	}
}

func resolveOptions(opts []Option) options {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	return o
}
