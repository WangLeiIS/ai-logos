package cmd

import (
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
		// TODO: rewrite in Task 4
	},
}

func init() {
	addMemoryCmd.Flags().StringVar(&addMemoryContent, "content", "", "Memory content")
	addMemoryCmd.MarkFlagRequired("content")
	addMemoryCmd.Flags().Float64Var(&addMemoryImportance, "importance", 0.5, "Importance (0.0-1.0)")
	addMemoryCmd.Flags().StringVar(&addMemoryCwd, "cwd", ".", "Working directory")

	pageCmd.AddCommand(addMemoryCmd)
}
