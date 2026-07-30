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
	scanossapi "github.com/scanoss/scanoss.api-sdk"
	"os"
	"path/filepath"
	"testing"
)

// The fixtures in testdata/ are verbatim GET /v3/wfp/scan/{id} envelopes captured
// from the v3 batch server, one per match_type the API emits. They guard the
// result-body parsing (scanossapi.ScanEnvelope/scanossapi.ScanResult and friends) against schema drift:
// if the server shape changes in a way parseScanEnvelope can't represent, these break.

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

// TestParseScanEnvelope_None: a scan with no matches. The server omits the
// "server" block entirely, so its fields must parse as zero values (not an error).
func TestParseScanEnvelope_None(t *testing.T) {
	e, err := parseScanEnvelope(readFixture(t, "scan_envelope_none.json"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if e.Status != scanStateCompleted {
		t.Fatalf("state = %q, want %q", e.Status, scanStateCompleted)
	}
	if e.Result == nil {
		t.Fatal("result is nil")
	}
	if len(e.Result.Files) != 1 {
		t.Fatalf("files = %d, want 1", len(e.Result.Files))
	}
	f := e.Result.Files[0]
	if f.Path != "scan.go" || f.MatchType != "none" || len(f.Matches) != 0 {
		t.Fatalf("file parsed wrong: %+v", f)
	}
	if len(e.Result.Components) != 0 {
		t.Fatalf("components = %d, want 0", len(e.Result.Components))
	}
	// The server block is absent in this response; the pointer must be nil so it
	// serializes as omitted (not an empty "server":{} object).
	if e.Result.Server != nil {
		t.Fatalf("expected omitted (nil) server block, got %+v", e.Result.Server)
	}
}

// TestParseScanEnvelope_File: full-file matches. Each file carries multiple
// url_hash matches (no snippet fields), and every hash joins into the component
// catalog with its full metadata.
func TestParseScanEnvelope_File(t *testing.T) {
	e, err := parseScanEnvelope(readFixture(t, "scan_envelope_file.json"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if e.Result == nil {
		t.Fatal("result is nil")
	}
	if len(e.Result.Files) != 3 {
		t.Fatalf("files = %d, want 3", len(e.Result.Files))
	}

	// zlib.h: two file matches, no snippet fields.
	var zlib *scanossapi.FileResult
	for i := range e.Result.Files {
		if e.Result.Files[i].Path == "zlib.h" {
			zlib = &e.Result.Files[i]
		}
	}
	if zlib == nil {
		t.Fatal("zlib.h not found in files")
	}
	if zlib.MatchType != "file" {
		t.Fatalf("zlib.h match_type = %q, want file", zlib.MatchType)
	}
	if len(zlib.Matches) != 2 {
		t.Fatalf("zlib.h matches = %d, want 2", len(zlib.Matches))
	}
	if zlib.Matches[0].MatchPercentage != 0 || zlib.Matches[0].InputLineRanges != nil {
		t.Fatalf("file match must not carry snippet fields: %+v", zlib.Matches[0])
	}
	// Scan API v0.4.4: source_hash equals file_hash for a file match; oss_file_path names
	// the matched file inside the OSS component.
	if zlib.SourceHash != zlib.FileHash {
		t.Fatalf("file-match source_hash = %q, want it to equal file_hash %q", zlib.SourceHash, zlib.FileHash)
	}
	if zlib.Matches[0].OssFilePath != "zlib/zlib.h" {
		t.Fatalf("zlib.h first match oss_file_path = %q, want zlib/zlib.h", zlib.Matches[0].OssFilePath)
	}

	// Every match's url_hash must resolve in the component catalog.
	for _, f := range e.Result.Files {
		for _, m := range f.Matches {
			if _, ok := e.Result.Components[m.UrlHash]; !ok {
				t.Fatalf("match url_hash %q has no component entry", m.UrlHash)
			}
		}
	}

	// Spot-check one fully-populated component.
	c, ok := e.Result.Components["00970cf622bd5b46f68eec9383753870"]
	if !ok {
		t.Fatal("expected ghostpdl-downloads component missing")
	}
	if c.Vendor != "ArtifexSoftware" || c.Version != "gs950" || c.Rank != 5 {
		t.Fatalf("component metadata parsed wrong: %+v", c)
	}
	if len(c.Purls) != 2 || c.Purls[0] != "pkg:github/artifexsoftware/ghostpdl-downloads" {
		t.Fatalf("component purls parsed wrong: %+v", c.Purls)
	}
	if c.Url == "" || c.ReleaseDate == "" || c.File == "" {
		t.Fatalf("optional component fields dropped: %+v", c)
	}

	// This fixture carries a server block; it must parse into the pointer types
	// (Server / scanossapi.KnowledgeBase) with every field populated. Guards the "server
	// present" path — the counterpart to the omitted-server case in _None.
	srv := e.Result.Server
	if srv == nil {
		t.Fatal("server block present in fixture but parsed as nil")
	}
	if srv.ApiVersion != "0.0.2" || srv.Hostname != "scanner-01" || srv.ElapsedMs != 1234 {
		t.Fatalf("server scalars parsed wrong: %+v", srv)
	}
	if srv.KnowledgeBase == nil {
		t.Fatal("knowledge_base present in fixture but parsed as nil")
	}
	if srv.KnowledgeBase.MonthlyVersion != "25.06" || srv.KnowledgeBase.DailyVersion != "25.06.30" {
		t.Fatalf("knowledge_base parsed wrong: %+v", srv.KnowledgeBase)
	}
}

// TestParseScanEnvelope_Snippet: a partial (snippet) match. This is the richest
// shape — match_percentage plus paired input/oss line ranges — and the one most
// likely to regress, so assert every field precisely.
func TestParseScanEnvelope_Snippet(t *testing.T) {
	e, err := parseScanEnvelope(readFixture(t, "scan_envelope_snippet.json"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if e.Result == nil || len(e.Result.Files) != 1 {
		t.Fatalf("unexpected result: %+v", e.Result)
	}
	f := e.Result.Files[0]
	if f.MatchType != "snippet" {
		t.Fatalf("match_type = %q, want snippet", f.MatchType)
	}
	if len(f.Matches) != 1 {
		t.Fatalf("matches = %d, want 1", len(f.Matches))
	}
	m := f.Matches[0]
	if m.MatchPercentage != 58 {
		t.Fatalf("match_percentage = %d, want 58", m.MatchPercentage)
	}
	// input and oss ranges come in parallel arrays of equal length.
	if len(m.InputLineRanges) != 6 || len(m.OssLineRanges) != 6 {
		t.Fatalf("line ranges = (%d,%d), want (6,6)", len(m.InputLineRanges), len(m.OssLineRanges))
	}
	if m.InputLineRanges[0] != (scanossapi.LineRange{StartLine: 82, EndLine: 209}) {
		t.Fatalf("first input range = %+v, want {82,209}", m.InputLineRanges[0])
	}
	if m.OssLineRanges[0] != (scanossapi.LineRange{StartLine: 77, EndLine: 204}) {
		t.Fatalf("first oss range = %+v, want {77,204}", m.OssLineRanges[0])
	}
	if _, ok := e.Result.Components[m.UrlHash]; !ok {
		t.Fatalf("snippet match url_hash %q has no component entry", m.UrlHash)
	}
	// Scan API v0.4.4: for a snippet match source_hash (the input file) differs from
	// file_hash (the matched OSS file); oss_file_path names the matched OSS file.
	if f.SourceHash == "" || f.SourceHash == f.FileHash {
		t.Fatalf("snippet source_hash = %q, want non-empty and != file_hash %q", f.SourceHash, f.FileHash)
	}
	if m.OssFilePath != "gcc/deflate.c" {
		t.Fatalf("oss_file_path = %q, want gcc/deflate.c", m.OssFilePath)
	}
}
