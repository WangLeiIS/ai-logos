package cmd

import (
	"database/sql"
	"fmt"
	"path/filepath"

	"logos/db"
	"logos/store"

	"github.com/spf13/cobra"
)

var loopCmd = newLoopCmd()

func newLoopCmd() *cobra.Command {
	command := &cobra.Command{
		Use:   "loop",
		Short: "Manage loop seeds and autonomous runs",
	}
	command.AddCommand(
		newLoopListCmd(outputLoopList),
		newLoopInspectCmd(outputLoopInspect),
		newLoopAddCmd(outputLoopAdd),
		newLoopEditCmd(outputLoopEdit),
		newLoopRemoveCmd(outputLoopRemove),
		newLoopArchiveCmd(outputLoopArchive),
		newLoopRestoreCmd(outputLoopRestore),
		newLoopRunCmd(outputLoopRun),
		newLoopUpdateCmd(outputLoopUpdate),
		newLoopCompleteCmd(outputLoopComplete),
		newLoopAbortCmd(outputLoopAbort),
		newLoopReflectCmd(outputLoopReflect),
		newLoopCurrentCmd(outputLoopCurrent),
		newLoopHistoryCmd(outputLoopHistory),
		newLoopShowCmd(outputLoopShow),
	)
	return command
}

func openActiveLoop(cwd string) (string, string, *sql.DB, error) {
	absoluteCwd, err := filepath.Abs(cwd)
	if err != nil {
		return "", "", nil, fmt.Errorf("resolve cwd: %w", err)
	}
	name, pageID, err := store.GetActive(absoluteCwd)
	if err != nil {
		return "", "", nil, err
	}
	dbPath, err := store.DbPath(name)
	if err != nil {
		return "", "", nil, err
	}
	conn, err := db.Open(dbPath)
	if err != nil {
		return "", "", nil, err
	}
	return name, pageID, conn, nil
}

func init() {
	rootCmd.AddCommand(loopCmd)
}
