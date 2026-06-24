package db

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
)

func TestListActiveRunsIsPageScoped(t *testing.T) {
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

	completedStartedAt := "2026-06-09T09:00:00.000000000Z"
	completedAt := "2026-06-09T10:00:00.900000000Z"
	abortedStartedAt := "2026-06-09T09:30:00.000000000Z"
	abortedAt := "2026-06-09T10:00:00.100000000Z"
	if _, err := conn.Exec(`
		INSERT INTO loop_runs (
			loop_id, page_id, seed_name, seed_describe, seed_content, seed_weight,
			status, plan, progress, result, reflection, started_at, ended_at, updated_at
		)
		SELECT id, 'page-b', name, describe, content, weight,
			'completed', 'null', 'null', '{"outcome":"latest"}', 'null', ?, ?, ?
		FROM loop WHERE name = 'review'
	`, completedStartedAt, completedAt, completedAt); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`
		INSERT INTO loop_runs (
			loop_id, page_id, seed_name, seed_describe, seed_content, seed_weight,
			status, plan, progress, result, reflection, started_at, ended_at, updated_at
		)
		SELECT id, 'page-c', name, describe, content, weight,
			'aborted', 'null', 'null', '{"outcome":"ended-earlier"}', 'null', ?, ?, ?
		FROM loop WHERE name = 'review'
	`, abortedStartedAt, abortedAt, abortedAt); err != nil {
		t.Fatal(err)
	}

	runs, err := ListActiveRuns(conn, "page-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("page-a active runs = %d, want 2", len(runs))
	}
	if runs[0].ParentRunID != nil || runs[0].ID != mainA.ID {
		t.Fatalf("page-a main = %#v", runs[0])
	}
	if runs[1].ParentRunID == nil || *runs[1].ParentRunID != mainA.ID || runs[1].ID != childA.ID {
		t.Fatalf("page-a child = %#v", runs[1])
	}
	if string(runs[0].Plan) != `{"step":1}` || string(runs[1].Plan) != `["child"]` {
		t.Fatalf("plan values = main %s child %s", runs[0].Plan, runs[1].Plan)
	}

	runs, err = ListActiveRuns(conn, "page-b")
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != mainB.ID {
		t.Fatalf("page-b active runs = %#v", runs)
	}

	runs, err = ListActiveRuns(conn, "page-new")
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("new page active runs = %#v", runs)
	}
}

func TestListAvailableLoopSeedsIsGlobal(t *testing.T) {
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
	if _, err := StartLoopRun(conn, "page-a", "heavy", &mainA.ID, `["child"]`); err != nil {
		t.Fatal(err)
	}
	if _, err := StartLoopRun(conn, "page-b", "heavy", nil, `null`); err != nil {
		t.Fatal(err)
	}

	completedStartedAt := "2026-06-09T09:00:00.000000000Z"
	completedAt := "2026-06-09T10:00:00.900000000Z"
	abortedStartedAt := "2026-06-09T09:30:00.000000000Z"
	abortedAt := "2026-06-09T10:00:00.100000000Z"
	if _, err := conn.Exec(`
		INSERT INTO loop_runs (
			loop_id, page_id, seed_name, seed_describe, seed_content, seed_weight,
			status, plan, progress, result, reflection, started_at, ended_at, updated_at
		)
		SELECT id, 'page-b', name, describe, content, weight,
			'completed', 'null', 'null', '{"outcome":"latest"}', 'null', ?, ?, ?
		FROM loop WHERE name = 'review'
	`, completedStartedAt, completedAt, completedAt); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`
		INSERT INTO loop_runs (
			loop_id, page_id, seed_name, seed_describe, seed_content, seed_weight,
			status, plan, progress, result, reflection, started_at, ended_at, updated_at
		)
		SELECT id, 'page-c', name, describe, content, weight,
			'aborted', 'null', 'null', '{"outcome":"ended-earlier"}', 'null', ?, ?, ?
		FROM loop WHERE name = 'review'
	`, abortedStartedAt, abortedAt, abortedAt); err != nil {
		t.Fatal(err)
	}

	seeds, err := ListAvailableLoopSeeds(conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(seeds) != 3 {
		t.Fatalf("available = %#v", seeds)
	}
	if got := []string{seeds[0].Name, seeds[1].Name, seeds[2].Name}; got[0] != "heavy" || got[1] != "alpha" || got[2] != "review" {
		t.Fatalf("available order = %v", got)
	}
	review := seeds[2]
	if review.Stats.Active != 1 || review.Stats.Completed != 1 || review.Stats.Aborted != 1 {
		t.Fatalf("review stats = %#v", review.Stats)
	}
	if review.Stats.LastEndedAt == nil || *review.Stats.LastEndedAt != completedAt || string(review.Stats.LastResult) != `{"outcome":"latest"}` {
		t.Fatalf("review last ended stats = %#v", review.Stats)
	}

	data, err := json.Marshal(review)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(data) {
		t.Fatalf("seed is invalid JSON: %s", data)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	stats := decoded["stats"].(map[string]any)
	active, ok := stats["active"].(float64)
	if !ok || int(active) != 1 {
		t.Fatalf("stats.active = %#v (type %T), want 1", stats["active"], stats["active"])
	}
}

func TestAvailableLoopSeedsLatestEndedQueryUsesPartialIndex(t *testing.T) {
	conn := setupLoopRunTest(t)
	seed, err := GetLoopSeedByName(conn, "review")
	if err != nil {
		t.Fatal(err)
	}

	rows, err := conn.Query(`
		EXPLAIN QUERY PLAN
		SELECT id
		FROM loop_runs
		WHERE loop_id = ?
			AND status IN ('completed', 'aborted')
			AND ended_at IS NOT NULL
		ORDER BY ended_at DESC, id DESC
		LIMIT 1
	`, seed.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var details []string
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatal(err)
		}
		details = append(details, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	plan := strings.Join(details, "\n")
	if !strings.Contains(plan, "idx_loop_runs_loop_ended") || strings.Contains(plan, "TEMP B-TREE") {
		t.Fatalf("latest ended query plan = %q", plan)
	}
}

func TestListAvailableLoopSeedsSelectsLatestEndAcrossOverlappingRuns(t *testing.T) {
	conn := setupLoopRunTest(t)
	insertEndedReviewRun(t, conn, "page-a", "completed",
		"2026-06-09T09:00:00.000000000Z", "2026-06-09T10:00:00.900000000Z", `{"outcome":"ended-last"}`)
	insertEndedReviewRun(t, conn, "page-b", "aborted",
		"2026-06-09T09:30:00.000000000Z", "2026-06-09T10:00:00.100000000Z", `{"outcome":"started-last"}`)

	seeds, err := ListAvailableLoopSeeds(conn)
	if err != nil {
		t.Fatal(err)
	}
	stats := seeds[0].Stats
	if stats.LastEndedAt == nil || *stats.LastEndedAt != "2026-06-09T10:00:00.900000000Z" ||
		string(stats.LastResult) != `{"outcome":"ended-last"}` {
		t.Fatalf("latest ended stats = %#v", stats)
	}
}

func TestListAvailableLoopSeedsPreservesSubMillisecondLatestEnd(t *testing.T) {
	conn := setupLoopRunTest(t)
	insertEndedReviewRun(t, conn, "page-a", "completed",
		"2026-06-09T09:00:00.000000000Z", "2026-06-09T10:00:00.123456789Z", `{"outcome":"one-nanosecond-later"}`)
	insertEndedReviewRun(t, conn, "page-b", "aborted",
		"2026-06-09T09:30:00.000000000Z", "2026-06-09T10:00:00.123456788Z", `{"outcome":"higher-id"}`)

	seeds, err := ListAvailableLoopSeeds(conn)
	if err != nil {
		t.Fatal(err)
	}
	stats := seeds[0].Stats
	if stats.LastEndedAt == nil || *stats.LastEndedAt != "2026-06-09T10:00:00.123456789Z" ||
		string(stats.LastResult) != `{"outcome":"one-nanosecond-later"}` {
		t.Fatalf("sub-millisecond latest ended stats = %#v", stats)
	}
}

func TestListAvailableLoopSeedsBreaksEqualEndTimeTieByRunID(t *testing.T) {
	conn := setupLoopRunTest(t)
	endedAt := "2026-06-09T10:00:00.123456789Z"
	insertEndedReviewRun(t, conn, "page-a", "completed",
		"2026-06-09T09:00:00.000000000Z", endedAt, `{"outcome":"first"}`)
	insertEndedReviewRun(t, conn, "page-b", "aborted",
		"2026-06-09T09:30:00.000000000Z", endedAt, `{"outcome":"second"}`)

	seeds, err := ListAvailableLoopSeeds(conn)
	if err != nil {
		t.Fatal(err)
	}
	stats := seeds[0].Stats
	if stats.LastEndedAt == nil || *stats.LastEndedAt != endedAt ||
		string(stats.LastResult) != `{"outcome":"second"}` {
		t.Fatalf("latest ended tie stats = %#v", stats)
	}
}

func TestListAvailableLoopSeedsIgnoresTerminalRunWithoutEndedAt(t *testing.T) {
	conn := setupLoopRunTest(t)
	endedAt := "2026-06-09T10:00:00.123456789Z"
	insertEndedReviewRun(t, conn, "page-a", "completed",
		"2026-06-09T09:00:00.000000000Z", endedAt, `{"outcome":"valid"}`)
	if _, err := conn.Exec(`
		INSERT INTO loop_runs (
			loop_id, page_id, seed_name, seed_describe, seed_content, seed_weight,
			status, plan, progress, result, reflection, started_at, ended_at, updated_at
		)
		SELECT id, 'page-b', name, describe, content, weight,
			'aborted', 'null', 'null', '{"outcome":"malformed"}', 'null',
			'2026-06-09T09:30:00.000000000Z', NULL, '2026-06-09T10:01:00.000000000Z'
		FROM loop WHERE name = 'review'
	`); err != nil {
		t.Fatal(err)
	}

	seeds, err := ListAvailableLoopSeeds(conn)
	if err != nil {
		t.Fatal(err)
	}
	stats := seeds[0].Stats
	if stats.LastEndedAt == nil || *stats.LastEndedAt != endedAt ||
		string(stats.LastResult) != `{"outcome":"valid"}` {
		t.Fatalf("latest ended stats with malformed terminal run = %#v", stats)
	}
}

func TestAvailableLoopSeedsAggregateQueryUsesEndedPartialIndex(t *testing.T) {
	conn := setupLoopRunTest(t)
	rows, err := conn.Query("EXPLAIN QUERY PLAN " + availableLoopSeedsQuery)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var details []string
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatal(err)
		}
		details = append(details, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	plan := strings.Join(details, "\n")
	if !strings.Contains(plan, "idx_loop_runs_loop_ended") {
		t.Fatalf("available seed aggregate query plan = %q", plan)
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
	loopFocus := context["loop_focus"].([]any)
	if len(loopFocus) != 2 {
		t.Fatalf("loop_focus length = %d, want 2", len(loopFocus))
	}
	mainEntry := loopFocus[0].(map[string]any)
	childEntry := loopFocus[1].(map[string]any)
	if mainEntry["parent_run_id"] != nil {
		t.Fatalf("main run has parent: %#v", mainEntry)
	}
	if childEntry["parent_run_id"] == nil {
		t.Fatalf("child run has no parent: %#v", childEntry)
	}
	if context["loop_available"] == nil {
		t.Fatal("loop_available is nil")
	}
	if context["system_prompt"] != "hello" {
		t.Fatalf("ordinary context changed: %#v", context)
	}
}

func insertEndedReviewRun(t *testing.T, conn *sql.DB, pageID, status, startedAt, endedAt, result string) int64 {
	t.Helper()
	insert, err := conn.Exec(`
		INSERT INTO loop_runs (
			loop_id, page_id, seed_name, seed_describe, seed_content, seed_weight,
			status, plan, progress, result, reflection, started_at, ended_at, updated_at
		)
		SELECT id, ?, name, describe, content, weight,
			?, 'null', 'null', ?, 'null', ?, ?, ?
		FROM loop WHERE name = 'review'
	`, pageID, status, result, startedAt, endedAt, endedAt)
	if err != nil {
		t.Fatal(err)
	}
	id, err := insert.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}
