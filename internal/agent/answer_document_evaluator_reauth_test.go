package agent

// Marker-stripping class root fix pins — ParseOutput no-emit fallback
// lane (audit 2026-07-10, MARKER batch; finding #69b).
//
// Pre-fix RED shape: recoverRetryStateAnswerDocumentV2 decoded
// RetryState.PrevEmitJSON and rendered it directly as the user-facing
// FinalAnswer; the json:"-" SystemGeneratedKind marker was stripped by
// the snapshot round trip, so genuine runtime-trace report chapters
// demoted from `##` to ###/bold on the recovered answer.

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestRecoverRetryStateAnswerDocumentV2_Reauthenticates — the fallback
// doc regains exactly the captured authority: the genuine system block
// passes RuntimeTraceSystemBlock again; an unmarked reserved-ID
// lookalike riding the same snapshot stays model grade.
func TestRecoverRetryStateAnswerDocumentV2_Reauthenticates(t *testing.T) {
	src := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal, Text: "正文。"},
		{ID: "runtime_trace_metric_snapshot", Kind: types.BlockSection,
			Title: "关键指标核对", Text: "聚合影响 46.821ms",
			SystemGeneratedKind: types.AnswerSystemGeneratedRuntimeTrace},
		{ID: "runtime_trace_facts", Kind: types.BlockSection, Text: "模型仿冒块"},
	}}
	raw, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	mut := types.NewMutableState("q")
	mut.SetRetryState(&types.RetryState{
		Attempt:                  1,
		PrevEmitJSON:             raw,
		PrevEmitSystemBlockKinds: types.CaptureSystemGeneratedBlockKinds(src),
	})
	ctx := &types.AgentContext{Mutable: mut}

	doc, ok := recoverRetryStateAnswerDocumentV2(ctx)
	if !ok || doc == nil || len(doc.Blocks) != 3 {
		t.Fatalf("fallback doc missing, got ok=%v doc=%+v", ok, doc)
	}
	if !tool.RuntimeTraceSystemBlock(doc.Blocks[1]) {
		t.Fatalf("fallback doc lost system authority on the PrevEmitJSON round trip: %+v", doc.Blocks[1])
	}
	if tool.RuntimeTraceSystemBlock(doc.Blocks[2]) {
		t.Fatalf("FORGERY: unmarked reserved-ID lookalike gained authority on the fallback lane")
	}
}

// TestRecoverRetryStateAnswerDocumentV2_RejectedDraftArmUntouched — the
// LastRejectedAnswerDocumentV2 arm is an in-memory clone whose markers
// survive on their own; the re-authentication step must not interfere
// with (or be required by) that arm.
func TestRecoverRetryStateAnswerDocumentV2_RejectedDraftArmUntouched(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.SetLastRejectedAnswerDocumentV2(&types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "runtime_trace_metric_snapshot", Kind: types.BlockSection, Title: "关键指标核对",
			Text: "聚合影响 46.821ms", SystemGeneratedKind: types.AnswerSystemGeneratedRuntimeTrace},
	}})
	ctx := &types.AgentContext{Mutable: mut}
	doc, ok := recoverRetryStateAnswerDocumentV2(ctx)
	if !ok || doc == nil || len(doc.Blocks) != 1 {
		t.Fatalf("rejected-draft arm missing, got ok=%v doc=%+v", ok, doc)
	}
	if !tool.RuntimeTraceSystemBlock(doc.Blocks[0]) {
		t.Fatalf("in-memory rejected draft must keep its live marker: %+v", doc.Blocks[0])
	}
}
