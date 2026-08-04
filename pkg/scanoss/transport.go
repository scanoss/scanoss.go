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
	"errors"
	"fmt"
	"github.com/scanoss/scanoss.go/internal/logging"
	"io"
	"net"
	"net/http"
	"net/url"
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
// Retry policy: an attempt is repeated when the failure looks transient — a network
// error, a truncated body, or a status the server could not rather than would not serve.
// maxRetries caps the retry COUNT (Config.MaxRetries, 0 disables). There are two waits,
// each with its own ceiling: the server's Retry-After when it sent one, capped by
// maxServerRetryWait, and retryBackoffBase doubled per attempt otherwise, capped by
// maxRetryBackoff.
type httpTransport struct {
	httpClient         *http.Client
	apiKey             string
	maxRetries         int
	retryBackoffBase   time.Duration
	maxServerRetryWait time.Duration
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
	// Header carries the response metadata: retryAfter reads Retry-After from it, and it
	// is where the rest — rate limits, pagination, a correlation id — would come from.
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
// Transient failures are retried up to maxRetries times: network errors, a response
// body that ends early, and the statuses retryableStatus accepts. The request body is
// replayed via req.GetBody (set automatically by http.NewRequest for bytes/strings
// readers); a request with a body but no GetBody is not retried. Waits honor ctx
// cancellation.
func (t *httpTransport) do(ctx context.Context, req *http.Request) (response, error) {
	req = t.prepare(ctx, req)

	for attempt := 0; ; attempt++ {
		res, err := t.send(req, attempt)
		if !failed(res, err) {
			return res, err
		}
		wait, retry := t.retryDelay(ctx, req, attempt, res, err)
		if !retry {
			return res, err
		}
		logging.Debug("retrying", "wait", wait, "attempt", attempt+2,
			"status", res.StatusCode, "url", req.URL.String())
		if werr := sleepCtx(ctx, wait); werr != nil {
			return res, werr
		}
		if rerr := rewindBody(req); rerr != nil {
			return res, rerr
		}
	}
}

// prepare attaches ctx and authentication. Every attempt re-sends the same request, so
// this runs once per call rather than once per attempt.
func (t *httpTransport) prepare(ctx context.Context, req *http.Request) *http.Request {
	req = req.WithContext(ctx)
	if t.apiKey != "" {
		req.Header.Set("x-api-key", t.apiKey)
		req.Header.Set("x-Session", t.apiKey)
	}
	return req
}

// send makes one attempt and reads the response to completion. A non-nil error means no
// usable response came back: the request never landed, or its body ended early. Deciding
// what to do about that belongs to retryDelay.
func (t *httpTransport) send(req *http.Request, attempt int) (response, error) {
	start := time.Now()

	resp, err := t.httpClient.Do(req)
	if err != nil {
		logging.Debug("http request error", "method", req.Method, "url", req.URL.String(),
			"attempt", attempt+1, "err", err)
		return response{}, fmt.Errorf("error making request: %w", err)
	}

	body, rerr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	logging.Debug("http request",
		"method", req.Method, "url", req.URL.String(),
		"status", resp.StatusCode, "bytes", len(body),
		"attempt", attempt+1,
		"duration", time.Since(start).Round(time.Millisecond))

	res := response{Body: body, StatusCode: resp.StatusCode, Header: resp.Header}
	if rerr != nil {
		return res, fmt.Errorf("error reading response: %w", rerr)
	}
	return res, nil
}

// failed reports whether an attempt went wrong at all. Anything else is the answer, and
// do returns it: a 2xx, or the 4xx that no number of retries would change.
//
// retryAfter only ever fires on the statuses retryableStatus accepts, so a response this
// rejects can carry no wait the SDK would honor.
func failed(res response, err error) bool {
	return err != nil || retryableStatus(res.StatusCode)
}

// retryDelay returns how long to wait before re-sending req, and whether to re-send it at
// all. It answers only for an attempt that failed — do gates on that, so this is the retry
// policy and nothing else.
func (t *httpTransport) retryDelay(ctx context.Context, req *http.Request, attempt int, res response, err error) (time.Duration, bool) {
	// Out of budget, or a request we cannot re-send at all.
	if attempt >= t.maxRetries || !replayable(req) {
		return 0, false
	}
	// No usable response: a network failure, or a body that ended early. Either must not
	// discard the work that got us here — on a chunked scan upload, failing now would
	// throw away every chunk the server already accepted.
	if err != nil {
		if !retryableError(ctx, err) {
			return 0, false
		}
		return backoff(attempt, t.retryBackoffBase), true
	}
	// A status the server could not serve. Its own Retry-After wins: it knows its load
	// better than we do, and maxServerRetryWait is the cap that belongs to it.
	if d, ok := retryAfter(res, t.maxServerRetryWait); ok {
		return d, true
	}
	return backoff(attempt, t.retryBackoffBase), true
}

// rewindBody restores the body the attempt just made consumed, so the next one can send it
// again. Called only once a retry is committed, which is why it needs no attempt counter.
// A request whose body cannot be rewound never reaches here — replayable keeps it out of
// the retry; a body-less GET has nothing to restore.
func rewindBody(req *http.Request) error {
	if req.GetBody == nil {
		return nil
	}
	body, err := req.GetBody()
	if err != nil {
		return fmt.Errorf("error rewinding request body: %w", err)
	}
	req.Body = body
	return nil
}

// replayable reports whether req can be re-sent: a body-less request (GET) always
// can; a request with a body needs GetBody to rewind it.
func replayable(req *http.Request) bool {
	return req.Body == nil || req.GetBody != nil
}

// retryAfter returns the wait duration when res is a 429/503 carrying a parseable
// Retry-After header — delta-seconds or HTTP-date (RFC 7231) — clamped to maxWait
// (maxWait <= 0 means no cap). ok is false otherwise.
func retryAfter(res response, maxWait time.Duration) (time.Duration, bool) {
	if res.StatusCode != http.StatusTooManyRequests &&
		res.StatusCode != http.StatusServiceUnavailable {
		return 0, false
	}
	v := strings.TrimSpace(res.Header.Get("Retry-After"))
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

// retryableStatus reports whether code means the server was unable rather than
// unwilling. 501 and 505 are excluded — a permanent "won't" — as is every other 4xx,
// where replaying the same request earns the same answer.
func retryableStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests, http.StatusInternalServerError,
		http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

// retryableError reports whether a failed attempt is worth repeating. It classifies by
// exclusion: nearly everything http.Client.Do returns is a *url.Error satisfying
// net.Error, so an allowlist would have missed the DNS blip this exists for.
func retryableError(ctx context.Context, err error) bool {
	// The caller gave up. A per-attempt Config.Timeout also surfaces as DeadlineExceeded,
	// so ask ctx — not the error — who cancelled.
	if ctx.Err() != nil {
		return false
	}
	// Anything else Do failed at is wrapped in a url.Error; a cause that is not
	// network-level is a request we built wrong, and no attempt will fix it.
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return transientCause(urlErr.Err)
	}
	return true
}

// transientCause reports whether a url.Error's cause is a network-level failure (dial,
// reset, timeout) rather than a bad scheme, a missing host, a redirect loop or a
// certificate we refused to trust.
func transientCause(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	// A server that closed the connection mid-flight. Neither is a net.Error.
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

// maxRetryBackoff caps how long the SDK waits before re-sending a request on its own
// initiative. It is separate from maxServerRetryWait, which caps a wait the server asked for
// and is loose (5 minutes) because obeying such a request is legitimate; parking a
// request that long on our own judgement is not.
const maxRetryBackoff = 30 * time.Second

// backoff returns the wait after the attempt that just failed: base doubled per prior
// attempt, capped at maxRetryBackoff. The shift is bounded because Duration is an int64 of
// nanoseconds — past ~60 doublings it wraps negative, and a negative wait is worse than
// a long one.
func backoff(attempt int, base time.Duration) time.Duration {
	return clampDelay(base<<min(attempt, 20), maxRetryBackoff)
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
