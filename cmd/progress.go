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

package cmd

import (
	"fmt"
	"os"

	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

// progressWidth is the bar width shared by every command's progress output.
const progressWidth = 40

// newProgress creates an mpb progress container writing to stderr. It is the single progress
// library used across all commands (scan, wfp, dependencies, purl queries).
func newProgress() *mpb.Progress {
	return mpb.New(mpb.WithOutput(os.Stderr), mpb.WithWidth(progressWidth))
}

// addBar adds a styled bar to p — a left-aligned label, a percentage, and a block-filled bar. This
// is the one bar look shared by every command, so scan phases, WFP fingerprinting, purl queries,
// and dependency parsing all render identically.
func addBar(p *mpb.Progress, total int, label string) *mpb.Bar {
	return p.New(int64(total),
		mpb.BarStyle().Lbound(" |").Filler("█").Tip("█").Padding(" ").Rbound("|"),
		mpb.PrependDecorators(
			decor.Name(fmt.Sprintf("  %-24s ", label)),
			decor.NewPercentage("%d"), // percentageType already appends "%"
		),
	)
}
