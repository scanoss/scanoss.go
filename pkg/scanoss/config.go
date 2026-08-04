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
	"net/http"
	"time"
)

// Defaults applied when the caller does not override them.
const (
	DefaultAPIURL     = "https://api.scanoss.com"
	DefaultChunkSize  = 10
	DefaultWorkers    = 5
	DefaultMaxRetries = 5
	// DefaultMaxServerRetryWait caps a single Retry-After wait. It is generous because it
	// bounds a wait the server asked for; the SDK's own waits are capped by maxRetryBackoff.
	DefaultMaxServerRetryWait = 5 * time.Minute
	// DefaultRetryBackoffBase is the first backoff wait; it doubles per attempt. With
	// DefaultMaxRetries that totals under 8s of waiting before a call gives up.
	DefaultRetryBackoffBase = 250 * time.Millisecond
	// DefaultTimeout bounds one request attempt, body transfer included. Sized for the
	// largest the SDK sends — a 1 MiB WFP chunk — over a poor link.
	DefaultTimeout = 120 * time.Second
)

// Config is the SDK's configuration. The zero value is the default configuration:
// every unset field falls back to its Default* constant.
type Config struct {
	// APIKey authenticates the requests. Empty means keyless, which the public
	// endpoint rejects and an on-prem one may allow.
	APIKey string
	// APIURL is the API base URL (default DefaultAPIURL). A trailing slash is trimmed.
	APIURL string

	// Proxy overrides HTTP_PROXY and HTTPS_PROXY for this client. It needs an http://
	// or https:// scheme; NO_PROXY still applies. Empty leaves Go's own environment
	// handling in place.
	Proxy string
	// CACertFile is a PEM file whose certificates are trusted in addition to the system
	// pool. Verification stays on: this adds an authority, it does not stop checking.
	CACertFile string
	// InsecureTLS disables certificate verification entirely. For self-signed or
	// internal endpoints only; prefer CACertFile, which keeps verification on.
	InsecureTLS bool
	// Timeout bounds one request attempt, body transfer included (default
	// DefaultTimeout). A negative value disables it. Retry-After waits happen between
	// attempts, so this does not cut them short.
	Timeout time.Duration

	// ChunkSize is the number of PURLs per decoration request (default
	// DefaultChunkSize), shared by every decoration service.
	ChunkSize int
	// Workers caps the concurrent requests (default DefaultWorkers). The effective
	// number is never larger than the number of chunks.
	Workers int
	// MaxRetries caps the retry count for a transient failure — a network error, a
	// truncated response, or a 429/5xx status (default DefaultMaxRetries). A negative
	// value disables retries; a request that fails once then fails the call.
	MaxRetries int
	// RetryBackoffBase is the first wait the SDK computes for itself, doubled per attempt
	// and capped internally (default DefaultRetryBackoffBase). A Retry-After the server
	// sent takes precedence over it.
	RetryBackoffBase time.Duration
	// MaxServerRetryWait caps a single Retry-After wait (default DefaultMaxServerRetryWait),
	// bounding a pathological server value. It does not bound RetryBackoffBase's waits.
	MaxServerRetryWait time.Duration
}

// httpClient builds the client for the transport. The SDK owns it: Config carries the
// settings, never a client of the caller's own.
func (cfg Config) httpClient() (*http.Client, error) {
	// With nothing to customise, leave Transport nil so the client shares
	// http.DefaultTransport, and with it one connection pool per process rather than a
	// private pool per client.
	if cfg.Proxy == "" && cfg.CACertFile == "" && !cfg.InsecureTLS {
		return &http.Client{Timeout: resolveTimeout(cfg.Timeout)}, nil
	}
	return newHTTPClient(httpClientOptions{
		Proxy:      cfg.Proxy,
		CACertFile: cfg.CACertFile,
		Insecure:   cfg.InsecureTLS,
		Timeout:    cfg.Timeout,
	})
}

// retryCount resolves Config.MaxRetries: unset takes the default, and a negative value
// disables retries — the convention Timeout already uses in this Config.
func retryCount(n int) int {
	if n < 0 {
		return 0
	}
	return positiveOr(n, DefaultMaxRetries)
}

// positiveOr returns value when it is above zero and fallback otherwise: how an unset
// Config field falls back to its default. A negative value is a typo, not a setting.
func positiveOr[T int | time.Duration](value, fallback T) T {
	if value > 0 {
		return value
	}
	return fallback
}
