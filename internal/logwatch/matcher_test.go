package logwatch

import (
	"testing"

	"github.com/fabienpiette/schedule-containers/internal/models"
)

func TestNewMatcher(t *testing.T) {
	sub, err := NewMatcher("OutOfMemory", models.MatchSubstring)
	if err != nil {
		t.Fatalf("substring: %v", err)
	}
	if !sub.Matches("java.lang.OutOfMemoryError: heap") {
		t.Error("substring should match within a line")
	}
	if sub.Matches("outofmemory") {
		t.Error("substring is case-sensitive")
	}

	re, err := NewMatcher(`(?i)panic:\s+\w+`, models.MatchRegex)
	if err != nil {
		t.Fatalf("regex: %v", err)
	}
	if !re.Matches("2026 PANIC: boom") {
		t.Error("regex with (?i) should match case-insensitively")
	}

	if _, err := NewMatcher("([", models.MatchRegex); err == nil {
		t.Error("expected error for invalid regex")
	}
	if _, err := NewMatcher("x", "bogus"); err == nil {
		t.Error("expected error for unknown match type")
	}
}
