package logwatch

import (
	"context"
	"testing"
	"time"

	"github.com/fabienpiette/schedule-containers/internal/models"
)

type fakeStore struct{ disabled []string }

func (f *fakeStore) ListEnabledLogRules(ctx context.Context) ([]models.LogRule, error) {
	return nil, nil
}
func (f *fakeStore) SetLogRuleDisabled(ctx context.Context, id, reason string) error {
	f.disabled = append(f.disabled, id)
	return nil
}
func (f *fakeStore) TouchLogRuleMatched(ctx context.Context, id string, at time.Time) error {
	return nil
}

func TestManager_AddAndRemoveRule(t *testing.T) {
	m := newMockDocker()
	mgr := NewManager(m, &fakeStore{})
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer mgr.Stop()

	rule := models.LogRule{ID: "r1", ContainerName: "web", Pattern: "boom", MatchType: models.MatchSubstring, Enabled: true, CooldownSec: 60}
	mgr.AddRule(rule)

	// a watcher should now exist for "web" and react to a matching line
	time.Sleep(50 * time.Millisecond)
	m.send("boom now")
	deadline := time.After(3 * time.Second)
	for m.restartCount() == 0 {
		select {
		case <-deadline:
			t.Fatal("expected watcher to restart the container")
		case <-time.After(20 * time.Millisecond):
		}
	}

	// removing the last rule stops the watcher (no panic, container no longer watched)
	mgr.RemoveRule("r1")
	if mgr.watcherCount() != 0 {
		t.Fatalf("expected 0 watchers after removing last rule, got %d", mgr.watcherCount())
	}
}
