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
	"encoding/json"
	"fmt"

	scanossapi "github.com/scanoss/scanoss.api-sdk"
)

// Scan states reported by GET /v3/wfp/scan/{id}.
//
// The first three are terminal and stop the wait loop; the last three mean the scan is still
// moving. A value in neither group is treated as still moving too — a server that grows a state
// must not break a client that predates it.
const (
	scanStateCompleted = "completed"
	scanStateFailed    = "failed"
	scanStateExpired   = "expired"

	scanStateQueued    = "queued"
	scanStateUploading = "uploading"
	scanStateScanning  = "scanning"
)

// parseScanEnvelope unmarshals the response body into a scanossapi.ScanEnvelope.
// It errors when the body carries no status field (an unexpected response),
// mirroring the reference client's guard.
func parseScanEnvelope(body []byte) (scanossapi.ScanEnvelope, error) {
	var e scanossapi.ScanEnvelope
	if err := json.Unmarshal(body, &e); err != nil {
		return scanossapi.ScanEnvelope{}, fmt.Errorf("error parsing scan response: %w", err)
	}
	if e.Status == "" {
		return scanossapi.ScanEnvelope{}, fmt.Errorf("unexpected scan response: %s", string(body))
	}
	return e, nil
}
