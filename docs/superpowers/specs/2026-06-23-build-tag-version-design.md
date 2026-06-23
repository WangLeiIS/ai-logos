# Build Tag Version System

> **Goal:** `-t` flag supports `name:version` format (default `latest`), with `~/.iroll/name/version/` storage.

**Date:** 2026-06-23

## Design

### Tag parsing

`builder.ParseTag(raw)` parses `name:version` → `(name, version, error)`.
Default version = `"latest"`.

### Storage

```
~/.iroll/<name>/<version>/     → data
~/.iroll/<name>/latest         → symlink to latest version
```

### API changes

- `Build(lf, name, version)` — versioned
- `store.IrollPath(name, version)` — versioned
- `store.DbPath(name, version)` — versioned
- `store.List()` — shows versions

### No backward compat

Old flat `~/.iroll/name/` format is not supported.

### Scope

- [x] `-t name:version` parsing
- [x] latest symlink
- [x] All callers updated
- [ ] No multi-tag
- [ ] No semver enforcement
- [ ] No auto-version
