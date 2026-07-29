# Contributing

Thank you for considering contributing to SCANOSS. It's people like you that make SCANOSS such a
great tool. Feel welcome and read the following sections in order to know how to get involved, ask
questions, and — more importantly — how to work on something.

SCANOSS is an open source project and we love to receive contributions from our community. There
are many ways to contribute: writing tutorials or blog posts, improving the documentation,
submitting bug reports and feature requests, or writing code.

## Submitting bugs

If you are submitting a bug, please tell us:

- the version of the CLI you are using (`scanoss-cli --version`);
- the version of Go you are using (`go version`), if you built it yourself;
- your operating system;
- how to reproduce the bug — ideally the exact command and, where possible, a minimal project
  that triggers it.

## Getting started

```bash
git clone https://github.com/scanoss/scanoss.go.git
cd scanoss.go
make build          # builds ./scanoss-cli
make check          # fmt-check + vet + lint + test
```

`make check` is what CI runs. Please run it before opening a pull request.

Other useful targets:

| Target | What it does |
|---|---|
| `make test` | unit tests |
| `make test-race` | unit tests with the race detector |
| `make lint` | golangci-lint |
| `make build` | build the CLI binary |

## Pull requests

Want to submit a pull request? Great. A few things that make review easier:

- **Describe what the change does** and link any relevant issue. What problem does it solve?
- **Keep commits atomic** — one logical change per commit, with the tree building and the tests
  passing at each one.
- **Use [Conventional Commits](https://www.conventionalcommits.org/)** for commit subjects:
  `feat:`, `fix:`, `refactor:`, `docs:`, `chore:`, `test:`. Keep subjects short and imperative.
- **Add tests** for new functionality, and for bug fixes add the test that fails without the fix.
- **Update `CHANGELOG.md`** under `## [Unreleased]` when the change is user-visible. Documentation
  and pure refactors with no behaviour change do not need an entry.
- **Only include the lines you changed** — watch out for your editor reformatting whole files.

## Project layout

| Path | What lives there |
|---|---|
| `cmd/` | the CLI (Cobra); `cmd/scanoss-cli` is the `go install` entrypoint |
| `pkg/` | the reusable Go SDK: scanning, fingerprinting, filtering, SBOM, API client |
| `internal/` | private helpers (config, version) |
| `libscanoss/` | C shared library with Python and Node.js wrappers |

See [CLIENT_HELP.md](CLIENT_HELP.md) for full CLI usage and the `scanoss.json` reference.

## Licensing

This project is released under the MIT license. If you wish to contribute, you must accept that you
are aware of the license under which the project is released, and that your contribution will be
released under the same license.

Unless you expressly request otherwise, we may use your name, email address, username or URL for
your attribution notice text. The submission of your contribution implies that you agree with these
licensing terms.
