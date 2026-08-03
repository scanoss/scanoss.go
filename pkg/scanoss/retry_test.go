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
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func resp(status int, retryAfterHdr string) response {
	h := http.Header{}
	if retryAfterHdr != "" {
		h.Set("Retry-After", retryAfterHdr)
	}
	return response{StatusCode: status, Header: h}
}

func TestRetryAfterParse(t *testing.T) {
	future := time.Now().Add(time.Hour).UTC().Format(http.TimeFormat) // > clamp
	past := time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat)

	tests := []struct {
		name   string
		resp   response
		wantOK bool
		want   time.Duration
	}{
		{"429 seconds", resp(429, "2"), true, 2 * time.Second},
		{"503 seconds", resp(503, "1"), true, 1 * time.Second},
		{"non-429/503 ignored", resp(500, "2"), false, 0},
		{"missing header", resp(429, ""), false, 0},
		{"garbage header", resp(429, "soon"), false, 0},
		{"clamped to max", resp(429, "100000"), true, DefaultMaxServerRetryWait},
		{"negative → 0", resp(429, "-5"), true, 0},
		{"http-date future (clamped)", resp(503, future), true, DefaultMaxServerRetryWait},
		{"http-date past → 0", resp(429, past), true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, ok := retryAfter(tt.resp, DefaultMaxServerRetryWait)
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

	tr := &httpTransport{httpClient: srv.Client(), maxRetries: 3, maxServerRetryWait: DefaultMaxServerRetryWait}
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

	tr := &httpTransport{httpClient: srv.Client(), maxRetries: 3, maxServerRetryWait: DefaultMaxServerRetryWait}
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

	tr := &httpTransport{httpClient: srv.Client(), maxRetries: 2, maxServerRetryWait: DefaultMaxServerRetryWait}
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

	tr := &httpTransport{httpClient: srv.Client(), maxRetries: 3, maxServerRetryWait: DefaultMaxServerRetryWait}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()

	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	if _, err := tr.do(ctx, req); err == nil {
		t.Fatal("want ctx error while waiting on Retry-After")
	}
}

// failN is a RoundTripper that fails its first n calls with err and delegates the rest,
// standing in for a failure that never reaches a server — the DNS blip of issue #64.
type failN struct {
	n    int32
	err  error
	next http.RoundTripper
	hits atomic.Int32
}

func (f *failN) RoundTrip(req *http.Request) (*http.Response, error) {
	if f.hits.Add(1) <= f.n {
		return nil, f.err
	}
	return f.next.RoundTrip(req)
}

// dnsBlip is the error from the issue: a name that momentarily does not resolve.
func dnsBlip() error {
	return &net.OpError{Op: "dial", Net: "tcp", Err: &net.DNSError{
		Err: "no such host", Name: "api.scanoss.com", IsNotFound: true,
	}}
}

func TestTransportRetriesOnNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	rt := &failN{n: 2, err: dnsBlip(), next: http.DefaultTransport}
	tr := &httpTransport{
		httpClient: &http.Client{Transport: rt},
		maxRetries: 3, retryBackoffBase: time.Millisecond, maxServerRetryWait: DefaultMaxServerRetryWait,
	}
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	res, err := tr.do(context.Background(), req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if res.StatusCode != http.StatusOK || string(res.Body) != `{"ok":true}` {
		t.Fatalf("status=%d body=%s", res.StatusCode, res.Body)
	}
	if got := rt.hits.Load(); got != 3 { // two failures + the attempt that landed
		t.Fatalf("attempts = %d, want 3", got)
	}
}

func TestTransportNetworkRetriesExhausted(t *testing.T) {
	rt := &failN{n: 99, err: dnsBlip(), next: http.DefaultTransport}
	tr := &httpTransport{
		httpClient: &http.Client{Transport: rt},
		maxRetries: 2, retryBackoffBase: time.Millisecond, maxServerRetryWait: DefaultMaxServerRetryWait,
	}
	req, _ := http.NewRequest(http.MethodGet, "http://example.invalid", nil)
	if _, err := tr.do(context.Background(), req); err == nil {
		t.Fatal("want error once the retries are spent")
	}
	if got := rt.hits.Load(); got != 3 { // initial + 2 retries
		t.Fatalf("attempts = %d, want 3", got)
	}
}

func TestTransportReplaysBodyOnNetworkRetry(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		mu.Unlock()
	}))
	defer srv.Close()

	rt := &failN{n: 1, err: dnsBlip(), next: http.DefaultTransport}
	tr := &httpTransport{
		httpClient: &http.Client{Transport: rt},
		maxRetries: 3, retryBackoffBase: time.Millisecond, maxServerRetryWait: DefaultMaxServerRetryWait,
	}
	req, _ := http.NewRequest(http.MethodPost, srv.URL, bytes.NewReader([]byte("payload")))
	if _, err := tr.do(context.Background(), req); err != nil {
		t.Fatalf("do: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	// The first attempt never reached the server, so the one that did must carry the
	// whole body: a chunk replayed short would corrupt the WFP byte range.
	if len(bodies) != 1 || bodies[0] != "payload" {
		t.Fatalf("bodies = %v, want one \"payload\"", bodies)
	}
}

func TestTransportRetriesOn500WithoutRetryAfter(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	tr := &httpTransport{
		httpClient: srv.Client(),
		maxRetries: 3, retryBackoffBase: time.Millisecond, maxServerRetryWait: DefaultMaxServerRetryWait,
	}
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	res, err := tr.do(context.Background(), req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Fatalf("hits = %d, want 2 (one retry)", got)
	}
}

func TestTransportRetriesClientTimeout(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			time.Sleep(300 * time.Millisecond) // outlives the client's timeout
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	// A per-attempt timeout is not the caller giving up, so it is retried — unlike a
	// cancelled context (see TestRetryableError).
	client := srv.Client()
	client.Timeout = 50 * time.Millisecond
	tr := &httpTransport{
		httpClient: client,
		maxRetries: 3, retryBackoffBase: time.Millisecond, maxServerRetryWait: DefaultMaxServerRetryWait,
	}
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	res, err := tr.do(context.Background(), req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
}

func TestTransportNoRetryOnUntrustedCertificate(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	// Deliberately not srv.Client(): its cert is signed by an authority we do not trust,
	// which no number of attempts will change.
	rt := &failN{next: &http.Transport{}}
	tr := &httpTransport{
		httpClient: &http.Client{Transport: rt},
		maxRetries: 3, retryBackoffBase: time.Millisecond, maxServerRetryWait: DefaultMaxServerRetryWait,
	}
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	if _, err := tr.do(context.Background(), req); err == nil {
		t.Fatal("want a certificate error")
	}
	if got := rt.hits.Load(); got != 1 {
		t.Fatalf("attempts = %d, want 1 (no retry)", got)
	}
}

func TestRetryableError(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name string
		ctx  context.Context
		err  error
		want bool
	}{
		{"dns blip", context.Background(), &url.Error{Err: dnsBlip()}, true},
		{"connection closed", context.Background(), &url.Error{Err: io.EOF}, true},
		{"truncated body read", context.Background(), io.ErrUnexpectedEOF, true},
		{"caller cancelled", cancelled, &url.Error{Err: dnsBlip()}, false},
		{"bad scheme", context.Background(), &url.Error{Err: errors.New("unsupported protocol scheme \"ftp\"")}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := retryableError(tt.ctx, tt.err); got != tt.want {
				t.Fatalf("retryableError = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFailed(t *testing.T) {
	if failed(resp(200, ""), nil) || failed(resp(404, ""), nil) {
		t.Error("a 2xx and a 4xx are answers, not failures")
	}
	if !failed(resp(503, ""), nil) || !failed(response{}, dnsBlip()) {
		t.Error("a retryable status and a network error are failures")
	}
}

// failed gates every retry, so no status may carry a Retry-After the SDK would honor while
// failed() rejects it — that wait would be silently dropped.
func TestFailedCoversEveryHonoredWait(t *testing.T) {
	for code := 100; code < 600; code++ {
		res := resp(code, "1")
		if _, ok := retryAfter(res, DefaultMaxServerRetryWait); ok && !failed(res, nil) {
			t.Errorf("status %d carries an honored Retry-After but failed() rejects it", code)
		}
	}
}

func TestRetryableStatus(t *testing.T) {
	for _, code := range []int{429, 500, 502, 503, 504} {
		if !retryableStatus(code) {
			t.Errorf("status %d should be retryable", code)
		}
	}
	for _, code := range []int{200, 400, 401, 404, 422, 501, 505} {
		if retryableStatus(code) {
			t.Errorf("status %d should not be retryable", code)
		}
	}
}

func TestBackoff(t *testing.T) {
	const base = DefaultRetryBackoffBase // 250ms
	tests := []struct {
		name    string
		attempt int
		base    time.Duration
		want    time.Duration
	}{
		{"first wait is the base", 0, base, 250 * time.Millisecond},
		{"doubles per attempt", 1, base, 500 * time.Millisecond},
		{"fourth attempt", 4, base, 4 * time.Second},
		{"capped at maxRetryBackoff", 7, base, maxRetryBackoff},
		{"absurd attempt stays capped", 40, base, maxRetryBackoff},
		{"a smaller base shortens every wait", 3, 10 * time.Millisecond, 80 * time.Millisecond},
		{"no base, no wait", 3, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := backoff(tt.attempt, tt.base); got != tt.want {
				t.Fatalf("backoff = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWithRetryBackoffBase(t *testing.T) {
	if c := mustNew(t, Config{RetryBackoffBase: time.Second}); c.transport.retryBackoffBase != time.Second {
		t.Fatalf("retryBase = %v, want 1s", c.transport.retryBackoffBase)
	}
	if c := mustNew(t, Config{}); c.transport.retryBackoffBase != DefaultRetryBackoffBase {
		t.Fatalf("default retryBase = %v, want %v", c.transport.retryBackoffBase, DefaultRetryBackoffBase)
	}
	if c := mustNew(t, Config{RetryBackoffBase: 0}); c.transport.retryBackoffBase != DefaultRetryBackoffBase {
		t.Fatalf("d<=0 should be ignored, got %v", c.transport.retryBackoffBase)
	}
}

func TestMaxRetriesDisabled(t *testing.T) {
	// Negative disables retries outright, the convention Config.Timeout already uses;
	// zero still means "unset", so the default applies.
	if c := mustNew(t, Config{MaxRetries: -1}); c.transport.maxRetries != 0 {
		t.Fatalf("maxRetries = %d, want 0 (disabled)", c.transport.maxRetries)
	}

	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := mustNew(t, Config{APIURL: srv.URL, MaxRetries: -1})
	if _, err := c.get(context.Background(), "/x", nil); err == nil {
		t.Fatal("want the 500 to surface")
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("hits = %d, want 1 (retries disabled)", got)
	}
}

func TestWithMaxRetries(t *testing.T) {
	if c := mustNew(t, Config{MaxRetries: 9}); c.transport.maxRetries != 9 {
		t.Fatalf("maxRetries = %d, want 9", c.transport.maxRetries)
	}
	if c := mustNew(t, Config{}); c.transport.maxRetries != DefaultMaxRetries {
		t.Fatalf("default maxRetries = %d, want %d", c.transport.maxRetries, DefaultMaxRetries)
	}
	if c := mustNew(t, Config{MaxRetries: 0}); c.transport.maxRetries != DefaultMaxRetries {
		t.Fatalf("n<=0 should be ignored, got %d", c.transport.maxRetries)
	}
}

func TestWithMaxServerRetryWait(t *testing.T) {
	if c := mustNew(t, Config{MaxServerRetryWait: 30 * time.Second}); c.transport.maxServerRetryWait != 30*time.Second {
		t.Fatalf("maxServerRetryWait = %v, want 30s", c.transport.maxServerRetryWait)
	}
	if c := mustNew(t, Config{}); c.transport.maxServerRetryWait != DefaultMaxServerRetryWait {
		t.Fatalf("default maxServerRetryWait = %v, want %v", c.transport.maxServerRetryWait, DefaultMaxServerRetryWait)
	}
	if c := mustNew(t, Config{MaxServerRetryWait: 0}); c.transport.maxServerRetryWait != DefaultMaxServerRetryWait {
		t.Fatalf("d<=0 should be ignored, got %v", c.transport.maxServerRetryWait)
	}
}
