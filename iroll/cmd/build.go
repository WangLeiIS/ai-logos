package cmd

import (
	"fmt"
	"logos/builder"

	"github.com/spf13/cobra"
)

var buildFile string
var buildTag string

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build an iroll from an Irollfile",
	Run: func(cmd *cobra.Command, args []string) {
		lf, err := builder.ParseIrollfile(buildFile)
		if err != nil {
			outputError(err.Error())
		}

		name, version, err := builder.ParseTag(buildTag)
		if err != nil {
			outputError(fmt.Sprintf("invalid tag: %v", err))
		}

		result, err := builder.Build(lf, name, version)
		if err != nil {
			outputError(err.Error())
		}

		outputJSON(result)
	},
}

func init() {
	buildCmd.Flags().StringVarP(&buildFile, "file", "f", "Irollfile", "Irollfile path")
	buildCmd.Flags().StringVarP(&buildTag, "tag", "t", "", "Output name[:version] (default version: latest)")
	buildCmd.MarkFlagRequired("tag")

	rollCmd.AddCommand(buildCmd)
}
