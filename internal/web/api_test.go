package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/gndm/schedule-containers/internal/config"
	"github.com/gndm/schedule-containers/internal/docker"
	"github.com/gndm/schedule-containers/internal/models"
	"github.com/gndm/schedule-containers/internal/store"
)

func setupTestServer(t *testing.T) (*Server, *mockSchedulerService) {
	t.Helper()
	f, err := os.CreateTemp("", "test-*.db")
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

	mockSched := &mockSchedulerService{schedules: make(map[string]*models.Schedule)}
	cfg := &config.Config{WebHost: "127.0.0.1", WebPort: 0}

	dockerClient, _ := docker.NewClient("unix:///var/run/docker.sock")
	srv := NewServer(cfg, db, dockerClient, mockSched)
	return srv, mockSched
}

type mockSchedulerService struct {
	schedules map[string]*models.Schedule
	added     []*models.Schedule
	removed   []string
}

func (m *mockSchedulerService) AddSchedule(schedule *models.Schedule) error {
	m.added = append(m.added, schedule)
	m.schedules[schedule.ID] = schedule
	return nil
}

func (m *mockSchedulerService) RemoveSchedule(scheduleID string) error {
	m.removed = append(m.removed, scheduleID)
	delete(m.schedules, scheduleID)
	return nil
}

func (m *mockSchedulerService) ScheduleCount() int {
	return len(m.schedules)
}

func TestAPIListSchedules(t *testing.T) {
	srv, _ := setupTestServer(t)

	sched := &models.Schedule{
		ContainerName: "my-app",
		DisplayName:   "My App",
		StartCron:     "0 8 * * *",
		StopCron:      "0 18 * * *",
		Enabled:       true,
	}
	srv.store.CreateSchedule(sched)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/schedules", nil)
	srv.apiListSchedules(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var schedules []models.Schedule
	if err := json.NewDecoder(w.Body).Decode(&schedules); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(schedules) != 1 {
		t.Errorf("expected 1 schedule, got %d", len(schedules))
	}
}

func TestAPICreateSchedule(t *testing.T) {
	srv, mockSched := setupTestServer(t)

	body := `{"container_name":"my-app","display_name":"My App","start_cron":"0 8 * * *","stop_cron":"0 18 * * *","enabled":true}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/schedules", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	srv.apiCreateSchedule(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d, body: %s", w.Code, w.Body.String())
	}
	if len(mockSched.added) != 1 {
		t.Errorf("expected 1 schedule added to scheduler, got %d", len(mockSched.added))
	}
}

func TestAPICreateScheduleInvalidCron(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := `{"container_name":"my-app","start_cron":"invalid","stop_cron":"0 18 * * *","enabled":true}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/schedules", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	srv.apiCreateSchedule(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAPIDeleteSchedule(t *testing.T) {
	srv, mockSched := setupTestServer(t)

	sched := &models.Schedule{
		ContainerName: "my-app",
		StartCron:     "0 8 * * *",
		StopCron:      "0 18 * * *",
		Enabled:       true,
	}
	created, _ := srv.store.CreateSchedule(sched)

	r := chi.NewRouter()
	r.Delete("/api/schedules/{id}", srv.apiDeleteSchedule)

	req := httptest.NewRequest(http.MethodDelete, "/api/schedules/"+created.ID, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
	if len(mockSched.removed) != 1 {
		t.Errorf("expected 1 schedule removed from scheduler, got %d", len(mockSched.removed))
	}
}

func TestAPIToggleSchedule(t *testing.T) {
	srv, _ := setupTestServer(t)

	sched := &models.Schedule{
		ContainerName: "my-app",
		StartCron:     "0 8 * * *",
		StopCron:      "0 18 * * *",
		Enabled:       true,
	}
	created, _ := srv.store.CreateSchedule(sched)

	r := chi.NewRouter()
	r.Post("/api/schedules/{id}/toggle", srv.apiToggleSchedule)

	req := httptest.NewRequest(http.MethodPost, "/api/schedules/"+created.ID+"/toggle", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var result models.Schedule
	json.NewDecoder(w.Body).Decode(&result)
	if result.Enabled != false {
		t.Errorf("expected Enabled=false after toggle, got %v", result.Enabled)
	}
}