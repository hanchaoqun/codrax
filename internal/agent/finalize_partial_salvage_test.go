package agent

import (
	"strings"
	"testing"
)

// TestFinalizePreviewHook_PartialReturnsAccumulated covers the new
// Partial() getter the orchestrator's force-finalize path consults
// when a Chat error fires before the emit_answer_document tool call
// closes. Without this getter, in-flight summary text decoded from
// argument chunks would be discarded on EOF / stream stall.
func TestFinalizePreviewHook_PartialReturnsAccumulated(t *testing.T) {
	h := newFinalizePreviewHook(nil) // nil emit OK — Partial doesn't emit
	if got := h.Partial(); got != "" {
		t.Errorf("Partial on fresh hook must be empty; got %q", got)
	}
	// Feed a partial JSON tool-call args chunk that the
	// SummaryExtractor decodes into actual summary text. The chunk
	// shape mirrors what the openai SSE stream produces — a single
	// JSON-shaped fragment of `{"summary":"<text...`.
	h.onToolCallDelta(0, "emit_answer_document", `{"summary":"hello world this is a partial answer`)
	got := h.Partial()
	if !strings.Contains(got, "hello world") {
		t.Errorf("Partial must surface decoded summary; got %q", got)
	}
}

// TestFinalizePreviewHook_PartialNilSafe locks the nil-receiver
// fast path. cmd/root.go and orchestrator paths construct hooks
// only for finalize stages; non-finalize agent dispatches pass nil
// hooks, and Partial must short-circuit gracefully.
func TestFinalizePreviewHook_PartialNilSafe(t *testing.T) {
	var h *finalizePreviewHook
	if got := h.Partial(); got != "" {
		t.Errorf("nil hook Partial must be empty; got %q", got)
	}
}

// TestFinalizePreviewHook_PartialIgnoresOtherTools verifies the
// extractor's tool-name filter: chunks for a non-emit_answer_document
// tool must not pollute the partial summary buffer.
func TestFinalizePreviewHook_PartialIgnoresOtherTools(t *testing.T) {
	h := newFinalizePreviewHook(nil)
	h.onToolCallDelta(0, "grep", `{"pattern":"foo"}`)
	if got := h.Partial(); got != "" {
		t.Errorf("non-finalizer tool must not populate partial; got %q", got)
	}
}
