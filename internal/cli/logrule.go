package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/fabienpiette/schedule-containers/internal/config"
	"github.com/fabienpiette/schedule-containers/internal/models"
	"github.com/fabienpiette/schedule-containers/internal/store"
)

var logRuleCmd = &cobra.Command{
	Use:   "log-rule",
	Short: "Manage log-based restart rules",
}

var logRuleListCmd = &cobra.Command{
	Use:   "list",
	Short: "List log rules",
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

		rules, err := db.ListLogRules(context.Background())
		if err != nil {
			slog.Error("failed to list log rules", "error", err)
			os.Exit(1)
		}
		for _, r := range rules {
			status := "enabled"
			if !r.Enabled {
				status = "disabled"
			}
			fmt.Printf("%s  %-20s  [%s] %q  cooldown=%ds  (%s)\n",
				r.ID, r.ContainerName, r.MatchType, r.Pattern, r.CooldownSec, status)
		}
	},
}

var logRuleAddCmd = &cobra.Command{
	Use:   "add <container> <pattern>",
	Short: "Add a log rule (default match type: substring)",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		useRegex, _ := cmd.Flags().GetBool("regex")
		cooldown, _ := cmd.Flags().GetInt("cooldown")
		matchType := models.MatchSubstring
		if useRegex {
			matchType = models.MatchRegex
		}
		rule := &models.LogRule{
			ContainerName: args[0],
			Pattern:       args[1],
			MatchType:     matchType,
			Enabled:       true,
			CooldownSec:   cooldown,
		}
		if err := rule.Validate(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

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

		created, err := db.CreateLogRule(context.Background(), rule)
		if err != nil {
			slog.Error("failed to create log rule", "error", err)
			os.Exit(1)
		}
		fmt.Printf("Created log rule %s for %s\n", created.ID, created.ContainerName)
		fmt.Println("Restart the server for the rule to take effect.")
	},
}

var logRuleRemoveCmd = &cobra.Command{
	Use:   "remove <id>",
	Short: "Remove a log rule",
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

		if err := db.DeleteLogRule(context.Background(), args[0]); err != nil {
			slog.Error("failed to delete log rule", "error", err)
			os.Exit(1)
		}
		fmt.Printf("Removed log rule %s\n", args[0])
	},
}

func setLogRuleEnabled(id string, enabled bool) {
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

	rule, err := db.GetLogRule(context.Background(), id)
	if err != nil {
		slog.Error("log rule not found", "id", id, "error", err)
		os.Exit(1)
	}
	rule.Enabled = enabled
	if enabled {
		rule.DisabledReason = nil
	}
	if _, err := db.UpdateLogRule(context.Background(), rule); err != nil {
		slog.Error("failed to update log rule", "error", err)
		os.Exit(1)
	}
	fmt.Printf("Log rule %s enabled=%v (restart the server to apply)\n", id, enabled)
}

var logRuleEnableCmd = &cobra.Command{
	Use:   "enable <id>",
	Short: "Enable a log rule",
	Args:  cobra.ExactArgs(1),
	Run:   func(cmd *cobra.Command, args []string) { setLogRuleEnabled(args[0], true) },
}

var logRuleDisableCmd = &cobra.Command{
	Use:   "disable <id>",
	Short: "Disable a log rule",
	Args:  cobra.ExactArgs(1),
	Run:   func(cmd *cobra.Command, args []string) { setLogRuleEnabled(args[0], false) },
}

func init() {
	logRuleAddCmd.Flags().Bool("regex", false, "treat the pattern as a regular expression")
	logRuleAddCmd.Flags().Int("cooldown", 60, "seconds to wait between restarts for this rule")

	logRuleCmd.AddCommand(logRuleListCmd, logRuleAddCmd, logRuleRemoveCmd, logRuleEnableCmd, logRuleDisableCmd)
	rootCmd.AddCommand(logRuleCmd)
}
