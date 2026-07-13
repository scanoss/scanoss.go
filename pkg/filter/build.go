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

package filter

// Build concatenates matchers from every source and removes duplicates by Key(),
// then wraps the result in a single Composite. Sources are processed in the order
// given, and the first matcher seen for a given Key() wins (later duplicates are
// dropped) — so a rule shared across sources (e.g. ".png" in both the defaults
// and a scanoss.json pattern) is applied once, not repeatedly.
func Build(sources ...[]Matcher) *Composite {
	seen := make(map[string]bool)
	var matchers []Matcher
	for _, src := range sources {
		for _, m := range src {
			key := m.Key()
			if seen[key] {
				continue
			}
			seen[key] = true
			matchers = append(matchers, m)
		}
	}
	return &Composite{matchers: matchers}
}
