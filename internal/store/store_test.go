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