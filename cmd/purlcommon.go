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
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"github.com/vbauerster/mpb/v8"

	"github.com/scanoss/scanoss.go/internal/cliconfig"
	"github.com/scanoss/scanoss.go/pkg/scanoss"
)

// addPurlInputFlags registers the PURL-list input flags (no API flags), as
// persistent flags so a parent command's subcommands inherit them.
func addPurlInputFlags(cmd *cobra.Command) {
	fs := cmd.PersistentFlags()
	fs.StringArray("purl", nil, "Package URL (purl) of a component; repeatable")
	fs.String("requirement", "", "Default version requirement applied to purls that don't specify one")
	fs.String("input", "", "File with purls: newline-delimited (purl[,requirement]) or a JSON {\"components\":[...]}")
	fs.Int("chunk-size", scanoss.DefaultChunkSize, "Number of purls per request")
	fs.IntP("workers", "t", scanoss.DefaultWorkers, "Maximum concurrent requests")
}

// addPurlServiceFlags registers the flags shared by the PURL-list service commands
// (cryptography, vulnerabilities, licenses, geoprovenance, copyright): the PURL-list
// input flags plus the API/output flags.
func addPurlServiceFlags(cmd *cobra.Command) {
	addPurlInputFlags(cmd)
	addAPIFlags(cmd)
}

// typedDecorateFunc is a typed per-service batch method (e.g. Vulnerabilities.Components),
// returning the OpenAPI response model instead of a raw *Result.
type typedDecorateFunc[T any] func(*scanoss.Client, context.Context, []scanoss.Component, ...scanoss.DecorateOption) (*T, error)

// newPurlServiceCmdTyped builds a PURL-list subcommand backed by a typed SDK method.
func newPurlServiceCmdTyped[T any](use, short, long string, call typedDecorateFunc[T]) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Long:  long,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return runPurlServiceTyped(cmd, call) },
	}
}

// resolveComponents builds the list of components to query from the --purl
// flags and/or the --input file, applying --requirement as the default for
// entries that don't carry one.
func resolveComponents(cmd *cobra.Command) ([]scanoss.Component, error) {
	purls, _ := cmd.Flags().GetStringArray("purl")
	requirement, _ := cmd.Flags().GetString("requirement")
	inputFile, _ := cmd.Flags().GetString("input")

	var components []scanoss.Component

	for _, p := range purls {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		components = append(components, scanoss.Component{Purl: p, Requirement: requirement})
	}

	if inputFile != "" {
		fromFile, err := readComponentsFile(inputFile, requirement)
		if err != nil {
			return nil, err
		}
		components = append(components, fromFile...)
	}

	if len(components) == 0 {
		return nil, fmt.Errorf("no purls provided: use --purl (repeatable) and/or --input")
	}

	return components, nil
}

// readComponentsFile parses an input file as either a JSON {"components":[...]}
// document or a newline-delimited list of "purl[,requirement]" entries.
func readComponentsFile(path, defaultRequirement string) ([]scanoss.Component, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading input file: %w", err)
	}

	if trimmed := strings.TrimSpace(string(data)); strings.HasPrefix(trimmed, "{") {
		var req struct {
			Components []scanoss.Component `json:"components"`
		}
		if err := json.Unmarshal([]byte(trimmed), &req); err != nil {
			return nil, fmt.Errorf("error parsing JSON input file: %w", err)
		}
		for i := range req.Components {
			if req.Components[i].Requirement == "" {
				req.Components[i].Requirement = defaultRequirement
			}
		}
		return req.Components, nil
	}

	var components []scanoss.Component
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		purl := line
		req := defaultRequirement
		if parts := strings.SplitN(line, ",", 2); len(parts) == 2 {
			purl = strings.TrimSpace(parts[0])
			req = strings.TrimSpace(parts[1])
		}
		components = append(components, scanoss.Component{Purl: purl, Requirement: req})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading input file: %w", err)
	}
	return components, nil
}

// clientOptions builds the SDK client options. The API URL and key are resolved
// (flag > environment > config file > default); the chunk-size and workers flags are
// optional (absent on the non-PURL-list commands) and when unset resolve to 0, so
// the SDK keeps its defaults.
func clientOptions(cmd *cobra.Command) ([]scanoss.Option, error) {
	api, err := cliconfig.ResolveAPI(cmd.Flags())
	if err != nil {
		return nil, err
	}
	chunkSize, _ := cmd.Flags().GetInt("chunk-size")
	workers, _ := cmd.Flags().GetInt("workers")
	httpClient, err := newHTTPClient(cmd)
	if err != nil {
		return nil, err
	}

	return []scanoss.Option{
		scanoss.WithAPIURL(api.URL),
		scanoss.WithAPIKey(api.Key),
		scanoss.WithChunkSize(chunkSize),
		scanoss.WithWorkers(workers),
		scanoss.WithHTTPClient(httpClient),
	}, nil
}

// newClient builds an SDK client from the API flags, without a progress bar. Used
// by the single-shot commands (components search/versions).
func newClient(cmd *cobra.Command) (*scanoss.Client, error) {
	opts, err := clientOptions(cmd)
	if err != nil {
		return nil, err
	}
	return scanoss.New(opts...), nil
}

// purlBar renders decoration progress onto a single bar. The purl commands query one service at a
// time, so which service reported is not shown — unlike a scan, where several run at once.
type purlBar struct{ draw func(done, total int) }

func (b purlBar) Decorating(service string, done, total int) { b.draw(done, total) }

// newProgressClient builds an SDK client, the option that reports this command's progress onto a
// stderr bar, and a finish func to flush that bar once the call returns. The option is returned
// rather than baked into the client because a reporter belongs to the call.
func newProgressClient(cmd *cobra.Command) (*scanoss.Client, scanoss.DecorateOption, func(), error) {
	var (
		mu    sync.Mutex
		prog  *mpb.Progress
		bar   *mpb.Bar
		total int
	)
	base, err := clientOptions(cmd)
	if err != nil {
		return nil, nil, nil, err
	}
	// These commands only decorate — they query PURLs and never scan — so they implement the one
	// contract that concerns them, and hand it to the call rather than to the client.
	reporter := scanoss.WithDecorationReporter(purlBar{
		draw: func(done, t int) {
			mu.Lock()
			defer mu.Unlock()
			if bar == nil {
				prog = newProgress()
				bar = addBar(prog, t, "Querying purls")
				total = t
			}
			bar.SetCurrent(int64(done))
		},
	})
	return scanoss.New(base...), reporter, func() {
		mu.Lock()
		defer mu.Unlock()
		if bar != nil {
			bar.SetCurrent(int64(total)) // force 100% in case the final tick was below total
			prog.Wait()
		}
	}, nil
}

// hasPurlInput reports whether the user supplied any component input (--purl or
// --input). Used so a bare invocation shows help instead of the auth banner.
func hasPurlInput(cmd *cobra.Command) bool {
	purls, _ := cmd.Flags().GetStringArray("purl")
	for _, p := range purls {
		if strings.TrimSpace(p) != "" {
			return true
		}
	}
	input, _ := cmd.Flags().GetString("input")
	return strings.TrimSpace(input) != ""
}

// runPurlServiceTyped is the entry point for the PURL-list service commands: it
// resolves the component list, calls the typed SDK method, and marshals the
// returned OpenAPI model to indented JSON output.
func runPurlServiceTyped[T any](cmd *cobra.Command, call typedDecorateFunc[T]) error {
	if !hasPurlInput(cmd) {
		return cmd.Help() // no --purl/--input: show usage, not the auth banner
	}
	if err := checkAuth(cmd); err != nil {
		return err
	}
	components, err := resolveComponents(cmd)
	if err != nil {
		return err
	}
	client, reportProgress, finish, err := newProgressClient(cmd)
	if err != nil {
		return err
	}
	v, err := call(client, cmd.Context(), components, reportProgress)
	if err != nil {
		return renderAPIError(err)
	}
	finish()
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	outputFile, _ := cmd.Flags().GetString("output")
	return writeOutput(string(out), outputFile)
}

// writeOutput writes content to stdout or a file. Shared by every
// component-service command.
func writeOutput(content, outputFile string) error {
	if outputFile == "" {
		fmt.Println(content)
		return nil
	}
	return os.WriteFile(outputFile, []byte(content), 0644)
}
