package safepath

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ValidateName checks that name is a single directory name, not a path.
func ValidateName(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("invalid iroll name %q: must be a non-empty directory name", name)
	}
	if filepath.IsAbs(name) || filepath.VolumeName(name) != "" {
		return fmt.Errorf("invalid iroll name %q: absolute paths are not allowed", name)
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("invalid iroll name %q: path separators are not allowed", name)
	}
	return nil
}

// Join resolves a relative path below root and rejects paths that escape root.
func Join(root string, relativePath string) (string, error) {
	if relativePath == "" || relativePath == "." {
		return "", fmt.Errorf("unsafe path %q: empty paths are not allowed", relativePath)
	}
	if filepath.IsAbs(relativePath) || filepath.VolumeName(relativePath) != "" {
		return "", fmt.Errorf("unsafe path %q: absolute paths are not allowed", relativePath)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}
	candidate := filepath.Join(absRoot, relativePath)
	rel, err := filepath.Rel(absRoot, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w", relativePath, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe path %q: path escapes root", relativePath)
	}
	return candidate, nil
}
