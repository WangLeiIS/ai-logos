package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"logos/builder"
	"logos/db"
	"logos/store"
)

func TestLoopEndToEndAcrossIndependentPages(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	layerfile, err := builder.ParseIrollfile(filepath.Join("..", "..", "examples", "base-agent", "Irollfile"))
	if err != nil {
		t.Fatal(err)
	}
	buildResult, err := builder.Build(layerfile, "loop-e2e", "latest")
	if err != nil {
		t.Fatal(err)
	}
	innerPath, err := store.InnerDbPath("loop-e2e", "latest")
	if err != nil {
		t.Fatal(err)
	}

	// Copy outer template to workspace and open with ATTACH
	outerPath := filepath.Join(buildResult.Path, "roll-outer.db")
	workspaceOuter := filepath.Join(t.TempDir(), "loop-e2e.outer.db")
	outerData, err := os.ReadFile(outerPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(workspaceOuter, outerData, 0644); err != nil {
		t.Fatal(err)
	}

	conn, err := db.OpenOuter(workspaceOuter, innerPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	pageA, err := db.InsertPage(conn, filepath.Join(t.TempDir(), "page-a"))
	if err != nil {
		t.Fatal(err)
	}
	pageB, err := db.InsertPage(conn, filepath.Join(t.TempDir(), "page-b"))
	if err != nil {
		t.Fatal(err)
	}
	runA, err := db.StartLoopRun(conn, pageA.PageID, "observe-human", nil, `{"steps":["read context"]}`)
	if err != nil {
		t.Fatal(err)
	}
	runB, err := db.StartLoopRun(conn, pageB.PageID, "observe-human", nil, `{"steps":["review dna"]}`)
	if err != nil {
		t.Fatal(err)
	}
	progress := `{"read_context":true}`
	if _, err := db.UpdateLoopRun(conn, pageA.PageID, nil, nil, &progress); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CompleteLoopRun(conn, pageA.PageID, nil, `{"summary":"understood"}`); err != nil {
		t.Fatal(err)
	}

	runsA, err := db.ListActiveRuns(conn, pageA.PageID)
	if err != nil {
		t.Fatal(err)
	}
	runsB, err := db.ListActiveRuns(conn, pageB.PageID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runsA) != 0 {
		t.Fatalf("page A still has active runs after completion: %#v", runsA)
	}
	if len(runsB) == 0 || runsB[0].ID != runB.ID || runA.ID == runB.ID {
		t.Fatalf("page B runs = %#v; runs A=%d B=%d", runsB, runA.ID, runB.ID)
	}
}
