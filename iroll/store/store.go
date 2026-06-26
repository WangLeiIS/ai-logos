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

func IrollPath(name string, version string) (string, error) {
	if err := safepath.ValidateName(name); err != nil {
		return "", err
	}
	// Two-step join: name then version
	root, err := safepath.Join(HomeDir(), name)
	if err != nil {
		return "", err
	}
	return safepath.Join(root, version)
}

// InnerDbPath returns the path to the inner database (roll-inner.db)
func InnerDbPath(name string, version string) (string, error) {
	root, err := IrollPath(name, version)
	if err != nil {
		return "", err
	}
	return safepath.Join(root, "roll-inner.db")
}

// Deprecated: use InnerDbPath
func DbPath(name string, version string) (string, error) {
	return InnerDbPath(name, version)
}

// WorkspaceOuterDbPath returns the outer db path for default workspace pages.
func WorkspaceOuterDbPath(name, version string) (string, error) {
	root, err := IrollPath(name, version)
	if err != nil {
		return "", err
	}
	ws, err := safepath.Join(root, "workspace")
	if err != nil {
		return "", err
	}
	return safepath.Join(ws, "."+name+".outer.db")
}

// CwdOuterDbPath returns the outer db path for custom-cwd pages.
func CwdOuterDbPath(cwd, name string) (string, error) {
	irollDir, err := safepath.Join(cwd, ".iroll")
	if err != nil {
		return "", err
	}
	return safepath.Join(irollDir, name+".db")
}

// ReadName reads the name value from metadata table inside roll-inner.db within a ZIP file
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
		if f.Name == "roll-inner.db" {
			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			outPath := filepath.Join(tmpDir, "roll-inner.db")
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

	return "", fmt.Errorf("roll-inner.db not found in zip")
}

// Extract extracts a .iroll ZIP to ~/.iroll/<name>/<version>/
func Extract(zipPath string, name string, version string) error {
	dest, err := IrollPath(name, version)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("iroll '%s:%s' already exists", name, version)
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

// List returns all loaded iroll names with versions as "name:version".
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
		if !e.IsDir() {
			continue
		}
		rootDir := filepath.Join(home, e.Name())
		versions, err := os.ReadDir(rootDir)
		if err != nil {
			continue
		}
		for _, v := range versions {
			if !v.IsDir() {
				continue
			}
			dbFile := filepath.Join(rootDir, v.Name(), "roll-inner.db")
			if _, err := os.Stat(dbFile); err == nil {
				names = append(names, e.Name()+":"+v.Name())
			}
		}
	}
	return names, nil
}
