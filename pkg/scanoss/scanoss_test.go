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
	"sync/atomic"
	"testing"
)

func comps(n int) []Component {
	out := make([]Component, n)
	for i := 0; i < n; i++ {
		out[i] = Component{Purl: "pkg:test/c" + string(rune('a'+i%26))}
	}
	return out
}

func TestChunk(t *testing.T) {
	tests := []struct {
		name     string
		n        int
		size     int
		wantLens []int
	}{
		{"empty", 0, 10, nil},
		{"fewer than size", 3, 10, []int{3}},
		{"exact multiple", 20, 10, []int{10, 10}},
		{"remainder", 25, 10, []int{10, 10, 5}},
		{"size one", 3, 1, []int{1, 1, 1}},
		{"size zero collapses to one chunk", 5, 0, []int{5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chunk(comps(tt.n), tt.size)
			if len(got) != len(tt.wantLens) {
				t.Fatalf("got %d chunks, want %d", len(got), len(tt.wantLens))
			}
			for i, c := range got {
				if len(c) != tt.wantLens[i] {
					t.Errorf("chunk %d: got len %d, want %d", i, len(c), tt.wantLens[i])
				}
			}
		})
	}
}

func TestVulnerabilitiesChunksAndAggregates(t *testing.T) {
	var calls int32
	var totalReceived int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		var req componentsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		atomic.AddInt32(&totalReceived, int32(len(req.Components)))
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"components": req.Components,
			"status":     map[string]string{"status": "success"},
		})
	}))
	defer srv.Close()

	client := mustNew(t, Config{APIURL: srv.URL, ChunkSize: 10, Workers: 5})

	// Transparent SDK call: just a list of PURLs.
	purls := make([]string, 25)
	for i := range purls {
		purls[i] = "pkg:test/c"
	}
	res, err := client.decorate(context.Background(), ServiceVulnerabilities, Components(purls...))
	if err != nil {
		t.Fatalf("decorate returned error: %v", err)
	}

	if got := atomic.LoadInt32(&calls); got != 3 { // 25 / 10 -> 3 chunks
		t.Errorf("expected 3 chunk calls, got %d", got)
	}
	if got := atomic.LoadInt32(&totalReceived); got != 25 {
		t.Errorf("expected 25 components received, got %d", got)
	}

	type merged struct {
		Components []Component       `json:"components"`
		Status     map[string]string `json:"status"`
	}
	decoded, err := as[merged](res)
	if err != nil {
		t.Fatalf("as: %v", err)
	}
	if len(decoded.Components) != 25 {
		t.Errorf("expected 25 merged components, got %d", len(decoded.Components))
	}
	if decoded.Status["status"] != "success" {
		t.Errorf("expected merged status preserved, got %v", decoded.Status)
	}
}

func TestServiceEndpointSelection(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"components":[]}`))
	}))
	defer srv.Close()

	client := mustNew(t, Config{APIURL: srv.URL})
	// Closures return only error so the assertions are independent of each
	// service's (typed) response model.
	cases := []struct {
		call func() error
		want string
	}{
		{func() error {
			_, err := client.Licenses.Attribution(context.Background(), Components("pkg:a"))
			return err
		}, "/v3/license/attribution"},
		{func() error {
			_, err := client.Copyright.Holders(context.Background(), Components("pkg:a"))
			return err
		}, "/v3/copyright/holders"},
		{func() error {
			_, err := client.Cryptography.Algorithms(context.Background(), Components("pkg:a"))
			return err
		}, "/v3/cryptography/algorithms"},
		{func() error {
			_, err := client.Cryptography.Hints(context.Background(), Components("pkg:a"))
			return err
		}, "/v3/cryptography/hints"},
		{func() error {
			_, err := client.Geoprovenance.Origins(context.Background(), Components("pkg:a"))
			return err
		}, "/v3/geoprovenance/origin"},
		{func() error {
			_, err := client.Geoprovenance.Countries(context.Background(), Components("pkg:a"))
			return err
		}, "/v3/geoprovenance/countries"},
		{func() error {
			_, err := client.Components.Status(context.Background(), Components("pkg:a"))
			return err
		}, "/v3/components/status"},
	}
	for _, c := range cases {
		if err := c.call(); err != nil {
			t.Fatalf("call failed: %v", err)
		}
		if gotPath != c.want {
			t.Errorf("got path %s, want %s", gotPath, c.want)
		}
	}
}

func TestWorkersCappedByChunkCount(t *testing.T) {
	var concurrent int32
	var maxConcurrent int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := atomic.AddInt32(&concurrent, 1)
		for {
			old := atomic.LoadInt32(&maxConcurrent)
			if cur <= old || atomic.CompareAndSwapInt32(&maxConcurrent, old, cur) {
				break
			}
		}
		atomic.AddInt32(&concurrent, -1)
		_, _ = w.Write([]byte(`{"components":[]}`))
	}))
	defer srv.Close()

	client := mustNew(t, Config{APIURL: srv.URL, ChunkSize: 1, Workers: 5})
	if _, err := client.decorate(context.Background(), ServiceVulnerabilities, Components("pkg:a", "pkg:b")); err != nil {
		t.Fatalf("decorate returned error: %v", err)
	}
	if got := atomic.LoadInt32(&maxConcurrent); got > 2 {
		t.Errorf("expected at most 2 concurrent requests (chunk count), got %d", got)
	}
}

func TestPartialFailure(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Fail the very first chunk, succeed afterwards.
		if atomic.AddInt32(&calls, 1) == 1 {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"components":[{"purl":"pkg:ok"}]}`))
	}))
	defer srv.Close()

	// Single worker so chunk order is deterministic (first chunk fails). Retries off:
	// the point is a chunk that stays failed, and a retried 500 would succeed.
	client := mustNew(t, Config{APIURL: srv.URL, ChunkSize: 1, Workers: 1, MaxRetries: -1})
	res, err := client.decorate(context.Background(), ServiceVulnerabilities, Components("pkg:a", "pkg:b"))
	if err != nil {
		t.Fatalf("expected partial success, got error: %v", err)
	}
	if len(res.failed) != 1 {
		t.Errorf("expected 1 failed chunk, got %d", len(res.failed))
	}
	if len(res.responses) != 1 {
		t.Errorf("expected 1 successful response, got %d", len(res.responses))
	}
}

func TestQueryContextCancelledSkipsRequests(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = w.Write([]byte(`{"components":[]}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before querying

	purls := make([]string, 10)
	for i := range purls {
		purls[i] = "pkg:test/c"
	}

	client := mustNew(t, Config{APIURL: srv.URL, ChunkSize: 1, Workers: 2})
	_, err := client.decorate(ctx, ServiceVulnerabilities, Components(purls...))
	if err == nil {
		t.Fatal("expected an error from a cancelled context")
	}
	// Every chunk should have skipped the network call.
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Errorf("expected 0 HTTP calls with a pre-cancelled context, got %d", got)
	}
}

func TestNoComponents(t *testing.T) {
	client := mustNew(t, Config{APIURL: "http://example.invalid"})
	if _, err := client.decorate(context.Background(), ServiceVulnerabilities, nil); err == nil {
		t.Error("expected error for empty purl list")
	}
}
