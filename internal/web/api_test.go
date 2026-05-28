package web

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/gndm/schedule-containers/internal/config"
	"github.com/gndm/schedule-containers/internal/cronpresets"
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

	presetPath := os.TempDir() + "/test-presets-" + t.Name() + ".yaml"
	presetSvc, err := cronpresets.NewService(presetPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(presetPath) })

	mockSched := &mockSchedulerService{schedules: make(map[string]*models.Schedule)}
	cfg := &config.Config{WebHost: "127.0.0.1", WebPort: 0}

	dockerClient, _ := docker.NewClient("unix:///var/run/docker.sock")
	srv := NewServer(cfg, db, dockerClient, mockSched, presetSvc, nil, nil)
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

func (m *mockSchedulerService) AddStack(stack *models.Stack) error {
	return nil
}

func (m *mockSchedulerService) RemoveStack(stackID string) {}

func (m *mockSchedulerService) UpdateStack(stack *models.Stack) error {
	return nil
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
	srv.store.CreateSchedule(context.Background(), sched)

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
	created, _ := srv.store.CreateSchedule(context.Background(), sched)

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
	created, _ := srv.store.CreateSchedule(context.Background(), sched)

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

func TestAPIUpdateSchedule(t *testing.T) {
	srv, mockSched := setupTestServer(t)

	sched := &models.Schedule{
		ContainerName: "my-app",
		StartCron:     "0 8 * * *",
		StopCron:      "0 18 * * *",
		Enabled:       true,
	}
	created, _ := srv.store.CreateSchedule(context.Background(), sched)

	body := `{"container_name":"my-app","start_cron":"0 9 * * *","stop_cron":"0 19 * * *","enabled":true}`
	r := chi.NewRouter()
	r.Put("/api/schedules/{id}", srv.apiUpdateSchedule)

	req := httptest.NewRequest(http.MethodPut, "/api/schedules/"+created.ID, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var updated models.Schedule
	json.NewDecoder(w.Body).Decode(&updated)
	if updated.StartCron != "0 9 * * *" {
		t.Errorf("expected start_cron updated, got %s", updated.StartCron)
	}

	schedAdded := mockSched.added[len(mockSched.added)-1]
	if schedAdded.StartCron != "0 9 * * *" {
		t.Errorf("expected scheduler to have updated schedule, got %s", schedAdded.StartCron)
	}
}

func TestAPICreateScheduleMissingContainer(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := `{"start_cron":"0 8 * * *","stop_cron":"0 18 * * *","enabled":true}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/schedules", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	srv.apiCreateSchedule(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAPICreateScheduleMissingCron(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := `{"container_name":"my-app","enabled":true}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/schedules", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	srv.apiCreateSchedule(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAPICreateScheduleDisabledNotAddedToRunner(t *testing.T) {
	srv, mockSched := setupTestServer(t)

	body := `{"container_name":"my-app","start_cron":"0 8 * * *","stop_cron":"0 18 * * *","enabled":false}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/schedules", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	srv.apiCreateSchedule(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
	if len(mockSched.added) != 0 {
		t.Errorf("expected 0 schedules added to runner for disabled schedule, got %d", len(mockSched.added))
	}
}

func TestAPIListPresets(t *testing.T) {
	srv, _ := setupTestServer(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/presets", nil)
	srv.apiListPresets(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var presets []models.CronPreset
	if err := json.NewDecoder(w.Body).Decode(&presets); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(presets) == 0 {
		t.Error("expected non-empty presets list")
	}
}

func TestAPICreateCustomPreset(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := `{"label":"Late Start","expression":"0 10 * * 1-5","category":"Custom","description":"Weekdays at 10am"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/presets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	srv.apiCreateCustomPreset(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d, body: %s", w.Code, w.Body.String())
	}

	var preset models.CronPreset
	json.NewDecoder(w.Body).Decode(&preset)
	if preset.Label != "Late Start" {
		t.Errorf("expected label 'Late Start', got %s", preset.Label)
	}
	if preset.Expression != "0 10 * * 1-5" {
		t.Errorf("expected expression, got %s", preset.Expression)
	}
	if preset.ID == "" {
		t.Error("expected non-empty ID")
	}
}

func TestAPICreateCustomPresetInvalidCron(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := `{"label":"Bad","expression":"invalid","category":"Custom"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/presets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	srv.apiCreateCustomPreset(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAPICreateCustomPresetMissingLabel(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := `{"expression":"0 8 * * *"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/presets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	srv.apiCreateCustomPreset(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAPIDeleteCustomPreset(t *testing.T) {
	srv, _ := setupTestServer(t)

	created, err := srv.presetService.Create("ToDelete", "0 8 * * *", "Custom", "")
	if err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	r.Delete("/api/presets/{id}", srv.apiDeleteCustomPreset)

	req := httptest.NewRequest(http.MethodDelete, "/api/presets/"+created.ID, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}

	presets := srv.presetService.List()
	for _, p := range presets {
		if p.ID == created.ID {
			t.Error("preset should have been deleted")
		}
	}
}

func TestAPIExportSchedules(t *testing.T) {
	srv, _ := setupTestServer(t)

	srv.store.CreateSchedule(context.Background(), &models.Schedule{
		ContainerName: "my-app",
		DisplayName:   "My App",
		StartCron:     "0 8 * * *",
		StopCron:      "0 18 * * *",
		Enabled:       true,
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/export", nil)
	srv.apiExportSchedules(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/yaml" {
		t.Errorf("expected application/yaml, got %s", ct)
	}
	if len(w.Body.Bytes()) == 0 {
		t.Error("expected non-empty body")
	}
}

func TestAPIImportSchedules(t *testing.T) {
	srv, mockSched := setupTestServer(t)

	yaml := `schedules:
  - container: my-app
    start_cron: "0 8 * * *"
    stop_cron: "0 18 * * *"
    enabled: true
  - container: redis
    start_cron: "0 9 * * *"
    stop_cron: "0 21 * * *"
    enabled: true
`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/import", bytes.NewBufferString(yaml))
	req.Header.Set("Content-Type", "application/yaml")
	srv.apiImportSchedules(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var result map[string]int
	json.NewDecoder(w.Body).Decode(&result)
	if result["imported"] != 2 {
		t.Errorf("expected 2 imported, got %d", result["imported"])
	}
	if result["total"] != 2 {
		t.Errorf("expected 2 total, got %d", result["total"])
	}
	if len(mockSched.added) != 2 {
		t.Errorf("expected 2 schedules added to runner, got %d", len(mockSched.added))
	}
}

func TestAPIImportSchedulesMultipart(t *testing.T) {
	srv, _ := setupTestServer(t)

	yaml := `schedules:
  - container: webapp
    start_cron: "0 7 * * 1-5"
    stop_cron: "0 19 * * 1-5"
    enabled: true
`
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("file", "schedules.yaml")
	part.Write([]byte(yaml))
	writer.Close()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/import", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	srv.apiImportSchedules(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var result map[string]int
	json.NewDecoder(w.Body).Decode(&result)
	if result["imported"] != 1 {
		t.Errorf("expected 1 imported, got %d", result["imported"])
	}
}

func TestAPIImportSchedulesInvalidYAML(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := `not valid yaml: {{{`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/import", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/yaml")
	srv.apiImportSchedules(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAPIImportSchedulesInvalidCron(t *testing.T) {
	srv, _ := setupTestServer(t)

	yaml := `schedules:
  - container: my-app
    start_cron: "invalid"
    stop_cron: "0 18 * * *"
    enabled: true
`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/import", bytes.NewBufferString(yaml))
	req.Header.Set("Content-Type", "application/yaml")
	srv.apiImportSchedules(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
func TestAPIListTags(t *testing.T) {
	srv, _ := setupTestServer(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tags", nil)
	srv.apiListTags(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var tags []models.Tag
	if err := json.NewDecoder(w.Body).Decode(&tags); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
}

func TestAPICreateTag(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := `{"name":"business-hours","start_cron":"0 8 * * 1-5","stop_cron":"0 18 * * 1-5","enabled":true}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/tags", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	srv.apiCreateTag(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d, body: %s", w.Code, w.Body.String())
	}

	var tag models.Tag
	json.NewDecoder(w.Body).Decode(&tag)
	if tag.Name != "business-hours" {
		t.Errorf("expected name 'business-hours', got %s", tag.Name)
	}
	if tag.ID == "" {
		t.Error("expected non-empty ID")
	}
}

func TestAPICreateTagInvalidCron(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := `{"name":"bad","start_cron":"invalid","stop_cron":"0 18 * * *"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/tags", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	srv.apiCreateTag(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAPICreateTagDuplicateName(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := `{"name":"test-tag","start_cron":"0 8 * * *","stop_cron":"0 18 * * *"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/tags", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	srv.apiCreateTag(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/tags", bytes.NewBufferString(body))
	req2.Header.Set("Content-Type", "application/json")
	srv.apiCreateTag(w2, req2)

	if w2.Code != http.StatusConflict {
		t.Errorf("expected 409 for duplicate name, got %d", w2.Code)
	}
}

func TestAPIGetTag(t *testing.T) {
	srv, _ := setupTestServer(t)

	tag, _ := srv.store.CreateTag(context.Background(), &models.Tag{Name: "test", StartCron: "0 8 * * *", StopCron: "0 18 * * *", Enabled: true})

	r := chi.NewRouter()
	r.Get("/api/tags/{id}", srv.apiGetTag)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tags/"+tag.ID, nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAPIUpdateTag(t *testing.T) {
	srv, _ := setupTestServer(t)

	tag, _ := srv.store.CreateTag(context.Background(), &models.Tag{Name: "test", StartCron: "0 8 * * *", StopCron: "0 18 * * *", Enabled: true})

	body := `{"name":"test","start_cron":"0 9 * * *","stop_cron":"0 18 * * *","enabled":true}`
	r := chi.NewRouter()
	r.Put("/api/tags/{id}", srv.apiUpdateTag)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/tags/"+tag.ID, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var updated models.Tag
	json.NewDecoder(w.Body).Decode(&updated)
	if updated.StartCron != "0 9 * * *" {
		t.Errorf("expected start_cron updated, got %s", updated.StartCron)
	}
}

func TestAPIDeleteTag(t *testing.T) {
	srv, mockSched := setupTestServer(t)

	tag, _ := srv.store.CreateTag(context.Background(), &models.Tag{Name: "test", StartCron: "0 8 * * *", StopCron: "0 18 * * *", Enabled: true})
	tagID := tag.ID
	sched, _ := srv.store.CreateSchedule(context.Background(), &models.Schedule{
		ContainerName: "my-app",
		DisplayName:   "my-app",
		StartCron:     "0 8 * * *",
		StopCron:      "0 18 * * *",
		Enabled:       true,
		TagID:         &tagID,
	})
	mockSched.added = nil

	r := chi.NewRouter()
	r.Delete("/api/tags/{id}", srv.apiDeleteTag)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/tags/"+tag.ID, nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}

	schedules, _ := srv.store.ListSchedules(context.Background())
	for _, s := range schedules {
		if s.ID == sched.ID {
			t.Error("tag-derived schedule should have been deleted")
		}
	}
	if len(mockSched.removed) != 1 || mockSched.removed[0] != sched.ID {
		t.Errorf("expected scheduler.RemoveSchedule called for %s", sched.ID)
	}
}

func TestAPIToggleTag(t *testing.T) {
	srv, _ := setupTestServer(t)

	tag, _ := srv.store.CreateTag(context.Background(), &models.Tag{Name: "test", StartCron: "0 8 * * *", StopCron: "0 18 * * *", Enabled: true})
	tagID := tag.ID
	srv.store.CreateSchedule(context.Background(), &models.Schedule{
		ContainerName: "my-app",
		DisplayName:   "my-app",
		StartCron:     "0 8 * * *",
		StopCron:      "0 18 * * *",
		Enabled:       true,
		TagID:         &tagID,
	})

	r := chi.NewRouter()
	r.Post("/api/tags/{id}/toggle", srv.apiToggleTag)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/tags/"+tag.ID+"/toggle", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var toggled models.Tag
	json.NewDecoder(w.Body).Decode(&toggled)
	if toggled.Enabled != false {
		t.Errorf("expected tag Enabled=false after toggle, got %v", toggled.Enabled)
	}

	schedules, _ := srv.store.ListSchedulesByTag(context.Background(), tag.ID)
	for _, s := range schedules {
		if s.Enabled != false {
			t.Errorf("expected schedule Enabled=false after tag toggle, got %v", s.Enabled)
		}
	}
}

func TestAPIApplyTagToContainers(t *testing.T) {
	srv, _ := setupTestServer(t)

	tag, _ := srv.store.CreateTag(context.Background(), &models.Tag{Name: "test", StartCron: "0 8 * * *", StopCron: "0 18 * * *", Enabled: true})

	r := chi.NewRouter()
	r.Post("/api/tags/{id}/containers", srv.apiApplyTagToContainers)

	body := `{"containers":["my-app","redis"]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/tags/"+tag.ID+"/containers", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	schedules, _ := srv.store.ListSchedulesByTag(context.Background(), tag.ID)
	if len(schedules) != 2 {
		t.Errorf("expected 2 schedules for tag, got %d", len(schedules))
	}
}

func TestAPIRemoveTagFromContainer(t *testing.T) {
	srv, _ := setupTestServer(t)

	tag, _ := srv.store.CreateTag(context.Background(), &models.Tag{Name: "test", StartCron: "0 8 * * *", StopCron: "0 18 * * *", Enabled: true})
	tagID := tag.ID
	srv.store.CreateSchedule(context.Background(), &models.Schedule{
		ContainerName: "my-app",
		DisplayName:   "my-app",
		StartCron:     "0 8 * * *",
		StopCron:      "0 18 * * *",
		Enabled:       true,
		TagID:         &tagID,
	})

	r := chi.NewRouter()
	r.Delete("/api/tags/{id}/containers/{name}", srv.apiRemoveTagFromContainer)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/tags/"+tag.ID+"/containers/my-app", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}

	schedules, _ := srv.store.ListSchedulesByTag(context.Background(), tag.ID)
	if len(schedules) != 0 {
		t.Errorf("expected 0 schedules for tag after removal, got %d", len(schedules))
	}
}

func TestAPIUpdateScheduleRejectsCronChangeForTagSchedule(t *testing.T) {
	srv, _ := setupTestServer(t)

	tag, _ := srv.store.CreateTag(context.Background(), &models.Tag{Name: "test", StartCron: "0 8 * * *", StopCron: "0 18 * * *", Enabled: true})
	tagID := tag.ID
	created, _ := srv.store.CreateSchedule(context.Background(), &models.Schedule{
		ContainerName: "my-app",
		DisplayName:   "my-app",
		StartCron:     "0 8 * * *",
		StopCron:      "0 18 * * *",
		Enabled:       true,
		TagID:         &tagID,
	})

	body := `{"container_name":"my-app","start_cron":"0 9 * * *","stop_cron":"0 18 * * *","enabled":true}`
	r := chi.NewRouter()
	r.Put("/api/schedules/{id}", srv.apiUpdateSchedule)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/schedules/"+created.ID, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
