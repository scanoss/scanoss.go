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

import (
	"strings"
	"testing"
)

func TestGenerate_Dispatch(t *testing.T) {
	inv := sampleInventory()

	cyclone, err := Generate(inv, FormatCycloneDX)
	if err != nil {
		t.Fatalf("cyclonedx: %v", err)
	}
	if !strings.Contains(cyclone, `"bomFormat": "CycloneDX"`) {
		t.Error("cyclonedx output does not look like a CycloneDX BOM")
	}

	spdx, err := Generate(inv, FormatSPDX)
	if err != nil {
		t.Fatalf("spdxlite: %v", err)
	}
	if !strings.Contains(spdx, `"spdxVersion": "SPDX-2.3"`) {
		t.Error("spdxlite output does not look like an SPDX document")
	}
}

func TestGenerate_UnknownFormat(t *testing.T) {
	if _, err := Generate(sampleInventory(), Format("xml")); err == nil {
		t.Error("expected error for unknown format")
	}
}
