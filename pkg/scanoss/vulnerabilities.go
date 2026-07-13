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

	scanossapi "github.com/scanoss/scanoss.api-sdk"
)

// Vulnerability service endpoints: known vulnerabilities and CPEs, each as a
// batch (POST, many components) and a single (GET, one component) endpoint.
var (
	ServiceVulnerabilities   = Service{name: "vulnerabilities", endpoint: "/v3/vulnerabilities/vulnerabilities"}
	ServiceVulnerability     = Service{name: "vulnerability", endpoint: "/v3/vulnerabilities/vulnerabilities"}
	ServiceVulnerabilityCpes = Service{name: "vulnerabilities.cpes", endpoint: "/v3/vulnerabilities/cpes"}
	ServiceVulnerabilityCpe  = Service{name: "vulnerability.cpes", endpoint: "/v3/vulnerabilities/cpes"}
)

// VulnerabilityAPI is the vulnerabilities service surface. Responses are typed
// from the OpenAPI v3 contract; the compiler enforces that vulnerabilityService
// implements every method (see the var _ below).
type VulnerabilityAPI interface {
	Components(ctx context.Context, comps []Component) (*scanossapi.VulnerabilitiesResponse, error)
	Component(ctx context.Context, comp Component) (*scanossapi.VulnerabilitiesResponse, error)
	Cpes(ctx context.Context, comps []Component) (*scanossapi.CpesResponse, error)
	Cpe(ctx context.Context, comp Component) (*scanossapi.CpesResponse, error)
}

type vulnerabilityService struct{ c *Client }

var _ VulnerabilityAPI = vulnerabilityService{}

// Components returns known vulnerabilities for the given components (batch).
func (s vulnerabilityService) Components(ctx context.Context, comps []Component) (*scanossapi.VulnerabilitiesResponse, error) {
	res, err := s.c.decorate(ctx, ServiceVulnerabilities, comps)
	if err != nil {
		return nil, err
	}
	return As[scanossapi.VulnerabilitiesResponse](res)
}

// Component returns known vulnerabilities for a single component.
func (s vulnerabilityService) Component(ctx context.Context, comp Component) (*scanossapi.VulnerabilitiesResponse, error) {
	res, err := s.c.decorateOne(ctx, ServiceVulnerability, comp)
	if err != nil {
		return nil, err
	}
	return As[scanossapi.VulnerabilitiesResponse](res)
}

// Cpes returns CPEs for the given components (batch).
func (s vulnerabilityService) Cpes(ctx context.Context, comps []Component) (*scanossapi.CpesResponse, error) {
	res, err := s.c.decorate(ctx, ServiceVulnerabilityCpes, comps)
	if err != nil {
		return nil, err
	}
	return As[scanossapi.CpesResponse](res)
}

// Cpe returns CPEs for a single component.
func (s vulnerabilityService) Cpe(ctx context.Context, comp Component) (*scanossapi.CpesResponse, error) {
	res, err := s.c.decorateOne(ctx, ServiceVulnerabilityCpe, comp)
	if err != nil {
		return nil, err
	}
	return As[scanossapi.CpesResponse](res)
}
