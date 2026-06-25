package cmd

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"

	"logos/db"
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

func checkedIrollPath(name string, version string) string {
	path, err := store.IrollPath(name, version)
	if err != nil {
		outputError(err.Error())
	}
	return path
}

// Deprecated: use checkedInnerPath or openOuterFromActive.
func checkedDbPath(name string, version string) string {
	return checkedInnerPath(name, version)
}

func checkedInnerPath(name, version string) string {
	path, err := store.InnerDbPath(name, version)
	if err != nil {
		outputError(err.Error())
	}
	return path
}

// openOuterFromActive gets the active page's outer db path from system.db,
// opens it with inner attached, and returns the connection + metadata.
func openOuterFromActive(cwd string) (*sql.DB, string, string, string) {
	irollName, irollVersion, pageID, outerDbPath, err := store.GetActive(cwd)
	if err != nil {
		outputError(err.Error())
	}
	innerPath := checkedInnerPath(irollName, irollVersion)
	conn, err := db.OpenOuter(outerDbPath, innerPath)
	if err != nil {
		outputError(err.Error())
	}
	return conn, irollName, irollVersion, pageID
}
