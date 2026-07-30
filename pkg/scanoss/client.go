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

// Package scanoss is a Go SDK for the SCANOSS component services
// (cryptography, vulnerabilities, licenses, geoprovenance).
//
// Each decoration is a grouped service on the Client; chunking the request into
// batches and querying those batches concurrently with a pool of workers is
// handled internally and is transparent to the caller:
//
//	client := scanoss.New(scanoss.WithAPIKey(key))
//	res, err := client.Vulnerabilities.Components(ctx, comps) // batch
//	res, err := client.Vulnerabilities.Component(ctx, comp)   // single
//	res, err := client.Licenses.Components(ctx, comps)
//	res, err := client.Cryptography.Algorithms(ctx, comps)
//	res, err := client.Geoprovenance.Origins(ctx, comps)
//
// Tune chunk size and concurrency at construction time with WithChunkSize and
// WithWorkers; everything else stays the same.
//
// The client also exposes a batch scan service that uploads WFP fingerprints and
// returns match results:
//
//	res, err := client.Scan.WFP(ctx, wfp) // upload (parallel chunks) + poll
//
// Tune the upload block size per call with the WithChunkBytes scan option;
// register WithScanIDNotify to capture the client-generated scan id for optional
// recovery via Scan.Wait.
package scanoss

import (
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Defaults applied when the caller does not override them.
const (
	DefaultAPIURL     = "https://api.scanoss.com"
	DefaultChunkSize  = 10
	DefaultWorkers    = 5
	DefaultMaxRetries = 5
)

// DefaultMaxRetryAfter caps a single Retry-After wait (used when WithMaxRetryAfter
// is not set).
const DefaultMaxRetryAfter = 5 * time.Minute

// Client is the SCANOSS SDK entry point. Create one with New and reuse it; it
// is safe for concurrent use.
type Client struct {
	apiURL string
	// transport is the SDK's HTTP transport layer (http client + auth + retries).
	// The Client composes it rather than being one — see transport.go.
	transport *httpTransport
	// chunkSize is the number of PURLs per decoration request (WithChunkSize),
	// shared by every decoration service.
	chunkSize int
	workers   int
	onScanID  func(string)
	// log receives the SDK's diagnostic logging (WithLogger); defaults to
	// slog.Default(). The SDK never writes to stdout, only through this logger.
	log *slog.Logger

	// Decoration services (grouped public API). Wired in New.
	Vulnerabilities VulnerabilityAPI
	Licenses        LicenseAPI
	Cryptography    CryptographyAPI
	Geoprovenance   GeoprovenanceAPI
	Copyright       CopyrightAPI
	Components      ComponentsAPI
	Dependencies    DependencyAPI

	// Scan service (batch WFP scanning). Wired in New.
	Scan ScanAPI
}

// Option configures a Client.
type Option func(*Client)

// WithAPIURL overrides the API base URL (default DefaultAPIURL).
func WithAPIURL(url string) Option {
	return func(c *Client) {
		if url != "" {
			c.apiURL = strings.TrimRight(url, "/")
		}
	}
}

// WithAPIKey sets the API key used for authentication.
func WithAPIKey(key string) Option { return func(c *Client) { c.transport.apiKey = key } }

// WithChunkSize sets the number of PURLs sent per request (default DefaultChunkSize).
func WithChunkSize(n int) Option {
	return func(c *Client) {
		if n > 0 {
			c.chunkSize = n
		}
	}
}

// WithWorkers sets the maximum number of concurrent requests (default DefaultWorkers).
// The effective number of workers is never larger than the number of chunks.
func WithWorkers(n int) Option {
	return func(c *Client) {
		if n > 0 {
			c.workers = n
		}
	}
}

// WithHTTPClient supplies a custom *http.Client (for timeouts, proxies, etc.).
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			c.transport.httpClient = h
		}
	}
}

// WithInsecureTLS disables TLS certificate verification when insecure is true.
// For self-signed or internal endpoints only — insecure, avoid in production.
func WithInsecureTLS(insecure bool) Option {
	return func(c *Client) {
		if !insecure {
			return
		}
		// Built through NewHTTPClient rather than by hand. A hand-built
		// http.Transport has a nil Proxy, which silently opts out of HTTP_PROXY and
		// HTTPS_PROXY — so this option used to disable proxy support as a side
		// effect. NewHTTPClient clones http.DefaultTransport and keeps it.
		//
		// The error cannot happen here: it only comes from a proxy or CA file, and
		// neither is set. Callers who want those use NewHTTPClient with
		// WithHTTPClient, where the error is theirs to see.
		client, err := NewHTTPClient(HTTPClientOptions{Insecure: true})
		if err != nil {
			return
		}
		c.transport.httpClient = client
	}
}

// WithMaxRetries sets how many times a request is retried when the server returns
// 429/503 with a Retry-After header (default DefaultMaxRetries). The wait between
// retries is the server's Retry-After value, not a client-chosen backoff. Values
// <= 0 are ignored.
func WithMaxRetries(n int) Option {
	return func(c *Client) {
		if n > 0 {
			c.transport.maxRetries = n
		}
	}
}

// WithMaxRetryAfter caps how long a single Retry-After wait may be (default
// DefaultMaxRetryAfter), bounding a pathological server value. Values <= 0 are
// ignored.
func WithMaxRetryAfter(d time.Duration) Option {
	return func(c *Client) {
		if d > 0 {
			c.transport.maxRetryAfter = d
		}
	}
}

// WithLogger sets the structured logger the SDK uses for diagnostics (HTTP
// requests, scan flow, decoration). Defaults to slog.Default(). Diagnostics are
// emitted at Debug/Info/Warn; nothing is written to stdout. Pass a logger whose
// handler level you control to capture or silence SDK logs independently.
func WithLogger(l *slog.Logger) Option {
	return func(c *Client) {
		if l != nil {
			c.log = l
		}
	}
}

// WithScanIDNotify registers an optional callback invoked once, after the full
// WFP has been uploaded and before polling begins — at which point the id is
// resumable via Scan.Wait. It lets a caller record the id so an interrupted scan
// can be resumed later. It is optional; recovery is not required for a normal scan.
func WithScanIDNotify(fn func(scanID string)) Option {
	return func(c *Client) { c.onScanID = fn }
}

// New creates a Client with the given options.
func New(opts ...Option) *Client {
	c := &Client{
		apiURL:    DefaultAPIURL,
		chunkSize: DefaultChunkSize,
		workers:   DefaultWorkers,
		log:       slog.Default(),
		transport: &httpTransport{
			httpClient:    &http.Client{},
			maxRetries:    DefaultMaxRetries,
			maxRetryAfter: DefaultMaxRetryAfter,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	// Propagate the (possibly overridden) logger to the transport.
	c.transport.log = c.log
	// Wire the per-service handles to this client.
	c.Vulnerabilities = vulnerabilityService{c}
	c.Licenses = licenseService{c}
	c.Cryptography = cryptographyService{c}
	c.Geoprovenance = geoprovenanceService{c}
	c.Copyright = copyrightService{c}
	c.Components = componentsService{c}
	c.Dependencies = dependencyService{c}
	c.Scan = scanService{c}
	return c
}
