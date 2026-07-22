package models

import (
	"testing"
	"time"
)

func TestLogRuleValidate(t *testing.T) {
	tests := []struct {
		name    string
		rule    LogRule
		wantErr bool
	}{
		{
			name: "valid substring",
			rule: LogRule{
				ContainerName: "web",
				Pattern:       "OutOfMemory",
				MatchType:     MatchSubstring,
			},
			wantErr: false,
		},
		{
			name: "valid regex",
			rule: LogRule{
				ContainerName: "web",
				Pattern:       `error: \d+`,
				MatchType:     MatchRegex,
			},
			wantErr: false,
		},
		{
			name: "empty container",
			rule: LogRule{
				ContainerName: "",
				Pattern:       "OutOfMemory",
				MatchType:     MatchSubstring,
			},
			wantErr: true,
		},
		{
			name: "empty pattern",
			rule: LogRule{
				ContainerName: "web",
				Pattern:       "",
				MatchType:     MatchSubstring,
			},
			wantErr: true,
		},
		{
			name: "unknown match type",
			rule: LogRule{
				ContainerName: "web",
				Pattern:       "OutOfMemory",
				MatchType:     "fuzzy",
			},
			wantErr: true,
		},
		{
			name: "invalid regex",
			rule: LogRule{
				ContainerName: "web",
				Pattern:       "(unclosed",
				MatchType:     MatchRegex,
			},
			wantErr: true,
		},
		{
			name: "negative cooldown",
			rule: LogRule{
				ContainerName: "web",
				Pattern:       "OutOfMemory",
				MatchType:     MatchSubstring,
				CooldownSec:   -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.rule.Validate()
			if tt.wantErr && err == nil {
				t.Fatalf("Validate() = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestLogRuleCooldown(t *testing.T) {
	r := LogRule{CooldownSec: 0}
	if got := r.Cooldown(); got != 60*time.Second {
		t.Fatalf("Cooldown() with CooldownSec=0 = %v, want 60s", got)
	}

	r = LogRule{CooldownSec: -5}
	if got := r.Cooldown(); got != 60*time.Second {
		t.Fatalf("Cooldown() with CooldownSec=-5 = %v, want 60s", got)
	}

	r = LogRule{CooldownSec: 45}
	if got := r.Cooldown(); got != 45*time.Second {
		t.Fatalf("Cooldown() with CooldownSec=45 = %v, want 45s", got)
	}
}
