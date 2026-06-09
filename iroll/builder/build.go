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

	"logos/book"
	"logos/db"
	"logos/safepath"

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
	if err := safepath.ValidateName(tagName); err != nil {
		return nil, err
	}

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

	dbPath := filepath.Join(tmpDir, "ai_roll.db")
	bundles, err := book.Discover(tmpDir)
	if err != nil {
		return nil, fmt.Errorf("validate books: %w", err)
	}
	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}
	if err := db.SyncBooks(conn, bundles); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("sync books: %w", err)
	}
	if err := checkpointAndCloseSQLite(conn); err != nil {
		return nil, fmt.Errorf("persist books: %w", err)
	}

	// The layer hash covers content state only; layer.json and history are build metadata added afterward.
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
	conn, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}
	if err := db.EnsureHistoryTable(conn); err != nil {
		_ = conn.Close()
		return nil, err
	}

	instrSummary, _ := json.Marshal(lf.Instructions)
	if err := db.InsertHistory(conn, parentLayerID, lj.Description, layerID, string(instrSummary)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := checkpointAndCloseSQLite(conn); err != nil {
		return nil, fmt.Errorf("persist build database: %w", err)
	}

	// Copy to ~/.iroll/<name>/
	home, _ := os.UserHomeDir()
	storeRoot := filepath.Join(home, ".iroll")
	dest, err := safepath.Join(storeRoot, tagName)
	if err != nil {
		return nil, err
	}
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

func checkpointSQLite(conn *sql.DB) error {
	var busy, logFrames, checkpointedFrames int
	if err := conn.QueryRow("PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logFrames, &checkpointedFrames); err != nil {
		return err
	}
	if busy != 0 {
		return fmt.Errorf("WAL checkpoint busy with %d log frames and %d checkpointed frames", logFrames, checkpointedFrames)
	}
	return nil
}

func checkpointAndCloseSQLite(conn *sql.DB) error {
	checkpointErr := checkpointSQLite(conn)
	closeErr := conn.Close()
	if checkpointErr != nil {
		if closeErr != nil {
			return fmt.Errorf("checkpoint: %v; close: %w", checkpointErr, closeErr)
		}
		return checkpointErr
	}
	return closeErr
}

func processFrom(tmpDir string, baseName string) (string, error) {
	if err := safepath.ValidateName(baseName); err != nil {
		return "", err
	}
	home, _ := os.UserHomeDir()
	src, err := safepath.Join(filepath.Join(home, ".iroll"), baseName)
	if err != nil {
		return "", err
	}
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
	sqlPath, err := safepath.Join(lfDir, sqlFile)
	if err != nil {
		return err
	}
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
	srcPath, err := safepath.Join(lfDir, src)
	if err != nil {
		return err
	}
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		return fmt.Errorf("source not found: %s", src)
	}

	destPath, err := safepath.Join(tmpDir, dest)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}

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
