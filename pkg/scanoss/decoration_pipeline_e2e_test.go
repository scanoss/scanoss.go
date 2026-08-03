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
	"sync"
	"testing"
)

// TestPipelineEndToEnd exercises the whole flow against a stub SCANOSS server
// backing every decoration endpoint: build input with Components, run a
// multi-service pipeline with chunking, and assert the combined keyed output,
// correct per-service routing, chunk merging, and final progress.
func TestPipelineEndToEnd(t *testing.T) {
	// The paths are recorded here rather than echoed back in the body: a typed response only
	// carries the fields its service declares, so an echoed field would not survive decoding.
	var pathMu sync.Mutex
	gotPaths := map[string]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pathMu.Lock()
		gotPaths[r.URL.Path] = true
		pathMu.Unlock()

		var req componentsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// Each service is decoded into its own type now, so the stub has to answer in that
		// service's schema: geoprovenance keys its list "components_locations".
		key := "components"
		if r.URL.Path == ServiceGeoprovenanceOrigin.endpoint {
			key = "components_locations"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			key:      req.Components,
			"status": map[string]string{"status": "success"},
		})
	}))
	defer srv.Close()

	rec := newRecorder()

	client := mustNew(t, Config{
		APIURL:    srv.URL,
		ChunkSize: 2, // 5 purls -> 3 chunks per service (exercises merge)
		Workers:   3,
	})

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
	if got := populatedLayers(res); len(got) != len(wantEndpoints) {
		t.Fatalf("layers = %d, want %d (%v)", len(got), len(wantEndpoints), got)
	}

	// 2) Every service was routed to its own endpoint.
	pathMu.Lock()
	for name, wantEP := range wantEndpoints {
		if !gotPaths[wantEP] {
			t.Errorf("%s was never requested at %q (paths seen: %v)", name, wantEP, gotPaths)
		}
	}
	pathMu.Unlock()

	// 3) Each service merged all 3 chunks back to 5 components.
	if n := len(res.Vulnerabilities.Response.Components); n != 5 {
		t.Errorf("vulnerabilities merged components = %d, want 5", n)
	}
	if n := len(*res.Licenses.Response.Components); n != 5 {
		t.Errorf("licenses merged components = %d, want 5", n)
	}
	if n := len(res.Cryptography.Response.Components); n != 5 {
		t.Errorf("cryptography merged components = %d, want 5", n)
	}
	if n := len(res.Geoprovenance.Response.ComponentsLocations); n != 5 {
		t.Errorf("geoprovenance merged components = %d, want 5", n)
	}

	// 4) Top-level MarshalJSON is a valid object keyed by service.
	var asObject map[string]json.RawMessage
	if err := json.Unmarshal([]byte(res.String()), &asObject); err != nil {
		t.Fatalf("res.String() not valid JSON object: %v", err)
	}
	if len(asObject) != len(wantEndpoints) {
		t.Errorf("marshaled keys = %d, want %d", len(asObject), len(wantEndpoints))
	}

	// 5) Final progress: every service reached Done==Total==5 purls.
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
