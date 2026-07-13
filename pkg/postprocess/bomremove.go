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

// Package postprocess is retained for backward compatibility. The bom.remove engine now
// lives in pkg/scanoss (scanoss.ApplyBOMRemove); prefer that directly.
package postprocess

import (
	scanossapi "github.com/scanoss/scanoss.api-sdk"
	"github.com/scanoss/scanoss.go/pkg/scanoss"
	"github.com/scanoss/scanoss.go/pkg/settings"
)

// ApplyBomRemoveResult applies bom.remove rules to a scan result in place.
//
// Deprecated: use scanoss.ApplyBOMRemove. Kept as a thin wrapper for existing callers.
func ApplyBomRemoveResult(result *scanossapi.ScanResult, bom *settings.BOM) {
	scanoss.ApplyBOMRemove(result, bom)
}
