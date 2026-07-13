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

package cmd

import (
	"context"
	scanossapi "github.com/scanoss/scanoss.api-sdk"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/scanoss/scanoss.go/pkg/scanoss"
)

func testScanResult() *scanossapi.ScanResult {
	return &scanossapi.ScanResult{
		Files: []scanossapi.FileResult{
			{Path: "a.go", MatchType: "file", Matches: []scanossapi.MatchResult{{UrlHash: "h1"}}},
		},
		Components: map[string]scanossapi.ComponentResult{
			"h1": {Purls: []string{"pkg:npm/lodash"}, Vendor: "lodash", Version: "4.17.20"},
		},
	}
}

// decorationServer routes the licenses and vulnerabilities decoration endpoints to
// canned responses, so renderResult can be exercised without a live API.
func decorationServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/vulnerabilities"):
			_, _ = w.Write([]byte(`{"components":[{"purl":"pkg:npm/lodash","vulnerabilities":[{"id":"CVE-2021-23337","severity":"HIGH","source":"NVD"}]}]}`))
		case strings.Contains(r.URL.Path, "/licenses"):
			// requirement echoes the queried version (testScanResult => 4.17.20).
			_, _ = w.Write([]byte(`{"components":[{"purl":"pkg:npm/lodash","requirement":"4.17.20","licenses":[{"id":"MIT"}]}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestRenderResult_Plain(t *testing.T) {
	// plain makes no decoration call, so a nil client is fine.
	out, err := renderResult(context.Background(), nil, testScanResult(), "plain", "proj")
	if err != nil {
		t.Fatalf("renderResult: %v", err)
	}
	if !strings.Contains(out, `"components"`) || !strings.Contains(out, "pkg:npm/lodash") {
		t.Errorf("plain output should be the raw result JSON, got:\n%s", out)
	}
}

func TestRenderResult_SPDXLite(t *testing.T) {
	srv := decorationServer()
	defer srv.Close()
	client := scanoss.New(scanoss.WithAPIURL(srv.URL), scanoss.WithAPIKey("x"))

	out, err := renderResult(context.Background(), client, testScanResult(), "spdx", "proj")
	if err != nil {
		t.Fatalf("renderResult: %v", err)
	}
	if !strings.Contains(out, `"spdxVersion": "SPDX-2.3"`) {
		t.Errorf("expected SPDX 2.3 document, got:\n%s", out)
	}
	// declared license fetched from the decoration service, matched by PURL.
	if !strings.Contains(out, `"licenseDeclared": "MIT"`) {
		t.Errorf("expected the decoration license in licenseDeclared, got:\n%s", out)
	}
}

func TestRenderResult_CycloneDXWithDecoration(t *testing.T) {
	srv := decorationServer()
	defer srv.Close()
	client := scanoss.New(scanoss.WithAPIURL(srv.URL), scanoss.WithAPIKey("x"))

	out, err := renderResult(context.Background(), client, testScanResult(), "cyclonedx", "proj")
	if err != nil {
		t.Fatalf("renderResult: %v", err)
	}
	if !strings.Contains(out, `"specVersion": "1.7"`) {
		t.Errorf("expected CycloneDX 1.7, got:\n%s", out)
	}
	if !strings.Contains(out, `"id": "MIT"`) {
		t.Errorf("expected the fetched license in the CycloneDX output, got:\n%s", out)
	}
	if !strings.Contains(out, "CVE-2021-23337") {
		t.Errorf("expected the fetched vulnerability in the CycloneDX output, got:\n%s", out)
	}
}

func TestRenderResult_DecorationFailureNonFatal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	client := scanoss.New(scanoss.WithAPIURL(srv.URL), scanoss.WithAPIKey("x"))

	out, err := renderResult(context.Background(), client, testScanResult(), "cyclonedx", "proj")
	if err != nil {
		t.Fatalf("a decoration failure should be non-fatal, got error: %v", err)
	}
	if !strings.Contains(out, `"specVersion": "1.7"`) {
		t.Errorf("expected a valid CycloneDX document despite the decoration failure")
	}
}

func TestRenderResult_UnknownFormat(t *testing.T) {
	if _, err := renderResult(context.Background(), nil, testScanResult(), "xml", "proj"); err == nil {
		t.Error("expected an error for an unsupported format")
	}
}
