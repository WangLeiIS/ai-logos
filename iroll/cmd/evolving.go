package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"logos/builder"
	"logos/db"
	"logos/safepath"
	"logos/store"

	"github.com/spf13/cobra"
)

var evolvingSQL string
var evolvingFile string
var evolvingDryRun bool
var evolvingCwd string

var evolvingCmd = &cobra.Command{
	Use:   "evolving [name:version] [sql]",
	Short: "Execute SQL against an iroll's ai_roll.db",
	Long: `Execute arbitrary SQL statements against an iroll's ai_roll.db database.
Supports SELECT (returns JSON rows) and mutations (returns affected row count).

Target iroll: specify a name:version tag explicitly, or omit to auto-detect from --cwd.
SQL input (priority order): --sql flag, positional arguments, --file flag, stdin.`,
	Args: cobra.ArbitraryArgs,
	Run:  runEvolving,
}

func runEvolving(cmd *cobra.Command, args []string) {
	name, version := resolveEvolvingTarget(args)
	dbPath := checkedDbPath(name, version)

	sql := resolveEvolvingSQL(args)
	if sql == "" || strings.TrimSpace(sql) == "" {
		outputError("no SQL provided (use --sql, positional args, --file, or stdin)")
	}

	conn, err := db.Open(dbPath)
	if err != nil {
		outputError(err.Error())
	}
	defer conn.Close()

	results, err := db.ExecuteAll(conn, sql, evolvingDryRun)
	if err != nil {
		if len(results) > 0 {
			outputJSON(results)
		}
		outputError(err.Error())
	}

	outputJSON(results)
}

// resolveEvolvingTarget resolves the target iroll (name, version).
// If the first positional argument looks like a tag, use it; otherwise detect from --cwd.
func resolveEvolvingTarget(args []string) (string, string) {
	if isTagArg(args) {
		name, version, err := builder.ParseTag(args[0])
		if err == nil {
			return name, version
		}
	}

	// Auto-detect from cwd
	absCwd, err := filepath.Abs(evolvingCwd)
	if err != nil {
		outputError(fmt.Sprintf("resolve cwd: %v", err))
	}
	name, version, _, _, err := store.GetActive(absCwd)
	if err != nil {
		outputError(err.Error())
	}
	return name, version
}

// isTagArg returns true if the first positional argument was consumed as a tag (not SQL).
func isTagArg(args []string) bool {
	if len(args) == 0 {
		return false
	}
	first := args[0]
	if strings.Contains(first, " ") {
		return false
	}
	if err := safepath.ValidateName(strings.SplitN(first, ":", 2)[0]); err != nil {
		return false
	}
	_, _, err := builder.ParseTag(first)
	return err == nil
}

// resolveEvolvingSQL resolves the SQL input from flags, args, file, or stdin.
func resolveEvolvingSQL(args []string) string {
	// Priority 1: --sql flag
	if evolvingSQL != "" {
		return evolvingSQL
	}

	// Priority 2: positional args (skip first if it's a tag)
	if len(args) > 0 {
		start := 0
		if isTagArg(args) {
			start = 1
		}
		if len(args) > start {
			return strings.Join(args[start:], " ")
		}
	}

	// Priority 3: --file flag
	if evolvingFile != "" {
		data, err := os.ReadFile(evolvingFile)
		if err != nil {
			outputError(fmt.Sprintf("read file %q: %v", evolvingFile, err))
		}
		return string(data)
	}

	// Priority 4: stdin
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			outputError(fmt.Sprintf("read stdin: %v", err))
		}
		return string(data)
	}

	return ""
}

func init() {
	evolvingCmd.Flags().StringVar(&evolvingSQL, "sql", "", "SQL statement(s) to execute")
	evolvingCmd.Flags().StringVar(&evolvingFile, "file", "", "Path to SQL file")
	evolvingCmd.Flags().BoolVar(&evolvingDryRun, "dry-run", false, "Preview mode: execute in transaction then rollback")
	evolvingCmd.Flags().StringVar(&evolvingCwd, "cwd", ".", "Working directory (auto-detect mode)")

	rollCmd.AddCommand(evolvingCmd)
}
