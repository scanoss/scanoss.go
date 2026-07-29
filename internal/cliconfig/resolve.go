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

package cliconfig

import (
	"log/slog"
	"os"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// Source names where a resolved value came from. `config list` reports it, and it
// is the only observable signal about a secret, whose value is never displayed.
type Source string

const (
	SourceFlag    Source = "flag"
	SourceEnv     Source = "env"
	SourceFile    Source = "config file"
	SourceDefault Source = "default"
	SourceUnset   Source = "unset"
)

// Setting is one recognized setting as a command sees it: the key in its stored form,
// the effective value, and which source won.
type Setting struct {
	Key    string
	Value  string
	Source Source
}

// API holds the resolved endpoint settings every API command needs.
type API struct {
	URL string
	Key string
}

// Transport holds the resolved settings for how to reach the API: through which
// proxy, and trusting which extra certificate authority. Both are empty when
// nothing is configured, which means "use the environment, and the system
// certificate pool".
type Transport struct {
	Proxy      string
	CACertFile string
}

// resolver answers "what value will this command actually use?" for each setting,
// applying flag > environment > config file > flag default.
//
// viper resolves the value. The ladder below is walked a second time to name the
// source, which viper does not expose per key (viper.Debug prints every layer to
// stdout and is not usable programmatically). The duplication is deliberate and
// covered by TestResolverValueAndSourceAgree.
//
// It is unexported on purpose: callers outside the package get the three Resolve
// functions below, so the resolution machinery is not part of the surface they can
// depend on.
type resolver struct {
	viper *viper.Viper
	flags *pflag.FlagSet
}

// newResolver reads the settings file once and binds the flags it recognizes.
// A command that does not declare a given flag is fine: that rung is simply
// skipped, which is what leaves `wfp` and `sbom` unaffected.
func newResolver(flags *pflag.FlagSet) (*resolver, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	v, err := fileViper(path)
	if err != nil {
		return nil, err
	}
	// AutomaticEnv derives SCANOSS_API_URL from api_url, so the variable names are
	// not spelled out twice; EnvName mirrors the same rule for reporting.
	v.SetEnvPrefix("SCANOSS")
	v.AutomaticEnv()

	if flags == nil {
		flags = pflag.NewFlagSet("", pflag.ContinueOnError)
	}
	for key, spec := range registry {
		flag := flags.Lookup(spec.cli)
		if flag == nil {
			continue // this command does not offer the flag
		}
		if err := v.BindPFlag(key, flag); err != nil {
			return nil, err
		}
	}
	return &resolver{viper: v, flags: flags}, nil
}

// Key returns the effective value of one recognized key and its source.
func (r *resolver) Key(key string) Setting {
	setting := r.walk(key)

	// Diagnostics name the setting the way the user types it, like every other
	// message. A secret's value must not reach a log, so only its source is recorded.
	if IsSecret(key) {
		slog.Debug("resolved "+CLIKey(key), "source", setting.Source)
	} else {
		slog.Debug("resolved "+CLIKey(key), "source", setting.Source, "value", setting.Value)
	}
	return setting
}

// walk is the precedence ladder. It mirrors what viper's own lookup does, purely
// so the winning rung can be named.
func (r *resolver) walk(key string) Setting {
	flag := r.flags.Lookup(CLIKey(key))

	// An explicitly-typed flag wins: the value a flag holds cannot tell you whether
	// the user chose it, so Changed is the only reliable signal.
	if flag != nil && flag.Changed {
		return Setting{Key: key, Value: r.viper.GetString(key), Source: SourceFlag}
	}
	// An empty environment variable counts as unset, matching viper's own
	// AutomaticEnv behaviour when AllowEmptyEnv is off (the default).
	if os.Getenv(EnvName(key)) != "" {
		return Setting{Key: key, Value: r.viper.GetString(key), Source: SourceEnv}
	}
	// InConfig alone is not enough: viper returns a present-but-empty config value
	// ahead of the flag default, and an empty value means unset.
	if r.viper.InConfig(key) && r.viper.GetString(key) != "" {
		return Setting{Key: key, Value: r.viper.GetString(key), Source: SourceFile}
	}
	// The built-in default is the last rung, read from the registry rather than from
	// the flag: `config list` declares no API flags and must still be able to explain
	// where a value came from. An absent default is not a value — report unset.
	if def := defaultOf(key); def != "" {
		return Setting{Key: key, Value: def, Source: SourceDefault}
	}
	return Setting{Key: key, Source: SourceUnset}
}

// ResolveAPI returns the API settings a command should use, applying
// flag > environment > config file > built-in default. This is what the commands that
// talk to the API call.
func ResolveAPI(flags *pflag.FlagSet) (API, error) {
	r, err := newResolver(flags)
	if err != nil {
		return API{}, err
	}
	return API{URL: r.Key(KeyAPIURL).Value, Key: r.Key(KeyAPIKey).Value}, nil
}

// ResolveTransport returns the transport settings a command should use, applying the
// same ladder as ResolveAPI. Separate from ResolveAPI because the two answer different
// questions — where to send the request, and how to get there — but resolved the same
// way, and each call reads the file once.
func ResolveTransport(flags *pflag.FlagSet) (Transport, error) {
	r, err := newResolver(flags)
	if err != nil {
		return Transport{}, err
	}
	return Transport{
		Proxy:      r.Key(KeyProxy).Value,
		CACertFile: r.Key(KeyCACert).Value,
	}, nil
}

// Resolve returns the effective value of one recognized key and its source. The key
// is the stored form; map a command argument with StoredKey first.
func Resolve(flags *pflag.FlagSet, key string) (Setting, error) {
	r, err := newResolver(flags)
	if err != nil {
		return Setting{}, err
	}
	return r.Key(key), nil
}

// ResolveAll returns every recognized setting, sorted by key — what `config list`
// reports. The file is read once for the whole set.
func ResolveAll(flags *pflag.FlagSet) ([]Setting, error) {
	r, err := newResolver(flags)
	if err != nil {
		return nil, err
	}
	keys := recognizedKeys()
	settings := make([]Setting, 0, len(keys))
	for _, key := range keys {
		settings = append(settings, r.Key(key))
	}
	return settings, nil
}
