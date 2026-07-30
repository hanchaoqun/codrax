package types

// tool_result_memo_test.go — EVALFIX-2B 类2: the run-scoped pure-tool memo
// storage follows the traceQueryPublishedBlobRefs lifecycle exactly —
// fork clone, union merge (first-writer-wins), per-task reset, and the
// Success-only store chokepoint.

import "testing"

func memoResult(summary string) ToolResult {
	return ToolResult{ToolName: "trace_query", Success: true, Summary: summary}
}

func TestToolResultMemo_StoreAndLookup(t *testing.T) {
	m := NewMutableState("memo lifecycle")
	if _, ok := m.ToolResultMemo("trace_query", "k1"); ok {
		t.Fatal("empty memo must miss")
	}
	m.StoreToolResultMemo("trace_query", "k1", memoResult("first"))
	got, ok := m.ToolResultMemo("trace_query", "k1")
	if !ok || got.Summary != "first" {
		t.Fatalf("stored entry must be retrievable: ok=%v got=%+v", ok, got)
	}
	// First-writer-wins: a second store under the same key is refused.
	m.StoreToolResultMemo("trace_query", "k1", memoResult("second"))
	if got, _ := m.ToolResultMemo("trace_query", "k1"); got.Summary != "first" {
		t.Fatalf("store must be first-writer-wins, got %q", got.Summary)
	}
	// Tool name is part of the identity: same key under another tool misses.
	if _, ok := m.ToolResultMemo("other_tool", "k1"); ok {
		t.Fatal("memo key must be scoped per tool")
	}
	// Success=false is refused at the chokepoint (retry must re-execute).
	m.StoreToolResultMemo("trace_query", "k2", ToolResult{ToolName: "trace_query", Success: false, Summary: "failed"})
	if _, ok := m.ToolResultMemo("trace_query", "k2"); ok {
		t.Fatal("failed results must never enter the memo")
	}
	// Empty tool / key never store or hit.
	m.StoreToolResultMemo("", "k3", memoResult("x"))
	m.StoreToolResultMemo("trace_query", "", memoResult("x"))
	if _, ok := m.ToolResultMemo("", "k3"); ok {
		t.Fatal("empty tool must miss")
	}
	if _, ok := m.ToolResultMemo("trace_query", ""); ok {
		t.Fatal("empty key must miss")
	}
}

func TestToolResultMemo_ForkMergeLifecycle(t *testing.T) {
	parent := NewMutableState("memo fork/merge")
	parent.StoreToolResultMemo("trace_query", "seed", memoResult("parent-seed"))

	fork := parent.ForkForExploreDispatch()
	// Fork inherits the parent's entries (clone, not alias).
	if got, ok := fork.ToolResultMemo("trace_query", "seed"); !ok || got.Summary != "parent-seed" {
		t.Fatalf("fork must see the parent's memo: ok=%v got=%+v", ok, got)
	}
	// Fork-local writes stay isolated until merge.
	fork.StoreToolResultMemo("trace_query", "fork-key", memoResult("fork-value"))
	if _, ok := parent.ToolResultMemo("trace_query", "fork-key"); ok {
		t.Fatal("fork-local store must not leak into the parent before merge")
	}
	// Conflict: parent writes the same key while the fork runs —
	// first-writer-wins at merge means the parent's entry survives.
	parent.StoreToolResultMemo("trace_query", "contested", memoResult("parent-won"))
	fork.StoreToolResultMemo("trace_query", "contested", memoResult("fork-lost"))

	parent.MergeExploreFork(fork)
	if got, ok := parent.ToolResultMemo("trace_query", "fork-key"); !ok || got.Summary != "fork-value" {
		t.Fatalf("merge must union the fork's new entries: ok=%v got=%+v", ok, got)
	}
	if got, _ := parent.ToolResultMemo("trace_query", "contested"); got.Summary != "parent-won" {
		t.Fatalf("merge must be first-writer-wins, got %q", got.Summary)
	}
}

func TestToolResultMemo_ResetTurnAArtifactsClears(t *testing.T) {
	m := NewMutableState("memo reset")
	m.StoreToolResultMemo("trace_query", "k1", memoResult("stale"))
	m.ResetTurnAArtifacts()
	if _, ok := m.ToolResultMemo("trace_query", "k1"); ok {
		t.Fatal("ResetTurnAArtifacts must clear the memo (same per-task boundary as traceQueryPublishedBlobRefs)")
	}
}
