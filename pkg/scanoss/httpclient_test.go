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
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// transportOf returns the *http.Transport a client was built with.
func transportOf(t *testing.T, c *http.Client) *http.Transport {
	t.Helper()
	transport, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport = %T, want *http.Transport", c.Transport)
	}
	return transport
}

// usesEnvironmentProxy reports whether the transport still resolves its proxy
// through the environment.
//
// It compares function identity rather than setting HTTP(S)_PROXY and making a
// request, because http.ProxyFromEnvironment reads the environment once per
// process and caches it — whichever test ran first would decide the answer for
// every other one. Identity asks the question this actually cares about: did we
// leave Go's environment handling in place, or replace it?
func usesEnvironmentProxy(transport *http.Transport) bool {
	if transport.Proxy == nil {
		return false
	}
	return reflect.ValueOf(transport.Proxy).Pointer() == reflect.ValueOf(http.ProxyFromEnvironment).Pointer()
}

// writePEM writes data to a file in the test's temp dir and returns the path.
func writePEM(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestNewHTTPClientDefaults(t *testing.T) {
	client, err := newHTTPClient(httpClientOptions{})
	if err != nil {
		t.Fatalf("newHTTPClient() error: %v", err)
	}
	transport := transportOf(t, client)

	// The zero value must not opt out of the environment's proxy, which is what a
	// freshly constructed http.Transport would silently do.
	if !usesEnvironmentProxy(transport) {
		t.Error("the default client does not resolve its proxy from the environment")
	}
	// Clone() carries DefaultTransport's own TLS config, so this is not nil — what
	// matters is that nothing was overridden.
	if cfg := transport.TLSClientConfig; cfg != nil {
		if cfg.RootCAs != nil {
			t.Error("RootCAs is set on a default client")
		}
		if cfg.InsecureSkipVerify {
			t.Error("InsecureSkipVerify = true on a default client")
		}
	}
}

// Configuring TLS must not cost what DefaultTransport sets up: an earlier version
// assigned a fresh &tls.Config{} and dropped NextProtos, quietly taking HTTP/2
// negotiation with it.
func TestNewHTTPClientKeepsTLSDefaults(t *testing.T) {
	want := http.DefaultTransport.(*http.Transport).Clone().TLSClientConfig
	if want == nil || len(want.NextProtos) == 0 {
		t.Skip("DefaultTransport carries no TLS config to preserve on this platform")
	}

	caPath := writePEM(t, "ca.pem", pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).Certificate().Raw,
	}))

	for name, opts := range map[string]httpClientOptions{
		"with a CA":     {CACertFile: caPath},
		"insecure":      {Insecure: true},
		"CA + insecure": {CACertFile: caPath, Insecure: true},
	} {
		t.Run(name, func(t *testing.T) {
			client, err := newHTTPClient(opts)
			if err != nil {
				t.Fatal(err)
			}
			got := transportOf(t, client).TLSClientConfig
			if got == nil {
				t.Fatal("TLSClientConfig is nil")
			}
			if !reflect.DeepEqual(got.NextProtos, want.NextProtos) {
				t.Errorf("NextProtos = %v, want %v", got.NextProtos, want.NextProtos)
			}
		})
	}
}

func TestNewHTTPClientProxy(t *testing.T) {
	const proxy = "http://proxy.example.com:8080"

	client, err := newHTTPClient(httpClientOptions{Proxy: proxy})
	if err != nil {
		t.Fatalf("newHTTPClient() error: %v", err)
	}
	transport := transportOf(t, client)

	req, err := http.NewRequest(http.MethodGet, "https://api.scanoss.com/", nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := transport.Proxy(req)
	if err != nil {
		t.Fatalf("resolving the proxy: %v", err)
	}
	if got == nil || got.String() != proxy {
		t.Errorf("proxy = %v, want %q", got, proxy)
	}
}

// An explicit proxy still honours NO_PROXY, so a split network — a corporate proxy for
// the internet, an internal endpoint reached directly — keeps working once the proxy is
// configured rather than left to the environment.
//
// Unlike http.ProxyFromEnvironment, newHTTPClient samples the environment when it builds
// the client, so t.Setenv is enough and no test decides the answer for the others.
func TestNewHTTPClientProxyHonoursNoProxy(t *testing.T) {
	const proxy = "http://proxy.example.com:8080"

	tests := []struct {
		name    string
		noProxy string
		target  string
		want    string // "" means the request must go direct
	}{
		{name: "exact host exempted", noProxy: "scanoss.internal.example.com", target: "https://scanoss.internal.example.com/v3", want: ""},
		{name: "another host still proxied", noProxy: "scanoss.internal.example.com", target: "https://api.scanoss.com/v3", want: proxy},
		{name: "domain suffix exempted", noProxy: ".internal.example.com", target: "https://scanoss.internal.example.com/v3", want: ""},
		{name: "wildcard exempts everything", noProxy: "*", target: "https://api.scanoss.com/v3", want: ""},
		{name: "unset proxies everything", noProxy: "", target: "https://scanoss.internal.example.com/v3", want: proxy},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("NO_PROXY", tt.noProxy)
			t.Setenv("no_proxy", "")

			client, err := newHTTPClient(httpClientOptions{Proxy: proxy})
			if err != nil {
				t.Fatalf("newHTTPClient() error: %v", err)
			}
			req, err := http.NewRequest(http.MethodGet, tt.target, nil)
			if err != nil {
				t.Fatal(err)
			}
			got, err := transportOf(t, client).Proxy(req)
			if err != nil {
				t.Fatalf("resolving the proxy: %v", err)
			}
			switch {
			case tt.want == "" && got != nil:
				t.Errorf("proxy = %v, want the request to go direct", got)
			case tt.want != "" && (got == nil || got.String() != tt.want):
				t.Errorf("proxy = %v, want %q", got, tt.want)
			}
		})
	}
}

// Loopback is exempt with no NO_PROXY at all, as it is when the proxy comes from the
// environment. A local endpoint is not what a proxy is for, and this keeps one rule
// rather than two. Note it is the *target* that matters: a proxy running on localhost
// still serves requests to a remote API.
func TestNewHTTPClientProxySkipsLoopbackTargets(t *testing.T) {
	const proxy = "http://localhost:8080" // a local intercepting proxy
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")

	client, err := newHTTPClient(httpClientOptions{Proxy: proxy})
	if err != nil {
		t.Fatalf("newHTTPClient() error: %v", err)
	}
	tests := map[string]string{
		"http://127.0.0.1:9000/v3":   "",    // loopback target: direct
		"http://localhost:9000/v3":   "",    // same by name
		"https://api.scanoss.com/v3": proxy, // remote target: proxied
	}
	for target, want := range tests {
		req, err := http.NewRequest(http.MethodGet, target, nil)
		if err != nil {
			t.Fatal(err)
		}
		got, err := transportOf(t, client).Proxy(req)
		if err != nil {
			t.Fatalf("resolving the proxy for %s: %v", target, err)
		}
		switch {
		case want == "" && got != nil:
			t.Errorf("%s: proxy = %v, want direct", target, got)
		case want != "" && (got == nil || got.String() != want):
			t.Errorf("%s: proxy = %v, want %q", target, got, want)
		}
	}
}

// Go reads no_proxy when NO_PROXY is unset, and a client that ignored the lowercase
// spelling would proxy hosts the rest of the user's tooling reaches directly.
func TestNewHTTPClientProxyHonoursLowercaseNoProxy(t *testing.T) {
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "scanoss.internal.example.com")

	client, err := newHTTPClient(httpClientOptions{Proxy: "http://proxy.example.com:8080"})
	if err != nil {
		t.Fatalf("newHTTPClient() error: %v", err)
	}
	req, err := http.NewRequest(http.MethodGet, "https://scanoss.internal.example.com/v3", nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := transportOf(t, client).Proxy(req)
	if err != nil {
		t.Fatalf("resolving the proxy: %v", err)
	}
	if got != nil {
		t.Errorf("proxy = %v, want the request to go direct", got)
	}
}

// A proxy without a scheme is refused where the user can still fix it. url.Parse
// would read the host as the scheme and leave no host, which surfaces much later
// as "dial tcp :0".
func TestNewHTTPClientProxyRequiresScheme(t *testing.T) {
	accepted := []string{
		"http://proxy.example.com:8080",
		"https://proxy.example.com:8443",
		"HTTP://proxy.example.com:8080", // schemes are case-insensitive
		"http://user:pass@proxy.example.com:8080",
	}
	rejected := []string{
		"proxy.example.com:8080",
		"proxy.example.com",
		"//proxy.example.com:8080",
		"socks5://127.0.0.1:1080",
	}

	for _, proxy := range accepted {
		if _, err := newHTTPClient(httpClientOptions{Proxy: proxy}); err != nil {
			t.Errorf("newHTTPClient(Proxy: %q) error: %v", proxy, err)
		}
	}
	for _, proxy := range rejected {
		_, err := newHTTPClient(httpClientOptions{Proxy: proxy})
		if err == nil {
			t.Errorf("newHTTPClient(Proxy: %q) succeeded, want an error", proxy)
			continue
		}
		if !strings.Contains(err.Error(), "https:// or http://") {
			t.Errorf("error %q does not name the accepted schemes", err)
		}
		if !strings.Contains(err.Error(), proxy) {
			t.Errorf("error %q does not quote the value back", err)
		}
	}
}

// The CA is added to the system pool, not swapped for it, and verification stays
// on. Proven end to end against a server the system pool cannot vouch for.
func TestNewHTTPClientCACert(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	caPath := writePEM(t, "ca.pem", pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: srv.Certificate().Raw,
	}))

	// Without the CA the handshake must fail — otherwise the test proves nothing.
	plain, err := newHTTPClient(httpClientOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plain.Get(srv.URL); err == nil {
		t.Fatal("the default client trusted a server the system pool cannot vouch for")
	}

	client, err := newHTTPClient(httpClientOptions{CACertFile: caPath})
	if err != nil {
		t.Fatalf("newHTTPClient() error: %v", err)
	}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("request with the CA configured: %v", err)
	}
	defer resp.Body.Close()

	transport := transportOf(t, client)
	if transport.TLSClientConfig == nil || transport.TLSClientConfig.RootCAs == nil {
		t.Error("RootCAs is not set")
	}
	if transport.TLSClientConfig.InsecureSkipVerify {
		t.Error("InsecureSkipVerify = true; a CA file must not disable verification")
	}
	// A CA file must not cost the environment's proxy.
	if !usesEnvironmentProxy(transport) {
		t.Error("configuring a CA dropped the environment proxy")
	}
}

func TestNewHTTPClientCACertErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.pem")
	notPEM := writePEM(t, "hosts", []byte("127.0.0.1 localhost\n"))
	keyOnly := writePEM(t, "key.pem", pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: []byte("not a certificate"),
	}))

	for _, tc := range []struct {
		name string
		path string
		want string
	}{
		{"missing file", missing, "reading CA certificate"},
		{"not a PEM file", notPEM, "contains no certificate"},
		{"a key, not a certificate", keyOnly, "contains no certificate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := newHTTPClient(httpClientOptions{CACertFile: tc.path})
			if err == nil {
				t.Fatalf("newHTTPClient(CACertFile: %q) succeeded, want an error", tc.path)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
			// The path is what the user has to go look at.
			if !strings.Contains(err.Error(), tc.path) {
				t.Errorf("error %q does not name the path", err)
			}
		})
	}
}

func TestNewHTTPClientInsecure(t *testing.T) {
	client, err := newHTTPClient(httpClientOptions{Insecure: true})
	if err != nil {
		t.Fatalf("newHTTPClient() error: %v", err)
	}
	transport := transportOf(t, client)

	if transport.TLSClientConfig == nil || !transport.TLSClientConfig.InsecureSkipVerify {
		t.Error("InsecureSkipVerify = false, want true")
	}
}

// The regression this feature is built on: an insecure client used to be built
// from a fresh http.Transport, whose nil Proxy silently bypassed HTTPS_PROXY. It
// must keep resolving the environment's proxy.
func TestInsecureKeepsTheEnvironmentProxy(t *testing.T) {
	client, err := newHTTPClient(httpClientOptions{Insecure: true})
	if err != nil {
		t.Fatalf("newHTTPClient() error: %v", err)
	}
	if !usesEnvironmentProxy(transportOf(t, client)) {
		t.Error("an insecure client no longer resolves its proxy from the environment")
	}
}

// All three together, since each one touches the transport and an earlier
// implementation had them overwrite one another.
func TestNewHTTPClientCombined(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	caPath := writePEM(t, "ca.pem", pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: srv.Certificate().Raw,
	}))

	const proxy = "http://proxy.example.com:8080"
	client, err := newHTTPClient(httpClientOptions{
		Proxy:      proxy,
		CACertFile: caPath,
		Insecure:   true,
	})
	if err != nil {
		t.Fatalf("newHTTPClient() error: %v", err)
	}
	transport := transportOf(t, client)

	req, err := http.NewRequest(http.MethodGet, "https://api.scanoss.com/", nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := transport.Proxy(req)
	if err != nil {
		t.Fatalf("resolving the proxy: %v", err)
	}
	if got == nil || got.String() != proxy {
		t.Errorf("proxy = %v, want %q", got, proxy)
	}
	if transport.TLSClientConfig.RootCAs == nil {
		t.Error("RootCAs was dropped")
	}
	if !transport.TLSClientConfig.InsecureSkipVerify {
		t.Error("InsecureSkipVerify was dropped")
	}
}

// The clone must keep what DefaultTransport configures, not just its proxy.
func TestNewHTTPClientKeepsTransportDefaults(t *testing.T) {
	client, err := newHTTPClient(httpClientOptions{Insecure: true})
	if err != nil {
		t.Fatal(err)
	}
	got := transportOf(t, client)
	want := http.DefaultTransport.(*http.Transport)

	if got.MaxIdleConns != want.MaxIdleConns {
		t.Errorf("MaxIdleConns = %d, want %d", got.MaxIdleConns, want.MaxIdleConns)
	}
	if got.IdleConnTimeout != want.IdleConnTimeout {
		t.Errorf("IdleConnTimeout = %v, want %v", got.IdleConnTimeout, want.IdleConnTimeout)
	}
	if got.TLSHandshakeTimeout != want.TLSHandshakeTimeout {
		t.Errorf("TLSHandshakeTimeout = %v, want %v", got.TLSHandshakeTimeout, want.TLSHandshakeTimeout)
	}
}
