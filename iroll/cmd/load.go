package cmd

import (
	"logos/store"

	"github.com/spf13/cobra"
)

var loadCmd = &cobra.Command{
	Use:   "load <file-path>",
	Short: "Load a .iroll file into ~/.iroll/",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		zipPath := args[0]

		name, err := store.ReadName(zipPath)
		if err != nil {
			outputError(err.Error())
		}

		if err := store.Extract(zipPath, name); err != nil {
			outputError(err.Error())
		}

		outputJSON(map[string]string{
			"name": name,
			"path": checkedIrollPath(name),
		})
	},
}

func init() {
	rollCmd.AddCommand(loadCmd)
}
