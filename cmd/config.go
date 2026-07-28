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
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/scanoss/scanoss.go/internal/cliconfig"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage stored CLI settings",
	Long: `The config command stores CLI settings in ~/.scanoss/settings.json so they do
not have to be passed on every invocation. Recognized keys are api_url and api_key.

Each setting is resolved as: flag > environment variable > config file > built-in
default. The environment variables are SCANOSS_API_URL and SCANOSS_API_KEY.

The API key is never displayed: list and get render it as ******** and there is no
flag that reveals it. To read the value, open the file itself
(cat "$(scanoss-cli config path)").

This is your own configuration, and is unrelated to a project's scanoss.json (the
--settings file), which holds that project's BOM and skip rules.

Examples:
  # Store an API key, then scan without passing it
  scanoss-cli config set api_key YOUR_API_KEY
  scanoss-cli scan ./my-project

  # Target an on-prem endpoint (a custom URL may run keyless)
  scanoss-cli config set api_url https://scanoss.internal.example.com

  # Show the effective settings and where each one came from
  scanoss-cli config list

  # Remove a stored key, or locate the file
  scanoss-cli config unset api_key
  scanoss-cli config path`,
	Args: cobra.NoArgs,
	// No subcommand: show usage rather than a terse error, matching scan/enrich.
	RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Store a setting in ~/.scanoss/settings.json",
	Args:  cobra.ExactArgs(2),
	RunE:  runConfigSet,
}

var configUnsetCmd = &cobra.Command{
	Use:   "unset <key>",
	Short: "Remove a setting from ~/.scanoss/settings.json",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigUnset,
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Print one setting (secrets render as ********)",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigGet,
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show the effective settings and the source of each",
	Args:  cobra.NoArgs,
	RunE:  runConfigList,
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print the config file path",
	Args:  cobra.NoArgs,
	RunE:  runConfigPath,
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configSetCmd, configUnsetCmd, configGetCmd, configListCmd, configPathCmd)
}

// maskedValue is what a secret renders as. It is a constant so that neither the
// value nor its length is disclosed.
const maskedValue = "********"

// display renders a value for output, masking it when the key is secret. Every
// output path goes through here, so there is one place where a secret can be
// printed — and it never prints one.
func display(key, value string) string {
	if cliconfig.IsSecret(key) {
		return maskedValue
	}
	return value
}

// sourceLabel renders where a value came from. The environment label names the
// variable, since "which one do I unset?" is the immediate follow-up question.
func sourceLabel(key string, source cliconfig.Source) string {
	switch source {
	case cliconfig.SourceEnv:
		return "(env: " + cliconfig.EnvName(key) + ")"
	case cliconfig.SourceUnset:
		return ""
	default:
		return "(" + string(source) + ")"
	}
}

func runConfigSet(_ *cobra.Command, args []string) error {
	key, value := args[0], args[1]

	// Store api_url the way the SDK will use it: trimmed, no trailing slash. Same
	// normalization the endpoint comparison in the no-key guard applies.
	if key == cliconfig.KeyAPIURL {
		value = normalizeURL(value)
	}
	// An empty value would be stored and then read back as unset — a silent no-op.
	// Point at the command that actually expresses the intent. Recognized keys only,
	// so a typo still reports the unknown key rather than complaining about the value.
	if cliconfig.IsRecognized(key) && strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s cannot be set to an empty value; use %q to remove it",
			key, "scanoss-cli config unset "+key)
	}

	if err := cliconfig.Set(key, value); err != nil {
		return err
	}
	path, err := cliconfig.Path()
	if err != nil {
		return err
	}
	okf("Saved %s to %s", key, path)
	return nil
}

func runConfigUnset(_ *cobra.Command, args []string) error {
	key := args[0]

	if err := cliconfig.Unset(key); err != nil {
		return err
	}
	path, err := cliconfig.Path()
	if err != nil {
		return err
	}
	okf("Removed %s from %s", key, path)
	return nil
}

// runConfigGet prints one setting's effective value, undecorated so it composes in a
// shell. A secret prints as ******** — so for those, get answers only whether the
// key is set (by its exit code), and a script that needs the value uses the
// environment variable instead.
func runConfigGet(cmd *cobra.Command, args []string) error {
	key := args[0]
	if !cliconfig.IsRecognized(key) {
		return &cliconfig.UnknownKeyError{Key: key}
	}

	resolver, err := cliconfig.NewResolver(cmd.Flags())
	if err != nil {
		return err
	}
	resolved := resolver.Key(key)
	if resolved.Value == "" {
		return fmt.Errorf("%s is not set", key)
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), display(key, resolved.Value))
	return nil
}

// runConfigList reports the value each command will actually use and where it came
// from. The source is not decoration: a secret always renders as ********, so the
// source is the only observable signal about it — without it, "your stored key" and
// "a different key from the environment" would print identically.
func runConfigList(cmd *cobra.Command, _ []string) error {
	resolver, err := cliconfig.NewResolver(cmd.Flags())
	if err != nil {
		return err
	}
	stored, err := cliconfig.Load()
	if err != nil {
		return err
	}
	path, err := cliconfig.Path()
	if err != nil {
		return err
	}

	out := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)

	// Recognized keys are always listed, so `list` is never blank on a fresh machine
	// and doubles as a reference for what can be set.
	for _, key := range cliconfig.RecognizedKeys() {
		resolved := resolver.Key(key)
		value := display(key, resolved.Value)
		if resolved.Source == cliconfig.SourceUnset {
			value = "(unset)"
		}
		// An unset key has no source to report; omitting the cell rather than
		// emitting an empty one keeps tabwriter from padding the line with trailing
		// spaces.
		if label := sourceLabel(key, resolved.Source); label != "" {
			_, _ = fmt.Fprintf(out, "%s\t%s\t%s\n", key, value, label)
		} else {
			_, _ = fmt.Fprintf(out, "%s\t%s\n", key, value)
		}
	}

	// Keys the file carries that this version does not recognize: shown so a
	// hand-edited file is visible rather than silently ignored.
	for _, key := range stored.Keys() {
		if cliconfig.IsRecognized(key) {
			continue
		}
		value, _ := stored.Get(key)
		_, _ = fmt.Fprintf(out, "%s\t%s\t%s\n", key, value, "(config file, unrecognized)")
	}
	if err := out.Flush(); err != nil {
		return err
	}

	// The file itself stays one command away, since `list` reports effective values
	// rather than the file's contents.
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nConfig file: %s\n", path)
	return nil
}

func runConfigPath(cmd *cobra.Command, _ []string) error {
	path, err := cliconfig.Path()
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), path)
	return nil
}
