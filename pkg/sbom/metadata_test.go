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
	"time"

	spdxjson "github.com/spdx/tools-golang/json"
)

var fixedTime = time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)

func TestDocumentMetadata_CycloneDX(t *testing.T) {
	inv := Inventory{Components: []Component{{Purl: "pkg:npm/x", Version: "1.0.0"}}}
	doc, err := Generate(inv, FormatCycloneDX,
		WithTool("my-tool-1.4.0"), WithAuthor("Acme Corp"), WithTimestamp(fixedTime))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	bom := decodeCDX(t, doc)

	if bom.Metadata == nil {
		t.Fatal("no metadata")
	}
	if bom.Metadata.Timestamp != "2026-07-14T12:00:00Z" {
		t.Errorf("timestamp = %q", bom.Metadata.Timestamp)
	}
	if bom.Metadata.Authors == nil || (*bom.Metadata.Authors)[0].Name != "Acme Corp" {
		t.Errorf("author = %+v", bom.Metadata.Authors)
	}
	if bom.Metadata.Tools == nil || bom.Metadata.Tools.Components == nil ||
		(*bom.Metadata.Tools.Components)[0].Name != "my-tool-1.4.0" {
		t.Errorf("tool = %+v", bom.Metadata.Tools)
	}
}

func TestDocumentMetadata_SPDX(t *testing.T) {
	inv := Inventory{Components: []Component{{Purl: "pkg:npm/x", Version: "1.0.0"}}}
	doc, err := Generate(inv, FormatSPDX,
		WithTool("my-tool-1.4.0"), WithAuthor("Acme Corp"), WithTimestamp(fixedTime))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	sd, err := spdxjson.Read(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("read SPDX: %v", err)
	}
	if sd.CreationInfo.Created != "2026-07-14T12:00:00Z" {
		t.Errorf("created = %q", sd.CreationInfo.Created)
	}
	var tool, org string
	for _, c := range sd.CreationInfo.Creators {
		switch c.CreatorType {
		case "Tool":
			tool = c.Creator
		case "Organization":
			org = c.Creator
		}
	}
	if tool != "my-tool-1.4.0" {
		t.Errorf("tool creator = %q", tool)
	}
	if org != "Acme Corp" {
		t.Errorf("organization creator = %q", org)
	}
}

// TestDocumentMetadata_Defaults asserts that without options the metadata is populated with
// the built-in defaults (behavior unchanged for existing callers).
func TestDocumentMetadata_Defaults(t *testing.T) {
	inv := Inventory{Components: []Component{{Purl: "pkg:npm/x", Version: "1.0.0"}}}
	doc, err := Generate(inv, FormatCycloneDX)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	bom := decodeCDX(t, doc)

	if bom.Metadata.Authors == nil || (*bom.Metadata.Authors)[0].Name == "" {
		t.Error("default author is empty")
	}
	if bom.Metadata.Tools == nil || bom.Metadata.Tools.Components == nil ||
		(*bom.Metadata.Tools.Components)[0].Name == "" {
		t.Error("default tool is empty")
	}
	if bom.Metadata.Timestamp == "" {
		t.Error("default timestamp is empty")
	}
}
