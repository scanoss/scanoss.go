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
	"os"
	"testing"
)

// TestTypedDecodeMerged proves the typed batch path against a stub: the engine
// merges chunk responses and Components returns a typed *VulnerabilitiesResponse
// with fields populated.
func TestTypedDecodeMerged(t *testing.T) {
	const body = `{"components":[{"purl":"pkg:npm/lodash","vulnerabilities":[{"id":"CVE-2021-23337","severity":"HIGH"}]}],"status":{"message":"ok","status":"SUCCESS"}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	// chunk-size 1 over 2 purls => 2 chunks merged back into one components array.
	client := New(WithAPIURL(srv.URL), WithChunkSize(1))
	v, err := client.Vulnerabilities.Components(context.Background(), Components("pkg:a", "pkg:b"))
	if err != nil {
		t.Fatalf("Components: %v", err)
	}
	if len(v.Components) != 2 {
		t.Fatalf("merged components = %d, want 2", len(v.Components))
	}
	if v.Status.Status != "SUCCESS" {
		t.Errorf("status = %q, want SUCCESS", v.Status.Status)
	}
	c := v.Components[0]
	if c.Purl == nil || *c.Purl != "pkg:npm/lodash" {
		t.Errorf("purl = %v, want pkg:npm/lodash", c.Purl)
	}
	if c.Vulnerabilities == nil || len(*c.Vulnerabilities) == 0 {
		t.Fatal("expected vulnerabilities")
	}
	if got := (*c.Vulnerabilities)[0].Id; got == nil || *got != "CVE-2021-23337" {
		t.Errorf("cve id = %v, want CVE-2021-23337", got)
	}
}

// TestTypedDecodeLiveAPI hits the real SCANOSS API when SCANOSS_API_KEY is set,
// proving the typed method round-trips a real v3 response.
func TestTypedDecodeLiveAPI(t *testing.T) {
	key := os.Getenv("SCANOSS_API_KEY")
	if key == "" {
		t.Skip("SCANOSS_API_KEY not set; skipping live API check")
	}

	client := New(WithAPIKey(key))
	v, err := client.Vulnerabilities.Components(context.Background(), Components("pkg:npm/lodash"))
	if err != nil {
		t.Fatalf("live Components: %v", err)
	}
	if len(v.Components) == 0 {
		t.Fatal("live response decoded to zero components")
	}
	t.Logf("live decode OK: %d component(s), status=%q", len(v.Components), v.Status.Status)
}
