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
	"reflect"
	"strings"
	"testing"
)

// TestRawDocument_LineRangesRoundTrip pins the wire shape of matched line ranges: objects
// carrying start_line/end_line, never "start-end" text, and unchanged by a marshal/parse cycle.
func TestRawDocument_LineRangesRoundTrip(t *testing.T) {
	input := []LineRange{{StartLine: 82, EndLine: 209}, {StartLine: 303, EndLine: 775}}
	oss := []LineRange{{StartLine: 77, EndLine: 204}, {StartLine: 298, EndLine: 770}}
	inv := Inventory{Components: []Component{{
		Purl:    "pkg:github/scanoss/engine",
		Version: "5.4.1",
		URLHash: "abc123",
		Evidence: []FileEvidence{{
			Path:            "deflate_modified.c",
			MatchType:       "snippet",
			InputLineRanges: input,
			OssLineRanges:   oss,
		}},
	}}}

	out, err := NewRawDocument(inv, RawMetadata{Tool: "scanoss"}).Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(out, `"start_line": 82`) || !strings.Contains(out, `"end_line": 209`) {
		t.Errorf("raw document should carry structured ranges, got:\n%s", out)
	}
	if strings.Contains(out, `"82-209"`) {
		t.Errorf("raw document should not encode ranges as text, got:\n%s", out)
	}
	if !strings.Contains(out, `"schema_version": "`+RawSchemaVersion+`"`) {
		t.Errorf("raw document should be stamped %s, got:\n%s", RawSchemaVersion, out)
	}

	got, err := ParseRaw([]byte(out))
	if err != nil {
		t.Fatalf("ParseRaw: %v", err)
	}
	if len(got.Components) != 1 || len(got.Components[0].Evidence) != 1 {
		t.Fatalf("want 1 component with 1 evidence, got %+v", got.Components)
	}
	ev := got.Components[0].Evidence[0]
	if !reflect.DeepEqual(ev.InputLineRanges, input) {
		t.Errorf("input ranges = %+v, want %+v", ev.InputLineRanges, input)
	}
	if !reflect.DeepEqual(ev.OssLineRanges, oss) {
		t.Errorf("oss ranges = %+v, want %+v", ev.OssLineRanges, oss)
	}
}
