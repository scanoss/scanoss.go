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

// Inventory is the neutral, format-agnostic bill of materials: the components to emit
// and the vulnerabilities affecting them. Build it directly, or via the scansource
// adapters from a scan result.
type Inventory struct {
	Components      []Component
	Vulnerabilities []Vulnerability
}

// Component is one detected component to emit in the SBOM.
type Component struct {
	Purl             string         // canonical PURL (identity), e.g. "pkg:github/scanoss/engine"
	AliasPurls       []string       // additional PURLs identifying the same component (beyond Purl)
	Vendor           string         // supplier / namespace
	Name             string         // component name
	Version          string         // resolved version; "" renders as "NOASSERTION"
	URL              string         // homepage / source URL ("" => no externalReference)
	URLHash          string         // SCANOSS url_hash (SPDX package checksum)
	Licenses         []License      // declared and/or concluded licenses
	DownloadLocation string         // download location (defaults to URL)
	Files            []FileEvidence // scanned files that matched this component
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
	ID              string                 // SPDX id or "LicenseRef-*", e.g. "GPL-2.0-only"
	Acknowledgement LicenseAcknowledgement // declared (default) | concluded
}

// FileEvidence is one scanned file that matched a component (a CycloneDX
// evidence.occurrence). For snippet matches it also carries the matched line ranges.
type FileEvidence struct {
	Path            string   // scanned file path (the occurrence "location")
	MatchType       string   // "file" (whole file) | "snippet"
	InputLineRanges []string // matched line ranges in the scanned file (snippet only)
	OssLineRanges   []string // matched line ranges in the OSS component (snippet only)
}

// Vulnerability is one known vulnerability affecting one or more components, in a
// format independent of any decoration API wire type.
type Vulnerability struct {
	ID       string   // advisory/CVE id, e.g. "CVE-2021-1234"
	Severity string   // critical|high|medium|low|none|"" (case-insensitive)
	Source   string   // advisory source name, e.g. "NVD"
	URL      string   // advisory URL (optional)
	Summary  string   // short description (optional)
	Purls    []string // base PURLs of affected components (join key to Component.Purl)
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
