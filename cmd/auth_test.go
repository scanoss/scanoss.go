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

package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/scanoss/scanoss.go/pkg/scanoss"
)

// The no-key guard fires only against the default SCANOSS endpoint with no key;
// a custom endpoint (on-prem) is exempt and may run keyless.
func TestRequireKeyForDefaultEndpoint(t *testing.T) {
	cases := []struct {
		name    string
		apiURL  string
		apiKey  string
		wantErr bool
	}{
		{"default endpoint, no key", scanoss.DefaultAPIURL, "", true},
		{"default endpoint, trailing slash, no key", scanoss.DefaultAPIURL + "/", "", true},
		{"default endpoint, surrounding space, no key", "  " + scanoss.DefaultAPIURL + "  ", "", true},
		{"default endpoint, with key", scanoss.DefaultAPIURL, "secret", false},
		{"custom endpoint, no key", "https://elgato.scanoss.com", "", false},
		{"custom endpoint, with key", "https://elgato.scanoss.com", "secret", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := requireKeyForDefaultEndpoint(tc.apiURL, tc.apiKey)
			if tc.wantErr {
				if !errors.Is(err, errNoAPIKey) {
					t.Fatalf("got %v, want errNoAPIKey", err)
				}
			} else if err != nil {
				t.Fatalf("got %v, want nil", err)
			}
		})
	}
}

// renderAPIError passes the error through unchanged for both 401 and non-401, so
// the exit code is preserved either way; only the printed hint differs.
func TestRenderAPIError(t *testing.T) {
	if got := renderAPIError(nil); got != nil {
		t.Fatalf("nil in → got %v", got)
	}
	unauth := &scanoss.StatusError{StatusCode: http.StatusUnauthorized, Body: "nope"}
	if got := renderAPIError(unauth); !errors.Is(got, unauth) {
		t.Fatalf("401 error not preserved: %v", got)
	}
	other := &scanoss.StatusError{StatusCode: http.StatusNotFound, Body: "missing"}
	if got := renderAPIError(other); !errors.Is(got, other) {
		t.Fatalf("non-401 error not preserved: %v", got)
	}
}
