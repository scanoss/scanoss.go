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
// SCANOSS scan uploads. Both entry points fingerprint through a bounded worker pool and
// return the combined stream together with the per-file detail behind it.
//
// Folder takes a directory and applies the filtering rules itself, which is what a caller
// holding a tree wants. Files takes a list and applies none: it fingerprints whatever it is
// given, because the list may have come from somewhere other than a walk — an explicit
// selection, an archive stream, a file named on a command line.
//
// The asymmetry is deliberate but not obvious from the names, so a caller building the list
// from a directory itself should filter it first, with filter.Collect and one of its profiles.
package wfp
