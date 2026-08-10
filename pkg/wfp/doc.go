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

// Package wfp turns files into WFP (Winnowing Fingerprint) fingerprints — the payload a
// SCANOSS scan uploads. All entry points fingerprint through a bounded worker pool; they
// differ in what they hand back. Folder and Files return the combined stream sorted by
// path together with the per-file detail behind it. Stream and StreamFolder write each
// file's block to an io.Writer as it completes and retain nothing, so memory stays
// bounded whatever the tree size — at the price of completion-order output.
//
// Folder and StreamFolder take a directory and apply the filtering rules themselves,
// which is what a caller holding a tree wants. Files and Stream take a list and apply
// none: they fingerprint whatever they are given, because the list may have come from
// somewhere other than a walk — an explicit selection, an archive stream, a file named
// on a command line.
//
// The asymmetry is deliberate but not obvious from the names, so a caller building the list
// from a directory itself should filter it first, with filter.Collect and one of its profiles.
package wfp
