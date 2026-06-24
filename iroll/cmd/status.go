package cmd

import (
	"logos/store"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show system status",
	Run: func(cmd *cobra.Command, args []string) {
		home := store.HomeDir()

		sdb, err := store.OpenSystem()
		if err != nil {
			outputError(err.Error())
		}
		defer sdb.Close()

		var pageCount int
		sdb.QueryRow("SELECT COUNT(*) FROM page_index").Scan(&pageCount)

		rolls, _ := store.List()

		outputJSON(map[string]interface{}{
			"version":     Version,
			"home":        home,
			"iroll_count": len(rolls),
			"page_count":  pageCount,
			"rolls":       rolls,
		})
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
