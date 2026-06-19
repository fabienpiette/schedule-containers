package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/fabienpiette/schedule-containers/internal/config"
	"github.com/fabienpiette/schedule-containers/internal/docker"
)

var containersCmd = &cobra.Command{
	Use:   "containers",
	Short: "Manage containers",
}

var containersListCmd = &cobra.Command{
	Use:   "list",
	Short: "List discovered containers",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			slog.Error("failed to load config", "error", err)
			os.Exit(1)
		}

		dockerClient, err := docker.NewClient(cfg.DockerHost)
		if err != nil {
			slog.Error("failed to connect to Docker", "error", err)
			os.Exit(1)
		}
		defer dockerClient.Close()

		containers, err := dockerClient.ListContainers(context.Background())
		if err != nil {
			slog.Error("failed to list containers", "error", err)
			os.Exit(1)
		}

		outputJSON, _ := cmd.Flags().GetBool("json")
		if outputJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			enc.Encode(containers)
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tIMAGE\tSTATE\tSTACK")
		for _, c := range containers {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", c.ID, c.Name, c.Image, c.State, c.StackName)
		}
		w.Flush()
	},
}

func init() {
	containersListCmd.Flags().Bool("json", false, "output as JSON")
	containersCmd.AddCommand(containersListCmd)
	rootCmd.AddCommand(containersCmd)
}