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
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// httpTransport is the HTTP implementation of the SDK's transport layer: it owns
// the http.Client and the API key, and executes a single request (attach ctx,
// apply auth, read body, check status). Client composes one of these as
// c.transport, and every request path (decoration, scan upload, status poll)
// funnels through do — so cross-cutting concerns (auth, status handling, retries)
// live here once. A future grpcTransport could sit beside it.
//
// Retry policy: maxRetries caps the retry COUNT (Config.MaxRetries); maxRetryAfter
// caps a single Retry-After WAIT (Config.MaxRetryAfter), so a pathological server
// value can't stall a call (<= 0 means no cap). The wait DURATION itself is the
// server's Retry-After value — the SDK obeys it, it does not compute its own
// backoff.
type httpTransport struct {
	httpClient    *http.Client
	apiKey        string
	maxRetries    int
	maxRetryAfter time.Duration
	log           *slog.Logger
}

// response is what the transport learned about a single API call. It is returned by
// value: small, never nil, so callers never have to nil-check it. Body is already
// fully read and the connection closed — owning the retry policy means owning the
// body, since a request cannot be replayed while its previous body is unread.
//
// Deliberately not an *http.Response: that type would carry a Body that is already
// closed, and it would tie this signature to HTTP (see the grpcTransport note above).
type response struct {
	Body       []byte
	StatusCode int
	// Header is here for the response metadata a caller may come to need — rate
	// limits, pagination, a correlation id for support. Nothing reads it yet.
	Header http.Header
}

// do attaches the context, applies authentication, executes the request and returns
// the fully read response.
//
// It reports what came back without judging it: any status, 4xx and 5xx included, is
// returned with a nil error. Whether a status counts as a failure is an API-level
// decision and belongs to Client.do. A non-nil error here means no response was
// obtained at all — the network failed, or the body could not be rewound or read.
//
// On 429 Too Many Requests or 503 Service Unavailable carrying a Retry-After
// header, it waits the server-indicated time (clamped to maxRetryAfter) and
// retries, up to maxRetries times. The request body is replayed via req.GetBody
// (set automatically by http.NewRequest for bytes/strings readers); a request
// with a body but no GetBody is not retried. The wait honors ctx cancellation.
func (t *httpTransport) do(ctx context.Context, req *http.Request) (response, error) {
	req = req.WithContext(ctx)
	logger := t.log
	if logger == nil {
		logger = slog.Default()
	}
	if t.apiKey != "" {
		req.Header.Set("x-api-key", t.apiKey)
		req.Header.Set("x-Session", t.apiKey)
	}

	for attempt := 0; ; attempt++ {
		// Replay the body the previous attempt consumed.
		if attempt > 0 && req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return response{}, fmt.Errorf("error rewinding request body: %w", err)
			}
			req.Body = body
		}

		start := time.Now()
		resp, err := t.httpClient.Do(req)
		if err != nil {
			logger.Debug("http request error", "method", req.Method, "url", req.URL.String(), "err", err)
			return response{}, fmt.Errorf("error making request: %w", err)
		}
		respBody, rerr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		logger.Debug("http request",
			"method", req.Method, "url", req.URL.String(),
			"status", resp.StatusCode, "bytes", len(respBody),
			"duration", time.Since(start).Round(time.Millisecond))
		result := response{Body: respBody, StatusCode: resp.StatusCode, Header: resp.Header}
		if rerr != nil {
			return result, fmt.Errorf("error reading response: %w", rerr)
		}

		// Server asked us to back off and we can replay → wait and retry.
		if d, ok := retryAfter(resp, t.maxRetryAfter); ok && attempt < t.maxRetries && replayable(req) {
			logger.Debug("server backoff; retrying", "wait", d, "attempt", attempt+1, "url", req.URL.String())
			if werr := sleepCtx(ctx, d); werr != nil {
				return result, werr
			}
			continue
		}

		return result, nil
	}
}

// replayable reports whether req can be re-sent: a body-less request (GET) always
// can; a request with a body needs GetBody to rewind it.
func replayable(req *http.Request) bool {
	return req.Body == nil || req.GetBody != nil
}

// retryAfter returns the wait duration when resp is a 429/503 carrying a parseable
// Retry-After header — delta-seconds or HTTP-date (RFC 7231) — clamped to maxWait
// (maxWait <= 0 means no cap). ok is false otherwise.
func retryAfter(resp *http.Response, maxWait time.Duration) (time.Duration, bool) {
	if resp.StatusCode != http.StatusTooManyRequests &&
		resp.StatusCode != http.StatusServiceUnavailable {
		return 0, false
	}
	v := strings.TrimSpace(resp.Header.Get("Retry-After"))
	if v == "" {
		return 0, false
	}
	// Form 1: delta-seconds, e.g. "120".
	if secs, err := strconv.Atoi(v); err == nil {
		if secs < 0 {
			secs = 0
		}
		return clampDelay(time.Duration(secs)*time.Second, maxWait), true
	}
	// Form 2: HTTP-date, e.g. "Wed, 21 Oct 2015 07:28:00 GMT".
	if ts, err := http.ParseTime(v); err == nil {
		d := time.Until(ts)
		if d < 0 {
			d = 0
		}
		return clampDelay(d, maxWait), true
	}
	return 0, false
}

func clampDelay(d, maxWait time.Duration) time.Duration {
	if maxWait > 0 && d > maxWait {
		return maxWait
	}
	return d
}

// sleepCtx waits for d or until ctx is cancelled, returning ctx.Err() if cancelled.
func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
