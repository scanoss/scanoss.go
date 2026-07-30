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

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"

	fingerprint "github.com/scanoss/scanoss.go/pkg/fingerprint/wfp"
)

// Client handles communications with the SCANOSS API
type Client struct {
	apiURL   string
	apiKey   string
	client   *http.Client
	strategy SendStrategy
}

// NewClient creates a new API client with DirectStrategy by default
func NewClient(apiURL, apiKey string) *Client {
	return &Client{
		apiURL:   apiURL,
		apiKey:   apiKey,
		client:   &http.Client{},
		strategy: &DirectStrategy{},
	}
}

// NewClientWithMode creates a new API client with specific mode (legacy)
// Deprecated: Use NewClientWithStrategy instead
func NewClientWithMode(apiURL, apiKey string, batchMode bool) *Client {
	var strategy SendStrategy
	if batchMode {
		strategy = &BatchStrategy{}
	} else {
		strategy = &DirectStrategy{}
	}
	return &Client{
		apiURL:   apiURL,
		apiKey:   apiKey,
		client:   &http.Client{},
		strategy: strategy,
	}
}

// NewClientWithStrategy creates a new API client with custom strategy
func NewClientWithStrategy(apiURL, apiKey string, strategy SendStrategy) *Client {
	return &Client{
		apiURL:   apiURL,
		apiKey:   apiKey,
		client:   &http.Client{},
		strategy: strategy,
	}
}

// SetHTTPClient replaces the HTTP client used for API calls, so a caller can
// supply one configured for a proxy or an internal CA. A nil client is ignored.
//
// This is the injection point for the C/Python/Node bindings, which reach the API
// through this package rather than through pkg/scanoss.
func (c *Client) SetHTTPClient(client *http.Client) {
	if client != nil {
		c.client = client
	}
}

// SetInsecureTLS disables TLS certificate verification when insecure is true.
// For self-signed or internal endpoints only — insecure, avoid in production.
func (c *Client) SetInsecureTLS(insecure bool) {
	if !insecure {
		return
	}
	// Clone rather than construct: a zero-value http.Transport has a nil Proxy,
	// which means no proxy at all — not even from HTTP_PROXY/HTTPS_PROXY — and it
	// drops Go's timeouts and connection pooling too. Building one by hand is how
	// this option used to disable proxy support as a side effect.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{}
	}
	transport.TLSClientConfig.InsecureSkipVerify = true

	c.SetHTTPClient(&http.Client{Transport: transport})
}

// SendFingerprints implements ScanClient.SendFingerprints
func (c *Client) SendFingerprints(wfp string, opts SendOptions) (*SendResponse, error) {
	// Convert SBOMContext string to *SBOMContext
	var sbom *SBOMContext
	if opts.SBOMContext != "" {
		sbom = &SBOMContext{
			Assets: opts.SBOMContext,
			Type:   "", // Type not provided in legacy opts
		}
	}

	data, err := c.sendWFP([]byte(wfp), opts.SessionID, opts.IsFinalChunk, sbom)
	if err != nil {
		return nil, err
	}

	return &SendResponse{
		Data:       data,
		StatusCode: 200,
		Immediate:  true,
	}, nil
}

// GetBatchStatus implements ScanClient.GetBatchStatus
func (c *Client) GetBatchStatus(sessionID string) (*BatchStatus, error) {
	results, status, progress, err := c.GetBatchResults(sessionID)
	if err != nil {
		return nil, err
	}

	return &BatchStatus{
		Status:   status,
		Progress: progress,
		Results:  results,
	}, nil
}

// SendFingerprint sends a single fingerprint to the API (legacy)
func (c *Client) SendFingerprint(fp *fingerprint.FileFingerprint) (string, error) {
	return c.sendWFP([]byte(fp.Fingerprint), "", false, nil)
}

// SendBatch sends a batch of combined fingerprints to the API (legacy)
func (c *Client) SendBatch(wfpContent string) (string, error) {
	return c.sendWFP([]byte(wfpContent), "", false, nil)
}

// SendBatchWithSession sends a batch with session tracking (legacy)
func (c *Client) SendBatchWithSession(wfpContent string, sessionID string, isFinalChunk bool) (string, error) {
	return c.sendWFP([]byte(wfpContent), sessionID, isFinalChunk, nil)
}

// SBOMContext represents SBOM data to be sent with the scan request
type SBOMContext struct {
	Assets string // JSON-serialized SBOM: {"components": [{"purl": "pkg:..."}, ...]}
	Type   string // "identify" or "blacklist"
}

// SendBatchWithContext sends a batch with session tracking and BOM context
func (c *Client) SendBatchWithContext(wfpContent string, sessionID string, isFinalChunk bool, sbomContext string) (string, error) {
	// Legacy support: treat sbomContext as just assets field
	var sbom *SBOMContext
	if sbomContext != "" {
		sbom = &SBOMContext{
			Assets: sbomContext,
			Type:   "", // No type for legacy calls
		}
	}
	return c.sendWFP([]byte(wfpContent), sessionID, isFinalChunk, sbom)
}

// SendBatchWithSBOM sends a batch with session tracking and SBOM context (new format)
func (c *Client) SendBatchWithSBOM(wfpContent string, sessionID string, isFinalChunk bool, sbom *SBOMContext) (string, error) {
	return c.sendWFP([]byte(wfpContent), sessionID, isFinalChunk, sbom)
}

// sendWFP sends WFP content to the API
func (c *Client) sendWFP(wfpData []byte, sessionID string, isFinalChunk bool, sbom *SBOMContext) (string, error) {
	bodyBuf := &bytes.Buffer{}
	bodyWriter := multipart.NewWriter(bodyBuf)

	// Add SBOM context as form fields BEFORE the file (compatible with scanoss.py)
	if sbom != nil {
		if sbom.Type != "" {
			if err := bodyWriter.WriteField("type", sbom.Type); err != nil {
				return "", fmt.Errorf("error writing type field: %w", err)
			}
		}
		if sbom.Assets != "" {
			if err := bodyWriter.WriteField("assets", sbom.Assets); err != nil {
				return "", fmt.Errorf("error writing assets field: %w", err)
			}
		}
	}

	// Create filename based on the SHA-256 of the content
	filename := fmt.Sprintf("%x.wfp", sha256.Sum256(wfpData))
	fileWriter, err := bodyWriter.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("error creating form file: %w", err)
	}

	// Write WFP data to form file
	_, err = fileWriter.Write(wfpData)
	if err != nil {
		return "", fmt.Errorf("error writing WFP data: %w", err)
	}

	contentType := bodyWriter.FormDataContentType()
	_ = bodyWriter.Close()

	// Build URL using strategy
	url := c.strategy.GetEndpoint(c.apiURL)

	// Create request
	req, err := http.NewRequest("POST", url, bodyBuf)
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}

	// Configure headers
	req.Header.Set("Content-Type", contentType)
	if c.apiKey != "" {
		req.Header.Set("x-api-key", c.apiKey)
		req.Header.Set("x-Session", c.apiKey)
	}

	// Add strategy-specific headers
	opts := SendOptions{
		SessionID:    sessionID,
		IsFinalChunk: isFinalChunk,
	}
	for k, v := range c.strategy.PrepareHeaders(opts) {
		req.Header.Set(k, v)
	}

	// Send request
	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response: %w", err)
	}

	// Verify status code
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return string(respBody), nil
}

type ScanStatus struct {
	Started  int64  `json:"started"`
	Status   string `json:"status"`
	Progress int    `json:"progress"`
}

func HasResults(data []byte) (bool, string, int) {
	var s ScanStatus

	// Try to detect if this is a status JSON
	if err := json.Unmarshal(data, &s); err == nil && s.Status != "" {

		switch s.Status {
		case "scanning":
			return false, "scanning", s.Progress
		case "completed":
			return true, "completed", 100
		case "failed":
			return false, "failed", s.Progress
		default:
			return false, "completed", 0
		}
	}

	// If it doesn't look like a status JSON → assume it's results
	return true, "completed", 100
}

// GetBatchResults retrieves results from a batch scan using session ID
func (c *Client) GetBatchResults(sessionID string) (string, string, int, error) {
	url := c.apiURL + "/wfp/scan/" + sessionID

	// Create request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", "failed", 0, fmt.Errorf("error creating request: %w", err)
	}

	// Configure headers
	if c.apiKey != "" {
		req.Header.Set("x-api-key", c.apiKey)
		req.Header.Set("x-Session", c.apiKey)
	}

	// Send request
	resp, err := c.client.Do(req)
	if err != nil {
		return "", "failed", 0, fmt.Errorf("error sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "failed", 0, fmt.Errorf("error reading response: %w", err)
	}
	hasRes, status, progress := HasResults(respBody)

	if hasRes {
		return string(respBody), "completed", 100, nil
	}

	return "", status, progress, nil
}
