package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func SystemDbPath() string {
	return filepath.Join(HomeDir(), "system.db")
}

func OpenSystem() (*sql.DB, error) {
	os.MkdirAll(HomeDir(), 0755)
	db, err := sql.Open("sqlite3", SystemDbPath())
	if err != nil {
		return nil, err
	}
	if err := ensureSystemTables(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func ensureSystemTables(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS page_index (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			iroll_name TEXT NOT NULL,
			page_id TEXT NOT NULL,
			cwd TEXT NOT NULL,
			created_at TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS active_page (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			cwd TEXT NOT NULL UNIQUE,
			iroll_name TEXT NOT NULL,
			page_id TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS config (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)
	`)
	return err
}

// IndexPage adds a page to the global index and sets it as active for the cwd
func IndexPage(irollName string, pageID string, cwd string) error {
	db, err := OpenSystem()
	if err != nil {
		return err
	}
	defer db.Close()

	now := time.Now().UTC().Format(time.RFC3339Nano)

	_, err = db.Exec(
		"INSERT INTO page_index (iroll_name, page_id, cwd, created_at) VALUES (?, ?, ?, ?)",
		irollName, pageID, cwd, now,
	)
	if err != nil {
		return fmt.Errorf("index page: %w", err)
	}

	// Upsert active page for this cwd
	_, err = db.Exec(`
		INSERT INTO active_page (cwd, iroll_name, page_id, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(cwd) DO UPDATE SET iroll_name=excluded.iroll_name, page_id=excluded.page_id, updated_at=excluded.updated_at
	`, cwd, irollName, pageID, now)
	return err
}

// ListAllPages returns pages from the global index, optionally filtered by cwd.
// Each page includes an "active" boolean.
func ListAllPages(cwd string) ([]map[string]interface{}, error) {
	db, err := OpenSystem()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	query := `
		SELECT p.iroll_name, p.page_id, p.cwd, p.created_at,
			CASE WHEN a.page_id IS NOT NULL THEN 1 ELSE 0 END AS active
		FROM page_index p
		LEFT JOIN active_page a ON p.cwd = a.cwd AND p.page_id = a.page_id
	`
	var rows *sql.Rows
	if cwd != "" {
		query += " WHERE p.cwd = ? ORDER BY p.created_at DESC"
		rows, err = db.Query(query, cwd)
	} else {
		query += " ORDER BY p.created_at DESC"
		rows, err = db.Query(query)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var name, pid, c, t string
		var active int
		if err := rows.Scan(&name, &pid, &c, &t, &active); err != nil {
			return nil, err
		}
		result = append(result, map[string]interface{}{
			"iroll_name": name,
			"page_id":    pid,
			"cwd":        c,
			"created_at": t,
			"active":     active == 1,
		})
	}
	return result, nil
}

// GetActive returns the active page for a given cwd (iroll_name, page_id)
func GetActive(cwd string) (string, string, error) {
	db, err := OpenSystem()
	if err != nil {
		return "", "", err
	}
	defer db.Close()

	var name, pid string
	err = db.QueryRow("SELECT iroll_name, page_id FROM active_page WHERE cwd = ?", cwd).Scan(&name, &pid)
	if err == sql.ErrNoRows {
		return "", "", fmt.Errorf("no active page for cwd '%s', run 'logos page new <name>' first", cwd)
	}
	return name, pid, err
}

// DeletePage removes a page from the index, clears active if matching
func DeletePage(pageID string) error {
	sdb, err := OpenSystem()
	if err != nil {
		return err
	}
	defer sdb.Close()

	// Check it exists
	var irollName string
	err = sdb.QueryRow("SELECT iroll_name FROM page_index WHERE page_id = ?", pageID).Scan(&irollName)
	if err == sql.ErrNoRows {
		return fmt.Errorf("page '%s' not found in index", pageID)
	}
	if err != nil {
		return err
	}

	// Delete from index
	if _, err := sdb.Exec("DELETE FROM page_index WHERE page_id = ?", pageID); err != nil {
		return err
	}

	// Clear active if it matches
	sdb.Exec("DELETE FROM active_page WHERE page_id = ?", pageID)

	// Delete from iroll's pages table
	dbPath, err := DbPath(irollName)
	if err != nil {
		return err
	}
	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = conn.Exec("DELETE FROM pages WHERE page_id = ?", pageID)
	return err
}

// CleanIndex removes all page_index and active_page entries for a given iroll name
func CleanIndex(irollName string) {
	db, err := OpenSystem()
	if err != nil {
		return
	}
	defer db.Close()

	db.Exec("DELETE FROM active_page WHERE iroll_name = ?", irollName)
	db.Exec("DELETE FROM page_index WHERE iroll_name = ?", irollName)
}

// SwitchPage sets an existing page as active for its cwd
func SwitchPage(pageID string) (string, error) {
	db, err := OpenSystem()
	if err != nil {
		return "", err
	}
	defer db.Close()

	var irollName, cwd string
	err = db.QueryRow("SELECT iroll_name, cwd FROM page_index WHERE page_id = ?", pageID).Scan(&irollName, &cwd)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("page '%s' not found in index", pageID)
	}
	if err != nil {
		return "", err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = db.Exec(`
		INSERT INTO active_page (cwd, iroll_name, page_id, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(cwd) DO UPDATE SET iroll_name=excluded.iroll_name, page_id=excluded.page_id, updated_at=excluded.updated_at
	`, cwd, irollName, pageID, now)
	if err != nil {
		return "", err
	}
	return irollName, nil
}
