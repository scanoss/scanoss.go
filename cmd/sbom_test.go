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
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

const cdxInput = `{"bomFormat":"CycloneDX","specVersion":"1.7","version":1,"components":[{"bom-ref":"pkg:github/scanoss/engine@5.4.7","type":"library","name":"pkg:github/scanoss/engine","version":"5.4.7","publisher":"scanoss","purl":"pkg:github/scanoss/engine@5.4.7","licenses":[{"license":{"id":"GPL-2.0-only","acknowledgement":"declared"}}]}],"vulnerabilities":[{"id":"CVE-2023-1234","source":{"name":"NVD"},"ratings":[{"severity":"high"}],"affects":[{"ref":"pkg:github/scanoss/engine@5.4.7"}]}]}`

const spdxInput = `{"spdxVersion":"SPDX-2.3","dataLicense":"CC0-1.0","SPDXID":"SPDXRef-DOCUMENT","name":"SBOM","documentNamespace":"https://spdx.org/spdxdocs/x","creationInfo":{"created":"2026-07-14T00:00:00Z","creators":["Tool: scanoss","Organization: SCANOSS"]},"packages":[{"SPDXID":"SPDXRef-a","name":"pkg:github/scanoss/engine","versionInfo":"5.4.7","downloadLocation":"NOASSERTION","filesAnalyzed":false,"licenseDeclared":"GPL-2.0-only","licenseConcluded":"NOASSERTION","copyrightText":"NOASSERTION","externalRefs":[{"referenceCategory":"PACKAGE-MANAGER","referenceType":"purl","referenceLocator":"pkg:github/scanoss/engine"}]}]}`

func TestIdentifyFormat(t *testing.T) {
	rawJSON, _ := json.Marshal(testScanResult())
	cases := []struct {
		name, input, want string
		wantErr           bool
	}{
		{name: "cyclonedx", input: cdxInput, want: inputCycloneDX},
		{name: "spdx", input: spdxInput, want: inputSPDX},
		{name: "raw", input: string(rawJSON), want: inputRaw},
		{name: "unrecognized", input: `{"hello":"world"}`, wantErr: true},
		{name: "invalid-json", input: `{not json`, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := identifyFormat([]byte(tc.input))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got format %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("identifyFormat: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// runSbomTest invokes the sbom command's RunE with the given flags/args.
func runSbomTest(t *testing.T, input, format, output string) error {
	t.Helper()
	c := &cobra.Command{RunE: runSbom}
	c.Flags().StringP("format", "f", "", "")
	c.Flags().StringP("output", "o", "", "")
	_ = c.Flags().Set("format", format)
	_ = c.Flags().Set("output", output)
	return runSbom(c, []string{input})
}

func TestSbom_CycloneDXToSPDX(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.cdx.json")
	out := filepath.Join(dir, "out.spdx.json")
	if err := os.WriteFile(in, []byte(cdxInput), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runSbomTest(t, in, "spdx", out); err != nil {
		t.Fatalf("sbom: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	if !strings.Contains(s, `"spdxVersion": "SPDX-2.3"`) {
		t.Errorf("expected an SPDX 2.3 document, got:\n%s", s)
	}
	if !strings.Contains(s, "pkg:github/scanoss/engine") {
		t.Errorf("expected the component in the output")
	}
	// SPDX can't carry vulnerabilities; the CVE must be dropped.
	if strings.Contains(s, "CVE-2023-1234") {
		t.Errorf("vulnerability should be dropped when converting to spdx")
	}
}

func TestSbom_SPDXToCycloneDX(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.spdx.json")
	out := filepath.Join(dir, "out.cdx.json")
	if err := os.WriteFile(in, []byte(spdxInput), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runSbomTest(t, in, "cyclonedx", out); err != nil {
		t.Fatalf("sbom: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	if !strings.Contains(s, `"specVersion": "1.7"`) {
		t.Errorf("expected CycloneDX 1.7, got:\n%s", s)
	}
	if !strings.Contains(s, "pkg:github/scanoss/engine") || !strings.Contains(s, "GPL-2.0-only") {
		t.Errorf("expected the component and license in the output")
	}
}

func TestSbom_RawToCycloneDX(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "result.json")
	out := filepath.Join(dir, "out.cdx.json")
	rawJSON, _ := json.Marshal(testScanResult())
	if err := os.WriteFile(in, rawJSON, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runSbomTest(t, in, "cyclonedx", out); err != nil {
		t.Fatalf("sbom: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "pkg:npm/lodash") {
		t.Errorf("expected the scanned component in the CycloneDX output, got:\n%s", got)
	}
}

func TestSbom_Errors(t *testing.T) {
	dir := t.TempDir()
	cdxPath := filepath.Join(dir, "in.cdx.json")
	_ = os.WriteFile(cdxPath, []byte(cdxInput), 0o644)
	badPath := filepath.Join(dir, "bad.json")
	_ = os.WriteFile(badPath, []byte(`{"hello":"world"}`), 0o644)
	out := filepath.Join(dir, "out.json")

	if err := runSbomTest(t, cdxPath, "", out); err == nil {
		t.Error("expected an error when --format is missing")
	}
	if err := runSbomTest(t, cdxPath, "xml", out); err == nil {
		t.Error("expected an error for an invalid target format")
	}
	if err := runSbomTest(t, cdxPath, "plain", out); err == nil {
		t.Error("plain is not a valid conversion target")
	}
	if err := runSbomTest(t, badPath, "spdx", out); err == nil {
		t.Error("expected an error for unrecognized input")
	}
	if err := runSbomTest(t, filepath.Join(dir, "missing.json"), "spdx", out); err == nil {
		t.Error("expected an error for a missing input file")
	}
}
