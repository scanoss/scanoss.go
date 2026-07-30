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
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// An expired session is terminal: the scan is gone and the id cannot be retried. Wait must stop on
// it. It used to fall through to "not a terminal state, keep polling", so an expired scan was
// polled until the caller's context ended — for the CLI, until someone pressed Ctrl-C.
func TestScanWaitStopsOnExpiredSession(t *testing.T) {
	var polls int32
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&polls, 1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"scan_id": "scan-123",
			"status":  "expired",
		})
	}))

	// The deadline is the safety net: without the fix Wait never returns on its own, and the test
	// has to fail rather than hang. It is not what is being asserted — reaching it IS the failure.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := c.Scan.Wait(ctx, "scan-123", WithPollInterval(20*time.Millisecond))
	if err == nil {
		t.Fatal("Wait returned no error for an expired session")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Wait polled the expired session until the context expired (%d polls)", atomic.LoadInt32(&polls))
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("error = %q, want it to say the scan expired", err)
	}
	if n := atomic.LoadInt32(&polls); n != 1 {
		t.Errorf("polls = %d, want 1: the first expired response is already terminal", n)
	}
}

// A state this client does not know is the opposite case: a server that grew a state must not
// break a client that predates it, so polling continues until the scan reaches one this client
// does recognise.
func TestScanWaitKeepsPollingUnknownState(t *testing.T) {
	var polls int32
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		env := map[string]any{"scan_id": "scan-123", "status": "reticulating-splines"}
		if atomic.AddInt32(&polls, 1) >= 3 {
			env["status"] = "completed"
			env["result"] = json.RawMessage(`{"files":[],"components":{}}`)
		}
		_ = json.NewEncoder(w).Encode(env)
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	env, err := c.Scan.Wait(ctx, "scan-123", WithPollInterval(20*time.Millisecond))
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if env.Status != scanStateCompleted {
		t.Errorf("status = %q, want completed", env.Status)
	}
	if n := atomic.LoadInt32(&polls); n != 3 {
		t.Errorf("polls = %d, want 3: an unrecognised state means keep waiting", n)
	}
}
