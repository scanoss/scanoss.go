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
	"fmt"
	"net/http"
	"time"
)

// ServiceScan is the v3 batch scan endpoint. WFP fingerprints are uploaded as
// octet-stream byte ranges (Content-Range); the server assigns a scan id and
// queues the scan once all bytes are received.
var ServiceScan = Service{Name: "scan", endpoint: "/v3/wfp/scan"}

// DefaultScanPollInterval is the cadence for polling the scan status endpoint
// when the caller does not override it with WithPollInterval.
//
// The server reports progress per pass, and polling samples it rather than streaming it: at a
// slower cadence a whole pass can come and go between two polls, leaving a progress display
// frozen for stretches that look like a hang.
const DefaultScanPollInterval = 2 * time.Second

// scanPollInitial is the delay before the first status poll. It is clamped to the
// poll interval when a smaller interval is set (see scanService.wait).
const scanPollInitial = 1 * time.Second

// chunkRanges splits a payload of total bytes into [off,end] (inclusive) blocks
// of at most size bytes; the final block is short. size <= 0 yields a single
// block; total <= 0 yields none.
func chunkRanges(total, size int) [][2]int {
	if total <= 0 {
		return nil
	}
	if size <= 0 {
		return [][2]int{{0, total - 1}}
	}
	ranges := make([][2]int, 0, (total+size-1)/size)
	for off := 0; off < total; off += size {
		end := off + size - 1
		if end > total-1 {
			end = total - 1
		}
		ranges = append(ranges, [2]int{off, end})
	}
	return ranges
}

// uploadChunk POSTs one WFP byte range to the scan endpoint. The client-generated
// scanID is sent on every chunk via the X-Scan-Id request header.
func (c *Client) uploadChunk(ctx context.Context, scanID string, off, end, total int, block []byte) error {
	req, err := c.newRequest(http.MethodPost, ServiceScan.endpoint, bytes.NewReader(block))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", off, end, total))
	req.Header.Set("X-Scan-Id", scanID)

	if _, _, err := c.transport.do(ctx, req); err != nil {
		return fmt.Errorf("chunk %d-%d/%d: %w", off, end, total, err)
	}
	return nil
}
