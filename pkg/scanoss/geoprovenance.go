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

// Geoprovenance service endpoints: country of origin vs contributor countries,
// each as a batch (POST, many components) and a single (GET, one component) endpoint.
var (
	ServiceGeoprovenanceOrigin       = Service{Name: "geoprovenance.origin", endpoint: "/v3/geoprovenance/origin"}
	ServiceGeoprovenanceOriginOne    = Service{Name: "geoprovenance.origin.one", endpoint: "/v3/geoprovenance/origin"}
	ServiceGeoprovenanceCountries    = Service{Name: "geoprovenance.countries", endpoint: "/v3/geoprovenance/countries"}
	ServiceGeoprovenanceCountriesOne = Service{Name: "geoprovenance.countries.one", endpoint: "/v3/geoprovenance/countries"}
)

// GeoprovenanceAPI is the geoprovenance service surface: country of origin and
// contributor countries, each batch and single. Responses are typed from the
// OpenAPI v3 contract.
type GeoprovenanceAPI interface {
	Origins(ctx context.Context, comps []Component, opts ...DecorateOption) (*scanossapi.GeoOriginResponse, error)
	Origin(ctx context.Context, comp Component, opts ...DecorateOption) (*scanossapi.GeoOriginResponse, error)
	Countries(ctx context.Context, comps []Component, opts ...DecorateOption) (*scanossapi.GeoContributorsResponse, error)
	Country(ctx context.Context, comp Component, opts ...DecorateOption) (*scanossapi.GeoContributorsResponse, error)
}

type geoprovenanceService struct{ c *Client }

var _ GeoprovenanceAPI = geoprovenanceService{}

// Origins returns the country of origin for the given components (batch).
func (s geoprovenanceService) Origins(ctx context.Context, comps []Component, opts ...DecorateOption) (*scanossapi.GeoOriginResponse, error) {
	res, err := s.c.decorate(ctx, ServiceGeoprovenanceOrigin, comps, opts...)
	if err != nil {
		return nil, err
	}
	return As[scanossapi.GeoOriginResponse](res)
}

// Origin returns the country of origin for a single component.
func (s geoprovenanceService) Origin(ctx context.Context, comp Component, opts ...DecorateOption) (*scanossapi.GeoOriginResponse, error) {
	res, err := s.c.decorateOne(ctx, ServiceGeoprovenanceOriginOne, comp, opts...)
	if err != nil {
		return nil, err
	}
	return As[scanossapi.GeoOriginResponse](res)
}

// Countries returns contributor countries for the given components (batch).
func (s geoprovenanceService) Countries(ctx context.Context, comps []Component, opts ...DecorateOption) (*scanossapi.GeoContributorsResponse, error) {
	res, err := s.c.decorate(ctx, ServiceGeoprovenanceCountries, comps, opts...)
	if err != nil {
		return nil, err
	}
	return As[scanossapi.GeoContributorsResponse](res)
}

// Country returns contributor countries for a single component.
func (s geoprovenanceService) Country(ctx context.Context, comp Component, opts ...DecorateOption) (*scanossapi.GeoContributorsResponse, error) {
	res, err := s.c.decorateOne(ctx, ServiceGeoprovenanceCountriesOne, comp, opts...)
	if err != nil {
		return nil, err
	}
	return As[scanossapi.GeoContributorsResponse](res)
}
