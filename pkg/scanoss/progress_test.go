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

// Decoration progress is counted in PURLs, not in the chunks the request is split into, and it
// only ever grows: a consumer drawing a bar from it never sees it move backwards.
func TestDecorationProgressCountsPurls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"components":[]}`))
	}))
	defer srv.Close()

	var mu sync.Mutex
	type update struct {
		service     string
		done, total int
	}
	var updates []update

	client := mustNew(t, Config{APIURL: srv.URL, ChunkSize: 10, Workers: 3})

	purls := make([]string, 25) // 25 purls, chunk 10 -> 3 chunks
	for i := range purls {
		purls[i] = "pkg:test/c"
	}
	if _, err := client.decorate(context.Background(), ServiceVulnerabilities, Components(purls...),
		WithDecorationReporter(decorationFunc(func(service string, done, total int) {
			mu.Lock()
			updates = append(updates, update{service, done, total})
			mu.Unlock()
		}))); err != nil {
		t.Fatalf("decorate returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(updates) != 3 { // one per chunk
		t.Fatalf("expected 3 updates, got %d", len(updates))
	}
	last := updates[len(updates)-1]
	if last.service != "vulnerabilities" {
		t.Errorf("service = %q, want vulnerabilities", last.service)
	}
	if last.total != 25 {
		t.Errorf("total = %d, want 25 (purls, not chunks)", last.total)
	}
	if last.done != 25 {
		t.Errorf("final done = %d, want 25", last.done)
	}
	prev := 0
	for _, u := range updates {
		if u.done < prev {
			t.Errorf("done went backwards: %d after %d", u.done, prev)
		}
		prev = u.done
	}
}

// decorationFunc adapts a func to DecorationReporter, for tests that want a closure.
type decorationFunc func(service string, done, total int)

func (f decorationFunc) Decorating(service string, done, total int) { f(service, done, total) }
