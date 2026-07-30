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

// Cryptography service endpoints: algorithms and library hints, each for an exact
// version or a version range (plus algorithm versions in a range). Every variant
// has a batch (POST, many components) and a single (GET, one component) endpoint.
var (
	ServiceCryptographyAlgorithms        = Service{Name: "cryptography.algorithms", endpoint: "/v3/cryptography/algorithms"}
	ServiceCryptographyAlgorithm         = Service{Name: "cryptography.algorithm", endpoint: "/v3/cryptography/algorithms"}
	ServiceCryptographyAlgorithmsInRange = Service{Name: "cryptography.algorithms.range", endpoint: "/v3/cryptography/algorithms/range"}
	ServiceCryptographyAlgorithmInRange  = Service{Name: "cryptography.algorithm.range", endpoint: "/v3/cryptography/algorithms/range"}
	ServiceCryptographyVersionsInRange   = Service{Name: "cryptography.versions.range", endpoint: "/v3/cryptography/algorithms/versions/range"}
	ServiceCryptographyVersionInRange    = Service{Name: "cryptography.version.range", endpoint: "/v3/cryptography/algorithms/versions/range"}
	ServiceCryptographyHints             = Service{Name: "cryptography.hints", endpoint: "/v3/cryptography/hints"}
	ServiceCryptographyHint              = Service{Name: "cryptography.hint", endpoint: "/v3/cryptography/hints"}
	ServiceCryptographyHintsInRange      = Service{Name: "cryptography.hints.range", endpoint: "/v3/cryptography/hints/range"}
	ServiceCryptographyHintInRange       = Service{Name: "cryptography.hint.range", endpoint: "/v3/cryptography/hints/range"}
)

// CryptographyAPI is the cryptography service surface: algorithms and library
// hints, each exact-version and version-range, plus algorithm versions in range.
// Responses are typed from the OpenAPI v3 contract.
type CryptographyAPI interface {
	Algorithms(ctx context.Context, comps []Component, opts ...DecorateOption) (*scanossapi.CryptoAlgorithmsResponse, error)
	Algorithm(ctx context.Context, comp Component, opts ...DecorateOption) (*scanossapi.CryptoAlgorithmsResponse, error)
	AlgorithmsInRange(ctx context.Context, comps []Component, opts ...DecorateOption) (*scanossapi.CryptoAlgorithmsInRangeResponse, error)
	AlgorithmInRange(ctx context.Context, comp Component, opts ...DecorateOption) (*scanossapi.CryptoAlgorithmsInRangeResponse, error)
	VersionsInRange(ctx context.Context, comps []Component, opts ...DecorateOption) (*scanossapi.CryptoVersionsInRangeResponse, error)
	VersionInRange(ctx context.Context, comp Component, opts ...DecorateOption) (*scanossapi.CryptoVersionsInRangeResponse, error)
	Hints(ctx context.Context, comps []Component, opts ...DecorateOption) (*scanossapi.CryptoHintsResponse, error)
	Hint(ctx context.Context, comp Component, opts ...DecorateOption) (*scanossapi.CryptoHintsResponse, error)
	HintsInRange(ctx context.Context, comps []Component, opts ...DecorateOption) (*scanossapi.CryptoHintsInRangeResponse, error)
	HintInRange(ctx context.Context, comp Component, opts ...DecorateOption) (*scanossapi.CryptoHintsInRangeResponse, error)
}

type cryptographyService struct{ c *Client }

var _ CryptographyAPI = cryptographyService{}

// Algorithms returns cryptographic algorithms for the given components (batch).
func (s cryptographyService) Algorithms(ctx context.Context, comps []Component, opts ...DecorateOption) (*scanossapi.CryptoAlgorithmsResponse, error) {
	res, err := s.c.decorate(ctx, ServiceCryptographyAlgorithms, comps, opts...)
	if err != nil {
		return nil, err
	}
	return As[scanossapi.CryptoAlgorithmsResponse](res)
}

// Algorithm returns cryptographic algorithms for a single component.
func (s cryptographyService) Algorithm(ctx context.Context, comp Component, opts ...DecorateOption) (*scanossapi.CryptoAlgorithmsResponse, error) {
	res, err := s.c.decorateOne(ctx, ServiceCryptographyAlgorithm, comp, opts...)
	if err != nil {
		return nil, err
	}
	return As[scanossapi.CryptoAlgorithmsResponse](res)
}

// AlgorithmsInRange returns algorithms across a version range (batch).
func (s cryptographyService) AlgorithmsInRange(ctx context.Context, comps []Component, opts ...DecorateOption) (*scanossapi.CryptoAlgorithmsInRangeResponse, error) {
	res, err := s.c.decorate(ctx, ServiceCryptographyAlgorithmsInRange, comps, opts...)
	if err != nil {
		return nil, err
	}
	return As[scanossapi.CryptoAlgorithmsInRangeResponse](res)
}

// AlgorithmInRange returns algorithms across a version range for a single component.
func (s cryptographyService) AlgorithmInRange(ctx context.Context, comp Component, opts ...DecorateOption) (*scanossapi.CryptoAlgorithmsInRangeResponse, error) {
	res, err := s.c.decorateOne(ctx, ServiceCryptographyAlgorithmInRange, comp, opts...)
	if err != nil {
		return nil, err
	}
	return As[scanossapi.CryptoAlgorithmsInRangeResponse](res)
}

// VersionsInRange returns algorithm versions across a range (batch).
func (s cryptographyService) VersionsInRange(ctx context.Context, comps []Component, opts ...DecorateOption) (*scanossapi.CryptoVersionsInRangeResponse, error) {
	res, err := s.c.decorate(ctx, ServiceCryptographyVersionsInRange, comps, opts...)
	if err != nil {
		return nil, err
	}
	return As[scanossapi.CryptoVersionsInRangeResponse](res)
}

// VersionInRange returns algorithm versions across a range for a single component.
func (s cryptographyService) VersionInRange(ctx context.Context, comp Component, opts ...DecorateOption) (*scanossapi.CryptoVersionsInRangeResponse, error) {
	res, err := s.c.decorateOne(ctx, ServiceCryptographyVersionInRange, comp, opts...)
	if err != nil {
		return nil, err
	}
	return As[scanossapi.CryptoVersionsInRangeResponse](res)
}

// Hints returns cryptographic library hints for the given components (batch).
func (s cryptographyService) Hints(ctx context.Context, comps []Component, opts ...DecorateOption) (*scanossapi.CryptoHintsResponse, error) {
	res, err := s.c.decorate(ctx, ServiceCryptographyHints, comps, opts...)
	if err != nil {
		return nil, err
	}
	return As[scanossapi.CryptoHintsResponse](res)
}

// Hint returns cryptographic library hints for a single component.
func (s cryptographyService) Hint(ctx context.Context, comp Component, opts ...DecorateOption) (*scanossapi.CryptoHintsResponse, error) {
	res, err := s.c.decorateOne(ctx, ServiceCryptographyHint, comp, opts...)
	if err != nil {
		return nil, err
	}
	return As[scanossapi.CryptoHintsResponse](res)
}

// HintsInRange returns library hints across a version range (batch).
func (s cryptographyService) HintsInRange(ctx context.Context, comps []Component, opts ...DecorateOption) (*scanossapi.CryptoHintsInRangeResponse, error) {
	res, err := s.c.decorate(ctx, ServiceCryptographyHintsInRange, comps, opts...)
	if err != nil {
		return nil, err
	}
	return As[scanossapi.CryptoHintsInRangeResponse](res)
}

// HintInRange returns library hints across a version range for a single component.
func (s cryptographyService) HintInRange(ctx context.Context, comp Component, opts ...DecorateOption) (*scanossapi.CryptoHintsInRangeResponse, error) {
	res, err := s.c.decorateOne(ctx, ServiceCryptographyHintInRange, comp, opts...)
	if err != nil {
		return nil, err
	}
	return As[scanossapi.CryptoHintsInRangeResponse](res)
}
