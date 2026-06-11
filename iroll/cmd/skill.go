package cmd

import (
	"database/sql"
	"fmt"
	"path/filepath"

	"logos/db"
	"logos/skill"
	"logos/store"

	"github.com/spf13/cobra"
)

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "List and inspect registered skills",
}

var skillListCwd string

var skillListCmd = &cobra.Command{
	Use:   "list [name]",
	Short: "List registered skills",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name, err := resolveSkillRoll(skillListCwd, args)
		if err != nil {
			outputError(err.Error())
		}
		conn, err := openSkillDB(name)
		if err != nil {
			outputError(err.Error())
		}
		defer conn.Close()

		skills, err := db.ListSkills(conn)
		if err != nil {
			outputError(err.Error())
		}
		if skills == nil {
			skills = []skill.Skill{}
		}

		rollRoot, err := store.IrollPath(name)
		if err != nil {
			outputError(err.Error())
		}
		type skillEntry struct {
			Name        string  `json:"name"`
			Description string  `json:"description"`
			Weight      float64 `json:"weight"`
			AbsPath     string  `json:"abs_path"`
		}
		entries := make([]skillEntry, len(skills))
		for i, s := range skills {
			entries[i] = skillEntry{
				Name:        s.Name,
				Description: s.Description,
				Weight:      s.Weight,
				AbsPath:     filepath.Join(rollRoot, s.Path),
			}
		}
		outputJSON(entries)
	},
}

var skillShowCwd string

var skillShowCmd = &cobra.Command{
	Use:   "show <skill-name> [iroll-name]",
	Short: "Show a registered skill's metadata and file path",
	Args:  cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		name, err := resolveSkillRoll(skillShowCwd, args[1:])
		if err != nil {
			outputError(err.Error())
		}
		conn, err := openSkillDB(name)
		if err != nil {
			outputError(err.Error())
		}
		defer conn.Close()

		s, err := db.GetSkill(conn, args[0])
		if err != nil {
			outputError(err.Error())
		}

		rollRoot, err := store.IrollPath(name)
		if err != nil {
			outputError(err.Error())
		}

		type skillDetail struct {
			Name        string   `json:"name"`
			Description string   `json:"description"`
			Weight      float64  `json:"weight"`
			Path        string   `json:"path"`
			AbsPath     string   `json:"abs_path"`
			ArchivedAt  *string  `json:"archived_at,omitempty"`
			CreatedAt   string   `json:"created_at"`
			UpdatedAt   string   `json:"updated_at"`
		}
		outputJSON(skillDetail{
			Name:        s.Name,
			Description: s.Description,
			Weight:      s.Weight,
			Path:        s.Path,
			AbsPath:     filepath.Join(rollRoot, s.Path),
			ArchivedAt:  s.ArchivedAt,
			CreatedAt:   s.CreatedAt,
			UpdatedAt:   s.UpdatedAt,
		})
	},
}

func resolveSkillRoll(cwd string, names []string) (string, error) {
	if len(names) > 1 {
		return "", fmt.Errorf("at most one iroll name may be specified")
	}
	if len(names) == 1 {
		if _, err := store.IrollPath(names[0]); err != nil {
			return "", err
		}
		return names[0], nil
	}
	absoluteCwd, err := filepath.Abs(cwd)
	if err != nil {
		return "", fmt.Errorf("resolve cwd: %w", err)
	}
	name, _, err := store.GetActive(absoluteCwd)
	return name, err
}

func openSkillDB(name string) (*sql.DB, error) {
	path, err := store.DbPath(name)
	if err != nil {
		return nil, err
	}
	return db.Open(path)
}

func init() {
	skillListCmd.Flags().StringVar(&skillListCwd, "cwd", ".", "Working directory")
	skillShowCmd.Flags().StringVar(&skillShowCwd, "cwd", ".", "Working directory")
	skillCmd.AddCommand(skillListCmd, skillShowCmd)
	rootCmd.AddCommand(skillCmd)
}
