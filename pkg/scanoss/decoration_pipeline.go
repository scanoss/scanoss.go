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
	"encoding/json"
	"fmt"
	"sync"

	scanossapi "github.com/scanoss/scanoss.api-sdk"
)

// DecorationPipeline runs a configurable set of decoration services over the same
// components, in parallel, and returns one result keyed by service. Create one
// with Client.DecorationPipeline and reuse it; configure the service set with Add/Remove.
type DecorationPipeline struct {
	client   *Client
	services []Service // ordered, deduped by Service.Name
}

// Layer is an answer that arrived. Failed lists the chunks lost from it: a decoration is split
// into chunks, and a partial answer covers fewer components than asked for — those components
// read exactly like components the service had nothing to say about.
type Layer[T any] struct {
	Response *T
	Failed   []ChunkError
}

// PipelineResult holds one layer per decoration service. A non-nil layer always carries a
// response; a nil one was either not requested or did not arrive — see Errors. Go cannot give a
// map a different value type per key, so the layers are fields rather than entries.
type PipelineResult struct {
	Licenses        *Layer[scanossapi.ComponentsLicenseResponse]
	Cryptography    *Layer[scanossapi.CryptoAlgorithmsResponse]
	Geoprovenance   *Layer[scanossapi.GeoOriginResponse]
	Vulnerabilities *Layer[scanossapi.VulnerabilitiesResponse]

	Errors map[string]error // service name → it failed outright, or answered unreadably
}

// DecorationPipeline creates a pipeline bound to this client, seeded with the given
// services (deduped by name, order preserved).
func (c *Client) DecorationPipeline(services ...Service) *DecorationPipeline {
	p := &DecorationPipeline{client: c}
	return p.Add(services...)
}

// Add appends services that are not already present (dedupe by name). Chainable.
func (p *DecorationPipeline) Add(services ...Service) *DecorationPipeline {
	for _, s := range services {
		if p.indexOf(s.Name) < 0 {
			p.services = append(p.services, s)
		}
	}
	return p
}

// Remove drops the named services if present. Chainable.
func (p *DecorationPipeline) Remove(services ...Service) *DecorationPipeline {
	for _, s := range services {
		if i := p.indexOf(s.Name); i >= 0 {
			p.services = append(p.services[:i], p.services[i+1:]...)
		}
	}
	return p
}

// Services returns a copy of the configured service set, in order.
func (p *DecorationPipeline) Services() []Service {
	out := make([]Service, len(p.services))
	copy(out, p.services)
	return out
}

func (p *DecorationPipeline) indexOf(name string) int {
	for i, s := range p.services {
		if s.Name == name {
			return i
		}
	}
	return -1
}

// Run queries every configured service concurrently over the same components
// and returns the combined result once all services have finished (success or
// failure). It is a barrier: Run returns only after the last service completes.
//
// Run returns an error only if every service failed; otherwise it returns the
// PipelineResult with any per-service failures recorded in PipelineResult.Errors.
func (p *DecorationPipeline) Run(ctx context.Context, components []Component, opts ...DecorateOption) (*PipelineResult, error) {
	if len(p.services) == 0 {
		return nil, fmt.Errorf("pipeline has no services")
	}
	p.client.log.Debug("decoration pipeline run", "services", len(p.services), "components", len(components))

	type outcome struct {
		name string
		res  *Result
		err  error
	}
	ch := make(chan outcome, len(p.services))

	var wg sync.WaitGroup
	for _, svc := range p.services {
		wg.Add(1)
		go func(svc Service) {
			defer wg.Done()
			res, err := p.client.decorate(ctx, svc, components, opts...)
			ch <- outcome{name: svc.Name, res: res, err: err}
		}(svc)
	}
	wg.Wait() // barrier: every service has finished
	close(ch)

	pr := &PipelineResult{Errors: make(map[string]error)}
	decoded := 0
	for o := range ch {
		if o.err != nil {
			pr.Errors[o.name] = o.err
			continue
		}
		// A response that cannot be decoded is a failure, not an absence: recording it here
		// keeps a layer from disappearing without explanation.
		if err := pr.setLayer(o.name, o.res); err != nil {
			pr.Errors[o.name] = err
			continue
		}
		decoded++
	}

	if decoded == 0 {
		return nil, fmt.Errorf("all %d pipeline service(s) failed", len(p.services))
	}
	return pr, nil
}

// setLayer merges one service's chunks and decodes them into the matching field. A service with
// no field here is not a decoration layer: running it would leave its answer unreachable, so it
// is reported rather than silently dropped.
func (pr *PipelineResult) setLayer(name string, res *Result) error {
	raw, err := res.Merged()
	if err != nil {
		return fmt.Errorf("merging the %s response: %w", name, err)
	}
	switch name {
	case ServiceLicenses.Name:
		pr.Licenses, err = decodeLayer[scanossapi.ComponentsLicenseResponse](raw, res.Failed)
	case ServiceCryptographyAlgorithms.Name:
		pr.Cryptography, err = decodeLayer[scanossapi.CryptoAlgorithmsResponse](raw, res.Failed)
	case ServiceGeoprovenanceOrigin.Name:
		pr.Geoprovenance, err = decodeLayer[scanossapi.GeoOriginResponse](raw, res.Failed)
	case ServiceVulnerabilities.Name:
		pr.Vulnerabilities, err = decodeLayer[scanossapi.VulnerabilitiesResponse](raw, res.Failed)
	default:
		return fmt.Errorf("service %q is not a decoration layer", name)
	}
	return err
}

// decodeLayer returns nil on failure, never a layer with no response: a non-nil layer is the
// caller's guarantee that Response is safe to read.
func decodeLayer[T any](raw json.RawMessage, failed []ChunkError) (*Layer[T], error) {
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("decoding the response: %w", err)
	}
	return &Layer[T]{Response: &v, Failed: failed}, nil
}

// MarshalJSON renders the result as {"<service>": <response>, …}, where each value is that
// service's full response object — the shape callers persisting this document already read.
func (pr *PipelineResult) MarshalJSON() ([]byte, error) {
	out := make(map[string]any, 4)
	if pr.Licenses != nil {
		out[ServiceLicenses.Name] = pr.Licenses.Response
	}
	if pr.Cryptography != nil {
		out[ServiceCryptographyAlgorithms.Name] = pr.Cryptography.Response
	}
	if pr.Geoprovenance != nil {
		out[ServiceGeoprovenanceOrigin.Name] = pr.Geoprovenance.Response
	}
	if pr.Vulnerabilities != nil {
		out[ServiceVulnerabilities.Name] = pr.Vulnerabilities.Response
	}
	return json.Marshal(out)
}

// String returns the pretty-printed keyed JSON.
func (pr *PipelineResult) String() string {
	b, err := json.MarshalIndent(pr, "", "  ")
	if err != nil {
		return fmt.Sprintf("<scanoss.PipelineResult: %v>", err)
	}
	return string(b)
}
