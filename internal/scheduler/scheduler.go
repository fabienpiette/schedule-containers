package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/fabienpiette/schedule-containers/internal/models"
	"github.com/robfig/cron/v3"
)

type DockerActionClient interface {
	StartContainer(ctx context.Context, name string) error
	StopContainer(ctx context.Context, name string) error
	ListContainers(ctx context.Context) ([]models.Container, error)
}

type Scheduler struct {
	cron   *cron.Cron
	docker DockerActionClient
	mu     sync.Mutex
	jobs   map[string][]cron.EntryID
	locks  map[string]*sync.Mutex
	tz     string
}

func NewScheduler(docker DockerActionClient, timezone string) *Scheduler {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		slog.Warn("invalid timezone, falling back to UTC", "timezone", timezone, "error", err)
		timezone = "UTC"
		loc = time.UTC
	}

	c := cron.New(cron.WithLocation(loc))
	slog.Info("scheduler timezone configured", "timezone", timezone)

	return &Scheduler{
		cron:   c,
		docker: docker,
		jobs:   make(map[string][]cron.EntryID),
		locks:  make(map[string]*sync.Mutex),
		tz:     timezone,
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

	if schedule.StartCron == "" && schedule.StopCron == "" {
		slog.Info("schedule has no cron expressions, not registering", "schedule_id", schedule.ID, "container", schedule.ContainerName)
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	containerLock := s.getOrCreateLock(schedule.ContainerName)
	var entryIDs []cron.EntryID

	if schedule.StartCron != "" {
		if err := ValidateCronExpression(schedule.StartCron); err != nil {
			return fmt.Errorf("invalid start cron expression: %w", err)
		}
		startID, err := s.cron.AddFunc(schedule.StartCron, func() {
			containerLock.Lock()
			defer containerLock.Unlock()
			slog.Info("cron fired: starting container", "container", schedule.ContainerName, "schedule_id", schedule.ID, "cron", schedule.StartCron)
			if err := s.docker.StartContainer(context.Background(), schedule.ContainerName); err != nil {
				slog.Error("failed to start container", "container", schedule.ContainerName, "error", err)
			} else {
				slog.Info("started container", "container", schedule.ContainerName)
			}
		})
		if err != nil {
			return fmt.Errorf("failed to add start cron job: %w", err)
		}
		entryIDs = append(entryIDs, startID)
	}

	if schedule.StopCron != "" {
		if err := ValidateCronExpression(schedule.StopCron); err != nil {
			for _, id := range entryIDs {
				s.cron.Remove(id)
			}
			return fmt.Errorf("invalid stop cron expression: %w", err)
		}
		stopID, err := s.cron.AddFunc(schedule.StopCron, func() {
			containerLock.Lock()
			defer containerLock.Unlock()
			slog.Info("cron fired: stopping container", "container", schedule.ContainerName, "schedule_id", schedule.ID, "cron", schedule.StopCron)
			if err := s.docker.StopContainer(context.Background(), schedule.ContainerName); err != nil {
				slog.Error("failed to stop container", "container", schedule.ContainerName, "error", err)
			} else {
				slog.Info("stopped container", "container", schedule.ContainerName)
			}
		})
		if err != nil {
			for _, id := range entryIDs {
				s.cron.Remove(id)
			}
			return fmt.Errorf("failed to add stop cron job: %w", err)
		}
		entryIDs = append(entryIDs, stopID)
	}

	if len(entryIDs) == 0 {
		slog.Info("schedule has no cron expressions, not registering", "schedule_id", schedule.ID, "container", schedule.ContainerName)
		return nil
	}

	s.jobs[schedule.ID] = entryIDs
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

func (s *Scheduler) AddStack(stack *models.Stack) error {
	if !stack.Enabled {
		slog.Info("stack disabled, not registering cron jobs", "stack_id", stack.ID, "stack", stack.Name)
		return nil
	}
	if stack.StartCron == "" && stack.StopCron == "" {
		slog.Info("stack has no cron expressions, not registering", "stack_id", stack.ID, "stack", stack.Name)
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var entryIDs []cron.EntryID

	if stack.StartCron != "" {
		if err := ValidateCronExpression(stack.StartCron); err != nil {
			return fmt.Errorf("invalid start cron: %w", err)
		}
		id, err := s.cron.AddFunc(stack.StartCron, func() {
			s.fireStack(stack, true)
		})
		if err != nil {
			return fmt.Errorf("failed to add start cron for stack: %w", err)
		}
		entryIDs = append(entryIDs, id)
	}

	if stack.StopCron != "" {
		if err := ValidateCronExpression(stack.StopCron); err != nil {
			for _, id := range entryIDs {
				s.cron.Remove(id)
			}
			return fmt.Errorf("invalid stop cron: %w", err)
		}
		id, err := s.cron.AddFunc(stack.StopCron, func() {
			s.fireStack(stack, false)
		})
		if err != nil {
			for _, id := range entryIDs {
				s.cron.Remove(id)
			}
			return fmt.Errorf("failed to add stop cron for stack: %w", err)
		}
		entryIDs = append(entryIDs, id)
	}

	s.jobs[stack.ID] = entryIDs
	slog.Info("registered stack cron jobs", "stack_id", stack.ID, "stack", stack.Name,
		"start_cron", stack.StartCron, "stop_cron", stack.StopCron)
	return nil
}

func (s *Scheduler) RemoveStack(stackID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids, ok := s.jobs[stackID]
	if !ok {
		return
	}
	for _, id := range ids {
		s.cron.Remove(id)
	}
	delete(s.jobs, stackID)
	slog.Info("removed stack cron jobs", "stack_id", stackID)
}

func (s *Scheduler) UpdateStack(stack *models.Stack) error {
	s.RemoveStack(stack.ID)
	return s.AddStack(stack)
}

func (s *Scheduler) fireStack(stack *models.Stack, start bool) {
	containers, err := s.docker.ListContainers(context.Background())
	if err != nil {
		slog.Error("stack cron: failed to list containers", "stack", stack.Name, "error", err)
		return
	}

	action := "start"
	if !start {
		action = "stop"
	}

	for _, c := range containers {
		if c.StackName != stack.Name {
			continue
		}
		lock := s.getOrCreateLock(c.Name)
		lock.Lock()
		var fireErr error
		if start {
			slog.Info("stack cron fired: starting container", "stack", stack.Name, "container", c.Name)
			fireErr = s.docker.StartContainer(context.Background(), c.Name)
		} else {
			slog.Info("stack cron fired: stopping container", "stack", stack.Name, "container", c.Name)
			fireErr = s.docker.StopContainer(context.Background(), c.Name)
		}
		lock.Unlock()
		if fireErr != nil {
			slog.Error("stack cron: failed to "+action+" container", "stack", stack.Name, "container", c.Name, "error", fireErr)
		} else {
			slog.Info("stack cron: "+action+"ed container", "stack", stack.Name, "container", c.Name)
		}
	}
}
