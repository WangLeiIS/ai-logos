package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// resolveSQLInput resolves SQL text from, in priority order:
// the --sql flag, positional args (optionally skipping the first when it is a tag),
// the --file flag, or piped stdin. Returns "" if no input is available.
// skipFirstPositional is true for commands whose first positional is a target tag
// (e.g. evolving's name:version), false otherwise.
func resolveSQLInput(sqlFlag, fileFlag string, args []string, skipFirstPositional bool) string {
	if sqlFlag != "" {
		return sqlFlag
	}
	start := 0
	if skipFirstPositional && len(args) > 0 {
		start = 1
	}
	if len(args) > start {
		return strings.Join(args[start:], " ")
	}
	if fileFlag != "" {
		data, err := os.ReadFile(fileFlag)
		if err != nil {
			outputFail(ErrCodeInternal, fmt.Sprintf("read file %q: %v", fileFlag, err), nil)
		}
		return string(data)
	}
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			outputFail(ErrCodeInternal, fmt.Sprintf("read stdin: %v", err), nil)
		}
		return string(data)
	}
	return ""
}
