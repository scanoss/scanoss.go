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

package models

import "time"

// Quality represents the quality of the detected component
type Quality struct {
	Score  string `json:"score,omitempty"`
	Source string `json:"source,omitempty"`
}

// Licenses represents license information
type Licenses struct {
	Name         string    `json:"name,omitempty"`
	PatentHints  string    `json:"patent_hints,omitempty"`
	Copyleft     string    `json:"copyleft,omitempty"`
	ChecklistURL string    `json:"checklist_url,omitempty"`
	OsadlUpdated time.Time `json:"osadl_updated,omitempty"`
	Source       string    `json:"source,omitempty"`
	URL          string    `json:"url,omitempty"`
}

// Health represents repository health information
type Health struct {
	CreationDate string      `json:"creation_date,omitempty"`
	LastUpdate   string      `json:"last_update,omitempty"`
	LastPush     string      `json:"last_push,omitempty"`
	Stars        interface{} `json:"stars,omitempty"`
	Issues       interface{} `json:"issues,omitempty"`
}

// Dependencies represents component dependencies
type Dependencies struct {
	Vendor    string `json:"vendor,omitempty"`
	Component string `json:"component,omitempty"`
	Version   string `json:"version,omitempty"`
	Source    string `json:"source,omitempty"`
}

// KbVersion represents knowledge base versions
type KbVersion struct {
	Monthly string `json:"monthly,omitempty"`
	Daily   string `json:"daily,omitempty"`
}

// Server represents server information
type Server struct {
	Version   string    `json:"version,omitempty"`
	KbVersion KbVersion `json:"kb_version,omitempty"`
	Hostname  string    `json:"hostname,omitempty"`
	Flags     string    `json:"flags,omitempty"`
	Elapsed   string    `json:"elapsed,omitempty"`
}

// ResultEntry represents a scan result
type ResultEntry struct {
	ID              string         `json:"id,omitempty"`
	Lines           string         `json:"lines,omitempty"`
	OssLines        string         `json:"oss_lines,omitempty"`
	Matched         string         `json:"matched,omitempty"`
	FileHash        string         `json:"file_hash,omitempty"`
	SourceHash      string         `json:"source_hash,omitempty"`
	FileURL         string         `json:"file_url,omitempty"`
	Quality         []Quality      `json:"quality,omitempty"`
	Cryptography    []interface{}  `json:"cryptography,omitempty"`
	Purl            []string       `json:"purl,omitempty"`
	Vendor          string         `json:"vendor,omitempty"`
	Component       string         `json:"component,omitempty"`
	Version         string         `json:"version,omitempty"`
	Latest          string         `json:"latest,omitempty"`
	URL             string         `json:"url,omitempty"`
	Status          string         `json:"status,omitempty"`
	ReleaseDate     string         `json:"release_date,omitempty"`
	File            string         `json:"file,omitempty"`
	URLHash         string         `json:"url_hash,omitempty"`
	Licenses        []Licenses     `json:"licenses,omitempty"`
	Health          Health         `json:"health,omitempty"`
	Provenance      string         `json:"provenance,omitempty"`
	Dependencies    []Dependencies `json:"dependencies,omitempty"`
	Copyrights      []interface{}  `json:"copyrights,omitempty"`
	Vulnerabilities []interface{}  `json:"vulnerabilities,omitempty"`
	Server          Server         `json:"server,omitempty"`
}

// FileFingerprint represents the fingerprint of a file
type FileFingerprint struct {
	Path        string
	Hash        string
	Size        int
	Fingerprint string // WFP content
}

// ScanResult groups file analysis results
type ScanResult struct {
	FilePath string
	Response string
	Error    error
}
