package models

import (
	"fmt"
	"regexp"
	"time"
)

const (
	MatchSubstring = "substring"
	MatchRegex     = "regex"
)

// LogRule restarts ContainerName when a log line matches Pattern.
type LogRule struct {
	ID             string     `json:"id"`
	ContainerName  string     `json:"container_name"`
	Pattern        string     `json:"pattern"`
	MatchType      string     `json:"match_type"` // MatchSubstring | MatchRegex
	Enabled        bool       `json:"enabled"`
	CooldownSec    int        `json:"cooldown_sec"`
	DisabledReason *string    `json:"disabled_reason,omitempty"`
	LastMatchedAt  *time.Time `json:"last_matched_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// Validate checks the rule is well-formed. It compiles the pattern for regex rules.
func (r *LogRule) Validate() error {
	if r.ContainerName == "" {
		return fmt.Errorf("container_name is required")
	}
	if r.Pattern == "" {
		return fmt.Errorf("pattern is required")
	}
	switch r.MatchType {
	case MatchSubstring:
	case MatchRegex:
		if _, err := regexp.Compile(r.Pattern); err != nil {
			return fmt.Errorf("invalid regex: %w", err)
		}
	default:
		return fmt.Errorf("match_type must be %q or %q", MatchSubstring, MatchRegex)
	}
	if r.CooldownSec < 0 {
		return fmt.Errorf("cooldown_sec must be >= 0")
	}
	return nil
}

// Cooldown returns the per-rule cooldown, defaulting to 60s when unset.
func (r *LogRule) Cooldown() time.Duration {
	if r.CooldownSec <= 0 {
		return 60 * time.Second
	}
	return time.Duration(r.CooldownSec) * time.Second
}
