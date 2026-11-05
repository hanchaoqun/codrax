package agent

import (
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
	"strings"
	"testing"
)

func TestAcceptedAdvisoryPatchFitsWithoutDuplicateFullSchema(t *testing.T) {
	bus, ctx, e := b1672Context("en")
	b1672Emit(t, bus, `,"trace_root_causes":{"schema_version":2,"root_causes":[{"candidate_id":"typo"}]}`)
	signal := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop})
	if !signal.HintRequested {
		t.Fatal("rejected selection must offer existing advisory repair")
	}
	schemas := []llm.ToolSchema{{Name: "emit_answer_document", Description: strings.Repeat("x", 232000)}, {Name: "emit_answer_document_patch", Description: strings.Repeat("y", 232000)}}
	messages := []llm.Message{{Role: "user", Content: strings.Repeat("m", 253340)}}
	e.requestBytePolicy = types.FinalizerRequestByteEnforce
	e.promptBudget = &finalizerPromptBudget{RequestAllocationBytes: 700000}
	if e.finalizerRequestByteError(ctx, messages, schemas) == nil {
		t.Fatal("fixture must reproduce duplicate-schema overflow")
	}
	filtered := e.FilterToolSchemas(ctx, schemas)
	if len(filtered) != 1 || filtered[0].Name != "emit_answer_document_patch" {
		t.Fatal("accepted advisory should use patch only")
	}
	if err := e.finalizerRequestByteError(ctx, messages, filtered); err != nil {
		t.Fatal(err)
	}
	if len(schemas) != 2 || schemas[0].Name != "emit_answer_document" {
		t.Fatal("mutated input schema")
	}
	if bus.Mutable.TraceRootCauseReport() != nil || !bus.Mutable.TraceRootCauseSelectorRejected() {
		t.Fatal("filter must not infer or correct model selection")
	}
	e.forceFullEmitNext = true
	escaped := e.FilterToolSchemas(ctx, schemas)
	if len(escaped) != 1 || escaped[0].Name != "emit_answer_document" {
		t.Fatal("reserved full-rewrite escape must retain priority")
	}
}

func TestAdvisoryPatchFilterKeepsOnlyAvailableRoute(t *testing.T) {
	bus, ctx, e := b1672Context("en")
	e.postEmitAdvisoryDelivered = true
	schemas := []llm.ToolSchema{{Name: "emit_answer_document"}, {Name: "emit_answer_document_patch"}}
	if len(e.FilterToolSchemas(ctx, schemas)) != 2 {
		t.Fatal("no accepted answer must retain ordinary surface")
	}
	b1672Emit(t, bus, "")
	onlyFull := schemas[:1]
	if len(e.FilterToolSchemas(ctx, onlyFull)) != 1 {
		t.Fatal("missing patch must not erase sole callable tool")
	}
	e.postEmitAdvisoryDelivered = false
	if len(e.FilterToolSchemas(ctx, schemas)) != 2 {
		t.Fatal("ordinary first emit surface changed")
	}
}
