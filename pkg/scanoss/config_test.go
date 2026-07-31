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
	"testing"
	"time"
)

// mustNew builds a Client and fails the test if the configuration is invalid.
func mustNew(t *testing.T, cfg Config) *Client {
	t.Helper()
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

// The zero Config is the default configuration.
func TestConfigZeroValueDefaults(t *testing.T) {
	c := mustNew(t, Config{})

	if c.apiURL != DefaultAPIURL {
		t.Errorf("apiURL = %q, want %q", c.apiURL, DefaultAPIURL)
	}
	if c.chunkSize != DefaultChunkSize {
		t.Errorf("chunkSize = %d, want %d", c.chunkSize, DefaultChunkSize)
	}
	if c.workers != DefaultWorkers {
		t.Errorf("workers = %d, want %d", c.workers, DefaultWorkers)
	}
	if c.transport.maxRetries != DefaultMaxRetries {
		t.Errorf("maxRetries = %d, want %d", c.transport.maxRetries, DefaultMaxRetries)
	}
	if c.transport.maxRetryAfter != DefaultMaxRetryAfter {
		t.Errorf("maxRetryAfter = %v, want %v", c.transport.maxRetryAfter, DefaultMaxRetryAfter)
	}
	if c.log == nil {
		t.Error("log is nil, want slog.Default()")
	}
}

// A non-positive value is not a setting, it is an omission: it falls back to the default.
func TestConfigNonPositiveFallsBackToDefault(t *testing.T) {
	c := mustNew(t, Config{ChunkSize: -1, Workers: 0, MaxRetries: -5, MaxRetryAfter: -time.Second})

	if c.chunkSize != DefaultChunkSize {
		t.Errorf("chunkSize = %d, want %d", c.chunkSize, DefaultChunkSize)
	}
	if c.workers != DefaultWorkers {
		t.Errorf("workers = %d, want %d", c.workers, DefaultWorkers)
	}
	if c.transport.maxRetries != DefaultMaxRetries {
		t.Errorf("maxRetries = %d, want %d", c.transport.maxRetries, DefaultMaxRetries)
	}
	if c.transport.maxRetryAfter != DefaultMaxRetryAfter {
		t.Errorf("maxRetryAfter = %v, want %v", c.transport.maxRetryAfter, DefaultMaxRetryAfter)
	}
}

func TestConfigAppliesSetFields(t *testing.T) {
	c := mustNew(t, Config{
		APIKey:        "k",
		APIURL:        "https://scanoss.internal/",
		ChunkSize:     7,
		Workers:       3,
		MaxRetries:    9,
		MaxRetryAfter: 30 * time.Second,
	})

	if c.apiURL != "https://scanoss.internal" {
		t.Errorf("apiURL = %q, want the trailing slash trimmed", c.apiURL)
	}
	if c.transport.apiKey != "k" {
		t.Errorf("apiKey = %q, want %q", c.transport.apiKey, "k")
	}
	if c.chunkSize != 7 || c.workers != 3 {
		t.Errorf("chunkSize/workers = %d/%d, want 7/3", c.chunkSize, c.workers)
	}
	if c.transport.maxRetries != 9 {
		t.Errorf("maxRetries = %d, want 9", c.transport.maxRetries)
	}
	if c.transport.maxRetryAfter != 30*time.Second {
		t.Errorf("maxRetryAfter = %v, want 30s", c.transport.maxRetryAfter)
	}
}

// A default client shares http.DefaultTransport rather than cloning its own, so every
// client in a process shares one connection pool.
func TestConfigDefaultSharesDefaultTransport(t *testing.T) {
	c := mustNew(t, Config{})
	if c.transport.httpClient.Transport != nil {
		t.Errorf("Transport = %v, want nil (shared http.DefaultTransport)", c.transport.httpClient.Transport)
	}
}

// Every request is bounded: an unset Timeout takes DefaultTimeout, on the plain path
// and on the one that builds a transport.
func TestConfigTimeout(t *testing.T) {
	for name, tc := range map[string]struct {
		cfg  Config
		want time.Duration
	}{
		"unset takes the default":       {Config{}, DefaultTimeout},
		"unset with a transport":        {Config{InsecureTLS: true}, DefaultTimeout},
		"explicit wins":                 {Config{Timeout: 5 * time.Second}, 5 * time.Second},
		"explicit with a transport":     {Config{InsecureTLS: true, Timeout: 5 * time.Second}, 5 * time.Second},
		"negative disables it":          {Config{Timeout: -1}, 0},
		"negative with a transport too": {Config{InsecureTLS: true, Timeout: -1}, 0},
	} {
		t.Run(name, func(t *testing.T) {
			c := mustNew(t, tc.cfg)
			if got := c.transport.httpClient.Timeout; got != tc.want {
				t.Errorf("Timeout = %v, want %v", got, tc.want)
			}
		})
	}
}

// A caller who brings their own client owns its timeout: the SDK does not overwrite it.
func TestConfigHTTPClientKeepsItsOwnTimeout(t *testing.T) {
	c := mustNew(t, Config{HTTPClient: &http.Client{Timeout: time.Second}, Timeout: time.Hour})
	if c.transport.httpClient.Timeout != time.Second {
		t.Errorf("Timeout = %v, want the supplied client's 1s", c.transport.httpClient.Timeout)
	}
}

// HTTPClient takes precedence over the transport fields.
func TestConfigHTTPClientWins(t *testing.T) {
	mine := &http.Client{}
	c := mustNew(t, Config{HTTPClient: mine, InsecureTLS: true, Proxy: "http://proxy.example.com:8080"})
	if c.transport.httpClient != mine {
		t.Error("httpClient is not the one supplied in Config.HTTPClient")
	}
}

// A proxy without a scheme is refused at construction, not on the first request.
func TestConfigRejectsProxyWithoutScheme(t *testing.T) {
	if _, err := New(Config{Proxy: "proxy.example.com:8080"}); err == nil {
		t.Fatal("New succeeded with a schemeless proxy, want an error")
	}
}

// An unreadable CA file is refused at construction too.
func TestConfigRejectsMissingCACert(t *testing.T) {
	if _, err := New(Config{CACertFile: "/nonexistent/ca.pem"}); err == nil {
		t.Fatal("New succeeded with a missing CA file, want an error")
	}
}
