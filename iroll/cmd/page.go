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
				outputFail(ErrCodeInternal, err.Error(), nil)
			}
			if pages == nil {
				pages = []map[string]interface{}{}
			}
			hints := []Hint{}
			if len(pages) > 0 {
				if pid, ok := pages[0]["page_id"].(string); ok {
					hints = append(hints, Hint{
						Action: "Get the full context for the first page listed",
						Cmd:    fmt.Sprintf("logos page get --page %s", pid),
					})
				}
			}
			hints = append(hints, Hint{
				Action: "Create a new page for a fresh context",
				Cmd:    "logos page new <iroll-name>",
			})
			outputOK(pages, hints)
			return
		}

		name, version, err := builder.ParseTag(args[0])
		if err != nil {
			outputFail(ErrCodeInvalidTag, fmt.Sprintf("invalid tag: %v", err), []Hint{
				{Action: "List all available iroll packages", Cmd: "logos status"},
			})
		}
		innerPath := checkedInnerPath(name, version)
		outerPath, err := store.WorkspaceOuterDbPath(name, version)
		if err != nil {
			outputFail(ErrCodeInternal, err.Error(), nil)
		}
		conn, err := db.OpenOuter(outerPath, innerPath)
		if err != nil {
			outputFail(ErrCodeDBOpen, err.Error(), nil)
		}
		defer conn.Close()

		var listCwd string
		if !pageListAll {
			listCwd, _ = filepath.Abs(pageListCwd)
		}
		pages, err := db.ListPagesByCwd(conn, listCwd)
		if err != nil {
			outputFail(ErrCodeInternal, err.Error(), nil)
		}

		briefs := make([]db.PageBrief, 0, len(pages))
		for _, p := range pages {
			briefs = append(briefs, db.PageBrief{
				PageID:    p.PageID,
				Cwd:       p.Cwd,
				Alias:     p.Alias,
				CreatedAt: p.CreatedAt,
			})
		}
		hints := []Hint{}
		if len(briefs) > 0 {
			hints = append(hints, Hint{
				Action: "Get the full context for the first page listed",
				Cmd:    fmt.Sprintf("logos page get --page %s", briefs[0].PageID),
			})
		}
		hints = append(hints, Hint{
			Action: "Create a new page for a fresh context",
			Cmd:    fmt.Sprintf("logos page new %s", args[0]),
		})
		outputOK(briefs, hints)
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
				{Action: "List all available iroll packages", Cmd: "logos status"},
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
			{Action: "Set an alias for this page, so you can reference it by name later", Cmd: fmt.Sprintf("logos page alias <name> --page %s", p.PageID)},
			{Action: "Get the full context including DNA, loops and system prompt", Cmd: fmt.Sprintf("logos page get --page %s", p.PageID)},
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
			outputFail(ErrCodePageNotFound, err.Error(), nil)
		}
		outputOK(map[string]string{
			"active":     "true",
			"iroll_name": irollName,
			"page_id":    pageID,
		}, []Hint{
			{Action: "Get the full context of the newly active page", Cmd: fmt.Sprintf("logos page get --page %s", pageID)},
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
			outputFail(ErrCodeInternal, err.Error(), nil)
		}
		outputOK(map[string]string{
			"deleted": "true",
			"page_id": pageID,
		}, []Hint{
			{Action: "List remaining pages", Cmd: "logos page list -a"},
			{Action: "Create a new page for a fresh context", Cmd: "logos page new <iroll-name>"},
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
				outputFail(ErrCodePageNotFound, err.Error(), nil)
			}
			if err := store.SetDefaultPage(name, pageID); err != nil {
				outputFail(ErrCodeInternal, err.Error(), nil)
			}
			outputOK(map[string]string{
				"status":  "ok",
				"message": fmt.Sprintf("default page for '%s' set to %s", name, pageID),
			}, []Hint{
				{Action: "Get the full context of the new default page", Cmd: fmt.Sprintf("logos page get --page %s", pageID)},
			})
			return
		}

		// Show or clear
		if pageDefaultClear && pageDefaultRoll != "" {
			if err := store.ClearDefaultPage(pageDefaultRoll); err != nil {
				outputFail(ErrCodeInternal, err.Error(), nil)
			}
			outputOK(map[string]string{
				"status":  "ok",
				"message": fmt.Sprintf("default page for '%s' cleared", pageDefaultRoll),
			}, []Hint{
				{Action: "Set a new default page", Cmd: "logos page default <page-id>"},
			})
			return
		}

		if pageDefaultRoll != "" {
			pageID, err := store.GetDefaultPage(pageDefaultRoll)
			if err != nil {
				outputFail(ErrCodeInternal, err.Error(), nil)
			}
			if pageID == "" {
				outputOK(map[string]string{
					"iroll":        pageDefaultRoll,
					"default_page": "",
				}, []Hint{
					{Action: "Create a new page and auto-set it as default", Cmd: fmt.Sprintf("logos page new %s", pageDefaultRoll)},
					{Action: "List all pages to find one to set as default", Cmd: "logos page list -a"},
				})
				return
			}
			outputOK(map[string]string{
				"iroll":        pageDefaultRoll,
				"default_page": pageID,
			}, []Hint{
				{Action: "Get the full context of the default page", Cmd: fmt.Sprintf("logos page get --page %s", pageID)},
				{Action: "Clear the default page setting", Cmd: fmt.Sprintf("logos page default --roll %s --clear", pageDefaultRoll)},
			})
			return
		}

		outputFail(ErrCodeInternal, "usage: logos page default <page-id>  OR  logos page default --roll <name> [--clear]", nil)
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
	pageDefaultCmd.Flags().StringVar(&pageDefaultRoll, "roll", "", "iroll name")
	pageDefaultCmd.Flags().BoolVar(&pageDefaultClear, "clear", false, "Clear the default page")

	pageCmd.AddCommand(pageListCmd)
	pageCmd.AddCommand(pageNewCmd)
	pageCmd.AddCommand(pageSwitchCmd)
	pageCmd.AddCommand(pageDeleteCmd)
	pageCmd.AddCommand(pageDefaultCmd)
	pageCmd.AddCommand(queryDnaCmd)
	rootCmd.AddCommand(pageCmd)
}
