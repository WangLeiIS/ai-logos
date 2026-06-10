package db

import (
	"database/sql"
	"fmt"
	"strings"
)

func InsertMemory(db *sql.DB, pageID, name, question, content string, importance float64) (*Memory, error) {
	name = strings.TrimSpace(name)
	question = strings.TrimSpace(question)
	content = strings.TrimSpace(content)
	if name == "" || question == "" || content == "" {
		return nil, fmt.Errorf("insert memory: name, question, content must not be blank")
	}
	if importance < 0 || importance > 1 {
		return nil, fmt.Errorf("insert memory: importance must be 0.0-1.0")
	}
	if strings.TrimSpace(pageID) == "" {
		return nil, fmt.Errorf("insert memory: page_id must not be blank")
	}

	now := nowISO()
	result, err := db.Exec(`
		INSERT INTO memory (page_id, name, question, content, importance, sleep_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 0, ?, ?)
	`, pageID, name, question, content, importance, now, now)
	if err != nil {
		return nil, fmt.Errorf("insert memory: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("insert memory: %w", err)
	}
	return &Memory{
		ID:         id,
		PageID:     pageID,
		Name:       name,
		Question:   question,
		Content:    content,
		Importance: importance,
		SleepCount: 0,
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

func QueryMemory(db *sql.DB, pageID string, params QueryMemoryParams) ([]Memory, error) {
	query := "SELECT id, page_id, name, question, content, importance, sleep_count, created_at, updated_at FROM memory WHERE page_id = ?"
	args := []any{pageID}

	if params.Name != "" {
		query += " AND name = ?"
		args = append(args, params.Name)
	}
	if params.Keyword != "" {
		query += " AND (name LIKE ? OR question LIKE ?)"
		kw := "%" + params.Keyword + "%"
		args = append(args, kw, kw)
	}
	if params.MinImportance > 0 {
		query += " AND importance >= ?"
		args = append(args, params.MinImportance)
	}
	if params.Since != "" {
		query += " AND created_at >= ?"
		args = append(args, params.Since)
	}
	if params.Before != "" {
		query += " AND created_at <= ?"
		args = append(args, params.Before)
	}

	query += " ORDER BY importance DESC, created_at DESC"

	if params.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, params.Limit)
	}
	if params.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, params.Offset)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query memory: %w", err)
	}
	defer rows.Close()

	var result []Memory
	for rows.Next() {
		var m Memory
		if err := rows.Scan(&m.ID, &m.PageID, &m.Name, &m.Question, &m.Content, &m.Importance, &m.SleepCount, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("query memory: %w", err)
		}
		result = append(result, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query memory: %w", err)
	}
	return result, nil
}

func IncrementSleepCount(db *sql.DB, memoryID int64) error {
	now := nowISO()
	result, err := db.Exec("UPDATE memory SET sleep_count = sleep_count + 1, updated_at = ? WHERE id = ?", now, memoryID)
	if err != nil {
		return fmt.Errorf("increment sleep count for memory %d: %w", memoryID, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("increment sleep count: memory %d not found", memoryID)
	}
	return nil
}

func UpdateMemoryContent(db *sql.DB, memoryID int64, content string, importance float64) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return fmt.Errorf("update memory content: content must not be blank")
	}
	if importance < 0 || importance > 1 {
		return fmt.Errorf("update memory content: importance must be 0.0-1.0")
	}

	now := nowISO()
	result, err := db.Exec(
		"UPDATE memory SET content = ?, importance = ?, updated_at = ? WHERE id = ?",
		content, importance, now, memoryID,
	)
	if err != nil {
		return fmt.Errorf("update memory content for %d: %w", memoryID, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("update memory content: memory %d not found", memoryID)
	}
	return nil
}
