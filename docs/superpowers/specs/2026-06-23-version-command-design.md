# Logos Version Command Design

> **Goal:** Add `logos version` command showing the build version, injected via Go ldflags.

**Date:** 2026-06-23

## Motivation

`logos version` currently fails with "unknown command". Users and tools need to identify which build they are running. The Logos CLI has no version infrastructure at all — no git tags, no version variable, no build scripts.

## Design

### Version variable

A single exported `Version` string in `cmd/root.go`, defaulting to `"dev"`:

```go
// Version is set at build time via ldflags.
// Default "dev" means unversioned development build.
var Version = "dev"
```

### version command

New file `cmd/version.go` — a minimal Cobra command that prints `Version`:

- Output: plain text version string (e.g. `1.0.0` or `dev`)
- NOT JSON-wrapped — `logos status` already covers machine-readable needs

### Build-time injection

```bash
# Development — shows "dev"
go build -o ../logos .

# Release — inject version
go build -ldflags "-X logos/cmd.Version=1.0.0" -o ../logos .

# Git-derived (recommended script)
VERSION=$(git describe --tags --always 2>/dev/null || echo "dev")
go build -ldflags "-X logos/cmd.Version=$VERSION" -o ../logos .
```

### Tests

`cmd/version_test.go`: verify default value is `"dev"`, command is registered, output matches `Version`.

## Files

| File | Action |
|------|--------|
| `iroll/cmd/root.go` | Add `var Version = "dev"` |
| `iroll/cmd/version.go` | New: version command |
| `iroll/cmd/version_test.go` | New: basic tests |

## Scope — not doing

- No `--json` flag (use `logos status` for machine output)
- No build metadata (commit hash, build date) — add later if needed
- No Makefile/CI changes — covered in build docs
