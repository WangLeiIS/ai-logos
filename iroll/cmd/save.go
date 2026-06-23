package cmd

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var saveOutput string

var saveCmd = &cobra.Command{
	Use:   "save <name>",
	Short: "Save an iroll to a .iroll file",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		srcDir := checkedIrollPath(name, "latest")

		if _, err := os.Stat(srcDir); os.IsNotExist(err) {
			outputError("iroll '" + name + "' not found")
		}

		if saveOutput == "" {
			saveOutput = name + ".iroll"
		}

		if err := packToZip(srcDir, saveOutput); err != nil {
			outputError(err.Error())
		}

		absPath, _ := filepath.Abs(saveOutput)
		outputJSON(map[string]string{
			"name":  name,
			"saved": absPath,
		})
	},
}

func packToZip(srcDir string, zipPath string) error {
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	w := zip.NewWriter(zipFile)
	defer w.Close()

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, _ := filepath.Rel(srcDir, path)
		if rel == "." {
			return nil
		}

		if info.IsDir() {
			_, err := w.Create(rel + "/")
			return err
		}

		wr, err := w.Create(rel)
		if err != nil {
			return err
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		_, err = io.Copy(wr, f)
		return err
	})
}

func init() {
	saveCmd.Flags().StringVarP(&saveOutput, "output", "o", "", "Output file path (default: <name>.iroll)")
	rollCmd.AddCommand(saveCmd)
}
