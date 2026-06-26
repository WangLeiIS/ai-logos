package cmd

import (
	"path/filepath"

	"logos/db"

	"github.com/spf13/cobra"
)

var queryMemoryKeyword string
var queryMemoryMinImportance float64
var queryMemorySince string
var queryMemoryBefore string
var queryMemoryLimit int
var queryMemoryFull bool
var queryMemoryCwd string

var queryMemoryCmd = &cobra.Command{
	Use:   "query-memory [name]",
	Short: "Query memories",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cwd, _ := filepath.Abs(queryMemoryCwd)
		conn, _, _, pageID := openOuterFromActive(cwd)
		defer conn.Close()

		params := db.QueryMemoryParams{
			MinImportance: queryMemoryMinImportance,
			Since:         queryMemorySince,
			Before:        queryMemoryBefore,
			Limit:         queryMemoryLimit,
		}
		if len(args) > 0 {
			params.Name = args[0]
		} else if queryMemoryKeyword != "" {
			params.Keyword = queryMemoryKeyword
		}

		results, err := db.QueryMemory(conn, pageID, params)
		if err != nil {
			outputFail(ErrCodeInternal, err.Error(), nil)
		}

		if queryMemoryFull {
			if results == nil {
				results = []db.Memory{}
			}
			hints := []Hint{
				{Action: "Get the full page context", Cmd: "logos page get"},
				{Action: "Query memory with a different keyword", Cmd: "logos page query-memory --keyword <keyword>"},
			}
			outputOK(results, hints)
		} else {
			summaries := make([]db.MemorySummary, len(results))
			for i, m := range results {
				summaries[i] = db.MemorySummary{
					Name:       m.Name,
					Question:   m.Question,
					ContentLen: len(m.Content),
					SleepCount: m.SleepCount,
				}
			}
			if summaries == nil {
				summaries = []db.MemorySummary{}
			}
			hints := []Hint{
				{Action: "Get the full page context", Cmd: "logos page get"},
				{Action: "Query memory with a keyword for full content", Cmd: "logos page query-memory --keyword <keyword> --full"},
			}
			outputOK(summaries, hints)
		}
	},
}

func init() {
	queryMemoryCmd.Flags().StringVar(&queryMemoryKeyword, "keyword", "", "Search keyword (matches name and question)")
	queryMemoryCmd.Flags().Float64Var(&queryMemoryMinImportance, "min-importance", 0, "Minimum importance (0.0-1.0)")
	queryMemoryCmd.Flags().StringVar(&queryMemorySince, "since", "", "Return memories after this ISO timestamp")
	queryMemoryCmd.Flags().StringVar(&queryMemoryBefore, "before", "", "Return memories before this ISO timestamp")
	queryMemoryCmd.Flags().IntVar(&queryMemoryLimit, "limit", 20, "Maximum results (1-100)")
	queryMemoryCmd.Flags().BoolVar(&queryMemoryFull, "full", false, "Return full records including content")
	queryMemoryCmd.Flags().StringVar(&queryMemoryCwd, "cwd", ".", "Working directory")

	pageCmd.AddCommand(queryMemoryCmd)
}
