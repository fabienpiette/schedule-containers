package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/gndm/schedule-containers/internal/models"
	"github.com/robfig/cron/v3"
)

type DockerActionClient interface {
	StartContainer(ctx context.Context, name string) error
	StopContainer(ctx context.Context, name string) error
}

type Scheduler struct {
	cron   *cron.Cron
	docker DockerActionClient
	mu     sync.Mutex
	jobs   map[string][]cron.EntryID
	locks  map[string]*sync.Mutex
}

func NewScheduler(docker DockerActionClient) *Scheduler {
	return &Scheduler{
		cron:   cron.New(),
		docker: docker,
		jobs:   make(map[string][]cron.EntryID),
		locks:  make(map[string]*sync.Mutex),
	}
}

func (s *Scheduler) Start() {
	s.cron.Start()
}

func (s *Scheduler) Stop() {
	s.cron.Stop()
}

func (s *Scheduler) AddSchedule(schedule *models.Schedule) error {
	if !schedule.Enabled {
		slog.Info("schedule disabled, not registering cron jobs", "schedule_id", schedule.ID, "container", schedule.ContainerName)
		return nil
	}

	if err := ValidateCronExpression(schedule.StartCron); err != nil {
		return fmt.Errorf("invalid start cron expression: %w", err)
	}
	if err := ValidateCronExpression(schedule.StopCron); err != nil {
		return fmt.Errorf("invalid stop cron expression: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	containerLock := s.getOrCreateLock(schedule.ContainerName)

	startID, err := s.cron.AddFunc(schedule.StartCron, func() {
		containerLock.Lock()
		defer containerLock.Unlock()
		slog.Info("cron fired: starting container", "container", schedule.ContainerName, "schedule_id", schedule.ID)
		if err := s.docker.StartContainer(context.Background(), schedule.ContainerName); err != nil {
			slog.Error("failed to start container", "container", schedule.ContainerName, "error", err)
		}
	})
	if err != nil {
		return fmt.Errorf("failed to add start cron job: %w", err)
	}

	stopID, err := s.cron.AddFunc(schedule.StopCron, func() {
		containerLock.Lock()
		defer containerLock.Unlock()
		slog.Info("cron fired: stopping container", "container", schedule.ContainerName, "schedule_id", schedule.ID)
		if err := s.docker.StopContainer(context.Background(), schedule.ContainerName); err != nil {
			slog.Error("failed to stop container", "container", schedule.ContainerName, "error", err)
		}
	})
	if err != nil {
		s.cron.Remove(startID)
		return fmt.Errorf("failed to add stop cron job: %w", err)
	}

	s.jobs[schedule.ID] = []cron.EntryID{startID, stopID}
	slog.Info("registered cron jobs", "schedule_id", schedule.ID, "container", schedule.ContainerName, "start_cron", schedule.StartCron, "stop_cron", schedule.StopCron)
	return nil
}

func (s *Scheduler) RemoveSchedule(scheduleID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ids, ok := s.jobs[scheduleID]
	if !ok {
		return nil
	}

	for _, id := range ids {
		s.cron.Remove(id)
	}
	delete(s.jobs, scheduleID)
	slog.Info("removed cron jobs", "schedule_id", scheduleID)
	return nil
}

func (s *Scheduler) ScheduleCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.jobs)
}

func (s *Scheduler) getOrCreateLock(containerName string) *sync.Mutex {
	if lock, ok := s.locks[containerName]; ok {
		return lock
	}
	lock := &sync.Mutex{}
	s.locks[containerName] = lock
	return lock
}

func ValidateCronExpression(expr string) error {
	_, err := cron.ParseStandard(expr)
	if err != nil {
		return fmt.Errorf("invalid cron expression: %w", err)
	}
	return nil
}