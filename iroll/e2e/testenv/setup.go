package testenv

import (
	"database/sql"
	"path/filepath"
	"testing"

	"logos/builder"
	"logos/db"
	"logos/store"
)

// Env holds a fully isolated Logos test environment.
type Env struct {
	Home  string // temporary HOME directory
	Store string // ~/.iroll/ path within temp HOME
	t     *testing.T
}

// New creates a temporary HOME with an initialized system.db.
func New(t *testing.T) *Env {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	storeDir := filepath.Join(home, ".iroll")
	return &Env{Home: home, Store: storeDir, t: t}
}

// Build runs builder.Build using examples/base-agent/Irollfile.
func (e *Env) Build(tagName string) (*builder.BuildResult, error) {
	e.t.Helper()
	lfPath := filepath.Join("..", "..", "examples", "base-agent", "Irollfile")
	lf, err := builder.ParseIrollfile(lfPath)
	if err != nil {
		return nil, err
	}
	return builder.Build(lf, tagName)
}

// DB opens the ai_roll.db for the given iroll name.
func (e *Env) DB(name string) (*sql.DB, error) {
	e.t.Helper()
	dbPath, err := store.DbPath(name)
	if err != nil {
		return nil, err
	}
	conn, err := db.Open(dbPath)
	if err != nil {
		return nil, err
	}
	e.t.Cleanup(func() { conn.Close() })
	return conn, nil
}

// CreatePage inserts a page into the iroll's DB and registers it in system.db.
func (e *Env) CreatePage(name, pageID, cwd string) (*db.Page, error) {
	e.t.Helper()
	conn, err := e.DB(name)
	if err != nil {
		return nil, err
	}
	page, err := db.InsertPage(conn, cwd)
	if err != nil {
		return nil, err
	}
	if err := store.IndexPage(name, page.PageID, cwd); err != nil {
		return nil, err
	}
	return page, nil
}

// IrollfilePath returns the path to examples/base-agent/Irollfile.
func (e *Env) IrollfilePath() string {
	return filepath.Join("..", "..", "examples", "base-agent", "Irollfile")
}

// SchemaPath returns the path to examples/base-agent/init_schema.sql.
func (e *Env) SchemaPath() string {
	return filepath.Join("..", "..", "examples", "base-agent", "init_schema.sql")
}
