package cmd

import (
	"logos/builder"

	"github.com/spf13/cobra"
)

var buildFile string
var buildTag string

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build an iroll from a Layerfile",
	Run: func(cmd *cobra.Command, args []string) {
		lf, err := builder.ParseLayerfile(buildFile)
		if err != nil {
			outputError(err.Error())
		}

		result, err := builder.Build(lf, buildTag)
		if err != nil {
			outputError(err.Error())
		}

		outputJSON(result)
	},
}

func init() {
	buildCmd.Flags().StringVarP(&buildFile, "file", "f", "Layerfile", "Layerfile path")
	buildCmd.Flags().StringVarP(&buildTag, "tag", "t", "", "Output iroll name")
	buildCmd.MarkFlagRequired("tag")

	rollCmd.AddCommand(buildCmd)
}
