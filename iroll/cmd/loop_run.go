package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"logos/db"

	"github.com/spf13/cobra"
)

func newLoopPsCmd(run func(string, bool) error) *cobra.Command {
	var cwd string
	var all bool
	command := &cobra.Command{
		Use:   "ps",
		Short: "List loop runs for the current page",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			defer resetLoopSeedFlags(cmd)
			return run(cwd, all)
		},
	}
	command.Flags().StringVar(&cwd, "cwd", ".", "Working directory")
	command.Flags().BoolVarP(&all, "all", "a", false, "Show all runs including completed/aborted")
	isolateLoopSeedCommand(command)
	return command
}

func parseLoopRunID(value string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid loop run ID %q", value)
	}
	return id, nil
}

func optionalLoopRunID(args []string) (*int64, error) {
	if len(args) == 0 {
		return nil, nil
	}
	id, err := parseLoopRunID(args[0])
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func newLoopRunCmd(run func(string, string, *int64, string) error) *cobra.Command {
	var cwd, parent, plan string
	command := &cobra.Command{
		Use:   "run <name>",
		Short: "Start an autonomous loop run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			defer resetLoopSeedFlags(cmd)
			var parentID *int64
			if strings.TrimSpace(parent) != "" {
				id, err := parseLoopRunID(parent)
				if err != nil {
					return err
				}
				parentID = &id
			}
			return run(cwd, args[0], parentID, plan)
		},
	}
	command.Flags().StringVar(&parent, "parent", "", "Active main run ID")
	command.Flags().StringVar(&plan, "plan", "null", "Initial plan as JSON or text")
	command.Flags().StringVar(&cwd, "cwd", ".", "Working directory")
	isolateLoopSeedCommand(command)
	return command
}

func newLoopUpdateCmd(run func(string, *int64, *string, *string) error) *cobra.Command {
	var cwd, plan, progress string
	command := &cobra.Command{
		Use:   "update [run-id]",
		Short: "Update an active loop run",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			defer resetLoopSeedFlags(cmd)
			runID, err := optionalLoopRunID(args)
			if err != nil {
				return err
			}
			var planValue, progressValue *string
			if cmd.Flags().Changed("plan") {
				planValue = &plan
			}
			if cmd.Flags().Changed("progress") {
				progressValue = &progress
			}
			return run(cwd, runID, planValue, progressValue)
		},
	}
	command.Flags().StringVar(&plan, "plan", "", "Replacement plan as JSON or text")
	command.Flags().StringVar(&progress, "progress", "", "Replacement progress as JSON or text")
	command.Flags().StringVar(&cwd, "cwd", ".", "Working directory")
	isolateLoopSeedCommand(command)
	return command
}

func newLoopCompleteCmd(run func(string, *int64, string) error) *cobra.Command {
	var cwd, result string
	command := &cobra.Command{
		Use:   "complete [run-id]",
		Short: "Complete an active loop run",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			defer resetLoopSeedFlags(cmd)
			runID, err := optionalLoopRunID(args)
			if err != nil {
				return err
			}
			return run(cwd, runID, result)
		},
	}
	command.Flags().StringVar(&result, "result", "", "Final result as JSON or text")
	command.Flags().StringVar(&cwd, "cwd", ".", "Working directory")
	_ = command.MarkFlagRequired("result")
	isolateLoopSeedCommand(command)
	return command
}

func newLoopAbortCmd(run func(string, *int64, string, string) error) *cobra.Command {
	var cwd, reason, result string
	command := &cobra.Command{
		Use:   "abort [run-id]",
		Short: "Abort an active loop run",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			defer resetLoopSeedFlags(cmd)
			runID, err := optionalLoopRunID(args)
			if err != nil {
				return err
			}
			return run(cwd, runID, reason, result)
		},
	}
	command.Flags().StringVar(&reason, "reason", "", "Abort reason")
	command.Flags().StringVar(&result, "result", "", "Optional result as JSON or text")
	command.Flags().StringVar(&cwd, "cwd", ".", "Working directory")
	_ = command.MarkFlagRequired("reason")
	isolateLoopSeedCommand(command)
	return command
}

func newLoopReflectCmd(run func(string, int64, string) error) *cobra.Command {
	var cwd, content string
	command := &cobra.Command{
		Use:   "reflect <run-id>",
		Short: "Reflect on an ended loop run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			defer resetLoopSeedFlags(cmd)
			runID, err := parseLoopRunID(args[0])
			if err != nil {
				return err
			}
			return run(cwd, runID, content)
		},
	}
	command.Flags().StringVar(&content, "content", "", "Reflection as JSON or text")
	command.Flags().StringVar(&cwd, "cwd", ".", "Working directory")
	_ = command.MarkFlagRequired("content")
	isolateLoopSeedCommand(command)
	return command
}

func newLoopHistoryCmd(run func(string, string, string, int) error) *cobra.Command {
	var cwd, pageID string
	var limit int
	command := &cobra.Command{
		Use:   "history <name>",
		Short: "Show loop run history",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			defer resetLoopSeedFlags(cmd)
			return run(cwd, args[0], pageID, limit)
		},
	}
	command.Flags().StringVar(&pageID, "page", "", "Filter by page ID")
	command.Flags().IntVar(&limit, "limit", 50, "Maximum runs to return")
	command.Flags().StringVar(&cwd, "cwd", ".", "Working directory")
	isolateLoopSeedCommand(command)
	return command
}

func newLoopShowCmd(run func(string, int64) error) *cobra.Command {
	var cwd string
	command := &cobra.Command{
		Use:   "show <run-id>",
		Short: "Show a loop run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			defer resetLoopSeedFlags(cmd)
			runID, err := parseLoopRunID(args[0])
			if err != nil {
				return err
			}
			return run(cwd, runID)
		},
	}
	command.Flags().StringVar(&cwd, "cwd", ".", "Working directory")
	isolateLoopSeedCommand(command)
	return command
}

func outputLoopRun(cwd, seedName string, parentRunID *int64, plan string) error {
	run, err := runLoopStart(cwd, seedName, parentRunID, plan)
	if err != nil {
		outputFail(ErrCodeInternal, err.Error(), nil)
	}
	outputOK(run, []Hint{
		{Action: "Get the latest loop run details", Cmd: "logos loop ps"},
		{Action: "Get the page context with active loop focus", Cmd: "logos page get-context"},
	})
	return nil
}

func outputLoopUpdate(cwd string, runID *int64, plan, progress *string) error {
	run, err := runLoopUpdate(cwd, runID, plan, progress)
	if err != nil {
		outputFail(ErrCodeInternal, err.Error(), nil)
	}
	outputOK(run, []Hint{
		{Action: "Get the page context with active loop focus", Cmd: "logos page get-context"},
	})
	return nil
}

func outputLoopComplete(cwd string, runID *int64, result string) error {
	run, err := runLoopComplete(cwd, runID, result)
	if err != nil {
		outputFail(ErrCodeInternal, err.Error(), nil)
	}
	outputOK(run, []Hint{
		{Action: "Reflect on the completed run", Cmd: fmt.Sprintf("logos loop reflect %d --content <reflection>", run.ID)},
		{Action: "List all loop runs", Cmd: "logos loop ps -a"},
		{Action: "Start a new loop run", Cmd: "logos loop run <seed-name>"},
	})
	return nil
}

func outputLoopAbort(cwd string, runID *int64, reason, result string) error {
	run, err := runLoopAbort(cwd, runID, reason, result)
	if err != nil {
		outputFail(ErrCodeInternal, err.Error(), nil)
	}
	outputOK(run, []Hint{
		{Action: "Reflect on the aborted run", Cmd: fmt.Sprintf("logos loop reflect %d --content <reflection>", run.ID)},
		{Action: "Start a new loop run with a different seed", Cmd: "logos loop run <seed-name>"},
		{Action: "List all loop seeds", Cmd: "logos loop list"},
	})
	return nil
}

func outputLoopReflect(cwd string, runID int64, content string) error {
	run, err := runLoopReflect(cwd, runID, content)
	if err != nil {
		outputFail(ErrCodeInternal, err.Error(), nil)
	}
	outputOK(run, []Hint{
		{Action: "View run history for this seed", Cmd: "logos loop history <seed-name>"},
		{Action: "List all loop runs", Cmd: "logos loop ps -a"},
	})
	return nil
}

func outputLoopPs(cwd string, all bool) error {
	runs, err := runLoopPs(cwd, all)
	if err != nil {
		outputFail(ErrCodeInternal, err.Error(), nil)
	}
	if runs == nil {
		runs = []db.LoopRun{}
	}
	outputOK(runs, []Hint{
		{Action: "Start a new loop run from a seed", Cmd: "logos loop run <seed-name>"},
		{Action: "List available loop seeds", Cmd: "logos loop list"},
	})
	return nil
}

func runLoopPs(cwd string, all bool) ([]db.LoopRun, error) {
	_, pageID, conn, err := openActiveLoop(cwd)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if all {
		return db.ListAllRuns(conn, pageID)
	}
	return db.ListActiveRuns(conn, pageID)
}

func outputLoopHistory(cwd, seedName, pageID string, limit int) error {
	runs, err := runLoopHistory(cwd, seedName, pageID, limit)
	if err != nil {
		outputFail(ErrCodeInternal, err.Error(), nil)
	}
	outputOK(runs, []Hint{
		{Action: "Start a new loop run from this seed", Cmd: fmt.Sprintf("logos loop run %s", seedName)},
		{Action: "Inspect the loop seed", Cmd: fmt.Sprintf("logos loop inspect %s", seedName)},
	})
	return nil
}

func outputLoopShow(cwd string, runID int64) error {
	run, err := runLoopShow(cwd, runID)
	if err != nil {
		outputFail(ErrCodeInternal, err.Error(), nil)
	}
	outputOK(run, []Hint{
		{Action: "Update this run's plan or progress", Cmd: fmt.Sprintf("logos loop update %d --plan <json>", runID)},
		{Action: "Complete this run with a result", Cmd: fmt.Sprintf("logos loop complete %d --result <json>", runID)},
	})
	return nil
}

func runLoopStart(cwd, seedName string, parentRunID *int64, plan string) (*db.LoopRun, error) {
	_, pageID, conn, err := openActiveLoop(cwd)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return db.StartLoopRun(conn, pageID, seedName, parentRunID, plan)
}

func runLoopUpdate(cwd string, runID *int64, plan, progress *string) (*db.LoopRun, error) {
	_, pageID, conn, err := openActiveLoop(cwd)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return db.UpdateLoopRun(conn, pageID, runID, plan, progress)
}

func runLoopComplete(cwd string, runID *int64, result string) (*db.LoopRun, error) {
	_, pageID, conn, err := openActiveLoop(cwd)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return db.CompleteLoopRun(conn, pageID, runID, result)
}

func runLoopAbort(cwd string, runID *int64, reason, result string) (*db.LoopRun, error) {
	_, pageID, conn, err := openActiveLoop(cwd)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return db.AbortLoopRun(conn, pageID, runID, reason, result)
}

func runLoopReflect(cwd string, runID int64, content string) (*db.LoopRun, error) {
	_, _, conn, err := openActiveLoop(cwd)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return db.ReflectLoopRun(conn, runID, content)
}

func runLoopHistory(cwd, seedName, pageID string, limit int) ([]db.LoopRun, error) {
	_, _, conn, err := openActiveLoop(cwd)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return db.ListLoopHistory(conn, seedName, pageID, limit)
}

func runLoopShow(cwd string, runID int64) (*db.LoopRun, error) {
	_, _, conn, err := openActiveLoop(cwd)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return db.GetLoopRun(conn, runID)
}
