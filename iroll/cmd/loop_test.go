package cmd

import (
	"bytes"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"logos/db"
	"logos/store"

	"github.com/spf13/cobra"
)

func TestRunLoopSeedLifecycleUsesActivePage(t *testing.T) {
	cwd, _ := setupLoopCommandTest(t)

	added, err := runLoopAdd(cwd, " review ", " Review memory ", " Inspect memories ", 0.8)
	if err != nil {
		t.Fatal(err)
	}
	if added.Name != "review" || added.Describe != "Review memory" || added.Content != "Inspect memories" {
		t.Fatalf("runLoopAdd = %#v", added)
	}

	inspected, err := runLoopInspect(cwd, "review")
	if err != nil {
		t.Fatal(err)
	}
	if inspected.ID != added.ID {
		t.Fatalf("runLoopInspect = %#v, want ID %d", inspected, added.ID)
	}

	describe := "Review useful memory"
	content := "Inspect and summarize memories"
	weight := 0.6
	edited, err := runLoopEdit(cwd, "review", db.LoopSeedPatch{
		Describe: &describe,
		Content:  &content,
		Weight:   &weight,
	})
	if err != nil {
		t.Fatal(err)
	}
	if edited.Describe != describe || edited.Content != content || edited.Weight != weight {
		t.Fatalf("runLoopEdit = %#v", edited)
	}

	archived, err := runLoopArchive(cwd, "review")
	if err != nil {
		t.Fatal(err)
	}
	if archived.ArchivedAt == nil {
		t.Fatalf("runLoopArchive = %#v", archived)
	}
	active, err := runLoopList(cwd, false)
	if err != nil || len(active) != 0 {
		t.Fatalf("runLoopList(active) = %#v, %v", active, err)
	}
	all, err := runLoopList(cwd, true)
	if err != nil || len(all) != 1 || all[0].Name != "review" {
		t.Fatalf("runLoopList(archived) = %#v, %v", all, err)
	}

	restored, err := runLoopRestore(cwd, "review")
	if err != nil {
		t.Fatal(err)
	}
	if restored.ArchivedAt != nil {
		t.Fatalf("runLoopRestore = %#v", restored)
	}
	if err := runLoopRemove(cwd, "review"); err != nil {
		t.Fatal(err)
	}
	if _, err := runLoopInspect(cwd, "review"); err == nil {
		t.Fatal("runLoopInspect found removed seed")
	}
}

func TestRunLoopLifecycleUsesCurrentPageMainByDefault(t *testing.T) {
	cwd, _ := setupLoopCommandTest(t)
	if _, err := runLoopAdd(cwd, "review", "Review memory", "Inspect memories", 0.8); err != nil {
		t.Fatal(err)
	}
	main, err := runLoopStart(cwd, "review", nil, `{"step":1}`)
	if err != nil {
		t.Fatal(err)
	}
	progress := `{"done":1}`
	updated, err := runLoopUpdate(cwd, nil, nil, &progress)
	if err != nil || string(updated.Progress) != progress {
		t.Fatalf("runLoopUpdate = %#v, %v", updated, err)
	}
	current, err := runLoopCurrent(cwd)
	if err != nil || current.Focus.Main == nil || current.Focus.Main.ID != main.ID {
		t.Fatalf("runLoopCurrent = %#v, %v", current, err)
	}
	completed, err := runLoopComplete(cwd, nil, `{"summary":"done"}`)
	if err != nil || completed.ID != main.ID || completed.Status != "completed" {
		t.Fatalf("runLoopComplete = %#v, %v", completed, err)
	}
	reflected, err := runLoopReflect(cwd, main.ID, "learned")
	if err != nil || string(reflected.Reflection) != `"learned"` {
		t.Fatalf("runLoopReflect = %#v, %v", reflected, err)
	}
	shown, err := runLoopShow(cwd, main.ID)
	if err != nil || shown.ID != main.ID {
		t.Fatalf("runLoopShow = %#v, %v", shown, err)
	}
	history, err := runLoopHistory(cwd, "review", "page-one", 10)
	if err != nil || len(history) != 1 || history[0].ID != main.ID {
		t.Fatalf("runLoopHistory = %#v, %v", history, err)
	}
}

func TestRunLoopChildMustEndBeforeMain(t *testing.T) {
	cwd, _ := setupLoopCommandTest(t)
	if _, err := runLoopAdd(cwd, "main", "Main", "Main", 0.8); err != nil {
		t.Fatal(err)
	}
	if _, err := runLoopAdd(cwd, "child", "Child", "Child", 0.7); err != nil {
		t.Fatal(err)
	}
	main, err := runLoopStart(cwd, "main", nil, "plan")
	if err != nil {
		t.Fatal(err)
	}
	child, err := runLoopStart(cwd, "child", &main.ID, "child plan")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runLoopComplete(cwd, nil, "done"); err == nil {
		t.Fatal("completed main with active child")
	}
	aborted, err := runLoopAbort(cwd, &child.ID, "not needed", "")
	if err != nil || aborted.Status != "aborted" {
		t.Fatalf("runLoopAbort = %#v, %v", aborted, err)
	}
	if _, err := runLoopComplete(cwd, nil, "done"); err != nil {
		t.Fatal(err)
	}
}

func TestLoopRunCommandWiringAndIDValidation(t *testing.T) {
	tests := []struct {
		name     string
		use      string
		flags    []string
		required []string
	}{
		{name: "run", use: "run <name>", flags: []string{"parent", "plan", "cwd"}},
		{name: "update", use: "update [run-id]", flags: []string{"plan", "progress", "cwd"}},
		{name: "complete", use: "complete [run-id]", flags: []string{"result", "cwd"}, required: []string{"result"}},
		{name: "abort", use: "abort [run-id]", flags: []string{"reason", "result", "cwd"}, required: []string{"reason"}},
		{name: "reflect", use: "reflect <run-id>", flags: []string{"content", "cwd"}, required: []string{"content"}},
		{name: "current", use: "current", flags: []string{"cwd"}},
		{name: "history", use: "history <name>", flags: []string{"page", "limit", "cwd"}},
		{name: "show", use: "show <run-id>", flags: []string{"cwd"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := findSubcommand(loopCmd, tt.name)
			if command == nil || command.Use != tt.use {
				t.Fatalf("command = %#v, want use %q", command, tt.use)
			}
			for _, flag := range tt.flags {
				if command.Flags().Lookup(flag) == nil {
					t.Errorf("missing --%s", flag)
				}
			}
			for _, flag := range tt.required {
				if len(command.Flags().Lookup(flag).Annotations[cobra.BashCompOneRequiredFlag]) == 0 {
					t.Errorf("--%s is not required", flag)
				}
			}
		})
	}
	for _, invalid := range []string{"", "0", "-1", "abc"} {
		if _, err := parseLoopRunID(invalid); err == nil {
			t.Fatalf("parseLoopRunID(%q) succeeded", invalid)
		}
	}
}

func TestLoopUpdateCommandDoesNotReuseChangedFlags(t *testing.T) {
	var updates [][2]string
	command := newLoopUpdateCmd(func(cwd string, runID *int64, plan, progress *string) error {
		update := [2]string{"<nil>", "<nil>"}
		if plan != nil {
			update[0] = *plan
		}
		if progress != nil {
			update[1] = *progress
		}
		updates = append(updates, update)
		return nil
	})
	command.SetArgs([]string{"--progress", "first"})
	if err := executeLoopCommand(command); err != nil {
		t.Fatal(err)
	}
	command.SetArgs(nil)
	if err := executeLoopCommand(command); err != nil {
		t.Fatal(err)
	}
	if len(updates) != 2 || updates[0] != [2]string{"<nil>", "first"} ||
		updates[1] != [2]string{"<nil>", "<nil>"} {
		t.Fatalf("updates reused flags: %#v", updates)
	}
}

func TestRunLoopSeedRejectsInvalidValuesAndNoEditFields(t *testing.T) {
	cwd, _ := setupLoopCommandTest(t)
	tests := []struct {
		name string
		run  func() error
		want string
	}{
		{
			name: "blank required",
			run: func() error {
				_, err := runLoopAdd(cwd, "review", " ", "content", 0.5)
				return err
			},
			want: "describe must not be blank",
		},
		{
			name: "invalid weight",
			run: func() error {
				_, err := runLoopAdd(cwd, "review", "describe", "content", 1.1)
				return err
			},
			want: "weight must be between 0 and 1",
		},
		{
			name: "no edit fields",
			run: func() error {
				_, err := runLoopEdit(cwd, "review", db.LoopSeedPatch{})
				return err
			},
			want: "no fields supplied",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestRunLoopEditPreservesExplicitEmptyValuesForDBValidation(t *testing.T) {
	cwd, _ := setupLoopCommandTest(t)
	if _, err := runLoopAdd(cwd, "review", "Review", "Inspect", 0.5); err != nil {
		t.Fatal(err)
	}
	empty := ""
	_, err := runLoopEdit(cwd, "review", db.LoopSeedPatch{Content: &empty})
	if err == nil || !strings.Contains(err.Error(), "content must not be blank") {
		t.Fatalf("runLoopEdit error = %v, want blank content error", err)
	}
}

func TestLoopSeedPatchFromFlagsDistinguishesOmittedAndEmpty(t *testing.T) {
	command := &cobra.Command{}
	command.Flags().String("describe", "", "")
	command.Flags().String("content", "", "")
	command.Flags().Float64("weight", 0.5, "")

	omitted := loopSeedPatchFromFlags(command, "", "", 0.5)
	if omitted.Describe != nil || omitted.Content != nil || omitted.Weight != nil {
		t.Fatalf("omitted patch = %#v", omitted)
	}
	if err := command.Flags().Set("content", ""); err != nil {
		t.Fatal(err)
	}
	explicitEmpty := loopSeedPatchFromFlags(command, "", "", 0.5)
	if explicitEmpty.Content == nil || *explicitEmpty.Content != "" {
		t.Fatalf("explicit empty patch = %#v", explicitEmpty)
	}
}

func TestRunLoopRemoveRejectsSeedWithHistory(t *testing.T) {
	cwd, conn := setupLoopCommandTest(t)
	seed, err := runLoopAdd(cwd, "review", "Review", "Inspect", 0.5)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`
		INSERT INTO loop_runs (
			loop_id, page_id, seed_name, seed_describe, seed_content, seed_weight,
			status, plan, progress, result, reflection, started_at, updated_at
		) VALUES (?, 'page-one', ?, ?, ?, ?, 'completed', 'null', 'null', 'null', 'null', 'now', 'now')
	`, seed.ID, seed.Name, seed.Describe, seed.Content, seed.Weight); err != nil {
		t.Fatal(err)
	}
	if err := runLoopRemove(cwd, "review"); err == nil || !strings.Contains(err.Error(), "archive it instead") {
		t.Fatalf("runLoopRemove error = %v, want archive instruction", err)
	}
}

func TestOpenActiveLoopRequiresActivePage(t *testing.T) {
	setLoopCommandTestHome(t)
	_, _, conn, err := openActiveLoop(t.TempDir())
	if conn != nil {
		conn.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "no active page") {
		t.Fatalf("openActiveLoop error = %v, want no active page", err)
	}
}

func TestOpenActiveLoopResolvesAbsoluteCwdAndReturnsContext(t *testing.T) {
	cwd, _ := setupLoopCommandTest(t)
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relativeCwd, err := filepath.Rel(workingDir, cwd)
	if err != nil {
		t.Skipf("cannot compute relative path from %s to %s: %v", workingDir, cwd, err)
	}
	name, pageID, conn, err := openActiveLoop(relativeCwd)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if name != "test-roll" || pageID != "page-one" {
		t.Fatalf("openActiveLoop = %q, %q, want test-roll, page-one", name, pageID)
	}
}

func TestLoopCommandWiringAndFlags(t *testing.T) {
	if findSubcommand(rootCmd, "loop") != loopCmd {
		t.Fatal("root loop command is not registered")
	}
	tests := []struct {
		name     string
		use      string
		flags    []string
		required []string
	}{
		{name: "list", use: "list", flags: []string{"archived", "cwd"}},
		{name: "inspect", use: "inspect <name>", flags: []string{"cwd"}},
		{name: "add", use: "add <name>", flags: []string{"describe", "content", "weight", "cwd"}, required: []string{"describe", "content"}},
		{name: "edit", use: "edit <name>", flags: []string{"describe", "content", "weight", "cwd"}},
		{name: "remove", use: "remove <name>", flags: []string{"cwd"}},
		{name: "archive", use: "archive <name>", flags: []string{"cwd"}},
		{name: "restore", use: "restore <name>", flags: []string{"cwd"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := findSubcommand(loopCmd, tt.name)
			if command == nil || command.Use != tt.use {
				t.Fatalf("command = %#v, want use %q", command, tt.use)
			}
			for _, flag := range tt.flags {
				if command.Flags().Lookup(flag) == nil {
					t.Errorf("missing --%s", flag)
				}
			}
			for _, flag := range tt.required {
				annotation := command.Flags().Lookup(flag).Annotations[cobra.BashCompOneRequiredFlag]
				if len(annotation) == 0 || annotation[0] != "true" {
					t.Errorf("--%s is not required", flag)
				}
			}
		})
	}
}

func TestLoopAddCommandRequiresFlagsOnEveryExecution(t *testing.T) {
	var calls int
	command := newLoopAddCmd(func(cwd, name, describe, content string, weight float64) error {
		calls++
		return nil
	})

	command.SetArgs([]string{"review", "--describe", "Review", "--content", "Inspect"})
	if err := executeLoopCommand(command); err != nil {
		t.Fatalf("first execution: %v", err)
	}
	command.SetArgs([]string{"again", "--describe", "Leaked"})
	err := executeLoopCommand(command)
	if err == nil || !strings.Contains(err.Error(), `required flag(s) "content" not set`) {
		t.Fatalf("second execution error = %v, want content required error", err)
	}
	command.SetArgs([]string{"third", "--content", "Fresh"})
	err = executeLoopCommand(command)
	if err == nil || !strings.Contains(err.Error(), `required flag(s) "describe" not set`) {
		t.Fatalf("third execution error = %v, want describe required error", err)
	}
	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1", calls)
	}
}

func TestLoopEditCommandDoesNotReuseChangedFlags(t *testing.T) {
	errNoFields := errors.New("no fields supplied")
	var patches []db.LoopSeedPatch
	command := newLoopEditCmd(func(cwd, name string, patch db.LoopSeedPatch) error {
		patches = append(patches, patch)
		if patch.Describe == nil && patch.Content == nil && patch.Weight == nil {
			return errNoFields
		}
		return nil
	})

	command.SetArgs([]string{"review", "--content", "First"})
	if err := executeLoopCommand(command); err != nil {
		t.Fatalf("first execution: %v", err)
	}
	command.SetArgs([]string{"review"})
	if err := executeLoopCommand(command); !errors.Is(err, errNoFields) {
		t.Fatalf("second execution error = %v, want no fields error", err)
	}
	if len(patches) != 2 || patches[0].Content == nil || *patches[0].Content != "First" {
		t.Fatalf("patches = %#v", patches)
	}
	if patches[1].Describe != nil || patches[1].Content != nil || patches[1].Weight != nil {
		t.Fatalf("second patch reused flags: %#v", patches[1])
	}
}

func TestLoopListCommandDoesNotReuseFlagValues(t *testing.T) {
	var archived []bool
	command := newLoopListCmd(func(cwd string, includeArchived bool) error {
		archived = append(archived, includeArchived)
		return nil
	})

	command.SetArgs([]string{"--archived"})
	if err := executeLoopCommand(command); err != nil {
		t.Fatalf("first execution: %v", err)
	}
	command.SetArgs(nil)
	if err := executeLoopCommand(command); err != nil {
		t.Fatalf("second execution: %v", err)
	}
	command.SetArgs([]string{"--archived", "unexpected"})
	if err := executeLoopCommand(command); err == nil {
		t.Fatal("third execution accepted an argument")
	}
	command.SetArgs(nil)
	if err := executeLoopCommand(command); err != nil {
		t.Fatalf("fourth execution: %v", err)
	}
	if len(archived) != 3 || !archived[0] || archived[1] || archived[2] {
		t.Fatalf("archived values = %#v, want [true false false]", archived)
	}
}

func executeLoopCommand(command *cobra.Command) error {
	command.SilenceErrors = true
	command.SilenceUsage = true
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	return command.Execute()
}

func setupLoopCommandTest(t *testing.T) (string, *sql.DB) {
	t.Helper()
	setLoopCommandTestHome(t)
	cwd := filepath.Join(t.TempDir(), "workspace")
	absoluteCwd, err := filepath.Abs(cwd)
	if err != nil {
		t.Fatal(err)
	}
	dbPath, err := store.DbPath("test-roll", "latest")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		t.Fatal(err)
	}
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	schema, err := os.ReadFile(filepath.Join("..", "..", "examples", "base-agent", "init_schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`
		INSERT INTO pages (page_id, cwd, context, created_at, updated_at)
		VALUES ('page-one', ?, '{}', 'now', 'now')
	`, absoluteCwd); err != nil {
		t.Fatal(err)
	}
	if err := store.IndexPage("test-roll", "page-one", absoluteCwd); err != nil {
		t.Fatal(err)
	}
	return cwd, conn
}

func setLoopCommandTestHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
}

func findSubcommand(parent *cobra.Command, name string) *cobra.Command {
	for _, command := range parent.Commands() {
		if command.Name() == name {
			return command
		}
	}
	return nil
}
