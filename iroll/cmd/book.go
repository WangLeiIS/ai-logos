package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"

	"logos/book"
	"logos/db"
	"logos/store"

	"github.com/spf13/cobra"
)

var bookCmd = &cobra.Command{
	Use:   "book",
	Short: "List, inspect, and query books",
}

var bookListCwd string

var bookListCmd = &cobra.Command{
	Use:   "list [name]",
	Short: "List books registered in an iroll",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		result, err := runBookList(bookListCwd, args)
		if err != nil {
			outputError(err.Error())
		}
		outputJSON(result)
	},
}

var bookInspectCwd string

var bookInspectCmd = &cobra.Command{
	Use:   "inspect <book-id> [name]",
	Short: "Inspect a registered book",
	Args:  cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		result, err := runBookInspect(bookInspectCwd, args[0], args[1:])
		if err != nil {
			outputError(err.Error())
		}
		outputJSON(result)
	},
}

var bookQueryBooks []string
var bookQueryTags []string
var bookQueryLimit int
var bookQueryPerBookLimit int
var bookQueryCwd string

var bookQueryCmd = &cobra.Command{
	Use:   "query",
	Short: "Query registered books by exact tags",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		result, err := runBookQuery(cmd.Context(), bookQueryCwd, book.Query{
			Books: bookQueryBooks, Tags: bookQueryTags, Limit: bookQueryLimit, PerBookLimit: bookQueryPerBookLimit,
		})
		if err != nil {
			outputError(err.Error())
		}
		outputJSON(result)
	},
}

func runBookList(cwd string, names []string) ([]book.Book, error) {
	name, err := resolveBookRoll(cwd, names)
	if err != nil {
		return nil, err
	}
	conn, err := openBookDB(name)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return db.ListBooks(conn)
}

func runBookInspect(cwd, bookID string, names []string) (*book.Book, error) {
	name, err := resolveBookRoll(cwd, names)
	if err != nil {
		return nil, err
	}
	conn, err := openBookDB(name)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return db.GetBook(conn, bookID)
}

func runBookQuery(ctx context.Context, cwd string, query book.Query) (*book.QueryResponse, error) {
	if err := validateBookQuery(query); err != nil {
		return nil, err
	}
	name, err := resolveBookRoll(cwd, nil)
	if err != nil {
		return nil, err
	}
	conn, err := openBookDB(name)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	registered, err := db.ListBooks(conn)
	if err != nil {
		return nil, err
	}
	rollRoot, err := store.IrollPath(name)
	if err != nil {
		return nil, err
	}
	return book.QueryBooks(ctx, rollRoot, registered, query)
}

func validateBookQuery(query book.Query) error {
	if len(query.Books) == 0 {
		return fmt.Errorf("at least one book is required")
	}
	if _, err := book.NormalizeTags(query.Tags); err != nil {
		return err
	}
	if query.Limit <= 0 {
		return fmt.Errorf("query limit must be positive")
	}
	if query.PerBookLimit <= 0 {
		return fmt.Errorf("per-book limit must be positive")
	}
	return nil
}

func resolveBookRoll(cwd string, names []string) (string, error) {
	if len(names) > 1 {
		return "", fmt.Errorf("at most one iroll name may be specified")
	}
	if len(names) == 1 {
		if _, err := store.IrollPath(names[0]); err != nil {
			return "", err
		}
		return names[0], nil
	}
	absoluteCwd, err := filepath.Abs(cwd)
	if err != nil {
		return "", fmt.Errorf("resolve cwd: %w", err)
	}
	name, _, err := store.GetActive(absoluteCwd)
	return name, err
}

func openBookDB(name string) (*sql.DB, error) {
	path, err := store.DbPath(name)
	if err != nil {
		return nil, err
	}
	return db.Open(path)
}

func init() {
	bookListCmd.Flags().StringVar(&bookListCwd, "cwd", ".", "Working directory")
	bookInspectCmd.Flags().StringVar(&bookInspectCwd, "cwd", ".", "Working directory")
	bookQueryCmd.Flags().StringArrayVar(&bookQueryBooks, "book", nil, "Book ID to query (repeatable)")
	bookQueryCmd.Flags().StringArrayVar(&bookQueryTags, "tag", nil, "Exact query tag (repeatable)")
	bookQueryCmd.Flags().IntVar(&bookQueryLimit, "limit", 10, "Maximum merged results")
	bookQueryCmd.Flags().IntVar(&bookQueryPerBookLimit, "per-book-limit", 5, "Maximum results per book")
	bookQueryCmd.Flags().StringVar(&bookQueryCwd, "cwd", ".", "Working directory")
	bookCmd.AddCommand(bookListCmd, bookInspectCmd, bookQueryCmd)
	rootCmd.AddCommand(bookCmd)
}
