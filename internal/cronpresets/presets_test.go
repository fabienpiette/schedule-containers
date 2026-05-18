package cronpresets

import (
	"testing"
)

func TestBuiltinsNotEmpty(t *testing.T) {
	presets := Builtins()
	if len(presets) == 0 {
		t.Error("expected non-empty presets list")
	}
}

func TestBuiltinsAllHaveIDs(t *testing.T) {
	presets := Builtins()
	for _, p := range presets {
		if p.ID == "" {
			t.Error("preset missing ID")
		}
	}
}

func TestBuiltinsAllBuiltin(t *testing.T) {
	presets := Builtins()
	for _, p := range presets {
		if !p.Builtin {
			t.Errorf("preset %s should be builtin", p.ID)
		}
	}
}

func TestBuiltinsAllHaveFields(t *testing.T) {
	presets := Builtins()
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

func TestBuiltinsCategories(t *testing.T) {
	presets := Builtins()
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

func TestBuiltinsUniqueIDs(t *testing.T) {
	presets := Builtins()
	ids := make(map[string]bool)
	for _, p := range presets {
		if ids[p.ID] {
			t.Errorf("duplicate preset ID: %s", p.ID)
		}
		ids[p.ID] = true
	}
}