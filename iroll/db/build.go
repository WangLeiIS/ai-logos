package db

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"logos/safepath"
)

type HistoryEntry struct {
	ID           int    `json:"id"`
	FromLayer    string `json:"from_layer"`
	Description  string `json:"description"`
	LayerID      string `json:"layer_id"`
	Instructions string `json:"instructions"`
	CreatedAt    string `json:"created_at"`
}

func ExecuteSQL(db *sql.DB, sqlPath string) error {
	content, err := ioutil.ReadFile(sqlPath)
	if err != nil {
		return fmt.Errorf("read sql file: %w", err)
	}

	_, err = db.Exec(string(content))
	if err != nil {
		return fmt.Errorf("execute sql %s: %w", filepath.Base(sqlPath), err)
	}
	return nil
}

func EnsureHistoryTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			from_layer TEXT,
			description TEXT NOT NULL,
			layer_id TEXT NOT NULL,
			instructions TEXT,
			created_at TEXT NOT NULL
		)
	`)
	return err
}

func InsertHistory(db *sql.DB, fromLayer string, description string, layerID string, instructions string) error {
	_, err := db.Exec(
		"INSERT INTO history (from_layer, description, layer_id, instructions, created_at) VALUES (?, ?, ?, ?, ?)",
		fromLayer, description, layerID, instructions, time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

func QueryHistory(db *sql.DB) ([]HistoryEntry, error) {
	rows, err := db.Query("SELECT id, from_layer, description, layer_id, instructions, created_at FROM history ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []HistoryEntry
	for rows.Next() {
		var h HistoryEntry
		var fromLayer sql.NullString
		if err := rows.Scan(&h.ID, &fromLayer, &h.Description, &h.LayerID, &h.Instructions, &h.CreatedAt); err != nil {
			return nil, err
		}
		if fromLayer.Valid {
			h.FromLayer = fromLayer.String
		}
		result = append(result, h)
	}
	return result, nil
}

func QueryTableStats(db *sql.DB) (map[string]int, error) {
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := make(map[string]int)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM " + name).Scan(&count); err != nil {
			count = -1
		}
		stats[name] = count
	}
	return stats, nil
}

func QueryAllMetadata(db *sql.DB) (map[string]string, error) {
	rows, err := db.Query("SELECT key, value FROM metadata")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		result[k] = v
	}
	return result, nil
}

func ListResources(name string) ([]string, error) {
	if err := safepath.ValidateName(name); err != nil {
		return nil, err
	}
	home, _ := os.UserHomeDir()
	irollDir, err := safepath.Join(filepath.Join(home, ".iroll"), name)
	if err != nil {
		return nil, err
	}
	resDir, err := safepath.Join(irollDir, "Resources")
	if err != nil {
		return nil, err
	}
	var files []string
	filepath.Walk(resDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(resDir, path)
		files = append(files, rel)
		return nil
	})
	return files, nil
}
