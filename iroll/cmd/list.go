package cmd

import (
	"logos/store"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all loaded irolls",
	Run: func(cmd *cobra.Command, args []string) {
		names, err := store.List()
		if err != nil {
			outputError(err.Error())
		}

		result := make([]map[string]string, len(names))
		for i, n := range names {
			result[i] = map[string]string{"name": n}
		}

		if len(result) == 0 {
			result = []map[string]string{}
		}
		outputJSON(result)
	},
}

func init() {
	rollCmd.AddCommand(listCmd)
}
