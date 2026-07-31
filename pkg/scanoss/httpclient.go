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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"golang.org/x/net/http/httpproxy"
)

// HTTPClientOptions configures how the SDK reaches the API: through which proxy,
// and trusting which certificate authorities.
//
// The zero value is the default behaviour — the proxy comes from HTTP_PROXY,
// HTTPS_PROXY and NO_PROXY, and verification uses the system certificate pool.
type HTTPClientOptions struct {
	// Proxy is the proxy URL to use, overriding HTTP_PROXY and HTTPS_PROXY for this
	// client. It must carry an https:// or http:// scheme. NO_PROXY still applies, so
	// the hosts it exempts are reached directly. Empty leaves Go's own environment
	// handling in place.
	Proxy string

	// CACertFile is a PEM file whose certificates are trusted in addition to the
	// system pool — an internal CA, or the one an intercepting proxy signs with.
	// Verification stays on: this adds an authority, it does not stop checking.
	CACertFile string

	// Insecure disables certificate verification entirely. For self-signed or
	// internal endpoints only; prefer CACertFile, which keeps verification on.
	Insecure bool
}

// NewHTTPClient builds an *http.Client from opts, for use with WithHTTPClient:
//
//	hc, err := scanoss.NewHTTPClient(scanoss.HTTPClientOptions{
//		Proxy:      "http://proxy.example.com:8080",
//		CACertFile: "/etc/ssl/corp-ca.pem",
//	})
//	if err != nil {
//		return err
//	}
//	client := scanoss.New(scanoss.WithAPIKey(key), scanoss.WithHTTPClient(hc))
//
// Reading and parsing the PEM happen here, so a bad path is an error at
// construction rather than a handshake failure on the first request.
func NewHTTPClient(opts HTTPClientOptions) (*http.Client, error) {
	// Clone rather than construct. A zero-value http.Transport has a nil Proxy,
	// which means no proxy at all — not even the environment's — and it drops Go's
	// timeouts and connection pooling along with it.
	transport := http.DefaultTransport.(*http.Transport).Clone()

	if opts.Proxy != "" {
		proxy, err := proxyFuncFor(opts.Proxy)
		if err != nil {
			return nil, err
		}
		transport.Proxy = proxy
	}

	if opts.CACertFile != "" {
		pool, err := certPoolWith(opts.CACertFile)
		if err != nil {
			return nil, err
		}
		ensureTLSConfig(transport).RootCAs = pool
	}
	if opts.Insecure {
		ensureTLSConfig(transport).InsecureSkipVerify = true
	}

	return &http.Client{Transport: transport}, nil
}

// ensureTLSConfig returns transport's TLS config, creating it if absent. It never
// replaces an existing one: that would drop DefaultTransport's NextProtos, and HTTP/2
// with it.
func ensureTLSConfig(transport *http.Transport) *tls.Config {
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{}
	}
	return transport.TLSClientConfig
}

// parseProxy turns a proxy setting into a URL, insisting on a scheme.
//
// url.Parse accepts "proxy.example.com:8080" and reads the host as the scheme,
// leaving no host at all. That reaches the user as "proxyconnect tcp: dial tcp
// :0", which names neither the proxy nor what is missing, so it is refused here
// instead. Go is forgiving about this in the environment variables and that is
// left alone — an environment that works today keeps working.
func parseProxy(proxy string) (*url.URL, error) {
	lower := strings.ToLower(proxy)
	if !strings.HasPrefix(lower, "https://") && !strings.HasPrefix(lower, "http://") {
		return nil, fmt.Errorf("proxy must start with https:// or http:// (got %q)", proxy)
	}
	parsed, err := url.Parse(proxy)
	if err != nil {
		return nil, fmt.Errorf("parsing proxy %q: %w", proxy, err)
	}
	return parsed, nil
}

// proxyFuncFor routes requests through proxy, minus the hosts NO_PROXY exempts.
//
// The exemption is the reason this is not http.ProxyURL: that returns the same URL for
// every request and cannot consult NO_PROXY, so an explicit proxy would also capture
// the internal hosts a split network deliberately reaches directly. httpproxy is what
// net/http itself uses for ProxyFromEnvironment, so the matching rules — suffixes,
// wildcards, ports, CIDR blocks, and the loopback exemption — are the same ones that
// apply when the proxy comes from the environment.
func proxyFuncFor(proxy string) (func(*http.Request) (*url.URL, error), error) {
	// Validated here, though httpproxy parses the string again: it is forgiving about a
	// missing scheme and would silently accept what parseProxy exists to reject.
	if _, err := parseProxy(proxy); err != nil {
		return nil, err
	}
	// Both spellings, as Go accepts both.
	noProxy := os.Getenv("NO_PROXY")
	if noProxy == "" {
		noProxy = os.Getenv("no_proxy")
	}
	// Both fields carry the same proxy: one setting covers the API whichever scheme it
	// uses. The environment is sampled here rather than per request, matching how Go
	// samples it once.
	cfg := httpproxy.Config{HTTPProxy: proxy, HTTPSProxy: proxy, NoProxy: noProxy}
	proxyFor := cfg.ProxyFunc()
	return func(req *http.Request) (*url.URL, error) { return proxyFor(req.URL) }, nil
}

// certPoolWith returns the system pool plus the certificates in path.
//
// Added to the system pool rather than replacing it: a client configured with an
// internal CA must still verify the public API, whose certificate a public
// authority signed.
func certPoolWith(path string) (*x509.CertPool, error) {
	pemData, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading CA certificate %s: %w", path, err)
	}
	pool, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("loading the system certificate pool: %w", err)
	}
	// AppendCertsFromPEM reports whether it added anything. False means the file
	// held no certificate — plain text, a private key, a truncated download — so
	// the setting would silently do nothing and the run would fail later with the
	// very "unknown authority" error it was given to avoid.
	if !pool.AppendCertsFromPEM(pemData) {
		return nil, fmt.Errorf("CA certificate %s contains no certificate", path)
	}
	return pool, nil
}
