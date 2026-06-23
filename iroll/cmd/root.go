package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"logos/store"

	"github.com/spf13/cobra"
)

// Version is set at build time via ldflags.
// Default "dev" means unversioned development build.
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:   "logos",
	Short: "logos - AI agent memory manager",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func outputJSON(v interface{}) {
	b, err := json.Marshal(v)
	if err != nil {
		outputError(fmt.Sprintf("JSON marshal error: %v", err))
	}
	fmt.Println(string(b))
}

func outputError(msg string) {
	b, _ := json.Marshal(map[string]string{"error": msg})
	fmt.Fprintln(os.Stderr, string(b))
	os.Exit(1)
}

func checkedIrollPath(name string) string {
	path, err := store.IrollPath(name)
	if err != nil {
		outputError(err.Error())
	}
	return path
}

func checkedDbPath(name string) string {
	path, err := store.DbPath(name)
	if err != nil {
		outputError(err.Error())
	}
	return path
}
