//go:build !windows

package book

import (
	"os"
	"path/filepath"
)

func resolveExistingPath(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}

func resolveOpenFilePath(_ *os.File, candidate string) (string, error) {
	return filepath.EvalSymlinks(candidate)
}
