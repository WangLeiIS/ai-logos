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

// Hint is a structured suggestion for the next command.
type Hint struct {
	Action string `json:"action"`
	Cmd    string `json:"cmd"`
}

// Error codes for structured output.
const (
	ErrCodeInvalidTag    = "invalid_tag"
	ErrCodeIrollNotFound = "iroll_not_found"
	ErrCodeNoDefaultPage = "no_default_page"
	ErrCodePageNotFound  = "page_not_found"
	ErrCodeDBOpen        = "db_open_failed"
	ErrCodeInternal      = "internal"
	ErrCodeNoActivePage  = "no_active_page"
	ErrCodeKeyNotFound   = "key_not_found"
)

func jsonLine(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// outputOK prints success JSON to stdout: status line, optional data line, optional hints line.
// data and hints are optional (nil/empty = line skipped). Exits on marshal error.
func outputOK(data interface{}, hints []Hint) {
	fmt.Println(jsonLine(map[string]string{"status": "ok"}))
	if data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			outputFail(ErrCodeInternal, fmt.Sprintf("JSON marshal error: %v", err), nil)
		}
		fmt.Println(string(b))
	}
	if len(hints) > 0 {
		fmt.Println(jsonLine(map[string]interface{}{"hints": hints}))
	}
}

// outputFail prints error JSON to stdout (error line + optional hints line), then os.Exit(1).
// hints is optional (nil/empty = line skipped).
func outputFail(code, errMsg string, hints []Hint) {
	fmt.Println(jsonLine(map[string]string{
		"status": "error",
		"code":   code,
		"error":  errMsg,
	}))
	if len(hints) > 0 {
		fmt.Println(jsonLine(map[string]interface{}{"hints": hints}))
	}
	os.Exit(1)
}

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
		outputFail(ErrCodeInternal, err.Error(), nil)
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
		outputFail(ErrCodeInternal, err.Error(), nil)
	}
	return path
}

// openOuterFromActive gets the active page's outer db path from system.db,
// opens it with inner attached, and returns the connection + metadata.
func openOuterFromActive(cwd string) (*sql.DB, string, string, string) {
	irollName, irollVersion, pageID, outerDbPath, err := store.GetActive(cwd)
	if err != nil {
		outputFail(ErrCodeNoActivePage, err.Error(), []Hint{
			{Action: "Create a new page and auto-set it as the active page for this directory", Cmd: "logos page new <iroll-name>"},
			{Action: "List all pages to find an existing one", Cmd: "logos page list -a"},
		})
	}
	innerPath := checkedInnerPath(irollName, irollVersion)
	conn, err := db.OpenOuter(outerDbPath, innerPath)
	if err != nil {
		outputFail(ErrCodeDBOpen, err.Error(), nil)
	}
	return conn, irollName, irollVersion, pageID
}
