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
	"strconv"

	scanossapi "github.com/scanoss/scanoss.api-sdk"
)

// Components service endpoints (v3): free-form search, version listing, and
// lifecycle status (single GET / batch POST share the same status path).
var (
	ServiceComponentsSearch   = Service{name: "components.search", endpoint: "/v3/components/search"}
	ServiceComponentsVersions = Service{name: "components.versions", endpoint: "/v3/components/versions"}
	ServiceComponentsStatus   = Service{name: "components.status", endpoint: "/v3/components/status"}
	ServiceComponentStatus    = Service{name: "component.status", endpoint: "/v3/components/status"}
)

// ComponentSearch holds the filters for a component search. At least one of
// Search, Vendor or Component must be set; Search takes precedence when present.
type ComponentSearch struct {
	Search    string // free-form term; overrides Vendor/Component when set
	Vendor    string
	Component string
	PurlType  string // purl type (github, npm, pypi, …); defaults to github server-side
	Limit     int    // max results (0 = server default)
	Offset    int    // pagination offset
}

// ComponentsAPI is the components service surface. Responses are typed from the
// OpenAPI v3 contract.
type ComponentsAPI interface {
	Search(ctx context.Context, q ComponentSearch) (*scanossapi.ComponentsSearchResponse, error)
	Versions(ctx context.Context, purl string, limit int) (*scanossapi.ComponentVersionsResponse, error)
	Status(ctx context.Context, comps []Component) (*scanossapi.ComponentsStatusResponse, error)
	StatusOne(ctx context.Context, comp Component) (*scanossapi.ComponentsStatusResponse, error)
}

type componentsService struct{ c *Client }

var _ ComponentsAPI = componentsService{}

// Search finds components by free term, vendor and/or component name.
func (s componentsService) Search(ctx context.Context, q ComponentSearch) (*scanossapi.ComponentsSearchResponse, error) {
	if q.Search == "" && q.Vendor == "" && q.Component == "" {
		return nil, fmt.Errorf("search requires at least one of search, vendor or component")
	}
	v := url.Values{}
	if q.Search != "" {
		v.Set("search", q.Search)
	}
	if q.Vendor != "" {
		v.Set("vendor", q.Vendor)
	}
	if q.Component != "" {
		v.Set("component", q.Component)
	}
	if q.PurlType != "" {
		v.Set("purl_type", q.PurlType)
	}
	if q.Limit > 0 {
		v.Set("limit", strconv.Itoa(q.Limit))
	}
	if q.Offset > 0 {
		v.Set("offset", strconv.Itoa(q.Offset))
	}
	res, err := s.c.getResult(ctx, ServiceComponentsSearch.endpoint, v)
	if err != nil {
		return nil, err
	}
	return As[scanossapi.ComponentsSearchResponse](res)
}

// Versions lists the known versions (with licenses) for a purl, most recent
// first. limit <= 0 uses the server default.
func (s componentsService) Versions(ctx context.Context, purl string, limit int) (*scanossapi.ComponentVersionsResponse, error) {
	if purl == "" {
		return nil, fmt.Errorf("no purl to query")
	}
	v := url.Values{"purl": {purl}}
	if limit > 0 {
		v.Set("limit", strconv.Itoa(limit))
	}
	res, err := s.c.getResult(ctx, ServiceComponentsVersions.endpoint, v)
	if err != nil {
		return nil, err
	}
	return As[scanossapi.ComponentVersionsResponse](res)
}

// Status resolves the lifecycle status for the given components (batch).
func (s componentsService) Status(ctx context.Context, comps []Component) (*scanossapi.ComponentsStatusResponse, error) {
	res, err := s.c.decorate(ctx, ServiceComponentsStatus, comps)
	if err != nil {
		return nil, err
	}
	return As[scanossapi.ComponentsStatusResponse](res)
}

// StatusOne resolves the lifecycle status for a single component.
func (s componentsService) StatusOne(ctx context.Context, comp Component) (*scanossapi.ComponentsStatusResponse, error) {
	res, err := s.c.decorateOne(ctx, ServiceComponentStatus, comp)
	if err != nil {
		return nil, err
	}
	return As[scanossapi.ComponentsStatusResponse](res)
}
