package models

import "time"

type CronPreset struct {
	ID          string    `json:"id"`
	Label       string    `json:"label"`
	Expression  string    `json:"expression"`
	Category    string    `json:"category"`
	Description string    `json:"description"`
	Builtin     bool      `json:"builtin"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
}