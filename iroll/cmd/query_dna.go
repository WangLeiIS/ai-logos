package cmd

import (
	"path/filepath"

	"logos/db"
	"logos/store"

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
		irollName, _, err := store.GetActive(cwd)
		if err != nil {
			outputError(err.Error())
		}

		conn, err := db.Open(store.DbPath(irollName))
		if err != nil {
			outputError(err.Error())
		}
		defer conn.Close()

		results, err := db.QueryDna(conn, name, queryDnaType)
		if err != nil {
			outputError(err.Error())
		}

		if results == nil {
			results = []db.Dna{}
		}
		outputJSON(results)
	},
}

func init() {
	queryDnaCmd.Flags().StringVar(&queryDnaType, "type", "", "Filter by type (认知观/伦理观/审美观/本体观)")
	queryDnaCmd.Flags().StringVar(&queryDnaCwd, "cwd", ".", "Working directory")
}
