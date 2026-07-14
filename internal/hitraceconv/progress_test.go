package hitraceconv

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRunCommandWithProgressPreservesStartFailureTerminalMessage(t *testing.T) {
	var events []ProgressEvent
	opts := Options{Progress: func(event ProgressEvent) { events = append(events, event) }}
	command := exec.Command(filepath.Join(t.TempDir(), "missing-external-tool"))
	if _, err := runCommandWithProgress(opts, command, "simpleperf_adapter", "running official simpleperf adapter"); err == nil {
		t.Fatal("missing external command unexpectedly started")
	}
	if len(events) != 2 || events[0].Status != ProgressStatusStarted ||
		events[1].Status != ProgressStatusFailed || events[1].Message != "external command failed to start" {
		t.Fatalf("external command start-failure progress drifted: %+v", events)
	}
}

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
