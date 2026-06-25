package cmd

import (
	"database/sql"
	"path/filepath"

	"logos/builder"
	"logos/db"
	"logos/store"

	"github.com/spf13/cobra"
)

var getContextPage string
var getContextCwd string
var updateContextPage string
var updateContextContext string
var updateContextCwd string

var getContextCmd = &cobra.Command{
	Use:   "get-context [name]",
	Short: "Get context by page id",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cwd, _ := filepath.Abs(getContextCwd)
		name, version, pageID := resolvePage(args, getContextPage, cwd)
		conn := openOuterForPage(name, version, cwd, args)
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
		name, version, pageID := resolvePage(args, updateContextPage, cwd)
		conn := openOuterForPage(name, version, cwd, args)
		defer conn.Close()

		p, err := db.UpdatePageContext(conn, pageID, updateContextContext)
		if err != nil {
			outputError(err.Error())
		}

		outputJSON(p)
	},
}

// openOuterForPage opens the outer db (with inner attached) for the given
// name/version. If args were provided (explicit tag), uses workspace default
// outer db; otherwise uses the active page's outer path from GetActive.
func openOuterForPage(name, version, cwd string, args []string) *sql.DB {
	if len(args) > 0 {
		outerPath, err := store.WorkspaceOuterDbPath(name, version)
		if err != nil {
			outputError(err.Error())
		}
		innerPath := checkedInnerPath(name, version)
		conn, err := db.OpenOuter(outerPath, innerPath)
		if err != nil {
			outputError(err.Error())
		}
		return conn
	}
	conn, _, _, _ := openOuterFromActive(cwd)
	return conn
}

// resolvePage returns (name, version, pageID) from args or active page for the cwd
func resolvePage(args []string, flagPage string, cwd string) (string, string, string) {
	if len(args) > 0 {
		name, version, err := builder.ParseTag(args[0])
		if err != nil {
			outputError(err.Error())
		}
		return name, version, flagPage
	}
	name, version, pageID, _, err := store.GetActive(cwd)
	if err != nil {
		outputError(err.Error())
	}
	if flagPage != "" {
		return name, version, flagPage
	}
	return name, version, pageID
}

func init() {
	getContextCmd.Flags().StringVar(&getContextPage, "page", "", "Page ID")
	getContextCmd.Flags().StringVar(&getContextCwd, "cwd", ".", "Working directory")

	updateContextCmd.Flags().StringVar(&updateContextPage, "page", "", "Page ID")
	updateContextCmd.Flags().StringVar(&updateContextContext, "content", "", "New context")
	updateContextCmd.Flags().StringVar(&updateContextCwd, "cwd", ".", "Working directory")
	updateContextCmd.MarkFlagRequired("content")

	pageCmd.AddCommand(getContextCmd)
	pageCmd.AddCommand(updateContextCmd)
}
