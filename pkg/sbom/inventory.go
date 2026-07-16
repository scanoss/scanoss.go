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

// Package sbom generates SBOM documents (CycloneDX, SPDX Lite) from a neutral
// inventory of components. It depends only on the CycloneDX library — never on the
// scan SDK — so it can be reused by the CLI and by external consumers alike. Adapters
// that build an Inventory from a scan result live in the scansource subpackage.
package sbom

import "strings"

// Inventory is the neutral, format-agnostic bill of materials and the core of the CLI's raw
// output. Detected components (scan matches) and declared dependency components live in ONE
// Components list, tagged by Component.Scope, so enrichment decorates the union origin-agnostic.
// Per-component layers (licenses, cryptography, geoprovenance) attach inline on each component;
// vulnerabilities are a flat top-level list joined to components by base PURL. Build it directly,
// or via the scansource adapters from a scan result.
type Inventory struct {
	Components      []Component     `json:"components"`
	Vulnerabilities []Vulnerability `json:"vulnerabilities,omitempty"`
}

// Component is one component in the inventory: identity, scope, scan evidence, and the
// per-component enrichment layers attached inline. Vulnerabilities are not stored here — they
// are the Inventory's flat top-level list, joined back by PURL.
type Component struct {
	Purl             string            `json:"purl"`                        // canonical PURL (identity), e.g. "pkg:github/scanoss/engine"
	Scope            ComponentScope    `json:"scope,omitempty"`             // detected (from the scan) | declared (from a manifest); "" == detected
	AliasPurls       []string          `json:"alias_purls,omitempty"`       // additional PURLs identifying the same component (beyond Purl)
	Vendor           string            `json:"vendor,omitempty"`            // supplier / namespace
	Name             string            `json:"name,omitempty"`              // component name
	Version          string            `json:"version,omitempty"`           // resolved version; "" renders as "NOASSERTION"
	URL              string            `json:"url,omitempty"`               // homepage / source URL ("" => no externalReference)
	URLHash          string            `json:"url_hash,omitempty"`          // SCANOSS url_hash (SPDX package checksum)
	Licenses         []License         `json:"licenses,omitempty"`          // declared and/or concluded licenses (licenses layer)
	Cryptography     []CryptoAlgorithm `json:"cryptography,omitempty"`      // cryptographic algorithms detected (crypto layer)
	Geoprovenance    []GeoLocation     `json:"geoprovenance,omitempty"`     // contributor geographic origin (geo layer)
	DownloadLocation string            `json:"download_location,omitempty"` // download location (defaults to URL)
	Evidence         []FileEvidence    `json:"evidence,omitempty"`          // where the component came from: scanned files that matched, or the manifest that declared it
}

// CryptoAlgorithm is a cryptographic algorithm detected in a component (crypto layer).
type CryptoAlgorithm struct {
	Algorithm string `json:"algorithm"`
	Strength  string `json:"strength,omitempty"`
}

// GeoLocation is one geographic origin of a component's contributors (geo layer), with the
// share of contribution when known.
type GeoLocation struct {
	Name       string  `json:"name"`
	Percentage float64 `json:"percentage,omitempty"`
}

// ComponentScope records how a component entered the inventory.
type ComponentScope string

const (
	// ScopeDetected marks a component found by the scan (an OSS match). The zero value is
	// treated as detected.
	ScopeDetected ComponentScope = "detected"
	// ScopeDeclared marks a component declared in a dependency manifest.
	ScopeDeclared ComponentScope = "declared"
)

// IsDeclared reports whether the component is a declared dependency rather than a
// scan-detected match. The zero-value scope counts as detected.
func (c Component) IsDeclared() bool {
	return c.Scope == ScopeDeclared
}

// AllPurls returns the canonical Purl followed by any AliasPurls.
func (c Component) AllPurls() []string {
	if c.Purl == "" {
		return nil
	}
	return append([]string{c.Purl}, c.AliasPurls...)
}

// LicenseAcknowledgement is how a license was established for a component.
type LicenseAcknowledgement string

const (
	// AckDeclared marks a license stated by the project (from the decoration service).
	AckDeclared LicenseAcknowledgement = "declared"
	// AckConcluded marks a license determined by review (e.g. from a downstream consumer's identifications).
	AckConcluded LicenseAcknowledgement = "concluded"
)

// License is a single license on a component, with its acknowledgement. The same id may
// appear more than once (e.g. both declared and concluded).
type License struct {
	ID              string                 `json:"id"`                        // SPDX id or "LicenseRef-*", e.g. "GPL-2.0-only"
	Acknowledgement LicenseAcknowledgement `json:"acknowledgement,omitempty"` // declared (default) | concluded
}

// FileEvidence is one occurrence of a component in the scanned project (a CycloneDX
// evidence.occurrence): a scanned file that matched (match_type "file"/"snippet", with — for
// snippets — where and how strongly it matched inside the OSS component), or the manifest that
// declared it (match_type "declared", with only the path set).
type FileEvidence struct {
	Path            string   `json:"path"`                        // occurrence location: scanned file path, or the manifest path for a declared dependency
	SourceHash      string   `json:"source_hash,omitempty"`       // hash of the scanned input file (from the WFP)
	FileHash        string   `json:"file_hash,omitempty"`         // hash of the matched file (== source_hash for a file match; the OSS file's for a snippet)
	MatchType       string   `json:"match_type,omitempty"`        // "file" (whole file) | "snippet" | "declared" (from a manifest)
	MatchPercentage int      `json:"match_percentage,omitempty"`  // match confidence (snippet only)
	OssFilePath     string   `json:"oss_file_path,omitempty"`     // matched file path inside the OSS component
	InputLineRanges []string `json:"input_line_ranges,omitempty"` // matched line ranges in the scanned file (snippet only)
	OssLineRanges   []string `json:"oss_line_ranges,omitempty"`   // matched line ranges in the OSS component (snippet only)
}

// Vulnerability is one known vulnerability affecting one or more components, in a
// format independent of any decoration API wire type.
type Vulnerability struct {
	ID       string   `json:"id"`                 // advisory/CVE id, e.g. "CVE-2021-1234"
	Severity string   `json:"severity,omitempty"` // critical|high|medium|low|none|"" (case-insensitive)
	Source   string   `json:"source,omitempty"`   // advisory source name, e.g. "NVD"
	URL      string   `json:"url,omitempty"`      // advisory URL (optional)
	Summary  string   `json:"summary,omitempty"`  // short description (optional)
	Purls    []string `json:"purls,omitempty"`    // base PURLs of affected components (join key to Component.Purl)

	// Optional quantitative scoring. All fields are optional: when unset they are not
	// rendered, and the output is identical to a severity-only vulnerability.
	CVSSScore  *float64 `json:"cvss_score,omitempty"`  // CVSS base score 0.0–10.0
	CVSSVector string   `json:"cvss_vector,omitempty"` // CVSS vector string, e.g. "CVSS:3.1/AV:N/..."
	CVSSMethod string   `json:"cvss_method,omitempty"` // CVSS scoring method, e.g. "CVSSv31"
	CWEs       []int    `json:"cwes,omitempty"`        // CWE ids, e.g. [77]
	EPSSScore  *float64 `json:"epss_score,omitempty"`  // EPSS probability 0.0–1.0 (no native CycloneDX field; emitted as a property)
}

// Format is a supported SBOM output format.
type Format string

const (
	// FormatCycloneDX selects CycloneDX JSON output.
	FormatCycloneDX Format = "cyclonedx"
	// FormatSPDX selects SPDX 2.3 (Lite field subset) JSON output.
	FormatSPDX Format = "spdx"
)

// noAssertion is the SPDX/CycloneDX placeholder for unknown values.
const noAssertion = "NOASSERTION"

// supplier returns the component vendor, or the namespace extracted from the PURL,
// falling back to NOASSERTION.
func supplier(comp Component) string {
	if comp.Vendor != "" {
		return comp.Vendor
	}
	if ns := purlNamespace(comp.Purl); ns != "" {
		return ns
	}
	return noAssertion
}

// purlNamespace extracts the namespace from a PURL string.
// e.g. "pkg:github/scanoss/scanner.c" -> "scanoss".
func purlNamespace(purl string) string {
	idx := strings.Index(purl, "/")
	if idx < 0 {
		return ""
	}
	rest := purl[idx+1:]
	slashIdx := strings.Index(rest, "/")
	if slashIdx < 0 {
		return ""
	}
	return rest[:slashIdx] // scoped npm "@scope" is returned as-is
}
