package db

import (
	"encoding/json"
	"testing"
)

func TestBuildLoopContextIsPageScopedWithGlobalAvailableStats(t *testing.T) {
	conn := setupLoopRunTest(t)
	if _, err := InsertLoopSeed(conn, "heavy", "Heavy work", "Do heavy work", 0.9); err != nil {
		t.Fatal(err)
	}
	if _, err := InsertLoopSeed(conn, "alpha", "Alpha work", "Do alpha work", 0.8); err != nil {
		t.Fatal(err)
	}
	if _, err := InsertLoopSeed(conn, "archived", "Archived work", "Do archived work", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := ArchiveLoopSeed(conn, "archived"); err != nil {
		t.Fatal(err)
	}

	mainA, err := StartLoopRun(conn, "page-a", "review", nil, `{"step":1}`)
	if err != nil {
		t.Fatal(err)
	}
	childA, err := StartLoopRun(conn, "page-a", "heavy", &mainA.ID, `["child"]`)
	if err != nil {
		t.Fatal(err)
	}
	mainB, err := StartLoopRun(conn, "page-b", "heavy", nil, `null`)
	if err != nil {
		t.Fatal(err)
	}

	completedAt := "2026-06-09T10:00:00Z"
	abortedAt := "2026-06-09T11:00:00Z"
	if _, err := conn.Exec(`
		INSERT INTO loop_runs (
			loop_id, page_id, seed_name, seed_describe, seed_content, seed_weight,
			status, plan, progress, result, reflection, started_at, ended_at, updated_at
		)
		SELECT id, 'page-b', name, describe, content, weight,
			'completed', 'null', 'null', '{"outcome":"old"}', 'null', ?, ?, ?
		FROM loop WHERE name = 'review'
	`, completedAt, completedAt, completedAt); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`
		INSERT INTO loop_runs (
			loop_id, page_id, seed_name, seed_describe, seed_content, seed_weight,
			status, plan, progress, result, reflection, started_at, ended_at, updated_at
		)
		SELECT id, 'page-c', name, describe, content, weight,
			'aborted', 'null', 'null', '{"outcome":"latest"}', 'null', ?, ?, ?
		FROM loop WHERE name = 'review'
	`, abortedAt, abortedAt, abortedAt); err != nil {
		t.Fatal(err)
	}

	viewA, err := BuildLoopContext(conn, "page-a")
	if err != nil {
		t.Fatal(err)
	}
	if viewA.Focus.Main == nil || viewA.Focus.Main.ID != mainA.ID {
		t.Fatalf("page-a main = %#v", viewA.Focus.Main)
	}
	if len(viewA.Focus.Children) != 1 || viewA.Focus.Children[0].ID != childA.ID {
		t.Fatalf("page-a children = %#v", viewA.Focus.Children)
	}
	if string(viewA.Focus.Main.Plan) != `{"step":1}` || string(viewA.Focus.Children[0].Plan) != `["child"]` {
		t.Fatalf("focus JSON values = main %s child %s", viewA.Focus.Main.Plan, viewA.Focus.Children[0].Plan)
	}

	viewB, err := BuildLoopContext(conn, "page-b")
	if err != nil {
		t.Fatal(err)
	}
	if viewB.Focus.Main == nil || viewB.Focus.Main.ID != mainB.ID || len(viewB.Focus.Children) != 0 {
		t.Fatalf("page-b focus = %#v", viewB.Focus)
	}

	empty, err := BuildLoopContext(conn, "page-new")
	if err != nil {
		t.Fatal(err)
	}
	if empty.Focus.Main != nil || empty.Focus.Children == nil || len(empty.Focus.Children) != 0 {
		t.Fatalf("new page focus = %#v", empty.Focus)
	}
	if empty.Available == nil {
		t.Fatal("new page available is nil")
	}

	if len(viewA.Available) != 3 {
		t.Fatalf("available = %#v", viewA.Available)
	}
	if got := []string{viewA.Available[0].Name, viewA.Available[1].Name, viewA.Available[2].Name}; got[0] != "heavy" || got[1] != "alpha" || got[2] != "review" {
		t.Fatalf("available order = %v", got)
	}
	review := viewA.Available[2]
	if review.Stats.Active != 1 || review.Stats.Completed != 1 || review.Stats.Aborted != 1 {
		t.Fatalf("review stats = %#v", review.Stats)
	}
	if review.Stats.LastEndedAt == nil || *review.Stats.LastEndedAt != abortedAt || string(review.Stats.LastResult) != `{"outcome":"latest"}` {
		t.Fatalf("review last ended stats = %#v", review.Stats)
	}

	data, err := json.Marshal(viewA)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(data) {
		t.Fatalf("view is invalid JSON: %s", data)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	main := decoded["focus"].(map[string]any)["main"].(map[string]any)
	if _, ok := main["plan"].(map[string]any); !ok {
		t.Fatalf("plan was encoded as a string: %#v", main["plan"])
	}
}

func TestResolveContextInjectsPageLoopViewAndReplacesRawLoop(t *testing.T) {
	conn := setupLoopRunTest(t)
	main, err := StartLoopRun(conn, "page-a", "review", nil, `{"step":1}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := StartLoopRun(conn, "page-a", "review", &main.ID, `null`); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveContext(`{"system_prompt":"hello","loop":"stale"}`, t.TempDir(), conn, "page-a")
	if err != nil {
		t.Fatal(err)
	}
	var context map[string]any
	if err := json.Unmarshal([]byte(got), &context); err != nil {
		t.Fatal(err)
	}
	loop := context["loop"].(map[string]any)
	focus := loop["focus"].(map[string]any)
	if focus["main"] == nil || len(focus["children"].([]any)) != 1 {
		t.Fatalf("loop view = %#v", loop)
	}
	if context["system_prompt"] != "hello" {
		t.Fatalf("ordinary context changed: %#v", context)
	}
}
