package cmd

import (
	"path/filepath"

	"logos/db"
	"logos/store"

	"github.com/spf13/cobra"
)

var addMemoryContent string
var addMemoryImportance float64
var addMemoryCwd string

var addMemoryCmd = &cobra.Command{
	Use:   "add-memory [name]",
	Short: "Add a memory",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var name string
		if len(args) > 0 {
			name = args[0]
		} else {
			cwd, _ := filepath.Abs(addMemoryCwd)
			var err error
			name, _, err = store.GetActive(cwd)
			if err != nil {
				outputError(err.Error())
			}
		}

		conn, err := db.Open(checkedDbPath(name))
		if err != nil {
			outputError(err.Error())
		}
		defer conn.Close()

		mem, err := db.InsertMemory(conn, addMemoryContent, addMemoryImportance)
		if err != nil {
			outputError(err.Error())
		}

		outputJSON(mem)
	},
}

func init() {
	addMemoryCmd.Flags().StringVar(&addMemoryContent, "content", "", "Memory content")
	addMemoryCmd.MarkFlagRequired("content")
	addMemoryCmd.Flags().Float64Var(&addMemoryImportance, "importance", 0.5, "Importance (0.0-1.0)")
	addMemoryCmd.Flags().StringVar(&addMemoryCwd, "cwd", ".", "Working directory")

	pageCmd.AddCommand(addMemoryCmd)
}
