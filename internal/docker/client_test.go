package docker

import (
	"testing"

	"github.com/docker/docker/api/types/container"
)

func TestTransformContainers(t *testing.T) {
	input := []container.Summary{
		{
			ID:     "abc123def456789",
			Names:  []string{"/my-app"},
			Image:  "nginx:latest",
			State:  "running",
			Status: "Up 2 hours",
			Labels: map[string]string{"com.docker.compose.project": "webstack"},
		},
		{
			ID:     "xyz789ghi012345",
			Names:  []string{"/another-app"},
			Image:  "redis:7",
			State:  "exited",
			Status: "Exited 3 days ago",
			Labels: map[string]string{},
		},
	}

	result := transformContainers(input)

	if len(result) != 2 {
		t.Fatalf("expected 2 containers, got %d", len(result))
	}
	if result[0].Name != "my-app" {
		t.Errorf("expected my-app, got %s", result[0].Name)
	}
	if result[0].StackName != "webstack" {
		t.Errorf("expected webstack, got %s", result[0].StackName)
	}
	if result[1].Name != "another-app" {
		t.Errorf("expected another-app, got %s", result[1].Name)
	}
	if result[1].StackName != "" {
		t.Errorf("expected empty stack, got %s", result[1].StackName)
	}
}

func TestTransformContainersStripsSlash(t *testing.T) {
	input := []container.Summary{
		{ID: "abc123def456789", Names: []string{"/test"}, Image: "img", State: "running", Labels: map[string]string{}},
	}
	result := transformContainers(input)
	if result[0].Name != "test" {
		t.Errorf("expected slash stripped, got %s", result[0].Name)
	}
}