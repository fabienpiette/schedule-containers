package ondemand

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"sort"
	"sync"
	"time"

	"github.com/gndm/schedule-containers/internal/docker"
	"github.com/gndm/schedule-containers/internal/models"
	"github.com/gndm/schedule-containers/internal/store"
)

var ErrScheduleNotFound = errors.New("no on-demand schedule found for container")

type OnDemandDockerClient interface {
	StartContainer(ctx context.Context, name string) error
	StopContainer(ctx context.Context, name string) error
	IsRunning(ctx context.Context, name string) (bool, error)
	InspectContainer(ctx context.Context, name string) (*docker.ContainerHealth, error)
	ContainerStats(ctx context.Context, name string) (<-chan docker.StatsSnapshot, error)
	ListContainers(ctx context.Context) ([]models.Container, error)
}

type WakeResult struct {
	Running     bool   `json:"running"`
	OnDemandURL string `json:"on_demand_url"`
}

type HealthResult struct {
	Healthy     bool   `json:"healthy"`
	OnDemandURL string `json:"on_demand_url"`
}

type OnDemandManager struct {
	docker        OnDemandDockerClient
	store         *store.Store
	mu            sync.Mutex
	wakeMu        map[string]*sync.Mutex
	trackers      map[string]*idleTracker
	schedules     map[string]*models.Schedule
	stacks        map[string]*models.Stack  // keyed by stack Name (compose project)
	stackTrackers map[string]*idleTracker   // keyed by stack ID
	cancel        context.CancelFunc
}

func NewManager(docker OnDemandDockerClient, store *store.Store) *OnDemandManager {
	return &OnDemandManager{
		docker:        docker,
		store:         store,
		wakeMu:        make(map[string]*sync.Mutex),
		trackers:      make(map[string]*idleTracker),
		schedules:     make(map[string]*models.Schedule),
		stacks:        make(map[string]*models.Stack),
		stackTrackers: make(map[string]*idleTracker),
	}
}

func (m *OnDemandManager) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	m.cancel = cancel

	schedules, err := m.store.ListSchedules(ctx)
	if err != nil {
		cancel()
		return fmt.Errorf("failed to list schedules: %w", err)
	}

	m.mu.Lock()
	for i := range schedules {
		s := &schedules[i]
		if !s.OnDemandEnabled {
			continue
		}
		m.schedules[s.ContainerName] = s
		slog.Info("on-demand: loaded schedule", "container", s.ContainerName, "on_demand_url", s.OnDemandURL, "idle_timeout_sec", s.IdleTimeoutSec)

		if s.IdleTimeoutSec > 0 {
			running, err := m.docker.IsRunning(ctx, s.ContainerName)
			if err != nil {
				slog.Warn("on-demand: failed to check running state", "container", s.ContainerName, "error", err)
				continue
			}
			if running {
				tracker := newIdleTracker(s.ContainerName, s.IdleTimeout(), m.docker)
				if tracker != nil {
					containerName := s.ContainerName
					tracker.start(ctx, func() {
						m.mu.Lock()
						delete(m.trackers, containerName)
						delete(m.schedules, containerName)
						m.mu.Unlock()
					})
					m.trackers[s.ContainerName] = tracker
					slog.Info("on-demand: started idle tracker", "container", s.ContainerName, "timeout", s.IdleTimeout())
				}
			}
		}
	}
	m.mu.Unlock()

	go m.listenDockerEvents(ctx)

	slog.Info("on-demand: manager started")
	return nil
}

func (m *OnDemandManager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}

	m.mu.Lock()
	trackers := make(map[string]*idleTracker, len(m.trackers))
	for k, v := range m.trackers {
		trackers[k] = v
	}
	m.mu.Unlock()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for _, t := range trackers {
			t.stop()
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		slog.Warn("on-demand: timed out waiting for tracker goroutines to exit")
	}

	slog.Info("on-demand: manager stopped")
}

func (m *OnDemandManager) Watch(schedule *models.Schedule) {
	m.mu.Lock()

	m.schedules[schedule.ContainerName] = schedule
	slog.Info("on-demand: watching schedule", "container", schedule.ContainerName)

	var oldTracker *idleTracker
	if schedule.IdleTimeoutSec > 0 {
		if existing, ok := m.trackers[schedule.ContainerName]; ok {
			oldTracker = existing
			delete(m.trackers, schedule.ContainerName)
		}
	}

	m.mu.Unlock()

	if oldTracker != nil {
		oldTracker.stop()
	}

	if schedule.IdleTimeoutSec > 0 {
		running, err := m.docker.IsRunning(context.Background(), schedule.ContainerName)
		if err != nil {
			slog.Warn("on-demand: failed to check running state for idle tracker", "container", schedule.ContainerName, "error", err)
			return
		}
		if !running {
			slog.Debug("on-demand: container not running, skipping idle tracker", "container", schedule.ContainerName)
			return
		}

		tracker := newIdleTracker(schedule.ContainerName, schedule.IdleTimeout(), m.docker)
		if tracker != nil {
			containerName := schedule.ContainerName
			tracker.start(context.Background(), func() {
				m.mu.Lock()
				delete(m.trackers, containerName)
				delete(m.schedules, containerName)
				m.mu.Unlock()
			})
			m.mu.Lock()
			m.trackers[schedule.ContainerName] = tracker
			m.mu.Unlock()
			slog.Info("on-demand: started idle tracker", "container", schedule.ContainerName, "timeout", schedule.IdleTimeout())
		}
	}
}

func (m *OnDemandManager) Unwatch(containerName string) {
	m.mu.Lock()
	var tracker *idleTracker
	if t, ok := m.trackers[containerName]; ok {
		tracker = t
		delete(m.trackers, containerName)
	}
	delete(m.schedules, containerName)
	m.mu.Unlock()

	if tracker != nil {
		tracker.stop()
	}
	slog.Info("on-demand: unwatched container", "container", containerName)
}

func (m *OnDemandManager) WakeContainer(ctx context.Context, containerName string) (*WakeResult, error) {
	wakeLock := m.getOrCreateWakeLock(containerName)
	wakeLock.Lock()
	defer wakeLock.Unlock()

	m.mu.Lock()
	schedule, ok := m.schedules[containerName]
	m.mu.Unlock()

	if !ok {
		return nil, ErrScheduleNotFound
	}

	running, err := m.docker.IsRunning(ctx, containerName)
	if err != nil {
		return nil, fmt.Errorf("failed to check container status: %w", err)
	}

	if running {
		slog.Info("on-demand: container already running", "container", containerName)
		return &WakeResult{Running: true, OnDemandURL: schedule.OnDemandURL}, nil
	}

	if err := m.docker.StartContainer(ctx, containerName); err != nil {
		return nil, fmt.Errorf("failed to start container: %w", err)
	}

	slog.Info("on-demand: started container", "container", containerName)
	return &WakeResult{Running: false, OnDemandURL: schedule.OnDemandURL}, nil
}

func (m *OnDemandManager) checkHealthFor(ctx context.Context, containerName string, schedule *models.Schedule) (*HealthResult, error) {
	health, err := m.docker.InspectContainer(ctx, containerName)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %w", err)
	}

	slog.Info("on-demand: health check",
		"container", containerName,
		"healthcheck_defined", health.HealthCheckDefined,
		"status", health.Status,
		"ports", health.Ports,
		"host_ports", health.HostPorts,
	)

	if health.HealthCheckDefined && health.Status == "healthy" {
		return &HealthResult{Healthy: true, OnDemandURL: schedule.OnDemandURL}, nil
	}

	if !health.HealthCheckDefined {
		if len(health.Ports) > 0 {
			port := selectPort(schedule.OnDemandURL, health.Ports)
			addrs := buildProbeAddrs(health, containerName, port)
			for _, addr := range addrs {
				slog.Info("on-demand: TCP probe", "container", containerName, "addr", addr)
				conn, err := net.DialTimeout("tcp", addr, 1*time.Second)
				if err == nil {
					conn.Close()
					slog.Info("on-demand: TCP probe succeeded", "container", containerName, "addr", addr)
					return &HealthResult{Healthy: true, OnDemandURL: schedule.OnDemandURL}, nil
				}
				slog.Info("on-demand: TCP probe failed", "container", containerName, "addr", addr, "error", err)
			}
		}

		// TCP probe unavailable or failed; use startup delay if configured
		if schedule.StartupDelaySec > 0 && health.StartedAt != nil {
			uptime := time.Since(*health.StartedAt)
			delay := time.Duration(schedule.StartupDelaySec) * time.Second
			slog.Info("on-demand: startup delay check", "container", containerName,
				"uptime", uptime.Round(time.Second), "delay", delay)
			if uptime >= delay {
				return &HealthResult{Healthy: true, OnDemandURL: schedule.OnDemandURL}, nil
			}
			return &HealthResult{Healthy: false, OnDemandURL: schedule.OnDemandURL}, nil
		}

		// No ports, no startup delay: wait for running state + brief grace period
		running, err := m.docker.IsRunning(ctx, containerName)
		if err != nil {
			return nil, fmt.Errorf("failed to check container running state: %w", err)
		}
		if running {
			select {
			case <-time.After(3 * time.Second):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return &HealthResult{Healthy: true, OnDemandURL: schedule.OnDemandURL}, nil
		}
		return &HealthResult{Healthy: false, OnDemandURL: schedule.OnDemandURL}, nil
	}

	if health.HealthCheckDefined && health.Status == "unhealthy" {
		return &HealthResult{Healthy: false, OnDemandURL: schedule.OnDemandURL}, nil
	}

	if health.HealthCheckDefined && health.Status == "starting" {
		return &HealthResult{Healthy: false, OnDemandURL: schedule.OnDemandURL}, nil
	}

	return &HealthResult{Healthy: false, OnDemandURL: schedule.OnDemandURL}, nil
}

func (m *OnDemandManager) CheckHealth(ctx context.Context, containerName string) (*HealthResult, error) {
	m.mu.Lock()
	schedule, ok := m.schedules[containerName]
	m.mu.Unlock()

	if !ok {
		return nil, ErrScheduleNotFound
	}

	return m.checkHealthFor(ctx, containerName, schedule)
}

func (m *OnDemandManager) getOrCreateWakeLock(containerName string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()

	if lock, ok := m.wakeMu[containerName]; ok {
		return lock
	}
	lock := &sync.Mutex{}
	m.wakeMu[containerName] = lock
	return lock
}

func (m *OnDemandManager) listenDockerEvents(ctx context.Context) {
	slog.Info("on-demand: Docker events listener started")
	<-ctx.Done()
	slog.Info("on-demand: Docker events listener stopped")
}

func selectPort(onDemandURL string, ports []uint16) uint16 {
	if onDemandURL != "" {
		u, err := url.Parse(onDemandURL)
		if err == nil {
			portStr := u.Port()
			if portStr != "" {
				var p uint16
				if _, err := fmt.Sscanf(portStr, "%d", &p); err == nil && p > 0 {
					return p
				}
			}
		}
	}

	sorted := make([]uint16, len(ports))
	copy(sorted, ports)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return sorted[0]
}

var ErrStackNotFound = errors.New("no on-demand stack found")

func (m *OnDemandManager) AddStack(stack *models.Stack) {
	m.mu.Lock()
	m.stacks[stack.Name] = stack
	slog.Info("on-demand: watching stack", "stack", stack.Name, "primary", stack.PrimaryContainer)

	var oldTracker *idleTracker
	if stack.IdleTimeoutSec > 0 {
		if existing, ok := m.stackTrackers[stack.ID]; ok {
			oldTracker = existing
			delete(m.stackTrackers, stack.ID)
		}
	}
	m.mu.Unlock()

	if oldTracker != nil {
		oldTracker.stop()
	}

	if !stack.OnDemandEnabled || stack.PrimaryContainer == "" || stack.IdleTimeoutSec <= 0 {
		return
	}

	running, err := m.docker.IsRunning(context.Background(), stack.PrimaryContainer)
	if err != nil {
		slog.Warn("on-demand: failed to check stack primary running state", "stack", stack.Name, "primary", stack.PrimaryContainer, "error", err)
		return
	}
	if !running {
		return
	}

	tracker := newIdleTracker(stack.PrimaryContainer, stack.IdleTimeout(), m.docker)
	if tracker == nil {
		return
	}

	stackName := stack.Name
	stackID := stack.ID
	tracker.start(context.Background(), func() {
		containers, err := m.docker.ListContainers(context.Background())
		if err != nil {
			slog.Error("on-demand: stack idle: failed to list containers", "stack", stackName, "error", err)
			return
		}
		for _, c := range containers {
			if c.StackName != stackName {
				continue
			}
			if err := m.docker.StopContainer(context.Background(), c.Name); err != nil {
				slog.Warn("on-demand: stack idle: failed to stop container", "stack", stackName, "container", c.Name, "error", err)
			} else {
				slog.Info("on-demand: stack idle: stopped container", "stack", stackName, "container", c.Name)
			}
		}
		m.mu.Lock()
		delete(m.stackTrackers, stackID)
		delete(m.stacks, stackName)
		m.mu.Unlock()
	})

	m.mu.Lock()
	m.stackTrackers[stack.ID] = tracker
	m.mu.Unlock()
	slog.Info("on-demand: started stack idle tracker", "stack", stack.Name, "primary", stack.PrimaryContainer, "timeout", stack.IdleTimeout())
}

func (m *OnDemandManager) RemoveStack(stackID string) {
	m.mu.Lock()
	var tracker *idleTracker
	if t, ok := m.stackTrackers[stackID]; ok {
		tracker = t
		delete(m.stackTrackers, stackID)
	}
	for name, st := range m.stacks {
		if st.ID == stackID {
			delete(m.stacks, name)
			break
		}
	}
	m.mu.Unlock()

	if tracker != nil {
		tracker.stop()
	}
	slog.Info("on-demand: removed stack", "stack_id", stackID)
}

func (m *OnDemandManager) WakeStack(ctx context.Context, stackName string) (*WakeResult, error) {
	m.mu.Lock()
	stack, ok := m.stacks[stackName]
	m.mu.Unlock()

	if !ok {
		return nil, ErrStackNotFound
	}

	containers, err := m.docker.ListContainers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list containers for stack %q: %w", stackName, err)
	}

	allRunning := true
	for _, c := range containers {
		if c.StackName != stackName {
			continue
		}
		if c.State != "running" {
			allRunning = false
			if err := m.docker.StartContainer(ctx, c.Name); err != nil {
				return nil, fmt.Errorf("failed to start container %q in stack %q: %w", c.Name, stackName, err)
			}
			slog.Info("on-demand: started stack container", "stack", stackName, "container", c.Name)
		}
	}

	if allRunning {
		slog.Info("on-demand: stack already running", "stack", stackName)
		return &WakeResult{Running: true, OnDemandURL: stack.OnDemandURL}, nil
	}

	return &WakeResult{Running: false, OnDemandURL: stack.OnDemandURL}, nil
}

func (m *OnDemandManager) CheckStackHealth(ctx context.Context, stackName string) (*HealthResult, error) {
	m.mu.Lock()
	stack, ok := m.stacks[stackName]
	m.mu.Unlock()

	if !ok {
		return nil, ErrStackNotFound
	}

	primarySched := &models.Schedule{
		ContainerName:   stack.PrimaryContainer,
		OnDemandURL:     stack.OnDemandURL,
		StartupDelaySec: stack.StartupDelaySec,
		IdleTimeoutSec:  stack.IdleTimeoutSec,
	}

	return m.checkHealthFor(ctx, stack.PrimaryContainer, primarySched)
}

// buildProbeAddrs returns TCP addresses to try in order:
// 1. container IP + container port (works from host and from any Docker container)
// 2. 127.0.0.1 + host port (works when running directly on the host with userland proxy)
// 3. container name + container port (works within the same Docker network)
func buildProbeAddrs(health *docker.ContainerHealth, containerName string, port uint16) []string {
	portStr := fmt.Sprintf("%d", port)
	var addrs []string

	if health.ContainerIP != "" {
		addrs = append(addrs, net.JoinHostPort(health.ContainerIP, portStr))
	}
	if len(health.HostPorts) > 0 {
		hp := selectPort("", health.HostPorts)
		addrs = append(addrs, net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", hp)))
	}
	addrs = append(addrs, net.JoinHostPort(containerName, portStr))
	return addrs
}
