package logwatch

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/fabienpiette/schedule-containers/internal/models"
)

// RuleStore is the persistence surface the manager needs.
type RuleStore interface {
	ListEnabledLogRules(ctx context.Context) ([]models.LogRule, error)
	SetLogRuleDisabled(ctx context.Context, id, reason string) error
	TouchLogRuleMatched(ctx context.Context, id string, at time.Time) error
}

// Manager owns one watcher per container that has >=1 enabled rule.
type Manager struct {
	docker DockerClient
	store  RuleStore

	mu       sync.Mutex
	rules    map[string]models.LogRule // ruleID -> rule (enabled only)
	watchers map[string]*watcher       // container -> watcher
	ctx      context.Context
	cancel   context.CancelFunc
}

func NewManager(docker DockerClient, store RuleStore) *Manager {
	return &Manager{
		docker:   docker,
		store:    store,
		rules:    make(map[string]models.LogRule),
		watchers: make(map[string]*watcher),
	}
}

func (m *Manager) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	m.mu.Lock()
	m.ctx = ctx
	m.cancel = cancel
	m.mu.Unlock()

	rules, err := m.store.ListEnabledLogRules(ctx)
	if err != nil {
		cancel()
		return err
	}
	m.mu.Lock()
	containers := make(map[string]bool)
	for _, r := range rules {
		m.rules[r.ID] = r
		containers[r.ContainerName] = true
	}
	for c := range containers {
		m.rebuildLocked(c)
	}
	m.mu.Unlock()
	slog.Info("logwatch: manager started", "rules", len(rules), "containers", len(containers))
	return nil
}

func (m *Manager) Stop() {
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
	}
	watchers := make([]*watcher, 0, len(m.watchers))
	for _, w := range m.watchers {
		watchers = append(watchers, w)
	}
	m.watchers = make(map[string]*watcher)
	m.mu.Unlock()

	for _, w := range watchers {
		w.stop()
	}
	slog.Info("logwatch: manager stopped")
}

// AddRule registers a newly created or re-enabled rule and (re)builds the
// affected container watcher(s).
func (m *Manager) AddRule(rule models.LogRule) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.upsertLocked(rule)
}

// UpdateRule applies an edited rule, rebuilding both the old and new container
// watchers when the rule's container changed.
func (m *Manager) UpdateRule(rule models.LogRule) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.upsertLocked(rule)
}

// upsertLocked stores the rule in the set (or removes it when disabled) and
// rebuilds the watcher for its container, plus the previous container when it
// changed. Caller holds m.mu.
func (m *Manager) upsertLocked(rule models.LogRule) {
	old, existed := m.rules[rule.ID]
	if rule.Enabled {
		m.rules[rule.ID] = rule
	} else {
		delete(m.rules, rule.ID)
	}
	if existed && old.ContainerName != rule.ContainerName {
		m.rebuildLocked(old.ContainerName)
	}
	m.rebuildLocked(rule.ContainerName)
}

func (m *Manager) RemoveRule(ruleID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rules[ruleID]
	if !ok {
		return
	}
	delete(m.rules, ruleID)
	m.rebuildLocked(r.ContainerName)
}

// rebuildLocked stops the container's current watcher and starts a fresh one
// from the current enabled-rule set (or leaves it stopped if none remain).
// Caller holds m.mu.
func (m *Manager) rebuildLocked(container string) {
	if old, ok := m.watchers[container]; ok {
		delete(m.watchers, container)
		go old.stop() // async: never block the manager (and never self-deadlock from a hook)
	}
	if m.ctx == nil || m.ctx.Err() != nil {
		return
	}
	var rules []models.LogRule
	for _, r := range m.rules {
		if r.ContainerName == container {
			rules = append(rules, r)
		}
	}
	if len(rules) == 0 {
		return
	}
	w, err := newWatcher(container, rules, m.docker, m.hooks(), time.Now)
	if err != nil {
		slog.Warn("logwatch: could not build watcher", "container", container, "error", err)
		return
	}
	w.start(m.ctx)
	m.watchers[container] = w
}

func (m *Manager) hooks() watcherHooks {
	return watcherHooks{
		onMatched: func(ruleID string, at time.Time) {
			if err := m.store.TouchLogRuleMatched(context.Background(), ruleID, at); err != nil {
				slog.Warn("logwatch: failed to persist last_matched_at", "rule", ruleID, "error", err)
			}
		},
		onBreakerTrip: func(ruleID, reason string) {
			if err := m.store.SetLogRuleDisabled(context.Background(), ruleID, reason); err != nil {
				slog.Error("logwatch: failed to persist rule disable", "rule", ruleID, "error", err)
			}
			// drop the rule and rebuild its container's watcher, off the watcher goroutine
			go m.RemoveRule(ruleID)
		},
	}
}

func (m *Manager) watcherCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.watchers)
}
