package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/fabienpiette/schedule-containers/internal/models"
)

type mockDockerClient struct {
	mu      sync.Mutex
	started []string
	stopped []string
}

func (m *mockDockerClient) StartContainer(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = append(m.started, name)
	return nil
}

func (m *mockDockerClient) StopContainer(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = append(m.stopped, name)
	return nil
}

func (m *mockDockerClient) ListContainers(_ context.Context) ([]models.Container, error) {
	return nil, nil
}

func (m *mockDockerClient) getStarted() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.started))
	copy(result, m.started)
	return result
}

func (m *mockDockerClient) getStopped() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.stopped))
	copy(result, m.stopped)
	return result
}

func TestAddSchedule(t *testing.T) {
	mock := &mockDockerClient{}
	s := NewScheduler(mock, "UTC")

	sched := &models.Schedule{
		ID:            "test-1",
		ContainerName: "my-app",
		StartCron:     "0 8 * * *",
		StopCron:      "0 18 * * *",
		Enabled:       true,
	}

	err := s.AddSchedule(sched)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if s.ScheduleCount() != 1 {
		t.Errorf("expected 1 schedule, got %d", s.ScheduleCount())
	}
}

func TestRemoveSchedule(t *testing.T) {
	mock := &mockDockerClient{}
	s := NewScheduler(mock, "UTC")

	sched := &models.Schedule{
		ID:            "test-1",
		ContainerName: "my-app",
		StartCron:     "0 8 * * *",
		StopCron:      "0 18 * * *",
		Enabled:       true,
	}

	s.AddSchedule(sched)
	err := s.RemoveSchedule("test-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if s.ScheduleCount() != 0 {
		t.Errorf("expected 0 schedules, got %d", s.ScheduleCount())
	}
}

func TestInvalidCronExpression(t *testing.T) {
	mock := &mockDockerClient{}
	s := NewScheduler(mock, "UTC")

	sched := &models.Schedule{
		ID:            "test-1",
		ContainerName: "my-app",
		StartCron:     "invalid",
		StopCron:      "0 18 * * *",
		Enabled:       true,
	}

	err := s.AddSchedule(sched)
	if err == nil {
		t.Fatal("expected error for invalid cron expression")
	}
}

func TestCronFiresStartStop(t *testing.T) {
	mock := &mockDockerClient{}
	s := NewScheduler(mock, "UTC")

	sched := &models.Schedule{
		ID:            "test-1",
		ContainerName: "my-app",
		StartCron:     "@every 1s",
		StopCron:      "@every 2s",
		Enabled:       true,
	}

	err := s.AddSchedule(sched)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	s.Start()
	defer s.Stop()

	time.Sleep(2500 * time.Millisecond)

	started := mock.getStarted()
	stopped := mock.getStopped()

	if len(started) < 1 {
		t.Errorf("expected at least 1 start call, got %d", len(started))
	}
	if len(stopped) < 1 {
		t.Errorf("expected at least 1 stop call, got %d", len(stopped))
	}
}

func TestDisabledScheduleDoesNotFire(t *testing.T) {
	mock := &mockDockerClient{}
	s := NewScheduler(mock, "UTC")

	sched := &models.Schedule{
		ID:            "test-1",
		ContainerName: "my-app",
		StartCron:     "@every 1s",
		StopCron:      "@every 1s",
		Enabled:       false,
	}

	err := s.AddSchedule(sched)
	if err != nil {
		t.Fatalf("expected no error for disabled schedule, got %v", err)
	}

	s.Start()
	defer s.Stop()

	time.Sleep(1500 * time.Millisecond)

	if len(mock.getStarted()) > 0 || len(mock.getStopped()) > 0 {
		t.Error("disabled schedule should not fire")
	}
}

func TestValidateCronExpression(t *testing.T) {
	tests := []struct {
		expr string
		ok   bool
	}{
		{"0 8 * * *", true},
		{"0 18 * * 1-5", true},
		{"@every 1h", true},
		{"invalid", false},
		{"0 25 * * *", false},
	}
	for _, tt := range tests {
		err := ValidateCronExpression(tt.expr)
		if tt.ok && err != nil {
			t.Errorf("expected %q to be valid, got error: %v", tt.expr, err)
		}
		if !tt.ok && err == nil {
			t.Errorf("expected %q to be invalid", tt.expr)
		}
	}
}
