# Version Command Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add `logos version` command showing build version injected via Go ldflags.

**Architecture:** Single exported `Version` variable in `cmd/root.go` with default `"dev"`. New `cmd/version.go` Cobra command prints it. Build injects via `-ldflags "-X logos/cmd.Version=..."`.

**Tech Stack:** Go 1.24, Cobra, standard `testing` package

---

### Task 1: Add Version variable + version command + tests

**Files:**
- Modify: `iroll/cmd/root.go` — add `var Version = "dev"`
- Create: `iroll/cmd/version.go` — version command
- Create: `iroll/cmd/version_test.go` — tests

- [ ] **Step 1: Write the tests first (TDD)**

```go
package cmd

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
)

func TestVersionDefaultIsDev(t *testing.T) {
	if Version != "dev" {
		t.Errorf("Version = %q, want %q", Version, "dev")
	}
}

func TestVersionCommandRegistered(t *testing.T) {
	found := false
	for _, c := range rootCmd.Commands() {
		if c.Name() == "version" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("version command not registered under rootCmd")
	}
}

func TestVersionCommandOutput(t *testing.T) {
	old := Version
	defer func() { Version = old }()
	Version = "1.2.3"

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"version"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("version command failed: %v", err)
	}
	if got := buf.String(); got != "1.2.3\n" {
		t.Errorf("output = %q, want %q", got, "1.2.3\n")
	}
}
```

- [ ] **Step 2: Run tests, confirm they fail**

```bash
cd iroll && go test ./cmd/ -run TestVersion -v
```

Expected: `TestVersionCommandRegistered` FAIL (version command not found); `TestVersionDefaultIsDev` FAIL (Version undefined).

- [ ] **Step 3: Add Version variable to root.go**

In `iroll/cmd/root.go`, add after the import block and before `var rootCmd`:

```go
// Version is set at build time via ldflags.
// Default "dev" means unversioned development build.
var Version = "dev"
```

- [ ] **Step 4: Create version.go**

```go
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show logos version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintln(cmd.OutOrStdout(), Version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
```

- [ ] **Step 5: Run tests, confirm they pass**

```bash
cd iroll && go test ./cmd/ -run TestVersion -v
```

Expected: all 3 tests PASS.

- [ ] **Step 6: Run full test suite**

```bash
cd iroll && go test ./... && go vet ./...
```

- [ ] **Step 7: Rebuild and manual verify**

```bash
cd iroll && go build -o ../logos . && cd .. && ./logos version
```

Expected: `dev`

- [ ] **Step 8: Commit**

```bash
git add iroll/cmd/root.go iroll/cmd/version.go iroll/cmd/version_test.go
git commit -m "feat: add version command with ldflags injection"
```
