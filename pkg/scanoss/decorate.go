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
	"net/url"
)

// ChunkError records the failure of a single chunk request.
type ChunkError struct {
	// Index is the zero-based position of the failed chunk.
	Index int
	// Err is the underlying error.
	Err error
}

func (e ChunkError) Error() string { return fmt.Sprintf("chunk %d: %v", e.Index, e.Err) }

// Result holds the outcome of a chunked, multi-request query. Successful chunk
// responses are kept in input order; any per-chunk failures are reported in
// Failed (a partial result is still returned as long as at least one chunk
// succeeds).
type Result struct {
	responses []json.RawMessage
	// Failed lists the chunks that did not succeed, if any.
	Failed []ChunkError
}

// Responses returns the raw JSON body of each successful chunk, in input order.
func (r *Result) Responses() []json.RawMessage { return r.responses }

// Merged combines the chunk responses into a single JSON document, concatenating
// top-level array fields (e.g. "components") across chunks and keeping the last
// value seen for scalar/object fields (e.g. "status").
func (r *Result) Merged() (json.RawMessage, error) {
	return mergeResponses(r.responses)
}

// Unmarshal decodes the merged JSON document into v.
func (r *Result) Unmarshal(v interface{}) error {
	merged, err := r.Merged()
	if err != nil {
		return err
	}
	return json.Unmarshal(merged, v)
}

// String returns the merged document pretty-printed. It implements fmt.Stringer
// on a best-effort basis (returns an error marker if merging fails).
func (r *Result) String() string {
	merged, err := r.Merged()
	if err != nil {
		return fmt.Sprintf("<scanoss.Result: merge error: %v>", err)
	}
	var v interface{}
	if err := json.Unmarshal(merged, &v); err != nil {
		return string(merged)
	}
	pretty, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return string(merged)
	}
	return string(pretty)
}

// As decodes a Result's merged JSON document into the typed response T. The
// per-service methods use it to return the generated OpenAPI models; it is also
// exported for callers that hold a raw *Result.
func As[T any](r *Result) (*T, error) {
	var v T
	if err := r.Unmarshal(&v); err != nil {
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
// per-service methods (e.g. Vulnerabilities).
func (c *Client) decorate(ctx context.Context, svc Service, components []Component) (*Result, error) {
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
	res := &Result{}
	done := 0
	for n := 0; n < len(chunks); n++ {
		r := <-resultsCh
		done += len(chunks[r.idx])
		if c.onProgress != nil {
			c.onProgress(Progress{
				Service: svc.Name,
				Done:    done,
				Total:   len(components),
				Unit:    "purls",
			})
		}
		if r.err != nil {
			res.Failed = append(res.Failed, ChunkError{Index: r.idx, Err: r.err})
			continue
		}
		res.responses = append(res.responses, r.response)
	}

	if len(res.responses) == 0 {
		if len(res.Failed) > 0 {
			return nil, fmt.Errorf("all %d chunk(s) failed: %w", len(chunks), res.Failed[0].Err)
		}
		return nil, fmt.Errorf("no results")
	}

	return res, nil
}

// decorateOne queries a single-component service: one GET with the component's
// purl (and optional requirement) as query parameters, wrapping the response in a
// *Result. It is the single path behind the singular per-service methods (e.g.
// Vulnerability). No chunking or worker pool — single is one request.
func (c *Client) decorateOne(ctx context.Context, svc Service, comp Component) (*Result, error) {
	if svc.endpoint == "" {
		return nil, fmt.Errorf("service %q has no endpoint", svc.Name)
	}
	if comp.Purl == "" {
		return nil, fmt.Errorf("no component to query")
	}

	q := url.Values{}
	q.Set("purl", comp.Purl)
	if comp.Requirement != "" {
		q.Set("requirement", comp.Requirement)
	}
	return c.getResult(ctx, svc.endpoint, q)
}

// getResult issues a GET to endpoint with the given query parameters and wraps the
// raw body in a *Result. It is the single-shot GET path shared by decorateOne and
// by non-component GET endpoints (e.g. license details/obligations).
func (c *Client) getResult(ctx context.Context, endpoint string, query url.Values) (*Result, error) {
	u := c.apiURL + endpoint
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	body, _, err := c.transport.do(ctx, req)
	if err != nil {
		return nil, err
	}
	return &Result{responses: []json.RawMessage{json.RawMessage(body)}}, nil
}

// postComponents sends one chunk of components as a ComponentsRequest body (POST)
// and returns the raw response. Used by the batch engine (decorate).
func (c *Client) postComponents(ctx context.Context, endpoint string, components []Component) (json.RawMessage, error) {
	body, err := json.Marshal(componentsRequest{Components: components})
	if err != nil {
		return nil, fmt.Errorf("error marshaling request body: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.apiURL+endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	b, _, err := c.transport.do(ctx, req)
	return json.RawMessage(b), err
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
