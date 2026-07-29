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

package api

import (
	"net/http"
	"reflect"
	"testing"
)

func TestSetInsecureTLS(t *testing.T) {
	t.Run("true disables verification", func(t *testing.T) {
		c := NewClient("https://example.com", "")
		c.SetInsecureTLS(true)
		tr, ok := c.client.Transport.(*http.Transport)
		if !ok {
			t.Fatalf("Transport = %T, want *http.Transport", c.client.Transport)
		}
		if tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
			t.Fatal("InsecureSkipVerify = false, want true")
		}
	})

	t.Run("false leaves the default client", func(t *testing.T) {
		c := NewClient("https://example.com", "")
		c.SetInsecureTLS(false)
		if c.client.Transport != nil {
			t.Fatalf("Transport = %v, want nil (default client)", c.client.Transport)
		}
	})

	// The transport used to be built by hand, and a hand-built http.Transport has a
	// nil Proxy — so turning off certificate verification also turned off
	// HTTP_PROXY/HTTPS_PROXY, silently. This is the assertion that fails against
	// that version.
	t.Run("true keeps the environment proxy", func(t *testing.T) {
		c := NewClient("https://example.com", "")
		c.SetInsecureTLS(true)
		tr, ok := c.client.Transport.(*http.Transport)
		if !ok {
			t.Fatalf("Transport = %T, want *http.Transport", c.client.Transport)
		}
		// Compared by identity rather than by setting HTTPS_PROXY and making a
		// request: http.ProxyFromEnvironment reads the environment once per process
		// and caches it, so whichever test ran first would decide the answer.
		if tr.Proxy == nil {
			t.Fatal("Proxy is nil; HTTP_PROXY and HTTPS_PROXY would be ignored")
		}
		if reflect.ValueOf(tr.Proxy).Pointer() != reflect.ValueOf(http.ProxyFromEnvironment).Pointer() {
			t.Error("the transport no longer resolves its proxy from the environment")
		}
	})
}

func TestSetHTTPClient(t *testing.T) {
	t.Run("replaces the client", func(t *testing.T) {
		c := NewClient("https://example.com", "")
		custom := &http.Client{Transport: &http.Transport{}}
		c.SetHTTPClient(custom)
		if c.client != custom {
			t.Error("SetHTTPClient did not replace the client")
		}
	})

	// Ignoring nil keeps a caller from accidentally removing the client that every
	// request depends on.
	t.Run("nil is ignored", func(t *testing.T) {
		c := NewClient("https://example.com", "")
		before := c.client
		c.SetHTTPClient(nil)
		if c.client != before {
			t.Error("a nil client replaced the existing one")
		}
	})
}
