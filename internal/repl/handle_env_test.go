package repl

import (
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/memory"
)

// TestLatestShellTurnResponse_PicksMostRecent pins the bug fix:
// when the recent buffer contains multiple shell-bang turns, the
// helper returns the NEWEST one. Pre-fix the inline forward-loop
// in /env explain returned the OLDEST shell turn — surfacing stale
// stderr that no longer matches what the user just ran.
func TestLatestShellTurnResponse_PicksMostRecent(t *testing.T) {
	recent := []memory.Turn{
		{Request: "!ls", Response: "OLD-ls-output", Timestamp: time.Now().Add(-3 * time.Minute)},
		{Request: "what does /chat do?", Response: "It is a chitchat command.", Timestamp: time.Now().Add(-2 * time.Minute)},
		{Request: "!cargo build", Response: "MID-cargo-output", Timestamp: time.Now().Add(-1 * time.Minute)},
		{Request: "!python -V", Response: "NEWEST-python-output", Timestamp: time.Now()},
	}
	got := latestShellTurnResponse(recent)
	if got != "NEWEST-python-output" {
		t.Errorf("expected newest shell turn response; got %q", got)
	}
}

// TestLatestShellTurnResponse_NoShellTurns guards the empty-result
// path: if the recent buffer has no `!`-prefixed turns, the helper
// returns "" so the caller falls through to the "no stderr supplied"
// warning.
func TestLatestShellTurnResponse_NoShellTurns(t *testing.T) {
	recent := []memory.Turn{
		{Request: "/chat hello", Response: "..."},
		{Request: "explain X", Response: "..."},
	}
	if got := latestShellTurnResponse(recent); got != "" {
		t.Errorf("expected empty result when no shell turns; got %q", got)
	}
}

// TestLatestShellTurnResponse_EmptyRecentBuffer covers the edge
// case where Recent() returns nil / empty.
func TestLatestShellTurnResponse_EmptyRecentBuffer(t *testing.T) {
	if got := latestShellTurnResponse(nil); got != "" {
		t.Errorf("nil buffer → expected empty result; got %q", got)
	}
	if got := latestShellTurnResponse([]memory.Turn{}); got != "" {
		t.Errorf("empty buffer → expected empty result; got %q", got)
	}
}

// TestLatestShellTurnResponse_OneShellTurn covers the simplest
// non-empty case.
func TestLatestShellTurnResponse_OneShellTurn(t *testing.T) {
	recent := []memory.Turn{
		{Request: "!echo hi", Response: "hi", Timestamp: time.Now()},
	}
	if got := latestShellTurnResponse(recent); got != "hi" {
		t.Errorf("expected 'hi'; got %q", got)
	}
}
