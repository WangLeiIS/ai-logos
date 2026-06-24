package db

import (
	"testing"
)

func TestSplitSQL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "single statement",
			input: "SELECT 1",
			want:  []string{"SELECT 1"},
		},
		{
			name:  "multiple statements",
			input: "SELECT 1; SELECT 2; SELECT 3",
			want:  []string{"SELECT 1", "SELECT 2", "SELECT 3"},
		},
		{
			name:  "trailing semicolon",
			input: "SELECT 1;",
			want:  []string{"SELECT 1"},
		},
		{
			name:  "leading semicolon",
			input: ";SELECT 1",
			want:  []string{"SELECT 1"},
		},
		{
			name:  "whitespace trimming",
			input: "  SELECT 1  ;  \nSELECT 2\t\n",
			want:  []string{"SELECT 1", "SELECT 2"},
		},
		{
			name:  "empty input",
			input: "",
			want:  nil,
		},
		{
			name:  "only semicolons",
			input: ";; ;",
			want:  nil,
		},
		{
			name:  "statement with semicolons in string literal",
			input: "INSERT INTO t VALUES ('hello;world')",
			want:  []string{"INSERT INTO t VALUES ('hello;world')"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SplitSQL(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("SplitSQL(%q) = %#v (len=%d), want %#v (len=%d)",
					tt.input, got, len(got), tt.want, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("SplitSQL(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestIsQuery(t *testing.T) {
	tests := []struct {
		stmt string
		want bool
	}{
		{"SELECT * FROM t", true},
		{"select * from t", true},
		{"  SELECT 1", true},
		{"PRAGMA table_info('t')", true},
		{"EXPLAIN SELECT 1", true},
		{"explain select 1", true},
		{"WITH cte AS (SELECT 1) SELECT * FROM cte", true},
		{"INSERT INTO t VALUES (1)", false},
		{"UPDATE t SET x=1", false},
		{"DELETE FROM t", false},
		{"CREATE TABLE t (id INT)", false},
		{"ALTER TABLE t ADD COLUMN x INT", false},
		{"DROP TABLE t", false},
	}

	for _, tt := range tests {
		t.Run(tt.stmt, func(t *testing.T) {
			got := isQuery(tt.stmt)
			if got != tt.want {
				t.Fatalf("isQuery(%q) = %v, want %v", tt.stmt, got, tt.want)
			}
		})
	}
}
