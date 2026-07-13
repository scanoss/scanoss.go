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

import "strings"

// Component is a single PURL (optionally pinned to a version requirement).
type Component struct {
	Purl        string `json:"purl"`
	Requirement string `json:"requirement,omitempty"`
}

// componentsRequest is the body shared by every batch (".../components") endpoint.
type componentsRequest struct {
	Components []Component `json:"components"`
}

// Components builds a []Component from PURL strings, with empty requirements.
// It accepts a slice via Components(purls...). For per-component version
// requirements, construct []Component{{Purl, Requirement}} directly.
// Blank entries are skipped.
func Components(purls ...string) []Component {
	comps := make([]Component, 0, len(purls))
	for _, p := range purls {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		comps = append(comps, Component{Purl: p})
	}
	return comps
}
