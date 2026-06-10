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

type LoopContextView struct {
	Focus struct {
		Main     *LoopRun  `json:"main"`
		Children []LoopRun `json:"children"`
	} `json:"focus"`
	Available []AvailableLoopSeed `json:"available"`
}

func BuildLoopContext(conn *sql.DB, pageID string) (*LoopContextView, error) {
	view := &LoopContextView{
		Available: make([]AvailableLoopSeed, 0),
	}
	view.Focus.Children = make([]LoopRun, 0)

	runs, err := listActivePageLoopRuns(conn, pageID)
	if err != nil {
		return nil, err
	}
	for i := range runs {
		if runs[i].ParentRunID == nil {
			view.Focus.Main = &runs[i]
			break
		}
	}
	if view.Focus.Main != nil {
		for i := range runs {
			if runs[i].ParentRunID != nil && *runs[i].ParentRunID == view.Focus.Main.ID {
				view.Focus.Children = append(view.Focus.Children, runs[i])
			}
		}
	}

	available, err := listAvailableLoopSeeds(conn)
	if err != nil {
		return nil, err
	}
	view.Available = available
	return view, nil
}

func listActivePageLoopRuns(conn *sql.DB, pageID string) ([]LoopRun, error) {
	rows, err := conn.Query(`
		SELECT `+loopRunColumns+`
		FROM loop_runs
		WHERE page_id = ? AND status = 'active'
		ORDER BY CASE WHEN parent_run_id IS NULL THEN 0 ELSE 1 END, id
	`, pageID)
	if err != nil {
		return nil, fmt.Errorf("list active loop focus for page %q: %w", pageID, err)
	}
	defer rows.Close()

	runs := make([]LoopRun, 0)
	for rows.Next() {
		run, err := scanLoopRun(rows)
		if err != nil {
			return nil, fmt.Errorf("scan active loop focus for page %q: %w", pageID, err)
		}
		runs = append(runs, *run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list active loop focus for page %q: %w", pageID, err)
	}
	return runs, nil
}

func listAvailableLoopSeeds(conn *sql.DB) ([]AvailableLoopSeed, error) {
	rows, err := conn.Query(`
		WITH stats AS (
			SELECT
				loop_id,
				SUM(CASE WHEN status = 'active' THEN 1 ELSE 0 END) AS active,
				SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) AS completed,
				SUM(CASE WHEN status = 'aborted' THEN 1 ELSE 0 END) AS aborted,
				MAX(ended_at) AS last_ended_at
			FROM loop_runs
			GROUP BY loop_id
		),
		latest_ended AS (
			SELECT loop_id, result, ROW_NUMBER() OVER (
				PARTITION BY loop_id ORDER BY ended_at DESC, id DESC
			) AS rank
			FROM loop_runs
			WHERE ended_at IS NOT NULL
		)
		SELECT
			loop.id, loop.name, loop.describe, loop.content, loop.weight,
			loop.archived_at, loop.created_at, loop.updated_at,
			COALESCE(stats.active, 0),
			COALESCE(stats.completed, 0),
			COALESCE(stats.aborted, 0),
			stats.last_ended_at,
			COALESCE(latest_ended.result, 'null')
		FROM loop
		LEFT JOIN stats ON stats.loop_id = loop.id
		LEFT JOIN latest_ended ON latest_ended.loop_id = loop.id AND latest_ended.rank = 1
		WHERE loop.archived_at IS NULL
		ORDER BY loop.weight DESC, loop.name ASC
	`)
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
