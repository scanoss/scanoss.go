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
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func resp(status int, retryAfterHdr string) *http.Response {
	h := http.Header{}
	if retryAfterHdr != "" {
		h.Set("Retry-After", retryAfterHdr)
	}
	return &http.Response{StatusCode: status, Header: h}
}

func TestRetryAfterParse(t *testing.T) {
	future := time.Now().Add(time.Hour).UTC().Format(http.TimeFormat) // > clamp
	past := time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat)

	tests := []struct {
		name   string
		resp   *http.Response
		wantOK bool
		want   time.Duration
	}{
		{"429 seconds", resp(429, "2"), true, 2 * time.Second},
		{"503 seconds", resp(503, "1"), true, 1 * time.Second},
		{"non-429/503 ignored", resp(500, "2"), false, 0},
		{"missing header", resp(429, ""), false, 0},
		{"garbage header", resp(429, "soon"), false, 0},
		{"clamped to max", resp(429, "100000"), true, DefaultMaxRetryAfter},
		{"negative → 0", resp(429, "-5"), true, 0},
		{"http-date future (clamped)", resp(503, future), true, DefaultMaxRetryAfter},
		{"http-date past → 0", resp(429, past), true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, ok := retryAfter(tt.resp, DefaultMaxRetryAfter)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && d != tt.want {
				t.Fatalf("delay = %v, want %v", d, tt.want)
			}
		})
	}
}

func TestTransportRetriesOn429(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	tr := &httpTransport{httpClient: srv.Client(), maxRetries: 3, maxRetryAfter: DefaultMaxRetryAfter}
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	start := time.Now()
	res, err := tr.do(context.Background(), req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if res.StatusCode != http.StatusOK || string(res.Body) != `{"ok":true}` {
		t.Fatalf("status=%d body=%s", res.StatusCode, res.Body)
	}
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Fatalf("hits = %d, want 2 (one retry)", got)
	}
	if time.Since(start) < time.Second {
		t.Fatalf("did not wait the Retry-After interval")
	}
}

func TestTransportReplaysBodyOnRetry(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		mu.Unlock()
		if atomic.AddInt32(&hits, 1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tr := &httpTransport{httpClient: srv.Client(), maxRetries: 3, maxRetryAfter: DefaultMaxRetryAfter}
	req, _ := http.NewRequest(http.MethodPost, srv.URL, bytes.NewReader([]byte("payload")))
	if _, err := tr.do(context.Background(), req); err != nil {
		t.Fatalf("do: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 || bodies[0] != "payload" || bodies[1] != "payload" {
		t.Fatalf("bodies = %v, want both \"payload\"", bodies)
	}
}

func TestTransportMaxRetriesExhausted(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	tr := &httpTransport{httpClient: srv.Client(), maxRetries: 2, maxRetryAfter: DefaultMaxRetryAfter}
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	res, err := tr.do(context.Background(), req)
	// The transport hands back the 429 it ended up with and no error: turning a status
	// into a failure is Client.do's decision, not the transport's.
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("want last 429 response, got status %d", res.StatusCode)
	}
	if got := atomic.LoadInt32(&hits); got != 3 { // initial + 2 retries
		t.Fatalf("hits = %d, want 3", got)
	}
}

func TestTransportCtxCancelDuringWait(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "60") // long wait
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	tr := &httpTransport{httpClient: srv.Client(), maxRetries: 3, maxRetryAfter: DefaultMaxRetryAfter}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()

	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	if _, err := tr.do(ctx, req); err == nil {
		t.Fatal("want ctx error while waiting on Retry-After")
	}
}

func TestWithMaxRetries(t *testing.T) {
	if c := New(WithMaxRetries(9)); c.transport.maxRetries != 9 {
		t.Fatalf("maxRetries = %d, want 9", c.transport.maxRetries)
	}
	if c := New(); c.transport.maxRetries != DefaultMaxRetries {
		t.Fatalf("default maxRetries = %d, want %d", c.transport.maxRetries, DefaultMaxRetries)
	}
	if c := New(WithMaxRetries(0)); c.transport.maxRetries != DefaultMaxRetries {
		t.Fatalf("n<=0 should be ignored, got %d", c.transport.maxRetries)
	}
}

func TestWithMaxRetryAfter(t *testing.T) {
	if c := New(WithMaxRetryAfter(30 * time.Second)); c.transport.maxRetryAfter != 30*time.Second {
		t.Fatalf("maxRetryAfter = %v, want 30s", c.transport.maxRetryAfter)
	}
	if c := New(); c.transport.maxRetryAfter != DefaultMaxRetryAfter {
		t.Fatalf("default maxRetryAfter = %v, want %v", c.transport.maxRetryAfter, DefaultMaxRetryAfter)
	}
	if c := New(WithMaxRetryAfter(0)); c.transport.maxRetryAfter != DefaultMaxRetryAfter {
		t.Fatalf("d<=0 should be ignored, got %v", c.transport.maxRetryAfter)
	}
}
