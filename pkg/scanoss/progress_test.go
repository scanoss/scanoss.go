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
	"testing"
)

func TestProgressCountsPurls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"components":[]}`))
	}))
	defer srv.Close()

	var mu sync.Mutex
	var events []Progress
	client := New(
		WithAPIURL(srv.URL),
		WithChunkSize(10),
		WithWorkers(3),
		WithProgress(func(p Progress) {
			mu.Lock()
			events = append(events, p)
			mu.Unlock()
		}),
	)

	purls := make([]string, 25) // 25 purls, chunk 10 -> 3 chunks
	for i := range purls {
		purls[i] = "pkg:test/c"
	}

	if _, err := client.decorate(context.Background(), ServiceVulnerabilities, Components(purls...)); err != nil {
		t.Fatalf("decorate returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 3 { // one per chunk
		t.Fatalf("expected 3 progress events, got %d", len(events))
	}
	last := events[len(events)-1]
	if last.Service != "vulnerabilities" {
		t.Errorf("Service = %q, want vulnerabilities", last.Service)
	}
	if last.Unit != "purls" {
		t.Errorf("Unit = %q, want purls", last.Unit)
	}
	if last.Total != 25 {
		t.Errorf("Total = %d, want 25", last.Total)
	}
	if last.Done != 25 { // all purls done by the final event
		t.Errorf("final Done = %d, want 25", last.Done)
	}
	// Done is monotonically non-decreasing and in purl units (multiples of chunk size here).
	prev := 0
	for _, e := range events {
		if e.Done < prev {
			t.Errorf("Done went backwards: %d after %d", e.Done, prev)
		}
		prev = e.Done
	}
}
