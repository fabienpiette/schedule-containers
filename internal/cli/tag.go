package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/gndm/schedule-containers/internal/config"
	"github.com/gndm/schedule-containers/internal/models"
	"github.com/gndm/schedule-containers/internal/scheduler"
	"github.com/gndm/schedule-containers/internal/store"
)

var tagCmd = &cobra.Command{
	Use:   "tag",
	Short: "Manage tags",
}

var tagListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all tags",
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

		tags, err := db.ListTags(context.Background())
		if err != nil {
			slog.Error("failed to list tags", "error", err)
			os.Exit(1)
		}

		outputJSON, _ := cmd.Flags().GetBool("json")
		if outputJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			enc.Encode(tags)
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tSTART\tSTOP\tENABLED")
		for _, t := range tags {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%v\n", t.ID[:8], t.Name, t.StartCron, t.StopCron, t.Enabled)
		}
		w.Flush()
	},
}

var tagAddCmd = &cobra.Command{
	Use:   "add <name> --start <cron> --stop <cron>",
	Short: "Create a tag",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			slog.Error("failed to load config", "error", err)
			os.Exit(1)
		}

		startCron, _ := cmd.Flags().GetString("start")
		stopCron, _ := cmd.Flags().GetString("stop")

		if startCron == "" || stopCron == "" {
			fmt.Fprintln(os.Stderr, "both --start and --stop cron expressions are required")
			os.Exit(1)
		}

		if err := scheduler.ValidateCronExpression(startCron); err != nil {
			fmt.Fprintf(os.Stderr, "invalid start cron expression: %v\n", err)
			os.Exit(1)
		}
		if err := scheduler.ValidateCronExpression(stopCron); err != nil {
			fmt.Fprintf(os.Stderr, "invalid stop cron expression: %v\n", err)
			os.Exit(1)
		}

		db, err := store.Open(cfg.DBPath)
		if err != nil {
			slog.Error("failed to open database", "error", err)
			os.Exit(1)
		}
		defer db.Close()

		tag := &models.Tag{
			Name:      args[0],
			StartCron: startCron,
			StopCron:  stopCron,
			Enabled:   true,
		}
		created, err := db.CreateTag(context.Background(), tag)
		if err != nil {
			slog.Error("failed to create tag", "error", err)
			os.Exit(1)
		}
		fmt.Printf("Created tag %s (%s)\n", created.ID, created.Name)
	},
}

var tagRemoveCmd = &cobra.Command{
	Use:   "remove <name-or-id>",
	Short: "Delete a tag and all its schedules",
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

		id := args[0]
		tag, err := db.GetTag(context.Background(), id)
		if err != nil {
			tag, err = db.GetTagByName(context.Background(), id)
			if err != nil {
				fmt.Fprintf(os.Stderr, "tag not found: %s\n", id)
				os.Exit(1)
			}
		}

		if err := db.DeleteTag(context.Background(), tag.ID); err != nil {
			slog.Error("failed to delete tag", "error", err)
			os.Exit(1)
		}
		fmt.Printf("Deleted tag %s (%s)\n", tag.ID, tag.Name)
	},
}

var tagToggleCmd = &cobra.Command{
	Use:   "toggle <name-or-id>",
	Short: "Toggle a tag on/off",
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

		id := args[0]
		tag, err := db.GetTag(context.Background(), id)
		if err != nil {
			tag, err = db.GetTagByName(context.Background(), id)
			if err != nil {
				fmt.Fprintf(os.Stderr, "tag not found: %s\n", id)
				os.Exit(1)
			}
		}

		tag.Enabled = !tag.Enabled
		updated, err := db.UpdateTag(context.Background(), tag)
		if err != nil {
			slog.Error("failed to toggle tag", "error", err)
			os.Exit(1)
		}
		fmt.Printf("Tag %s is now %s\n", updated.Name, map[bool]string{true: "enabled", false: "disabled"}[updated.Enabled])
	},
}

var tagApplyCmd = &cobra.Command{
	Use:   "apply <name-or-id> --containers app1,app2",
	Short: "Apply a tag to containers",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			slog.Error("failed to load config", "error", err)
			os.Exit(1)
		}

		containersStr, _ := cmd.Flags().GetString("containers")
		if containersStr == "" {
			fmt.Fprintln(os.Stderr, "--containers is required")
			os.Exit(1)
		}
		containers := strings.Split(containersStr, ",")

		db, err := store.Open(cfg.DBPath)
		if err != nil {
			slog.Error("failed to open database", "error", err)
			os.Exit(1)
		}
		defer db.Close()

		id := args[0]
		tag, err := db.GetTag(context.Background(), id)
		if err != nil {
			tag, err = db.GetTagByName(context.Background(), id)
			if err != nil {
				fmt.Fprintf(os.Stderr, "tag not found: %s\n", id)
				os.Exit(1)
			}
		}

		tagID := tag.ID
		created := 0
		skipped := 0
		for _, c := range containers {
			existing, _ := db.GetScheduleByTagAndContainer(context.Background(), tagID, c)
			if existing != nil {
				fmt.Printf("Skipped %s (already has this tag)\n", c)
				skipped++
				continue
			}
			sched := &models.Schedule{
				ContainerName:   c,
				DisplayName:     c,
				StartCron:       tag.StartCron,
				StopCron:        tag.StopCron,
				Enabled:         tag.Enabled,
				TagID:           &tagID,
				OnDemandEnabled: false,
			}
			if _, err := db.CreateSchedule(context.Background(), sched); err != nil {
				slog.Error("failed to create schedule", "container", c, "error", err)
				continue
			}
			created++
		}
		fmt.Printf("Applied tag %s: %d created, %d skipped\n", tag.Name, created, skipped)
	},
}

var tagRemoveContainerCmd = &cobra.Command{
	Use:   "remove-container <name-or-id> --container <name>",
	Short: "Remove a tag from a container",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			slog.Error("failed to load config", "error", err)
			os.Exit(1)
		}

		containerName, _ := cmd.Flags().GetString("container")
		if containerName == "" {
			fmt.Fprintln(os.Stderr, "--container is required")
			os.Exit(1)
		}

		db, err := store.Open(cfg.DBPath)
		if err != nil {
			slog.Error("failed to open database", "error", err)
			os.Exit(1)
		}
		defer db.Close()

		id := args[0]
		tag, err := db.GetTag(context.Background(), id)
		if err != nil {
			tag, err = db.GetTagByName(context.Background(), id)
			if err != nil {
				fmt.Fprintf(os.Stderr, "tag not found: %s\n", id)
				os.Exit(1)
			}
		}

		sched, err := db.GetScheduleByTagAndContainer(context.Background(), tag.ID, containerName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "no schedule found for tag %s and container %s\n", tag.Name, containerName)
			os.Exit(1)
		}

		if err := db.DeleteSchedule(context.Background(), sched.ID); err != nil {
			slog.Error("failed to delete schedule", "error", err)
			os.Exit(1)
		}
		fmt.Printf("Removed tag %s from container %s\n", tag.Name, containerName)
	},
}

func init() {
	tagListCmd.Flags().Bool("json", false, "output as JSON")
	tagAddCmd.Flags().String("start", "", "start cron expression")
	tagAddCmd.Flags().String("stop", "", "stop cron expression")
	tagApplyCmd.Flags().String("containers", "", "comma-separated container names")
	tagRemoveContainerCmd.Flags().String("container", "", "container name")

	tagCmd.AddCommand(tagListCmd)
	tagCmd.AddCommand(tagAddCmd)
	tagCmd.AddCommand(tagRemoveCmd)
	tagCmd.AddCommand(tagToggleCmd)
	tagCmd.AddCommand(tagApplyCmd)
	tagCmd.AddCommand(tagRemoveContainerCmd)
	rootCmd.AddCommand(tagCmd)
}