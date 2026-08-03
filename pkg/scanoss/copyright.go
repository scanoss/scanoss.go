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

// Copyright service endpoints (v3): per-file copyright evidence and the distinct
// set of copyright holders. Both accept one or many components in a single POST.
var (
	ServiceCopyrightEvidence = Service{Name: "copyright.evidence", endpoint: "/v3/copyright/evidence"}
	ServiceCopyrightHolders  = Service{Name: "copyright.holders", endpoint: "/v3/copyright/holders"}
)

// CopyrightAPI is the copyright service surface. Responses are typed from the
// OpenAPI v3 contract.
type CopyrightAPI interface {
	Evidence(ctx context.Context, comps []Component, opts ...DecorateOption) (*scanossapi.CopyrightEvidenceResponse, error)
	Holders(ctx context.Context, comps []Component, opts ...DecorateOption) (*scanossapi.CopyrightHoldersResponse, error)
}

type copyrightService struct{ c *Client }

var _ CopyrightAPI = copyrightService{}

// Evidence returns per-file copyright evidence for the given components.
func (s copyrightService) Evidence(ctx context.Context, comps []Component, opts ...DecorateOption) (*scanossapi.CopyrightEvidenceResponse, error) {
	res, err := s.c.decorate(ctx, ServiceCopyrightEvidence, comps, opts...)
	if err != nil {
		return nil, err
	}
	return as[scanossapi.CopyrightEvidenceResponse](res)
}

// Holders returns the distinct copyright holders for the given components.
func (s copyrightService) Holders(ctx context.Context, comps []Component, opts ...DecorateOption) (*scanossapi.CopyrightHoldersResponse, error) {
	res, err := s.c.decorate(ctx, ServiceCopyrightHolders, comps, opts...)
	if err != nil {
		return nil, err
	}
	return as[scanossapi.CopyrightHoldersResponse](res)
}
