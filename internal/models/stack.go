package models

import "time"

type Stack struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	DisplayName      string    `json:"display_name"`
	StartCron        string    `json:"start_cron"`
	StopCron         string    `json:"stop_cron"`
	Enabled          bool      `json:"enabled"`
	OnDemandEnabled  bool      `json:"on_demand_enabled"`
	OnDemandURL      string    `json:"on_demand_url"`
	PrimaryContainer string    `json:"primary_container"`
	IdleTimeoutSec   int       `json:"idle_timeout_sec"`
	StartupDelaySec  int       `json:"startup_delay_sec"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (s *Stack) IdleTimeout() time.Duration {
	if s.IdleTimeoutSec <= 0 {
		return 0
	}
	return time.Duration(s.IdleTimeoutSec) * time.Second
}
