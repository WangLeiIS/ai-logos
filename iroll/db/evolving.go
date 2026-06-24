package db

import (
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
