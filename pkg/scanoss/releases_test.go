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
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// releasesStub returns a server that records the query it was called with and
// replies with a minimal ReleasesResponse.
func releasesStub(t *testing.T, gotQuery *url.Values) *httptest.Server {
	t.Helper()
	const body = `{"component":{"purl":"pkg:github/scanoss/engine","version":"5.4.7"},` +
		`"releases":[{"version":"5.4.7","date":"2024-01-01","release_notes":"notes","url":"https://x"}],` +
		`"status":{"message":"ok","status":"SUCCESS"}}`
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotQuery = r.URL.Query()
		_, _ = w.Write([]byte(body))
	}))
}

// TestReleasesQueryBuilding proves the three query shapes (list / single / range)
// pass the right params and that the typed ReleasesResponse decodes.
func TestReleasesQueryBuilding(t *testing.T) {
	cases := []struct {
		name        string
		requirement string
		limit       int
		offset      int
		want        url.Values
	}{
		{
			name: "list",
			want: url.Values{"purl": {"pkg:github/scanoss/engine"}},
		},
		{
			name:        "single",
			requirement: "5.4.7",
			want:        url.Values{"purl": {"pkg:github/scanoss/engine"}, "requirement": {"5.4.7"}},
		},
		{
			name:        "range with pagination",
			requirement: ">=1.0.0, <=2.0.0",
			limit:       10,
			offset:      20,
			want: url.Values{
				"purl":        {"pkg:github/scanoss/engine"},
				"requirement": {">=1.0.0, <=2.0.0"},
				"limit":       {"10"},
				"offset":      {"20"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got url.Values
			srv := releasesStub(t, &got)
			defer srv.Close()

			client := mustNew(t, Config{APIURL: srv.URL})
			res, err := client.Components.Releases(context.Background(),
				"pkg:github/scanoss/engine", tc.requirement, tc.limit, tc.offset)
			if err != nil {
				t.Fatalf("Releases: %v", err)
			}
			if got.Encode() != tc.want.Encode() {
				t.Errorf("query = %q, want %q", got.Encode(), tc.want.Encode())
			}
			if len(res.Releases) != 1 || res.Releases[0].Version == nil || *res.Releases[0].Version != "5.4.7" {
				t.Errorf("release version = %v, want 5.4.7", res.Releases)
			}
			if res.Status.Status != "SUCCESS" {
				t.Errorf("status = %q, want SUCCESS", res.Status.Status)
			}
		})
	}
}

// TestReleasesRequiresPurl proves an empty purl is rejected before any request.
func TestReleasesRequiresPurl(t *testing.T) {
	client := mustNew(t, Config{APIURL: "http://127.0.0.1:0"})
	if _, err := client.Components.Releases(context.Background(), "", "", 0, 0); err == nil {
		t.Fatal("expected an error for an empty purl")
	}
}
