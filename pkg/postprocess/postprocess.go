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

// Package postprocess applies a scan's bill-of-materials rules to its results: what the user says
// about the components the scan found, once the scan has found them.
//
// It is deliberately apart from the scan SDK. Nothing here talks to the API, holds a client or
// takes a context — these are local transformations over a result the caller already has, which is
// also why a result parsed from a file can go through them just as one straight off a scan can.
package postprocess

import (
	scanossapi "github.com/scanoss/scanoss.api-sdk"

	"github.com/scanoss/scanoss.go/pkg/settings"
)

// Apply runs the BOM rules over a scan result in place, and returns what the selection rules
// concluded (see Report).
//
// The order is the point of this function, and the reason the individual rules are not exported:
//
//  1. The match-level rules — bom.ignore, bom.identify and the ranking threshold — settle which
//     of the candidates the scanner returned for each file survive, and which of them leads.
//     They run first so the file-level rules below see the candidate list the user actually
//     meant, rather than dismissing a file over a component that was about to be dropped
//     anyway. The three share one pass; how they interact is internal to applySelect, which
//     documents it.
//  2. bom.remove neutralizes whole files.
//  3. bom.replace re-points what survived. Reversed with remove, a replacement rewrites the PURL
//     a remove rule was written against, the remove rule then matches nothing, and a component
//     the user dismissed is reported under its new name — silently, since neither rule has
//     failed.
//
// Making the order the caller's problem would be handing them that trap.
//
// A nil result, with nothing to apply, is a no-op returning an empty Report.
func Apply(result *scanossapi.ScanResult, bom *settings.BOM, opts ...Option) Report {
	o := resolveOptions(opts)
	report := applySelect(result, bom, o.rankingThreshold)
	applyRemove(result, bom)
	applyReplace(result, bom)
	return report
}

// Option tunes what Apply does beyond the BOM rules the settings file carries.
type Option func(*options)

type options struct {
	rankingThreshold int
}

// WithRankingThreshold drops matches whose component ranks worse than threshold. Rank is the
// scanner's own ordering of how well a component explains a match, lowest is strongest; ranks
// seen in practice run 1..9.
//
// A threshold at or below 0 disables the filter, which is how the engine reads it too. Matches a
// bom.identify rule claims are never dropped by it.
//
// It is an option rather than a field on settings.BOM because it is not a statement about any
// component: it is a quality bar over whatever the scan found, and it reaches this package from
// a flag as readily as from a settings file.
func WithRankingThreshold(threshold int) Option {
	return func(o *options) { o.rankingThreshold = threshold }
}

func resolveOptions(opts []Option) options {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	return o
}
