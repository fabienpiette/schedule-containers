package store

import (
	"os"
	"testing"

	"github.com/gndm/schedule-containers/internal/models"
)

func tempDB(t *testing.T) *Store {
	t.Helper()
	f, err := os.CreateTemp("", "schedule-containers-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(f.Name()) })
	f.Close()

	s, err := Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpenAndMigrate(t *testing.T) {
	s := tempDB(t)
	if s == nil {
		t.Fatal("expected non-nil store")
	}
}

func TestCreateAndGetSchedule(t *testing.T) {
	s := tempDB(t)
	sched := &models.Schedule{
		ContainerName: "my-app",
		DisplayName:   "My App",
		StackName:     "webstack",
		StartCron:      "0 8 * * 1-5",
		StopCron:       "0 18 * * 1-5",
		Enabled:        true,
	}
	created, err := s.CreateSchedule(sched)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if created.ID == "" {
		t.Error("expected non-empty ID")
	}
	if created.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}

	got, err := s.GetSchedule(created.ID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got.ContainerName != "my-app" {
		t.Errorf("expected my-app, got %s", got.ContainerName)
	}
	if got.StartCron != "0 8 * * 1-5" {
		t.Errorf("unexpected StartCron: %s", got.StartCron)
	}
}

func TestListSchedules(t *testing.T) {
	s := tempDB(t)
	s1 := &models.Schedule{ContainerName: "app1", StartCron: "0 8 * * *", StopCron: "0 18 * * *", Enabled: true}
	s2 := &models.Schedule{ContainerName: "app2", StartCron: "0 9 * * *", StopCron: "0 19 * * *", Enabled: false}
	s.CreateSchedule(s1)
	s.CreateSchedule(s2)

	schedules, err := s.ListSchedules()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(schedules) != 2 {
		t.Errorf("expected 2 schedules, got %d", len(schedules))
	}
}

func TestUpdateSchedule(t *testing.T) {
	s := tempDB(t)
	sched := &models.Schedule{ContainerName: "app1", StartCron: "0 8 * * *", StopCron: "0 18 * * *", Enabled: true}
	created, _ := s.CreateSchedule(sched)

	created.StartCron = "0 9 * * *"
	updated, err := s.UpdateSchedule(created)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if updated.StartCron != "0 9 * * *" {
		t.Errorf("expected updated StartCron, got %s", updated.StartCron)
	}
}

func TestDeleteSchedule(t *testing.T) {
	s := tempDB(t)
	sched := &models.Schedule{ContainerName: "app1", StartCron: "0 8 * * *", StopCron: "0 18 * * *", Enabled: true}
	created, _ := s.CreateSchedule(sched)

	err := s.DeleteSchedule(created.ID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	_, err = s.GetSchedule(created.ID)
	if err == nil {
		t.Error("expected error getting deleted schedule")
	}
}

func TestToggleSchedule(t *testing.T) {
	s := tempDB(t)
	sched := &models.Schedule{ContainerName: "app1", StartCron: "0 8 * * *", StopCron: "0 18 * * *", Enabled: true}
	created, _ := s.CreateSchedule(sched)

	toggled, err := s.ToggleSchedule(created.ID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if toggled.Enabled != false {
		t.Errorf("expected Enabled=false, got %v", toggled.Enabled)
	}

	toggled2, err := s.ToggleSchedule(created.ID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if toggled2.Enabled != true {
		t.Errorf("expected Enabled=true, got %v", toggled2.Enabled)
	}
}

func TestCreateCustomPreset(t *testing.T) {
	s := tempDB(t)
	p := &models.CronPreset{
		Label:       "My Preset",
		Expression:  "0 9 * * 1-5",
		Category:    "Custom",
		Description: "Weekdays at 9am",
	}
	created, err := s.CreateCustomPreset(p)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if created.ID == "" {
		t.Error("expected non-empty ID")
	}
	if created.Builtin != false {
		t.Error("expected Builtin=false for custom preset")
	}
	if created.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
}

func TestGetCustomPreset(t *testing.T) {
	s := tempDB(t)
	p := &models.CronPreset{Label: "Test", Expression: "0 10 * * *", Category: "Custom"}
	created, _ := s.CreateCustomPreset(p)

	got, err := s.GetCustomPreset(created.ID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got.Label != "Test" {
		t.Errorf("expected label 'Test', got %s", got.Label)
	}
	if got.Expression != "0 10 * * *" {
		t.Errorf("expected expression '0 10 * * *', got %s", got.Expression)
	}
	if got.Builtin != false {
		t.Error("expected Builtin=false for custom preset")
	}
}

func TestListCustomPresets(t *testing.T) {
	s := tempDB(t)
	s.CreateCustomPreset(&models.CronPreset{Label: "P1", Expression: "0 8 * * *", Category: "Custom"})
	s.CreateCustomPreset(&models.CronPreset{Label: "P2", Expression: "0 9 * * *", Category: "Work"})

	presets, err := s.ListCustomPresets()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(presets) != 2 {
		t.Errorf("expected 2 presets, got %d", len(presets))
	}
}

func TestDeleteCustomPreset(t *testing.T) {
	s := tempDB(t)
	p := &models.CronPreset{Label: "ToDelete", Expression: "0 8 * * *", Category: "Custom"}
	created, _ := s.CreateCustomPreset(p)

	err := s.DeleteCustomPreset(created.ID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	_, err = s.GetCustomPreset(created.ID)
	if err == nil {
		t.Error("expected error getting deleted preset")
	}
}

func TestCreateCustomPresetDefaultCategory(t *testing.T) {
	s := tempDB(t)
	p := &models.CronPreset{Label: "NoCat", Expression: "0 8 * * *", Category: "Custom"}
	created, err := s.CreateCustomPreset(p)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if created.Category != "Custom" {
		t.Errorf("expected category 'Custom', got %s", created.Category)
	}
}