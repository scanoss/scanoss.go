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

	client := mustNew(t, Config{APIURL: srv.URL, ChunkSize: 100, Workers: 1})
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

// recorder records decoration progress per service, for tests that assert what each service
// reported. Services run concurrently, so it guards its map.
type recorder struct {
	mu   sync.Mutex
	last map[string][2]int // service -> {done, total}
	sawN int
}

func newRecorder() *recorder { return &recorder{last: map[string][2]int{}} }

func (r *recorder) Decorating(service string, done, total int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.last[service] = [2]int{done, total}
	r.sawN++
}

func (r *recorder) snapshot() map[string][2]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string][2]int, len(r.last))
	for k, v := range r.last {
		out[k] = v
	}
	return out
}

func (r *recorder) calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sawN
}

// Every service in a pipeline reports its own advance, ending at Done == Total. The pipeline used
// to aggregate this into a snapshot of its own; it no longer does, because the client's decoration
// reporter already carries the service name on every update.
func TestPipelineReportsEachServiceToCompletion(t *testing.T) {
	srv := httptest.NewServer(echoServer(""))
	defer srv.Close()

	rec := newRecorder()
	client := mustNew(t, Config{APIURL: srv.URL, ChunkSize: 5, Workers: 4})
	p := client.DecorationPipeline(ServiceVulnerabilities, ServiceLicenses, ServiceGeoprovenanceOrigin)

	purls := make([]string, 12) // 12 purls / chunk 5 -> 3 chunks per service
	for i := range purls {
		purls[i] = "pkg:test/c"
	}
	if _, err := p.Run(context.Background(), Components(purls...), WithDecorationReporter(rec)); err != nil {
		t.Fatalf("Run error: %v", err)
	}

	got := rec.snapshot()
	if len(got) != 3 {
		t.Fatalf("services reported = %d, want 3: %v", len(got), got)
	}
	for _, name := range []string{"vulnerabilities", "licenses", "geoprovenance.origin"} {
		dt, ok := got[name]
		if !ok {
			t.Errorf("%q never reported", name)
			continue
		}
		if dt[0] != 12 || dt[1] != 12 {
			t.Errorf("%s ended at %d/%d, want 12/12", name, dt[0], dt[1])
		}
	}
	if rec.calls() < 3 {
		t.Errorf("progress was reported %d times, want at least one per service", rec.calls())
	}
}
