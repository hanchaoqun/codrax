package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
)

// finalize_repair_arbitration_test.go — S3' ④ pins (§29.47.1 user ruling,
// 2026-07-12; witnesses 2779 + 76278: soft findings drew whole-answer
// rewrites whose second drafts were WORSE). The strict finalize-local
// repair lane keeps its repair round, but the accepted patch draft ships
// only when strictly better (named cleared ∧ zero new kinds ∧ no
// block/facet loss); otherwise the FIRST draft ships (发第一稿+附注).
// 红线: precise-signal selection between two model drafts — the system
// edits neither.

func repairArbFirstDraftDoc() *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal,
			FacetIDs: []string{"observed_artifact_fact"},
			Text:     "主根因是 app.main-42591 的 D-state 等待，合计 36.757ms。"},
		{ID: "chain_hops", Kind: types.BlockSection, FacetIDs: []string{"mechanism_path"},
			Text: "链路：app-9511 → .ugc.aweme.lite-17267。"},
	}}
}

func repairArbHarness(t *testing.T) (*Orchestrator, *types.MutableState, *agent.StageOutput, []finalizeRepairDraftRecord, *finalizeRepairArbitration) {
	t.Helper()
	mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "36.757"))
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut, Language: "zh"}}
	doc := repairArbFirstDraftDoc()
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	out := &agent.StageOutput{FinalAnswer: "第一稿正文"}
	strict := []types.Violation{{Kind: types.ViolBlockCoverageMissing, Detail: "missing next_steps"}}
	ledger := recordFinalizeRepairDraft(nil, out, mut, strict, 1, 3)
	if len(ledger) != 1 {
		t.Fatalf("first draft must be retained, got %d records", len(ledger))
	}
	arb := armFinalizeRepairArbitration(strict, strict, ledger, out, doc)
	if arb == nil {
		t.Fatalf("a strict finalize-local repair round must arm the arbitration")
	}
	return o, mut, out, ledger, arb
}

// TestFinalizeRepairArbitrationRestoresFirstDraftOnDegradedPatch — the
// degraded shape (丢块/新违规): the patch clears the named kind but drops a
// facet-covered block AND introduces a new violation kind → the FIRST
// draft ships, and the one-line note (for the system cross-check appendix)
// states the outcome.
func TestFinalizeRepairArbitrationRestoresFirstDraftOnDegradedPatch(t *testing.T) {
	o, mut, out, ledger, arb := repairArbHarness(t)
	// Patch draft: chain_hops (facet mechanism_path) is GONE.
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2", Blocks: []types.AnswerBlock{
			{ID: "summary", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal,
				FacetIDs: []string{"observed_artifact_fact"}, Text: "补丁后的正文。"},
		}})
	out.FinalAnswer = "补丁稿正文"
	note, restored := o.arbitrateFinalizeRepairDraft(arb, ledger, out,
		[]types.Violation{{Kind: types.ViolFacetUncovered, Detail: "new"}})
	if !restored {
		t.Fatalf("a degraded patch (lost block/facet + new kind) must restore the first draft")
	}
	if !strings.Contains(out.FinalAnswer, "app.main-42591") || strings.Contains(out.FinalAnswer, "补丁后的正文") {
		t.Fatalf("the first draft's content must ship, got %q", out.FinalAnswer)
	}
	if !strings.Contains(note, "已保留第一稿") {
		t.Fatalf("the note must state the arbitration outcome, got %q", note)
	}
	if doc := mut.AnswerDocumentV2(); doc == nil || len(doc.Blocks) != 2 {
		t.Fatalf("the first draft's document must be restored, got %+v", doc)
	}
}

// TestFinalizeRepairArbitrationShipsStrictlyBetterPatch — the healthy
// shape: named kind cleared, no new kinds, coverage kept → the patch ships
// and no note is emitted.
func TestFinalizeRepairArbitrationShipsStrictlyBetterPatch(t *testing.T) {
	o, mut, out, ledger, arb := repairArbHarness(t)
	improved := repairArbFirstDraftDoc()
	improved.Blocks = append(improved.Blocks, types.AnswerBlock{
		ID: "next_steps", Kind: types.BlockSection, Text: "验证窗口重放。"})
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, improved)
	out.FinalAnswer = "补丁改善后的正文"
	note, restored := o.arbitrateFinalizeRepairDraft(arb, ledger, out, nil)
	if restored || note != "" {
		t.Fatalf("a strictly better patch must ship as-is, got restored=%v note=%q", restored, note)
	}
	if out.FinalAnswer != "补丁改善后的正文" {
		t.Fatalf("the patch must remain the shipping answer, got %q", out.FinalAnswer)
	}
}

// TestFinalizeRepairArbitrationNamedKindPersisting — a patch that reaches
// an accept point with the NAMED kind still present (e.g. accepted through
// the soft/non-actionable branch after a policy change) is not strictly
// better — the first draft ships.
func TestFinalizeRepairArbitrationNamedKindPersisting(t *testing.T) {
	o, mut, out, ledger, arb := repairArbHarness(t)
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, repairArbFirstDraftDoc())
	out.FinalAnswer = "补丁但未清除点名问题的正文"
	note, restored := o.arbitrateFinalizeRepairDraft(arb, ledger, out,
		[]types.Violation{{Kind: types.ViolBlockCoverageMissing, Detail: "still missing"}})
	if !restored || note == "" {
		t.Fatalf("a patch that keeps the named kind must yield to the first draft, got restored=%v", restored)
	}
	if strings.Contains(out.FinalAnswer, "补丁但未清除点名问题的正文") {
		t.Fatalf("the patch's rendered answer must not ship, got %q", out.FinalAnswer)
	}
}

// TestFinalizeRepairArbitrationArmRequiresRetainedDraft — the arm refuses
// when the FRCAP ledger did not retain THIS draft (blank-answer bound
// etc.) and when there is nothing to repair.
func TestFinalizeRepairArbitrationArmRequiresRetainedDraft(t *testing.T) {
	mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "36.757"))
	doc := repairArbFirstDraftDoc()
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	out := &agent.StageOutput{FinalAnswer: "第一稿正文"}
	strict := []types.Violation{{Kind: types.ViolBlockCoverageMissing, Detail: "x"}}
	if arb := armFinalizeRepairArbitration(strict, strict, nil, out, doc); arb != nil {
		t.Fatalf("an empty ledger must never arm")
	}
	ledger := recordFinalizeRepairDraft(nil, out, mut, strict, 1, 3)
	other := &agent.StageOutput{FinalAnswer: "不同的草稿"}
	if arb := armFinalizeRepairArbitration(strict, strict, ledger, other, doc); arb != nil {
		t.Fatalf("a ledger that retained a DIFFERENT draft must never arm")
	}
	if arb := armFinalizeRepairArbitration(nil, nil, ledger, out, doc); arb != nil {
		t.Fatalf("an empty violation set must never arm")
	}
}

// TestFinalizeRepairArbitrationSoftKindJitterShipsPatch — P2-2 narrowing
// pin: a patch draft that clears the named strict kind and only picks up a
// NEW information-lane soft finding (soft kinds jitter round to round)
// ships as-is — the soft finding discloses on the appendix, never through
// draft selection.
func TestFinalizeRepairArbitrationSoftKindJitterShipsPatch(t *testing.T) {
	o, mut, out, ledger, arb := repairArbHarness(t)
	improved := repairArbFirstDraftDoc()
	improved.Blocks = append(improved.Blocks, types.AnswerBlock{
		ID: "next_steps", Kind: types.BlockSection, Text: "验证窗口重放。"})
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, improved)
	out.FinalAnswer = "补丁改善后的正文"
	note, restored := o.arbitrateFinalizeRepairDraft(arb, ledger, out,
		[]types.Violation{{Kind: types.ViolProseScalarUngrounded, Detail: "45ms"}})
	if restored || note != "" {
		t.Fatalf("a NEW soft-only kind must never veto the patch draft (P2-2), got restored=%v note=%q", restored, note)
	}
	if out.FinalAnswer != "补丁改善后的正文" {
		t.Fatalf("the patch must remain the shipping answer, got %q", out.FinalAnswer)
	}
}

// TestFinalizeRepairArbitrationResidualStrictDisclosed — P2-2 residual
// disclosure: when the arbitration restores the first draft, its unfixed
// STRICT concerns render on the system cross-check appendix (the P6
// residual-concerns family on this surface).
func TestFinalizeRepairArbitrationResidualStrictDisclosed(t *testing.T) {
	o, mut, out, ledger, arb := repairArbHarness(t)
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2", Blocks: []types.AnswerBlock{
			{ID: "summary", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal,
				FacetIDs: []string{"observed_artifact_fact"}, Text: "补丁后的正文。"}}})
	out.FinalAnswer = "补丁稿正文"
	note, restored := o.arbitrateFinalizeRepairDraft(arb, ledger, out, nil)
	if !restored {
		t.Fatalf("the lost-block patch must restore the first draft")
	}
	o.attachSystemCrossCheckAppendix(out, note, ledger[arb.record].Violations)
	atts := o.busCtx.Mutable.AnswerDisplayAttachments()
	if len(atts) != 1 {
		t.Fatalf("expected the appendix attachment, got %+v", atts)
	}
	body := atts[0].Body
	if !strings.Contains(body, "已保留第一稿") {
		t.Fatalf("the arbitration note must open the appendix:\n%s", body)
	}
	if !strings.Contains(body, "未完全解决") {
		t.Fatalf("the restored draft's residual strict concerns must disclose on the appendix:\n%s", body)
	}
}
