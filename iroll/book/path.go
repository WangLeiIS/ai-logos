package book

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"logos/safepath"
)

func openValidatedFile(root, relativePath string) (*os.File, error) {
	candidate, err := safepath.Join(root, relativePath)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(candidate)
	if err != nil {
		return nil, err
	}
	closeOnError := func(err error) (*os.File, error) {
		file.Close()
		return nil, err
	}
	resolvedRoot, err := evalAbsolute(root)
	if err != nil {
		return closeOnError(fmt.Errorf("resolve root links: %w", err))
	}
	resolvedFile, err := resolveOpenFilePath(file, candidate)
	if err != nil {
		return closeOnError(fmt.Errorf("resolve opened file: %w", err))
	}
	if !pathWithin(resolvedRoot, resolvedFile) {
		return closeOnError(fmt.Errorf("path %q escapes root through symlink or junction", relativePath))
	}
	handleInfo, err := file.Stat()
	if err != nil {
		return closeOnError(fmt.Errorf("stat opened file: %w", err))
	}
	resolvedInfo, err := os.Stat(resolvedFile)
	if err != nil {
		return closeOnError(fmt.Errorf("stat resolved file: %w", err))
	}
	if !os.SameFile(handleInfo, resolvedInfo) {
		return closeOnError(fmt.Errorf("opened file changed during validation"))
	}
	return file, nil
}

func existingPathWithin(root, relativePath string) (string, error) {
	candidate, err := safepath.Join(root, relativePath)
	if err != nil {
		return "", err
	}
	resolvedRoot, err := evalAbsolute(root)
	if err != nil {
		return "", fmt.Errorf("resolve root links: %w", err)
	}
	resolvedCandidate, err := evalAbsolute(candidate)
	if err != nil {
		return "", err
	}
	if !pathWithin(resolvedRoot, resolvedCandidate) {
		return "", fmt.Errorf("path %q escapes root through symlink or junction", relativePath)
	}
	return resolvedCandidate, nil
}

func optionalExistingPathWithin(root, relativePath string) error {
	candidate, err := safepath.Join(root, relativePath)
	if err != nil {
		return err
	}
	current := candidate
	for {
		if _, err := os.Lstat(current); err == nil {
			break
		} else if !os.IsNotExist(err) {
			return err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return fmt.Errorf("cannot resolve existing ancestor for %q", relativePath)
		}
		current = parent
	}
	resolvedRoot, err := evalAbsolute(root)
	if err != nil {
		return fmt.Errorf("resolve root links: %w", err)
	}
	resolvedCurrent, err := evalAbsolute(current)
	if err != nil {
		return err
	}
	if !pathWithin(resolvedRoot, resolvedCurrent) {
		return fmt.Errorf("path %q escapes root through symlink or junction", relativePath)
	}
	return nil
}

func evalAbsolute(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return resolveExistingPath(absolute)
}

func pathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
