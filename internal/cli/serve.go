package cli

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/gndm/schedule-containers/internal/config"
	"github.com/gndm/schedule-containers/internal/cronpresets"
	"github.com/gndm/schedule-containers/internal/docker"
	"github.com/gndm/schedule-containers/internal/scheduler"
	"github.com/gndm/schedule-containers/internal/store"
	"github.com/gndm/schedule-containers/internal/web"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the web server and scheduler",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			slog.Error("failed to load config", "error", err)
			os.Exit(1)
		}

		setupLogger(cfg.LogLevel)

		db, err := store.Open(cfg.DBPath)
		if err != nil {
			slog.Error("failed to open database", "error", err)
			os.Exit(1)
		}
		defer db.Close()

		dockerClient, err := docker.NewClient(cfg.DockerHost)
		if err != nil {
			slog.Error("failed to connect to Docker", "error", err)
			os.Exit(1)
		}
		defer dockerClient.Close()

		sched := scheduler.NewScheduler(dockerClient, cfg.Timezone)

		schedules, err := db.ListSchedules()
		if err != nil {
			slog.Error("failed to load schedules", "error", err)
			os.Exit(1)
		}
		for _, s := range schedules {
			if s.Enabled {
				if err := sched.AddSchedule(&s); err != nil {
					slog.Warn("failed to add schedule", "id", s.ID, "error", err)
				}
			}
		}

		sched.Start()
		slog.Info("scheduler started", "schedules_loaded", len(schedules))

		presetSvc, err := cronpresets.NewService(cfg.PresetsPath)
		if err != nil {
			slog.Error("failed to initialize preset service", "error", err)
			os.Exit(1)
		}

		webSrv := web.NewServer(cfg, db, dockerClient, sched, presetSvc)
		go func() {
			if err := webSrv.Start(); err != nil {
				slog.Error("web server error", "error", err)
			}
		}()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		slog.Info("shutting down")
		sched.Stop()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := webSrv.Shutdown(shutdownCtx); err != nil {
			slog.Error("web server shutdown error", "error", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

func setupLogger(level string) {
	var sl slog.Level
	switch level {
	case "debug":
		sl = slog.LevelDebug
	case "warn":
		sl = slog.LevelWarn
	case "error":
		sl = slog.LevelError
	default:
		sl = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: sl})))
}