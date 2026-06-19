package ondemand

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/fabienpiette/schedule-containers/internal/docker"
	"github.com/fabienpiette/schedule-containers/internal/models"
)

type mockOnDemandDocker struct {
	mu       sync.Mutex
	started  []string
	stopped  []string
	running  map[string]bool
	health   map[string]*docker.ContainerHealth
	statsErr error
}

func newMockDocker() *mockOnDemandDocker {
	return &mockOnDemandDocker{
		running: make(map[string]bool),
		health:  make(map[string]*docker.ContainerHealth),
	}
}

func (m *mockOnDemandDocker) StartContainer(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = append(m.started, name)
	m.running[name] = true
	return nil
}

func (m *mockOnDemandDocker) StopContainer(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = append(m.stopped, name)
	m.running[name] = false
	return nil
}

func (m *mockOnDemandDocker) IsRunning(ctx context.Context, name string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running[name], nil
}

func (m *mockOnDemandDocker) InspectContainer(ctx context.Context, name string) (*docker.ContainerHealth, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if h, ok := m.health[name]; ok {
		return h, nil
	}
	return &docker.ContainerHealth{}, nil
}

func (m *mockOnDemandDocker) ContainerStats(ctx context.Context, name string) (<-chan docker.StatsSnapshot, error) {
	if m.statsErr != nil {
		return nil, m.statsErr
	}
	ch := make(chan docker.StatsSnapshot, 1)
	close(ch)
	return ch, nil
}

func (m *mockOnDemandDocker) ListContainers(ctx context.Context) ([]models.Container, error) {
	return nil, nil
}

func (m *mockOnDemandDocker) getStarted() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.started))
	copy(result, m.started)
	return result
}

func TestWakeContainer_AlreadyRunning(t *testing.T) {
	mock := newMockDocker()
	mock.running["my-app"] = true

	mgr := NewManager(mock, nil)
	mgr.schedules["my-app"] = &models.Schedule{
		ContainerName: "my-app",
		OnDemandURL:   "http://my-app.local:8080",
	}

	result, err := mgr.WakeContainer(context.Background(), "my-app")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !result.Running {
		t.Error("expected Running=true")
	}
	if result.OnDemandURL != "http://my-app.local:8080" {
		t.Errorf("expected on_demand_url http://my-app.local:8080, got %s", result.OnDemandURL)
	}
	if len(mock.getStarted()) != 0 {
		t.Error("expected no StartContainer call")
	}
}

func TestWakeContainer_StartsContainer(t *testing.T) {
	mock := newMockDocker()
	mock.running["my-app"] = false

	mgr := NewManager(mock, nil)
	mgr.schedules["my-app"] = &models.Schedule{
		ContainerName: "my-app",
		OnDemandURL:   "http://my-app.local:9090",
	}

	result, err := mgr.WakeContainer(context.Background(), "my-app")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Running {
		t.Error("expected Running=false since container was just started")
	}
	if result.OnDemandURL != "http://my-app.local:9090" {
		t.Errorf("expected on_demand_url http://my-app.local:9090, got %s", result.OnDemandURL)
	}
	started := mock.getStarted()
	if len(started) != 1 || started[0] != "my-app" {
		t.Errorf("expected StartContainer called with my-app, got %v", started)
	}
}

func TestWakeContainer_NotFound(t *testing.T) {
	mock := newMockDocker()
	mgr := NewManager(mock, nil)

	_, err := mgr.WakeContainer(context.Background(), "no-such-container")
	if err == nil {
		t.Fatal("expected error for unknown container")
	}
}

func TestCheckHealth_Healthy(t *testing.T) {
	mock := newMockDocker()
	mock.health["my-app"] = &docker.ContainerHealth{
		Status:             "healthy",
		HealthCheckDefined: true,
	}

	mgr := NewManager(mock, nil)
	mgr.schedules["my-app"] = &models.Schedule{
		ContainerName: "my-app",
		OnDemandURL:   "http://my-app.local",
	}

	result, err := mgr.CheckHealth(context.Background(), "my-app")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !result.Healthy {
		t.Error("expected Healthy=true")
	}
	if result.OnDemandURL != "http://my-app.local" {
		t.Errorf("expected on_demand_url http://my-app.local, got %s", result.OnDemandURL)
	}
}

func TestCheckHealth_TCPFallback(t *testing.T) {
	mock := newMockDocker()
	mock.health["my-app"] = &docker.ContainerHealth{
		Status:             "",
		HealthCheckDefined: false,
		Ports:              []uint16{8080, 9090},
	}

	mgr := NewManager(mock, nil)
	mgr.schedules["my-app"] = &models.Schedule{
		ContainerName: "my-app",
		OnDemandURL:   "http://my-app.local:8080",
	}

	result, err := mgr.CheckHealth(context.Background(), "my-app")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if result.OnDemandURL != "http://my-app.local:8080" {
		t.Errorf("expected on_demand_url http://my-app.local:8080, got %s", result.OnDemandURL)
	}
}

func TestCheckHealth_RunningFallback(t *testing.T) {
	mock := newMockDocker()
	mock.running["my-app"] = true
	mock.health["my-app"] = &docker.ContainerHealth{
		Status:             "",
		HealthCheckDefined: false,
		Ports:              []uint16{},
	}

	mgr := NewManager(mock, nil)
	mgr.schedules["my-app"] = &models.Schedule{
		ContainerName: "my-app",
		OnDemandURL:   "http://my-app.local",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := mgr.CheckHealth(ctx, "my-app")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !result.Healthy {
		t.Error("expected Healthy=true for running container with no ports")
	}
	if result.OnDemandURL != "http://my-app.local" {
		t.Errorf("expected on_demand_url http://my-app.local, got %s", result.OnDemandURL)
	}
}

func TestSelectPort_PrefersURLPort(t *testing.T) {
	port := selectPort("http://example.com:9090", []uint16{8080, 9090})
	if port != 9090 {
		t.Errorf("expected port 9090 from URL, got %d", port)
	}
}

func TestSelectPort_LowestWhenNoURL(t *testing.T) {
	port := selectPort("", []uint16{9090, 8080, 3000})
	if port != 3000 {
		t.Errorf("expected lowest port 3000, got %d", port)
	}
}

func TestSelectPort_LowestWhenNoURLPort(t *testing.T) {
	port := selectPort("http://example.com", []uint16{9090, 8080})
	if port != 8080 {
		t.Errorf("expected lowest port 8080, got %d", port)
	}
}
