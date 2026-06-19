package ondemand

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/fabienpiette/schedule-containers/internal/docker"
)

const (
	idleCPUPercent   = 0.1
	idleNetworkBytes = 1024
	statsPollInterval = 5 * time.Second
	idleCheckInterval = 5 * time.Second
)

type idleTracker struct {
	containerName string
	timeout       time.Duration
	docker        OnDemandDockerClient
	done          chan struct{}
	cancel        context.CancelFunc
	lastActive    time.Time
	mu            sync.Mutex
}

func newIdleTracker(containerName string, timeout time.Duration, docker OnDemandDockerClient) *idleTracker {
	if timeout == 0 {
		return nil
	}
	return &idleTracker{
		containerName: containerName,
		timeout:       timeout,
		docker:        docker,
		lastActive:    time.Now(),
	}
}

func (t *idleTracker) start(ctx context.Context, onStop func()) {
	ctx, cancel := context.WithCancel(ctx)
	t.cancel = cancel
	t.done = make(chan struct{})

	go func() {
		defer close(t.done)

		statsCh, err := t.docker.ContainerStats(ctx, t.containerName)
		if err != nil {
			slog.Warn("on-demand: failed to start stats stream", "container", t.containerName, "error", err)
			return
		}

		ticker := time.NewTicker(idleCheckInterval)
		defer ticker.Stop()

		for {
			select {
			case snap, ok := <-statsCh:
				if !ok {
					slog.Debug("on-demand: stats stream closed", "container", t.containerName)
					return
				}
				if snap.CPUPercent > idleCPUPercent || (snap.NetworkRxBytes+snap.NetworkTxBytes) > idleNetworkBytes {
					t.updateActivity()
				}

			case <-ticker.C:
				t.mu.Lock()
				if time.Since(t.lastActive) > t.timeout {
					t.mu.Unlock()
					slog.Info("on-demand: container idle, stopping", "container", t.containerName)
					if err := t.docker.StopContainer(ctx, t.containerName); err != nil {
						slog.Warn("on-demand: failed to stop idle container", "container", t.containerName, "error", err)
					}
					if onStop != nil {
						onStop()
					}
					return
				}
				t.mu.Unlock()

			case <-ctx.Done():
				slog.Debug("on-demand: idle tracker context cancelled", "container", t.containerName)
				return
			}
		}
	}()
}

func (t *idleTracker) stop() {
	if t.cancel != nil {
		t.cancel()
	}
	if t.done != nil {
		select {
		case <-t.done:
		case <-time.After(5 * time.Second):
			slog.Warn("on-demand: timed out waiting for idle tracker to stop", "container", t.containerName)
		}
	}
}

func (t *idleTracker) isActive() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return time.Since(t.lastActive) < t.timeout
}

func (t *idleTracker) updateActivity() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lastActive = time.Now()
}

func isStatsActive(snap docker.StatsSnapshot) bool {
	return snap.CPUPercent > idleCPUPercent || (snap.NetworkRxBytes+snap.NetworkTxBytes) > idleNetworkBytes
}
