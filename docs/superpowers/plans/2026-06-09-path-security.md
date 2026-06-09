# Path Security Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reject filesystem inputs that can escape an iroll package, Layerfile directory, or extraction destination.

**Architecture:** Introduce a focused `safepath` package and call it from existing filesystem boundaries. Keep command behavior unchanged for valid inputs and return explicit errors for unsafe inputs.

**Tech Stack:** Go 1.24, standard library, Go testing

---

### Task 1: Safe Path Primitives

**Files:**
- Create: `iroll/safepath/path.go`
- Create: `iroll/safepath/path_test.go`

- [ ] Write tests for valid and invalid iroll names.
- [ ] Run tests and verify they fail because `ValidateName` does not exist.
- [ ] Implement `ValidateName`.
- [ ] Write tests for valid nested paths, traversal, and absolute paths.
- [ ] Run tests and verify they fail because `Join` does not exist.
- [ ] Implement `Join` and rerun tests.

### Task 2: Store Boundaries

**Files:**
- Modify: `iroll/store/store.go`
- Create: `iroll/store/store_test.go`

- [ ] Write a ZIP Slip extraction regression test and verify it fails.
- [ ] Validate names in `IrollPath` callers and use safe joining during extraction.
- [ ] Verify malicious ZIP entries are rejected and valid archives still extract.

### Task 3: Builder and Context Boundaries

**Files:**
- Modify: `iroll/builder/build.go`
- Create: `iroll/builder/build_test.go`
- Modify: `iroll/db/db.go`
- Create: `iroll/db/db_test.go`

- [ ] Write failing tests for Layerfile source/destination traversal.
- [ ] Apply safe joining to `FROM`, `MIGRATE`, and `COPY`.
- [ ] Write a failing test for traversal through a context `@file`.
- [ ] Apply safe joining to context file resolution.

### Task 4: Verification

**Files:**
- Modify only files required by failing verification.

- [ ] Run `gofmt` on changed Go files.
- [ ] Run `go test ./...`.
- [ ] Run `go vet ./...`.
- [ ] Run `go build ./...`.
- [ ] Review the final diff for unrelated changes.

