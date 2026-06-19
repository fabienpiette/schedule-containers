package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/fabienpiette/schedule-containers/internal/config"
	"github.com/fabienpiette/schedule-containers/internal/cronpresets"
	"github.com/fabienpiette/schedule-containers/internal/docker"
	"github.com/fabienpiette/schedule-containers/internal/models"
	"github.com/fabienpiette/schedule-containers/internal/ondemand"
	"github.com/fabienpiette/schedule-containers/internal/store"
)

type mockOnDemandService struct {
	wakeResult        *ondemand.WakeResult
	wakeError         error
	healthResult      *ondemand.HealthResult
	healthError       error
	stackWakeResult   *ondemand.WakeResult
	stackWakeError    error
	stackHealthResult *ondemand.HealthResult
	stackHealthError  error
	watched           []*models.Schedule
	unwatched         []string
}

func (m *mockOnDemandService) WakeContainer(ctx context.Context, name string) (*ondemand.WakeResult, error) {
	return m.wakeResult, m.wakeError
}

func (m *mockOnDemandService) CheckHealth(ctx context.Context, name string) (*ondemand.HealthResult, error) {
	return m.healthResult, m.healthError
}

func (m *mockOnDemandService) WakeStack(ctx context.Context, name string) (*ondemand.WakeResult, error) {
	return m.stackWakeResult, m.stackWakeError
}

func (m *mockOnDemandService) CheckStackHealth(ctx context.Context, name string) (*ondemand.HealthResult, error) {
	return m.stackHealthResult, m.stackHealthError
}

func (m *mockOnDemandService) AddStack(stack *models.Stack)    {}
func (m *mockOnDemandService) RemoveStack(stackID string) {}

func (m *mockOnDemandService) Watch(schedule *models.Schedule) {
	m.watched = append(m.watched, schedule)
}

func (m *mockOnDemandService) Unwatch(name string) {
	m.unwatched = append(m.unwatched, name)
}

func setupWakeTestServer(t *testing.T, mockODM *mockOnDemandService) (*Server, *mockSchedulerService) {
	t.Helper()
	f, err := os.CreateTemp("", "test-wake-*.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(f.Name()) })
	f.Close()

	db, err := store.Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	presetPath := os.TempDir() + "/test-presets-wake-" + t.Name() + ".yaml"
	presetSvc, err := cronpresets.NewService(presetPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(presetPath) })

	mockSched := &mockSchedulerService{schedules: make(map[string]*models.Schedule)}
	cfg := &config.Config{WebHost: "127.0.0.1", WebPort: 0}

	dockerClient, _ := docker.NewClient("unix:///var/run/docker.sock")
	srv := NewServer(cfg, db, dockerClient, mockSched, presetSvc, mockODM, mockODM)
	return srv, mockSched
}

func TestHandleWake_Success(t *testing.T) {
	mockODM := &mockOnDemandService{
		wakeResult: &ondemand.WakeResult{Running: false, OnDemandURL: "http://example.com"},
	}
	srv, _ := setupWakeTestServer(t, mockODM)

	r := chi.NewRouter()
	r.Get("/wake/{name}", srv.handleWake)

	req := httptest.NewRequest(http.MethodGet, "/wake/my-container", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "my-container") {
		t.Errorf("expected body to contain 'my-container', got %s", w.Body.String())
	}
}

func TestHandleWake_AlreadyRunning(t *testing.T) {
	mockODM := &mockOnDemandService{
		wakeResult: &ondemand.WakeResult{Running: true, OnDemandURL: "http://example.com"},
	}
	srv, _ := setupWakeTestServer(t, mockODM)

	r := chi.NewRouter()
	r.Get("/wake/{name}", srv.handleWake)

	req := httptest.NewRequest(http.MethodGet, "/wake/my-container", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("expected 302, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != "http://example.com" {
		t.Errorf("expected redirect to 'http://example.com', got '%s'", loc)
	}
}

func TestHandleWake_NotFound(t *testing.T) {
	mockODM := &mockOnDemandService{
		wakeError:      ondemand.ErrScheduleNotFound,
		stackWakeError: ondemand.ErrStackNotFound,
	}
	srv, _ := setupWakeTestServer(t, mockODM)

	r := chi.NewRouter()
	r.Get("/wake/{name}", srv.handleWake)

	req := httptest.NewRequest(http.MethodGet, "/wake/unknown-container", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleWakeStatus_Healthy(t *testing.T) {
	mockODM := &mockOnDemandService{
		healthResult: &ondemand.HealthResult{Healthy: true, OnDemandURL: "http://example.com"},
	}
	srv, _ := setupWakeTestServer(t, mockODM)

	r := chi.NewRouter()
	r.Get("/wake/{name}/status", srv.handleWakeStatus)

	req := httptest.NewRequest(http.MethodGet, "/wake/my-container/status", nil)
	req.Header.Set("Accept", "text/html")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	redirect := w.Header().Get("HX-Redirect")
	if redirect != "http://example.com" {
		t.Errorf("expected HX-Redirect 'http://example.com', got '%s'", redirect)
	}
}

func TestHandleWakeStatus_NotHealthyYet(t *testing.T) {
	mockODM := &mockOnDemandService{
		healthResult: &ondemand.HealthResult{Healthy: false, OnDemandURL: "http://example.com"},
	}
	srv, _ := setupWakeTestServer(t, mockODM)

	r := chi.NewRouter()
	r.Get("/wake/{name}/status", srv.handleWakeStatus)

	req := httptest.NewRequest(http.MethodGet, "/wake/my-container/status", nil)
	req.Header.Set("Accept", "text/html")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "not ready") {
		t.Errorf("expected body to contain 'Still starting', got %s", w.Body.String())
	}
}

func TestApiContainerHealth_Healthy(t *testing.T) {
	mockODM := &mockOnDemandService{
		healthResult: &ondemand.HealthResult{Healthy: true, OnDemandURL: "http://example.com"},
	}
	srv, _ := setupWakeTestServer(t, mockODM)

	r := chi.NewRouter()
	r.Get("/api/containers/{name}/health", srv.apiContainerHealth)

	req := httptest.NewRequest(http.MethodGet, "/api/containers/my-container/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var result map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result["healthy"] != true {
		t.Errorf("expected healthy=true, got %v", result["healthy"])
	}
	if result["url"] != "http://example.com" {
		t.Errorf("expected url='http://example.com', got %v", result["url"])
	}
}

func TestApiContainerHealth_NotFound(t *testing.T) {
	mockODM := &mockOnDemandService{
		healthError: ondemand.ErrScheduleNotFound,
	}
	srv, _ := setupWakeTestServer(t, mockODM)

	r := chi.NewRouter()
	r.Get("/api/containers/{name}/health", srv.apiContainerHealth)

	req := httptest.NewRequest(http.MethodGet, "/api/containers/unknown-container/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestApiWakeURL(t *testing.T) {
	mockODM := &mockOnDemandService{}
	srv, _ := setupWakeTestServer(t, mockODM)

	sched := &models.Schedule{
		ContainerName: "my-app",
		StartCron:     "0 8 * * *",
		StopCron:      "0 18 * * *",
		Enabled:       true,
	}
	created, _ := srv.store.CreateSchedule(context.Background(), sched)

	r := chi.NewRouter()
	r.Get("/api/schedules/{id}/wake-url", srv.apiWakeURL)

	req := httptest.NewRequest(http.MethodGet, "/api/schedules/"+created.ID+"/wake-url", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var result map[string]string
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if _, ok := result["wake_url"]; !ok {
		t.Error("expected wake_url field in response")
	}
	if !strings.Contains(result["wake_url"], "my-app") {
		t.Errorf("expected wake_url to contain 'my-app', got '%s'", result["wake_url"])
	}
}

func TestApiWakeURL_NotFound(t *testing.T) {
	mockODM := &mockOnDemandService{}
	srv, _ := setupWakeTestServer(t, mockODM)

	r := chi.NewRouter()
	r.Get("/api/schedules/{id}/wake-url", srv.apiWakeURL)

	req := httptest.NewRequest(http.MethodGet, "/api/schedules/nonexistent/wake-url", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleWake_InternalError(t *testing.T) {
	mockODM := &mockOnDemandService{
		wakeError: errors.New("docker connection failed"),
	}
	srv, _ := setupWakeTestServer(t, mockODM)

	r := chi.NewRouter()
	r.Get("/wake/{name}", srv.handleWake)

	req := httptest.NewRequest(http.MethodGet, "/wake/my-container", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestApiContainerHealth_InternalError(t *testing.T) {
	mockODM := &mockOnDemandService{
		healthError: errors.New("docker connection failed"),
	}
	srv, _ := setupWakeTestServer(t, mockODM)

	r := chi.NewRouter()
	r.Get("/api/containers/{name}/health", srv.apiContainerHealth)

	req := httptest.NewRequest(http.MethodGet, "/api/containers/my-container/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}