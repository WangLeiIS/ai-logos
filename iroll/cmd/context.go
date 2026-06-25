package cmd

import (
	"database/sql"
	"fmt"
	"path/filepath"

	"logos/builder"
	"logos/db"
	"logos/store"

	"github.com/spf13/cobra"
)

var getContextPage string
var getContextRoll string
var getContextAlias string
var getContextCwd string

var updateContextPage string
var updateContextRoll string
var updateContextAlias string
var updateContextSetAlias string
var updateContextContext string
var updateContextCwd string

var getContextCmd = &cobra.Command{
	Use:   "get-context [name]",
	Short: "Get context by page id",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cwd, _ := filepath.Abs(getContextCwd)
		name, version, pageID, conn := resolvePageContext(args, getContextPage, getContextAlias, getContextRoll, cwd)
		defer conn.Close()

		p, err := db.GetPageByPageID(conn, pageID)
		if err != nil {
			outputFail(ErrCodePageNotFound, err.Error(), nil)
		}

		p.Context, err = db.ResolveContext(p.Context, checkedIrollPath(name, version), conn, p.PageID)
		if err != nil {
			outputFail(ErrCodeInternal, err.Error(), nil)
		}

		hints := getContextHints(p)
		outputOK(p, hints)
	},
}

var updateContextCmd = &cobra.Command{
	Use:   "update-context [name]",
	Short: "Update page context",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cwd, _ := filepath.Abs(updateContextCwd)
		_, _, pageID, conn := resolvePageContext(args, updateContextPage, updateContextAlias, updateContextRoll, cwd)
		defer conn.Close()

		hasContent := cmd.Flags().Changed("content")
		hasSetAlias := cmd.Flags().Changed("set-alias")

		if !hasContent && !hasSetAlias {
			outputFail(ErrCodeInternal, "at least one of --content or --set-alias is required", nil)
		}

		// Handle --set-alias
		if hasSetAlias {
			// Update alias in page_index (system.db)
			if err := store.SetPageAlias(pageID, updateContextSetAlias); err != nil {
				outputFail(ErrCodeInternal, err.Error(), nil)
			}
			// Also update alias in the iroll pages table
			if err := db.UpdatePageAlias(conn, pageID, updateContextSetAlias); err != nil {
				outputFail(ErrCodeInternal, err.Error(), nil)
			}
		}

		if hasContent {
			p, err := db.UpdatePageContext(conn, pageID, updateContextContext)
			if err != nil {
				outputFail(ErrCodeInternal, err.Error(), nil)
			}
			outputOK(p, getContextHints(p))
			return
		}

		// Only alias was set, return current page
		p, err := db.GetPageByPageID(conn, pageID)
		if err != nil {
			outputFail(ErrCodeInternal, err.Error(), nil)
		}
		outputOK(p, getContextHints(p))
	},
}

// getContextHints returns hints suggesting the agent get the full context.
func getContextHints(p *db.Page) []Hint {
	hints := make([]Hint, 0, 2)

	// If alias is set, suggest --alias lookup
	if p.Alias != "" {
		hints = append(hints, Hint{
			Action: "Get the full context including DNA, loops and system prompt",
			Cmd:    fmt.Sprintf("logos page get-context --alias %s", p.Alias),
		})
	}

	// Always suggest --page lookup as fallback
	hints = append(hints, Hint{
		Action: "Get the full context including DNA, loops and system prompt",
		Cmd:    fmt.Sprintf("logos page get-context --page %s", p.PageID),
	})

	return hints
}

// resolvePageContext resolves args/flags into a db connection with attached inner db.
// Priority: --page > --alias > --roll > positional arg > current cwd.
// Returns (name, version, pageID, conn).
func resolvePageContext(args []string, flagPage, flagAlias, flagRoll, cwd string) (string, string, string, *sql.DB) {
	// 1. --page: look up by page_id
	if flagPage != "" {
		name, version, outerPath, err := store.LookupPageByID(flagPage)
		if err != nil {
			outputFail(ErrCodePageNotFound, fmt.Sprintf("page %s not found: %v", flagPage, err), nil)
		}
		innerPath := checkedInnerPath(name, version)
		conn, err := db.OpenOuter(outerPath, innerPath)
		if err != nil {
			outputFail(ErrCodeDBOpen, err.Error(), nil)
		}
		return name, version, flagPage, conn
	}

	// 2. --alias: look up by alias
	if flagAlias != "" {
		name, version, pageID, outerPath, err := store.LookupPageByAlias(flagAlias)
		if err != nil {
			outputFail(ErrCodePageNotFound, fmt.Sprintf("alias %s not found: %v", flagAlias, err), nil)
		}
		innerPath := checkedInnerPath(name, version)
		conn, err := db.OpenOuter(outerPath, innerPath)
		if err != nil {
			outputFail(ErrCodeDBOpen, err.Error(), nil)
		}
		return name, version, pageID, conn
	}

	// 3. --roll: use default page for the named iroll
	if flagRoll != "" {
		pageID, err := store.GetDefaultPage(flagRoll)
		if err != nil {
			outputFail(ErrCodeInternal, err.Error(), nil)
		}
		if pageID == "" {
			outputFail(ErrCodeNoDefaultPage, fmt.Sprintf("no default page for iroll '%s'", flagRoll), []Hint{
				{Action: "Create a new page for this iroll", Cmd: fmt.Sprintf("logos page new %s", flagRoll)},
				{Action: "List all pages to find one to set as default", Cmd: "logos page list -a"},
			})
		}
		_, version, outerPath, err := store.LookupPageByID(pageID)
		if err != nil {
			outputFail(ErrCodePageNotFound, fmt.Sprintf("default page %s gone: %v", pageID, err), nil)
		}
		innerPath := checkedInnerPath(flagRoll, version)
		conn, err := db.OpenOuter(outerPath, innerPath)
		if err != nil {
			outputFail(ErrCodeDBOpen, err.Error(), nil)
		}
		return flagRoll, version, pageID, conn
	}

	// 4. Positional arg (iroll name): use default page for that iroll
	if len(args) > 0 {
		name, version, err := builder.ParseTag(args[0])
		if err != nil {
			outputFail(ErrCodeInvalidTag, fmt.Sprintf("invalid tag: %v", err), []Hint{
				{Action: "List all available iroll packages", Cmd: "logos status --list"},
			})
		}
		pageID, err := store.GetDefaultPage(name)
		if err != nil {
			outputFail(ErrCodeInternal, err.Error(), nil)
		}
		if pageID == "" {
			outputFail(ErrCodeNoDefaultPage, fmt.Sprintf("no default page for iroll '%s'", name), []Hint{
				{Action: "Create a new page and auto-set it as default", Cmd: fmt.Sprintf("logos page new %s", name)},
				{Action: "List all pages to find one to set as default", Cmd: "logos page list -a"},
			})
		}
		_, _, outerPath, err := store.LookupPageByID(pageID)
		if err != nil {
			outputFail(ErrCodePageNotFound, fmt.Sprintf("default page %s gone: %v", pageID, err), nil)
		}
		innerPath := checkedInnerPath(name, version)
		conn, err := db.OpenOuter(outerPath, innerPath)
		if err != nil {
			outputFail(ErrCodeDBOpen, err.Error(), nil)
		}
		return name, version, pageID, conn
	}

	// 5. Fallback: current cwd active page
	conn, irollName, irollVersion, pageID := openOuterFromActive(cwd)
	return irollName, irollVersion, pageID, conn
}

func init() {
	getContextCmd.Flags().StringVar(&getContextPage, "page", "", "Page ID")
	getContextCmd.Flags().StringVar(&getContextAlias, "alias", "", "Page alias")
	getContextCmd.Flags().StringVar(&getContextRoll, "roll", "", "iroll name (uses default page)")
	getContextCmd.Flags().StringVar(&getContextCwd, "cwd", ".", "Working directory")

	updateContextCmd.Flags().StringVar(&updateContextPage, "page", "", "Page ID")
	updateContextCmd.Flags().StringVar(&updateContextAlias, "alias", "", "Page alias")
	updateContextCmd.Flags().StringVar(&updateContextRoll, "roll", "", "iroll name (uses default page)")
	updateContextCmd.Flags().StringVar(&updateContextSetAlias, "set-alias", "", "Set page alias")
	updateContextCmd.Flags().StringVar(&updateContextContext, "content", "", "New context")
	updateContextCmd.Flags().StringVar(&updateContextCwd, "cwd", ".", "Working directory")

	pageCmd.AddCommand(getContextCmd)
	pageCmd.AddCommand(updateContextCmd)
}
