package book

import (
	"fmt"
	"strings"
)

func NormalizeKeyword(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func NormalizeTags(values []string) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		normalized := NormalizeKeyword(value)
		if normalized == "" {
			return nil, fmt.Errorf("tag %q must not be empty", value)
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("at least one non-empty tag is required")
	}
	return result, nil
}
