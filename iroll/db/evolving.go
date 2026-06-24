package db

import (
	"database/sql"
	"fmt"
	"strings"
)

// EvolvingResult is the result of executing a single SQL statement.
type EvolvingResult struct {
	Type         string     `json:"type"`          // "rows" or "affected"
	Statement    string     `json:"statement"`
	Columns      []string   `json:"columns,omitempty"`
	Rows         [][]string `json:"rows,omitempty"`
	Count        int        `json:"count"`
	AffectedRows int64      `json:"affected_rows,omitempty"`
}

// SplitSQL splits raw SQL by semicolons, trimming whitespace and skipping empty statements.
// Semicolons inside single-quoted strings are preserved as part of the statement.
func SplitSQL(raw string) []string {
	result := make([]string, 0)
	start := 0
	inQuote := false
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if ch == '\'' {
			inQuote = !inQuote
		} else if ch == ';' && !inQuote {
			trimmed := strings.TrimSpace(raw[start:i])
			if trimmed != "" {
				result = append(result, trimmed)
			}
			start = i + 1
		}
	}
	// Handle the last segment after the final semicolon
	trimmed := strings.TrimSpace(raw[start:])
	if trimmed != "" {
		result = append(result, trimmed)
	}
	return result
}

// isQuery returns true if the statement is a read-only query (SELECT, PRAGMA, EXPLAIN, WITH).
func isQuery(stmt string) bool {
	upper := strings.ToUpper(strings.TrimSpace(stmt))
	return strings.HasPrefix(upper, "SELECT") ||
		strings.HasPrefix(upper, "PRAGMA") ||
		strings.HasPrefix(upper, "EXPLAIN") ||
		strings.HasPrefix(upper, "WITH")
}

// ExecuteOne executes a single SQL statement against the database.
// If dryRun is true, the statement is executed inside a transaction that is rolled back.
func ExecuteOne(db *sql.DB, stmt string, dryRun bool) (EvolvingResult, error) {
	if dryRun {
		tx, err := db.Begin()
		if err != nil {
			return EvolvingResult{}, fmt.Errorf("begin dry-run transaction: %w", err)
		}
		defer tx.Rollback()

		result, err := executeOneConn(tx, stmt)
		if err != nil {
			return EvolvingResult{}, err
		}
		// tx.Rollback() via defer — never committed
		return result, nil
	}

	return executeOneConn(db, stmt)
}

func executeOneConn(exe execer, stmt string) (EvolvingResult, error) {
	if isQuery(stmt) {
		return executeQuery(exe, stmt)
	}
	return executeMutation(exe, stmt)
}

type execer interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	Query(query string, args ...interface{}) (*sql.Rows, error)
}

func executeQuery(exe execer, stmt string) (EvolvingResult, error) {
	rows, err := exe.Query(stmt)
	if err != nil {
		return EvolvingResult{}, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return EvolvingResult{}, fmt.Errorf("columns: %w", err)
	}

	var result [][]string
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return EvolvingResult{}, fmt.Errorf("scan: %w", err)
		}
		row := make([]string, len(columns))
		for i, v := range values {
			if v == nil {
				row[i] = "null"
			} else {
				row[i] = fmt.Sprintf("%v", v)
			}
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return EvolvingResult{}, fmt.Errorf("rows iteration: %w", err)
	}
	if result == nil {
		result = [][]string{}
	}

	return EvolvingResult{
		Type:      "rows",
		Statement: stmt,
		Columns:   columns,
		Rows:      result,
		Count:     len(result),
	}, nil
}

func executeMutation(exe execer, stmt string) (EvolvingResult, error) {
	res, err := exe.Exec(stmt)
	if err != nil {
		return EvolvingResult{}, fmt.Errorf("exec: %w", err)
	}

	affected, _ := res.RowsAffected()

	return EvolvingResult{
		Type:         "affected",
		Statement:    stmt,
		AffectedRows: affected,
	}, nil
}

// ExecuteAll splits raw SQL and executes all statements sequentially.
// Stops at the first error, returning results for all statements that succeeded.
func ExecuteAll(db *sql.DB, rawSQL string, dryRun bool) ([]EvolvingResult, error) {
	statements := SplitSQL(rawSQL)
	if len(statements) == 0 {
		return []EvolvingResult{}, nil
	}

	if dryRun {
		tx, err := db.Begin()
		if err != nil {
			return nil, fmt.Errorf("begin dry-run transaction: %w", err)
		}
		defer tx.Rollback()

		var results []EvolvingResult
		for i, stmt := range statements {
			result, err := executeOneConn(tx, stmt)
			if err != nil {
				return results, fmt.Errorf("statement %d/%d failed: %w", i+1, len(statements), err)
			}
			results = append(results, result)
		}
		return results, nil
	}

	var results []EvolvingResult
	for i, stmt := range statements {
		result, err := ExecuteOne(db, stmt, false)
		if err != nil {
			return results, fmt.Errorf("statement %d/%d failed: %w", i+1, len(statements), err)
		}
		results = append(results, result)
	}
	return results, nil
}
