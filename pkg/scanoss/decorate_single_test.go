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

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A singular method issues one GET to the single-component endpoint, with the
// component's purl/requirement as query params, and wraps the body verbatim.
func TestSingleComponentGET(t *testing.T) {
	const body = `{"component":{"purl":"pkg:a","version":"1.2.3"},"status":{"status":"SUCCESS"}}`
	var gotMethod, gotPath, gotPurl, gotReq string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotPurl = r.URL.Query().Get("purl")
		gotReq = r.URL.Query().Get("requirement")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	client := mustNew(t, Config{APIURL: srv.URL})
	res, err := client.decorateOne(context.Background(), ServiceVulnerability, Component{Purl: "pkg:a", Requirement: "1.2.3"})
	if err != nil {
		t.Fatalf("decorateOne returned error: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("expected GET, got %s", gotMethod)
	}
	if gotPath != "/v3/vulnerabilities/vulnerabilities" {
		t.Errorf("expected singular endpoint, got %s", gotPath)
	}
	if gotPurl != "pkg:a" || gotReq != "1.2.3" {
		t.Errorf("expected purl/requirement params, got purl=%q requirement=%q", gotPurl, gotReq)
	}

	merged, err := res.Merged()
	if err != nil {
		t.Fatalf("Merged failed: %v", err)
	}
	if string(merged) != body {
		t.Errorf("Merged got %s, want %s", merged, body)
	}
}

func TestSingleComponentRequiresPurl(t *testing.T) {
	client := mustNew(t, Config{APIURL: "http://example.invalid"})
	if _, err := client.decorateOne(context.Background(), ServiceVulnerability, Component{}); err == nil {
		t.Error("expected error for empty component")
	}
}

// Components.Versions is keyed by purl (not a component body): a GET with
// ?purl=&limit=.
func TestComponentVersionsGET(t *testing.T) {
	var gotPath, gotPurl, gotLimit string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotPurl = r.URL.Query().Get("purl")
		gotLimit = r.URL.Query().Get("limit")
		_, _ = w.Write([]byte(`{"component":{"purl":"pkg:a"},"status":{"status":"SUCCESS"}}`))
	}))
	defer srv.Close()

	client := mustNew(t, Config{APIURL: srv.URL})
	if _, err := client.Components.Versions(context.Background(), "pkg:a", 5); err != nil {
		t.Fatalf("Versions error: %v", err)
	}
	if gotPath != "/v3/components/versions" {
		t.Errorf("got path %s, want /v3/components/versions", gotPath)
	}
	if gotPurl != "pkg:a" {
		t.Errorf("got purl %q, want pkg:a", gotPurl)
	}
	if gotLimit != "5" {
		t.Errorf("got limit %q, want 5", gotLimit)
	}

	if _, err := client.Components.Versions(context.Background(), "", 0); err == nil {
		t.Error("expected error for empty purl")
	}
}
