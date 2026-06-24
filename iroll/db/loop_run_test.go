package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStartLoopRunAllowsSameSeedAcrossPagesAndNormalizesPlans(t *testing.T) {
	conn := setupLoopRunTest(t)

	first, err := StartLoopRun(conn, "page-a", "review", nil, ` { "step": 1 } `)
	if err != nil {
		t.Fatal(err)
	}
	second, err := StartLoopRun(conn, "page-b", "review", nil, "review memory")
	if err != nil {
		t.Fatal(err)
	}

	if first.ID == second.ID || first.PageID == second.PageID {
		t.Fatalf("runs are not independent: %#v %#v", first, second)
	}
	assertLoopRunJSON(t, first, `{"step":1}`, `null`, `null`, `null`)
	assertLoopRunJSON(t, second, `"review memory"`, `null`, `null`, `null`)
}

func TestStartLoopRunRejectsBlankPageAndSeed(t *testing.T) {
	conn := setupLoopRunTest(t)

	for name, call := range map[string]func() error{
		"page": func() error {
			_, err := StartLoopRun(conn, " ", "review", nil, "null")
			return err
		},
		"seed": func() error {
			_, err := StartLoopRun(conn, "page-a", "\t", nil, "null")
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := call()
			if err == nil || !errors.Is(err, ErrInvalidLoopRun) || !strings.Contains(err.Error(), "must not be blank") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestStartLoopRunRejectsMissingPageWithStableError(t *testing.T) {
	conn := setupLoopRunTest(t)

	if _, err := StartLoopRun(conn, "missing-page", "review", nil, "null"); err == nil ||
		!errors.Is(err, ErrPageNotFound) || !strings.Contains(err.Error(), `page "missing-page" not found`) {
		t.Fatalf("missing page error = %v", err)
	}
}

func TestStartLoopRunAllowsMultipleMainPerPage(t *testing.T) {
	conn := setupLoopRunTest(t)

	first, err := StartLoopRun(conn, "page-a", "review", nil, "null")
	if err != nil {
		t.Fatal(err)
	}
	second, err := StartLoopRun(conn, "page-a", "review", nil, "null")
	if err != nil {
		t.Fatalf("second main run should be allowed: %v", err)
	}
	if first.ID == second.ID {
		t.Fatalf("expected different runs, got same id %d", first.ID)
	}
	if _, err := StartLoopRun(conn, "page-b", "review", nil, "null"); err != nil {
		t.Fatalf("same seed on another page: %v", err)
	}
}

func TestStartLoopRunConcurrentSamePageBothSucceed(t *testing.T) {
	firstConn, secondConn := setupConcurrentLoopRunTest(t)
	errs := startLoopRunsConcurrently(firstConn, secondConn, "page-a", "page-a")

	var successCount int
	for _, err := range errs {
		switch {
		case err == nil:
			successCount++
		default:
			t.Fatalf("unexpected concurrent start error: %v", err)
		}
	}
	if successCount != 2 {
		t.Fatalf("successes = %d, want 2, errors = %v", successCount, errs)
	}
}

func TestStartLoopRunConcurrentIndependentPagesBothSucceed(t *testing.T) {
	firstConn, secondConn := setupConcurrentLoopRunTest(t)
	errs := startLoopRunsConcurrently(firstConn, secondConn, "page-a", "page-b")

	for i, err := range errs {
		if err != nil {
			t.Fatalf("start %d failed: %v", i, err)
		}
	}
}

func TestStartLoopRunConcurrentWithDeleteCannotLeaveActiveOrphan(t *testing.T) {
	for attempt := 0; attempt < 20; attempt++ {
		first, second := setupConcurrentLoopRunTest(t)
		start := make(chan struct{})
		startResult := make(chan error, 1)
		deleteResult := make(chan error, 1)
		go func() {
			<-start
			_, err := StartLoopRun(first, "page-a", "review", nil, "null")
			startResult <- err
		}()
		go func() {
			<-start
			deleteResult <- DeletePage(second, "page-a")
		}()
		close(start)
		startErr, deleteErr := <-startResult, <-deleteResult

		if deleteErr != nil {
			t.Fatalf("attempt %d delete error = %v; start error = %v", attempt, deleteErr, startErr)
		}
		if startErr != nil && !errors.Is(startErr, ErrPageNotFound) {
			t.Fatalf("attempt %d start error = %v", attempt, startErr)
		}
		if _, err := GetActiveMainLoopRun(first, "page-a"); err == nil {
			t.Fatalf("attempt %d left active orphan run; start error = %v", attempt, startErr)
		} else if !errors.Is(err, ErrLoopRunNotFound) {
			t.Fatal(err)
		}
	}
}

func TestStartLoopRunAllowsMultipleChildrenAndListsThemInStableOrder(t *testing.T) {
	conn := setupLoopRunTest(t)
	main := mustStartLoopRun(t, conn, "page-a", nil)
	first := mustStartLoopRun(t, conn, "page-a", &main.ID)
	second := mustStartLoopRun(t, conn, "page-a", &main.ID)

	children, err := ListActiveChildLoopRuns(conn, main.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 2 || children[0].ID != first.ID || children[1].ID != second.ID {
		t.Fatalf("children = %#v", children)
	}
}

func TestStartLoopRunRequiresActiveMainParentOnSamePage(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		conn := setupLoopRunTest(t)
		parentID := int64(999)
		_, err := StartLoopRun(conn, "page-a", "review", &parentID, "null")
		assertInvalidParentError(t, err, "not found")
	})

	t.Run("different page", func(t *testing.T) {
		conn := setupLoopRunTest(t)
		main := mustStartLoopRun(t, conn, "page-a", nil)
		_, err := StartLoopRun(conn, "page-b", "review", &main.ID, "null")
		assertInvalidParentError(t, err, "same page")
	})

	t.Run("inactive", func(t *testing.T) {
		conn := setupLoopRunTest(t)
		main := mustStartLoopRun(t, conn, "page-a", nil)
		if _, err := conn.Exec("UPDATE loop_runs SET status = 'completed', ended_at = ? WHERE id = ?", nowISO(), main.ID); err != nil {
			t.Fatal(err)
		}
		_, err := StartLoopRun(conn, "page-a", "review", &main.ID, "null")
		assertInvalidParentError(t, err, "active")
	})

	t.Run("grandchild", func(t *testing.T) {
		conn := setupLoopRunTest(t)
		main := mustStartLoopRun(t, conn, "page-a", nil)
		child := mustStartLoopRun(t, conn, "page-a", &main.ID)
		_, err := StartLoopRun(conn, "page-a", "review", &child.ID, "null")
		assertInvalidParentError(t, err, "main")
	})
}

func TestStartLoopRunRejectsMissingAndArchivedSeed(t *testing.T) {
	conn := setupLoopRunTest(t)

	if _, err := StartLoopRun(conn, "page-a", "missing", nil, "null"); err == nil ||
		!errors.Is(err, ErrLoopSeedNotFound) {
		t.Fatalf("missing seed error = %v", err)
	}
	if _, err := ArchiveLoopSeed(conn, "review"); err != nil {
		t.Fatal(err)
	}
	if _, err := StartLoopRun(conn, "page-a", "review", nil, "null"); err == nil ||
		!errors.Is(err, ErrLoopSeedNotFound) || !strings.Contains(err.Error(), "not active") {
		t.Fatalf("archived seed error = %v", err)
	}
}

func TestStartLoopRunPreservesSeedSnapshot(t *testing.T) {
	conn := setupLoopRunTest(t)
	run := mustStartLoopRun(t, conn, "page-a", nil)

	describe := "Changed describe"
	content := "Changed content"
	weight := 0.2
	if _, err := UpdateLoopSeed(conn, "review", LoopSeedPatch{
		Describe: &describe,
		Content:  &content,
		Weight:   &weight,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := GetLoopRun(conn, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.SeedName != "review" || got.SeedDescribe != "Review memory" ||
		got.SeedContent != "Inspect memories" || got.SeedWeight != 0.8 {
		t.Fatalf("snapshot changed: %#v", got)
	}
}

func TestLoopRunGettersDecodeJSONAndReturnStableErrors(t *testing.T) {
	conn := setupLoopRunTest(t)
	main := mustStartLoopRun(t, conn, "page-a", nil)
	child := mustStartLoopRun(t, conn, "page-a", &main.ID)
	if _, err := conn.Exec(`
		UPDATE loop_runs
		SET progress = '{"done":1}', result = '"ok"', reflection = '[1,2]'
		WHERE id = ?
	`, child.ID); err != nil {
		t.Fatal(err)
	}

	got, err := GetLoopRun(conn, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertLoopRunJSON(t, got, `null`, `{"done":1}`, `"ok"`, `[1,2]`)

	active, err := GetActiveMainLoopRun(conn, " page-a ")
	if err != nil {
		t.Fatal(err)
	}
	if active.ID != main.ID {
		t.Fatalf("active main = %#v", active)
	}

	if _, err := GetLoopRun(conn, 999); err == nil || !errors.Is(err, ErrLoopRunNotFound) {
		t.Fatalf("GetLoopRun missing error = %v", err)
	}
	if _, err := GetActiveMainLoopRun(conn, "page-b"); err == nil ||
		!errors.Is(err, ErrLoopRunNotFound) || !strings.Contains(err.Error(), "active main") {
		t.Fatalf("GetActiveMainLoopRun missing error = %v", err)
	}
	if _, err := GetActiveMainLoopRun(conn, " "); err == nil || !errors.Is(err, ErrInvalidLoopRun) {
		t.Fatalf("GetActiveMainLoopRun blank page error = %v", err)
	}
}

func TestLoopRunGettersRejectInvalidStoredJSONWithStableError(t *testing.T) {
	conn := setupLoopRunTest(t)
	main := mustStartLoopRun(t, conn, "page-a", nil)
	child := mustStartLoopRun(t, conn, "page-a", &main.ID)

	if _, err := conn.Exec("UPDATE loop_runs SET plan = 'not-json' WHERE id = ?", main.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := GetLoopRun(conn, main.ID); err == nil ||
		!errors.Is(err, ErrInvalidLoopRun) || !strings.Contains(err.Error(), "invalid plan JSON") {
		t.Fatalf("GetLoopRun invalid JSON error = %v", err)
	}

	if _, err := conn.Exec("UPDATE loop_runs SET result = 'not-json' WHERE id = ?", child.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := ListActiveChildLoopRuns(conn, main.ID); err == nil ||
		!errors.Is(err, ErrInvalidLoopRun) || !strings.Contains(err.Error(), "invalid result JSON") {
		t.Fatalf("ListActiveChildLoopRuns invalid JSON error = %v", err)
	}
}

func TestUpdateLoopRunDefaultsToMainAndReplacesOnlySuppliedFields(t *testing.T) {
	conn := setupLoopRunTest(t)
	main := mustStartLoopRun(t, conn, "page-a", nil)
	child := mustStartLoopRun(t, conn, "page-a", &main.ID)
	plan := `{"steps":[1,2]}`

	updated, err := UpdateLoopRun(conn, " page-a ", nil, &plan, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertLoopRunJSON(t, updated, `{"steps":[1,2]}`, `null`, `null`, `null`)

	progress := "child progress"
	updated, err = UpdateLoopRun(conn, "page-a", &child.ID, nil, &progress)
	if err != nil {
		t.Fatal(err)
	}
	assertLoopRunJSON(t, updated, `null`, `"child progress"`, `null`, `null`)

	gotMain, err := GetLoopRun(conn, main.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertLoopRunJSON(t, gotMain, `{"steps":[1,2]}`, `null`, `null`, `null`)
}

func TestUpdateLoopRunRejectsNoFieldsWrongPageAndEndedRun(t *testing.T) {
	conn := setupLoopRunTest(t)
	main := mustStartLoopRun(t, conn, "page-a", nil)
	child := mustStartLoopRun(t, conn, "page-a", &main.ID)
	progress := "changed"

	for name, call := range map[string]func() error{
		"no fields": func() error {
			_, err := UpdateLoopRun(conn, "page-a", nil, nil, nil)
			return err
		},
		"wrong page": func() error {
			_, err := UpdateLoopRun(conn, "page-b", &child.ID, nil, &progress)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); err == nil || !errors.Is(err, ErrInvalidLoopRun) {
				t.Fatalf("error = %v", err)
			}
		})
	}

	if _, err := AbortLoopRun(conn, "page-a", &child.ID, "not needed", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateLoopRun(conn, "page-a", &child.ID, nil, &progress); err == nil ||
		!errors.Is(err, ErrInvalidLoopRun) || !strings.Contains(err.Error(), "active") {
		t.Fatalf("ended update error = %v", err)
	}
}

func TestCompleteLoopRunRequiresResultAndActiveChildrenToEndFirst(t *testing.T) {
	conn := setupLoopRunTest(t)
	main := mustStartLoopRun(t, conn, "page-a", nil)
	child := mustStartLoopRun(t, conn, "page-a", &main.ID)

	if _, err := CompleteLoopRun(conn, "page-a", nil, ""); err == nil || !errors.Is(err, ErrInvalidLoopRun) {
		t.Fatalf("empty result error = %v", err)
	}
	if _, err := CompleteLoopRun(conn, "page-a", nil, `"done"`); err == nil ||
		!errors.Is(err, ErrInvalidLoopRun) || !strings.Contains(err.Error(), "active child") {
		t.Fatalf("active child error = %v", err)
	}
	if _, err := CompleteLoopRun(conn, "page-b", &child.ID, `"done"`); err == nil ||
		!errors.Is(err, ErrInvalidLoopRun) {
		t.Fatalf("wrong page error = %v", err)
	}
	if _, err := AbortLoopRun(conn, "page-a", &child.ID, "not needed", ""); err != nil {
		t.Fatal(err)
	}

	completed, err := CompleteLoopRun(conn, "page-a", nil, "main result")
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != "completed" || completed.EndedAt == nil ||
		completed.UpdatedAt != *completed.EndedAt || completed.AbortReason != nil {
		t.Fatalf("completed run = %#v", completed)
	}
	assertLoopRunJSON(t, completed, `null`, `null`, `"main result"`, `null`)
}

func TestAbortLoopRunValidatesReasonDefaultsResultAndRejectsEndedRun(t *testing.T) {
	conn := setupLoopRunTest(t)
	run := mustStartLoopRun(t, conn, "page-a", nil)

	if _, err := AbortLoopRun(conn, "page-a", nil, " ", "ignored"); err == nil ||
		!errors.Is(err, ErrInvalidLoopRun) {
		t.Fatalf("blank reason error = %v", err)
	}
	aborted, err := AbortLoopRun(conn, "page-a", nil, " no longer needed ", "")
	if err != nil {
		t.Fatal(err)
	}
	if aborted.ID != run.ID || aborted.Status != "aborted" || aborted.EndedAt == nil ||
		aborted.UpdatedAt != *aborted.EndedAt || aborted.AbortReason == nil ||
		*aborted.AbortReason != "no longer needed" {
		t.Fatalf("aborted run = %#v", aborted)
	}
	assertLoopRunJSON(t, aborted, `null`, `null`, `null`, `null`)

	if _, err := CompleteLoopRun(conn, "page-a", &run.ID, `"late"`); err == nil ||
		!errors.Is(err, ErrInvalidLoopRun) || !strings.Contains(err.Error(), "active") {
		t.Fatalf("second end error = %v", err)
	}
}

func TestDeletePageAbortsActiveMainAndChildrenAndPreservesEndedRuns(t *testing.T) {
	conn := setupLoopRunTest(t)
	main := mustStartLoopRun(t, conn, "page-a", nil)
	child := mustStartLoopRun(t, conn, "page-a", &main.ID)
	ended := insertEndedReviewRunForDelete(t, conn, "page-a")

	if err := DeletePage(conn, "page-a"); err != nil {
		t.Fatal(err)
	}

	if _, err := GetPageByPageID(conn, "page-a"); err == nil {
		t.Fatal("deleted page still exists")
	}
	for _, runID := range []int64{main.ID, child.ID} {
		run, err := GetLoopRun(conn, runID)
		if err != nil {
			t.Fatal(err)
		}
		if run.Status != "aborted" || run.AbortReason == nil || *run.AbortReason != "page_deleted" ||
			run.EndedAt == nil || run.UpdatedAt != *run.EndedAt {
			t.Fatalf("active run after page deletion = %#v", run)
		}
		mustParseLoopRunTime(t, *run.EndedAt)
	}
	gotEnded, err := GetLoopRun(conn, ended.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotEnded.Status != ended.Status || gotEnded.AbortReason != nil ||
		gotEnded.EndedAt == nil || *gotEnded.EndedAt != *ended.EndedAt ||
		gotEnded.UpdatedAt != ended.UpdatedAt {
		t.Fatalf("ended run changed: before = %#v after = %#v", ended, gotEnded)
	}
}

func TestDeletePageRollsBackRunAbortsWhenPageDeleteFails(t *testing.T) {
	conn := setupLoopRunTest(t)
	main := mustStartLoopRun(t, conn, "page-a", nil)
	child := mustStartLoopRun(t, conn, "page-a", &main.ID)
	if _, err := conn.Exec(`
		CREATE TRIGGER reject_page_delete
		BEFORE DELETE ON pages
		BEGIN
			SELECT RAISE(ABORT, 'page delete rejected');
		END
	`); err != nil {
		t.Fatal(err)
	}

	if err := DeletePage(conn, "page-a"); err == nil || !strings.Contains(err.Error(), "page delete rejected") {
		t.Fatalf("DeletePage error = %v", err)
	}
	if _, err := GetPageByPageID(conn, "page-a"); err != nil {
		t.Fatalf("page missing after rollback: %v", err)
	}
	for _, runID := range []int64{main.ID, child.ID} {
		run, err := GetLoopRun(conn, runID)
		if err != nil {
			t.Fatal(err)
		}
		if run.Status != "active" || run.AbortReason != nil || run.EndedAt != nil {
			t.Fatalf("run changed despite rollback: %#v", run)
		}
	}
}

func TestReflectLoopRunOnlyAllowsEndedRunsAndReplacesReflection(t *testing.T) {
	conn := setupLoopRunTest(t)
	run := mustStartLoopRun(t, conn, "page-a", nil)

	if _, err := ReflectLoopRun(conn, run.ID, "too soon"); err == nil ||
		!errors.Is(err, ErrInvalidLoopRun) {
		t.Fatalf("active reflection error = %v", err)
	}
	if _, err := CompleteLoopRun(conn, "page-a", nil, `"done"`); err != nil {
		t.Fatal(err)
	}
	first, err := ReflectLoopRun(conn, run.ID, "first thought")
	if err != nil {
		t.Fatal(err)
	}
	if first.ReflectedAt == nil || first.UpdatedAt != *first.ReflectedAt {
		t.Fatalf("first reflected timestamps = %#v", first)
	}
	mustParseLoopRunTime(t, *first.ReflectedAt)
	second, err := ReflectLoopRun(conn, run.ID, `{"lesson":"second"}`)
	if err != nil {
		t.Fatal(err)
	}
	if second.ReflectedAt == nil || second.UpdatedAt != *second.ReflectedAt {
		t.Fatalf("reflected timestamps = %#v", second)
	}
	mustParseLoopRunTime(t, *second.ReflectedAt)
	assertLoopRunJSON(t, second, `null`, `null`, `"done"`, `{"lesson":"second"}`)
}

func TestListLoopHistoryFiltersOrdersByDescendingIDAndLimits(t *testing.T) {
	conn := setupLoopRunTest(t)
	if _, err := InsertLoopSeed(conn, "other", "normal", "Other", "Other work", 0.5); err != nil {
		t.Fatal(err)
	}
	first := mustStartLoopRun(t, conn, "page-a", nil)
	if _, err := CompleteLoopRun(conn, "page-a", nil, `"first"`); err != nil {
		t.Fatal(err)
	}
	second := mustStartLoopRun(t, conn, "page-b", nil)
	if _, err := CompleteLoopRun(conn, "page-b", nil, `"second"`); err != nil {
		t.Fatal(err)
	}
	if _, err := StartLoopRun(conn, "page-c", "other", nil, "null"); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`
		UPDATE loop_runs SET started_at = CASE id
			WHEN ? THEN '2026-06-10T00:00:00.9Z'
			WHEN ? THEN '2026-06-10T00:00:00.10Z'
		END
		WHERE id IN (?, ?)
	`, first.ID, second.ID, first.ID, second.ID); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 20; i++ {
		all, err := ListLoopHistory(conn, " review ", "", 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(all) != 2 || all[0].ID != second.ID || all[1].ID != first.ID {
			t.Fatalf("history iteration %d = %#v", i, all)
		}
	}
	page, err := ListLoopHistory(conn, "review", " page-a ", 5000)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 1 || page[0].ID != first.ID {
		t.Fatalf("page history = %#v", page)
	}
	limited, err := ListLoopHistory(conn, "review", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 1 || limited[0].ID != second.ID {
		t.Fatalf("limited history = %#v", limited)
	}
	if _, err := ListLoopHistory(conn, " ", "", 1); err == nil || !errors.Is(err, ErrInvalidLoopRun) {
		t.Fatalf("blank seed error = %v", err)
	}
}

func TestListLoopHistoryResolvesArchivedSeedByLoopID(t *testing.T) {
	conn := setupLoopRunTest(t)
	run := mustStartLoopRun(t, conn, "page-a", nil)
	if _, err := CompleteLoopRun(conn, "page-a", nil, `"done"`); err != nil {
		t.Fatal(err)
	}
	if _, err := ArchiveLoopSeed(conn, "review"); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec("UPDATE loop_runs SET seed_name = 'review snapshot' WHERE id = ?", run.ID); err != nil {
		t.Fatal(err)
	}

	history, err := ListLoopHistory(conn, "review", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].ID != run.ID || history[0].SeedName != "review snapshot" {
		t.Fatalf("archived seed history = %#v", history)
	}
}

func TestListLoopHistoryQueryUsesLoopIDIndexWithoutTempSort(t *testing.T) {
	conn := setupLoopRunTest(t)
	run := mustStartLoopRun(t, conn, "page-a", nil)

	rows, err := conn.Query(`
		EXPLAIN QUERY PLAN
		SELECT `+loopRunColumns+`
		FROM loop_runs
		WHERE loop_id = ?
		ORDER BY id DESC
		LIMIT ?
	`, run.LoopID, defaultLoopHistoryLimit)
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
	if !strings.Contains(plan, "idx_loop_runs_loop_started") || strings.Contains(plan, "TEMP B-TREE") {
		t.Fatalf("history query plan = %q", plan)
	}
}

func TestLoopRunConcurrentUpdateAndEndPreservesLifecycleInvariant(t *testing.T) {
	first, second := setupConcurrentLoopRunTest(t)
	run := mustStartLoopRun(t, first, "page-a", nil)
	progress := "concurrent progress"
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, errs[0] = UpdateLoopRun(first, "page-a", &run.ID, nil, &progress)
	}()
	go func() {
		defer wg.Done()
		<-start
		_, errs[1] = CompleteLoopRun(second, "page-a", &run.ID, `"done"`)
	}()
	close(start)
	wg.Wait()

	if errs[1] != nil {
		t.Fatalf("complete error = %v; update error = %v", errs[1], errs[0])
	}
	if errs[0] != nil && !errors.Is(errs[0], ErrInvalidLoopRun) {
		t.Fatalf("update error = %v", errs[0])
	}
	got, err := GetLoopRun(first, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "completed" {
		t.Fatalf("run status = %q", got.Status)
	}
	if errs[0] == nil && string(got.Progress) != `"concurrent progress"` {
		t.Fatalf("successful update was lost: %#v", got)
	}
}

func TestLoopRunConcurrentChildStartAndMainEndPreservesLifecycleInvariant(t *testing.T) {
	first, second := setupConcurrentLoopRunTest(t)
	main := mustStartLoopRun(t, first, "page-a", nil)
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		<-start
		_, err := StartLoopRun(first, "page-a", "review", &main.ID, "null")
		results <- err
	}()
	go func() {
		<-start
		_, err := CompleteLoopRun(second, "page-a", &main.ID, `"done"`)
		results <- err
	}()
	close(start)
	errs := [2]error{<-results, <-results}

	var successes int
	for _, err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrInvalidLoopRun), errors.Is(err, ErrInvalidLoopRunParent):
		default:
			t.Fatalf("unexpected lifecycle error = %v; errors = %v", err, errs)
		}
	}
	if successes != 1 {
		t.Fatalf("successes = %d, errors = %v", successes, errs)
	}

	got, err := GetLoopRun(first, main.ID)
	if err != nil {
		t.Fatal(err)
	}
	children, err := ListActiveChildLoopRuns(first, main.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "active" && len(children) != 0 {
		t.Fatalf("ended main has active children: main = %#v children = %#v", got, children)
	}
}

func setupLoopRunTest(t *testing.T) *sql.DB {
	t.Helper()
	conn := openLoopTestDB(t)
	insertLoopTestPage(t, conn, "page-a")
	insertLoopTestPage(t, conn, "page-b")
	insertLoopTestPage(t, conn, "page-c")
	if _, err := InsertLoopSeed(conn, "review", "normal", "Review memory", "Inspect memories", 0.8); err != nil {
		t.Fatal(err)
	}
	return conn
}

func setupConcurrentLoopRunTest(t *testing.T) (*sql.DB, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "loop.db") + "?_busy_timeout=0"
	first, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	first.SetMaxOpenConns(1)
	t.Cleanup(func() { first.Close() })
	applyLoopTestSchema(t, first)
	insertLoopTestPage(t, first, "page-a")
	insertLoopTestPage(t, first, "page-b")
	if _, err := InsertLoopSeed(first, "review", "normal", "Review memory", "Inspect memories", 0.8); err != nil {
		t.Fatal(err)
	}

	second, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	second.SetMaxOpenConns(1)
	t.Cleanup(func() { second.Close() })
	return first, second
}

func startLoopRunsConcurrently(first, second *sql.DB, firstPage, secondPage string) [2]error {
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, call := range []struct {
		conn   *sql.DB
		pageID string
	}{
		{conn: first, pageID: firstPage},
		{conn: second, pageID: secondPage},
	} {
		go func() {
			<-start
			_, err := StartLoopRun(call.conn, call.pageID, "review", nil, "null")
			results <- err
		}()
	}
	close(start)
	return [2]error{<-results, <-results}
}

func mustStartLoopRun(t *testing.T, conn *sql.DB, pageID string, parentRunID *int64) *LoopRun {
	t.Helper()
	run, err := StartLoopRun(conn, pageID, "review", parentRunID, "null")
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func assertInvalidParentError(t *testing.T, err error, detail string) {
	t.Helper()
	if err == nil || !errors.Is(err, ErrInvalidLoopRunParent) || !strings.Contains(err.Error(), detail) {
		t.Fatalf("parent error = %v, want detail %q", err, detail)
	}
}

func assertLoopRunJSON(t *testing.T, run *LoopRun, plan, progress, result, reflection string) {
	t.Helper()
	for field, value := range map[string]json.RawMessage{
		"plan": run.Plan, "progress": run.Progress, "result": run.Result, "reflection": run.Reflection,
	} {
		if !json.Valid(value) {
			t.Fatalf("%s is invalid JSON: %q", field, value)
		}
	}
	if string(run.Plan) != plan || string(run.Progress) != progress ||
		string(run.Result) != result || string(run.Reflection) != reflection {
		t.Fatalf("JSON fields = plan %s progress %s result %s reflection %s",
			run.Plan, run.Progress, run.Result, run.Reflection)
	}
}

func mustParseLoopRunTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		t.Fatalf("parse loop run timestamp %q: %v", value, err)
	}
	return parsed
}

func insertLoopTestPage(t *testing.T, conn *sql.DB, pageID string) {
	t.Helper()
	now := nowISO()
	if _, err := conn.Exec(`
		INSERT INTO pages (page_id, cwd, context, created_at, updated_at)
		VALUES (?, '.', '{}', ?, ?)
	`, pageID, now, now); err != nil {
		t.Fatal(err)
	}
}

func insertEndedReviewRunForDelete(t *testing.T, conn *sql.DB, pageID string) *LoopRun {
	t.Helper()
	endedAt := "2026-06-09T10:00:00.123456789Z"
	res, err := conn.Exec(`
		INSERT INTO loop_runs (
			loop_id, page_id, seed_name, seed_describe, seed_content, seed_weight,
			status, plan, progress, result, reflection, started_at, ended_at, updated_at
		)
		SELECT id, ?, name, describe, content, weight,
			'completed', 'null', 'null', '"done"', 'null', ?, ?, ?
		FROM loop WHERE name = 'review'
	`, pageID, endedAt, endedAt, endedAt)
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	run, err := GetLoopRun(conn, id)
	if err != nil {
		t.Fatal(err)
	}
	return run
}
