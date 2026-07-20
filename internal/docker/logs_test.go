package docker

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func collect(ctx context.Context, r *strings.Reader) []string {
	ch := make(chan string, 16)
	go func() {
		defer close(ch)
		scanLogLines(ctx, r, ch)
	}()
	var out []string
	for line := range ch {
		out = append(out, line)
	}
	return out
}

func TestScanLogLines_SplitsLines(t *testing.T) {
	got := collect(context.Background(), strings.NewReader("first\nsecond\nthird\n"))
	want := []string{"first", "second", "third"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestScanLogLines_TruncatesLongLine(t *testing.T) {
	long := strings.Repeat("x", maxLogLine+500)
	got := collect(context.Background(), strings.NewReader(long+"\nok\n"))
	if len(got) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(got))
	}
	if len(got[0]) != maxLogLine {
		t.Fatalf("expected first line truncated to %d, got %d", maxLogLine, len(got[0]))
	}
	if got[1] != "ok" {
		t.Fatalf("expected second line 'ok', got %q", got[1])
	}
}

// ensures the helper handles a final line without a trailing newline
func TestScanLogLines_NoTrailingNewline(t *testing.T) {
	got := collect(context.Background(), strings.NewReader("only"))
	if len(got) != 1 || got[0] != "only" {
		t.Fatalf("got %v", got)
	}
	_ = bytes.MinRead
}
