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
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPipelineOnProgressSnapshot(t *testing.T) {
	srv := httptest.NewServer(echoServer(""))
	defer srv.Close()

	client := New(WithAPIURL(srv.URL), WithChunkSize(5), WithWorkers(4))
	p := client.DecorationPipeline(ServiceVulnerabilities, ServiceLicenses, ServiceGeoprovenanceOrigin)

	// Serial delivery: a plain map write with no lock must be race-free.
	var lastByService map[string]Progress
	p.OnProgress(func(pp PipelineProgress) {
		lastByService = pp.Services // no mutex needed — serial delivery
	})

	purls := make([]string, 12) // 12 purls / chunk 5 -> 3 chunks per service
	for i := range purls {
		purls[i] = "pkg:test/c"
	}

	if _, err := p.Run(context.Background(), Components(purls...)); err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// Final snapshot has every service at Done==Total==12 purls.
	snap := p.Snapshot()
	if len(snap.Services) != 3 {
		t.Fatalf("snapshot services = %d, want 3", len(snap.Services))
	}
	for _, name := range []string{"vulnerabilities", "licenses", "geoprovenance.origin"} {
		pr, ok := snap.Services[name]
		if !ok {
			t.Errorf("missing %q in snapshot", name)
			continue
		}
		if pr.Done != 12 || pr.Total != 12 {
			t.Errorf("%s progress = %d/%d, want 12/12", name, pr.Done, pr.Total)
		}
		if pr.Unit != "purls" {
			t.Errorf("%s unit = %q, want purls", name, pr.Unit)
		}
	}
	if len(lastByService) == 0 {
		t.Error("OnProgress was never called")
	}
}

func TestPipelineRunsInParallel(t *testing.T) {
	// Barrier: each handler blocks until `want` requests are concurrently in
	// flight, which deterministically proves the services overlap.
	const want = 2
	var mu sync.Mutex
	count := 0
	release := make(chan struct{})
	var inFlight, maxInFlight int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := atomic.AddInt32(&inFlight, 1)
		for {
			old := atomic.LoadInt32(&maxInFlight)
			if cur <= old || atomic.CompareAndSwapInt32(&maxInFlight, old, cur) {
				break
			}
		}
		mu.Lock()
		count++
		if count == want {
			close(release) // enough requests are now simultaneously waiting
		}
		mu.Unlock()
		select {
		case <-release:
		case <-time.After(2 * time.Second): // safety, avoids hanging the test
		}
		atomic.AddInt32(&inFlight, -1)
		_, _ = w.Write([]byte(`{"components":[]}`))
	}))
	defer srv.Close()

	client := New(WithAPIURL(srv.URL), WithChunkSize(100), WithWorkers(1))
	p := client.DecorationPipeline(
		ServiceVulnerabilities,
		ServiceLicenses,
		ServiceCryptographyAlgorithms,
		ServiceGeoprovenanceOrigin,
	)
	if _, err := p.Run(context.Background(), Components("pkg:a")); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if atomic.LoadInt32(&maxInFlight) < want {
		t.Errorf("expected services to run in parallel (max in-flight ≥ %d), got %d", want, maxInFlight)
	}
}
