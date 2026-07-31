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
//	client, err := scanoss.New(scanoss.Config{APIKey: key})
//	res, err := client.Vulnerabilities.Components(ctx, comps) // batch
//	res, err := client.Vulnerabilities.Component(ctx, comp)   // single
//	res, err := client.Licenses.Components(ctx, comps)
//	res, err := client.Cryptography.Algorithms(ctx, comps)
//	res, err := client.Geoprovenance.Origins(ctx, comps)
//
// New is the only way to build a Client. Everything client-wide — credentials,
// endpoint, proxy and TLS, concurrency, retries — is a field of Config, whose zero
// value is the default configuration.
//
// The client also exposes a batch scan service that uploads WFP fingerprints and
// returns match results:
//
//	res, err := client.Scan.WFP(ctx, wfp) // upload (parallel chunks) + poll
//
// Per-call tuning stays a functional option: WithChunkBytes for the upload block
// size, WithScanReporter for progress. Config.OnScanID captures the
// client-generated scan id for optional recovery via Scan.Wait.
package scanoss

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Defaults applied when the caller does not override them.
const (
	DefaultAPIURL     = "https://api.scanoss.com"
	DefaultChunkSize  = 10
	DefaultWorkers    = 5
	DefaultMaxRetries = 5
	// DefaultMaxRetryAfter caps a single Retry-After wait.
	DefaultMaxRetryAfter = 5 * time.Minute
	// DefaultTimeout bounds one request attempt, body transfer included. Sized for the
	// largest the SDK sends — a 1 MiB WFP chunk — over a poor link.
	DefaultTimeout = 120 * time.Second
)

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
	// HTTPClient replaces the SDK's own client wholesale, for a caller that needs
	// transport behaviour the fields above do not cover. It takes precedence over
	// Proxy, CACertFile and InsecureTLS.
	HTTPClient *http.Client

	// ChunkSize is the number of PURLs per decoration request (default
	// DefaultChunkSize), shared by every decoration service.
	ChunkSize int
	// Workers caps the concurrent requests (default DefaultWorkers). The effective
	// number is never larger than the number of chunks.
	Workers int
	// MaxRetries caps the retry count when the server answers 429/503 with a
	// Retry-After header (default DefaultMaxRetries).
	MaxRetries int
	// MaxRetryAfter caps a single Retry-After wait (default DefaultMaxRetryAfter),
	// bounding a pathological server value.
	MaxRetryAfter time.Duration

	// Logger receives the SDK's diagnostics, at Debug/Info/Warn (default
	// slog.Default()). The SDK never writes to stdout, only through this logger.
	Logger *slog.Logger
	// OnScanID is called once with the client-generated scan id, after the full WFP has
	// been uploaded and before polling begins — the point from which Scan.Wait can
	// resume it. Optional; a normal scan needs no recovery.
	OnScanID func(scanID string)
}

// httpClient returns the caller's HTTPClient, or builds one from the transport fields.
func (cfg Config) httpClient() (*http.Client, error) {
	if cfg.HTTPClient != nil {
		return cfg.HTTPClient, nil
	}
	// With nothing to customise, leave Transport nil so the client shares
	// http.DefaultTransport, and with it one connection pool per process rather than a
	// private pool per client.
	if cfg.Proxy == "" && cfg.CACertFile == "" && !cfg.InsecureTLS {
		return &http.Client{Timeout: resolveTimeout(cfg.Timeout)}, nil
	}
	return NewHTTPClient(HTTPClientOptions{
		Proxy:      cfg.Proxy,
		CACertFile: cfg.CACertFile,
		Insecure:   cfg.InsecureTLS,
		Timeout:    cfg.Timeout,
	})
}

// positiveOr returns value when it is above zero and fallback otherwise: how an unset
// Config field falls back to its default. A negative value is a typo, not a setting.
func positiveOr[T int | time.Duration](value, fallback T) T {
	if value > 0 {
		return value
	}
	return fallback
}

// New creates a Client from cfg. It reports a configuration it cannot apply — an
// unreadable CA file, a proxy without a scheme — so a bad setting fails here instead of
// on the first request.
func New(cfg Config) (*Client, error) {
	httpClient, err := cfg.httpClient()
	if err != nil {
		return nil, err
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	apiURL := DefaultAPIURL
	if cfg.APIURL != "" {
		apiURL = strings.TrimRight(cfg.APIURL, "/")
	}

	c := &Client{
		apiURL:    apiURL,
		chunkSize: positiveOr(cfg.ChunkSize, DefaultChunkSize),
		workers:   positiveOr(cfg.Workers, DefaultWorkers),
		onScanID:  cfg.OnScanID,
		log:       logger,
		transport: &httpTransport{
			httpClient:    httpClient,
			apiKey:        cfg.APIKey,
			maxRetries:    positiveOr(cfg.MaxRetries, DefaultMaxRetries),
			maxRetryAfter: positiveOr(cfg.MaxRetryAfter, DefaultMaxRetryAfter),
			log:           logger,
		},
	}
	// Wire the per-service handles to this client.
	c.Vulnerabilities = vulnerabilityService{c}
	c.Licenses = licenseService{c}
	c.Cryptography = cryptographyService{c}
	c.Geoprovenance = geoprovenanceService{c}
	c.Copyright = copyrightService{c}
	c.Components = componentsService{c}
	c.Dependencies = dependencyService{c}
	c.Scan = scanService{c}
	return c, nil
}

// newRequest builds a request against endpoint, resolved relative to the API base
// URL. Every request the SDK sends is built here, so the base URL is joined — and the
// failure wrapped — in one place.
func (c *Client) newRequest(method, endpoint string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, c.apiURL+endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	return req, nil
}

// do sends req through the transport and turns an unacceptable status into an error.
// Any 2xx is success — restricting this to 200/202 used to turn a 201 Created or a
// 204 No Content into a failure.
//
// Deciding which statuses are failures is an API-level call, so it is made here rather
// than in the transport, which only reports what came back. Every response the SDK
// receives passes through this method.
func (c *Client) do(ctx context.Context, req *http.Request) (response, error) {
	res, err := c.transport.do(ctx, req)
	if err != nil {
		return res, err
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return res, &StatusError{StatusCode: res.StatusCode, Body: string(res.Body)}
	}
	return res, nil
}

// get issues a GET to endpoint with the given query parameters and returns the raw
// response body.
//
// err alone tells the caller the call failed, status included — see do. The body comes
// back even then, so the caller can report what the server said.
func (c *Client) get(ctx context.Context, endpoint string, query url.Values) ([]byte, error) {
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := c.newRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	res, err := c.do(ctx, req)
	return res.Body, err
}

// postJSON sends payload as the JSON body of a POST to endpoint and returns the raw
// response body. Status handling and the body-on-error rule are the same as in get.
func (c *Client) postJSON(ctx context.Context, endpoint string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request body: %w", err)
	}
	req, err := c.newRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.do(ctx, req)
	return res.Body, err
}
