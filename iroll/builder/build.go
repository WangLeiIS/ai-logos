package builder

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"logos/db"

	_ "github.com/mattn/go-sqlite3"
)

type LayerJSON struct {
	LayerID       string `json:"layer_id"`
	Parent        string `json:"parent,omitempty"`
	Description   string `json:"description"`
	CreatedAt     string `json:"created_at"`
	SchemaVersion int    `json:"schema_version"`
}

type BuildResult struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	LayerID string `json:"layer_id"`
}

func Build(lf *Layerfile, tagName string) (*BuildResult, error) {
	tmpDir, err := os.MkdirTemp("", "iroll-build-*")
	if err != nil {
		return nil, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			os.RemoveAll(tmpDir)
		}
	}()

	var parentLayerID string

	for _, inst := range lf.Instructions {
		switch inst.Type {
		case InstFrom:
			parentLayerID, err = processFrom(tmpDir, inst.Args[0])
			if err != nil {
				return nil, err
			}

		case InstMigrate:
			err = processMigrate(tmpDir, lf.Dir, inst.Args[0])
			if err != nil {
				return nil, err
			}

		case InstCopy:
			err = processCopy(tmpDir, lf.Dir, inst.Args[0], inst.Args[1])
			if err != nil {
				return nil, err
			}
		}
	}

	// Ensure ai_roll.db exists
	dbPath := filepath.Join(tmpDir, "ai_roll.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		conn, err := sql.Open("sqlite3", dbPath)
		if err != nil {
			return nil, err
		}
		conn.Close()
	}

	// Compute layer hash
	layerID, err := computeDirHash(tmpDir)
	if err != nil {
		return nil, err
	}

	// Write layer.json
	now := time.Now().UTC().Format(time.RFC3339Nano)
	lj := LayerJSON{
		LayerID:       layerID,
		Parent:        parentLayerID,
		Description:   fmt.Sprintf("build from Layerfile for %s", tagName),
		CreatedAt:     now,
		SchemaVersion: 1,
	}
	ljBytes, _ := json.MarshalIndent(lj, "", "  ")
	if err := os.WriteFile(filepath.Join(tmpDir, "layer.json"), ljBytes, 0644); err != nil {
		return nil, err
	}

	// Record history
	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := db.EnsureHistoryTable(conn); err != nil {
		return nil, err
	}

	instrSummary, _ := json.Marshal(lf.Instructions)
	if err := db.InsertHistory(conn, parentLayerID, lj.Description, layerID, string(instrSummary)); err != nil {
		return nil, err
	}

	// Copy to ~/.iroll/<name>/
	home, _ := os.UserHomeDir()
	dest := filepath.Join(home, ".iroll", tagName)
	if _, err := os.Stat(dest); err == nil {
		return nil, fmt.Errorf("iroll '%s' already exists", tagName)
	}
	if err := copyDir(tmpDir, dest); err != nil {
		return nil, fmt.Errorf("copy to store: %w", err)
	}

	return &BuildResult{
		Name:    tagName,
		Path:    dest,
		LayerID: layerID,
	}, nil
}

func processFrom(tmpDir string, baseName string) (string, error) {
	home, _ := os.UserHomeDir()
	src := filepath.Join(home, ".iroll", baseName)
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return "", fmt.Errorf("base iroll '%s' not found in ~/.iroll/", baseName)
	}

	if err := copyDir(src, tmpDir); err != nil {
		return "", fmt.Errorf("copy base layer: %w", err)
	}

	ljPath := filepath.Join(tmpDir, "layer.json")
	if data, err := ioutil.ReadFile(ljPath); err == nil {
		var lj LayerJSON
		if json.Unmarshal(data, &lj) == nil {
			return lj.LayerID, nil
		}
	}
	return "", nil
}

func processMigrate(tmpDir string, lfDir string, sqlFile string) error {
	sqlPath := filepath.Join(lfDir, sqlFile)
	if _, err := os.Stat(sqlPath); os.IsNotExist(err) {
		return fmt.Errorf("sql file not found: %s", sqlPath)
	}

	dbPath := filepath.Join(tmpDir, "ai_roll.db")
	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}
	defer conn.Close()

	return db.ExecuteSQL(conn, sqlPath)
}

func processCopy(tmpDir string, lfDir string, src string, dest string) error {
	srcPath := filepath.Join(lfDir, src)
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		return fmt.Errorf("source not found: %s", src)
	}

	destPath := filepath.Join(tmpDir, dest)
	os.MkdirAll(filepath.Dir(destPath), 0755)

	return copyDir(srcPath, destPath)
}

func copyDir(src string, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}

	if info.IsDir() {
		os.MkdirAll(dst, 0755)
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := copyDir(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
				return err
			}
		}
		return nil
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

func computeDirHash(dir string) (string, error) {
	h := sha256.New()
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		h.Write([]byte(rel))
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		io.Copy(h, f)
		f.Close()
		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", h.Sum(nil)), nil
}
