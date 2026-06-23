# Build Tag Version System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** `-t` flag supports `name:version` format (default `latest`), storage path becomes `~/.iroll/name/version/`.

**Architecture:** New `builder.ParseTag()` parses `name:version` strings. `Build()` accepts separate name+version params. `store.IrollPath(name, version)` builds versioned paths. All callers updated. No backward compat.

**Tech Stack:** Go 1.24, Cobra

---

### Task 1: ParseTag + Store layer

**Files:**
- Create: `iroll/builder/tag.go`
- Create: `iroll/builder/tag_test.go`
- Modify: `iroll/store/store.go`

- [ ] **Step 1: Write ParseTag and its tests**

```go
// iroll/builder/tag.go
package builder

import (
	"fmt"
	"strings"

	"logos/safepath"
)

// ParseTag parses a build tag like "my-agent:v0.1.0" into name and version.
// Default version is "latest".
func ParseTag(raw string) (name, version string, err error) {
	if raw == "" {
		return "", "", fmt.Errorf("tag cannot be empty")
	}

	parts := strings.SplitN(raw, ":", 2)
	name = strings.TrimSpace(parts[0])
	if name == "" {
		return "", "", fmt.Errorf("name cannot be empty")
	}
	if err := safepath.ValidateName(name); err != nil {
		return "", "", fmt.Errorf("invalid name: %w", err)
	}

	if len(parts) == 2 {
		version = strings.TrimSpace(parts[1])
		if version == "" {
			return "", "", fmt.Errorf("version cannot be empty")
		}
	} else {
		version = "latest"
	}
	return name, version, nil
}
```

```go
// iroll/builder/tag_test.go
package builder

import "testing"

func TestParseTagDefaultLatest(t *testing.T) {
	name, version, err := ParseTag("my-agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "my-agent" || version != "latest" {
		t.Errorf("got %q:%q, want my-agent:latest", name, version)
	}
}

func TestParseTagWithVersion(t *testing.T) {
	name, version, err := ParseTag("my-agent:v0.1.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "my-agent" || version != "v0.1.0" {
		t.Errorf("got %q:%q, want my-agent:v0.1.0", name, version)
	}
}

func TestParseTagEmpty(t *testing.T) {
	_, _, err := ParseTag("")
	if err == nil {
		t.Fatal("expected error for empty tag")
	}
}

func TestParseTagEmptyName(t *testing.T) {
	_, _, err := ParseTag(":v1")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestParseTagEmptyVersion(t *testing.T) {
	_, _, err := ParseTag("name:")
	if err == nil {
		t.Fatal("expected error for empty version")
	}
}

func TestParseTagInvalidName(t *testing.T) {
	_, _, err := ParseTag("../escape:v1")
	if err == nil {
		t.Fatal("expected error for invalid name")
	}
}
```

- [ ] **Step 2: Run tag tests, verify pass**

```bash
cd iroll && go test ./builder/ -run TestParseTag -v
```

- [ ] **Step 3: Update store.go — versioned IrollPath, DbPath, List**

In `iroll/store/store.go`, change `IrollPath` and `DbPath` to accept version:

```go
func IrollPath(name string, version string) (string, error) {
	if err := safepath.ValidateName(name); err != nil {
		return "", err
	}
	return safepath.Join(HomeDir(), name, version)
}

func DbPath(name string, version string) (string, error) {
	root, err := IrollPath(name, version)
	if err != nil {
		return "", err
	}
	return safepath.Join(root, "ai_roll.db")
}
```

Update `List()` to traverse version subdirectories:

```go
func List() ([]string, error) {
	home := HomeDir()
	entries, err := os.ReadDir(home)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Scan version subdirectories
		rootDir := filepath.Join(home, e.Name())
		versions, err := os.ReadDir(rootDir)
		if err != nil {
			continue
		}
		for _, v := range versions {
			if !v.IsDir() || v.Name() == "latest" {
				continue
			}
			dbFile := filepath.Join(rootDir, v.Name(), "ai_roll.db")
			if _, err := os.Stat(dbFile); err == nil {
				names = append(names, e.Name()+":"+v.Name())
			}
		}
	}
	return names, nil
}
```

- [ ] **Step 4: Run store tests**

```bash
cd iroll && go build ./store/...
```

- [ ] **Step 5: Commit**

```bash
git add iroll/builder/tag.go iroll/builder/tag_test.go iroll/store/store.go
git commit -m "feat: add ParseTag and versioned store paths"
```

---

### Task 2: Builder + cmd/build integration

**Files:**
- Modify: `iroll/builder/build.go`
- Modify: `iroll/cmd/build.go`

- [ ] **Step 1: Update Build signature**

In `iroll/builder/build.go`, change:

```go
func Build(lf *Irollfile, name string, version string) (*BuildResult, error) {
	if err := safepath.ValidateName(name); err != nil {
		return nil, err
	}

	// ... build process unchanged ...

	// Copy to ~/.iroll/<name>/<version>/
	home, _ := os.UserHomeDir()
	storeRoot := filepath.Join(home, ".iroll")
	dest, err := safepath.Join(storeRoot, name, version)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return nil, fmt.Errorf("ensure name dir: %w", err)
	}
	if err := os.Mkdir(dest, 0755); err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("iroll '%s:%s' already exists", name, version)
		}
		return nil, fmt.Errorf("create store directory: %w", err)
	}
	if err := copyDir(tmpDir, dest); err != nil {
		return nil, fmt.Errorf("copy to store: %w", err)
	}

	// Update latest symlink
	latestLink := filepath.Join(storeRoot, name, "latest")
	os.Remove(latestLink) // remove old symlink if exists
	if err := os.Symlink(version, latestLink); err != nil {
		// On Windows without admin, symlink may fail; fall back to a file marker
		os.WriteFile(latestLink+".txt", []byte(version), 0644)
	}

	// Update description
	lj := LayerJSON{
		LayerID:       layerID,
		Parent:        parentLayerID,
		Description:   fmt.Sprintf("build from Irollfile for %s:%s", name, version),
		CreatedAt:     now,
		SchemaVersion: 1,
	}

	return &BuildResult{
		Name:    name,
		Version: version,
		Path:    dest,
		LayerID: layerID,
	}, nil
}
```

Update `processFrom` to use versioned paths:

```go
func processFrom(tmpDir string, baseTag string) (string, error) {
	baseName, baseVersion, err := ParseTag(baseTag)
	if err != nil {
		return "", fmt.Errorf("invalid FROM tag: %w", err)
	}
	home, _ := os.UserHomeDir()
	src, err := safepath.Join(filepath.Join(home, ".iroll"), baseName, baseVersion)
	if err != nil {
		return "", err
	}
	// ... rest unchanged ...
}
```

Update `BuildResult`:

```go
type BuildResult struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Path    string `json:"path"`
	LayerID string `json:"layer_id"`
}
```

- [ ] **Step 2: Update cmd/build.go**

```go
var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build an iroll from an Irollfile",
	Run: func(cmd *cobra.Command, args []string) {
		lf, err := builder.ParseIrollfile(buildFile)
		if err != nil {
			outputError(err.Error())
		}

		name, version, err := builder.ParseTag(buildTag)
		if err != nil {
			outputError(fmt.Sprintf("invalid tag: %v", err))
		}

		result, err := builder.Build(lf, name, version)
		if err != nil {
			outputError(err.Error())
		}

		outputJSON(result)
	},
}

func init() {
	buildCmd.Flags().StringVarP(&buildFile, "file", "f", "Irollfile", "Irollfile path")
	buildCmd.Flags().StringVarP(&buildTag, "tag", "t", "", "Output name[:version] (default version: latest)")
	buildCmd.MarkFlagRequired("tag")
	rollCmd.AddCommand(buildCmd)
}
```

- [ ] **Step 3: Run tests**

```bash
cd iroll && go test ./builder/... ./cmd/ -v
```

- [ ] **Step 4: Commit**

```bash
git add iroll/builder/build.go iroll/cmd/build.go
git commit -m "feat: versioned Build with name:version tag support"
```

---

### Task 3: Update all callers

**Files:**
- Modify: `iroll/cmd/hub_push.go`
- Modify: `iroll/cmd/hub_pull.go`
- Modify: `iroll/cmd/load.go`
- Modify: `iroll/cmd/rm.go`
- Modify: `iroll/cmd/save.go`
- Modify: `iroll/cmd/inspect.go`
- Modify: `iroll/cmd/history.go`
- Modify: `iroll/cmd/query_dna.go`
- Modify: `iroll/cmd/context.go`
- Modify: `iroll/cmd/page.go`
- Modify: `iroll/cmd/loop.go`
- Modify: `iroll/cmd/loop_seed.go`
- Modify: `iroll/cmd/loop_run.go`
- Modify: `iroll/cmd/memory.go`
- Modify: `iroll/cmd/skill.go`
- Modify: `iroll/cmd/book.go`
- Modify: `iroll/e2e/testenv/setup.go`

All callers of `store.IrollPath(name)` → `store.IrollPath(name, "latest")`.
All callers of `store.DbPath(name)` → `store.DbPath(name, "latest")`.
All callers of `builder.Build(lf, tag)` → `builder.Build(lf, name, version)`.

- [ ] **Step 1: Update all callers via batch replace**

```bash
# Replace single-arg IrollPath → two-arg
sed -i 's/store\.IrollPath(\([^,)]*\))/store.IrollPath(\1, "latest")/g' iroll/cmd/*.go
sed -i 's/store\.DbPath(\([^,)]*\))/store.DbPath(\1, "latest")/g' iroll/cmd/*.go
# Fix testenv
sed -i 's/store\.IrollPath(\([^,)]*\))/store.IrollPath(\1, "latest")/g' iroll/e2e/testenv/setup.go
sed -i 's/store\.DbPath(\([^,)]*\))/store.DbPath(\1, "latest")/g' iroll/e2e/testenv/setup.go
# Fix testenv Build method
```

- [ ] **Step 2: Build and fix errors**

```bash
cd iroll && go build ./... 2>&1
```

Fix each compilation error manually. Key files to check:
- `cmd/load.go` — `ReadName` uses `store.DbPath`
- `cmd/save.go` — zips from `store.IrollPath`
- `cmd/rm.go` — removes via `store.IrollPath`
- `cmd/inspect.go` — opens `store.DbPath`
- `cmd/history.go` — opens `store.DbPath`
- `cmd/query_dna.go` — opens `store.DbPath`
- `cmd/context.go` — resolves via `store.IrollPath`
- `cmd/page.go` — creates pages via `store.DbPath`
- `cmd/loop.go` — uses iroll path
- `cmd/loop_seed.go` — uses iroll path
- `cmd/loop_run.go` — uses iroll path
- `cmd/memory.go` — uses db path
- `cmd/skill.go` — uses db path
- `cmd/book.go` — uses db path
- `cmd/hub_push.go` — uses `store.IrollPath`
- `cmd/hub_pull.go` — uses `store.Extract`

- [ ] **Step 3: Run tests and fix failures**

```bash
cd iroll && go test ./... 2>&1
```

- [ ] **Step 4: Commit**

```bash
git add -u .
git commit -m "refactor: update all callers for versioned store API"
```

---

### Task 4: E2E tests + final integration

**Files:**
- Modify: `iroll/e2e/testenv/setup.go`
- Modify: `iroll/e2e/scenario_*.go` (as needed)
- Modify: `iroll/cmd/loop_integration_test.go`

- [ ] **Step 1: Update testenv Build helper**

```go
func (e *Env) Build(tagName string) (*builder.BuildResult, error) {
	e.t.Helper()
	name, version, err := builder.ParseTag(tagName)
	if err != nil {
		return nil, err
	}
	lfPath := filepath.Join("..", "..", "examples", "base-agent", "Irollfile")
	lf, err := builder.ParseIrollfile(lfPath)
	if err != nil {
		return nil, err
	}
	return builder.Build(lf, name, version)
}
```

- [ ] **Step 2: Update e2e tests that call build**

Replace `Build("test-name")` → `Build("test-name:v0.1.0")` or `Build("test-name")` (implicit latest).

- [ ] **Step 3: Run full test suite**

```bash
cd iroll && go test ./... -v && go vet ./...
```

- [ ] **Step 4: Rebuild and manual test**

```bash
cd iroll && go build -ldflags "-X logos/cmd.Version=0.1.0" -o ../logos .
cd .. && ./logos roll build -t test-v:v0.1.0  # with version
./logos roll build -t test-latest             # default latest
./logos status                                  # check list output
```

- [ ] **Step 5: Final commit**

```bash
git add -A && git commit -m "test: update e2e tests for versioned build system"
```
