package web

import (
	"bytes"
	"html/template"
	"strings"
	"testing"

	"github.com/gndm/schedule-containers/internal/models"
)

// TestSchedulesContentRenders exercises the full schedules "content" template
// end-to-end: the slim stat strip, the tab panels, and both partial tbodies
// (which call humanCron). It guards against template wiring regressions.
func TestSchedulesContentRenders(t *testing.T) {
	tmpl := template.Must(template.New("").Funcs(templateFuncs).ParseFS(embeddedFS,
		"templates/layout.html",
		"templates/partials.html",
		"templates/schedules.html",
	))

	data := SchedulesData{
		Title:          "Schedules",
		RunningCount:   6,
		StoppedCount:   3,
		SchedulesCount: 27,
		Mode:           "container",
		Schedules: []ScheduleView{{
			ID:          "s1",
			DisplayName: "gitea",
			StartCron:   "0 8 * * 1-5",
			StopCron:    "0 18 * * 1-5",
			TagName:     "business-hours",
			Enabled:     true,
		}},
		Stacks: []StackView{{
			Stack: models.Stack{
				ID:        "k1",
				Name:      "media",
				StartCron: "0 7 * * *",
				StopCron:  "0 0 * * *",
				Enabled:   true,
			},
			ContainerCount: 4,
		}},
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
		t.Fatalf("render schedules content: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		"Weekdays 08:00",   // humanized container start
		"Weekdays 18:00",   // humanized container stop
		"Daily 07:00",      // humanized stack start
		`id="panel-containers"`,
		`id="panel-stacks"`,
		"stat-strip",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered content missing %q", want)
		}
	}
}

// TestPresetTbodyRenders guards the preset auto-refresh fragment: the
// preset-tbody partial must render rows with the delete action wired to a
// tbody refresh (not a row-level swap or page reload).
func TestPresetTbodyRenders(t *testing.T) {
	tmpl := template.Must(template.New("").Funcs(templateFuncs).ParseFS(embeddedFS,
		"templates/layout.html", "templates/partials.html", "templates/presets.html",
	))
	data := PresetsData{Presets: []PresetView{
		{ID: "p1", Label: "Weekday mornings", Expression: "0 8 * * 1-5", Category: "Custom"},
		{ID: "p2", Label: "Hourly", Expression: "0 * * * *", Category: "Common"},
	}}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "preset-tbody", data); err != nil {
		t.Fatalf("render preset-tbody: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "0 8 * * 1-5") {
		t.Errorf("preset-tbody missing custom preset expression")
	}
	if !strings.Contains(out, "htmx.trigger('#preset-tbody','refreshList')") {
		t.Errorf("custom preset delete should trigger a tbody refresh")
	}
	// Embedded (non-Custom) presets must not be deletable.
	if strings.Contains(out, "Delete preset 'Hourly'") {
		t.Errorf("embedded preset should not have a delete button")
	}
}
