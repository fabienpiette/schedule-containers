package models

import "time"

type Schedule struct {
	ID              string    `json:"id"`
	ContainerName   string    `json:"container_name"`
	DisplayName     string    `json:"display_name"`
	StackName       string    `json:"stack_name"`
	StartCron       string    `json:"start_cron"`
	StopCron        string    `json:"stop_cron"`
	Enabled         bool      `json:"enabled"`
	OnDemandEnabled bool      `json:"on_demand_enabled"`
	OnDemandURL     string    `json:"on_demand_url"`
	IdleTimeoutSec  int       `json:"idle_timeout_sec"`
	StartupDelaySec int       `json:"startup_delay_sec"`
	TagID           *string   `json:"tag_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (s *Schedule) IdleTimeout() time.Duration {
	if s.IdleTimeoutSec <= 0 {
		return 0
	}
	return time.Duration(s.IdleTimeoutSec) * time.Second
}
