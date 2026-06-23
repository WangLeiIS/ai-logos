package builder

import (
	"fmt"
	"strings"

	"logos/safepath"
)

// ParseTag parses a build tag like "my-agent:v0.1.0" into name and version.
// Default version is "latest".
func ParseTag(raw string) (name, version string, err error) {
	if raw == "" {
		return "", "", fmt.Errorf("tag cannot be empty")
	}

	parts := strings.SplitN(raw, ":", 2)
	name = strings.TrimSpace(parts[0])
	if name == "" {
		return "", "", fmt.Errorf("name cannot be empty")
	}
	if err := safepath.ValidateName(name); err != nil {
		return "", "", fmt.Errorf("invalid name: %w", err)
	}

	if len(parts) == 2 {
		version = strings.TrimSpace(parts[1])
		if version == "" {
			return "", "", fmt.Errorf("version cannot be empty")
		}
	} else {
		version = "latest"
	}
	return name, version, nil
}
