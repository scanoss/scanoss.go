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

package dependencies

import (
	"sort"
	"testing"

	"github.com/scanoss/scanoss.go/pkg/manifests"
)

// TestManifestsMatchParsers guards the single-source-of-truth contract: every
// dependency file the parser handles MUST be listed in pkg/manifests.Patterns,
// and vice versa. pkg/filter relies on manifests.Is to decide which files to
// preserve (PreserveDependencyManifests) while pruning everything else — if the
// two lists drift, the filter would either drop a manifest the parser needs or
// keep a file the parser can't use.
func TestManifestsMatchParsers(t *testing.T) {
	parserFiles := NewDependencyParser().SupportedFiles()

	got := append([]string(nil), parserFiles...)
	want := append([]string(nil), manifests.Patterns...)
	sort.Strings(got)
	sort.Strings(want)

	if len(got) != len(want) {
		t.Fatalf("parser supports %d files, manifests.Patterns has %d\n parser=%v\n manifests=%v",
			len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("drift at %d: parser=%q manifests=%q (full parser=%v manifests=%v)",
				i, got[i], want[i], got, want)
		}
	}
}
