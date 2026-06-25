package cmd

import (
	"fmt"
	"io"
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
		conn, irollName, _, pageID := openOuterFromActive(cwd)
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
		innerPath := checkedInnerPath(name, version)
		outerPath, err := store.WorkspaceOuterDbPath(name, version)
		if err != nil {
			outputError(err.Error())
		}
		conn, err := db.OpenOuter(outerPath, innerPath)
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
			outputFail(ErrCodeInvalidTag, fmt.Sprintf("invalid tag: %v", err), []Hint{
				{Action: "List all available iroll packages", Cmd: "logos status --list"},
				{Action: "Build an iroll from an Irollfile", Cmd: "logos roll build -f <Irollfile> -t <name>"},
			})
		}
		cwd, outerPath, err := resolvePageNewCwd(name, version, args)
		if err != nil {
			outputFail(ErrCodeInternal, err.Error(), nil)
		}
		innerPath := checkedInnerPath(name, version)

		// Copy outer template if not exists
		if _, err := os.Stat(outerPath); os.IsNotExist(err) {
			templateOuter := filepath.Join(checkedIrollPath(name, version), "roll-outer.db")
			if err := copyFile(templateOuter, outerPath); err != nil {
				outputFail(ErrCodeInternal, fmt.Sprintf("copy outer db template: %v", err), nil)
			}
		}

		conn, err := db.OpenOuter(outerPath, innerPath)
		if err != nil {
			outputFail(ErrCodeDBOpen, err.Error(), nil)
		}
		defer conn.Close()

		p, err := db.InsertPage(conn, cwd)
		if err != nil {
			outputFail(ErrCodeInternal, err.Error(), nil)
		}

		if _, err := db.AutoStartLoopSeeds(conn, p.PageID); err != nil {
			outputFail(ErrCodeInternal, "auto-start loop seeds: "+err.Error(), nil)
		}

		if err := store.IndexPage(name, version, p.PageID, cwd, outerPath, ""); err != nil {
			outputFail(ErrCodeInternal, err.Error(), nil)
		}

		// Auto-set as default page when using workspace (no explicit cwd)
		if pageNewCwd == "" && len(args) < 2 {
			if err := store.SetDefaultPage(name, p.PageID); err != nil {
				outputFail(ErrCodeInternal, "set default page: "+err.Error(), nil)
			}
		}

		brief := &db.PageBrief{
			PageID:    p.PageID,
			Cwd:       p.Cwd,
			Alias:     p.Alias,
			CreatedAt: p.CreatedAt,
		}

		hints := []Hint{
			{Action: "Set an alias for this page, so you can reference it by name later", Cmd: fmt.Sprintf("logos page update-context --page %s --set-alias <name>", p.PageID)},
			{Action: "Get the full context including DNA, loops and system prompt", Cmd: fmt.Sprintf("logos page get-context --page %s", p.PageID)},
		}

		outputOK(brief, hints)
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

var pageDefaultRoll string
var pageDefaultClear bool

var pageDefaultCmd = &cobra.Command{
	Use:   "default [page-id]",
	Short: "Set or show the default page for an iroll",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) > 0 {
			// Set: logos page default <page-id>
			pageID := args[0]
			// Look up the iroll name for this page
			name, _, _, err := store.LookupPageByID(pageID)
			if err != nil {
				outputError(err.Error())
			}
			if err := store.SetDefaultPage(name, pageID); err != nil {
				outputError(err.Error())
			}
			outputJSON(map[string]string{
				"status":  "ok",
				"message": fmt.Sprintf("default page for '%s' set to %s", name, pageID),
			})
			return
		}

		// Show or clear
		if pageDefaultClear && pageDefaultRoll != "" {
			if err := store.ClearDefaultPage(pageDefaultRoll); err != nil {
				outputError(err.Error())
			}
			outputJSON(map[string]string{
				"status":  "ok",
				"message": fmt.Sprintf("default page for '%s' cleared", pageDefaultRoll),
			})
			return
		}

		if pageDefaultRoll != "" {
			pageID, err := store.GetDefaultPage(pageDefaultRoll)
			if err != nil {
				outputError(err.Error())
			}
			if pageID == "" {
				outputJSON(map[string]string{
					"iroll":        pageDefaultRoll,
					"default_page": "",
				})
				return
			}
			outputJSON(map[string]string{
				"iroll":        pageDefaultRoll,
				"default_page": pageID,
			})
			return
		}

		outputError("usage: logos page default <page-id>  OR  logos page default --roll <name> [--clear]")
	},
}

// resolvePageNewCwd determines the cwd for a new page based on priority:
// 1. --cwd flag (if explicitly set)
// 2. Second positional argument
// 3. Default workspace: ~/.iroll/<name>/<version>/workspace/
// Returns (cwd, outerDbPath, error).
func resolvePageNewCwd(name, version string, args []string) (string, string, error) {
	// Priority 1: --cwd flag explicitly set
	if pageNewCwd != "" {
		absCwd, err := filepath.Abs(pageNewCwd)
		if err != nil {
			return "", "", err
		}
		outerPath, err := store.CwdOuterDbPath(absCwd, name)
		if err != nil {
			return "", "", err
		}
		// Ensure .iroll directory exists
		if err := os.MkdirAll(filepath.Dir(outerPath), 0755); err != nil {
			return "", "", err
		}
		return absCwd, outerPath, nil
	}
	// Priority 2: second positional argument
	if len(args) > 1 {
		absCwd, err := filepath.Abs(args[1])
		if err != nil {
			return "", "", err
		}
		outerPath, err := store.CwdOuterDbPath(absCwd, name)
		if err != nil {
			return "", "", err
		}
		if err := os.MkdirAll(filepath.Dir(outerPath), 0755); err != nil {
			return "", "", err
		}
		return absCwd, outerPath, nil
	}
	// Priority 3: default workspace
	irollPath, err := store.IrollPath(name, version)
	if err != nil {
		return "", "", err
	}
	workspace := filepath.Join(irollPath, "workspace")
	if err := os.MkdirAll(workspace, 0755); err != nil {
		return "", "", err
	}
	outerPath, err := store.WorkspaceOuterDbPath(name, version)
	if err != nil {
		return "", "", err
	}
	return workspace, outerPath, nil
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

func init() {
	pageListCmd.Flags().StringVar(&pageListCwd, "cwd", ".", "Working directory to filter by")
	pageListCmd.Flags().BoolVarP(&pageListAll, "all", "a", false, "List all pages across all directories")
	pageNewCmd.Flags().StringVar(&pageNewCwd, "cwd", "", "Working directory for the page")
	pageCurrentCmd.Flags().StringVar(&pageCurrentCwd, "cwd", ".", "Working directory")

	pageDefaultCmd.Flags().StringVar(&pageDefaultRoll, "roll", "", "iroll name")
	pageDefaultCmd.Flags().BoolVar(&pageDefaultClear, "clear", false, "Clear the default page")

	pageCmd.AddCommand(pageListCmd)
	pageCmd.AddCommand(pageNewCmd)
	pageCmd.AddCommand(pageSwitchCmd)
	pageCmd.AddCommand(pageCurrentCmd)
	pageCmd.AddCommand(pageDeleteCmd)
	pageCmd.AddCommand(pageDefaultCmd)
	pageCmd.AddCommand(queryDnaCmd)
	rootCmd.AddCommand(pageCmd)
}
