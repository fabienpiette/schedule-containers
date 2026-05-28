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
	Tags      []TagEntry      `yaml:"tags,omitempty"`
	Stacks    []StackEntry    `yaml:"stacks,omitempty"`
}

type ScheduleEntry struct {
	ContainerName string `yaml:"container"`
	DisplayName   string `yaml:"display_name,omitempty"`
	StackName     string `yaml:"stack_name,omitempty"`
	StartCron     string `yaml:"start_cron"`
	StopCron      string `yaml:"stop_cron"`
	Enabled       bool   `yaml:"enabled"`
}

type TagEntry struct {
	Name        string   `yaml:"name"`
	StartCron   string   `yaml:"start_cron"`
	StopCron    string   `yaml:"stop_cron"`
	Enabled     bool     `yaml:"enabled"`
	Containers  []string `yaml:"containers"`
}

type StackEntry struct {
	Name             string `yaml:"name"`
	DisplayName      string `yaml:"display_name,omitempty"`
	StartCron        string `yaml:"start_cron,omitempty"`
	StopCron         string `yaml:"stop_cron,omitempty"`
	Enabled          bool   `yaml:"enabled"`
	OnDemandEnabled  bool   `yaml:"on_demand_enabled,omitempty"`
	OnDemandURL      string `yaml:"on_demand_url,omitempty"`
	PrimaryContainer string `yaml:"primary_container,omitempty"`
	IdleTimeoutSec   int    `yaml:"idle_timeout_sec,omitempty"`
	StartupDelaySec  int    `yaml:"startup_delay_sec,omitempty"`
}

func FromSchedulesAndTags(schedules []models.Schedule, tags []models.Tag) []byte {
	return FromSchedulesTagsAndStacks(schedules, tags, nil)
}

func FromSchedulesTagsAndStacks(schedules []models.Schedule, tags []models.Tag, stacks []models.Stack) []byte {
	var directEntries []ScheduleEntry
	tagSchedules := make(map[string][]models.Schedule)

	for _, s := range schedules {
		if s.TagID != nil {
			tagSchedules[*s.TagID] = append(tagSchedules[*s.TagID], s)
		} else {
			directEntries = append(directEntries, ScheduleEntry{
				ContainerName: s.ContainerName,
				DisplayName:   s.DisplayName,
				StackName:     s.StackName,
				StartCron:     s.StartCron,
				StopCron:      s.StopCron,
				Enabled:       s.Enabled,
			})
		}
	}

	var tagEntries []TagEntry
	for _, tag := range tags {
		entry := TagEntry{
			Name:      tag.Name,
			StartCron: tag.StartCron,
			StopCron:  tag.StopCron,
			Enabled:   tag.Enabled,
		}
		for _, s := range tagSchedules[tag.ID] {
			entry.Containers = append(entry.Containers, s.ContainerName)
		}
		tagEntries = append(tagEntries, entry)
	}

	stackEntries := make([]StackEntry, len(stacks))
	for i, st := range stacks {
		stackEntries[i] = StackEntry{
			Name:             st.Name,
			DisplayName:      st.DisplayName,
			StartCron:        st.StartCron,
			StopCron:         st.StopCron,
			Enabled:          st.Enabled,
			OnDemandEnabled:  st.OnDemandEnabled,
			OnDemandURL:      st.OnDemandURL,
			PrimaryContainer: st.PrimaryContainer,
			IdleTimeoutSec:   st.IdleTimeoutSec,
			StartupDelaySec:  st.StartupDelaySec,
		}
	}

	data, _ := yaml.Marshal(&Config{Tags: tagEntries, Schedules: directEntries, Stacks: stackEntries})
	return data
}

func ToSchedules(data []byte) ([]models.Schedule, error) {
	schedules, _, err := ToSchedulesAndTags(data)
	return schedules, err
}

func ToSchedulesAndTags(data []byte) ([]models.Schedule, []models.Tag, error) {
	schedules, tags, _, err := ToSchedulesTagsAndStacks(data)
	return schedules, tags, err
}

func ToSchedulesTagsAndStacks(data []byte) ([]models.Schedule, []models.Tag, []models.Stack, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	var tags []models.Tag
	for i, entry := range cfg.Tags {
		if entry.Name == "" {
			return nil, nil, nil, fmt.Errorf("tag %d: name is required", i+1)
		}
		if entry.StartCron == "" {
			return nil, nil, nil, fmt.Errorf("tag %d (%s): start_cron is required", i+1, entry.Name)
		}
		if entry.StopCron == "" {
			return nil, nil, nil, fmt.Errorf("tag %d (%s): stop_cron is required", i+1, entry.Name)
		}
		if err := scheduler.ValidateCronExpression(entry.StartCron); err != nil {
			return nil, nil, nil, fmt.Errorf("tag %d (%s): invalid start_cron: %w", i+1, entry.Name, err)
		}
		if err := scheduler.ValidateCronExpression(entry.StopCron); err != nil {
			return nil, nil, nil, fmt.Errorf("tag %d (%s): invalid stop_cron: %w", i+1, entry.Name, err)
		}
		tags = append(tags, models.Tag{
			Name:      entry.Name,
			StartCron: entry.StartCron,
			StopCron:  entry.StopCron,
			Enabled:   entry.Enabled,
		})
	}

	schedules := make([]models.Schedule, len(cfg.Schedules))
	for i, entry := range cfg.Schedules {
		if entry.ContainerName == "" {
			return nil, nil, nil, fmt.Errorf("schedule %d: container is required", i+1)
		}
		if entry.StartCron == "" {
			return nil, nil, nil, fmt.Errorf("schedule %d (%s): start_cron is required", i+1, entry.ContainerName)
		}
		if entry.StopCron == "" {
			return nil, nil, nil, fmt.Errorf("schedule %d (%s): stop_cron is required", i+1, entry.ContainerName)
		}
		if err := scheduler.ValidateCronExpression(entry.StartCron); err != nil {
			return nil, nil, nil, fmt.Errorf("schedule %d (%s): invalid start_cron: %w", i+1, entry.ContainerName, err)
		}
		if err := scheduler.ValidateCronExpression(entry.StopCron); err != nil {
			return nil, nil, nil, fmt.Errorf("schedule %d (%s): invalid stop_cron: %w", i+1, entry.ContainerName, err)
		}
		displayName := entry.DisplayName
		if displayName == "" {
			displayName = entry.ContainerName
		}
		schedules[i] = models.Schedule{
			ContainerName: entry.ContainerName,
			DisplayName:   displayName,
			StackName:     entry.StackName,
			StartCron:     entry.StartCron,
			StopCron:      entry.StopCron,
			Enabled:       entry.Enabled,
		}
	}

	var stacks []models.Stack
	for i, entry := range cfg.Stacks {
		if entry.Name == "" {
			return nil, nil, nil, fmt.Errorf("stack %d: name is required", i+1)
		}
		if entry.StartCron != "" {
			if err := scheduler.ValidateCronExpression(entry.StartCron); err != nil {
				return nil, nil, nil, fmt.Errorf("stack %d (%s): invalid start_cron: %w", i+1, entry.Name, err)
			}
		}
		if entry.StopCron != "" {
			if err := scheduler.ValidateCronExpression(entry.StopCron); err != nil {
				return nil, nil, nil, fmt.Errorf("stack %d (%s): invalid stop_cron: %w", i+1, entry.Name, err)
			}
		}
		stacks = append(stacks, models.Stack{
			Name:             entry.Name,
			DisplayName:      entry.DisplayName,
			StartCron:        entry.StartCron,
			StopCron:         entry.StopCron,
			Enabled:          entry.Enabled,
			OnDemandEnabled:  entry.OnDemandEnabled,
			OnDemandURL:      entry.OnDemandURL,
			PrimaryContainer: entry.PrimaryContainer,
			IdleTimeoutSec:   entry.IdleTimeoutSec,
			StartupDelaySec:  entry.StartupDelaySec,
		})
	}

	return schedules, tags, stacks, nil
}

func FromSchedules(schedules []models.Schedule) []byte {
	return FromSchedulesAndTags(schedules, nil)
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