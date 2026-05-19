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
	"github.com/gndm/schedule-containers/internal/cronpresets"
	"github.com/gndm/schedule-containers/internal/docker"
	"github.com/gndm/schedule-containers/internal/models"
	"github.com/gndm/schedule-containers/internal/scheduler"
	"github.com/gndm/schedule-containers/internal/store"
)

type SchedulerService interface {
	AddSchedule(schedule *models.Schedule) error
	RemoveSchedule(scheduleID string) error
	ScheduleCount() int
}

type Server struct {
	httpServer    *http.Server
	store         *store.Store
	docker        *docker.Client
	scheduler     SchedulerService
	presetService *cronpresets.Service
	templates     map[string]*template.Template
}

//go:embed templates/* static/*
var embeddedFS embed.FS

var _ SchedulerService = (*scheduler.Scheduler)(nil)

func NewServer(cfg *config.Config, s *store.Store, d *docker.Client, sched SchedulerService, ps *cronpresets.Service) *Server {
	baseFiles := []string{
		"templates/layout.html",
		"templates/partials.html",
	}
	pages := map[string]string{
		"dashboard.html":  "templates/dashboard.html",
		"containers.html": "templates/containers.html",
		"schedules.html":  "templates/schedules.html",
		"presets.html":    "templates/presets.html",
		"tags.html":       "templates/tags.html",
	}

	templates := make(map[string]*template.Template)
	for name, pageFile := range pages {
		files := append(baseFiles, pageFile)
		templates[name] = template.Must(template.New("").ParseFS(embeddedFS, files...))
	}

	srv := &Server{
		store:         s,
		docker:        d,
		scheduler:     sched,
		presetService: ps,
		templates:     templates,
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
	r.Get("/tags", srv.handleTags)

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
		r.Get("/tags", srv.apiListTags)
		r.Post("/tags", srv.apiCreateTag)
		r.Get("/tags/{id}", srv.apiGetTag)
		r.Put("/tags/{id}", srv.apiUpdateTag)
		r.Delete("/tags/{id}", srv.apiDeleteTag)
		r.Post("/tags/{id}/containers", srv.apiApplyTagToContainers)
		r.Delete("/tags/{id}/containers/{name}", srv.apiRemoveTagFromContainer)
		r.Post("/tags/{id}/toggle", srv.apiToggleTag)
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

func (s *Server) renderPartial(w http.ResponseWriter, name string, data any) {
	for _, t := range s.templates {
		if t.Lookup(name) != nil {
			if err := t.ExecuteTemplate(w, name, data); err != nil {
				slog.Error("failed to render partial", "name", name, "error", err)
			}
			return
		}
	}
	slog.Error("partial template not found", "name", name)
	http.Error(w, "template not found", http.StatusInternalServerError)
}

func (s *Server) renderPage(w http.ResponseWriter, name string, data any) {
	t, ok := s.templates[name]
	if !ok {
		slog.Error("page template not found", "name", name)
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		slog.Error("failed to render page", "name", name, "error", err)
		return
	}
}

func (s *Server) respondNoContent(w http.ResponseWriter, r *http.Request, toastMsg string) {
	if wantsHTML(r) {
		w.Header().Set("X-Toast-Message", toastMsg)
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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