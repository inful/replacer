# Contributing to replacer

Thanks for your interest in contributing. This document covers the day-to-day
workflow: how to set up a development environment, how to run the gates, and
the conventions the project follows so changes land clean.

## Project at a glance

- **Language:** Go (see `go.mod` for the version)
- **CLI framework:** [kong](https://github.com/alecthomas/kong)
- **License:** GPL-3.0
- **Module path:** `github.com/inful/replacer`

## Development setup

```bash
git clone https://github.com/inful/replacer
cd replacer

# Run the test suite
go test ./...

# Run the test suite with race detection
go test -race -count=1 ./...

# Run the linter (config in .golangci.yml)
golangci-lint run ./...

# Build the binary
go build -o replacer ./cmd/replacer

# Exercise the whole release path locally, without touching GitHub
goreleaser release --snapshot --clean --skip=publish
```

The dev container (`.devcontainer/`) is set up for VS Code; if you use it, the
toolchain is pre-installed.

## Layout

```
cmd/replacer/   CLI entry point: kong commands (extract, anonymize, version)
replacer.go     Library: payload extraction, sample detection, pseudonym mapping
```

## Gates

Every commit goes through the same checks, locally via `lefthook` and in CI:

1. `golangci-lint run ./...` — no lint findings.
2. `go test -race -count=1 ./...` — no failing tests.

Both must pass before a commit is made. CI runs them on every push to `main`
and every pull request, plus a cross-compile matrix for the six platforms the
release builds.

## Commits and versions

Commit messages follow Conventional Commits:

```
feat(cli): add a --sample-regex flag
fix: keep the longest sample name when replacing
docs: explain the sample map
```

The version is the git tag — there is no version file to edit. The next
version follows the commit types since the last tag:

| commit type | bump |
| --- | --- |
| `fix:` | patch |
| `feat:` | minor |
| `feat!:` or a `BREAKING CHANGE:` footer | major |

```bash
git tag v0.2.0 && git push origin v0.2.0
```

The tag push runs GoReleaser, which builds the binaries, writes the checksums
and opens a **draft** release. Read the notes, then publish it.
