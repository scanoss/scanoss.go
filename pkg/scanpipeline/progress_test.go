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

package scanpipeline

import (
	"testing"

	scanossapi "github.com/scanoss/scanoss.api-sdk"
)

// poll is one scan status response.
func poll(status, phase string, done, total int) *scanossapi.ScanEnvelope {
	return &scanossapi.ScanEnvelope{
		Status:     scanossapi.ScanEnvelopeStatus(status),
		Phase:      phase,
		PhaseDone:  done,
		PhaseTotal: total,
	}
}

// track runs a sequence of polls through one tracker and returns what each produced.
func track(polls ...*scanossapi.ScanEnvelope) []Progress {
	var t scanTracker
	out := make([]Progress, 0, len(polls))
	for _, e := range polls {
		out = append(out, t.progress(e))
	}
	return out
}

// The server scans in passes and restarts its counter on each, so the numbers it reports cannot
// drive a bar directly. This is the sequence a 7586-file scan actually produced: the raw counters
// ran 7586/7586 -> 194/1314 -> 13/13, which as one counter reads 100% -> 2% -> 0%.
func TestScanProgressNeverGoesBackwards(t *testing.T) {
	got := track(
		poll("scanning", "Pass 1: scan files", 337, 7586),
		poll("scanning", "Pass 1: scan files", 7586, 7586),
		poll("scanning", "Pass 2: scan snippets", 194, 1314),
		poll("scanning", "Pass 2: scan snippets", 1253, 1314),
		poll("scanning", "Final: fetch components", 13, 13),
		poll("completed", "", 0, 0),
	)

	prev := 0
	for i, p := range got {
		if p.Done < prev {
			t.Errorf("poll %d: went backwards, %d after %d (all: %v)", i, p.Done, prev, dones(got))
		}
		prev = p.Done
	}
	if last := got[len(got)-1]; last.Done != 100 || last.Status != StatusCompleted {
		t.Errorf("final = %d/%s, want 100/completed (all: %v)", last.Done, last.Status, dones(got))
	}
}

// Each pass owns an equal share of the range, so a pass that finishes lands on its boundary no
// matter which unit it counted in — files, then a subset of those files, then lookup batches.
func TestScanProgressPassBoundaries(t *testing.T) {
	got := dones(track(
		poll("scanning", "Pass 1: scan files", 7586, 7586),
		poll("scanning", "Pass 2: scan snippets", 1314, 1314),
		poll("scanning", "Final: fetch components", 13, 13),
	))
	want := []int{50, 100, 100}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("pass %d ended at %d, want %d (all: %v)", i+1, got[i], want[i], got)
		}
	}
}

// Progress is sampled by polling, not streamed, so a pass shorter than the interval is never seen.
// A 3-file scan reported only its last pass. The status is what completes the layer; the counters,
// having missed what they missed, never would.
func TestScanProgressToleratesMissedPasses(t *testing.T) {
	got := track(
		poll("scanning", "Final: fetch components", 1, 1),
		poll("completed", "", 0, 0),
	)
	if got[0].Done != 50 {
		t.Errorf("the only pass seen reported %d, want 50 (it occupies the first segment)", got[0].Done)
	}
	if got[1].Done != 100 || got[1].Status != StatusCompleted {
		t.Errorf("completion reported %d/%s, want 100/completed", got[1].Done, got[1].Status)
	}
}

// A queued scan has nothing to count yet, and must not be mistaken for one making no progress.
func TestScanProgressQueued(t *testing.T) {
	got := track(poll("queued", "", 0, 0))[0]
	if got.Status != StatusPending {
		t.Errorf("status = %q, want pending", got.Status)
	}
	if got.Done != 0 {
		t.Errorf("done = %d, want 0: a queued scan has not started", got.Done)
	}
}

// A fourth pass would share the last segment and restart inside it; a server that reports no pass
// name keeps every poll in the first. Neither may drag the layer backwards.
func TestScanProgressExtraPassDoesNotRestart(t *testing.T) {
	got := dones(track(
		poll("scanning", "Pass 1", 1, 1),
		poll("scanning", "Pass 2", 1, 1),
		poll("scanning", "Pass 3", 1, 1),
		poll("scanning", "Pass 4", 0, 100),
	))
	if got[3] < got[2] {
		t.Errorf("a fourth pass dropped the layer from %d to %d (all: %v)", got[2], got[3], got)
	}
}

func TestScanProgressWithoutPassNames(t *testing.T) {
	got := dones(track(
		poll("scanning", "", 25, 100),
		poll("scanning", "", 50, 100),
		poll("scanning", "", 100, 100),
	))
	want := []int{12, 25, 50}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("poll %d = %d, want %d: an unnamed pass stays in the first segment (all: %v)", i, got[i], want[i], got)
		}
	}
}

// A failed scan keeps whatever it had reached: inventing completion would say the work finished.
func TestScanProgressFailedKeepsItsPosition(t *testing.T) {
	got := track(
		poll("scanning", "Pass 1: scan files", 3793, 7586),
		poll("failed", "", 0, 0),
	)
	if got[1].Status != StatusFailed {
		t.Errorf("status = %q, want failed", got[1].Status)
	}
	if got[1].Done != got[0].Done {
		t.Errorf("done = %d, want %d: a failure holds its position rather than completing", got[1].Done, got[0].Done)
	}
}

// Reporter is the one translation from the SDK's stages to layers, shared by Run and by the
// callers that drive the SDK themselves.
func TestReporterTranslatesEachStage(t *testing.T) {
	var got []Progress
	r := NewReporter(func(u Progress) { got = append(got, u) })

	r.Fingerprinting(4210, 7586)
	r.Uploading(3, 12)
	r.Scanning(*poll("scanning", "Pass 1: scan files", 7586, 7586))
	r.Decorating("vulnerabilities", 20, 47)

	want := []Progress{
		{Layer: LayerFingerprint, Status: StatusRunning, Done: 4210, Total: 7586},
		{Layer: LayerUpload, Status: StatusRunning, Done: 3, Total: 12},
		{Layer: LayerScan, Status: StatusRunning, Done: 50, Total: 100},
		{Layer: "vulnerabilities", Status: StatusRunning, Done: 20, Total: 47},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d updates, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("update %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// A nil callback must be a no-op rather than a panic: OnProgress is optional throughout.
func TestReporterWithoutCallback(t *testing.T) {
	NewReporter(nil).Uploading(1, 2)
}

func dones(ps []Progress) []int {
	out := make([]int, len(ps))
	for i, p := range ps {
		out[i] = p.Done
	}
	return out
}

// The server's third pass has only ever been seen arriving already completed — it is shorter than
// the poll interval, so no poll catches it running. Reserving a segment for it left the last part
// of the bar unreachable: the bar stopped two thirds along and jumped to done.
func TestScanProgressReachesTheEndBeforeCompleting(t *testing.T) {
	got := track(
		poll("scanning", "Pass 1: scan files", 7586, 7586),
		poll("scanning", "Pass 2: scan snippets", 1300, 1314),
		poll("completed", "Final: fetch components", 13, 13),
	)
	if before := got[1].Done; before < 95 {
		t.Errorf("the bar was at %d when the last observable pass ended, want it near the end", before)
	}
	if last := got[2]; last.Done != 100 || last.Status != StatusCompleted {
		t.Errorf("completion reported %d/%s, want 100/completed", last.Done, last.Status)
	}
}
