package agent

import (
	"strings"
	"sync"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestFinalizePreviewHook_EmitsCumulativeChunks confirms the hook
// emits EventLivePreviewChunk events with the cumulative decoded
// summary text (not just the per-chunk delta) — pterm.Area.Update
// expects the full content per call.
func TestFinalizePreviewHook_EmitsCumulativeChunks(t *testing.T) {
	var mu sync.Mutex
	var events []render.Event
	emit := func(ev render.Event) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, ev)
	}
	h := newFinalizePreviewHook(emit)
	// Simulate three chunks of args streaming in.
	h.onToolCallDelta(0, "emit_answer_document", `{"summary":"hel`)
	h.onToolCallDelta(0, "emit_answer_document", `lo wor`)
	h.onToolCallDelta(0, "emit_answer_document", `ld","shape":"explanation"}`)
	// flush() forces emit of the throttled-out tail so the renderer
	// sees the final cumulative state. Mirrors the agent.go call
	// site after Chat returns.
	h.flush()

	mu.Lock()
	defer mu.Unlock()
	if len(events) < 1 {
		t.Fatalf("expected at least 1 preview event; got %d", len(events))
	}
	final := events[len(events)-1]
	if final.Kind != render.EventLivePreviewChunk {
		t.Errorf("Kind = %v, want EventLivePreviewChunk", final.Kind)
	}
	if final.Stage != types.StageFinalize {
		t.Errorf("Stage = %v, want StageFinalize", final.Stage)
	}
	if final.PreviewText != "hello world" {
		t.Errorf("final cumulative = %q, want %q", final.PreviewText, "hello world")
	}
	// Verify monotonic accumulation (each event's PreviewText is a
	// prefix of subsequent ones — pterm.Area.Update with shorter
	// content would erase parts mid-stream which is a regression).
	prev := ""
	for i, ev := range events {
		if ev.PreviewText == "" {
			continue
		}
		if !strings.HasPrefix(ev.PreviewText, prev) {
			t.Errorf("event[%d] cumulative %q does not extend prior %q", i, ev.PreviewText, prev)
		}
		prev = ev.PreviewText
	}
}

// TestFinalizePreviewHook_NonFinalizerToolNoop: the hook must do
// nothing when the tool name is not emit_answer_document. A
// finalizer dispatch SHOULD only call emit_answer_document, but
// defense-in-depth keeps the hook silent for unrelated tools so a
// future schema change cannot accidentally leak garbage to the
// preview.
func TestFinalizePreviewHook_NonFinalizerToolNoop(t *testing.T) {
	emitted := 0
	emit := func(render.Event) { emitted++ }
	h := newFinalizePreviewHook(emit)
	h.onToolCallDelta(0, "some_other_tool", `{"summary":"x"}`)
	if emitted != 0 {
		t.Errorf("expected 0 events for non-finalizer tool; got %d", emitted)
	}
}

// TestFinalizePreviewHook_NilSafe — defensive: a nil hook is
// observed when the agent layer constructed nothing for non-
// finalize stages. Calling onToolCallDelta on nil must not panic.
func TestFinalizePreviewHook_NilSafe(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("nil hook panicked: %v", r)
		}
	}()
	var h *finalizePreviewHook
	h.onToolCallDelta(0, "emit_answer_document", `{"summary":"x"}`)
}

// TestFinalizePreviewHook_NoEmitWhenSummaryAbsent: a tool args
// payload without a summary field should produce no preview events.
func TestFinalizePreviewHook_NoEmitWhenSummaryAbsent(t *testing.T) {
	emitted := 0
	emit := func(render.Event) { emitted++ }
	h := newFinalizePreviewHook(emit)
	h.onToolCallDelta(0, "emit_answer_document", `{"shape":"value","value":{"literal":"42"}}`)
	if emitted != 0 {
		t.Errorf("expected 0 events when summary absent; got %d", emitted)
	}
}
