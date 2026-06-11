package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"logos/safepath"
)

func Discover(rollRoot string) ([]ValidatedSkill, error) {
	if _, err := os.Lstat(filepath.Join(rollRoot, "Resources", "skills")); os.IsNotExist(err) {
		return []ValidatedSkill{}, nil
	}

	skillsDir, err := resolveSafePath(rollRoot, "Resources/skills")
	if err != nil {
		return nil, fmt.Errorf("resolve skills directory: %w", err)
	}
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil, fmt.Errorf("read skills directory: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	var result []ValidatedSkill
	for _, entry := range entries {
		dir, err := resolveSafePath(rollRoot, "Resources/skills/"+entry.Name())
		if err != nil {
			return nil, fmt.Errorf("validate skill %q: %w", entry.Name(), err)
		}
		info, err := os.Stat(dir)
		if err != nil {
			return nil, fmt.Errorf("stat skill %q: %w", entry.Name(), err)
		}
		if !info.IsDir() {
			continue
		}
		s, err := ValidateSkill(dir)
		if err != nil {
			return nil, fmt.Errorf("validate skill %q: %w", entry.Name(), err)
		}
		result = append(result, *s)
	}
	return result, nil
}

func ValidateSkill(dir string) (*ValidatedSkill, error) {
	data, err := os.ReadFile(filepath.Join(dir, "skill.md"))
	if err != nil {
		return nil, fmt.Errorf("read skill.md: %w", err)
	}

	name, desc, err := parseFrontmatter(string(data))
	if err != nil {
		return nil, err
	}

	base := filepath.Base(dir)
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("skill name must not be empty")
	}
	if strings.TrimSpace(name) != base {
		return nil, fmt.Errorf("skill name %q does not match directory %q", name, base)
	}
	if strings.TrimSpace(desc) == "" {
		return nil, fmt.Errorf("skill description must not be empty")
	}

	return &ValidatedSkill{
		Dir:          dir,
		ResourcePath: "Resources/skills/" + name,
		Name:         strings.TrimSpace(name),
		Description:  strings.TrimSpace(desc),
	}, nil
}

func parseFrontmatter(content string) (name, description string, err error) {
	if !strings.HasPrefix(content, "---") {
		return "", "", fmt.Errorf("skill.md must start with --- frontmatter delimiter")
	}
	rest := content[3:]
	endIdx := strings.Index(rest, "---")
	if endIdx < 0 {
		return "", "", fmt.Errorf("skill.md frontmatter not closed")
	}
	fm := rest[:endIdx]

	for _, line := range strings.Split(fm, "\n") {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "name:"); ok {
			name = strings.TrimSpace(after)
		} else if after, ok := strings.CutPrefix(line, "description:"); ok {
			description = strings.TrimSpace(after)
		}
	}
	return name, description, nil
}

func resolveSafePath(root, relativePath string) (string, error) {
	candidate, err := safepath.Join(root, relativePath)
	if err != nil {
		return "", err
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}
	resolvedCandidate, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(resolvedRoot, resolvedCandidate)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes root", relativePath)
	}
	return resolvedCandidate, nil
}
