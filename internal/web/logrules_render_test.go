package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fabienpiette/schedule-containers/internal/models"
)

// TestHandleLogRules_RendersRows exercises handleLogRules end-to-end: an
// enabled rule and a rule that was auto-disabled with a reason must both
// appear in the rendered page, including the disabled-reason text.
func TestHandleLogRules_RendersRows(t *testing.T) {
	srv, _ := setupTestServer(t)

	reason := "circuit breaker"
	if _, err := srv.store.CreateLogRule(context.Background(), &models.LogRule{
		ContainerName: "web", Pattern: "boom", MatchType: models.MatchSubstring, Enabled: true,
	}); err != nil {
		t.Fatalf("create enabled log rule: %v", err)
	}
	if _, err := srv.store.CreateLogRule(context.Background(), &models.LogRule{
		ContainerName: "db", Pattern: "panic", MatchType: models.MatchRegex, Enabled: false, DisabledReason: &reason,
	}); err != nil {
		t.Fatalf("create disabled log rule: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/log-rules", nil)
	w := httptest.NewRecorder()
	srv.handleLogRules(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	out := w.Body.String()
	if !strings.Contains(out, "web") || !strings.Contains(out, "boom") {
		t.Fatalf("expected rule content in page, got: %s", out)
	}
	if !strings.Contains(out, "db") || !strings.Contains(out, "panic") {
		t.Fatalf("expected disabled rule content in page, got: %s", out)
	}
	if !strings.Contains(out, "circuit breaker") {
		t.Fatalf("expected disabled reason to render, got: %s", out)
	}
}
