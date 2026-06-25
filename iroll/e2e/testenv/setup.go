package testenv

import (
	"database/sql"
	"io"
	"os"
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
// tagName supports "name:version" format; defaults to "latest" version.
func (e *Env) Build(tagName string) (*builder.BuildResult, error) {
	e.t.Helper()
	name, version, err := builder.ParseTag(tagName)
	if err != nil {
		return nil, err
	}
	lfPath := filepath.Join("..", "..", "examples", "base-agent", "Irollfile")
	lf, err := builder.ParseIrollfile(lfPath)
	if err != nil {
		return nil, err
	}
	return builder.Build(lf, name, version)
}

// DB opens the inner roll-inner.db for the given iroll name and attaches it as "inner".
// This allows both direct table access (e.g., FROM dna) and "inner." prefix access
// (e.g., FROM inner.dna, FROM inner.loop) to work on the same connection.
func (e *Env) DB(name string) (*sql.DB, error) {
	e.t.Helper()
	innerPath, err := store.InnerDbPath(name, "latest")
	if err != nil {
		return nil, err
	}
	conn, err := db.Open(innerPath)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1)
	if _, err := conn.Exec("ATTACH DATABASE ? AS inner", innerPath); err != nil {
		conn.Close()
		return nil, err
	}
	e.t.Cleanup(func() { conn.Close() })
	return conn, nil
}

// OpenWorkspace creates a workspace outer DB (copying from the iroll template)
// and opens it with the inner DB attached. Use this for functions that require
// the "inner." prefix (e.g., QueryDna, InsertPage, InsertLoopSeed, StartLoopRun,
// ResolveContext).
func (e *Env) OpenWorkspace(name, version, cwd string) (*sql.DB, error) {
	e.t.Helper()
	innerPath, err := store.InnerDbPath(name, version)
	if err != nil {
		return nil, err
	}
	outerPath, err := store.CwdOuterDbPath(cwd, name)
	if err != nil {
		return nil, err
	}
	// Copy template outer DB if not exists
	if _, err := os.Stat(outerPath); os.IsNotExist(err) {
		irollPath, err := store.IrollPath(name, version)
		if err != nil {
			return nil, err
		}
		templateOuter := filepath.Join(irollPath, "roll-outer.db")
		if err := copyFile(templateOuter, outerPath); err != nil {
			return nil, err
		}
	}
	conn, err := db.OpenOuter(outerPath, innerPath)
	if err != nil {
		return nil, err
	}
	e.t.Cleanup(func() { conn.Close() })
	return conn, nil
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()
	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()
	_, err = io.Copy(dstFile, srcFile)
	return err
}

// CreatePage creates a workspace outer DB, inserts a page, and registers it in system.db.
func (e *Env) CreatePage(name, version, cwd string) (*db.Page, error) {
	e.t.Helper()
	outerPath, err := store.CwdOuterDbPath(cwd, name)
	if err != nil {
		return nil, err
	}
	conn, err := e.OpenWorkspace(name, version, cwd)
	if err != nil {
		return nil, err
	}
	page, err := db.InsertPage(conn, cwd)
	if err != nil {
		return nil, err
	}
	if err := store.IndexPage(name, version, page.PageID, cwd, outerPath); err != nil {
		return nil, err
	}
	return page, nil
}

// IrollfilePath returns the path to examples/base-agent/Irollfile.
func (e *Env) IrollfilePath() string {
	return filepath.Join("..", "..", "examples", "base-agent", "Irollfile")
}
