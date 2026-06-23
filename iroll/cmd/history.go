package cmd

import (
	"logos/db"

	"github.com/spf13/cobra"
)

var historyCmd = &cobra.Command{
	Use:   "history <name>",
	Short: "Show build history",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		conn, err := db.Open(checkedDbPath(name, "latest"))
		if err != nil {
			outputError(err.Error())
		}
		defer conn.Close()

		entries, err := db.QueryHistory(conn)
		if err != nil {
			outputError(err.Error())
		}

		if entries == nil {
			entries = []db.HistoryEntry{}
		}
		outputJSON(entries)
	},
}

func init() {
	rollCmd.AddCommand(historyCmd)
}
