package tool

// answer_document_projection_selfrun_disc_test.go — SELFRUN-DISC (§29.192①
// (b) user ruling; A2 件11(b) handoff §29.194, 2026-07-21) display pins: the
// self supply-fold 「量不了」 absence disclosure's ◎ auxiliary 另账 row —
// 产线 A/B (records → compile → render: the disclosure sentence appears with
// the wire record and disappears without it), zh/EN word-face parity, the
// empty-board survival arm, and the render-time admission fail-closed arms.
//
// MUTATION self-checks (突变 cp 纪律, verified red by hand):
//   - dropping either elim.go call site reds
//     TestSelfrunDiscProductionABZH / TestSelfrunDiscEmptyBoardStillDiscloses;
//   - flipping the admission identity (accepting running != unknown) reds
//     TestSelfrunDiscAdmissionFailClosed.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// selfrunDiscProjection — representative shape: one on-chain seat (the board
// renders) plus the admitted absence disclosure for the analysis target.
func selfrunDiscProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:              []string{"dep-200", "app-100"},
		WindowStartTs:           5.000,
		WindowEndTs:             5.041,
		RootCauseFamilyObserved: true,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleRootCauseContext, Subject: "dep-200",
				Object: "sleep_wait", StateKind: "s_sleep", ChainRelevance: "on_chain",
				ImpactMS: 12.0, EffectiveImpactMS: 12.0, Rank: 1, Confidence: 0.8},
		},
		SelfRunningFoldUnmeasured: []types.TraceCausalProjectionSelfRunningFoldUnmeasured{
			{Subject: "app-100", RunningMS: 19.8, UnknownMS: 19.8},
		},
	}
}

// TestSelfrunDiscElimRowZH — the ◎ auxiliary 另账 row carries the ruled
// absence sentence verbatim (词面单点): value-first content, conditional
// disclosure label, no ordinal.
func TestSelfrunDiscElimRowZH(t *testing.T) {
	proj := selfrunDiscProjection()
	model := buildRuntimeTraceProjTreeModel(proj, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	elim := partsplitSquash(runtimeTraceProjElimOverviewFence(proj, model, true))
	if !strings.Contains(elim, partsplitSquash("· 折算不可量 app-100 窗内 running 19.800ms:运行频点未采集,自身降频折算不可量")) {
		t.Fatalf("zh ◎ must carry the 折算不可量 另账 row with the ruled sentence:\n%s", elim)
	}
}

// TestSelfrunDiscElimRowEN — EN parity (same values, EN word face).
func TestSelfrunDiscElimRowEN(t *testing.T) {
	proj := selfrunDiscProjection()
	model := buildRuntimeTraceProjTreeModel(proj, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	elim := partsplitSquash(runtimeTraceProjElimOverviewFence(proj, model, false))
	if !strings.Contains(elim, partsplitSquash("· fold unmeasurable app-100 ran 19.800ms in-window: running-frequency samples were not collected; the self down-clock fold is unmeasurable")) {
		t.Fatalf("EN ◎ must carry the fold-unmeasurable aux row:\n%s", elim)
	}
}

// TestSelfrunDiscEmptyBoardStillDiscloses — a board that admitted nothing is
// exactly the no-frequency-data shape; the absence row must survive on the
// empty-board form (silence here would be the "no loss" face this disclosure
// exists to kill).
func TestSelfrunDiscEmptyBoardStillDiscloses(t *testing.T) {
	proj := selfrunDiscProjection()
	proj.OnChainCauses = nil
	model := buildRuntimeTraceProjTreeModel(proj, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	elim := partsplitSquash(runtimeTraceProjElimOverviewFence(proj, model, true))
	if !strings.Contains(elim, partsplitSquash("运行频点未采集,自身降频折算不可量")) {
		t.Fatalf("the empty-board ◎ form must keep the absence row:\n%s", elim)
	}
}

// TestSelfrunDiscAdmissionFailClosed — render-time admission: a broken
// running==unknown identity (partially-known basis), a non-positive value,
// and a blank subject all render NOTHING (absence silent, 宁缺勿错).
func TestSelfrunDiscAdmissionFailClosed(t *testing.T) {
	mutate := map[string]func(*types.TraceCausalProjection){
		"identity broken (running != unknown — partially-known basis)": func(p *types.TraceCausalProjection) {
			p.SelfRunningFoldUnmeasured[0].UnknownMS = 12.0
		},
		"non-positive running": func(p *types.TraceCausalProjection) {
			p.SelfRunningFoldUnmeasured[0].RunningMS = 0
			p.SelfRunningFoldUnmeasured[0].UnknownMS = 0
		},
		"blank subject": func(p *types.TraceCausalProjection) {
			p.SelfRunningFoldUnmeasured[0].Subject = "  "
		},
	}
	for name, mut := range mutate {
		proj := selfrunDiscProjection()
		mut(&proj)
		model := buildRuntimeTraceProjTreeModel(proj, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
		elim := runtimeTraceProjElimOverviewFence(proj, model, true)
		if strings.Contains(elim, "折算不可量") || strings.Contains(elim, "运行频点未采集") {
			t.Fatalf("%s: the absence row must stay silent:\n%s", name, elim)
		}
	}
}

// selfrunDiscWireObs is the disclosure record in the EXACT production wire
// shape (verbatim key literals — the deliberate wire-format double-write, do
// not replace with constants).
func selfrunDiscWireObs() types.ObservationRecord {
	return types.ObservationRecord{
		ID: "trace_query:t#self_running_fold_unmeasured:1", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard,
		Predicate: "self_running_fold_unmeasured", ClaimKey: "self_running_fold_unmeasured:app-100",
		Subject: "app-100", Object: "running", Value: "19.800", Unit: "ms",
		Span: types.ObservationSpan{LineStart: 2, LineEnd: 7},
		RichNotes: []string{
			"self_running_fold_unmeasured_running_ms=19.800",
			"self_running_fold_unmeasured_unknown_ms=19.800",
			"selected_window=5.000000..5.041000",
		},
	}
}

// TestSelfrunDiscProductionABZH — 产线 A/B through the REAL chain
// (observations → compile/join → blocks → markdown): with the wire record the
// rendered answer carries the ruled sentence; without it, zero bytes of the
// word family remain.
func TestSelfrunDiscProductionABZH(t *testing.T) {
	seat := projV3Obs("root-sleep", "root_cause_primary", "root_cause_primary:sleep",
		"dep-200", "sleep_wait", "12.000", 12.0, 100, 200,
		"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
		"chain_depth=1", "dominant_state=s_sleep", "effective_impact_ms=12.000")
	// A: disclosure record present → the ◎ 另账 row renders the sentence.
	withDisc := []types.ObservationRecord{sfdQ6Anchor(), seat, selfrunDiscWireObs()}
	mdA := audit730Render(t, audit730Bus(""), withDisc, "")
	if !strings.Contains(partsplitSquash(mdA), partsplitSquash("运行频点未采集,自身降频折算不可量")) {
		t.Fatalf("production A: the absence sentence must reach the rendered answer:\n%s", mdA)
	}
	// B: same set minus the disclosure record → the word family is absent.
	mdB := audit730Render(t, audit730Bus(""), []types.ObservationRecord{sfdQ6Anchor(), seat}, "")
	for _, banned := range []string{"折算不可量", "运行频点未采集"} {
		if strings.Contains(mdB, banned) {
			t.Fatalf("production B: %q must not render without the wire record:\n%s", banned, mdB)
		}
	}
}
