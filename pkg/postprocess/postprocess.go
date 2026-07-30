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

// Apply runs the BOM rules over a scan result in place.
//
// The order is the point of this function, and the reason the individual rules are not exported:
// bom.remove runs first, then bom.replace over what survived it. Reversed, a replacement rewrites
// the PURL a remove rule was written against, the remove rule then matches nothing, and a
// component the user dismissed is reported under its new name — silently, since neither rule has
// failed. Making the order the caller's problem would be handing them that trap.
//
// A nil result or nil BOM is a no-op.
func Apply(result *scanossapi.ScanResult, bom *settings.BOM) {
	applyRemove(result, bom)
	applyReplace(result, bom)
}
