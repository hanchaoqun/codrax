package tool

import (
	"fmt"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// These tests pin the maxBlocksPerDoc headroom invariant (see the
// constant's doc): system-side block inserters that run between the
// model's emit and validateMergedV2Doc must degrade to a no-op at the
// cap instead of pushing a model-fitting document into a hard reject
// naming a block count the model never produced. The fused-split
// budget can legitimately fill a document to exactly the cap, so
// every one of these inserters composes with it.

func atCapDoc() *types.AnswerDocumentV2 {
	doc := &types.AnswerDocumentV2{DocumentModel: "v2"}
	for i := 0; i < maxBlocksPerDoc; i++ {
		doc.Blocks = append(doc.Blocks, types.AnswerBlock{
			ID: fmt.Sprintf("p%d", i), Kind: types.BlockSummary, Text: "prose",
		})
	}
	return doc
}

// externalPerfBus returns a ctx whose attached perf trace qualifies
// for runtime_trace_facts materialization (external source + one
// structured observation).
func externalPerfBus() *types.BusContext {
	mu := types.NewMutableState("perf question")
	mu.SetPerfTrace(&types.PerfBundle{
		Meta: types.PerfMeta{Source: "hitrace"},
		Observations: []types.PerfObservation{
			{Kind: "time_semantics", Summary: "trace clock is boottime"},
		},
	})
	return &types.BusContext{Mutable: mu}
}

func TestMaterializeRuntimeTraceObservationBlock_SkipsAtCap(t *testing.T) {
	ctx := externalPerfBus()

	// Control: with headroom the materializer inserts.
	below := atCapDoc()
	below.Blocks = below.Blocks[:maxBlocksPerDoc-1]
	if !materializeRuntimeTraceObservationBlock(below, ctx) {
		t.Fatal("control broken: materializer should insert with headroom")
	}
	if len(below.Blocks) != maxBlocksPerDoc {
		t.Fatalf("control: expected %d blocks, got %d", maxBlocksPerDoc, len(below.Blocks))
	}
	if err := validateMergedV2Doc(below); err != nil {
		t.Fatalf("control doc must pass the gate: %v", err)
	}

	// At cap: skip, never overflow into a fabricated reject.
	at := atCapDoc()
	if materializeRuntimeTraceObservationBlock(at, ctx) {
		t.Fatal("materializer must skip at the cap")
	}
	if len(at.Blocks) != maxBlocksPerDoc {
		t.Fatalf("at-cap doc mutated: %d blocks", len(at.Blocks))
	}
	if err := validateMergedV2Doc(at); err != nil {
		t.Fatalf("at-cap doc must stay accepted: %v", err)
	}
}

func TestMaterializeRequiredCaveatWhenOnlyMissing_SkipsAtCap(t *testing.T) {
	view := &types.AnswerSemanticView{
		UncertaintyRules: []types.UncertaintyRule{{}},
	}
	hints := []emitFixHint{{Field: "blocks[].kind=caveat"}}

	below := atCapDoc()
	below.Blocks = below.Blocks[:maxBlocksPerDoc-1]
	if got := materializeRequiredCaveatWhenOnlyMissing(below, view, hints, &types.BusContext{}); got != 1 {
		t.Fatalf("control broken: caveat should materialize with headroom, got %d", got)
	}

	at := atCapDoc()
	if got := materializeRequiredCaveatWhenOnlyMissing(at, view, hints, &types.BusContext{}); got != 0 {
		t.Fatalf("caveat materializer must skip at the cap, got %d", got)
	}
	if len(at.Blocks) != maxBlocksPerDoc {
		t.Fatalf("at-cap doc mutated: %d blocks", len(at.Blocks))
	}
}

func TestAppendAggregateMemberSetCarrierBlock_SkipsAtCap(t *testing.T) {
	below := atCapDoc()
	below.Blocks = below.Blocks[:maxBlocksPerDoc-1]
	if idx := appendAggregateMemberSetCarrierBlock(below, 0, "Members"); idx < 0 {
		t.Fatal("control broken: carrier should append with headroom")
	}

	at := atCapDoc()
	if idx := appendAggregateMemberSetCarrierBlock(at, 0, "Members"); idx != -1 {
		t.Fatalf("carrier must return -1 at the cap, got %d", idx)
	}
	if len(at.Blocks) != maxBlocksPerDoc {
		t.Fatalf("at-cap doc mutated: %d blocks", len(at.Blocks))
	}
}

func TestAppendRequiredMechanismAnchorBlock_SkipsAtCap(t *testing.T) {
	below := atCapDoc()
	below.Blocks = below.Blocks[:maxBlocksPerDoc-1]
	if idx := appendRequiredMechanismAnchorBlock(below, &types.BusContext{}); idx < 0 {
		t.Fatal("control broken: anchor block should append with headroom")
	}

	at := atCapDoc()
	if idx := appendRequiredMechanismAnchorBlock(at, &types.BusContext{}); idx != -1 {
		t.Fatalf("anchor block must return -1 at the cap, got %d", idx)
	}
	if len(at.Blocks) != maxBlocksPerDoc {
		t.Fatalf("at-cap doc mutated: %d blocks", len(at.Blocks))
	}
}

// TestPersistMergedAtCapWithExternalTrace_EndToEnd pins the proven
// composition: a budget-sanctioned split (or any model emit) filling
// the merged doc to exactly maxBlocksPerDoc, with an external perf
// trace attached, must persist — the trace materializer skips instead
// of fabricating "merged doc has 65 blocks".
func TestPersistMergedAtCapWithExternalTrace_EndToEnd(t *testing.T) {
	ctx := externalPerfBus()
	doc := atCapDoc()
	res, err := persistMergedAnswerDocument(ctx, "emit_answer_document", types.MutationReplaceAll, "test mutation", doc, time.Now())
	if err != nil {
		t.Fatalf("persist returned transport error: %v", err)
	}
	if !res.Success {
		t.Fatalf("at-cap doc with external trace must persist, got refusal: %s", res.Summary)
	}
	if got := ctx.Mutable.AnswerDocumentV2(); got == nil || len(got.Blocks) != maxBlocksPerDoc {
		t.Fatalf("persisted doc shape wrong: %+v", got)
	}
}
