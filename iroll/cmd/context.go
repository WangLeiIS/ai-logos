package cmd

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"logos/builder"
	"logos/db"
	"logos/store"

	"github.com/spf13/cobra"
)

// pageTarget holds the shared page-targeting flags used by page subcommands.
type pageTarget struct {
	page, alias, roll, cwd string
}

func (t *pageTarget) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&t.page, "page", "", "Page ID")
	cmd.Flags().StringVar(&t.alias, "alias", "", "Page alias")
	cmd.Flags().StringVar(&t.roll, "roll", "", "iroll name (uses default page)")
	cmd.Flags().StringVar(&t.cwd, "cwd", ".", "Working directory")
}

var (
	pageGetTarget   pageTarget
	pageSetTarget   pageTarget
	pageUnsetTarget pageTarget
	pageAliasTarget pageTarget
)

var pageSetContent string
var pageAliasClear bool

// pageBrief converts a full Page into a lightweight PageBrief (no context),
// encouraging callers to use `page get` when they need the context body.
func pageBrief(p *db.Page) db.PageBrief {
	return db.PageBrief{
		PageID:    p.PageID,
		Cwd:       p.Cwd,
		Alias:     p.Alias,
		CreatedAt: p.CreatedAt,
	}
}

var pageGetCmd = &cobra.Command{
	Use:   "get [path]",
	Short: "Get page context (full, or a single resolved key by dot-path)",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cwd, _ := filepath.Abs(pageGetTarget.cwd)
		name, version, pageID, conn := resolvePageContext(nil, pageGetTarget.page, pageGetTarget.alias, pageGetTarget.roll, cwd)
		defer conn.Close()

		if len(args) == 0 {
			p, err := db.GetPageByPageID(conn, pageID)
			if err != nil {
				outputFail(ErrCodePageNotFound, err.Error(), nil)
			}
			resolved, err := db.ResolveContext(p.Context, checkedIrollPath(name, version), conn, pageID)
			if err != nil {
				outputFail(ErrCodeInternal, err.Error(), nil)
			}
			outputOK(json.RawMessage(resolved), contextFollowupHints(p))
			return
		}

		val, err := db.GetContextKey(conn, pageID, args[0], checkedIrollPath(name, version))
		if err != nil {
			if errors.Is(err, db.ErrContextKeyNotFound) {
				outputFail(ErrCodeKeyNotFound, err.Error(), nil)
			}
			outputFail(ErrCodeInternal, err.Error(), nil)
		}
		outputOK(val, nil)
	},
}

var pageSetCmd = &cobra.Command{
	Use:   "set [path] [value]",
	Short: "Set a context key (json-or-text value), or replace the whole context with --content",
	Args:  cobra.MaximumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		cwd, _ := filepath.Abs(pageSetTarget.cwd)
		_, _, pageID, conn := resolvePageContext(nil, pageSetTarget.page, pageSetTarget.alias, pageSetTarget.roll, cwd)
		defer conn.Close()

		if cmd.Flags().Changed("content") {
			if len(args) > 0 {
				outputFail(ErrCodeInternal, "--content cannot be combined with path/value arguments", nil)
			}
			p, err := db.UpdatePageContext(conn, pageID, pageSetContent)
			if err != nil {
				outputFail(ErrCodeInternal, err.Error(), nil)
			}
			outputOK(pageBrief(p), contextFollowupHints(p))
			return
		}

		if len(args) != 2 {
			outputFail(ErrCodeInternal, "usage: logos page set <path> <value>  (or: logos page set --content '<json>')", nil)
		}
		if err := db.SetContextKey(conn, pageID, args[0], args[1]); err != nil {
			outputFail(ErrCodeInternal, err.Error(), nil)
		}
		p, err := db.GetPageByPageID(conn, pageID)
		if err != nil {
			outputFail(ErrCodeInternal, err.Error(), nil)
		}
		outputOK(pageBrief(p), contextFollowupHints(p))
	},
}

var pageUnsetCmd = &cobra.Command{
	Use:   "unset <path>",
	Short: "Delete a context key by dot-path",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cwd, _ := filepath.Abs(pageUnsetTarget.cwd)
		_, _, pageID, conn := resolvePageContext(nil, pageUnsetTarget.page, pageUnsetTarget.alias, pageUnsetTarget.roll, cwd)
		defer conn.Close()

		if err := db.UnsetContextKey(conn, pageID, args[0]); err != nil {
			if errors.Is(err, db.ErrContextKeyNotFound) {
				outputFail(ErrCodeKeyNotFound, err.Error(), nil)
			}
			outputFail(ErrCodeInternal, err.Error(), nil)
		}
		p, err := db.GetPageByPageID(conn, pageID)
		if err != nil {
			outputFail(ErrCodeInternal, err.Error(), nil)
		}
		outputOK(pageBrief(p), contextFollowupHints(p))
	},
}

var pageAliasCmd = &cobra.Command{
	Use:   "alias [name]",
	Short: "Set or clear the page alias",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cwd, _ := filepath.Abs(pageAliasTarget.cwd)
		_, _, pageID, conn := resolvePageContext(nil, pageAliasTarget.page, pageAliasTarget.alias, pageAliasTarget.roll, cwd)
		defer conn.Close()

		var alias string
		if pageAliasClear {
			alias = ""
		} else {
			if len(args) != 1 {
				outputFail(ErrCodeInternal, "usage: logos page alias <name>  (or: logos page alias --clear)", nil)
			}
			alias = args[0]
		}
		if err := store.SetPageAlias(pageID, alias); err != nil {
			outputFail(ErrCodeInternal, err.Error(), nil)
		}
		if err := db.UpdatePageAlias(conn, pageID, alias); err != nil {
			outputFail(ErrCodeInternal, err.Error(), nil)
		}
		p, err := db.GetPageByPageID(conn, pageID)
		if err != nil {
			outputFail(ErrCodeInternal, err.Error(), nil)
		}
		outputOK(pageBrief(p), contextFollowupHints(p))
	},
}

// contextFollowupHints suggests common next steps after a context read/write.
func contextFollowupHints(p *db.Page) []Hint {
	hints := []Hint{
		{Action: "Read the full resolved context", Cmd: fmt.Sprintf("logos page get --page %s", p.PageID)},
		{Action: "Update a single context key", Cmd: "logos page set <path> <value>"},
	}
	if p.Alias == "" {
		hints = append(hints, Hint{
			Action: "Set an alias to reference this page by name",
			Cmd:    fmt.Sprintf("logos page alias <name> --page %s", p.PageID),
		})
	}
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
				{Action: "List all available iroll packages", Cmd: "logos status"},
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
	pageGetTarget.bind(pageGetCmd)
	pageSetTarget.bind(pageSetCmd)
	pageSetCmd.Flags().StringVar(&pageSetContent, "content", "", "Replace the whole context with this JSON")
	pageUnsetTarget.bind(pageUnsetCmd)
	pageAliasTarget.bind(pageAliasCmd)
	pageAliasCmd.Flags().BoolVar(&pageAliasClear, "clear", false, "Clear the alias")

	pageCmd.AddCommand(pageGetCmd)
	pageCmd.AddCommand(pageSetCmd)
	pageCmd.AddCommand(pageUnsetCmd)
	pageCmd.AddCommand(pageAliasCmd)
}
