package db

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"logos/book"
)

const createBookTableSQL = `
	CREATE TABLE IF NOT EXISTS book (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		book_id TEXT NOT NULL UNIQUE,
		title TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		resource_path TEXT NOT NULL,
		format_version INTEGER NOT NULL,
		authors TEXT NOT NULL DEFAULT '[]',
		language TEXT NOT NULL DEFAULT '',
		tags TEXT NOT NULL DEFAULT '[]',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)
`

func EnsureBookTable(conn *sql.DB) error {
	if _, err := conn.Exec(createBookTableSQL); err != nil {
		return fmt.Errorf("ensure book table: %w", err)
	}
	return nil
}

func SyncBooks(conn *sql.DB, bundles []book.Bundle) (err error) {
	tx, err := conn.Begin()
	if err != nil {
		return fmt.Errorf("begin book sync: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.Exec(createBookTableSQL); err != nil {
		return fmt.Errorf("ensure book table: %w", err)
	}

	present := make(map[string]struct{}, len(bundles))
	for _, bundle := range bundles {
		manifest := bundle.Manifest
		authors, marshalErr := marshalStringArray(manifest.Authors)
		if marshalErr != nil {
			return fmt.Errorf("marshal authors for book %q: %w", manifest.BookID, marshalErr)
		}
		tags, marshalErr := marshalStringArray(manifest.Tags)
		if marshalErr != nil {
			return fmt.Errorf("marshal tags for book %q: %w", manifest.BookID, marshalErr)
		}
		now := nowISO()
		if _, err = tx.Exec(`
			INSERT INTO book (
				book_id, title, description, resource_path, format_version,
				authors, language, tags, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(book_id) DO UPDATE SET
				title = excluded.title,
				description = excluded.description,
				resource_path = excluded.resource_path,
				format_version = excluded.format_version,
				authors = excluded.authors,
				language = excluded.language,
				tags = excluded.tags,
				updated_at = excluded.updated_at
		`, manifest.BookID, manifest.Title, manifest.Description, bundle.ResourcePath,
			manifest.FormatVersion, authors, manifest.Language, tags, now, now); err != nil {
			return fmt.Errorf("upsert book %q: %w", manifest.BookID, err)
		}
		present[manifest.BookID] = struct{}{}
	}

	rows, err := tx.Query("SELECT book_id FROM book ORDER BY book_id")
	if err != nil {
		return fmt.Errorf("list registered books during sync: %w", err)
	}
	var stale []string
	for rows.Next() {
		var bookID string
		if err = rows.Scan(&bookID); err != nil {
			rows.Close()
			return fmt.Errorf("scan registered book during sync: %w", err)
		}
		if _, exists := present[bookID]; !exists {
			stale = append(stale, bookID)
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("list registered books during sync: %w", err)
	}
	rows.Close()

	for _, bookID := range stale {
		if _, err = tx.Exec("DELETE FROM book WHERE book_id = ?", bookID); err != nil {
			return fmt.Errorf("delete stale book %q: %w", bookID, err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit book sync: %w", err)
	}
	return nil
}

func ListBooks(conn *sql.DB) ([]book.Book, error) {
	rows, err := conn.Query(`
		SELECT book_id, title, description, resource_path, format_version,
		       authors, language, tags, created_at, updated_at
		FROM book
		ORDER BY book_id
	`)
	if err != nil {
		return nil, fmt.Errorf("list books: %w", err)
	}
	defer rows.Close()

	books := make([]book.Book, 0)
	for rows.Next() {
		item, err := scanBook(rows)
		if err != nil {
			return nil, err
		}
		books = append(books, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list books: %w", err)
	}
	return books, nil
}

func GetBook(conn *sql.DB, bookID string) (*book.Book, error) {
	item, err := scanBook(conn.QueryRow(`
		SELECT book_id, title, description, resource_path, format_version,
		       authors, language, tags, created_at, updated_at
		FROM book
		WHERE book_id = ?
	`, bookID))
	if err != nil {
		return nil, fmt.Errorf("get book %q: %w", bookID, err)
	}
	return item, nil
}

type bookScanner interface {
	Scan(dest ...any) error
}

func scanBook(scanner bookScanner) (*book.Book, error) {
	var item book.Book
	var authorsJSON, tagsJSON string
	if err := scanner.Scan(
		&item.BookID, &item.Title, &item.Description, &item.ResourcePath,
		&item.FormatVersion, &authorsJSON, &item.Language, &tagsJSON,
		&item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(authorsJSON), &item.Authors); err != nil {
		return nil, fmt.Errorf("decode authors for book %q: %w", item.BookID, err)
	}
	if err := json.Unmarshal([]byte(tagsJSON), &item.Tags); err != nil {
		return nil, fmt.Errorf("decode tags for book %q: %w", item.BookID, err)
	}
	return &item, nil
}

func marshalStringArray(values []string) (string, error) {
	if values == nil {
		values = []string{}
	}
	data, err := json.Marshal(values)
	return string(data), err
}
