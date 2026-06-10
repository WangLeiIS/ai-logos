package cmd

import (
	"logos/db"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func newLoopListCmd(run func(string, bool) error) *cobra.Command {
	var cwd string
	var includeArchived bool
	command := &cobra.Command{
		Use:   "list",
		Short: "List loop seeds",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			defer resetLoopSeedFlags(cmd)
			return run(cwd, includeArchived)
		},
	}
	command.Flags().BoolVar(&includeArchived, "archived", false, "Include archived loop seeds")
	command.Flags().StringVar(&cwd, "cwd", ".", "Working directory")
	isolateLoopSeedCommand(command)
	return command
}

func newLoopInspectCmd(run func(string, string) error) *cobra.Command {
	var cwd string
	command := &cobra.Command{
		Use:   "inspect <name>",
		Short: "Inspect a loop seed",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			defer resetLoopSeedFlags(cmd)
			return run(cwd, args[0])
		},
	}
	command.Flags().StringVar(&cwd, "cwd", ".", "Working directory")
	isolateLoopSeedCommand(command)
	return command
}

func newLoopAddCmd(run func(string, string, string, string, float64) error) *cobra.Command {
	var cwd string
	var describe string
	var content string
	var weight float64
	command := &cobra.Command{
		Use:   "add <name>",
		Short: "Add a loop seed",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			defer resetLoopSeedFlags(cmd)
			return run(cwd, args[0], describe, content, weight)
		},
	}
	command.Flags().StringVar(&describe, "describe", "", "Loop seed description")
	command.Flags().StringVar(&content, "content", "", "Loop seed content")
	command.Flags().Float64Var(&weight, "weight", 0.5, "Loop seed weight")
	command.Flags().StringVar(&cwd, "cwd", ".", "Working directory")
	_ = command.MarkFlagRequired("describe")
	_ = command.MarkFlagRequired("content")
	isolateLoopSeedCommand(command)
	return command
}

func newLoopEditCmd(run func(string, string, db.LoopSeedPatch) error) *cobra.Command {
	var cwd string
	var describe string
	var content string
	var weight float64
	command := &cobra.Command{
		Use:   "edit <name>",
		Short: "Edit a loop seed",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			defer resetLoopSeedFlags(cmd)
			return run(cwd, args[0], loopSeedPatchFromFlags(cmd, describe, content, weight))
		},
	}
	command.Flags().StringVar(&describe, "describe", "", "Loop seed description")
	command.Flags().StringVar(&content, "content", "", "Loop seed content")
	command.Flags().Float64Var(&weight, "weight", 0.5, "Loop seed weight")
	command.Flags().StringVar(&cwd, "cwd", ".", "Working directory")
	isolateLoopSeedCommand(command)
	return command
}

func newLoopRemoveCmd(run func(string, string) error) *cobra.Command {
	var cwd string
	command := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a loop seed without run history",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			defer resetLoopSeedFlags(cmd)
			return run(cwd, args[0])
		},
	}
	command.Flags().StringVar(&cwd, "cwd", ".", "Working directory")
	isolateLoopSeedCommand(command)
	return command
}

func newLoopArchiveCmd(run func(string, string) error) *cobra.Command {
	var cwd string
	command := &cobra.Command{
		Use:   "archive <name>",
		Short: "Archive a loop seed",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			defer resetLoopSeedFlags(cmd)
			return run(cwd, args[0])
		},
	}
	command.Flags().StringVar(&cwd, "cwd", ".", "Working directory")
	isolateLoopSeedCommand(command)
	return command
}

func newLoopRestoreCmd(run func(string, string) error) *cobra.Command {
	var cwd string
	command := &cobra.Command{
		Use:   "restore <name>",
		Short: "Restore an archived loop seed",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			defer resetLoopSeedFlags(cmd)
			return run(cwd, args[0])
		},
	}
	command.Flags().StringVar(&cwd, "cwd", ".", "Working directory")
	isolateLoopSeedCommand(command)
	return command
}

func outputLoopList(cwd string, includeArchived bool) error {
	seeds, err := runLoopList(cwd, includeArchived)
	if err != nil {
		outputError(err.Error())
	}
	outputJSON(seeds)
	return nil
}

func outputLoopInspect(cwd, name string) error {
	seed, err := runLoopInspect(cwd, name)
	if err != nil {
		outputError(err.Error())
	}
	outputJSON(seed)
	return nil
}

func outputLoopAdd(cwd, name, describe, content string, weight float64) error {
	seed, err := runLoopAdd(cwd, name, describe, content, weight)
	if err != nil {
		outputError(err.Error())
	}
	outputJSON(seed)
	return nil
}

func outputLoopEdit(cwd, name string, patch db.LoopSeedPatch) error {
	seed, err := runLoopEdit(cwd, name, patch)
	if err != nil {
		outputError(err.Error())
	}
	outputJSON(seed)
	return nil
}

func outputLoopRemove(cwd, name string) error {
	if err := runLoopRemove(cwd, name); err != nil {
		outputError(err.Error())
	}
	outputJSON(map[string]string{"removed": name})
	return nil
}

func outputLoopArchive(cwd, name string) error {
	seed, err := runLoopArchive(cwd, name)
	if err != nil {
		outputError(err.Error())
	}
	outputJSON(seed)
	return nil
}

func outputLoopRestore(cwd, name string) error {
	seed, err := runLoopRestore(cwd, name)
	if err != nil {
		outputError(err.Error())
	}
	outputJSON(seed)
	return nil
}

func runLoopList(cwd string, includeArchived bool) ([]db.LoopSeed, error) {
	_, _, conn, err := openActiveLoop(cwd)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return db.ListLoopSeeds(conn, includeArchived)
}

func runLoopInspect(cwd, name string) (*db.LoopSeed, error) {
	_, _, conn, err := openActiveLoop(cwd)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return db.GetLoopSeedByName(conn, name)
}

func runLoopAdd(cwd, name, describe, content string, weight float64) (*db.LoopSeed, error) {
	_, _, conn, err := openActiveLoop(cwd)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return db.InsertLoopSeed(conn, name, describe, content, weight)
}

func runLoopEdit(cwd, name string, patch db.LoopSeedPatch) (*db.LoopSeed, error) {
	_, _, conn, err := openActiveLoop(cwd)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return db.UpdateLoopSeed(conn, name, patch)
}

func runLoopRemove(cwd, name string) error {
	_, _, conn, err := openActiveLoop(cwd)
	if err != nil {
		return err
	}
	defer conn.Close()
	return db.RemoveLoopSeed(conn, name)
}

func runLoopArchive(cwd, name string) (*db.LoopSeed, error) {
	_, _, conn, err := openActiveLoop(cwd)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return db.ArchiveLoopSeed(conn, name)
}

func runLoopRestore(cwd, name string) (*db.LoopSeed, error) {
	_, _, conn, err := openActiveLoop(cwd)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return db.RestoreLoopSeed(conn, name)
}

func loopSeedPatchFromFlags(cmd *cobra.Command, describe, content string, weight float64) db.LoopSeedPatch {
	patch := db.LoopSeedPatch{}
	if cmd.Flags().Changed("describe") {
		patch.Describe = &describe
	}
	if cmd.Flags().Changed("content") {
		patch.Content = &content
	}
	if cmd.Flags().Changed("weight") {
		patch.Weight = &weight
	}
	return patch
}

func resetLoopSeedFlags(cmd *cobra.Command) {
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		_ = flag.Value.Set(flag.DefValue)
		flag.Changed = false
	})
}

func isolateLoopSeedCommand(cmd *cobra.Command) {
	validateArgs := cmd.Args
	cmd.Args = func(cmd *cobra.Command, args []string) error {
		if err := validateArgs(cmd, args); err != nil {
			resetLoopSeedFlags(cmd)
			return err
		}
		if err := cmd.ValidateRequiredFlags(); err != nil {
			resetLoopSeedFlags(cmd)
			return err
		}
		return nil
	}
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		resetLoopSeedFlags(cmd)
		return err
	})
	help := cmd.HelpFunc()
	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		defer resetLoopSeedFlags(cmd)
		help(cmd, args)
	})
}
