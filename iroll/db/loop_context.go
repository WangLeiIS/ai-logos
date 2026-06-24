package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

type LoopSeedStats struct {
	Active      int             `json:"active"`
	Completed   int             `json:"completed"`
	Aborted     int             `json:"aborted"`
	LastEndedAt *string         `json:"last_ended_at"`
	LastResult  json.RawMessage `json:"last_result"`
}

type AvailableLoopSeed struct {
	LoopSeed
	Stats LoopSeedStats `json:"stats"`
}

// ListActiveRuns returns all active loop runs for a page, main run first then children.
func ListActiveRuns(conn *sql.DB, pageID string) ([]LoopRun, error) {
	rows, err := conn.Query(`
		SELECT `+loopRunColumns+`
		FROM loop_runs
		WHERE page_id = ? AND status = 'active'
		ORDER BY CASE WHEN parent_run_id IS NULL THEN 0 ELSE 1 END, id
	`, pageID)
	if err != nil {
		return nil, fmt.Errorf("list active runs for page %q: %w", pageID, err)
	}
	defer rows.Close()
	return scanLoopRuns(rows)
}

// ListAllRuns returns all loop runs (any status) for a page, newest first.
func ListAllRuns(conn *sql.DB, pageID string) ([]LoopRun, error) {
	rows, err := conn.Query(`
		SELECT `+loopRunColumns+`
		FROM loop_runs
		WHERE page_id = ?
		ORDER BY id DESC
	`, pageID)
	if err != nil {
		return nil, fmt.Errorf("list all runs for page %q: %w", pageID, err)
	}
	defer rows.Close()
	return scanLoopRuns(rows)
}

func scanLoopRuns(rows *sql.Rows) ([]LoopRun, error) {
	runs := make([]LoopRun, 0)
	for rows.Next() {
		run, err := scanLoopRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, *run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return runs, nil
}

func ListAvailableLoopSeeds(conn *sql.DB) ([]AvailableLoopSeed, error) {
	rows, err := conn.Query(availableLoopSeedsQuery)
	if err != nil {
		return nil, fmt.Errorf("list available loop seeds: %w", err)
	}
	defer rows.Close()

	seeds := make([]AvailableLoopSeed, 0)
	for rows.Next() {
		var seed AvailableLoopSeed
		var archivedAt, lastEndedAt sql.NullString
		var lastResult string
		if err := rows.Scan(
			&seed.ID, &seed.Name, &seed.Describe, &seed.Content, &seed.Weight,
			&archivedAt, &seed.CreatedAt, &seed.UpdatedAt,
			&seed.Stats.Active, &seed.Stats.Completed, &seed.Stats.Aborted,
			&lastEndedAt, &lastResult,
		); err != nil {
			return nil, fmt.Errorf("scan available loop seed: %w", err)
		}
		if archivedAt.Valid {
			seed.ArchivedAt = &archivedAt.String
		}
		if lastEndedAt.Valid {
			seed.Stats.LastEndedAt = &lastEndedAt.String
		}
		if !json.Valid([]byte(lastResult)) {
			return nil, fmt.Errorf("available loop seed %q has invalid last result JSON", seed.Name)
		}
		seed.Stats.LastResult = json.RawMessage(lastResult)
		seeds = append(seeds, seed)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list available loop seeds: %w", err)
	}
	return seeds, nil
}

const availableLoopSeedsQuery = `
	WITH stats AS (
		SELECT
			loop_id,
			SUM(CASE WHEN status = 'active' THEN 1 ELSE 0 END) AS active,
			SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) AS completed,
			SUM(CASE WHEN status = 'aborted' THEN 1 ELSE 0 END) AS aborted
		FROM loop_runs
		GROUP BY loop_id
	)
	SELECT
		loop.id, loop.name, loop.describe, loop.content, loop.weight,
		loop.archived_at, loop.created_at, loop.updated_at,
		COALESCE(stats.active, 0),
		COALESCE(stats.completed, 0),
		COALESCE(stats.aborted, 0),
		latest_ended.ended_at,
		COALESCE(latest_ended.result, 'null')
	FROM loop
	LEFT JOIN stats ON stats.loop_id = loop.id
	LEFT JOIN loop_runs latest_ended ON latest_ended.id = (
		SELECT candidate.id
		FROM loop_runs candidate
		WHERE candidate.loop_id = loop.id
			AND candidate.status IN ('completed', 'aborted')
			AND candidate.ended_at IS NOT NULL
		ORDER BY candidate.ended_at DESC, candidate.id DESC
		LIMIT 1
	)
	WHERE loop.archived_at IS NULL
	ORDER BY loop.weight DESC, loop.name ASC
`
