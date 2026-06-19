package cronpresets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestServiceLoadFromEmbedded(t *testing.T) {
	svc, err := NewService("")
	if err != nil {
		t.Fatalf("failed to create service with embedded presets: %v", err)
	}
	presets := svc.List()
	if len(presets) == 0 {
		t.Error("expected non-empty presets list from embedded defaults")
	}
}

func TestServiceLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "presets.yaml")

	svc, err := NewService(path)
	if err != nil {
		t.Fatalf("failed to create service with file path: %v", err)
	}
	presets := svc.List()
	if len(presets) == 0 {
		t.Error("expected non-empty presets list from file")
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected presets file to be created when path provided and file doesn't exist")
	}
}

func TestServiceLoadFromExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "presets.yaml")

	content := `presets:
  - label: "Test"
    expression: "0 8 * * *"
    category: "Daily"
    description: "Test preset"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	svc, err := NewService(path)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	presets := svc.List()
	if len(presets) != 1 {
		t.Fatalf("expected 1 preset, got %d", len(presets))
	}
	if presets[0].Label != "Test" {
		t.Errorf("expected label 'Test', got %s", presets[0].Label)
	}
}

func TestServiceAllHaveIDs(t *testing.T) {
	svc, err := NewService("")
	if err != nil {
		t.Fatal(err)
	}
	presets := svc.List()
	for _, p := range presets {
		if p.ID == "" {
			t.Error("preset missing ID")
		}
	}
}

func TestServiceAllHaveFields(t *testing.T) {
	svc, err := NewService("")
	if err != nil {
		t.Fatal(err)
	}
	presets := svc.List()
	for _, p := range presets {
		if p.Label == "" {
			t.Errorf("preset %s missing label", p.ID)
		}
		if p.Expression == "" {
			t.Errorf("preset %s missing expression", p.ID)
		}
		if p.Category == "" {
			t.Errorf("preset %s missing category", p.ID)
		}
	}
}

func TestServiceCategories(t *testing.T) {
	svc, err := NewService("")
	if err != nil {
		t.Fatal(err)
	}
	presets := svc.List()
	categories := map[string]bool{}
	for _, p := range presets {
		categories[p.Category] = true
	}
	expectedCategories := []string{"Daily", "Weekdays", "Specific Days", "Weekends", "Frequent", "Monthly"}
	for _, cat := range expectedCategories {
		if !categories[cat] {
			t.Errorf("missing category: %s", cat)
		}
	}
}

func TestServiceUniqueIDs(t *testing.T) {
	svc, err := NewService("")
	if err != nil {
		t.Fatal(err)
	}
	presets := svc.List()
	ids := make(map[string]bool)
	for _, p := range presets {
		if ids[p.ID] {
			t.Errorf("duplicate preset ID: %s", p.ID)
		}
		ids[p.ID] = true
	}
}

func TestServiceCreate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "presets.yaml")

	svc, err := NewService(path)
	if err != nil {
		t.Fatal(err)
	}

	preset, err := svc.Create("My Preset", "0 10 * * 1-5", "Custom", "Weekdays at 10am")
	if err != nil {
		t.Fatalf("failed to create preset: %v", err)
	}
	if preset.ID == "" {
		t.Error("expected non-empty ID")
	}
	if preset.Label != "My Preset" {
		t.Errorf("expected label 'My Preset', got %s", preset.Label)
	}
	if preset.Category != "Custom" {
		t.Errorf("expected category 'Custom', got %s", preset.Category)
	}

	presets := svc.List()
	found := false
	for _, p := range presets {
		if p.ID == preset.ID {
			found = true
		}
	}
	if !found {
		t.Error("created preset not found in list")
	}

	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) == 0 {
		t.Error("expected presets file to be saved after create")
	}
}

func TestServiceCreateDefaultCategory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "presets.yaml")

	svc, err := NewService(path)
	if err != nil {
		t.Fatal(err)
	}

	preset, err := svc.Create("Test", "0 8 * * *", "", "desc")
	if err != nil {
		t.Fatal(err)
	}
	if preset.Category != "Custom" {
		t.Errorf("expected default category 'Custom', got %s", preset.Category)
	}
}

func TestServiceCreateInvalidCron(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "presets.yaml")

	svc, err := NewService(path)
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.Create("Bad", "invalid", "Custom", "")
	if err == nil {
		t.Error("expected error for invalid cron expression")
	}
}

func TestServiceCreateEmptyLabel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "presets.yaml")

	svc, err := NewService(path)
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.Create("", "0 8 * * *", "Custom", "")
	if err == nil {
		t.Error("expected error for empty label")
	}
}

func TestServiceDelete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "presets.yaml")

	svc, err := NewService(path)
	if err != nil {
		t.Fatal(err)
	}

	preset, err := svc.Create("ToDelete", "0 8 * * *", "Custom", "")
	if err != nil {
		t.Fatal(err)
	}

	countBefore := len(svc.List())

	if err := svc.Delete(preset.ID); err != nil {
		t.Fatalf("failed to delete preset: %v", err)
	}

	presets := svc.List()
	if len(presets) != countBefore-1 {
		t.Errorf("expected %d presets after delete, got %d", countBefore-1, len(presets))
	}

	for _, p := range presets {
		if p.ID == preset.ID {
			t.Error("deleted preset still in list")
		}
	}
}

func TestServiceDeleteNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "presets.yaml")

	svc, err := NewService(path)
	if err != nil {
		t.Fatal(err)
	}

	err = svc.Delete("nonexistent")
	if err == nil {
		t.Error("expected error for deleting nonexistent preset")
	}
}

func TestServiceCreateEmbeddedNoSave(t *testing.T) {
	svc, err := NewService("")
	if err != nil {
		t.Fatal(err)
	}

	preset, err := svc.Create("EmbeddedTest", "0 8 * * *", "Custom", "test")
	if err != nil {
		t.Fatalf("failed to create preset in embedded mode: %v", err)
	}
	if preset.ID == "" {
		t.Error("expected non-empty ID even in embedded mode")
	}

	presets := svc.List()
	found := false
	for _, p := range presets {
		if p.ID == preset.ID {
			found = true
		}
	}
	if !found {
		t.Error("created preset should be in list even in embedded mode")
	}
}

func TestServiceDeleteEmbeddedNoError(t *testing.T) {
	svc, err := NewService("")
	if err != nil {
		t.Fatal(err)
	}

	presets := svc.List()
	if len(presets) == 0 {
		t.Fatal("expected presets from embedded defaults")
	}

	err = svc.Delete(presets[0].ID)
	if err != nil {
		t.Fatalf("delete in embedded mode should succeed: %v", err)
	}

	remaining := svc.List()
	if len(remaining) != len(presets)-1 {
		t.Errorf("expected %d presets after delete, got %d", len(presets)-1, len(remaining))
	}
}
