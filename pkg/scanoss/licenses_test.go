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

// The declared-licenses batch method POSTs to /v3/licenses and decodes into the
// generated ComponentsLicenseResponse.
func TestLicensesComponentsBatch(t *testing.T) {
	const body = `{"components":[{"purl":"pkg:npm/lodash","statement":"MIT","licenses":[{"id":"MIT","is_spdx_approved":true}]}],"status":{"status":"SUCCESS"}}`
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	client := New(WithAPIURL(srv.URL))
	res, err := client.Licenses.Components(context.Background(), Components("pkg:npm/lodash"))
	if err != nil {
		t.Fatalf("Components: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/v3/licenses" {
		t.Errorf("got %s %s, want POST /v3/licenses", gotMethod, gotPath)
	}
	if res.Components == nil || len(*res.Components) != 1 {
		t.Fatalf("expected 1 component, got %+v", res.Components)
	}
	c := (*res.Components)[0]
	if c.Statement == nil || *c.Statement != "MIT" {
		t.Errorf("statement = %v, want MIT", c.Statement)
	}
	if c.Licenses == nil || len(*c.Licenses) == 0 || (*c.Licenses)[0].Id == nil || *(*c.Licenses)[0].Id != "MIT" {
		t.Errorf("expected MIT license info, got %+v", c.Licenses)
	}
}

// The single method GETs /v3/licenses with purl/requirement query params.
func TestLicensesComponentSingle(t *testing.T) {
	var gotMethod, gotPath, gotPurl, gotReq string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotPurl = r.URL.Query().Get("purl")
		gotReq = r.URL.Query().Get("requirement")
		_, _ = w.Write([]byte(`{"component":{"purl":"pkg:npm/lodash","statement":"MIT"},"status":{"status":"SUCCESS"}}`))
	}))
	defer srv.Close()

	client := New(WithAPIURL(srv.URL))
	res, err := client.Licenses.Component(context.Background(), Component{Purl: "pkg:npm/lodash", Requirement: "4.17.21"})
	if err != nil {
		t.Fatalf("Component: %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/v3/licenses" {
		t.Errorf("got %s %s, want GET /v3/licenses", gotMethod, gotPath)
	}
	if gotPurl != "pkg:npm/lodash" || gotReq != "4.17.21" {
		t.Errorf("got purl=%q requirement=%q", gotPurl, gotReq)
	}
	if res.Component == nil || res.Component.Statement == nil || *res.Component.Statement != "MIT" {
		t.Errorf("expected component with statement MIT, got %+v", res.Component)
	}
}

// Details and Obligations are keyed by license id (GET ?id=) and error on empty id.
func TestLicensesDetailsAndObligations(t *testing.T) {
	var gotPath, gotID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotID = r.URL.Query().Get("id")
		_, _ = w.Write([]byte(`{"status":{"status":"SUCCESS"}}`))
	}))
	defer srv.Close()

	client := New(WithAPIURL(srv.URL))

	if _, err := client.Licenses.Details(context.Background(), "MIT"); err != nil {
		t.Fatalf("Details: %v", err)
	}
	if gotPath != "/v3/licenses/details" || gotID != "MIT" {
		t.Errorf("got path=%q id=%q, want /v3/licenses/details id=MIT", gotPath, gotID)
	}

	if _, err := client.Licenses.Obligations(context.Background(), "MIT"); err != nil {
		t.Fatalf("Obligations: %v", err)
	}
	if gotPath != "/v3/licenses/obligations" || gotID != "MIT" {
		t.Errorf("got path=%q id=%q, want /v3/licenses/obligations id=MIT", gotPath, gotID)
	}

	if _, err := client.Licenses.Details(context.Background(), ""); err == nil {
		t.Error("Details: expected error for empty license id")
	}
	if _, err := client.Licenses.Obligations(context.Background(), ""); err == nil {
		t.Error("Obligations: expected error for empty license id")
	}
}
