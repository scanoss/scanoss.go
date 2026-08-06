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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	scanossapi "github.com/scanoss/scanoss.api-sdk"

	"github.com/scanoss/scanoss.go/pkg/settings"
)

func TestChunkRanges(t *testing.T) {
	tests := []struct {
		name        string
		total, size int
		want        [][2]int
	}{
		{"empty", 0, 10, nil},
		{"single-when-size-zero", 100, 0, [][2]int{{0, 99}}},
		{"exact-multiple", 10, 5, [][2]int{{0, 4}, {5, 9}}},
		{"remainder", 12, 5, [][2]int{{0, 4}, {5, 9}, {10, 11}}},
		{"smaller-than-size", 3, 10, [][2]int{{0, 2}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chunkRanges(tt.total, tt.size)
			if fmt.Sprint(got) != fmt.Sprint(tt.want) {
				t.Fatalf("chunkRanges(%d,%d) = %v, want %v", tt.total, tt.size, got, tt.want)
			}
		})
	}
}

// scanMock is a stateful fake of the v3 scan endpoint.
type scanMock struct {
	mu            sync.Mutex
	received      []byte // reassembled by Content-Range offset
	total         int
	uploads       int32
	statusCalls   int32
	completeAt    int32           // return completed once statusCalls >= this
	result        json.RawMessage // result object returned on completion
	failScan      bool
	scanID        string // client-generated id captured from incoming chunks
	missingIDPOST int32  // chunks that arrived without an X-Scan-Id header
}

func (m *scanMock) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			atomic.AddInt32(&m.uploads, 1)
			// The client generates the id and sends it on every chunk.
			id := r.Header.Get("X-Scan-Id")
			if id == "" {
				atomic.AddInt32(&m.missingIDPOST, 1)
			} else {
				m.mu.Lock()
				m.scanID = id
				m.mu.Unlock()
			}
			w.WriteHeader(http.StatusAccepted)
		case http.MethodGet:
			n := atomic.AddInt32(&m.statusCalls, 1)
			state := "scanning"
			if m.failScan {
				state = "failed"
			} else if n >= m.completeAt {
				state = "completed"
			}
			m.mu.Lock()
			id := m.scanID
			m.mu.Unlock()
			env := map[string]interface{}{
				"scan_id":     id,
				"status":      state,
				"phase":       "scanning",
				"phase_done":  1,
				"phase_total": 1,
			}
			if state == "completed" {
				env["result"] = m.result
			}
			_ = json.NewEncoder(w).Encode(env)
		}
	}
}

func newTestClient(t *testing.T, h http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return mustNew(t, Config{APIURL: srv.URL})
}

func TestScanUploadAndPoll(t *testing.T) {
	result := json.RawMessage(`{
      "files": [{"path":"/a.c","file_hash":"m","match_type":"file","matches":[{"url_hash":"h1"}]}],
      "components": {"h1": {"purls":["pkg:x/y"]}}
    }`)
	mock := &scanMock{completeAt: 2, result: result}
	c := newTestClient(t, mock.handler())

	var gotID string

	wfp := []byte("0123456789abc") // 13 bytes → ceil(13/4)=4 chunks
	// WithChunkBytes(4) per-call override forces several chunks.
	res, err := c.Scan.WFP(context.Background(), wfp, WithChunkBytes(4),
		WithScanIDNotify(func(id string) { gotID = id }))
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if gotID == "" {
		t.Fatalf("notify id is empty")
	}
	mock.mu.Lock()
	sentID := mock.scanID
	mock.mu.Unlock()
	if gotID != sentID {
		t.Fatalf("notify id = %q, but chunks carried %q", gotID, sentID)
	}
	if got := atomic.LoadInt32(&mock.missingIDPOST); got != 0 {
		t.Fatalf("%d chunk(s) arrived without an X-Scan-Id header", got)
	}
	if got := atomic.LoadInt32(&mock.uploads); got != 4 {
		t.Fatalf("uploads = %d, want 4", got)
	}
	if res.Status != "completed" {
		t.Fatalf("state = %q", res.Status)
	}
	if res.Result == nil {
		t.Fatalf("result is nil")
	}
	if len(res.Result.Files) != 1 {
		t.Fatalf("want 1 file, got %d", len(res.Result.Files))
	}
	fm := res.Result.Files[0]
	if fm.Path != "/a.c" || fm.MatchType != "file" || len(fm.Matches) != 1 || fm.Matches[0].UrlHash != "h1" {
		t.Fatalf("file match parsed wrong: %+v", fm)
	}
	if len(res.Result.Components) != 1 {
		t.Fatalf("components parsed wrong: %+v", res.Result.Components)
	}
	if _, ok := res.Result.Components["h1"]; !ok {
		t.Fatalf("component h1 missing: %+v", res.Result.Components)
	}
}

func TestScanChunkCarriesClientID(t *testing.T) {
	// Every chunk must carry the client-generated X-Scan-Id, and the notify
	// hook must fire with that same id.
	mock := &scanMock{completeAt: 1, result: json.RawMessage(`{"files":[],"components":{}}`)}
	c := newTestClient(t, mock.handler())

	var gotID string

	notify := WithScanIDNotify(func(id string) { gotID = id })
	if _, err := c.Scan.WFP(context.Background(), []byte("abc"), notify); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if got := atomic.LoadInt32(&mock.missingIDPOST); got != 0 {
		t.Fatalf("%d chunk(s) arrived without an X-Scan-Id header", got)
	}
	mock.mu.Lock()
	sentID := mock.scanID
	mock.mu.Unlock()
	if gotID == "" || gotID != sentID {
		t.Fatalf("notify id = %q, chunks carried %q", gotID, sentID)
	}
}

func TestScanNotifyAfterUpload(t *testing.T) {
	// The notify must fire only after every chunk has been uploaded, so the
	// surfaced id always points at a fully-uploaded, resumable scan.
	mock := &scanMock{completeAt: 1, result: json.RawMessage(`{"files":[],"components":{}}`)}
	c := newTestClient(t, mock.handler())

	var uploadsAtNotify int32
	notify := WithScanIDNotify(func(string) { uploadsAtNotify = atomic.LoadInt32(&mock.uploads) })

	wfp := []byte("0123456789abc") // 13 bytes → ceil(13/4)=4 chunks
	if _, err := c.Scan.WFP(context.Background(), wfp, WithChunkBytes(4), notify); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if got := atomic.LoadInt32(&mock.uploads); got != 4 {
		t.Fatalf("uploads = %d, want 4", got)
	}
	if uploadsAtNotify != 4 {
		t.Fatalf("notify fired after %d of 4 uploads; id surfaced before upload finished", uploadsAtNotify)
	}
}

func TestScanNoNotifyOnFailedUpload(t *testing.T) {
	// A failed upload leaves nothing resumable, so the notify must not fire.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			http.Error(w, "boom", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)
	c := mustNew(t, Config{APIURL: srv.URL})

	notified := false
	notify := WithScanIDNotify(func(string) { notified = true })

	if _, err := c.Scan.WFP(context.Background(), []byte("abc"), notify); err == nil {
		t.Fatal("want upload error, got nil")
	}
	if notified {
		t.Fatal("notify fired despite a failed upload")
	}
}

func TestScanFailed(t *testing.T) {
	mock := &scanMock{completeAt: 1, failScan: true}
	c := newTestClient(t, mock.handler())
	_, err := c.Scan.WFP(context.Background(), []byte("abc"))
	if err == nil || !strings.Contains(err.Error(), "failed") {
		t.Fatalf("want failed error, got %v", err)
	}
}

func TestScanContextCancel(t *testing.T) {
	mock := &scanMock{completeAt: 1 << 30} // never completes
	c := newTestClient(t, mock.handler())
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	_, err := c.Scan.WFP(ctx, []byte("abc"))
	if err == nil {
		t.Fatalf("want context error, got nil")
	}
}

func TestScanFolder(t *testing.T) {
	// Folder collects + fingerprints + scans a real directory.
	dir := t.TempDir()
	src := "#include <stdio.h>\n" + strings.Repeat("int compute(int x) { return x * 42 + 7; }\n", 30)
	if err := os.WriteFile(filepath.Join(dir, "main.c"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	mock := &scanMock{completeAt: 1, result: json.RawMessage(`{"files":[],"components":{}}`)}
	c := newTestClient(t, mock.handler())

	res, err := c.Scan.Folder(context.Background(), dir)
	if err != nil {
		t.Fatalf("Folder: %v", err)
	}
	if res.Status != "completed" || res.Result == nil {
		t.Fatalf("unexpected envelope: state=%q result=%v", res.Status, res.Result)
	}
	if atomic.LoadInt32(&mock.uploads) == 0 {
		t.Fatalf("expected at least one chunk upload")
	}
}

func TestParseScanEnvelope_Unexpected(t *testing.T) {
	// A body without a status field is an unexpected response.
	if _, err := parseScanEnvelope([]byte(`{"foo":1}`)); err == nil {
		t.Fatalf("want error for missing status, got nil")
	}
}

func TestScanAppliesBOM(t *testing.T) {
	// Two matched files; a remove rule targets /drop.c's component.
	result := json.RawMessage(`{
      "files": [
        {"path":"/keep.c","file_hash":"a","match_type":"file","matches":[{"url_hash":"h1"}]},
        {"path":"/drop.c","file_hash":"b","match_type":"file","matches":[{"url_hash":"h2"}]}
      ],
      "components": {"h1": {"purls":["pkg:a/keep"]}, "h2": {"purls":["pkg:b/drop"]}}
    }`)
	c := newTestClient(t, (&scanMock{completeAt: 1, result: result}).handler())

	bom := &settings.BOM{Remove: []settings.BOMEntry{{Purl: "pkg:b/drop"}}}
	res, err := c.Scan.WFP(context.Background(), []byte("abc"), WithBOM(bom))
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if res.Result == nil {
		t.Fatalf("result is nil")
	}

	var keep, drop *scanossapi.FileResult
	for i := range res.Result.Files {
		switch res.Result.Files[i].Path {
		case "/keep.c":
			keep = &res.Result.Files[i]
		case "/drop.c":
			drop = &res.Result.Files[i]
		}
	}
	if keep == nil || keep.MatchType != "file" {
		t.Errorf("/keep.c should be intact: %+v", keep)
	}
	if drop == nil || drop.MatchType != "none" || len(drop.Matches) != 0 {
		t.Errorf("/drop.c should be neutralized: %+v", drop)
	}
	if len(res.Result.Components) != 1 {
		t.Errorf("orphaned component should be pruned: %+v", res.Result.Components)
	}
	if _, ok := res.Result.Components["h1"]; !ok {
		t.Errorf("h1 should remain: %+v", res.Result.Components)
	}
}

func TestWithPollInterval(t *testing.T) {
	if o := resolveScanOptions(nil); o.pollInterval != DefaultScanPollInterval {
		t.Fatalf("default pollInterval = %v, want %v", o.pollInterval, DefaultScanPollInterval)
	}
	if o := resolveScanOptions([]ScanOption{WithPollInterval(2 * time.Second)}); o.pollInterval != 2*time.Second {
		t.Fatalf("pollInterval = %v, want 2s", o.pollInterval)
	}
	if o := resolveScanOptions([]ScanOption{WithPollInterval(0)}); o.pollInterval != DefaultScanPollInterval {
		t.Fatalf("d=0 should be ignored, got %v", o.pollInterval)
	}
	if o := resolveScanOptions([]ScanOption{WithPollInterval(-1)}); o.pollInterval != DefaultScanPollInterval {
		t.Fatalf("negative should be ignored, got %v", o.pollInterval)
	}
}

func TestScanPollIntervalOverride(t *testing.T) {
	// Wait needs 3 status polls to complete. With a 20ms override the whole wait
	// finishes well under a second; at the 2s default it would take >4s. A generous
	// 2s bound proves the override (and the initial-delay clamp) are honored without
	// being flaky.
	mock := &scanMock{completeAt: 3, result: json.RawMessage(`{"files":[],"components":{}}`)}
	c := newTestClient(t, mock.handler())

	start := time.Now()
	env, err := c.Scan.Wait(context.Background(), "scan-123", WithPollInterval(20*time.Millisecond))
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if env.Status != scanStateCompleted {
		t.Fatalf("state = %q, want completed", env.Status)
	}
	if got := atomic.LoadInt32(&mock.statusCalls); got != 3 {
		t.Fatalf("statusCalls = %d, want 3", got)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("Wait took %v; WithPollInterval override not honored (default 2s cadence)", elapsed)
	}
}

func TestScanWithoutBOMUnchanged(t *testing.T) {
	// A rule would match, but no WithBOM option is passed → result untouched.
	result := json.RawMessage(`{
      "files": [{"path":"/drop.c","file_hash":"b","match_type":"file","matches":[{"url_hash":"h2"}]}],
      "components": {"h2": {"purls":["pkg:b/drop"]}}
    }`)
	c := newTestClient(t, (&scanMock{completeAt: 1, result: result}).handler())

	res, err := c.Scan.WFP(context.Background(), []byte("abc"))
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(res.Result.Files) != 1 || res.Result.Files[0].MatchType != "file" {
		t.Errorf("result should be unchanged without WithBOM: %+v", res.Result.Files)
	}
	if len(res.Result.Components) != 1 {
		t.Errorf("components should be unchanged: %+v", res.Result.Components)
	}
}

// The scenario of issue #64: one chunk of a multi-chunk upload fails transiently. The
// scan must survive it, and every byte range must still reach the server exactly once —
// a retry that skipped or duplicated a range would leave the WFP unscannable.
//
// The chunk fails with a 503 rather than a dropped connection on purpose: Go's own
// transport never retries a status, so a scan that completes here proves the SDK
// retried, not net/http. The network-error path is covered at the transport level by
// TestTransportRetriesOnNetworkError.
func TestScanRetriesFailedChunk(t *testing.T) {
	var mu sync.Mutex
	accepted := map[string]int{} // Content-Range → times accepted
	var posts, failed int32
	var scanID atomic.Value

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			id, _ := scanID.Load().(string)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"scan_id": id, "status": "completed", "phase": "done",
				"phase_done": 1, "phase_total": 1,
				"result": json.RawMessage(`{"files":[],"components":{}}`),
			})
			return
		}
		atomic.AddInt32(&posts, 1)
		// Fail the first chunk once, then accept everything.
		if atomic.AddInt32(&failed, 1) == 1 {
			http.Error(w, "briefly unavailable", http.StatusServiceUnavailable)
			return
		}
		scanID.Store(r.Header.Get("X-Scan-Id"))
		mu.Lock()
		accepted[r.Header.Get("Content-Range")]++
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(srv.Close)

	// One worker so the failing chunk is deterministic; a tiny backoff to stay fast.
	c := mustNew(t, Config{APIURL: srv.URL, Workers: 1, RetryBackoffBase: time.Millisecond})
	env, err := c.Scan.WFP(context.Background(), []byte(strings.Repeat("x", 300)), WithChunkBytes(100))
	if err != nil {
		t.Fatalf("a scan must survive one transient chunk failure: %v", err)
	}
	if env.Status != "completed" {
		t.Fatalf("status = %q, want completed", env.Status)
	}

	mu.Lock()
	defer mu.Unlock()
	want := []string{"bytes 0-99/300", "bytes 100-199/300", "bytes 200-299/300"}
	for _, r := range want {
		if accepted[r] != 1 {
			t.Errorf("range %q accepted %d times, want exactly 1", r, accepted[r])
		}
	}
	if len(accepted) != len(want) {
		t.Errorf("ranges accepted = %v, want exactly %v", accepted, want)
	}
	if got := atomic.LoadInt32(&posts); got != 4 { // 3 chunks + the one that failed
		t.Errorf("POSTs = %d, want 4", got)
	}
}

// The upload is only half the exposure: a blip while polling would discard a scan the
// server has already accepted and is working on.
func TestScanRetriesFailedStatusPoll(t *testing.T) {
	var polls int32
	var scanID atomic.Value

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			scanID.Store(r.Header.Get("X-Scan-Id"))
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if atomic.AddInt32(&polls, 1) == 1 {
			http.Error(w, "briefly unavailable", http.StatusServiceUnavailable)
			return
		}
		id, _ := scanID.Load().(string)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"scan_id": id, "status": "completed", "phase": "done",
			"phase_done": 1, "phase_total": 1,
			"result": json.RawMessage(`{"files":[],"components":{}}`),
		})
	}))
	t.Cleanup(srv.Close)

	c := mustNew(t, Config{APIURL: srv.URL, RetryBackoffBase: time.Millisecond})
	env, err := c.Scan.WFP(context.Background(), []byte("abc"), WithPollInterval(10*time.Millisecond))
	if err != nil {
		t.Fatalf("a scan must survive one transient status-poll failure: %v", err)
	}
	if env.Status != "completed" {
		t.Fatalf("status = %q, want completed", env.Status)
	}
	if got := atomic.LoadInt32(&polls); got != 2 {
		t.Errorf("polls = %d, want 2 (one retried)", got)
	}
}

// The run that surfaced this: a proxy rewrote a successful chunk response to 503, so the
// SDK retried a chunk the server had already accepted and got 409 RANGE_CONFLICT. A 409
// means the upload is done, not broken — the scan must complete, not fail.
func TestScanChunkConflictCountsAsUploaded(t *testing.T) {
	var posts int32
	var scanID atomic.Value

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			id, _ := scanID.Load().(string)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"scan_id": id, "status": "completed", "phase": "done",
				"phase_done": 1, "phase_total": 1,
				"result": json.RawMessage(`{"files":[],"components":{}}`),
			})
			return
		}
		scanID.Store(r.Header.Get("X-Scan-Id"))
		// The first POST lands, but the caller is told 503 — the response was mangled or
		// lost. Every later POST for the same range is therefore a duplicate.
		if atomic.AddInt32(&posts, 1) == 1 {
			http.Error(w, "rewritten by a proxy", http.StatusServiceUnavailable)
			return
		}
		http.Error(w, `{"error":{"code":"RANGE_CONFLICT","message":"upload already complete"}}`,
			http.StatusConflict)
	}))
	t.Cleanup(srv.Close)

	notified := ""
	c := mustNew(t, Config{APIURL: srv.URL, RetryBackoffBase: time.Millisecond})
	env, err := c.Scan.WFP(context.Background(), []byte("abc"),
		WithPollInterval(10*time.Millisecond),
		WithScanIDNotify(func(id string) { notified = id }))
	if err != nil {
		t.Fatalf("a 409 means the upload completed, not that the scan failed: %v", err)
	}
	if env.Status != "completed" {
		t.Fatalf("status = %q, want completed", env.Status)
	}
	// The id must still be surfaced: the session it names is resumable.
	if notified == "" {
		t.Error("notify never fired, so the completed session was not resumable")
	}
	if got := atomic.LoadInt32(&posts); got != 2 {
		t.Errorf("POSTs = %d, want 2 (the 503 then the 409)", got)
	}
}
