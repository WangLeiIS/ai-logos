package cmd

import (
	"github.com/spf13/cobra"
)

var rollCmd = &cobra.Command{
	Use:   "roll",
	Short: "Manage iroll packages",
}

func init() {
	rootCmd.AddCommand(rollCmd)
}
