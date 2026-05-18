package cli

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/gndm/schedule-containers/internal/config"
	"github.com/gndm/schedule-containers/internal/models"
	"github.com/gndm/schedule-containers/internal/scheduler"
	"github.com/gndm/schedule-containers/internal/store"
	"github.com/gndm/schedule-containers/internal/yamlconfig"
)

var scheduleCmd = &cobra.Command{
	Use:   "schedule",
	Short: "Manage schedules",
}

var scheduleListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all schedules",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			slog.Error("failed to load config", "error", err)
			os.Exit(1)
		}

		db, err := store.Open(cfg.DBPath)
		if err != nil {
			slog.Error("failed to open database", "error", err)
			os.Exit(1)
		}
		defer db.Close()

		schedules, err := db.ListSchedules()
		if err != nil {
			slog.Error("failed to list schedules", "error", err)
			os.Exit(1)
		}

		outputJSON, _ := cmd.Flags().GetBool("json")
		if outputJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			enc.Encode(schedules)
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tCONTAINER\tSTART\tSTOP\tENABLED")
		for _, s := range schedules {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%v\n", s.ID[:8], s.ContainerName, s.StartCron, s.StopCron, s.Enabled)
		}
		w.Flush()
	},
}

var scheduleAddCmd = &cobra.Command{
	Use:   "add <container> <cron-start> <cron-stop>",
	Short: "Add a schedule",
	Args:  cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			slog.Error("failed to load config", "error", err)
			os.Exit(1)
		}

		db, err := store.Open(cfg.DBPath)
		if err != nil {
			slog.Error("failed to open database", "error", err)
			os.Exit(1)
		}
		defer db.Close()

		if err := scheduler.ValidateCronExpression(args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "invalid start cron expression: %v\n", err)
			os.Exit(1)
		}
		if err := scheduler.ValidateCronExpression(args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "invalid stop cron expression: %v\n", err)
			os.Exit(1)
		}

		displayName, _ := cmd.Flags().GetString("display-name")
		if displayName == "" {
			displayName = args[0]
		}

		schedule := &models.Schedule{
			ContainerName: args[0],
			DisplayName:   displayName,
			StartCron:      args[1],
			StopCron:       args[2],
			Enabled:        true,
		}

		created, err := db.CreateSchedule(schedule)
		if err != nil {
			slog.Error("failed to create schedule", "error", err)
			os.Exit(1)
		}
		fmt.Printf("Created schedule %s\n", created.ID)
	},
}

var scheduleRemoveCmd = &cobra.Command{
	Use:   "remove <id>",
	Short: "Remove a schedule",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			slog.Error("failed to load config", "error", err)
			os.Exit(1)
		}

		db, err := store.Open(cfg.DBPath)
		if err != nil {
			slog.Error("failed to open database", "error", err)
			os.Exit(1)
		}
		defer db.Close()

		if err := db.DeleteSchedule(args[0]); err != nil {
			slog.Error("failed to delete schedule", "error", err)
			os.Exit(1)
		}
		fmt.Printf("Removed schedule %s\n", args[0])
	},
}

var scheduleExportCmd = &cobra.Command{
	Use:   "export [file]",
	Short: "Export schedules to YAML config file (stdout if no file)",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			slog.Error("failed to load config", "error", err)
			os.Exit(1)
		}

		db, err := store.Open(cfg.DBPath)
		if err != nil {
			slog.Error("failed to open database", "error", err)
			os.Exit(1)
		}
		defer db.Close()

		schedules, err := db.ListSchedules()
		if err != nil {
			slog.Error("failed to list schedules", "error", err)
			os.Exit(1)
		}

		data := yamlconfig.FromSchedules(schedules)

		if len(args) == 1 {
			if err := os.WriteFile(args[0], data, 0644); err != nil {
				slog.Error("failed to write file", "error", err)
				os.Exit(1)
			}
			fmt.Printf("Exported %d schedules to %s\n", len(schedules), args[0])
		} else {
			os.Stdout.Write(data)
		}
	},
}

var scheduleImportCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Import schedules from YAML config file",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			slog.Error("failed to load config", "error", err)
			os.Exit(1)
		}

		db, err := store.Open(cfg.DBPath)
		if err != nil {
			slog.Error("failed to open database", "error", err)
			os.Exit(1)
		}
		defer db.Close()

		schedules, err := yamlconfig.ImportFromFile(args[0])
		if err != nil {
			slog.Error("failed to import schedules", "error", err)
			os.Exit(1)
		}

		dryRun, _ := cmd.Flags().GetBool("dry-run")
		if dryRun {
			fmt.Printf("Would import %d schedules:\n", len(schedules))
			for _, s := range schedules {
				fmt.Printf("  %s: %s / %s (enabled: %v)\n", s.ContainerName, s.StartCron, s.StopCron, s.Enabled)
			}
			return
		}

		created := 0
		for _, s := range schedules {
			if _, err := db.CreateSchedule(&s); err != nil {
				slog.Error("failed to create schedule", "container", s.ContainerName, "error", err)
				continue
			}
			created++
		}
		fmt.Printf("Imported %d/%d schedules from %s\n", created, len(schedules), args[0])
	},
}

func init() {
	scheduleListCmd.Flags().Bool("json", false, "output as JSON")
	scheduleAddCmd.Flags().String("display-name", "", "display name for the schedule")
	scheduleImportCmd.Flags().Bool("dry-run", false, "preview import without creating schedules")

	scheduleCmd.AddCommand(scheduleListCmd)
	scheduleCmd.AddCommand(scheduleAddCmd)
	scheduleCmd.AddCommand(scheduleRemoveCmd)
	scheduleCmd.AddCommand(scheduleExportCmd)
	scheduleCmd.AddCommand(scheduleImportCmd)
	rootCmd.AddCommand(scheduleCmd)
}