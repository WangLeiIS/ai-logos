package cmd

import (
	"fmt"
	"path/filepath"

	"logos/db"

	"github.com/spf13/cobra"
)

var queryDnaType string
var queryDnaCwd string

var queryDnaCmd = &cobra.Command{
	Use:   "query-dna <name-keyword>",
	Short: "Query dna entries by name (fuzzy match)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]

		cwd, _ := filepath.Abs(queryDnaCwd)
		conn, _, _, _ := openOuterFromActive(cwd)
		defer conn.Close()

		results, err := db.QueryDna(conn, name, queryDnaType)
		if err != nil {
			outputFail(ErrCodeInternal, err.Error(), nil)
		}

		if results == nil {
			results = []db.Dna{}
		}

		hints := []Hint{}
		if len(results) > 0 {
			hints = append(hints, Hint{
				Action: "Use a DNA answer in your page context",
				Cmd:    fmt.Sprintf("logos page set --page <page-id> dna_answer '{\"answer\":\"%s\"}'", results[0].Answer),
			})
		}
		hints = append(hints, Hint{
			Action: "Get the full page context including DNA",
			Cmd:    "logos page get",
		})
		outputOK(results, hints)
	},
}

func init() {
	queryDnaCmd.Flags().StringVar(&queryDnaType, "type", "", "Filter by DNA type")
	queryDnaCmd.Flags().StringVar(&queryDnaCwd, "cwd", ".", "Working directory")
}
