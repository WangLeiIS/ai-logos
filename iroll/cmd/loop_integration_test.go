package cmd

import (
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
	if _, err := builder.Build(layerfile, "loop-e2e"); err != nil {
		t.Fatal(err)
	}
	dbPath, err := store.DbPath("loop-e2e")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.Open(dbPath)
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
	runA, err := db.StartLoopRun(conn, pageA.PageID, "self-cognition", nil, `{"steps":["read context"]}`)
	if err != nil {
		t.Fatal(err)
	}
	runB, err := db.StartLoopRun(conn, pageB.PageID, "self-cognition", nil, `{"steps":["review dna"]}`)
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

	gotA, err := db.BuildLoopContext(conn, pageA.PageID)
	if err != nil {
		t.Fatal(err)
	}
	gotB, err := db.BuildLoopContext(conn, pageB.PageID)
	if err != nil {
		t.Fatal(err)
	}
	if gotA.Focus.Main != nil {
		t.Fatalf("page A still focused after completion: %#v", gotA.Focus.Main)
	}
	if gotB.Focus.Main == nil || gotB.Focus.Main.ID != runB.ID || runA.ID == runB.ID {
		t.Fatalf("page B focus = %#v; runs A=%d B=%d", gotB.Focus.Main, runA.ID, runB.ID)
	}
}
