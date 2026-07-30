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

	scanossapi "github.com/scanoss/scanoss.api-sdk"
)

// countingReporter counts Decorating calls, safely: services run concurrently.
type countingReporter struct {
	mu    sync.Mutex
	calls int
	last  string
}

func (r *countingReporter) Decorating(service string, done, total int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.last = service
}

func (r *countingReporter) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

// A reporter handed to a service method has to reach the request that method makes. The methods
// take the option and pass it down; a method that accepted it and dropped it on the floor would
// still compile, still return the right answer, and silently draw no progress at all — which is
// exactly what happened once, leaving every PURL command without its bar.
func TestServiceMethodsForwardTheirOptions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"components":[]}`))
	}))
	defer srv.Close()

	comps := make([]Component, 25) // 25 purls at chunk 10 -> 3 chunks
	for i := range comps {
		comps[i] = Component{Purl: "pkg:test/c"}
	}

	// One case per service surface a caller can reach with components in hand.
	for name, call := range map[string]func(c *Client, r DecorationReporter) error{
		"Vulnerabilities.Components": func(c *Client, r DecorationReporter) error {
			_, err := c.Vulnerabilities.Components(context.Background(), comps, WithDecorationReporter(r))
			return err
		},
		"Licenses.Components": func(c *Client, r DecorationReporter) error {
			_, err := c.Licenses.Components(context.Background(), comps, WithDecorationReporter(r))
			return err
		},
		"Cryptography.Algorithms": func(c *Client, r DecorationReporter) error {
			_, err := c.Cryptography.Algorithms(context.Background(), comps, WithDecorationReporter(r))
			return err
		},
		"Geoprovenance.Origins": func(c *Client, r DecorationReporter) error {
			_, err := c.Geoprovenance.Origins(context.Background(), comps, WithDecorationReporter(r))
			return err
		},
		"Components.Status": func(c *Client, r DecorationReporter) error {
			_, err := c.Components.Status(context.Background(), comps, WithDecorationReporter(r))
			return err
		},
		"Dependencies.Dependencies": func(c *Client, r DecorationReporter) error {
			_, err := c.Dependencies.Dependencies(context.Background(), comps, WithDecorationReporter(r))
			return err
		},
		"DecorationPipeline.Run": func(c *Client, r DecorationReporter) error {
			_, err := c.DecorationPipeline(ServiceLicenses).Run(context.Background(), comps, WithDecorationReporter(r))
			return err
		},
		"Vulnerabilities.Component (singular)": func(c *Client, r DecorationReporter) error {
			_, err := c.Vulnerabilities.Component(context.Background(), Component{Purl: "pkg:test/c"}, WithDecorationReporter(r))
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			rep := &countingReporter{}
			client := New(WithAPIURL(srv.URL), WithChunkSize(10), WithWorkers(3))
			if err := call(client, rep); err != nil {
				t.Fatalf("call returned error: %v", err)
			}
			if rep.count() == 0 {
				t.Error("the reporter was never called: this method takes a DecorateOption and does not pass it on")
			}
		})
	}
}

// The scan reporter has to reach the upload and the polling, whichever entry point started the
// scan. Scan.WFP is the one the pipeline and `scan wfp` use, and it lost its reporting once by
// nobody registering one.
func TestScanWFPReportsItsStages(t *testing.T) {
	var uploads int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			atomic.AddInt32(&uploads, 1)
			w.WriteHeader(http.StatusAccepted)
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"scan_id":"x","status":"completed","phase":"Pass 1: scan files","phase_done":1,"phase_total":1,"result":{"files":[],"components":{}}}`))
		}
	}))
	defer srv.Close()

	rep := &stageRecorder{}
	client := New(WithAPIURL(srv.URL))
	if _, err := client.Scan.WFP(context.Background(), []byte("file=abc,10,a.c\n"),
		WithScanReporter(rep), WithPollInterval(10_000_000)); err != nil {
		t.Fatalf("WFP: %v", err)
	}
	if !rep.uploaded() {
		t.Error("Uploading was never reported")
	}
	if !rep.scanned() {
		t.Error("Scanning was never reported")
	}
}

// stageRecorder notes which scan stages reported at all.
type stageRecorder struct {
	mu           sync.Mutex
	up, sc, fing bool
}

func (r *stageRecorder) Fingerprinting(done, total int) { r.mu.Lock(); r.fing = true; r.mu.Unlock() }
func (r *stageRecorder) Uploading(done, total int)      { r.mu.Lock(); r.up = true; r.mu.Unlock() }
func (r *stageRecorder) Scanning(env scanossapi.ScanEnvelope) {
	r.mu.Lock()
	r.sc = true
	r.mu.Unlock()
}

func (r *stageRecorder) uploaded() bool { r.mu.Lock(); defer r.mu.Unlock(); return r.up }
func (r *stageRecorder) scanned() bool  { r.mu.Lock(); defer r.mu.Unlock(); return r.sc }
