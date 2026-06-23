package cmd

import (
	"logos/db"

	"github.com/spf13/cobra"
)

var inspectCmd = &cobra.Command{
	Use:   "inspect <name>",
	Short: "Show iroll details",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		conn, err := db.Open(checkedDbPath(name, "latest"))
		if err != nil {
			outputError(err.Error())
		}
		defer conn.Close()

		metadata, err := db.QueryAllMetadata(conn)
		if err != nil {
			outputError(err.Error())
		}

		tableStats, err := db.QueryTableStats(conn)
		if err != nil {
			outputError(err.Error())
		}

		resources, err := db.ListResources(name)
		if err != nil {
			resources = []string{}
		}

		outputJSON(map[string]interface{}{
			"name":      name,
			"metadata":  metadata,
			"tables":    tableStats,
			"resources": resources,
		})
	},
}

func init() {
	rollCmd.AddCommand(inspectCmd)
}
