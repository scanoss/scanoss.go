# README: Configuration section (draft for T007)

Apply this to `README.md` **in the same commit as T006**, never before — `main` must not
document a command that does not exist yet. Insert after the `## Commands` table and its
`CLIENT_HELP.md` pointer, and in the same edit:

- add a `config` row to the command table:
  `| ``config`` | Store the API URL and key in ``~/.scanoss/settings.json`` (see [Configuration](#configuration)). |`
- reword the sentence under the table from "requires `--api-key`" / "a custom `--api-url`"
  to "requires an API key" / "a custom API URL", since either can now come from the config
  file or the environment rather than a flag.

---

## Configuration

Store your credentials once instead of passing `--api-key` on every command:

```bash
scanoss-cli config set api_key SC_abc123def456
scanoss-cli scan ./my-project --output results.json
```

Settings live in `~/.scanoss/settings.json` (created on first `config set`, directory
`0700`, file `0600`), with `snake_case` keys:

```json
{
  "api_key": "SC_abc123def456",
  "api_url": "https://api.scanoss.com"
}
```

> **Not the same file as `scanoss.json`.** `~/.scanoss/settings.json` is *your*
> configuration — credentials and endpoint. A project's `scanoss.json` (or the file you
> pass to `--settings`) holds that project's BOM rules and skip rules. Different scope,
> different file.

### Precedence

Every command that accepts `--api-url`/`--api-key` resolves each value the same way:

```
--flag  >  environment variable  >  ~/.scanoss/settings.json  >  built-in default
```

Environment variables are `SCANOSS_API_KEY` and `SCANOSS_API_URL`. An empty value from
the environment or the file is treated as unset and falls through to the next source.

```bash
scanoss-cli config set api_url https://scanoss.internal.acme.com

# 1. the stored value is used
scanoss-cli scan .

# 2. the environment overrides the file
SCANOSS_API_URL=https://staging.scanoss.com scanoss-cli scan .

# 3. the flag overrides both
SCANOSS_API_URL=https://staging.scanoss.com \
  scanoss-cli scan . --api-url https://api.scanoss.com
```

### Inspecting

`config list` shows the value each command will actually use, and where it came from:

```console
$ scanoss-cli config list
api_key  ********                           (env: SCANOSS_API_KEY)
api_url  https://scanoss.internal.acme.com  (config file)

Config file: /Users/you/.scanoss/settings.json
```

**The API key is never printed.** `list` and `get` always render it as `********` — there
is no flag that reveals it, so it cannot land in your shell history or a CI log. `config
get api_key` therefore only tells you whether it is set (exit code `0` or `1`). Scripts
that need the value should use `$SCANOSS_API_KEY`; to read your own file, open it
directly:

```bash
cat "$(scanoss-cli config path)"
```

Non-secret values print normally, so `config get` composes:

```console
$ scanoss-cli config get api_url
https://scanoss.internal.acme.com
```

### On-prem endpoint

A custom API URL may run keyless, so pointing the CLI at an internal deployment is one
command:

```bash
scanoss-cli config set api_url https://scanoss.internal.acme.com
scanoss-cli scan .
```

### CI

Use the environment instead of a config file — no `config set`, and no key on the
command line where it would land in build logs:

```yaml
- name: SCANOSS scan
  env:
    SCANOSS_API_KEY: ${{ secrets.SCANOSS_API_KEY }}
  run: scanoss-cli scan . --output results.json
```

### Rotating and removing

```bash
scanoss-cli config set api_key SC_newkey789   # overwrite in place
scanoss-cli config unset api_key              # remove the key
scanoss-cli config path                       # print the file location
```

Hand-editing the file is supported, and keys this version does not recognize are left
untouched by `config set`.
