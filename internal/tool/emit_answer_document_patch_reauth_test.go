package tool

// Marker-stripping class root fix pins — patch-base lane (audit
// 2026-07-10, MARKER batch; finding #68).
//
// Pre-fix RED shape: RetryState.PrevEmitJSON (a system-side snapshot of
// the persisted document) lost the json:"-" SystemGeneratedKind marker,
// so when ResetForFallback cleared the live doc and the patch tool fell
// back to the snapshot as its base, persist-time
// normalizeRuntimeTraceReservedBlockIDCollisions treated the GENUINE
// system blocks as model-authored collisions, renamed them to
// model_runtime_trace_*, and the materializers re-minted fresh copies —
// duplicated runtime-trace chapters with the stale ones laundered as
// model analysis.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func patchReauthSnapshot(t *testing.T, doc *types.AnswerDocumentV2) *types.MutableState {
	t.Helper()
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	mut := types.NewMutableState("q")
	mut.SetRetryState(&types.RetryState{
		Attempt:                  1,
		PrevEmitJSON:             raw,
		PrevEmitSystemBlockKinds: types.CaptureSystemGeneratedBlockKinds(doc),
	})
	return mut
}

// TestRecoverPrevFromRetryState_ReauthenticatesSystemBlocks — the patch
// base decoded from PrevEmitJSON regains system authority: the genuine
// block passes RuntimeTraceSystemBlock again and the collision
// normalizer performs ZERO renames (pre-fix: rename to
// model_runtime_trace_* → duplicate chapters downstream).
func TestRecoverPrevFromRetryState_ReauthenticatesSystemBlocks(t *testing.T) {
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal, Text: "正文。"},
		{ID: "runtime_trace_metric_snapshot", Kind: types.BlockSection,
			Title: "关键指标核对", Text: "聚合影响 46.821ms",
			SystemGeneratedKind: types.AnswerSystemGeneratedRuntimeTrace},
	}}
	prev := recoverPrevFromRetryState(patchReauthSnapshot(t, doc))
	if prev == nil || len(prev.Blocks) != 2 {
		t.Fatalf("patch base missing, got %+v", prev)
	}
	if !RuntimeTraceSystemBlock(prev.Blocks[1]) {
		t.Fatalf("patch base lost system authority on the PrevEmitJSON round trip: %+v", prev.Blocks[1])
	}
	if renamed := normalizeRuntimeTraceReservedBlockIDCollisions(prev); renamed != 0 {
		t.Fatalf("collision normalizer renamed %d GENUINE system block(s) — duplicate-chapter shape is back", renamed)
	}
	if prev.Blocks[1].ID != "runtime_trace_metric_snapshot" {
		t.Fatalf("system block id must keep its reserved spelling, got %q", prev.Blocks[1].ID)
	}
}

// TestRecoverPrevFromRetryState_ForgeryLaneStaysClosed — a reserved-ID
// block that was NOT marked when the snapshot was taken (a model-
// authored lookalike that bypassed the persist choke, e.g. via lossless
// text recovery) must come back UNMARKED and still be renamed by the
// collision normalizer: re-authentication restores exactly the captured
// authority, never minting any from ID spelling.
func TestRecoverPrevFromRetryState_ForgeryLaneStaysClosed(t *testing.T) {
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "正文。"},
		// Exact reserved spelling, NO live marker at snapshot time.
		{ID: "runtime_trace_facts", Kind: types.BlockSection, Text: "模型仿冒 99.999ms"},
	}}
	prev := recoverPrevFromRetryState(patchReauthSnapshot(t, doc))
	if prev == nil {
		t.Fatalf("patch base missing")
	}
	if RuntimeTraceSystemBlock(prev.Blocks[1]) {
		t.Fatalf("FORGERY: unmarked reserved-ID lookalike gained authority through the snapshot lane")
	}
	if renamed := normalizeRuntimeTraceReservedBlockIDCollisions(prev); renamed != 1 {
		t.Fatalf("model-authored reserved-ID collision must still be renamed, got %d", renamed)
	}
	if !strings.HasPrefix(prev.Blocks[1].ID, "model_runtime_trace_facts") {
		t.Fatalf("lookalike must leave the reserved namespace, got %q", prev.Blocks[1].ID)
	}
}
