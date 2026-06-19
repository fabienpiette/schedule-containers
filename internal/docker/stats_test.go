package docker

import "testing"

func TestParseHealthStatus(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"healthy", "healthy"},
		{"unhealthy", "unhealthy"},
		{"starting", "starting"},
		{"", ""},
		{"none", ""},
		{"unknown", ""},
		{"Running", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseHealthStatus(tt.input)
			if got != tt.want {
				t.Errorf("parseHealthStatus(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
