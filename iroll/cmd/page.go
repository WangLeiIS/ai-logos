package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"logos/builder"
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
		irollName, irollVersion, pageID, err := store.GetActive(cwd)
		if err != nil {
			outputError(err.Error())
		}

		conn, err := db.Open(checkedDbPath(irollName, irollVersion))
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

		name, version, err := builder.ParseTag(args[0])
		if err != nil {
			outputError(fmt.Sprintf("invalid tag: %v", err))
		}
		conn, err := db.Open(checkedDbPath(name, version))
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
	Use:   "new <iroll-name> [cwd]",
	Short: "Create a new page",
	Args:  cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		name, version, err := builder.ParseTag(args[0])
		if err != nil {
			outputError(fmt.Sprintf("invalid tag: %v", err))
		}
		cwd, err := resolvePageNewCwd(name, version, args)
		if err != nil {
			outputError(err.Error())
		}
		conn, err := db.Open(checkedDbPath(name, version))
		if err != nil {
			outputError(err.Error())
		}
		defer conn.Close()

		p, err := db.InsertPage(conn, cwd)
		if err != nil {
			outputError(err.Error())
		}

		if _, err := db.AutoStartLoopSeeds(conn, p.PageID); err != nil {
			outputError("auto-start loop seeds: " + err.Error())
		}

		if err := store.IndexPage(name, version, p.PageID, cwd); err != nil {
			outputError(err.Error())
		}

		p.Context, err = db.ResolveContext(p.Context, checkedIrollPath(name, version), conn, p.PageID)
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
		irollName, _, err := store.SwitchPage(pageID)
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

// resolvePageNewCwd determines the cwd for a new page based on priority:
// 1. --cwd flag (if explicitly set)
// 2. Second positional argument
// 3. Default workspace: ~/.iroll/<name>/<version>/workspace/
func resolvePageNewCwd(name, version string, args []string) (string, error) {
	// Priority 1: --cwd flag explicitly set
	if pageNewCwd != "" {
		return filepath.Abs(pageNewCwd)
	}
	// Priority 2: second positional argument
	if len(args) > 1 {
		return filepath.Abs(args[1])
	}
	// Priority 3: default workspace
	irollPath, err := store.IrollPath(name, version)
	if err != nil {
		return "", err
	}
	workspace := filepath.Join(irollPath, "workspace")
	if err := os.MkdirAll(workspace, 0755); err != nil {
		return "", err
	}
	return workspace, nil
}

func init() {
	pageListCmd.Flags().StringVar(&pageListCwd, "cwd", ".", "Working directory to filter by")
	pageListCmd.Flags().BoolVarP(&pageListAll, "all", "a", false, "List all pages across all directories")
	pageNewCmd.Flags().StringVar(&pageNewCwd, "cwd", "", "Working directory for the page")
	pageCurrentCmd.Flags().StringVar(&pageCurrentCwd, "cwd", ".", "Working directory")

	pageCmd.AddCommand(pageListCmd)
	pageCmd.AddCommand(pageNewCmd)
	pageCmd.AddCommand(pageSwitchCmd)
	pageCmd.AddCommand(pageCurrentCmd)
	pageCmd.AddCommand(pageDeleteCmd)
	pageCmd.AddCommand(queryDnaCmd)
	rootCmd.AddCommand(pageCmd)
}
