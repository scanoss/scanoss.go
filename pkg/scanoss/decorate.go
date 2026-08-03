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
	"net/url"
	"strings"
)

// ChunkError records a batch of components the service never answered for. It names the
// components rather than the batch: chunking is this SDK's own arithmetic, so a batch number
// tells a caller nothing it can act on, while the PURLs say exactly which components came back
// without data.
type ChunkError struct {
	// Purls are the components left without data.
	Purls []string
	// Err is the underlying error.
	Err error
}

// Error names the components, not just how many. Most callers only ever see this string — logged,
// or wrapped with %w — and a bare count is exactly what the batch index already told them.
//
// The list is bounded because a chunk holds ChunkSize components and a caller may raise that: past
// the first few the full set is on Purls, rather than in every log line.
func (e ChunkError) Error() string {
	const shown = 10
	purls, more := e.Purls, ""
	if len(purls) > shown {
		purls, more = purls[:shown], fmt.Sprintf(" (+%d more)", len(e.Purls)-shown)
	}
	return fmt.Sprintf("%d component(s) unanswered [%s%s]: %v",
		len(e.Purls), strings.Join(purls, ", "), more, e.Err)
}

// result holds the outcome of a chunked, multi-request query: one raw body per chunk that
// succeeded, in input order, plus the chunks that did not. A partial result is still returned as
// long as one chunk succeeded — the API never sends a document of this shape, the SDK assembles
// it because one logical query is several requests.
//
// Unexported, along with as and merged: no exported function hands one out, so a caller outside
// this package has no way to obtain one.
type result struct {
	responses []json.RawMessage
	failed    []ChunkError
}

// merged combines the chunk responses into a single JSON document, concatenating top-level array
// fields (e.g. "components") across chunks and keeping the last value seen for scalar/object
// fields (e.g. "status").
func (r *result) merged() (json.RawMessage, error) {
	return mergeResponses(r.responses)
}

// as decodes the merged JSON document into the typed response T. Every per-service method ends
// in a call to it, which is how they return the generated OpenAPI models.
func as[T any](r *result) (*T, error) {
	merged, err := r.merged()
	if err != nil {
		return nil, err
	}
	var v T
	if err := json.Unmarshal(merged, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// purlsOf returns the PURLs of the given components, for diagnostic logging.
func purlsOf(components []Component) []string {
	purls := make([]string, len(components))
	for i, comp := range components {
		purls[i] = comp.Purl
	}
	return purls
}

// decorate splits components into chunks and queries the given batch service
// concurrently, merging the responses. It is the batch engine behind the plural
// per-service methods (e.g. client.Vulnerabilities.Components).
func (c *Client) decorate(ctx context.Context, svc Service, components []Component, opts ...DecorateOption) (*result, error) {
	o := resolveDecorateOptions(opts)
	if svc.endpoint == "" {
		return nil, fmt.Errorf("service %q has no endpoint", svc.Name)
	}
	if len(components) == 0 {
		return nil, fmt.Errorf("no components to query")
	}

	c.log.Debug("decorating components", "service", svc.Name, "count", len(components), "purls", purlsOf(components))

	chunks := chunk(components, c.chunkSize)

	// Never spawn more workers than there are chunks.
	workers := c.workers
	if len(chunks) < workers {
		workers = len(chunks)
	}
	if workers < 1 {
		workers = 1
	}

	type job struct {
		idx   int
		comps []Component
	}
	type chunkResult struct {
		idx      int
		response json.RawMessage
		err      error
	}

	jobs := make(chan job, len(chunks))
	resultsCh := make(chan chunkResult, len(chunks))

	// Start a fixed pool of workers; each drains the jobs channel. Every job
	// emits exactly one result so the collector always receives len(chunks)
	// values. If the context is cancelled, remaining jobs skip the network call
	// and fail fast instead of draining the queue with real requests.
	for w := 0; w < workers; w++ {
		go func() {
			for j := range jobs {
				select {
				case <-ctx.Done():
					resultsCh <- chunkResult{idx: j.idx, err: ctx.Err()}
					continue
				default:
				}
				resp, err := c.postComponents(ctx, svc.endpoint, j.comps)
				resultsCh <- chunkResult{idx: j.idx, response: resp, err: err}
			}
		}()
	}

	// Queue every chunk as a job, then close so workers exit once drained.
	for i, ch := range chunks {
		jobs <- job{idx: i, comps: ch}
	}
	close(jobs)

	// Collect exactly one result per chunk. Order does not matter.
	res := &result{}
	done := 0
	for n := 0; n < len(chunks); n++ {
		r := <-resultsCh
		done += len(chunks[r.idx])
		if o.reporter != nil {
			o.reporter.Decorating(svc.Name, done, len(components))
		}
		if r.err != nil {
			res.failed = append(res.failed, ChunkError{Purls: purlsOf(chunks[r.idx]), Err: r.err})
			continue
		}
		res.responses = append(res.responses, r.response)
	}

	if len(res.responses) == 0 {
		if len(res.failed) > 0 {
			return nil, fmt.Errorf("all %d chunk(s) failed: %w", len(chunks), res.failed[0].Err)
		}
		return nil, fmt.Errorf("no results")
	}

	return res, nil
}

// decorateOne queries a single-component service: one GET with the component's
// purl (and optional requirement) as query parameters, wrapping the response in a
// *Result. It is the single path behind the singular per-service methods (e.g.
// client.Vulnerabilities.Component). No chunking or worker pool — single is one request.
func (c *Client) decorateOne(ctx context.Context, svc Service, comp Component, opts ...DecorateOption) (*result, error) {
	if svc.endpoint == "" {
		return nil, fmt.Errorf("service %q has no endpoint", svc.Name)
	}
	if comp.Purl == "" {
		return nil, fmt.Errorf("no component to query")
	}
	o := resolveDecorateOptions(opts)

	q := url.Values{}
	q.Set("purl", comp.Purl)
	if comp.Requirement != "" {
		q.Set("requirement", comp.Requirement)
	}
	res, err := c.getResult(ctx, svc.endpoint, q)
	// One component is one unit of work: report it done so a caller drawing a bar sees the same
	// shape here as from the batch path, rather than a bar that never moves.
	if err == nil && o.reporter != nil {
		o.reporter.Decorating(svc.Name, 1, 1)
	}
	return res, err
}

// getResult issues a GET to endpoint with the given query parameters and wraps the
// raw body in a *Result. It is the single-shot GET path shared by decorateOne and
// by non-component GET endpoints (e.g. license details/obligations).
func (c *Client) getResult(ctx context.Context, endpoint string, query url.Values) (*result, error) {
	body, err := c.get(ctx, endpoint, query)
	if err != nil {
		return nil, err
	}
	return &result{responses: []json.RawMessage{json.RawMessage(body)}}, nil
}

// postComponents sends one chunk of components as a ComponentsRequest body (POST)
// and returns the raw response. Used by the batch engine (decorate).
func (c *Client) postComponents(ctx context.Context, endpoint string, components []Component) (json.RawMessage, error) {
	body, err := c.postJSON(ctx, endpoint, componentsRequest{Components: components})
	return json.RawMessage(body), err
}

// chunk splits components into batches of at most size. size <= 0 yields a
// single chunk with all components.
func chunk(components []Component, size int) [][]Component {
	if size <= 0 {
		if len(components) == 0 {
			return nil
		}
		return [][]Component{components}
	}
	chunks := make([][]Component, 0, (len(components)+size-1)/size)
	for start := 0; start < len(components); start += size {
		end := start + size
		if end > len(components) {
			end = len(components)
		}
		chunks = append(chunks, components[start:end])
	}
	return chunks
}

// mergeResponses concatenates top-level array fields across chunk responses.
func mergeResponses(responses []json.RawMessage) (json.RawMessage, error) {
	if len(responses) == 0 {
		return json.RawMessage("{}"), nil
	}
	if len(responses) == 1 {
		return responses[0], nil
	}

	merged := make(map[string]json.RawMessage)
	arrays := make(map[string][]json.RawMessage)

	for _, resp := range responses {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(resp, &obj); err != nil {
			return nil, fmt.Errorf("error parsing chunk response: %w", err)
		}
		for key, raw := range obj {
			if elems, ok := asArray(raw); ok {
				arrays[key] = append(arrays[key], elems...)
				continue
			}
			merged[key] = raw
		}
	}

	for key, elems := range arrays {
		combined, err := json.Marshal(elems)
		if err != nil {
			return nil, fmt.Errorf("error merging array field %q: %w", key, err)
		}
		merged[key] = combined
	}

	out, err := json.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("error marshaling merged JSON: %w", err)
	}
	return out, nil
}

// asArray reports whether raw is a JSON array and returns its elements.
func asArray(raw json.RawMessage) ([]json.RawMessage, bool) {
	var elems []json.RawMessage
	if err := json.Unmarshal(raw, &elems); err != nil {
		return nil, false
	}
	return elems, true
}
