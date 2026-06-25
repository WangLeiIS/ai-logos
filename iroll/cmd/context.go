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
			outputError(err.Error())
		}

		p.Context, err = db.ResolveContext(p.Context, checkedIrollPath(name, version), conn, p.PageID)
		if err != nil {
			outputError(err.Error())
		}

		outputJSON(p)
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
			outputError("at least one of --content or --set-alias is required")
		}

		// Handle --set-alias
		if hasSetAlias {
			// Update alias in page_index (system.db)
			if err := store.SetPageAlias(pageID, updateContextSetAlias); err != nil {
				outputError(err.Error())
			}
			// Also update alias in the iroll pages table
			if err := db.UpdatePageAlias(conn, pageID, updateContextSetAlias); err != nil {
				outputError(err.Error())
			}
		}

		if hasContent {
			p, err := db.UpdatePageContext(conn, pageID, updateContextContext)
			if err != nil {
				outputError(err.Error())
			}
			outputJSON(p)
			return
		}

		// Only alias was set, return current page
		p, err := db.GetPageByPageID(conn, pageID)
		if err != nil {
			outputError(err.Error())
		}
		outputJSON(p)
	},
}

// resolvePageContext resolves args/flags into a db connection with attached inner db.
// Priority: --page > --alias > --roll > positional arg > current cwd.
// Returns (name, version, pageID, conn).
func resolvePageContext(args []string, flagPage, flagAlias, flagRoll, cwd string) (string, string, string, *sql.DB) {
	// 1. --page: look up by page_id
	if flagPage != "" {
		name, version, outerPath, err := store.LookupPageByID(flagPage)
		if err != nil {
			outputError(err.Error())
		}
		innerPath := checkedInnerPath(name, version)
		conn, err := db.OpenOuter(outerPath, innerPath)
		if err != nil {
			outputError(err.Error())
		}
		return name, version, flagPage, conn
	}

	// 2. --alias: look up by alias
	if flagAlias != "" {
		name, version, pageID, outerPath, err := store.LookupPageByAlias(flagAlias)
		if err != nil {
			outputError(err.Error())
		}
		innerPath := checkedInnerPath(name, version)
		conn, err := db.OpenOuter(outerPath, innerPath)
		if err != nil {
			outputError(err.Error())
		}
		return name, version, pageID, conn
	}

	// 3. --roll: use default page for the named iroll
	if flagRoll != "" {
		pageID, err := store.GetDefaultPage(flagRoll)
		if err != nil {
			outputError(err.Error())
		}
		if pageID == "" {
			outputError(fmt.Sprintf("no default page for iroll '%s'", flagRoll))
		}
		_, version, outerPath, err := store.LookupPageByID(pageID)
		if err != nil {
			outputError(err.Error())
		}
		innerPath := checkedInnerPath(flagRoll, version)
		conn, err := db.OpenOuter(outerPath, innerPath)
		if err != nil {
			outputError(err.Error())
		}
		return flagRoll, version, pageID, conn
	}

	// 4. Positional arg (iroll name): use default page for that iroll
	if len(args) > 0 {
		name, version, err := builder.ParseTag(args[0])
		if err != nil {
			outputError(err.Error())
		}
		pageID, err := store.GetDefaultPage(name)
		if err != nil {
			outputError(err.Error())
		}
		if pageID == "" {
			errorMsg := fmt.Sprintf("no default page for iroll '%s', run 'logos page default <page-id>' or 'logos page new %s .'", name, name)
			outputError(errorMsg)
		}
		_, _, outerPath, err := store.LookupPageByID(pageID)
		if err != nil {
			outputError(err.Error())
		}
		innerPath := checkedInnerPath(name, version)
		conn, err := db.OpenOuter(outerPath, innerPath)
		if err != nil {
			outputError(err.Error())
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
