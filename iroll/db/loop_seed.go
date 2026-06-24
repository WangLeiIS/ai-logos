package db

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/mattn/go-sqlite3"
)

var (
	ErrLoopSeedNotFound      = errors.New("loop seed not found")
	ErrLoopSeedAlreadyExists = errors.New("loop seed already exists")
	ErrInvalidLoopSeed       = errors.New("invalid loop seed")
)

func InsertLoopSeed(conn *sql.DB, name, loopType, describe, content string, weight float64) (*LoopSeed, error) {
	name, loopType, describe, content, err := validateLoopSeed(name, loopType, describe, content, weight)
	if err != nil {
		return nil, err
	}

	now := nowISO()
	result, err := conn.Exec(`
		INSERT INTO loop (name, type, describe, content, weight, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, name, loopType, describe, content, weight, now, now)
	if err != nil {
		if isUniqueConstraint(err) {
			return nil, fmt.Errorf("loop seed %q already exists: %w", name, ErrLoopSeedAlreadyExists)
		}
		return nil, fmt.Errorf("insert loop seed %q: %w", name, err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("read inserted loop seed %q id: %w", name, err)
	}
	return &LoopSeed{
		ID:        id,
		Name:      name,
		Type:      loopType,
		Describe:  describe,
		Content:   content,
		Weight:    weight,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func UpdateLoopSeed(conn *sql.DB, name string, patch LoopSeedPatch) (*LoopSeed, error) {
	name, err := validateLoopSeedName(name)
	if err != nil {
		return nil, err
	}
	if patch.Type == nil && patch.Describe == nil && patch.Content == nil && patch.Weight == nil {
		return nil, fmt.Errorf("loop seed update: no fields supplied: %w", ErrInvalidLoopSeed)
	}

	fields := make([]string, 0, 5)
	args := make([]any, 0, 6)
	if patch.Describe != nil {
		describe, err := validateLoopSeedText("describe", *patch.Describe)
		if err != nil {
			return nil, err
		}
		fields = append(fields, "describe = ?")
		args = append(args, describe)
	}
	if patch.Content != nil {
		content, err := validateLoopSeedText("content", *patch.Content)
		if err != nil {
			return nil, err
		}
		fields = append(fields, "content = ?")
		args = append(args, content)
	}
	if patch.Weight != nil {
		if err := validateLoopSeedWeight(*patch.Weight); err != nil {
			return nil, err
		}
		fields = append(fields, "weight = ?")
		args = append(args, *patch.Weight)
	}
	if patch.Type != nil {
		loopType, err := validateLoopSeedType(*patch.Type)
		if err != nil {
			return nil, err
		}
		fields = append(fields, "type = ?")
		args = append(args, loopType)
	}
	fields = append(fields, "updated_at = ?")
	args = append(args, nowISO(), name)

	seed, err := scanLoopSeed(conn.QueryRow(
		"UPDATE loop SET "+strings.Join(fields, ", ")+" WHERE name = ? "+loopSeedReturning,
		args...,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, loopSeedNotFound(name)
	}
	if err != nil {
		return nil, fmt.Errorf("update loop seed %q: %w", name, err)
	}
	return seed, nil
}

func GetLoopSeedByName(conn *sql.DB, name string) (*LoopSeed, error) {
	name, err := validateLoopSeedName(name)
	if err != nil {
		return nil, err
	}
	seed, err := scanLoopSeed(conn.QueryRow(`
		SELECT id, name, type, describe, content, weight, archived_at, created_at, updated_at
		FROM loop
		WHERE name = ?
	`, name))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, loopSeedNotFound(name)
	}
	if err != nil {
		return nil, fmt.Errorf("get loop seed %q: %w", name, err)
	}
	return seed, nil
}

func ListLoopSeeds(conn *sql.DB, includeArchived bool) ([]LoopSeed, error) {
	query := `
		SELECT id, name, type, describe, content, weight, archived_at, created_at, updated_at
		FROM loop
	`
	if !includeArchived {
		query += " WHERE archived_at IS NULL"
	}
	query += " ORDER BY name, id"

	rows, err := conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("list loop seeds: %w", err)
	}
	defer rows.Close()

	seeds := make([]LoopSeed, 0)
	for rows.Next() {
		seed, err := scanLoopSeed(rows)
		if err != nil {
			return nil, fmt.Errorf("scan loop seed: %w", err)
		}
		seeds = append(seeds, *seed)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list loop seeds: %w", err)
	}
	return seeds, nil
}

func ArchiveLoopSeed(conn *sql.DB, name string) (*LoopSeed, error) {
	return setLoopSeedArchived(conn, name, true)
}

func RestoreLoopSeed(conn *sql.DB, name string) (*LoopSeed, error) {
	return setLoopSeedArchived(conn, name, false)
}

func RemoveLoopSeed(conn *sql.DB, name string) (err error) {
	name, err = validateLoopSeedName(name)
	if err != nil {
		return err
	}
	tx, err := conn.Begin()
	if err != nil {
		return fmt.Errorf("begin removing loop seed %q: %w", name, err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var id int64
	if err = tx.QueryRow("SELECT id FROM loop WHERE name = ?", name).Scan(&id); errors.Is(err, sql.ErrNoRows) {
		return loopSeedNotFound(name)
	} else if err != nil {
		return fmt.Errorf("get loop seed %q for removal: %w", name, err)
	}
	var historyCount int
	if err = tx.QueryRow("SELECT COUNT(*) FROM loop_runs WHERE loop_id = ?", id).Scan(&historyCount); err != nil {
		return fmt.Errorf("check loop seed %q history: %w", name, err)
	}
	if historyCount != 0 {
		return fmt.Errorf("loop seed %q has run history; archive it instead", name)
	}
	if _, err = tx.Exec("DELETE FROM loop WHERE id = ?", id); err != nil {
		return fmt.Errorf("remove loop seed %q: %w", name, err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit removing loop seed %q: %w", name, err)
	}
	return nil
}

func setLoopSeedArchived(conn *sql.DB, name string, archived bool) (*LoopSeed, error) {
	name, err := validateLoopSeedName(name)
	if err != nil {
		return nil, err
	}
	var query string
	var args []any
	if archived {
		now := nowISO()
		query = "UPDATE loop SET archived_at = ?, updated_at = ? WHERE name = ? " + loopSeedReturning
		args = []any{now, now, name}
	} else {
		query = "UPDATE loop SET archived_at = NULL, updated_at = ? WHERE name = ? " + loopSeedReturning
		args = []any{nowISO(), name}
	}
	seed, err := scanLoopSeed(conn.QueryRow(query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, loopSeedNotFound(name)
	}
	if err != nil {
		return nil, fmt.Errorf("set loop seed %q archived state: %w", name, err)
	}
	return seed, nil
}

func validateLoopSeed(name, loopType, describe, content string, weight float64) (string, string, string, string, error) {
	name, err := validateLoopSeedName(name)
	if err != nil {
		return "", "", "", "", err
	}
	loopType, err = validateLoopSeedType(loopType)
	if err != nil {
		return "", "", "", "", err
	}
	describe, err = validateLoopSeedText("describe", describe)
	if err != nil {
		return "", "", "", "", err
	}
	content, err = validateLoopSeedText("content", content)
	if err != nil {
		return "", "", "", "", err
	}
	if err := validateLoopSeedWeight(weight); err != nil {
		return "", "", "", "", err
	}
	return name, loopType, describe, content, nil
}

func validateLoopSeedName(name string) (string, error) {
	return validateLoopSeedText("name", name)
}

func validateLoopSeedText(field, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("loop seed %s must not be blank: %w", field, ErrInvalidLoopSeed)
	}
	return value, nil
}

func validateLoopSeedWeight(weight float64) error {
	if math.IsNaN(weight) || weight < 0 || weight > 1 {
		return fmt.Errorf("loop seed weight must be between 0 and 1: %w", ErrInvalidLoopSeed)
	}
	return nil
}

func validateLoopSeedType(loopType string) (string, error) {
	loopType = strings.TrimSpace(loopType)
	switch loopType {
	case "auto", "normal":
		return loopType, nil
	default:
		return "", fmt.Errorf("loop seed type must be 'auto' or 'normal', got %q: %w", loopType, ErrInvalidLoopSeed)
	}
}

func loopSeedNotFound(name string) error {
	return fmt.Errorf("loop seed %q not found: %w", name, ErrLoopSeedNotFound)
}

func isUniqueConstraint(err error) bool {
	var sqliteErr sqlite3.Error
	return errors.As(err, &sqliteErr) && sqliteErr.ExtendedCode == sqlite3.ErrConstraintUnique
}

type loopSeedScanner interface {
	Scan(dest ...any) error
}

const loopSeedReturning = `
	RETURNING id, name, type, describe, content, weight, archived_at, created_at, updated_at
`

func scanLoopSeed(scanner loopSeedScanner) (*LoopSeed, error) {
	var seed LoopSeed
	var archivedAt sql.NullString
	if err := scanner.Scan(
		&seed.ID, &seed.Name, &seed.Type, &seed.Describe, &seed.Content, &seed.Weight,
		&archivedAt, &seed.CreatedAt, &seed.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if archivedAt.Valid {
		seed.ArchivedAt = &archivedAt.String
	}
	return &seed, nil
}
