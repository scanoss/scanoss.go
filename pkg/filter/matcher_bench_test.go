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

import "testing"

// benchInfo is a realistic mix of entries a scan walk feeds to composite.Match:
// kept source files, skipped extensions (early and late in the default list),
// skipped names/endings, and directories.
var benchInfo = []fakeInfo{
	aFile("main.go", 2000),
	aFile("handler.go", 2000),
	aFile("index.html", 2000), // skip: ext
	aFile("styles.css", 2000), // skip: ext
	aFile("data.json", 2000),  // skip: ext
	aFile("photo.png", 2000),  // skip: ext
	aFile("notes.txt", 2000),  // skip: ext
	aFile("app.min.js", 2000), // skip: compound ext
	aFile("README", 2000),     // skip: ending
	aFile("Makefile", 2000),   // skip: name
	aFile("service.py", 2000),
	aFile("lib.rs", 2000),
	aDir("src"),
	aDir("node_modules"), // skip: dir
	aDir("vendor"),       // skip: dir
	aFile("go.mod", 2000),
}

// BenchmarkCompositeMatch measures the per-entry filtering cost over the built-in
// defaults — the hot path run once per file during a scan walk.
func BenchmarkCompositeMatch(b *testing.B) {
	c := build(defaultSource(stdDefaults()))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, info := range benchInfo {
			c.Match(info.Name(), info)
		}
	}
}
