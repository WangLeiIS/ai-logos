package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"logos/safepath"

	_ "github.com/mattn/go-sqlite3"

	"github.com/google/uuid"
)

var ErrPageNotFound = errors.New("page not found")

type Page struct {
	ID        int    `json:"id"`
	PageID    string `json:"page_id"`
	Cwd       string `json:"cwd"`
	Alias     string `json:"alias,omitempty"`
	Context   string `json:"context"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// PageBrief is a lightweight page representation without context,
// used for structured CLI output to encourage agent to call page get.
type PageBrief struct {
	PageID    string `json:"page_id"`
	Cwd       string `json:"cwd"`
	Alias     string `json:"alias,omitempty"`
	CreatedAt string `json:"created_at"`
}

type Memory struct {
	ID         int64   `json:"id"`
	PageID     string  `json:"page_id"`
	Name       string  `json:"name"`
	Question   string  `json:"question"`
	Content    string  `json:"content"`
	Importance float64 `json:"importance"`
	SleepCount int     `json:"sleep_count"`
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`
}

type MemorySummary struct {
	Name       string `json:"name"`
	Question   string `json:"question"`
	ContentLen int    `json:"content_len"`
	SleepCount int    `json:"sleep_count"`
}

type QueryMemoryParams struct {
	Name          string
	Keyword       string
	MinImportance float64
	Since         string
	Before        string
	Limit         int
	Offset        int
}

type Dna struct {
	ID        int     `json:"id"`
	Name      string  `json:"name"`
	Type      string  `json:"type"`
	Question  string  `json:"question"`
	Answer    string  `json:"answer"`
	Weight    float64 `json:"weight"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

func nowISO() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000000000Z")
}

func Open(dbPath string) (*sql.DB, error) {
	path, rawQuery, _ := strings.Cut(dbPath, "?")
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return nil, fmt.Errorf("parse sqlite DSN: %w", err)
	}
	query.Del("_fk")
	query.Set("_foreign_keys", "on")
	return sql.Open("sqlite3", path+"?"+query.Encode())
}

// OpenOuter opens the outer database and attaches the inner database.
// All inner tables (metadata, dna, loop, book, skill, history, template pages/memory)
// are accessed with the inner. prefix in SQL queries.
func OpenOuter(outerPath, innerPath string) (*sql.DB, error) {
	conn, err := Open(outerPath)
	if err != nil {
		return nil, err
	}
	_, err = conn.Exec("ATTACH DATABASE ? AS inner", innerPath)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("attach inner: %w", err)
	}
	return conn, nil
}

func InsertPage(db *sql.DB, cwd string) (*Page, error) {
	var templateContext string
	db.QueryRow("SELECT context FROM inner.pages WHERE page_id = '0'").Scan(&templateContext)

	pageID := uuid.New().String()
	now := nowISO()

	res, err := db.Exec(
		"INSERT INTO pages (page_id, cwd, context, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		pageID, cwd, templateContext, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert page: %w", err)
	}

	id, _ := res.LastInsertId()
	return &Page{
		ID:        int(id),
		PageID:    pageID,
		Cwd:       cwd,
		Context:   templateContext,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func ListPagesByCwd(db *sql.DB, cwd string) ([]Page, error) {
	rows, err := db.Query(
		"SELECT id, page_id, cwd, COALESCE(alias,''), context, created_at, updated_at FROM pages WHERE cwd = ?",
		cwd,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Page
	for rows.Next() {
		var p Page
		if err := rows.Scan(&p.ID, &p.PageID, &p.Cwd, &p.Alias, &p.Context, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, nil
}

func GetPageByPageID(db *sql.DB, pageID string) (*Page, error) {
	var p Page
	err := db.QueryRow(
		"SELECT id, page_id, cwd, COALESCE(alias,''), context, created_at, updated_at FROM pages WHERE page_id = ?",
		pageID,
	).Scan(&p.ID, &p.PageID, &p.Cwd, &p.Alias, &p.Context, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, pageNotFound(pageID)
	}
	if err != nil {
		return nil, fmt.Errorf("get page %q: %w", pageID, err)
	}
	return &p, nil
}

func UpdatePageContext(db *sql.DB, pageID string, context string) (*Page, error) {
	now := nowISO()
	_, err := db.Exec(
		"UPDATE pages SET context = ?, updated_at = ? WHERE page_id = ?",
		context, now, pageID,
	)
	if err != nil {
		return nil, fmt.Errorf("update page: %w", err)
	}
	return GetPageByPageID(db, pageID)
}

// UpdatePageAlias sets the alias for a page in the iroll's pages table.
func UpdatePageAlias(db *sql.DB, pageID, alias string) error {
	var err error
	if alias == "" {
		_, err = db.Exec("UPDATE pages SET alias = NULL WHERE page_id = ?", pageID)
	} else {
		_, err = db.Exec("UPDATE pages SET alias = ? WHERE page_id = ?", alias, pageID)
	}
	return err
}

func DeletePage(db *sql.DB, pageID string) error {
	delay := time.Millisecond
	for attempt := 0; ; attempt++ {
		err := deletePageOnce(db, pageID)
		if !isSQLiteBusyOrLocked(err) || attempt == loopRunMaxRetries {
			return err
		}
		time.Sleep(delay)
		delay *= 2
	}
}

func deletePageOnce(db *sql.DB, pageID string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin deleting page %q: %w", pageID, err)
	}
	defer tx.Rollback()

	now := nowISO()
	if err := abortActiveLoopRunsForPage(tx, pageID, "page_deleted", now); err != nil {
		return err
	}
	res, err := tx.Exec("DELETE FROM pages WHERE page_id = ?", pageID)
	if err != nil {
		return fmt.Errorf("delete page: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("get deleted page count: %w", err)
	}
	if n == 0 {
		return pageNotFound(pageID)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit deleting page %q: %w", pageID, err)
	}
	return nil
}

func pageNotFound(pageID string) error {
	return fmt.Errorf("page %q not found: %w", pageID, ErrPageNotFound)
}

// ResolveContext parses a raw context JSON string and resolves @file and @sql references.
// irollPath is the root directory of the iroll package (e.g. ~/.iroll/my-agent/).
// db is the opened database connection for SQL queries (outer with inner attached, or inner standalone).
func ResolveContext(rawContext string, irollPath string, db *sql.DB, pageID string) (string, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(rawContext), &raw); err != nil {
		// Not valid JSON, return as-is
		return rawContext, nil
	}
	if raw == nil {
		return rawContext, nil
	}

	resolved := make(map[string]interface{}, len(raw))
	for k, v := range raw {
		value, err := resolveValue(v, irollPath, db)
		if err != nil {
			return "", err
		}
		resolved[k] = value
	}

	focus, err := ListActiveRuns(db, pageID)
	if err != nil {
		return "", err
	}
	if focus == nil {
		focus = []LoopRun{}
	}
	resolved["loop_focus"] = focus

	allSeeds, err := ListAvailableLoopSeeds(db)
	if err != nil {
		return "", err
	}
	available := make([]AvailableLoopSeed, 0)
	for _, s := range allSeeds {
		if s.Type == "normal" {
			available = append(available, s)
		}
	}
	resolved["loop_available"] = available

	out, err := json.Marshal(resolved)
	if err != nil {
		return rawContext, nil
	}
	return string(out), nil
}

func resolveValue(v interface{}, irollPath string, db *sql.DB) (interface{}, error) {
	obj, ok := v.(map[string]interface{})
	if !ok {
		return v, nil
	}

	if filePath, exists := obj["@file"].(string); exists {
		absPath, err := safepath.Join(irollPath, filePath)
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(absPath)
		if err != nil {
			return fmt.Sprintf("[file not found: %s]", filePath), nil
		}
		return string(data), nil
	}

	if query, exists := obj["@sql"].(string); exists {
		return resolveSQL(db, query), nil
	}

	return v, nil
}

func resolveSQL(db *sql.DB, query string) interface{} {
	// Only allow SELECT queries to prevent data modification via @sql references.
	upper := strings.ToUpper(strings.TrimSpace(query))
	if !strings.HasPrefix(upper, "SELECT") {
		return "[sql error: only SELECT queries are allowed]"
	}
	rows, err := db.Query(query)
	if err != nil {
		return fmt.Sprintf("[sql error: %s]", err.Error())
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return fmt.Sprintf("[sql error: %s]", err.Error())
	}

	if len(columns) == 1 {
		var results []string
		for rows.Next() {
			var val string
			if err := rows.Scan(&val); err != nil {
				return fmt.Sprintf("[sql error: %s]", err.Error())
			}
			results = append(results, val)
		}
		if len(results) == 1 {
			return results[0]
		}
		return results
	}

	var results []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return fmt.Sprintf("[sql error: %s]", err.Error())
		}
		row := make(map[string]interface{}, len(columns))
		for i, col := range columns {
			row[col] = values[i]
		}
		results = append(results, row)
	}
	return results
}

func QueryDna(db *sql.DB, name string, dnaType string) ([]Dna, error) {
	query := "SELECT id, name, type, question, answer, weight, created_at, updated_at FROM inner.dna WHERE name LIKE ?"
	args := []interface{}{"%" + name + "%"}
	if dnaType != "" {
		query += " AND type = ?"
		args = append(args, dnaType)
	}
	query += " ORDER BY weight DESC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query dna: %w", err)
	}
	defer rows.Close()

	var result []Dna
	for rows.Next() {
		var d Dna
		if err := rows.Scan(&d.ID, &d.Name, &d.Type, &d.Question, &d.Answer, &d.Weight, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("query dna: %w", err)
		}
		result = append(result, d)
	}
	return result, nil
}
