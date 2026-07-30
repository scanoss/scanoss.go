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

package sbom

import (
	"regexp"
	"strings"

	"github.com/github/go-spdx/v2/spdxexp"
)

// License identifiers reach us from the scan API, which is not obliged to send
// canonical SPDX. Both output formats validate them: CycloneDX's `license.id`
// is an enum of the SPDX list, and SPDX rejects identifiers that are neither on
// the list nor declared as a LicenseRef. Copying what the API sent into either
// field produces documents that fail validation — so everything an SBOM emits
// goes through here first.
//
// What arrives is not always a single identifier, either: a value may be a whole
// expression ("MIT AND LicenseRef-foo", or a malformed one). That is why these
// helpers work on expressions rather than on bare ids.

// nonIDStringChars matches everything SPDX forbids in a LicenseRef idstring,
// which is limited to letters, digits, "." and "-".
var nonIDStringChars = regexp.MustCompile(`[^a-zA-Z0-9.-]+`)

// normalizeLicense validates a license identifier or expression against the SPDX
// list and returns its canonical form. ok is false when SPDX does not recognise
// it — either because the identifier does not exist, or because the expression
// is malformed (an exception used outside a WITH, say).
func normalizeLicense(id string) (canonical string, ok bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", false
	}
	normalized, invalid := spdxexp.ValidateAndNormalizeLicensesWithOptions(
		[]string{id}, spdxexp.ValidateLicensesOptions{})
	if len(invalid) > 0 || len(normalized) == 0 {
		return "", false
	}
	return normalized[0], true
}

// licenseRef converts an identifier SPDX does not recognise into a LicenseRef,
// which is the format's own escape hatch for licenses outside its list. The
// information survives instead of being dropped, and the document stays valid —
// provided the ref is also declared (see licenseRefsIn).
func licenseRef(id string) string {
	if strings.HasPrefix(id, "LicenseRef-") {
		return id
	}
	cleaned := strings.Trim(nonIDStringChars.ReplaceAllString(strings.TrimSpace(id), "-"), "-")
	if cleaned == "" {
		return ""
	}
	return "LicenseRef-" + cleaned
}

// licenseRefsIn returns every LicenseRef mentioned by an identifier or
// expression, so each can be declared in hasExtractedLicensingInfos. Matching
// only bare identifiers would miss the ones nested inside an expression, which
// is exactly how a document ends up using a ref it never declares.
func licenseRefsIn(id string) []string {
	if !strings.Contains(id, "LicenseRef-") {
		return nil
	}
	// A well-formed expression can be taken apart properly.
	if parts, err := spdxexp.ExtractLicenses(id); err == nil {
		var refs []string
		for _, p := range parts {
			if strings.HasPrefix(p, "LicenseRef-") {
				refs = append(refs, p)
			}
		}
		return refs
	}
	// A malformed one cannot, so fall back to scanning the tokens: a ref that is
	// present but unparseable still has to be declared.
	var refs []string
	for _, tok := range strings.FieldsFunc(id, func(r rune) bool {
		return r == ' ' || r == '(' || r == ')'
	}) {
		if strings.HasPrefix(tok, "LicenseRef-") {
			refs = append(refs, tok)
		}
	}
	return refs
}

// isCompound reports whether an expression needs parentheses before being joined with another —
// "A OR B" AND "C" is not the same as "A OR (B AND C)".
//
// WITH is not here, and that is not an oversight: it binds tighter than AND and OR, so
// "A WITH B AND C" already means "(A WITH B) AND C" and parenthesising it changes nothing.
// Whether an expression may appear where only a bare identifier is allowed is a different
// question — see isExpression.
func isCompound(expr string) bool {
	for _, op := range []string{" AND ", " OR "} {
		if strings.Contains(expr, op) {
			return true
		}
	}
	return false
}

// isExpression reports whether expr carries any operator, and so is an SPDX expression rather than
// a single identifier. Every operator counts, WITH included: a field restricted to the SPDX
// identifier list rejects "GPL-2.0-only WITH Classpath-exception-2.0" as surely as it rejects
// "MIT OR Apache-2.0", and one such value invalidates the whole document.
func isExpression(expr string) bool {
	for _, op := range []string{" AND ", " OR ", " WITH "} {
		if strings.Contains(expr, op) {
			return true
		}
	}
	return false
}
