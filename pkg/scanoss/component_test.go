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

package scanoss

import "testing"

func TestComponents(t *testing.T) {
	got := Components("pkg:npm/lodash", "  pkg:npm/express  ", "", "   ")
	want := []Component{
		{Purl: "pkg:npm/lodash"},
		{Purl: "pkg:npm/express"}, // trimmed
	}
	if len(got) != len(want) {
		t.Fatalf("got %d components, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("component %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestComponentsEmpty(t *testing.T) {
	if got := Components(); len(got) != 0 {
		t.Errorf("Components() = %+v, want empty", got)
	}
}

func TestComponentsFromSlice(t *testing.T) {
	purls := []string{"pkg:a", "pkg:b"}
	if got := Components(purls...); len(got) != 2 {
		t.Errorf("Components(slice...) = %+v, want 2 components", got)
	}
}
