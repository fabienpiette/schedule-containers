package ondemand

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gndm/schedule-containers/internal/docker"
)

type mockStatsDocker struct {
	mu       sync.Mutex
	stopped  []string
	statsCh  chan docker.StatsSnapshot
	statsErr error
}

func newMockStatsDocker() *mockStatsDocker {
	return &mockStatsDocker{
		statsCh: make(chan docker.StatsSnapshot, 20),
	}
}

func (m *mockStatsDocker) StartContainer(ctx context.Context, name string) error {
	return nil
}

func (m *mockStatsDocker) StopContainer(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = append(m.stopped, name)
	return nil
}

func (m *mockStatsDocker) IsRunning(ctx context.Context, name string) (bool, error) {
	return true, nil
}

func (m *mockStatsDocker) InspectContainer(ctx context.Context, name string) (*docker.ContainerHealth, error) {
	return &docker.ContainerHealth{}, nil
}

func (m *mockStatsDocker) ContainerStats(ctx context.Context, name string) (<-chan docker.StatsSnapshot, error) {
	if m.statsErr != nil {
		return nil, m.statsErr
	}
	return m.statsCh, nil
}

func (m *mockStatsDocker) getStopped() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.stopped))
	copy(result, m.stopped)
	return result
}

func TestIdleTracker_StopsContainer(t *testing.T) {
	mock := newMockStatsDocker()
	timeout := 5 * time.Second
	tracker := newIdleTracker("test-container", timeout, mock)
	if tracker == nil {
		t.Fatal("expected non-nil tracker")
	}

	stopCalled := make(chan struct{}, 1)
	tracker.start(context.Background(), func() {
		select {
		case stopCalled <- struct{}{}:
		default:
		}
	})

	mock.statsCh <- docker.StatsSnapshot{
		CPUPercent:     5.0,
		NetworkRxBytes: 5000,
		NetworkTxBytes: 3000,
		Timestamp:      time.Now(),
	}

	for i := 0; i < 3; i++ {
		mock.statsCh <- docker.StatsSnapshot{
			CPUPercent:     0.01,
			NetworkRxBytes: 100,
			NetworkTxBytes: 50,
			Timestamp:      time.Now(),
		}
	}

	tracker.mu.Lock()
	tracker.lastActive = time.Now().Add(-timeout - 1*time.Second)
	tracker.mu.Unlock()

	select {
	case <-stopCalled:
	case <-time.After(10 * time.Second):
		t.Fatal("expected StopContainer to be called")
	}

	stopped := mock.getStopped()
	if len(stopped) != 1 || stopped[0] != "test-container" {
		t.Errorf("expected StopContainer called with test-container, got %v", stopped)
	}
}

func TestIdleTracker_ResetsOnActivity(t *testing.T) {
	mock := newMockStatsDocker()
	timeout := 10 * time.Second
	tracker := newIdleTracker("test-container", timeout, mock)

	stopCalled := make(chan struct{}, 1)
	tracker.start(context.Background(), func() {
		select {
		case stopCalled <- struct{}{}:
		default:
		}
	})

	mock.statsCh <- docker.StatsSnapshot{
		CPUPercent:     0.01,
		NetworkRxBytes: 100,
		NetworkTxBytes: 50,
		Timestamp:      time.Now(),
	}

	tracker.mu.Lock()
	tracker.lastActive = time.Now().Add(-timeout - 1*time.Second)
	tracker.mu.Unlock()

	if tracker.isActive() {
		t.Error("expected tracker to be inactive after setting lastActive far in past")
	}

	mock.statsCh <- docker.StatsSnapshot{
		CPUPercent:     5.0,
		NetworkRxBytes: 5000,
		NetworkTxBytes: 3000,
		Timestamp:      time.Now(),
	}

	time.Sleep(50 * time.Millisecond)

	if !tracker.isActive() {
		t.Error("expected tracker to be active after receiving active snapshot")
	}

	tracker.stop()

	select {
	case <-stopCalled:
		t.Error("did not expect StopContainer to be called")
	default:
	}
}

func TestIdleTracker_ZeroTimeout(t *testing.T) {
	mock := newMockStatsDocker()
	tracker := newIdleTracker("test-container", 0, mock)
	if tracker != nil {
		t.Error("expected nil tracker when timeout is 0")
	}
}

func TestIdleTracker_StatsStreamError(t *testing.T) {
	mock := newMockStatsDocker()
	mock.statsErr = errors.New("stats unavailable")

	tracker := newIdleTracker("test-container", 5*time.Second, mock)
	if tracker == nil {
		t.Fatal("expected non-nil tracker")
	}

	tracker.start(context.Background(), nil)

	time.Sleep(100 * time.Millisecond)

	stopped := mock.getStopped()
	if len(stopped) != 0 {
		t.Errorf("expected no StopContainer calls, got %v", stopped)
	}
}

func TestIdleTracker_ContextCancellation(t *testing.T) {
	mock := newMockStatsDocker()
	tracker := newIdleTracker("test-container", 5*time.Second, mock)

	tracker.start(context.Background(), nil)

	time.Sleep(50 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		tracker.stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("expected tracker to stop on context cancellation")
	}

	stopped := mock.getStopped()
	if len(stopped) != 0 {
		t.Errorf("expected no StopContainer calls on context cancellation, got %v", stopped)
	}
}

func TestIdleTracker_StatsChannelClosed(t *testing.T) {
	mock := newMockStatsDocker()
	tracker := newIdleTracker("test-container", 5*time.Second, mock)

	tracker.start(context.Background(), nil)

	close(mock.statsCh)

	tracker.stop()

	stopped := mock.getStopped()
	if len(stopped) != 0 {
		t.Errorf("expected no StopContainer calls when stats channel closes, got %v", stopped)
	}
}

func TestIsStatsActive(t *testing.T) {
	tests := []struct {
		name string
		snap docker.StatsSnapshot
		want bool
	}{
		{
			name: "active CPU",
			snap: docker.StatsSnapshot{CPUPercent: 1.0},
			want: true,
		},
		{
			name: "active network",
			snap: docker.StatsSnapshot{NetworkRxBytes: 2000, NetworkTxBytes: 0},
			want: true,
		},
		{
			name: "idle",
			snap: docker.StatsSnapshot{CPUPercent: 0.01, NetworkRxBytes: 100, NetworkTxBytes: 50},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isStatsActive(tt.snap)
			if got != tt.want {
				t.Errorf("isStatsActive() = %v, want %v", got, tt.want)
			}
		})
	}
}
