package cmd

import (
	"path/filepath"

	"logos/db"
	"logos/store"

	"github.com/spf13/cobra"
)

var pageCmd = &cobra.Command{
	Use:   "page",
	Short: "Manage pages",
}

var pageCurrentCwd string

var pageCurrentCmd = &cobra.Command{
	Use:   "current",
	Short: "Show current active page",
	Run: func(cmd *cobra.Command, args []string) {
		cwd, _ := filepath.Abs(pageCurrentCwd)
		irollName, pageID, err := store.GetActive(cwd)
		if err != nil {
			outputError(err.Error())
		}

		conn, err := db.Open(checkedDbPath(irollName))
		if err != nil {
			outputError(err.Error())
		}
		defer conn.Close()

		p, err := db.GetPageByPageID(conn, pageID)
		if err != nil {
			outputError(err.Error())
		}

		outputJSON(map[string]interface{}{
			"iroll_name": irollName,
			"page_id":    pageID,
			"cwd":        p.Cwd,
			"context":    p.Context,
			"updated_at": p.UpdatedAt,
		})
	},
}

var pageListCwd string
var pageListAll bool
var pageNewCwd string

var pageListCmd = &cobra.Command{
	Use:   "list [name]",
	Short: "List pages",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			var cwd string
			if !pageListAll {
				cwd, _ = filepath.Abs(pageListCwd)
			}
			pages, err := store.ListAllPages(cwd)
			if err != nil {
				outputError(err.Error())
			}
			if pages == nil {
				pages = []map[string]interface{}{}
			}
			outputJSON(pages)
			return
		}

		name := args[0]
		conn, err := db.Open(checkedDbPath(name))
		if err != nil {
			outputError(err.Error())
		}
		defer conn.Close()

		var listCwd string
		if !pageListAll {
			listCwd, _ = filepath.Abs(pageListCwd)
		}
		pages, err := db.ListPagesByCwd(conn, listCwd)
		if err != nil {
			outputError(err.Error())
		}

		if pages == nil {
			pages = []db.Page{}
		}
		outputJSON(pages)
	},
}

var pageNewCmd = &cobra.Command{
	Use:   "new <name>",
	Short: "Create a new page",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		cwd, _ := filepath.Abs(pageNewCwd)
		conn, err := db.Open(checkedDbPath(name))
		if err != nil {
			outputError(err.Error())
		}
		defer conn.Close()

		p, err := db.InsertPage(conn, cwd)
		if err != nil {
			outputError(err.Error())
		}

		if err := store.IndexPage(name, p.PageID, cwd); err != nil {
			outputError(err.Error())
		}

		p.Context, err = db.ResolveContext(p.Context, checkedIrollPath(name), conn, p.PageID)
		if err != nil {
			outputError(err.Error())
		}

		outputJSON(p)
	},
}

var pageSwitchCmd = &cobra.Command{
	Use:   "switch <page-id>",
	Short: "Switch active page",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		pageID := args[0]
		irollName, err := store.SwitchPage(pageID)
		if err != nil {
			outputError(err.Error())
		}

		outputJSON(map[string]string{
			"active":     "true",
			"iroll_name": irollName,
			"page_id":    pageID,
		})
	},
}

var pageDeleteCmd = &cobra.Command{
	Use:   "delete <page-id>",
	Short: "Delete a page",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		pageID := args[0]
		if err := store.DeletePage(pageID); err != nil {
			outputError(err.Error())
		}

		outputJSON(map[string]string{
			"deleted": "true",
			"page_id": pageID,
		})
	},
}

func init() {
	pageListCmd.Flags().StringVar(&pageListCwd, "cwd", ".", "Working directory to filter by")
	pageListCmd.Flags().BoolVarP(&pageListAll, "all", "a", false, "List all pages across all directories")
	pageNewCmd.Flags().StringVar(&pageNewCwd, "cwd", ".", "Working directory for the page")
	pageCurrentCmd.Flags().StringVar(&pageCurrentCwd, "cwd", ".", "Working directory")

	pageCmd.AddCommand(pageListCmd)
	pageCmd.AddCommand(pageNewCmd)
	pageCmd.AddCommand(pageSwitchCmd)
	pageCmd.AddCommand(pageCurrentCmd)
	pageCmd.AddCommand(pageDeleteCmd)
	pageCmd.AddCommand(queryDnaCmd)
	rootCmd.AddCommand(pageCmd)
}
