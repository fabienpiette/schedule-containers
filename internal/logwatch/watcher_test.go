package logwatch

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/fabienpiette/schedule-containers/internal/models"
)

type mockDocker struct {
	mu        sync.Mutex
	restarts  []string
	lines     chan string
	openCount int
	running   bool
}

func newMockDocker() *mockDocker {
	return &mockDocker{lines: make(chan string, 64), running: true}
}

func (m *mockDocker) FollowLogs(ctx context.Context, name string, since time.Time) (<-chan string, error) {
	m.mu.Lock()
	m.openCount++
	m.mu.Unlock()
	return m.lines, nil
}

func (m *mockDocker) RestartContainer(ctx context.Context, name string) error {
	m.mu.Lock()
	m.restarts = append(m.restarts, name)
	// a real restart ends the current stream; simulate by replacing the channel
	close(m.lines)
	m.lines = make(chan string, 64)
	m.mu.Unlock()
	return nil
}

func (m *mockDocker) IsRunning(ctx context.Context, name string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running, nil
}

func (m *mockDocker) restartCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.restarts)
}

func (m *mockDocker) send(line string) {
	m.mu.Lock()
	ch := m.lines
	m.mu.Unlock()
	ch <- line
}

func TestWatcher_RestartsOnMatch(t *testing.T) {
	m := newMockDocker()
	rule := models.LogRule{ID: "r1", ContainerName: "web", Pattern: "boom", MatchType: models.MatchSubstring, Enabled: true, CooldownSec: 60}

	matched := make(chan string, 4)
	w, err := newWatcher("web", []models.LogRule{rule}, m, watcherHooks{
		onMatched: func(id string, at time.Time) { matched <- id },
	}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	w.start(context.Background())
	defer w.stop()

	m.send("all fine")
	m.send("boom happened")

	select {
	case id := <-matched:
		if id != "r1" {
			t.Fatalf("expected r1, got %s", id)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("expected a restart")
	}
	if m.restartCount() != 1 {
		t.Fatalf("expected 1 restart, got %d", m.restartCount())
	}
}

func TestWatcher_CooldownSuppresses(t *testing.T) {
	m := newMockDocker()
	rule := models.LogRule{ID: "r1", ContainerName: "web", Pattern: "boom", MatchType: models.MatchSubstring, Enabled: true, CooldownSec: 300}

	// controllable clock
	var mu sync.Mutex
	nowT := time.Now()
	clock := func() time.Time { mu.Lock(); defer mu.Unlock(); return nowT }

	matched := make(chan string, 4)
	w, _ := newWatcher("web", []models.LogRule{rule}, m, watcherHooks{
		onMatched: func(id string, at time.Time) { matched <- id },
	}, clock)
	w.start(context.Background())
	defer w.stop()

	m.send("boom 1")
	<-matched // first restart
	// second match within cooldown window (clock not advanced) must be suppressed
	m.send("boom 2")
	select {
	case <-matched:
		t.Fatal("cooldown should have suppressed the second restart")
	case <-time.After(500 * time.Millisecond):
	}
	if m.restartCount() != 1 {
		t.Fatalf("expected 1 restart under cooldown, got %d", m.restartCount())
	}
}

func TestWatcher_BreakerTrips(t *testing.T) {
	m := newMockDocker()
	rule := models.LogRule{ID: "r1", ContainerName: "web", Pattern: "boom", MatchType: models.MatchSubstring, Enabled: true, CooldownSec: 0}

	tripped := make(chan string, 1)
	w, _ := newWatcher("web", []models.LogRule{rule}, m, watcherHooks{
		onBreakerTrip: func(id, reason string) { tripped <- id },
	}, time.Now)
	w.start(context.Background())
	defer w.stop()

	// cooldown 0 → each "boom" restarts; after maxRestarts+1 the breaker trips
	for i := 0; i < maxRestarts+1; i++ {
		m.send("boom")
		time.Sleep(20 * time.Millisecond)
	}

	select {
	case id := <-tripped:
		if id != "r1" {
			t.Fatalf("expected r1 tripped, got %s", id)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("expected breaker to trip")
	}
}
