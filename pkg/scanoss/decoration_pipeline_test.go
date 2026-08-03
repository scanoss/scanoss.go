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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// echoServer returns a handler that replies with a per-component "components"
// array and a success status, optionally failing requests whose path contains
// failPathSubstr.
func echoServer(failPathSubstr string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failPathSubstr != "" && strings.Contains(r.URL.Path, failPathSubstr) {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		var req componentsRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"components": req.Components,
			"status":     map[string]string{"status": "success"},
		})
	})
}

func TestPipelineRunKeyedOutput(t *testing.T) {
	srv := httptest.NewServer(echoServer(""))
	defer srv.Close()

	client := mustNew(t, Config{APIURL: srv.URL, ChunkSize: 10, Workers: 5})
	p := client.DecorationPipeline(
		ServiceVulnerabilities,
		ServiceLicenses,
		ServiceGeoprovenanceOrigin,
	)

	res, err := p.Run(context.Background(), Components("pkg:a", "pkg:b", "pkg:c"))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if res.Vulnerabilities == nil || res.Licenses == nil || res.Geoprovenance == nil {
		t.Fatalf("a requested layer is missing: %v", populatedLayers(res))
	}
	if res.Cryptography != nil {
		t.Error("cryptography was not requested, so its layer must stay nil")
	}

	// MarshalJSON yields {"<service>": {components,status}}.
	var obj map[string]struct {
		Components []Component       `json:"components"`
		Status     map[string]string `json:"status"`
	}
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("MarshalJSON error: %v", err)
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, raw)
	}
	if len(obj) != 3 {
		t.Errorf("marshaled keys = %d, want 3", len(obj))
	}
	if obj["vulnerabilities"].Status["status"] != "success" {
		t.Errorf("vulnerabilities status not preserved: %+v", obj["vulnerabilities"])
	}
	if len(obj["vulnerabilities"].Components) != 3 {
		t.Errorf("vulnerabilities components = %d, want 3", len(obj["vulnerabilities"].Components))
	}
}

func TestPipelineAddRemoveDedupe(t *testing.T) {
	client := mustNew(t, Config{})
	p := client.DecorationPipeline(ServiceVulnerabilities, ServiceLicenses)

	p.Add(ServiceVulnerabilities) // duplicate -> ignored
	p.Add(ServiceGeoprovenanceOrigin)
	if names := serviceNames(p); !equal(names, []string{"vulnerabilities", "licenses", "geoprovenance.origin"}) {
		t.Fatalf("after Add: %v", names)
	}

	p.Remove(ServiceLicenses)
	if names := serviceNames(p); !equal(names, []string{"vulnerabilities", "geoprovenance.origin"}) {
		t.Fatalf("after Remove: %v", names)
	}
}

func TestPipelinePartialFailure(t *testing.T) {
	srv := httptest.NewServer(echoServer("/v3/licenses")) // licenses fails, others ok
	defer srv.Close()

	client := mustNew(t, Config{APIURL: srv.URL})
	p := client.DecorationPipeline(ServiceVulnerabilities, ServiceLicenses, ServiceGeoprovenanceOrigin)

	res, err := p.Run(context.Background(), Components("pkg:a"))
	if err != nil {
		t.Fatalf("expected partial success, got error: %v", err)
	}
	if _, ok := res.Errors["licenses"]; !ok {
		t.Errorf("expected licenses in Errors, got %v", res.Errors)
	}
	if res.Vulnerabilities == nil || res.Geoprovenance == nil {
		t.Errorf("expected the other two services to succeed: %v", populatedLayers(res))
	}
}

func TestPipelineAllFail(t *testing.T) {
	srv := httptest.NewServer(echoServer("/")) // everything fails
	defer srv.Close()

	client := mustNew(t, Config{APIURL: srv.URL})
	p := client.DecorationPipeline(ServiceVulnerabilities, ServiceLicenses)
	if _, err := p.Run(context.Background(), Components("pkg:a")); err == nil {
		t.Error("expected error when every service fails")
	}
}

func TestPipelineNoServices(t *testing.T) {
	client := mustNew(t, Config{})
	if _, err := client.DecorationPipeline().Run(context.Background(), Components("pkg:a")); err == nil {
		t.Error("expected error for pipeline with no services")
	}
}

// --- helpers ---

func serviceNames(p *DecorationPipeline) []string {
	out := make([]string, 0, len(p.Services()))
	for _, s := range p.Services() {
		out = append(out, s.Name)
	}
	return out
}

// populatedLayers names the layers a result carries, for a failure message.
func populatedLayers(res *PipelineResult) []string {
	var out []string
	if res.Licenses != nil {
		out = append(out, ServiceLicenses.Name)
	}
	if res.Cryptography != nil {
		out = append(out, ServiceCryptographyAlgorithms.Name)
	}
	if res.Geoprovenance != nil {
		out = append(out, ServiceGeoprovenanceOrigin.Name)
	}
	if res.Vulnerabilities != nil {
		out = append(out, ServiceVulnerabilities.Name)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// A pipeline can only hand back the services PipelineResult has a field for. Any other is
// refused before the first request, rather than after paying for the chunked round trip and
// discarding the answer.
func TestPipelineRejectsNonLayerService(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write([]byte(`{"components":[]}`))
	}))
	defer srv.Close()

	client := mustNew(t, Config{APIURL: srv.URL})
	p := client.DecorationPipeline(ServiceLicenses, ServiceLicenseEvidence)

	if _, err := p.Run(context.Background(), Components("pkg:a")); err == nil {
		t.Fatal("want an error naming the service that has no layer")
	} else if !strings.Contains(err.Error(), ServiceLicenseEvidence.Name) {
		t.Errorf("error should name %q, got: %v", ServiceLicenseEvidence.Name, err)
	}
	if got := atomic.LoadInt32(&hits); got != 0 {
		t.Errorf("requests = %d, want 0: the check must happen before any of them", got)
	}
}
