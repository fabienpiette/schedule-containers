package web

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/gndm/schedule-containers/internal/config"
	"github.com/gndm/schedule-containers/internal/docker"
	"github.com/gndm/schedule-containers/internal/models"
	"github.com/gndm/schedule-containers/internal/store"
)

type SchedulerService interface {
	AddSchedule(schedule *models.Schedule) error
	RemoveSchedule(scheduleID string) error
	ScheduleCount() int
}

type Server struct {
	httpServer *http.Server
	store      *store.Store
	docker     *docker.Client
	scheduler  SchedulerService
	templates  *template.Template
}

//go:embed templates/* static/*
var embeddedFS embed.FS

func NewServer(cfg *config.Config, s *store.Store, d *docker.Client, sched SchedulerService) *Server {
	tmpl := template.Must(template.New("").ParseFS(embeddedFS,
		"templates/layout.html",
		"templates/dashboard.html",
		"templates/containers.html",
		"templates/schedules.html",
		"templates/presets.html",
		"templates/partials.html",
	))

	srv := &Server{
		store:     s,
		docker:    d,
		scheduler: sched,
		templates: tmpl,
	}

	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)

	staticContent, _ := fs.Sub(embeddedFS, "static")
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticContent))))
	r.Get("/", srv.handleDashboard)
	r.Get("/containers", srv.handleContainers)
	r.Get("/schedules", srv.handleSchedulesNew)
	r.Get("/presets", srv.handlePresets)

	r.Route("/api", func(r chi.Router) {
		r.Get("/containers", srv.apiListContainers)
		r.Get("/schedules", srv.apiListSchedules)
		r.Post("/schedules", srv.apiCreateSchedule)
		r.Put("/schedules/{id}", srv.apiUpdateSchedule)
		r.Delete("/schedules/{id}", srv.apiDeleteSchedule)
		r.Post("/schedules/{id}/toggle", srv.apiToggleSchedule)
		r.Post("/containers/{name}/start", srv.apiStartContainer)
		r.Post("/containers/{name}/stop", srv.apiStopContainer)
		r.Get("/presets", srv.apiListPresets)
		r.Post("/presets", srv.apiCreateCustomPreset)
		r.Delete("/presets/{id}", srv.apiDeleteCustomPreset)
		r.Post("/import", srv.apiImportSchedules)
		r.Get("/export", srv.apiExportSchedules)
	})

	addr := fmt.Sprintf("%s:%d", cfg.WebHost, cfg.WebPort)
	slog.Info("web server starting", "addr", addr)
	srv.httpServer = &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	return srv
}

func (s *Server) Start() error {
	err := s.httpServer.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}