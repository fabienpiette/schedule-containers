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
	"github.com/gndm/schedule-containers/internal/ondemand"
	"github.com/gndm/schedule-containers/internal/scheduler"
	"github.com/gndm/schedule-containers/internal/store"
)

type SchedulerService interface {
	AddSchedule(schedule *models.Schedule) error
	RemoveSchedule(scheduleID string) error
	ScheduleCount() int
	AddStack(stack *models.Stack) error
	RemoveStack(stackID string)
	UpdateStack(stack *models.Stack) error
}

type OnDemandService interface {
	WakeContainer(ctx context.Context, containerName string) (*ondemand.WakeResult, error)
	CheckHealth(ctx context.Context, containerName string) (*ondemand.HealthResult, error)
	Watch(schedule *models.Schedule)
	Unwatch(containerName string)
}

type StackOnDemandService interface {
	AddStack(stack *models.Stack)
	RemoveStack(stackID string)
	WakeStack(ctx context.Context, stackName string) (*ondemand.WakeResult, error)
	CheckStackHealth(ctx context.Context, stackName string) (*ondemand.HealthResult, error)
}

type Server struct {
	httpServer    *http.Server
	store         *store.Store
	docker        *docker.Client
	scheduler     SchedulerService
	presetService *cronpresets.Service
	ondemand      OnDemandService
	stackOndemand StackOnDemandService
	templates     map[string]*template.Template
	oidcProvider  *oidcProvider
}

//go:embed templates/* static/*
var embeddedFS embed.FS

var (
	_ SchedulerService    = (*scheduler.Scheduler)(nil)
	_ OnDemandService     = (*ondemand.OnDemandManager)(nil)
	_ StackOnDemandService = (*ondemand.OnDemandManager)(nil)
)

func NewServer(cfg *config.Config, s *store.Store, d *docker.Client, sched SchedulerService, ps *cronpresets.Service, odm OnDemandService, sodm StackOnDemandService) *Server {
	baseFiles := []string{
		"templates/layout.html",
		"templates/partials.html",
	}
	pages := map[string]string{
		"containers.html":  "templates/containers.html",
		"schedules.html":   "templates/schedules.html",
		"presets.html":     "templates/presets.html",
		"tags.html":        "templates/tags.html",
		"admin_users.html": "templates/admin_users.html",
	}

	templates := make(map[string]*template.Template)
	for name, pageFile := range pages {
		files := append(baseFiles, pageFile)
		templates[name] = template.Must(template.New("").ParseFS(embeddedFS, files...))
	}

	wakeContent, _ := embeddedFS.ReadFile("templates/wake.html")
	templates["wake.html"] = template.Must(template.New("wake.html").Parse(string(wakeContent)))

	loginContent, _ := embeddedFS.ReadFile("templates/login.html")
	templates["login.html"] = template.Must(template.New("login.html").Parse(string(loginContent)))

	setupContent, _ := embeddedFS.ReadFile("templates/setup.html")
	templates["setup.html"] = template.Must(template.New("setup.html").Parse(string(setupContent)))

	srv := &Server{
		store:         s,
		docker:        d,
		scheduler:     sched,
		presetService: ps,
		ondemand:      odm,
		stackOndemand: sodm,
		templates:     templates,
	}

	if cfg.OIDCEnabled() {
		if provider, err := newOIDCProvider(cfg); err != nil {
			slog.Warn("OIDC initialization failed — OIDC login disabled", "error", err)
		} else {
			srv.oidcProvider = provider
			slog.Info("OIDC initialized", "issuer", cfg.OIDCIssuer)
		}
	}

	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)

	staticContent, _ := fs.Sub(embeddedFS, "static")
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticContent))))

	// Public routes — no auth required
	r.Get("/login", srv.handleLogin)
	r.Post("/login", srv.handleLogin)
	r.Post("/logout", srv.handleLogout)
	r.Get("/setup", srv.handleSetup)
	r.Post("/setup", srv.handleSetup)
	r.Get("/wake/stack/{name}", srv.handleWakeStack)
	r.Get("/wake/stack/{name}/", srv.handleWakeStack)
	r.Get("/wake/stack/{name}/status", srv.handleWakeStackStatus)
	r.Get("/auth/oidc/login", srv.handleOIDCLogin)
	r.Get("/auth/oidc/callback", srv.handleOIDCCallback)
	r.Get("/wake/{name}", srv.handleWake)
	r.Get("/wake/{name}/", srv.handleWake)
	r.Get("/wake/{name}/status", srv.handleWakeStatus)

	// Protected routes — reader role minimum
	r.Group(func(r chi.Router) {
		r.Use(srv.firstRunRedirect)
		r.Use(srv.requireRole(models.RoleReader))

		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/schedules", http.StatusMovedPermanently)
		})
		r.Get("/containers", srv.handleContainers)
		r.Get("/schedules", srv.handleSchedulesNew)
		r.Get("/presets", srv.handlePresets)
		r.Get("/tags", srv.handleTags)

		r.Route("/api", func(r chi.Router) {
			// Reader endpoints
			r.Get("/containers", srv.apiListContainers)
			r.Get("/containers/{name}/health", srv.apiContainerHealth)
			r.Get("/schedules", srv.apiListSchedules)
			r.Get("/schedules/{id}/wake-url", srv.apiWakeURL)
			r.Get("/presets", srv.apiListPresets)
			r.Get("/tags", srv.apiListTags)
			r.Get("/tags/{id}", srv.apiGetTag)
			r.Get("/stacks", srv.apiListStacks)
			r.Get("/stacks/{id}", srv.apiGetStack)

			// Writer endpoints
			r.Group(func(r chi.Router) {
				r.Use(srv.requireRole(models.RoleWriter))
				r.Post("/schedules", srv.apiCreateSchedule)
				r.Put("/schedules/{id}", srv.apiUpdateSchedule)
				r.Post("/schedules/{id}/toggle", srv.apiToggleSchedule)
				r.Post("/containers/{name}/start", srv.apiStartContainer)
				r.Post("/containers/{name}/stop", srv.apiStopContainer)
				r.Post("/presets", srv.apiCreateCustomPreset)
				r.Post("/tags", srv.apiCreateTag)
				r.Put("/tags/{id}", srv.apiUpdateTag)
				r.Delete("/tags/{id}/containers/{name}", srv.apiRemoveTagFromContainer)
				r.Post("/tags/{id}/containers", srv.apiApplyTagToContainers)
				r.Post("/tags/{id}/toggle", srv.apiToggleTag)
				r.Post("/stacks", srv.apiCreateStack)
				r.Put("/stacks/{id}", srv.apiUpdateStack)
				r.Post("/stacks/{id}/toggle", srv.apiToggleStack)
				r.Post("/import", srv.apiImportSchedules)
			})

			// Admin endpoints
			r.Group(func(r chi.Router) {
				r.Use(srv.requireRole(models.RoleAdmin))
				r.Delete("/schedules/{id}", srv.apiDeleteSchedule)
				r.Delete("/presets/{id}", srv.apiDeleteCustomPreset)
				r.Delete("/tags/{id}", srv.apiDeleteTag)
				r.Delete("/stacks/{id}", srv.apiDeleteStack)
				r.Get("/export", srv.apiExportSchedules)
			})
		})

		// Admin pages
		r.Group(func(r chi.Router) {
			r.Use(srv.requireRole(models.RoleAdmin))
			r.Get("/admin/users", srv.handleAdminUsers)
			r.Post("/admin/users", srv.handleAdminCreateUser)
			r.Put("/admin/users/{id}", srv.handleAdminUpdateUser)
			r.Delete("/admin/users/{id}", srv.handleAdminDeleteUser)
		})
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

func (s *Server) renderStandalone(w http.ResponseWriter, name string, data any) {
	t, ok := s.templates[name]
	if !ok {
		slog.Error("standalone template not found", "name", name)
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	if err := t.Execute(w, data); err != nil {
		slog.Error("failed to render standalone page", "name", name, "error", err)
		return
	}
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

