package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
)

// citation_floor_waiver_sync_test.go — F1 (arch_stability_batch_plan
// 2026-07-02): waiver × finalize ViolCitation sync pins.
//
// A model-declared EvidenceFloorWaiver that survived every
// emit_investigation_complete completion gate is retained into the
// stable answer-surface slot (MutableState.RetainEvidenceFloorWaiver).
// The finalize citation floor MUST honor that typed post-acceptance
// signal directly: a waived observation-only answer legitimately has
// zero repo citations, and re-raising ViolCitation /
// ViolAcceptance(CitationReq) every finalize pass while the completion
// gate keeps waiving is the F1 divergence-window ping-pong.
//
// These tests mirror the system-detected external-source pin
// (TestFinalizerCitationSupportCount_RuntimeTraceObservationOnly...
// in contract_check_test.go) for the model-declared lane. All
// assertions are on typed state — violation kinds and typed cluster
// keys — never on prose.

// waiverSyncCitationFloorContract declares the citation floor shape the
// divergence window needs: CitationReq.Required plus the acceptance-side
// citation_count_ge mirror, so the pins cover both checker producers.
func waiverSyncCitationFloorContract() types.AnswerContract {
	return types.AnswerContract{
		CitationReq: types.CitationReq{
			Required:     true,
			Granularity:  "file_line",
			MinCitations: 2,
		},
		AcceptanceTests: []types.Criterion{
			{Kind: types.CritCitationCountGE, Expr: "2"},
		},
	}
}

// citationFloorViolations filters the violations produced by the two
// citation-floor lanes: ViolCitation (CitationReq) and the
// citation_count_ge acceptance mirror, which carries the typed root
// cluster key for CitationReq.
func citationFloorViolations(vs []types.Violation) []types.Violation {
	var out []types.Violation
	for _, v := range vs {
		if v.Kind == types.ViolCitation {
			out = append(out, v)
			continue
		}
		if v.Kind == types.ViolAcceptance && v.ClusterKey == types.RootClusterKey("CitationReq") {
			out = append(out, v)
		}
	}
	return out
}

// TestRunContractCheck_RetainedWaiverWaivesCitationFloor is the F1-T4
// per-class pin: for each of the four typed waiver reasons, a retained
// (post-acceptance) waiver plus a zero-citation draft must produce no
// citation-floor violation from runContractCheck. Before F1-T1 this
// was the divergence window: emit_investigation_complete kept
// completing via the waiver bypass while every finalize pass re-raised
// ViolCitation + ViolAcceptance(CitationReq).
//
// No log/perf bundle is attached — this exercises the referenced-path
// acceptance route where the RuntimeGroundingDisposition projection is
// intentionally dropped (commit 20c0f3fc), so only the typed
// StableEvidenceFloorWaiver arm can waive the floor.
func TestRunContractCheck_RetainedWaiverWaivesCitationFloor(t *testing.T) {
	for _, reason := range types.EvidenceFloorWaiverReasonValues() {
		t.Run(string(reason), func(t *testing.T) {
			mut := types.NewMutableState("只分析引用的运行时工件，不分析代码")
			mut.SetEvidenceFloorWaiver(&types.EvidenceFloorWaiver{
				Reason:    reason,
				Rationale: "the referenced runtime artifact answers the question; repo grounding is not applicable",
			})
			mut.RetainEvidenceFloorWaiver(true)
			if !mut.StableEvidenceFloorWaiver().IsActive() {
				t.Fatal("test setup: retained waiver must be active in the stable slot")
			}

			out := &agent.StageOutput{FinalAnswer: "observation-only answer with no repo citations"}
			res := runContractCheck(out, waiverSyncCitationFloorContract(), mut, nil)
			if got := citationFloorViolations(res.Violations); len(got) != 0 {
				t.Fatalf("retained evidence-floor waiver must waive the finalize citation floor; got %+v", got)
			}
		})
	}
}

// TestRunContractCheck_NoWaiverCitationFloorStillFires is the F1-T4
// negative control: without a waiver the citation floor itself is NOT
// weakened — a zero-citation draft still produces ViolCitation and the
// acceptance-side mirror.
func TestRunContractCheck_NoWaiverCitationFloorStillFires(t *testing.T) {
	mut := types.NewMutableState("分析 orchestrator 的重试逻辑")
	out := &agent.StageOutput{FinalAnswer: "answer with no citations at all"}
	res := runContractCheck(out, waiverSyncCitationFloorContract(), mut, nil)
	got := citationFloorViolations(res.Violations)
	if len(got) == 0 {
		t.Fatal("citation floor must still fire for a zero-citation draft without a waiver")
	}
	foundCitation := false
	for _, v := range got {
		if v.Kind == types.ViolCitation {
			foundCitation = true
		}
	}
	if !foundCitation {
		t.Fatalf("expected ViolCitation among citation-floor violations; got %+v", got)
	}
}

// TestRunContractCheck_StaleWaiverClearedFloorApplies is the F1-T4
// stale-waiver control: the pending slot is declared-until-retracted
// across DENIED attempts, so a later accepted completion whose payload
// did NOT declare the waiver must clear the stable slot even though the
// stale pending waiver is still active — otherwise a declaration from an
// earlier denied attempt would arm the finalize citation-floor relax for
// a completion that stood on repo evidence (adversarial-review finding
// on the first F1 cut).
func TestRunContractCheck_StaleWaiverClearedFloorApplies(t *testing.T) {
	mut := types.NewMutableState("先分析外部 trace，再分析仓库代码")
	mut.SetEvidenceFloorWaiver(&types.EvidenceFloorWaiver{
		Reason:    types.EvidenceFloorWaiverExternalTrace,
		Rationale: "trace spans do not resolve to the current repository",
	})
	mut.RetainEvidenceFloorWaiver(true)

	out := &agent.StageOutput{FinalAnswer: "zero-citation draft"}
	if got := citationFloorViolations(runContractCheck(out, waiverSyncCitationFloorContract(), mut, nil).Violations); len(got) != 0 {
		t.Fatalf("phase 1: retained waiver must waive the citation floor; got %+v", got)
	}

	// A later accepted completion whose payload omitted the waiver: the
	// pending declaration is STILL active (declared-until-retracted),
	// but retention with declaredThisAttempt=false must wipe the stable
	// slot instead of promoting the stale declaration.
	mut.RetainEvidenceFloorWaiver(false)
	if mut.StableEvidenceFloorWaiver() != nil {
		t.Fatal("stable waiver slot must be cleared by a waiver-less accepted completion, even with a stale pending declaration")
	}

	if got := citationFloorViolations(runContractCheck(out, waiverSyncCitationFloorContract(), mut, nil).Violations); len(got) == 0 {
		t.Fatal("phase 2: citation floor must apply again after the stable waiver is cleared")
	}
}

// TestFinalizeCitationFloorSuccessCriterionWaived pins the F1-T2 SC
// merge arm: the finalize SuccessCriteria loop waives exactly the two
// citation-floor criterion kinds (citation_count_ge +
// contract_satisfied) through the SAME chokepoint that relaxes
// CitationReq in runContractCheck — a retained waiver waives both, an
// unrelated criterion kind is never waived, and without a waiver
// nothing is waived.
func TestFinalizeCitationFloorSuccessCriterionWaived(t *testing.T) {
	waived := types.NewMutableState("只分析引用的运行时工件")
	waived.SetEvidenceFloorWaiver(&types.EvidenceFloorWaiver{
		Reason:    types.EvidenceFloorWaiverNoRepoIntersection,
		Rationale: "apparent repo-path intersections are coincidental; the artifact answers the question",
	})
	waived.RetainEvidenceFloorWaiver(true)
	bare := types.NewMutableState("分析仓库代码")

	cases := []struct {
		name string
		kind string
		mut  *types.MutableState
		want bool
	}{
		{"citation_count_ge waived", types.CritCitationCountGE, waived, true},
		{"contract_satisfied waived", types.CritContractSatisfied, waived, true},
		{"unrelated kind never waived", types.CritEvidenceCount, waived, false},
		{"no waiver not waived", types.CritCitationCountGE, bare, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := finalizeCitationFloorSuccessCriterionWaived(tc.kind, tc.mut, nil); got != tc.want {
				t.Fatalf("finalizeCitationFloorSuccessCriterionWaived(%q) = %v, want %v", tc.kind, got, tc.want)
			}
		})
	}

	// Single-truth-source pin: the SC waive must agree with the
	// checker-side relax for the same MutableState.
	if finalizeCitationFloorSuccessCriterionWaived(types.CritCitationCountGE, waived, nil) !=
		runtimeArtifactCitationFloorWaived(waived, nil) {
		t.Fatal("SC merge waive and checker citation-floor relax diverged for the same state")
	}
}
