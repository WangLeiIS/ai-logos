package db

import (
	"database/sql"
	"fmt"

	"logos/skill"
)

const createSkillTableSQL = `
	CREATE TABLE IF NOT EXISTS skill (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		description TEXT NOT NULL,
		path TEXT NOT NULL,
		weight REAL NOT NULL DEFAULT 0.5,
		archived_at TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)
`

func EnsureSkillTable(conn *sql.DB) error {
	if _, err := conn.Exec(createSkillTableSQL); err != nil {
		return fmt.Errorf("ensure skill table: %w", err)
	}
	return nil
}

func SyncSkills(conn *sql.DB, skills []skill.ValidatedSkill) (err error) {
	tx, err := conn.Begin()
	if err != nil {
		return fmt.Errorf("begin skill sync: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.Exec(createSkillTableSQL); err != nil {
		return fmt.Errorf("ensure skill table: %w", err)
	}

	present := make(map[string]struct{}, len(skills))
	for _, s := range skills {
		now := nowISO()
		skillPath := s.ResourcePath + "/skill.md"
		if _, err = tx.Exec(`
			INSERT INTO skill (name, description, path, weight, created_at, updated_at)
			VALUES (?, ?, ?, 0.5, ?, ?)
			ON CONFLICT(name) DO UPDATE SET
				description = excluded.description,
				path = excluded.path,
				updated_at = excluded.updated_at
		`, s.Name, s.Description, skillPath, now, now); err != nil {
			return fmt.Errorf("upsert skill %q: %w", s.Name, err)
		}
		present[s.Name] = struct{}{}
	}

	rows, err := tx.Query("SELECT name FROM skill")
	if err != nil {
		return fmt.Errorf("list registered skills during sync: %w", err)
	}
	var stale []string
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			rows.Close()
			return fmt.Errorf("scan registered skill during sync: %w", err)
		}
		if _, exists := present[name]; !exists {
			stale = append(stale, name)
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("list registered skills during sync: %w", err)
	}
	rows.Close()

	for _, name := range stale {
		if _, err = tx.Exec("DELETE FROM skill WHERE name = ?", name); err != nil {
			return fmt.Errorf("delete stale skill %q: %w", name, err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit skill sync: %w", err)
	}
	return nil
}

func ListSkills(conn *sql.DB) ([]skill.Skill, error) {
	rows, err := conn.Query(`
		SELECT id, name, description, path, weight, archived_at, created_at, updated_at
		FROM skill
		WHERE archived_at IS NULL
		ORDER BY weight DESC, name
	`)
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}
	defer rows.Close()

	var result []skill.Skill
	for rows.Next() {
		var s skill.Skill
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.Path, &s.Weight, &s.ArchivedAt, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("list skills: %w", err)
		}
		result = append(result, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}
	return result, nil
}

func GetSkill(conn *sql.DB, name string) (*skill.Skill, error) {
	var s skill.Skill
	err := conn.QueryRow(`
		SELECT id, name, description, path, weight, archived_at, created_at, updated_at
		FROM skill
		WHERE name = ?
	`, name).Scan(&s.ID, &s.Name, &s.Description, &s.Path, &s.Weight, &s.ArchivedAt, &s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("skill %q not found", name)
	}
	if err != nil {
		return nil, fmt.Errorf("get skill %q: %w", name, err)
	}
	return &s, nil
}
