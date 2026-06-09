package store

import (
	"archive/zip"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"logos/safepath"

	_ "github.com/mattn/go-sqlite3"
)

func HomeDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".iroll")
}

func IrollPath(name string) (string, error) {
	if err := safepath.ValidateName(name); err != nil {
		return "", err
	}
	return safepath.Join(HomeDir(), name)
}

func DbPath(name string) (string, error) {
	root, err := IrollPath(name)
	if err != nil {
		return "", err
	}
	return safepath.Join(root, "ai_roll.db")
}

// ReadName reads the name value from metadata table inside ai_roll.db within a ZIP file
func ReadName(zipPath string) (string, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

	tmpDir, err := os.MkdirTemp("", "iroll-load-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	for _, f := range r.File {
		if f.Name == "ai_roll.db" {
			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			outPath := filepath.Join(tmpDir, "ai_roll.db")
			outFile, err := os.Create(outPath)
			if err != nil {
				rc.Close()
				return "", err
			}
			if _, err = outFile.ReadFrom(rc); err != nil {
				outFile.Close()
				rc.Close()
				return "", err
			}
			outFile.Close()
			rc.Close()

			db, err := sql.Open("sqlite3", outPath)
			if err != nil {
				return "", err
			}
			defer db.Close()

			var name string
			err = db.QueryRow("SELECT value FROM metadata WHERE key = 'name'").Scan(&name)
			if err != nil {
				return "", fmt.Errorf("read name from metadata: %w", err)
			}
			return name, nil
		}
	}

	return "", fmt.Errorf("ai_roll.db not found in zip")
}

// Extract extracts a .iroll ZIP to ~/.iroll/<name>/
func Extract(zipPath string, name string) error {
	dest, err := IrollPath(name)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("iroll '%s' already exists", name)
	}

	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

	paths := make(map[*zip.File]string, len(r.File))
	for _, f := range r.File {
		outPath, err := safepath.Join(dest, f.Name)
		if err != nil {
			return fmt.Errorf("invalid zip entry %q: %w", f.Name, err)
		}
		paths[f] = outPath
	}

	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}

	for _, f := range r.File {
		outPath := paths[f]

		if f.FileInfo().IsDir() {
			os.MkdirAll(outPath, 0755)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return err
		}

		outFile, err := os.Create(outPath)
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = outFile.ReadFrom(rc)
		rc.Close()
		outFile.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

// List returns all loaded iroll names
func List() ([]string, error) {
	home := HomeDir()
	entries, err := os.ReadDir(home)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() {
			dbFile := filepath.Join(home, e.Name(), "ai_roll.db")
			if _, err := os.Stat(dbFile); err == nil {
				names = append(names, e.Name())
			}
		}
	}
	return names, nil
}
