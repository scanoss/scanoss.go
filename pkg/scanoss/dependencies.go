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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	scanossapi "github.com/scanoss/scanoss.api-sdk"
)

// Dependency service endpoints (v3): declared dependency + license resolution as a
// batch (POST, many components) and a single (GET, one component), plus a
// transitive walk bounded by depth and limit (POST).
var (
	ServiceDependencies = Service{name: "dependencies", endpoint: "/v3/dependencies/dependencies"}
	ServiceDependency   = Service{name: "dependency", endpoint: "/v3/dependencies/dependencies"}
	ServiceTransitive   = Service{name: "dependencies.transitive", endpoint: "/v3/dependencies/transitive"}
)

// DependencyAPI is the dependencies service surface. Responses are typed from the
// OpenAPI v3 contract; the compiler enforces that dependencyService implements
// every method (see the var _ below).
type DependencyAPI interface {
	// Dependencies resolves declared dependencies + licenses for the given
	// components (batch POST).
	Dependencies(ctx context.Context, comps []Component) (*scanossapi.DependenciesResolveResponse, error)
	// Dependency resolves declared dependencies for a single component (GET).
	Dependency(ctx context.Context, comp Component) (*scanossapi.DependenciesResolveResponse, error)
	// Transitive walks declared dependencies bounded by depth and limit (POST).
	// depth/limit <= 0 are omitted so the server defaults apply.
	Transitive(ctx context.Context, comps []Component, depth, limit int) (*scanossapi.TransitiveResponse, error)
}

type dependencyService struct{ c *Client }

var _ DependencyAPI = dependencyService{}

// Dependencies resolves declared dependencies for the given components (batch).
func (s dependencyService) Dependencies(ctx context.Context, comps []Component) (*scanossapi.DependenciesResolveResponse, error) {
	res, err := s.c.decorate(ctx, ServiceDependencies, comps)
	if err != nil {
		return nil, err
	}
	return As[scanossapi.DependenciesResolveResponse](res)
}

// Dependency resolves declared dependencies for a single component.
func (s dependencyService) Dependency(ctx context.Context, comp Component) (*scanossapi.DependenciesResolveResponse, error) {
	res, err := s.c.decorateOne(ctx, ServiceDependency, comp)
	if err != nil {
		return nil, err
	}
	return As[scanossapi.DependenciesResolveResponse](res)
}

// transitiveRequest is the /v3/dependencies/transitive body: components plus an
// optional depth/limit. Mirrors the OpenAPI TransitiveRequest shape.
type transitiveRequest struct {
	Components []Component `json:"components"`
	Depth      *int        `json:"depth,omitempty"`
	Limit      *int        `json:"limit,omitempty"`
}

// Transitive walks the declared dependency tree for the given components.
func (s dependencyService) Transitive(ctx context.Context, comps []Component, depth, limit int) (*scanossapi.TransitiveResponse, error) {
	if len(comps) == 0 {
		return nil, fmt.Errorf("no components to query")
	}
	reqBody := transitiveRequest{Components: comps}
	if depth > 0 {
		reqBody.Depth = &depth
	}
	if limit > 0 {
		reqBody.Limit = &limit
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request body: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, s.c.apiURL+ServiceTransitive.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	raw, _, err := s.c.transport.do(ctx, req)
	if err != nil {
		return nil, err
	}
	var out scanossapi.TransitiveResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("error parsing transitive response: %w", err)
	}
	return &out, nil
}
