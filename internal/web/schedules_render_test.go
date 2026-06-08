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
