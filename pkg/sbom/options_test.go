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

package sbom

import "testing"

func TestOptions_ProjectNameDefault(t *testing.T) {
	if got := newOptions().projectName; got != defaultProjectName {
		t.Errorf("default projectName = %q, want %q", got, defaultProjectName)
	}
}

func TestOptions_ProjectNameOverride(t *testing.T) {
	if got := newOptions(WithProjectName("acme")).projectName; got != "acme" {
		t.Errorf("projectName = %q, want acme", got)
	}
}

func TestOptions_EmptyProjectNameIgnored(t *testing.T) {
	if got := newOptions(WithProjectName("")).projectName; got != defaultProjectName {
		t.Errorf("empty WithProjectName should keep default, got %q", got)
	}
}
