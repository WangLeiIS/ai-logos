package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mattn/go-sqlite3"
)

var (
	ErrLoopRunNotFound         = errors.New("loop run not found")
	ErrInvalidLoopRun          = errors.New("invalid loop run")
	ErrActiveMainLoopRunExists = errors.New("active main loop run already exists")
	ErrInvalidLoopRunParent    = errors.New("invalid loop run parent")
)

func StartLoopRun(conn *sql.DB, pageID, seedName string, parentRunID *int64, plan string) (_ *LoopRun, err error) {
	pageID, err = validateLoopRunText("page ID", pageID)
	if err != nil {
		return nil, err
	}
	seedName, err = validateLoopRunText("seed name", seedName)
	if err != nil {
		return nil, err
	}
	plan, err = NormalizeLoopJSON(plan)
	if err != nil {
		return nil, fmt.Errorf("normalize loop run plan: %w", err)
	}

	delay := time.Millisecond
	for attempt := 0; ; attempt++ {
		run, err := startLoopRunOnce(conn, pageID, seedName, parentRunID, plan)
		if !isSQLiteBusyOrLocked(err) || attempt == loopRunMaxRetries {
			return run, err
		}
		time.Sleep(delay)
		delay *= 2
	}
}

const (
	loopRunMaxRetries       = 8
	defaultLoopHistoryLimit = 50
	maximumLoopHistoryLimit = 200
)

func abortActiveLoopRunsForPage(tx *sql.Tx, pageID, reason, endedAt string) error {
	if _, err := tx.Exec(`
		UPDATE loop_runs
		SET status = 'aborted', abort_reason = ?, ended_at = ?, updated_at = ?
		WHERE page_id = ? AND status = 'active'
	`, reason, endedAt, endedAt, pageID); err != nil {
		return fmt.Errorf("abort active loop runs for page %q: %w", pageID, err)
	}
	return nil
}

func startLoopRunOnce(conn *sql.DB, pageID, seedName string, parentRunID *int64, plan string) (*LoopRun, error) {
	tx, err := conn.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin starting loop run: %w", err)
	}
	defer tx.Rollback()

	if err := requirePageForLoopRun(tx, pageID); err != nil {
		return nil, err
	}
	seed, err := getActiveLoopSeedForRun(tx, seedName)
	if err != nil {
		return nil, err
	}
	if parentRunID == nil {
		var activeMainID int64
		checkErr := tx.QueryRow(`
			SELECT id
			FROM loop_runs
			WHERE page_id = ? AND status = 'active' AND parent_run_id IS NULL
		`, pageID).Scan(&activeMainID)
		if checkErr == nil {
			return nil, activeMainLoopRunExists(pageID)
		}
		if !errors.Is(checkErr, sql.ErrNoRows) {
			return nil, fmt.Errorf("check active main loop run for page %q: %w", pageID, checkErr)
		}
	} else if err = validateLoopRunParent(tx, *parentRunID, pageID); err != nil {
		return nil, err
	}

	now := nowISO()
	run, err := scanLoopRun(tx.QueryRow(`
		INSERT INTO loop_runs (
			loop_id, page_id, parent_run_id,
			seed_name, seed_describe, seed_content, seed_weight,
			status, plan, progress, result, reflection,
			started_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, 'active', ?, 'null', 'null', 'null', ?, ?)
		RETURNING `+loopRunColumns,
		seed.ID, pageID, parentRunID,
		seed.Name, seed.Describe, seed.Content, seed.Weight,
		plan, now, now,
	))
	if err != nil {
		if parentRunID == nil && isUniqueConstraint(err) {
			return nil, activeMainLoopRunExists(pageID)
		}
		return nil, fmt.Errorf("start loop run for page %q with seed %q: %w", pageID, seedName, err)
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit starting loop run for page %q: %w", pageID, err)
	}
	return run, nil
}

func requirePageForLoopRun(tx *sql.Tx, pageID string) error {
	var exists int
	err := tx.QueryRow("SELECT 1 FROM pages WHERE page_id = ? LIMIT 1", pageID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return pageNotFound(pageID)
	}
	if err != nil {
		return fmt.Errorf("validate page %q for loop run: %w", pageID, err)
	}
	return nil
}

func isSQLiteBusyOrLocked(err error) bool {
	var sqliteErr sqlite3.Error
	return errors.As(err, &sqliteErr) && (sqliteErr.Code == sqlite3.ErrBusy || sqliteErr.Code == sqlite3.ErrLocked)
}

func UpdateLoopRun(conn *sql.DB, pageID string, runID *int64, plan, progress *string) (*LoopRun, error) {
	if plan == nil && progress == nil {
		return nil, fmt.Errorf("loop run update: no fields supplied: %w", ErrInvalidLoopRun)
	}
	pageID, err := validateLoopRunText("page ID", pageID)
	if err != nil {
		return nil, err
	}

	fields := make([]string, 0, 3)
	args := make([]any, 0, 4)
	if plan != nil {
		normalized, err := NormalizeLoopJSON(*plan)
		if err != nil {
			return nil, fmt.Errorf("normalize loop run plan: %w", err)
		}
		fields = append(fields, "plan = ?")
		args = append(args, normalized)
	}
	if progress != nil {
		normalized, err := NormalizeLoopJSON(*progress)
		if err != nil {
			return nil, fmt.Errorf("normalize loop run progress: %w", err)
		}
		fields = append(fields, "progress = ?")
		args = append(args, normalized)
	}

	return mutateLoopRunWithRetry(conn, "update", func(tx *sql.Tx) (*LoopRun, error) {
		run, err := resolveRunForMutation(tx, pageID, runID)
		if err != nil {
			return nil, err
		}
		if run.Status != "active" {
			return nil, loopRunMustBeActive(run.ID)
		}
		now := nowISO()
		queryArgs := append(append([]any{}, args...), now, run.ID)
		updated, err := scanLoopRun(tx.QueryRow(
			"UPDATE loop_runs SET "+strings.Join(fields, ", ")+", updated_at = ? WHERE id = ? AND status = 'active' RETURNING "+loopRunColumns,
			queryArgs...,
		))
		if errors.Is(err, sql.ErrNoRows) {
			return nil, loopRunMustBeActive(run.ID)
		}
		if err != nil {
			return nil, fmt.Errorf("update loop run %d: %w", run.ID, err)
		}
		return updated, nil
	})
}

func CompleteLoopRun(conn *sql.DB, pageID string, runID *int64, result string) (*LoopRun, error) {
	if strings.TrimSpace(result) == "" {
		return nil, fmt.Errorf("loop run completion result must not be blank: %w", ErrInvalidLoopRun)
	}
	normalized, err := NormalizeLoopJSON(result)
	if err != nil {
		return nil, fmt.Errorf("normalize loop run result: %w", err)
	}
	return endLoopRun(conn, pageID, runID, "completed", normalized, nil)
}

func AbortLoopRun(conn *sql.DB, pageID string, runID *int64, reason, result string) (*LoopRun, error) {
	reason, err := validateLoopRunText("abort reason", reason)
	if err != nil {
		return nil, err
	}
	normalized := "null"
	if strings.TrimSpace(result) != "" {
		normalized, err = NormalizeLoopJSON(result)
		if err != nil {
			return nil, fmt.Errorf("normalize loop run result: %w", err)
		}
	}
	return endLoopRun(conn, pageID, runID, "aborted", normalized, &reason)
}

func endLoopRun(conn *sql.DB, pageID string, runID *int64, status, result string, abortReason *string) (*LoopRun, error) {
	pageID, err := validateLoopRunText("page ID", pageID)
	if err != nil {
		return nil, err
	}
	return mutateLoopRunWithRetry(conn, status, func(tx *sql.Tx) (*LoopRun, error) {
		run, err := resolveRunForMutation(tx, pageID, runID)
		if err != nil {
			return nil, err
		}
		if run.Status != "active" {
			return nil, loopRunMustBeActive(run.ID)
		}
		if run.ParentRunID == nil {
			var activeChildren int
			if err := tx.QueryRow(
				"SELECT COUNT(*) FROM loop_runs WHERE parent_run_id = ? AND status = 'active'",
				run.ID,
			).Scan(&activeChildren); err != nil {
				return nil, fmt.Errorf("check active child loop runs for run %d: %w", run.ID, err)
			}
			if activeChildren != 0 {
				return nil, fmt.Errorf("main loop run %d has active child runs: %w", run.ID, ErrInvalidLoopRun)
			}
		}

		now := nowISO()
		ended, err := scanLoopRun(tx.QueryRow(`
			UPDATE loop_runs
			SET status = ?, result = ?, abort_reason = ?, ended_at = ?, updated_at = ?
			WHERE id = ? AND status = 'active'
				AND (parent_run_id IS NOT NULL OR NOT EXISTS (
					SELECT 1 FROM loop_runs child
					WHERE child.parent_run_id = loop_runs.id AND child.status = 'active'
				))
			RETURNING `+loopRunColumns,
			status, result, abortReason, now, now, run.ID,
		))
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("loop run %d changed or has active child runs: %w", run.ID, ErrInvalidLoopRun)
		}
		if err != nil {
			return nil, fmt.Errorf("end loop run %d as %s: %w", run.ID, status, err)
		}
		return ended, nil
	})
}

func ReflectLoopRun(conn *sql.DB, runID int64, reflection string) (*LoopRun, error) {
	normalized, err := NormalizeLoopJSON(reflection)
	if err != nil {
		return nil, fmt.Errorf("normalize loop run reflection: %w", err)
	}
	return mutateLoopRunWithRetry(conn, "reflect", func(tx *sql.Tx) (*LoopRun, error) {
		now := nowISO()
		run, err := scanLoopRun(tx.QueryRow(`
			UPDATE loop_runs
			SET reflection = ?, reflected_at = ?, updated_at = ?
			WHERE id = ? AND status != 'active'
			RETURNING `+loopRunColumns,
			normalized, now, now, runID,
		))
		if errors.Is(err, sql.ErrNoRows) {
			var status string
			checkErr := tx.QueryRow("SELECT status FROM loop_runs WHERE id = ?", runID).Scan(&status)
			if errors.Is(checkErr, sql.ErrNoRows) {
				return nil, loopRunNotFound(runID)
			}
			if checkErr != nil {
				return nil, fmt.Errorf("get loop run %d for reflection: %w", runID, checkErr)
			}
			return nil, fmt.Errorf("loop run %d must be ended before reflection: %w", runID, ErrInvalidLoopRun)
		}
		if err != nil {
			return nil, fmt.Errorf("reflect on loop run %d: %w", runID, err)
		}
		return run, nil
	})
}

func ListLoopHistory(conn *sql.DB, seedName, pageID string, limit int) ([]LoopRun, error) {
	seedName, err := validateLoopRunText("seed name", seedName)
	if err != nil {
		return nil, err
	}
	var loopID int64
	if err := conn.QueryRow("SELECT id FROM loop WHERE name = ?", seedName).Scan(&loopID); errors.Is(err, sql.ErrNoRows) {
		return nil, loopSeedNotFound(seedName)
	} else if err != nil {
		return nil, fmt.Errorf("resolve loop seed %q for history: %w", seedName, err)
	}
	if pageID != "" {
		pageID, err = validateLoopRunText("page ID", pageID)
		if err != nil {
			return nil, err
		}
	}
	if limit <= 0 {
		limit = defaultLoopHistoryLimit
	} else if limit > maximumLoopHistoryLimit {
		limit = maximumLoopHistoryLimit
	}

	query := "SELECT " + loopRunColumns + " FROM loop_runs WHERE loop_id = ?"
	args := []any{loopID}
	if pageID != "" {
		query += " AND page_id = ?"
		args = append(args, pageID)
	}
	query += " ORDER BY id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list loop history for seed %q: %w", seedName, err)
	}
	defer rows.Close()
	runs := make([]LoopRun, 0)
	for rows.Next() {
		run, err := scanLoopRun(rows)
		if err != nil {
			return nil, fmt.Errorf("scan loop history for seed %q: %w", seedName, err)
		}
		runs = append(runs, *run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list loop history for seed %q: %w", seedName, err)
	}
	return runs, nil
}

func mutateLoopRunWithRetry(conn *sql.DB, operation string, mutate func(*sql.Tx) (*LoopRun, error)) (*LoopRun, error) {
	delay := time.Millisecond
	for attempt := 0; ; attempt++ {
		run, err := mutateLoopRunOnce(conn, mutate)
		if !isSQLiteBusyOrLocked(err) || attempt == loopRunMaxRetries {
			if err != nil && isSQLiteBusyOrLocked(err) {
				return nil, fmt.Errorf("%s loop run after retries: %w", operation, err)
			}
			return run, err
		}
		time.Sleep(delay)
		delay *= 2
	}
}

func mutateLoopRunOnce(conn *sql.DB, mutate func(*sql.Tx) (*LoopRun, error)) (*LoopRun, error) {
	tx, err := conn.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	run, err := mutate(tx)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return run, nil
}

func resolveRunForMutation(tx *sql.Tx, pageID string, runID *int64) (*LoopRun, error) {
	var (
		run *LoopRun
		err error
	)
	if runID == nil {
		run, err = scanLoopRun(tx.QueryRow(`
			SELECT `+loopRunColumns+`
			FROM loop_runs
			WHERE page_id = ? AND status = 'active' AND parent_run_id IS NULL
		`, pageID))
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("active main loop run for page %q not found: %w", pageID, ErrLoopRunNotFound)
		}
	} else {
		run, err = scanLoopRun(tx.QueryRow(`
			SELECT `+loopRunColumns+`
			FROM loop_runs
			WHERE id = ?
		`, *runID))
		if errors.Is(err, sql.ErrNoRows) {
			return nil, loopRunNotFound(*runID)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("resolve loop run for page %q: %w", pageID, err)
	}
	if run.PageID != pageID {
		return nil, fmt.Errorf("loop run %d does not belong to page %q: %w", run.ID, pageID, ErrInvalidLoopRun)
	}
	return run, nil
}

func GetLoopRun(conn *sql.DB, runID int64) (*LoopRun, error) {
	run, err := scanLoopRun(conn.QueryRow(`
		SELECT `+loopRunColumns+`
		FROM loop_runs
		WHERE id = ?
	`, runID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, loopRunNotFound(runID)
	}
	if err != nil {
		return nil, fmt.Errorf("get loop run %d: %w", runID, err)
	}
	return run, nil
}

func GetActiveMainLoopRun(conn *sql.DB, pageID string) (*LoopRun, error) {
	pageID, err := validateLoopRunText("page ID", pageID)
	if err != nil {
		return nil, err
	}
	run, err := scanLoopRun(conn.QueryRow(`
		SELECT `+loopRunColumns+`
		FROM loop_runs
		WHERE page_id = ? AND status = 'active' AND parent_run_id IS NULL
	`, pageID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("active main loop run for page %q not found: %w", pageID, ErrLoopRunNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get active main loop run for page %q: %w", pageID, err)
	}
	return run, nil
}

func ListActiveChildLoopRuns(conn *sql.DB, mainRunID int64) ([]LoopRun, error) {
	rows, err := conn.Query(`
		SELECT `+loopRunColumns+`
		FROM loop_runs
		WHERE parent_run_id = ? AND status = 'active'
		ORDER BY id
	`, mainRunID)
	if err != nil {
		return nil, fmt.Errorf("list active child loop runs for run %d: %w", mainRunID, err)
	}
	defer rows.Close()

	runs := make([]LoopRun, 0)
	for rows.Next() {
		run, err := scanLoopRun(rows)
		if err != nil {
			return nil, fmt.Errorf("scan active child loop run for run %d: %w", mainRunID, err)
		}
		runs = append(runs, *run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list active child loop runs for run %d: %w", mainRunID, err)
	}
	return runs, nil
}

func getActiveLoopSeedForRun(tx *sql.Tx, name string) (*LoopSeed, error) {
	seed, err := scanLoopSeed(tx.QueryRow(`
		SELECT id, name, describe, content, weight, archived_at, created_at, updated_at
		FROM loop
		WHERE name = ? AND archived_at IS NULL
	`, name))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("loop seed %q not found or not active: %w", name, ErrLoopSeedNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get loop seed %q for run: %w", name, err)
	}
	return seed, nil
}

func validateLoopRunParent(tx *sql.Tx, parentRunID int64, pageID string) error {
	var parentPageID, status string
	var parentParentRunID sql.NullInt64
	err := tx.QueryRow(`
		SELECT page_id, parent_run_id, status
		FROM loop_runs
		WHERE id = ?
	`, parentRunID).Scan(&parentPageID, &parentParentRunID, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("parent loop run %d not found: %w", parentRunID, ErrInvalidLoopRunParent)
	}
	if err != nil {
		return fmt.Errorf("get parent loop run %d: %w", parentRunID, err)
	}
	if status != "active" {
		return fmt.Errorf("parent loop run %d must be active: %w", parentRunID, ErrInvalidLoopRunParent)
	}
	if parentPageID != pageID {
		return fmt.Errorf("parent loop run %d must belong to the same page: %w", parentRunID, ErrInvalidLoopRunParent)
	}
	if parentParentRunID.Valid {
		return fmt.Errorf("parent loop run %d must be a main run: %w", parentRunID, ErrInvalidLoopRunParent)
	}
	return nil
}

func validateLoopRunText(field, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("loop run %s must not be blank: %w", field, ErrInvalidLoopRun)
	}
	return value, nil
}

func activeMainLoopRunExists(pageID string) error {
	return fmt.Errorf("page %q already has an active main loop run: %w", pageID, ErrActiveMainLoopRunExists)
}

func loopRunNotFound(runID int64) error {
	return fmt.Errorf("loop run %d not found: %w", runID, ErrLoopRunNotFound)
}

func loopRunMustBeActive(runID int64) error {
	return fmt.Errorf("loop run %d must be active: %w", runID, ErrInvalidLoopRun)
}

type loopRunScanner interface {
	Scan(dest ...any) error
}

const loopRunColumns = `
	id, loop_id, page_id, parent_run_id,
	seed_name, seed_describe, seed_content, seed_weight,
	status, plan, progress, result, reflection, abort_reason,
	started_at, ended_at, reflected_at, updated_at
`

func scanLoopRun(scanner loopRunScanner) (*LoopRun, error) {
	var run LoopRun
	var parentRunID sql.NullInt64
	var plan, progress, result, reflection string
	var abortReason, endedAt, reflectedAt sql.NullString
	if err := scanner.Scan(
		&run.ID, &run.LoopID, &run.PageID, &parentRunID,
		&run.SeedName, &run.SeedDescribe, &run.SeedContent, &run.SeedWeight,
		&run.Status, &plan, &progress, &result, &reflection, &abortReason,
		&run.StartedAt, &endedAt, &reflectedAt, &run.UpdatedAt,
	); err != nil {
		return nil, err
	}

	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "plan", value: plan},
		{name: "progress", value: progress},
		{name: "result", value: result},
		{name: "reflection", value: reflection},
	} {
		if !json.Valid([]byte(field.value)) {
			return nil, fmt.Errorf("loop run %d has invalid %s JSON: %w", run.ID, field.name, ErrInvalidLoopRun)
		}
	}
	run.Plan = json.RawMessage(plan)
	run.Progress = json.RawMessage(progress)
	run.Result = json.RawMessage(result)
	run.Reflection = json.RawMessage(reflection)
	if parentRunID.Valid {
		run.ParentRunID = &parentRunID.Int64
	}
	if abortReason.Valid {
		run.AbortReason = &abortReason.String
	}
	if endedAt.Valid {
		run.EndedAt = &endedAt.String
	}
	if reflectedAt.Valid {
		run.ReflectedAt = &reflectedAt.String
	}
	return &run, nil
}
