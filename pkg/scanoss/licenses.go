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
	"fmt"
	"net/url"

	scanossapi "github.com/scanoss/scanoss.api-sdk"
)

// License service endpoints (v3):
//   - attribution / evidence: LICENSE-NOTICE files and per-file license evidence.
//   - the License service under /v3/licenses: declared licenses for a
//     component (single GET / batch POST), plus SPDX-registry details and OSADL
//     obligations keyed by license id.
var (
	ServiceLicenseAttribution  = Service{Name: "license.attribution", endpoint: "/v3/license/attribution"}
	ServiceLicenseEvidence     = Service{Name: "license.evidence", endpoint: "/v3/license/evidence"}
	ServiceLicenses            = Service{Name: "licenses", endpoint: "/v3/licenses"}
	ServiceLicense             = Service{Name: "license", endpoint: "/v3/licenses"}
	ServiceLicensesDetails     = Service{Name: "licenses.details", endpoint: "/v3/licenses/details"}
	ServiceLicensesObligations = Service{Name: "licenses.obligations", endpoint: "/v3/licenses/obligations"}
)

// LicenseAPI is the licenses service surface. Responses are typed from the
// OpenAPI v3 contract.
type LicenseAPI interface {
	Attribution(ctx context.Context, comps []Component) (*scanossapi.AttributionResponse, error)
	Evidence(ctx context.Context, comps []Component) (*scanossapi.LicenseEvidenceResponse, error)
	Components(ctx context.Context, comps []Component) (*scanossapi.ComponentsLicenseResponse, error)
	Component(ctx context.Context, comp Component) (*scanossapi.ComponentLicenseResponse, error)
	Details(ctx context.Context, license string) (*scanossapi.LicenseDetailsResponse, error)
	Obligations(ctx context.Context, license string) (*scanossapi.ObligationsResponse, error)
}

type licenseService struct{ c *Client }

var _ LicenseAPI = licenseService{}

// Attribution returns the attribution files (LICENSE/NOTICE/…) for the given
// components.
func (s licenseService) Attribution(ctx context.Context, comps []Component) (*scanossapi.AttributionResponse, error) {
	res, err := s.c.decorate(ctx, ServiceLicenseAttribution, comps)
	if err != nil {
		return nil, err
	}
	return As[scanossapi.AttributionResponse](res)
}

// Evidence returns per-file license evidence for the given components.
func (s licenseService) Evidence(ctx context.Context, comps []Component) (*scanossapi.LicenseEvidenceResponse, error) {
	res, err := s.c.decorate(ctx, ServiceLicenseEvidence, comps)
	if err != nil {
		return nil, err
	}
	return As[scanossapi.LicenseEvidenceResponse](res)
}

// Components returns the declared licenses for the given components (batch).
func (s licenseService) Components(ctx context.Context, comps []Component) (*scanossapi.ComponentsLicenseResponse, error) {
	res, err := s.c.decorate(ctx, ServiceLicenses, comps)
	if err != nil {
		return nil, err
	}
	return As[scanossapi.ComponentsLicenseResponse](res)
}

// Component returns the declared licenses for a single component.
func (s licenseService) Component(ctx context.Context, comp Component) (*scanossapi.ComponentLicenseResponse, error) {
	res, err := s.c.decorateOne(ctx, ServiceLicense, comp)
	if err != nil {
		return nil, err
	}
	return As[scanossapi.ComponentLicenseResponse](res)
}

// Details returns the SPDX-registry metadata for a license id (e.g. "MIT").
func (s licenseService) Details(ctx context.Context, license string) (*scanossapi.LicenseDetailsResponse, error) {
	if license == "" {
		return nil, fmt.Errorf("no license id to query")
	}
	res, err := s.c.getResult(ctx, ServiceLicensesDetails.endpoint, url.Values{"id": {license}})
	if err != nil {
		return nil, err
	}
	return As[scanossapi.LicenseDetailsResponse](res)
}

// Obligations returns the OSADL compliance obligations for a license id.
func (s licenseService) Obligations(ctx context.Context, license string) (*scanossapi.ObligationsResponse, error) {
	if license == "" {
		return nil, fmt.Errorf("no license id to query")
	}
	res, err := s.c.getResult(ctx, ServiceLicensesObligations.endpoint, url.Values{"id": {license}})
	if err != nil {
		return nil, err
	}
	return As[scanossapi.ObligationsResponse](res)
}
