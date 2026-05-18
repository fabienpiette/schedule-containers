package yamlconfig

import (
	"testing"

	"github.com/gndm/schedule-containers/internal/models"
)

func TestFromSchedules(t *testing.T) {
	schedules := []models.Schedule{
		{
			ContainerName: "my-app",
			DisplayName:  "My App",
			StartCron:    "0 8 * * 1-5",
			StopCron:     "0 18 * * 1-5",
			Enabled:      true,
		},
		{
			ContainerName: "redis",
			DisplayName:  "Redis",
			StartCron:    "0 9 * * *",
			StopCron:     "0 21 * * *",
			Enabled:      false,
		},
	}

	data := FromSchedules(schedules)
	if len(data) == 0 {
		t.Fatal("expected non-empty YAML output")
	}

	parsed, err := ToSchedules(data)
	if err != nil {
		t.Fatalf("failed to parse exported YAML: %v", err)
	}
	if len(parsed) != 2 {
		t.Fatalf("expected 2 schedules, got %d", len(parsed))
	}
	if parsed[0].ContainerName != "my-app" {
		t.Errorf("expected my-app, got %s", parsed[0].ContainerName)
	}
	if parsed[0].StartCron != "0 8 * * 1-5" {
		t.Errorf("expected start cron, got %s", parsed[0].StartCron)
	}
	if parsed[1].Enabled != false {
		t.Errorf("expected enabled=false, got %v", parsed[1].Enabled)
	}
}

func TestToSchedulesValidation(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
	}{
		{
			name: "valid",
			yaml: `schedules:
  - container: my-app
    start_cron: "0 8 * * *"
    stop_cron: "0 18 * * *"
    enabled: true`,
			wantErr: false,
		},
		{
			name: "missing container",
			yaml: `schedules:
  - start_cron: "0 8 * * *"
    stop_cron: "0 18 * * *"
    enabled: true`,
			wantErr: true,
		},
		{
			name: "missing start_cron",
			yaml: `schedules:
  - container: my-app
    stop_cron: "0 18 * * *"
    enabled: true`,
			wantErr: true,
		},
		{
			name: "invalid cron",
			yaml: `schedules:
  - container: my-app
    start_cron: "invalid"
    stop_cron: "0 18 * * *"
    enabled: true`,
			wantErr: true,
		},
		{
			name: "display_name defaults to container",
			yaml: `schedules:
  - container: my-app
    start_cron: "0 8 * * *"
    stop_cron: "0 18 * * *"
    enabled: true`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ToSchedules([]byte(tt.yaml))
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	original := []models.Schedule{
		{
			ContainerName: "webapp",
			DisplayName:  "Web App",
			StackName:     "webstack",
			StartCron:    "0 7 * * 1-5",
			StopCron:     "0 19 * * 1-5",
			Enabled:      true,
		},
	}

	data := FromSchedules(original)
	parsed, err := ToSchedules(data)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if parsed[0].ContainerName != original[0].ContainerName {
		t.Errorf("container name mismatch: got %s, want %s", parsed[0].ContainerName, original[0].ContainerName)
	}
	if parsed[0].StartCron != original[0].StartCron {
		t.Errorf("start cron mismatch: got %s, want %s", parsed[0].StartCron, original[0].StartCron)
	}
	if parsed[0].StopCron != original[0].StopCron {
		t.Errorf("stop cron mismatch: got %s, want %s", parsed[0].StopCron, original[0].StopCron)
	}
	if parsed[0].DisplayName != original[0].DisplayName {
		t.Errorf("display name mismatch: got %s, want %s", parsed[0].DisplayName, original[0].DisplayName)
	}
}