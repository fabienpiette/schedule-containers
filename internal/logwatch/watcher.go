package logwatch

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/fabienpiette/schedule-containers/internal/models"
)

const (
	maxRestarts   = 5
	breakerWindow = 10 * time.Minute
	reattachDelay = 5 * time.Second
)

// DockerClient is the minimal Docker surface the watcher needs.
type DockerClient interface {
	FollowLogs(ctx context.Context, name string, since time.Time) (<-chan string, error)
	RestartContainer(ctx context.Context, name string) error
	IsRunning(ctx context.Context, name string) (bool, error)
}

type watcherHooks struct {
	onMatched     func(ruleID string, at time.Time) // persist LastMatchedAt (may be nil)
	onBreakerTrip func(ruleID, reason string)       // persist disable + rebuild (may be nil)
}

type compiledRule struct {
	rule    models.LogRule
	matcher Matcher
	fires   []time.Time // restart timestamps within breakerWindow; last entry is the most recent fire
}

type watcher struct {
	container string
	rules     []*compiledRule
	docker    DockerClient
	hooks     watcherHooks
	now       func() time.Time

	cancel context.CancelFunc
	done   chan struct{}
}

func newWatcher(container string, rules []models.LogRule, docker DockerClient, hooks watcherHooks, now func() time.Time) (*watcher, error) {
	if now == nil {
		now = time.Now
	}
	var compiled []*compiledRule
	for i := range rules {
		mch, err := NewMatcher(rules[i].Pattern, rules[i].MatchType)
		if err != nil {
			// defensive: create-time validation should prevent this
			slog.Warn("logwatch: skipping rule with invalid pattern", "rule", rules[i].ID, "error", err)
			if hooks.onBreakerTrip != nil {
				hooks.onBreakerTrip(rules[i].ID, fmt.Sprintf("invalid pattern: %v", err))
			}
			continue
		}
		compiled = append(compiled, &compiledRule{rule: rules[i], matcher: mch})
	}
	if len(compiled) == 0 {
		return nil, fmt.Errorf("no valid rules for container %s", container)
	}
	return &watcher{container: container, rules: compiled, docker: docker, hooks: hooks, now: now}, nil
}

// start launches the watcher's background goroutine. It must be called at
// most once per watcher — a second call orphans the first goroutine.
func (w *watcher) start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	w.done = make(chan struct{})
	go func() {
		defer close(w.done)
		w.run(ctx)
	}()
}

func (w *watcher) run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		lines, err := w.docker.FollowLogs(ctx, w.container, w.now())
		if err != nil {
			// container down / missing — back off and retry
			slog.Warn("logwatch: cannot attach to logs, will retry", "container", w.container, "error", err)
			select {
			case <-time.After(reattachDelay):
				continue
			case <-ctx.Done():
				return
			}
		}
		slog.Debug("logwatch: attached", "container", w.container)
		restarted := w.consume(ctx, lines)
		if ctx.Err() != nil {
			return
		}
		if !restarted {
			// stream ended on its own (container stopped) — brief pause before reattach
			select {
			case <-time.After(reattachDelay):
			case <-ctx.Done():
				return
			}
		}
		// restarted==true → immediately reattach with a fresh since=now (stop-and-reattach)
	}
}

// consume reads lines until a restart fires (returns true) or the stream ends (false).
func (w *watcher) consume(ctx context.Context, lines <-chan string) bool {
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				return false
			}
			if w.evaluate(ctx, line) {
				return true // stop-and-reattach: do not read further from this stream
			}
		case <-ctx.Done():
			return false
		}
	}
}

// evaluate tests all rules against a line; fires at most one restart. Returns
// true if a restart was triggered.
func (w *watcher) evaluate(ctx context.Context, line string) bool {
	for _, cr := range w.rules {
		if !cr.matcher.Matches(line) {
			continue
		}
		now := w.now()
		if n := len(cr.fires); n > 0 && now.Sub(cr.fires[n-1]) < cr.rule.Cooldown() {
			slog.Debug("logwatch: match within cooldown, skipping", "container", w.container, "rule", cr.rule.ID)
			continue
		}
		w.fire(ctx, cr, now)
		return true
	}
	return false
}

func (w *watcher) fire(ctx context.Context, cr *compiledRule, now time.Time) {
	slog.Info("logwatch: match, restarting container", "container", w.container, "rule", cr.rule.ID)
	if err := w.docker.RestartContainer(ctx, w.container); err != nil {
		slog.Error("logwatch: restart failed", "container", w.container, "rule", cr.rule.ID, "error", err)
	}
	if w.hooks.onMatched != nil {
		w.hooks.onMatched(cr.rule.ID, now)
	}

	// circuit breaker (per rule)
	cr.fires = append(pruneOld(cr.fires, now.Add(-breakerWindow)), now)
	if len(cr.fires) > maxRestarts {
		reason := fmt.Sprintf("circuit breaker: %d restarts within %s", len(cr.fires), breakerWindow)
		slog.Warn("logwatch: circuit breaker tripped, disabling rule", "container", w.container, "rule", cr.rule.ID, "reason", reason)
		if w.hooks.onBreakerTrip != nil {
			w.hooks.onBreakerTrip(cr.rule.ID, reason)
		}
	}
}

func pruneOld(times []time.Time, cutoff time.Time) []time.Time {
	kept := times[:0]
	for _, t := range times {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	return kept
}

func (w *watcher) stop() {
	if w.cancel != nil {
		w.cancel()
	}
	if w.done != nil {
		select {
		case <-w.done:
		case <-time.After(5 * time.Second):
			slog.Warn("logwatch: timed out waiting for watcher to stop", "container", w.container)
		}
	}
}
