package cmd

import (
	"fmt"
	"os"

	"logos/builder"
	"logos/store"

	"github.com/spf13/cobra"
)

var rmCmd = &cobra.Command{
	Use:   "rm <name>",
	Short: "Remove an iroll package",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name, version, err := builder.ParseTag(args[0])
		if err != nil {
			outputError(fmt.Sprintf("invalid tag: %v", err))
		}
		path := checkedIrollPath(name, version)

		if _, err := os.Stat(path); os.IsNotExist(err) {
			outputError("iroll '" + name + "' not found")
		}

		if err := os.RemoveAll(path); err != nil {
			outputError(err.Error())
		}

		// Clean up system.db references
		store.CleanIndex(name)

		outputJSON(map[string]string{
			"removed": name,
			"path":    path,
		})
	},
}

func init() {
	rollCmd.AddCommand(rmCmd)
}
