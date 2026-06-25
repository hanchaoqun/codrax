package hitraceconv

import "testing"

func TestTerminalProgressMessageDistinguishesRunningAndComplete(t *testing.T) {
	if got := terminalProgressMessage("running trace_streamer SQLite DB export", ProgressStatusComplete); got != "completed trace_streamer SQLite DB export" {
		t.Fatalf("complete message = %q", got)
	}
	if got := terminalProgressMessage("running trace_streamer SQLite DB export", ProgressStatusFailed); got != "trace_streamer SQLite DB export failed" {
		t.Fatalf("failed message = %q", got)
	}
	if got := terminalProgressMessage("normalizing trace_streamer SQLite DB to systrace", ProgressStatusComplete); got != "normalizing trace_streamer SQLite DB to systrace" {
		t.Fatalf("unmapped message should pass through, got %q", got)
	}
}
