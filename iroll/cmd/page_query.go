package cmd

import (
	"path/filepath"
	"strings"

	"logos/db"
	"logos/store"

	"github.com/spf13/cobra"
)

var (
	pageQuerySQL    string
	pageQueryFile   string
	pageQueryDryRun bool
)

// pageQueryTarget holds targeting flags for page query (no --roll: it is page-scoped).
var pageQueryTarget struct {
	page, alias, cwd string
}

// resolveActiveOuter resolves --page / --alias / --cwd to the cwd outer.db path + pageID.
// Used by `page query`, which opens that outer standalone (no inner attach).
func resolveActiveOuter(flagPage, flagAlias, cwd string) (outerPath, pageID string) {
	if flagPage != "" {
		_, _, op, err := store.LookupPageByID(flagPage)
		if err != nil {
			outputFail(ErrCodePageNotFound, flagPage+" not found: "+err.Error(), nil)
		}
		return op, flagPage
	}
	if flagAlias != "" {
		_, _, pid, op, err := store.LookupPageByAlias(flagAlias)
		if err != nil {
			outputFail(ErrCodePageNotFound, "alias "+flagAlias+" not found: "+err.Error(), nil)
		}
		return op, pid
	}
	_, _, pid, op, err := store.GetActive(cwd)
	if err != nil {
		outputFail(ErrCodeNoActivePage, err.Error(), []Hint{
			{Action: "Create a new page for this directory", Cmd: "logos page new <iroll-name>"},
			{Action: "List all pages", Cmd: "logos page list -a"},
		})
	}
	return op, pid
}

var pageQueryCmd = &cobra.Command{
	Use:   "query [sql]",
	Short: "Run raw SQL against this page's outer database (pages / memory / loop_runs)",
	Long: `Run raw SQL against the current page's outer database.
Target page is resolved from --page, --alias, or the active page of --cwd.

Only the outer tables (pages, memory, loop_runs) are visible; inner tables
(dna, loop seeds, skills, metadata) are NOT attached — use query-dna or
roll evolving for those.

SQL input priority: --sql flag, positional args, --file flag, stdin.`,
	Args: cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		cwd, _ := filepath.Abs(pageQueryTarget.cwd)
		outerPath, _ := resolveActiveOuter(pageQueryTarget.page, pageQueryTarget.alias, cwd)

		// Standalone outer connection (read-write). No inner attach by design.
		conn, err := db.Open(outerPath)
		if err != nil {
			outputFail(ErrCodeDBOpen, err.Error(), nil)
		}
		defer conn.Close()

		sqlText := resolveSQLInput(pageQuerySQL, pageQueryFile, args, false)
		if strings.TrimSpace(sqlText) == "" {
			outputFail(ErrCodeInternal, "no SQL provided (use --sql, positional args, --file, or stdin)", nil)
		}

		results, err := db.ExecuteAll(conn, sqlText, pageQueryDryRun)
		if err != nil {
			if len(results) > 0 {
				outputOK(results, nil)
			}
			outputFail(ErrCodeInternal, err.Error(), nil)
		}
		outputOK(results, nil)
	},
}

func init() {
	pageQueryCmd.Flags().StringVar(&pageQueryTarget.page, "page", "", "Page ID")
	pageQueryCmd.Flags().StringVar(&pageQueryTarget.alias, "alias", "", "Page alias")
	pageQueryCmd.Flags().StringVar(&pageQueryTarget.cwd, "cwd", ".", "Working directory")
	pageQueryCmd.Flags().StringVar(&pageQuerySQL, "sql", "", "SQL statement(s) to execute")
	pageQueryCmd.Flags().StringVar(&pageQueryFile, "file", "", "Path to SQL file")
	pageQueryCmd.Flags().BoolVar(&pageQueryDryRun, "dry-run", false, "Preview mode: execute in a transaction then rollback")

	pageCmd.AddCommand(pageQueryCmd)
}
