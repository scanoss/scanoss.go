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

package api

// SendStrategy defines how to send fingerprints based on operation mode
type SendStrategy interface {
	// GetEndpoint returns the specific endpoint for this mode
	GetEndpoint(baseURL string) string

	// PrepareHeaders prepares mode-specific headers
	PrepareHeaders(opts SendOptions) map[string]string

	// NeedsSessionID indicates if this strategy requires a Session ID
	NeedsSessionID() bool
}

// DirectStrategy implements the strategy for direct mode (synchronous)
// In this mode, each request gets an immediate response
type DirectStrategy struct{}

// GetEndpoint returns the endpoint for direct mode
func (s *DirectStrategy) GetEndpoint(baseURL string) string {
	return baseURL + "/scan/direct"
}

// PrepareHeaders does not add special headers in direct mode
func (s *DirectStrategy) PrepareHeaders(opts SendOptions) map[string]string {
	return map[string]string{} // No special headers
}

// NeedsSessionID indicates that direct mode does not need session tracking
func (s *DirectStrategy) NeedsSessionID() bool {
	return false
}

// BatchStrategy implements the strategy for batch mode (asynchronous)
// In this mode, multiple chunks are sent with the same Session-Id
// and results are retrieved later via polling
type BatchStrategy struct{}

// GetEndpoint returns the endpoint for batch mode
func (s *BatchStrategy) GetEndpoint(baseURL string) string {
	return baseURL + "/wfp/scan"
}

// PrepareHeaders prepares batch protocol-specific headers
func (s *BatchStrategy) PrepareHeaders(opts SendOptions) map[string]string {
	headers := make(map[string]string)

	// Session-Id: groups multiple requests in a session
	if opts.SessionID != "" {
		headers["Session-Id"] = opts.SessionID
	}

	// X-Final-Chunk: indicates if this is the last chunk of the session
	if opts.IsFinalChunk {
		headers["X-Final-Chunk"] = "true"
	} else {
		headers["X-Final-Chunk"] = "false"
	}

	// Additional headers if provided
	for k, v := range opts.Metadata {
		headers[k] = v
	}

	return headers
}

// NeedsSessionID indicates that batch mode requires session tracking
func (s *BatchStrategy) NeedsSessionID() bool {
	return true
}
