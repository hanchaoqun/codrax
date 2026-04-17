package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func newEmitCtx() *types.BusContext {
	return &types.BusContext{Mutable: types.NewMutableState("")}
}

func TestEmitEvidence_AcceptsValidBatch(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
        "items": [
          {"kind": "registration", "subject": "Foo", "object": "bar", "source": "internal/agent/foo.go", "line_start": 12, "summary": "Foo registers bar", "anchor_kind": "call", "anchor_symbol": "Register"},
          {"kind": "direct", "subject": "isOK", "source": "internal/agent/foo.go", "line_start": 30, "summary": "isOK returns true", "anchor_kind": "definition", "anchor_symbol": "isOK"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 2 {
		t.Fatalf("want 2 items in buffer, got %d", len(got))
	}
	for _, it := range got {
		if it.Producer != EmitEvidenceProducer {
			t.Errorf("producer = %q, want %q", it.Producer, EmitEvidenceProducer)
		}
		if it.ID == "" {
			t.Errorf("missing stable ID")
		}
		if it.LineEnd == 0 {
			t.Errorf("LineEnd should default to LineStart")
		}
	}
	if got[0].Kind != types.EvidenceRegistration {
		t.Errorf("kind = %q", got[0].Kind)
	}
	if got[0].Predicate != "registration" {
		t.Errorf("predicate default failed: %q", got[0].Predicate)
	}
}

func TestEmitEvidence_RejectsUnknownKind(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[{"kind":"speculation","source":"x.go"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("expected failure, got success: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "unknown kind") {
		t.Errorf("error msg should mention unknown kind, got: %s", res.Summary)
	}
	if len(ctx.Mutable.EmittedEvidence()) != 0 {
		t.Errorf("buffer should not be touched on failure")
	}
}

func TestEmitEvidence_RejectsUnknownTopLevelField(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"evidence":[{"kind":"direct","source":"x.go"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure on unknown top-level field")
	}
	if !strings.Contains(res.Summary, "invalid params") {
		t.Errorf("got: %s", res.Summary)
	}
}

func TestEmitEvidence_RejectsUnknownItemField(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[{"kind":"direct","source":"x.go","note":"hi"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure on unknown item field")
	}
	if !strings.Contains(res.Summary, "unknown field") {
		t.Errorf("error should mention unknown field: %s", res.Summary)
	}
}

func TestEmitEvidence_RejectsMissingSource(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[{"kind":"direct","subject":"Foo"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure on missing source")
	}
}

func TestEmitEvidence_RejectsNonPathSource(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[{"kind":"direct","source":"FooBar"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure: 'FooBar' has no slash or dot, not path-shaped")
	}
}

func TestEmitEvidence_RejectsStringLineStart(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[{"kind":"direct","source":"x.go","line_start":"twelve"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure on string line_start")
	}
}

func TestEmitEvidence_RejectsRelationshipWithoutObject(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[{"kind":"relationship","subject":"A","source":"x.go","line_start":1}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure: relationship needs object")
	}
}

// TestEmitEvidence_AutoSwapsLineEndBeforeStart pins the 2026-04-17
// change: line_end < line_start is treated as an obvious
// transposition typo and auto-swapped. The summary includes a
// "double-check the range" warning so the LLM can notice. Earlier
// behaviour rejected the whole batch over this single keystroke
// mistake; trace 1776439797257469553 iter=2 lost four otherwise-
// valid evidence items to a single typo on item[3].
func TestEmitEvidence_AutoSwapsLineEndBeforeStart(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[{
		"kind":"direct","source":"x.go","line_start":50,"line_end":10,
		"anchor_kind":"definition","anchor_symbol":"Foo",
		"subject":"Foo","summary":"test"
	}]}`)
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("auto-swap path must succeed, got rejection: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "AUTO-SWAPPED") {
		t.Errorf("summary must warn about auto-swap: %q", res.Summary)
	}
	// Item should be in the accepted buffer with line_start=10, line_end=50.
	emitted := ctx.Mutable.EmittedEvidence()
	if len(emitted) != 1 {
		t.Fatalf("expected 1 accepted item, got %d", len(emitted))
	}
	if emitted[0].LineStart != 10 || emitted[0].LineEnd != 50 {
		t.Errorf("swapped range wrong: got [%d, %d], want [10, 50]",
			emitted[0].LineStart, emitted[0].LineEnd)
	}
}

// TestEmitEvidence_PerItemRejectSkipsInsteadOfFailingBatch locks in
// the graceful-degrade behaviour: a single invalid item in a mostly-
// healthy batch skips that item and keeps the rest. Fires ONLY when
// the reject ratio is below 50%; above that the batch is rejected
// wholesale.
func TestEmitEvidence_PerItemRejectSkipsInsteadOfFailingBatch(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	// 4 valid + 1 invalid (missing anchor_kind). 1/5 = 20% → sparse
	// reject path. Batch succeeds with 4 accepted; 1 skipped in the
	// Summary. (The healthy items all need required fields filled.)
	params := json.RawMessage(`{"items":[
		{"kind":"direct","source":"a.go","line_start":1,"anchor_kind":"definition","anchor_symbol":"A","subject":"A","summary":"x"},
		{"kind":"direct","source":"b.go","line_start":2,"anchor_kind":"definition","anchor_symbol":"B","subject":"B","summary":"x"},
		{"kind":"direct","source":"c.go","line_start":3,"subject":"C","summary":"x"},
		{"kind":"direct","source":"d.go","line_start":4,"anchor_kind":"definition","anchor_symbol":"D","subject":"D","summary":"x"},
		{"kind":"direct","source":"e.go","line_start":5,"anchor_kind":"definition","anchor_symbol":"E","subject":"E","summary":"x"}
	]}`)
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("sparse-reject batch must succeed, got rejection: %s", res.Summary)
	}
	if len(ctx.Mutable.EmittedEvidence()) != 4 {
		t.Errorf("expected 4 accepted, got %d", len(ctx.Mutable.EmittedEvidence()))
	}
	if !strings.Contains(res.Summary, "SKIPPED") {
		t.Errorf("summary must mention skipped item: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "items[2]") {
		t.Errorf("summary must identify the bad item index: %q", res.Summary)
	}
}

// TestEmitEvidence_MajorityRejectFailsEntireBatch — when ≥ 50% of
// items fail, the whole call fails with every reason listed in one
// error envelope. Keeps poisoned batches from slipping through.
func TestEmitEvidence_MajorityRejectFailsEntireBatch(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	// 1 valid + 2 invalid (missing anchor_kind on two items).
	// 2/3 = 66% → majority reject path.
	params := json.RawMessage(`{"items":[
		{"kind":"direct","source":"a.go","line_start":1,"anchor_kind":"definition","anchor_symbol":"A","subject":"A","summary":"x"},
		{"kind":"direct","source":"b.go","line_start":2,"subject":"B","summary":"x"},
		{"kind":"direct","source":"c.go","line_start":3,"subject":"C","summary":"x"}
	]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("majority-reject batch must fail wholesale, got success: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "2 of 3 items") {
		t.Errorf("summary must report the ratio: %q", res.Summary)
	}
}

func TestEmitEvidence_RejectsEmptyItems(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure on empty items")
	}
}

func TestEmitEvidence_RejectsMissingMutable(t *testing.T) {
	tool := &EmitEvidence{}
	res, _ := tool.Execute(&types.BusContext{}, json.RawMessage(`{"items":[]}`))
	if res.Success {
		t.Fatal("expected failure when Mutable is nil")
	}
}

func TestEmitEvidence_StableIDDedups(t *testing.T) {
	// Same content emitted twice produces identical IDs so the
	// downstream merger can dedup. Verify the IDs are stable.
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	body := `{"items":[{"kind":"direct","subject":"Foo","source":"a/b.go","line_start":7,"summary":"x","anchor_kind":"definition","anchor_symbol":"Foo"}]}`
	_, _ = tool.Execute(ctx, json.RawMessage(body))
	_, _ = tool.Execute(ctx, json.RawMessage(body))
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 2 {
		t.Fatalf("want 2 items in buffer (dedup happens later), got %d", len(got))
	}
	if got[0].ID == "" || got[0].ID != got[1].ID {
		t.Errorf("IDs should match for identical content: %q vs %q", got[0].ID, got[1].ID)
	}
}

func TestEmitEvidence_ResetEmittedEvidence(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	_, _ = tool.Execute(ctx, json.RawMessage(`{"items":[{"kind":"direct","source":"x.go","line_start":1,"anchor_kind":"definition","anchor_symbol":"X"}]}`))
	if len(ctx.Mutable.EmittedEvidence()) != 1 {
		t.Fatalf("expected 1 item before reset")
	}
	ctx.Mutable.ResetEmittedEvidence()
	if len(ctx.Mutable.EmittedEvidence()) != 0 {
		t.Errorf("expected empty buffer after reset")
	}
}

func TestEmitEvidence_ToolSurface(t *testing.T) {
	tool := &EmitEvidence{}
	if tool.Name() != "emit_evidence" {
		t.Errorf("name = %q", tool.Name())
	}
	if tool.IsWrite() {
		t.Errorf("emit_evidence must not be write-classified")
	}
	if tool.Confidence() != 0 {
		t.Errorf("confidence = %f, want 0 (NonEvidenceTool — the tool itself isn't a fact source)", tool.Confidence())
	}
	if !strings.Contains(tool.Description(), "emit_evidence") &&
		!strings.Contains(tool.Description(), "structured evidence") {
		// loose check; the description must guide the LLM
		t.Errorf("description should explain the tool's purpose")
	}
	if !json.Valid(tool.Parameters()) {
		t.Errorf("parameters JSON schema is invalid")
	}
}
