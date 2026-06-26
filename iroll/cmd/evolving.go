package cmd

import (
	"fmt"
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
	Short: "Execute SQL against an iroll's database",
	Long: `Execute arbitrary SQL statements against an iroll's external database (with inner database attached).
	Supports SELECT (returns JSON rows) and mutations (returns affected row count).

	Target iroll: specify a name:version tag explicitly, or omit to auto-detect from --cwd.
	SQL input (priority order): --sql flag, positional arguments, --file flag, stdin.`,
	Args: cobra.ArbitraryArgs,
	Run:  runEvolving,
}

func runEvolving(cmd *cobra.Command, args []string) {
	name, version := resolveEvolvingTarget(args)
	innerPath := checkedInnerPath(name, version)

	// evolving operates at the ROLL level: the template roll-outer.db + roll-inner.db.
	// It never touches page-level (cwd) live outer databases.
	templateOuter := filepath.Join(checkedIrollPath(name, version), "roll-outer.db")
	if _, err := os.Stat(templateOuter); err != nil {
		outputFail(ErrCodeIrollNotFound, fmt.Sprintf("template outer db not found for %s:%s: %v", name, version, err), nil)
	}

	sql := resolveEvolvingSQL(args)
	if strings.TrimSpace(sql) == "" {
		outputFail(ErrCodeInternal, "no SQL provided (use --sql, positional args, --file, or stdin)", nil)
	}

	// Open the template as the main db with inner attached (both read-write).
	// Bare tables address the template outer; inner.* addresses the blueprint.
	conn, err := db.OpenOuter(templateOuter, innerPath)
	if err != nil {
		outputFail(ErrCodeDBOpen, err.Error(), nil)
	}
	defer conn.Close()

	results, err := db.ExecuteAll(conn, sql, evolvingDryRun)
	if err != nil {
		if len(results) > 0 {
			outputOK(results, nil)
		}
		outputFail(ErrCodeInternal, err.Error(), nil)
	}
	outputOK(results, nil)
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
		outputFail(ErrCodeInternal, fmt.Sprintf("resolve cwd: %v", err), nil)
	}
	name, version, _, _, err := store.GetActive(absCwd)
	if err != nil {
		outputFail(ErrCodeNoActivePage, err.Error(), []Hint{
			{Action: "Create a new page for this directory", Cmd: "logos page new <iroll-name>"},
			{Action: "List all pages", Cmd: "logos page list -a"},
		})
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

// resolveEvolvingSQL resolves the SQL input for evolving. The first positional arg
// is a target tag (name:version), so it is skipped when extracting SQL from positionals.
func resolveEvolvingSQL(args []string) string {
	return resolveSQLInput(evolvingSQL, evolvingFile, args, isTagArg(args))
}

func init() {
	evolvingCmd.Flags().StringVar(&evolvingSQL, "sql", "", "SQL statement(s) to execute")
	evolvingCmd.Flags().StringVar(&evolvingFile, "file", "", "Path to SQL file")
	evolvingCmd.Flags().BoolVar(&evolvingDryRun, "dry-run", false, "Preview mode: execute in transaction then rollback")
	evolvingCmd.Flags().StringVar(&evolvingCwd, "cwd", ".", "Working directory (auto-detect mode)")

	rollCmd.AddCommand(evolvingCmd)
}
