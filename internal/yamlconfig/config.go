package yamlconfig

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/gndm/schedule-containers/internal/models"
	"github.com/gndm/schedule-containers/internal/scheduler"
)

type Config struct {
	Schedules []ScheduleEntry `yaml:"schedules"`
}

type ScheduleEntry struct {
	ContainerName string `yaml:"container"`
	DisplayName   string `yaml:"display_name,omitempty"`
	StackName     string `yaml:"stack_name,omitempty"`
	StartCron     string `yaml:"start_cron"`
	StopCron      string `yaml:"stop_cron"`
	Enabled       bool   `yaml:"enabled"`
}

func FromSchedules(schedules []models.Schedule) []byte {
	entries := make([]ScheduleEntry, len(schedules))
	for i, s := range schedules {
		entries[i] = ScheduleEntry{
			ContainerName: s.ContainerName,
			DisplayName:   s.DisplayName,
			StackName:     s.StackName,
			StartCron:     s.StartCron,
			StopCron:      s.StopCron,
			Enabled:       s.Enabled,
		}
	}
	data, _ := yaml.Marshal(&Config{Schedules: entries})
	return data
}

func ToSchedules(data []byte) ([]models.Schedule, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	schedules := make([]models.Schedule, len(cfg.Schedules))
	for i, entry := range cfg.Schedules {
		if entry.ContainerName == "" {
			return nil, fmt.Errorf("schedule %d: container is required", i+1)
		}
		if entry.StartCron == "" {
			return nil, fmt.Errorf("schedule %d (%s): start_cron is required", i+1, entry.ContainerName)
		}
		if entry.StopCron == "" {
			return nil, fmt.Errorf("schedule %d (%s): stop_cron is required", i+1, entry.ContainerName)
		}
		if err := scheduler.ValidateCronExpression(entry.StartCron); err != nil {
			return nil, fmt.Errorf("schedule %d (%s): invalid start_cron: %w", i+1, entry.ContainerName, err)
		}
		if err := scheduler.ValidateCronExpression(entry.StopCron); err != nil {
			return nil, fmt.Errorf("schedule %d (%s): invalid stop_cron: %w", i+1, entry.ContainerName, err)
		}
		displayName := entry.DisplayName
		if displayName == "" {
			displayName = entry.ContainerName
		}
		schedules[i] = models.Schedule{
			ContainerName: entry.ContainerName,
			DisplayName:  displayName,
			StackName:     entry.StackName,
			StartCron:     entry.StartCron,
			StopCron:      entry.StopCron,
			Enabled:       entry.Enabled,
		}
	}
	return schedules, nil
}

func ExportToFile(schedules []models.Schedule, path string) error {
	data := FromSchedules(schedules)
	return os.WriteFile(path, data, 0644)
}

func ImportFromFile(path string) ([]models.Schedule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	return ToSchedules(data)
}