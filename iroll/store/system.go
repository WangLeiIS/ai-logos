package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	rolldb "logos/db"

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
			iroll_version TEXT NOT NULL DEFAULT 'latest',
			page_id TEXT NOT NULL,
			cwd TEXT NOT NULL,
			created_at TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS active_page (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			cwd TEXT NOT NULL UNIQUE,
			iroll_name TEXT NOT NULL,
			iroll_version TEXT NOT NULL DEFAULT 'latest',
			page_id TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS config (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)
	`)
	if err != nil {
		return err
	}

	// Migration: add outer_db_path to page_index
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('page_index') WHERE name = 'outer_db_path'").Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		if _, err := db.Exec("ALTER TABLE page_index ADD COLUMN outer_db_path TEXT NOT NULL DEFAULT ''"); err != nil {
			return err
		}
	}

	// Migration: add outer_db_path to active_page
	if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('active_page') WHERE name = 'outer_db_path'").Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		if _, err := db.Exec("ALTER TABLE active_page ADD COLUMN outer_db_path TEXT NOT NULL DEFAULT ''"); err != nil {
			return err
		}
	}

	// Migration: add alias to page_index
	if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('page_index') WHERE name = 'alias'").Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		if _, err := db.Exec("ALTER TABLE page_index ADD COLUMN alias TEXT"); err != nil {
			return err
		}
	}

	return nil
}

// IndexPage adds a page to the global index and sets it as active for the cwd
func IndexPage(irollName string, version string, pageID string, cwd string, outerDbPath string, alias string) error {
	db, err := OpenSystem()
	if err != nil {
		return err
	}
	defer db.Close()

	now := time.Now().UTC().Format(time.RFC3339Nano)

	_, err = db.Exec(
		"INSERT INTO page_index (iroll_name, iroll_version, page_id, cwd, outer_db_path, alias, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		irollName, version, pageID, cwd, outerDbPath, alias, now,
	)
	if err != nil {
		return fmt.Errorf("index page: %w", err)
	}

	// Upsert active page for this cwd
	_, err = db.Exec(`
		INSERT INTO active_page (cwd, iroll_name, iroll_version, page_id, outer_db_path, updated_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(cwd) DO UPDATE SET iroll_name=excluded.iroll_name, iroll_version=excluded.iroll_version, page_id=excluded.page_id, outer_db_path=excluded.outer_db_path, updated_at=excluded.updated_at
	`, cwd, irollName, version, pageID, outerDbPath, now)
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
		SELECT p.iroll_name, p.iroll_version, p.page_id, p.cwd, p.outer_db_path, p.created_at,
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
		var name, version, pid, c, odbp, t string
		var active int
		if err := rows.Scan(&name, &version, &pid, &c, &odbp, &t, &active); err != nil {
			return nil, err
		}
		result = append(result, map[string]interface{}{
			"iroll":         name + ":" + version,
			"page_id":       pid,
			"cwd":           c,
			"outer_db_path": odbp,
			"created_at":    t,
			"active":        active == 1,
		})
	}
	return result, nil
}

// GetActive returns the active page for a given cwd (iroll_name, iroll_version, page_id, outer_db_path)
func GetActive(cwd string) (string, string, string, string, error) {
	db, err := OpenSystem()
	if err != nil {
		return "", "", "", "", err
	}
	defer db.Close()

	var name, version, pid, outerDbPath string
	err = db.QueryRow("SELECT iroll_name, iroll_version, page_id, outer_db_path FROM active_page WHERE cwd = ?", cwd).Scan(&name, &version, &pid, &outerDbPath)
	if err == sql.ErrNoRows {
		return "", "", "", "", fmt.Errorf("no active page for cwd '%s', run 'logos page new <name>' first", cwd)
	}
	return name, version, pid, outerDbPath, err
}

// DeletePage removes a page from the index, clears active if matching
func DeletePage(pageID string) error {
	sdb, err := OpenSystem()
	if err != nil {
		return err
	}
	defer sdb.Close()

	// Check it exists
	var irollName, irollVersion, outerDbPath string
	err = sdb.QueryRow("SELECT iroll_name, iroll_version, outer_db_path FROM page_index WHERE page_id = ?", pageID).Scan(&irollName, &irollVersion, &outerDbPath)
	if err == sql.ErrNoRows {
		return fmt.Errorf("page '%s' not found in index", pageID)
	}
	if err != nil {
		return err
	}

	innerPath, err := InnerDbPath(irollName, irollVersion)
	if err != nil {
		return err
	}
	var conn *sql.DB
	if outerDbPath == "" {
		// Fall back to opening inner DB directly for legacy entries without outer path
		conn, err = rolldb.Open(innerPath)
	} else {
		conn, err = rolldb.OpenOuter(outerDbPath, innerPath)
	}
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := rolldb.DeletePage(conn, pageID); err != nil && !errors.Is(err, rolldb.ErrPageNotFound) {
		return err
	}

	tx, err := sdb.Begin()
	if err != nil {
		return fmt.Errorf("begin deleting page %q from index: %w", pageID, err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM page_index WHERE page_id = ?", pageID); err != nil {
		return fmt.Errorf("delete page %q from index: %w", pageID, err)
	}
	if _, err := tx.Exec("DELETE FROM active_page WHERE page_id = ?", pageID); err != nil {
		return fmt.Errorf("clear active page %q: %w", pageID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit deleting page %q from index: %w", pageID, err)
	}
	return nil
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
func SwitchPage(pageID string) (string, string, error) {
	db, err := OpenSystem()
	if err != nil {
		return "", "", err
	}
	defer db.Close()

	var irollName, irollVersion, cwd, outerDbPath string
	err = db.QueryRow("SELECT iroll_name, iroll_version, cwd, outer_db_path FROM page_index WHERE page_id = ?", pageID).Scan(&irollName, &irollVersion, &cwd, &outerDbPath)
	if err == sql.ErrNoRows {
		return "", "", fmt.Errorf("page '%s' not found in index", pageID)
	}
	if err != nil {
		return "", "", err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = db.Exec(`
		INSERT INTO active_page (cwd, iroll_name, iroll_version, page_id, outer_db_path, updated_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(cwd) DO UPDATE SET iroll_name=excluded.iroll_name, iroll_version=excluded.iroll_version, page_id=excluded.page_id, outer_db_path=excluded.outer_db_path, updated_at=excluded.updated_at
	`, cwd, irollName, irollVersion, pageID, outerDbPath, now)
	if err != nil {
		return "", "", err
	}
	return irollName, irollVersion, nil
}

// SetDefaultPage sets the default page_id for an iroll name in the config table.
func SetDefaultPage(name, pageID string) error {
	db, err := OpenSystem()
	if err != nil {
		return err
	}
	defer db.Close()

	key := "default_page:" + name
	_, err = db.Exec(
		"INSERT INTO config (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
		key, pageID,
	)
	return err
}

// GetDefaultPage returns the default page_id for an iroll name.
// Returns empty string if no default is set.
func GetDefaultPage(name string) (string, error) {
	db, err := OpenSystem()
	if err != nil {
		return "", err
	}
	defer db.Close()

	key := "default_page:" + name
	var value string
	err = db.QueryRow("SELECT value FROM config WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// ClearDefaultPage removes the default page for an iroll name.
func ClearDefaultPage(name string) error {
	db, err := OpenSystem()
	if err != nil {
		return err
	}
	defer db.Close()

	key := "default_page:" + name
	_, err = db.Exec("DELETE FROM config WHERE key = ?", key)
	return err
}

// LookupPageByAlias looks up a page by its alias from page_index.
func LookupPageByAlias(alias string) (name, version, pageID, outerDbPath string, err error) {
	db, err := OpenSystem()
	if err != nil {
		return "", "", "", "", err
	}
	defer db.Close()

	err = db.QueryRow(
		"SELECT iroll_name, iroll_version, page_id, outer_db_path FROM page_index WHERE alias = ?",
		alias,
	).Scan(&name, &version, &pageID, &outerDbPath)
	if err == sql.ErrNoRows {
		return "", "", "", "", fmt.Errorf("no page found with alias '%s'", alias)
	}
	return name, version, pageID, outerDbPath, err
}

// LookupPageByID looks up a page by page_id from page_index.
func LookupPageByID(pageID string) (name, version, outerDbPath string, err error) {
	db, err := OpenSystem()
	if err != nil {
		return "", "", "", err
	}
	defer db.Close()

	err = db.QueryRow(
		"SELECT iroll_name, iroll_version, outer_db_path FROM page_index WHERE page_id = ?",
		pageID,
	).Scan(&name, &version, &outerDbPath)
	if err == sql.ErrNoRows {
		return "", "", "", fmt.Errorf("page '%s' not found in index", pageID)
	}
	return name, version, outerDbPath, err
}

// SetPageAlias sets the alias for a page in page_index.
func SetPageAlias(pageID, alias string) error {
	db, err := OpenSystem()
	if err != nil {
		return err
	}
	defer db.Close()

	// Clear alias on empty
	if alias == "" {
		_, err = db.Exec("UPDATE page_index SET alias = NULL WHERE page_id = ?", pageID)
	} else {
		_, err = db.Exec("UPDATE page_index SET alias = ? WHERE page_id = ?", alias, pageID)
	}
	return err
}
