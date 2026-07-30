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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestPipelineEndToEnd exercises the whole flow against a stub SCANOSS server
// backing every decoration endpoint: build input with Components, run a
// multi-service pipeline with chunking, and assert the combined keyed output,
// correct per-service routing, chunk merging, and final progress.
func TestPipelineEndToEnd(t *testing.T) {
	// Stub server: echoes the chunk's components back, tagged with the endpoint
	// path so we can verify each service's response is keyed correctly.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req componentsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"components": req.Components,
			"endpoint":   r.URL.Path, // scalar — merge keeps it across chunks
			"status":     map[string]string{"status": "success"},
		})
	}))
	defer srv.Close()

	rec := newRecorder()

	client := New(
		WithAPIURL(srv.URL),
		WithChunkSize(2), // 5 purls -> 3 chunks per service (exercises merge)
		WithWorkers(3),
	)

	p := client.DecorationPipeline(
		ServiceVulnerabilities,
		ServiceLicenses,
		ServiceCryptographyAlgorithms,
		ServiceGeoprovenanceOrigin,
	)

	comps := Components("pkg:a", "pkg:b", "pkg:c", "pkg:d", "pkg:e") // 5 purls
	res, err := p.Run(context.Background(), comps, WithDecorationReporter(rec))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// 1) Combined keyed output: one entry per service.
	wantEndpoints := map[string]string{
		"vulnerabilities":         "/v3/vulnerabilities/vulnerabilities",
		"licenses":                "/v3/licenses",
		"cryptography.algorithms": "/v3/cryptography/algorithms",
		"geoprovenance.origin":    "/v3/geoprovenance/origin",
	}
	if len(res.Services) != len(wantEndpoints) {
		t.Fatalf("services = %d, want %d (%v)", len(res.Services), len(wantEndpoints), keys(res.Services))
	}

	// 2) Each service merged all 3 chunks back to 5 components, and was routed
	//    to the correct endpoint.
	for name, wantEP := range wantEndpoints {
		r := res.Services[name]
		if r == nil {
			t.Errorf("missing service %q", name)
			continue
		}
		var got struct {
			Components []Component `json:"components"`
			Endpoint   string      `json:"endpoint"`
		}
		if err := r.Unmarshal(&got); err != nil {
			t.Errorf("%s unmarshal: %v", name, err)
			continue
		}
		if len(got.Components) != 5 {
			t.Errorf("%s merged components = %d, want 5", name, len(got.Components))
		}
		if got.Endpoint != wantEP {
			t.Errorf("%s endpoint = %q, want %q", name, got.Endpoint, wantEP)
		}
	}

	// 3) Top-level MarshalJSON is a valid object keyed by service.
	var asObject map[string]json.RawMessage
	if err := json.Unmarshal([]byte(res.String()), &asObject); err != nil {
		t.Fatalf("res.String() not valid JSON object: %v", err)
	}
	if len(asObject) != len(wantEndpoints) {
		t.Errorf("marshaled keys = %d, want %d", len(asObject), len(wantEndpoints))
	}

	// 4) Final progress: every service reached Done==Total==5 purls.
	final := rec.snapshot()
	if len(final) != len(wantEndpoints) {
		t.Fatalf("services that reported = %d, want %d", len(final), len(wantEndpoints))
	}
	for name := range wantEndpoints {
		if dt := final[name]; dt[0] != 5 || dt[1] != 5 {
			t.Errorf("%s ended at %d/%d, want 5/5", name, dt[0], dt[1])
		}
	}
}
