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
)

// DecorationPipeline runs a configurable set of decoration services over the same
// components, in parallel, and returns one result keyed by service. Create one
// with Client.DecorationPipeline and reuse it; configure the service set with Add/Remove.
type DecorationPipeline struct {
	client   *Client
	services []Service // ordered, deduped by Service.Name

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

	pr := &PipelineResult{
		Services: make(map[string]*Result),
		Errors:   make(map[string]error),
	}
	for o := range ch {
		if o.err != nil {
			pr.Errors[o.name] = o.err
			continue
		}
		pr.Services[o.name] = o.res
	}

	if len(pr.Services) == 0 {
		return nil, fmt.Errorf("all %d pipeline service(s) failed", len(p.services))
	}
	return pr, nil
}

// PipelineResult holds each service's output, keyed by service name, plus any
// per-service failures.
type PipelineResult struct {
	Services map[string]*Result // keyed by service name; full per-service result
	Errors   map[string]error   // services that failed entirely
}

// MarshalJSON renders the result as {"<service>": <merged response>, …}, where
// each value is that service's full merged response object.
func (pr *PipelineResult) MarshalJSON() ([]byte, error) {
	out := make(map[string]json.RawMessage, len(pr.Services))
	for name, res := range pr.Services {
		merged, err := res.Merged()
		if err != nil {
			return nil, fmt.Errorf("merging %q: %w", name, err)
		}
		out[name] = merged
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
