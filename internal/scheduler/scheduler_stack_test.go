package scheduler

import (
	"context"
	"sync"
	"testing"

	"github.com/gndm/schedule-containers/internal/models"
)

type mockStackDocker struct {
	mu         sync.Mutex
	started    []string
	stopped    []string
	containers []models.Container
}

func (m *mockStackDocker) StartContainer(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = append(m.started, name)
	return nil
}

func (m *mockStackDocker) StopContainer(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = append(m.stopped, name)
	return nil
}

func (m *mockStackDocker) ListContainers(_ context.Context) ([]models.Container, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.containers, nil
}

func TestAddStackRegistersJobs(t *testing.T) {
	d := &mockStackDocker{}
	s := NewScheduler(d, "UTC")

	stack := &models.Stack{
		ID:        "stack-1",
		Name:      "myproject",
		StartCron: "* * * * *",
		StopCron:  "* * * * *",
		Enabled:   true,
	}
	if err := s.AddStack(stack); err != nil {
		t.Fatalf("AddStack failed: %v", err)
	}
	if len(s.jobs["stack-1"]) != 2 {
		t.Errorf("expected 2 cron entries, got %d", len(s.jobs["stack-1"]))
	}
}

func TestAddStackDisabledSkips(t *testing.T) {
	d := &mockStackDocker{}
	s := NewScheduler(d, "UTC")

	stack := &models.Stack{ID: "s1", Name: "proj", StartCron: "* * * * *", Enabled: false}
	if err := s.AddStack(stack); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s.jobs["s1"]) != 0 {
		t.Errorf("expected 0 jobs for disabled stack, got %d", len(s.jobs["s1"]))
	}
}

func TestRemoveStack(t *testing.T) {
	d := &mockStackDocker{}
	s := NewScheduler(d, "UTC")

	stack := &models.Stack{ID: "s1", Name: "proj", StartCron: "* * * * *", Enabled: true}
	s.AddStack(stack)
	s.RemoveStack("s1")
	if len(s.jobs["s1"]) != 0 {
		t.Errorf("expected jobs to be removed")
	}
}

func TestUpdateStack(t *testing.T) {
	d := &mockStackDocker{}
	s := NewScheduler(d, "UTC")

	stack := &models.Stack{ID: "s1", Name: "proj", StartCron: "* * * * *", Enabled: true}
	s.AddStack(stack)
	stack.StopCron = "0 18 * * *"
	if err := s.UpdateStack(stack); err != nil {
		t.Fatalf("UpdateStack failed: %v", err)
	}
	// should have re-registered: 1 start + 1 stop
	if len(s.jobs["s1"]) != 2 {
		t.Errorf("expected 2 jobs after update, got %d", len(s.jobs["s1"]))
	}
}
