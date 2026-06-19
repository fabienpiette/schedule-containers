package models

import "time"

type Tag struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	StartCron string    `json:"start_cron"`
	StopCron  string    `json:"stop_cron"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
