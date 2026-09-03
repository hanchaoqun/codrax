package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/types"
)

func traceRootCauseTestContext(objective string) *types.AgentContext {
	return &types.AgentContext{
		Objective: objective,
		Mutable:   types.NewMutableState(objective),
		RuntimeArtifactPreflight: types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{
			Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: "capture.systrace", Carrier: "request_path"}},
		}),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentRootCause}},
	}
}

func TestRootCauseSelectorContextPublishesTheSameValueCaliberAsSidecar(t *testing.T) {
	ctx := traceRootCauseTestContext("analyze this trace root cause")
	contract, err := tracefinding.CompileCandidateContract(types.ObservationLedger{}, types.TraceCausalProjectionSet{
		Projections: []types.TraceCausalProjection{{RankedSeats: []types.TraceCausalProjectionNode{{
			EvidenceID: "E1", Subject: "worker", Rank: 1, TypeToken: "running", ChainRelevance: "on_chain",
			ImpactMS: 10, EffectiveImpactMS: 3, EffectiveImpactPublished: true, SupplyFoldComputed: true,
			SupplyFoldDeficitMS: 3, SupplyFoldIdealMS: 7, SupplyFoldKnownMS: 8, SupplyFoldUnknownMS: 2,
		}}}}}, tracefinding.SeatFrameCausalityAuthority{Applicable: true})
	if err != nil {
		t.Fatal(err)
	}
	contract.Required, contract.RootCauseReportEnabled = false, true
	ctx.Mutable.SetTraceFindingContract(contract)
	description := tracefinding.RootCauseValueDescription(contract.Candidates[0].Decision)
	got := renderTraceFindingContract(ctx)
	if !strings.Contains(got, `"value_description": "`+description+`"`) || !strings.Contains(got, `"impact_ms": 3`) {
		t.Fatalf("selector omitted the exact value meaning: %s", got)
	}
}

func TestPrepareTraceFindingContractDoesNotEnableReportWithoutTypedCandidates(t *testing.T) {
	ctx := traceRootCauseTestContext("analyze this trace root cause")
	if err := prepareTraceFindingContract(ctx); err != nil {
		t.Fatalf("prepareTraceFindingContract: %v", err)
	}
	contract := ctx.Mutable.TraceFindingContract()
	if contract == nil || contract.Required || contract.RootCauseReportEnabled {
		t.Fatalf("empty typed roster must not expose root-cause selection: %+v", contract)
	}
	if got := renderAnswerDocTraceDecisionHandoff(ctx); strings.Contains(got, "Trace Root Cause JSON") {
		t.Fatalf("empty roster leaked a root-cause report contract:\n%s", got)
	}
	if finding := ctx.Mutable.TraceFinding(); finding != nil {
		t.Fatalf("runtime must not mint a model conclusion: %+v", finding)
	}
}

func TestPrepareTraceFindingContractSidecarFlagCannotBypassTypedRoster(t *testing.T) {
	ctx := traceRootCauseTestContext("analyze this trace root cause")
	if err := prepareTraceFindingContract(ctx); err != nil {
		t.Fatal(err)
	}
	if contract := ctx.Mutable.TraceFindingContract(); contract == nil || contract.Required || contract.RootCauseReportEnabled {
		t.Fatalf("caller flag must not manufacture selectable root causes: %+v", contract)
	}
}

func TestRenderTraceRootCauseContractOffersOnlyTypedCandidateIDs(t *testing.T) {
	ctx := traceRootCauseTestContext("analyze this trace root cause")
	ctx.Mutable.SetTraceFindingContract(&types.TraceFindingContract{
		RootCauseReportEnabled: true,
		CandidateSetID:         "set-1",
		Candidates: []types.TraceFindingCandidateV1{{
			PrimaryEligible: true,
			Decision: types.TraceCauseDecision{
				CandidateID: "candidate-1", SubjectName: "RenderThread",
				Token:        types.TraceCausalTokenSnapshot{Token: "scheduler_latency", Lane: "scheduling_demand"},
				Magnitude:    &types.TypedMagnitude{Value: 4, Unit: "ms", Additivity: "wall_clock_per_thread", Caliber: "effective_attribution"},
				EvidenceRefs: []string{"E1"}, CausalQualifier: types.TraceCausalQualifierProven,
			},
		}},
	})
	got := renderAnswerDocTraceDecisionHandoff(ctx)
	if !strings.Contains(got, "Optional Trace Root Cause JSON") || !strings.Contains(got, `"candidate_id": "candidate-1"`) || strings.Contains(got, "Required") {
		t.Fatalf("typed optional selector prompt drifted:\n%s", got)
	}
	for _, teaching := range []string{"`trace_root_causes` in `emit_answer_document`", "`replace_trace_root_causes` in `emit_answer_document_patch`",
		"Do not quote the object or the number", "previously accepted report is retained", "complete ordered selection",
		"Omitting it on a later re-emit keeps the previously accepted selection"} {
		if !strings.Contains(got, teaching) {
			t.Fatalf("full/patch JSON teaching drifted: missing %q in %s", teaching, got)
		}
	}
}

func TestPrepareTraceFindingContractNarrowTraceFactStaysInactive(t *testing.T) {
	ctx := traceRootCauseTestContext("list the target thread state")
	ctx.AnalysisIR.RequestModel = types.RequestModel{
		Intent: types.IntentTrace,
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{
			Scope:        types.RuntimeQuestionScopeBoundedFactSet,
			FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetSchedulerState},
		},
	}
	if err := prepareTraceFindingContract(ctx); err != nil {
		t.Fatal(err)
	}
	if contract := ctx.Mutable.TraceFindingContract(); contract != nil {
		t.Fatalf("narrow fact request must not be widened: %+v", contract)
	}
}

// SIDECAR-Q1 (§40.28 ②): the model-facing roster carries each candidate's
// seat-level qualifier and caliber, plus the teaching line that explains them.
func TestRenderTraceRootCauseContractTeachesQualifierAndCaliber(t *testing.T) {
	ctx := traceRootCauseTestContext("analyze this trace root cause")
	ctx.Mutable.SetTraceFindingContract(&types.TraceFindingContract{
		RootCauseReportEnabled: true,
		CandidateSetID:         "set-1",
		Candidates: []types.TraceFindingCandidateV1{{
			PrimaryEligible: true,
			Decision: types.TraceCauseDecision{
				CandidateID: "candidate-1", SubjectName: "RenderThread", CausalQualifier: types.TraceCausalQualifierFrameUnproven,
				Token:        types.TraceCausalTokenSnapshot{Token: "scheduler_latency", Lane: "scheduling_demand"},
				Magnitude:    &types.TypedMagnitude{Value: 4, Unit: "ms", Additivity: "wall_clock_per_thread", Caliber: "effective_attribution"},
				EvidenceRefs: []string{"E1"},
			},
		}},
	})
	got := renderAnswerDocTraceDecisionHandoff(ctx)
	for _, want := range []string{
		`"impact_caliber": "effective_attribution"`, `"causal_qualifier": "frame_unproven"`,
		"seat-level — the same qualifier the answer headline wears",
		// QUALGATE-1: the closed set names the third value and its meaning.
		"`not_applicable`", "not a frame/jank question",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("roster missing %q:\n%s", want, got)
		}
	}
}

// SIDECAR-NARR-1: the selector context teaches the optional plain-language
// description (same wording family as the schema), without internal names.
func TestRootCauseSelectorContextTeachesPlainLanguageDescription(t *testing.T) {
	ctx := traceRootCauseTestContext("analyze this trace root cause")
	contract, err := tracefinding.CompileCandidateContract(types.ObservationLedger{}, types.TraceCausalProjectionSet{
		Projections: []types.TraceCausalProjection{{RankedSeats: []types.TraceCausalProjectionNode{{
			EvidenceID: "E1", Subject: "worker", Rank: 1, TypeToken: "running", ChainRelevance: "on_chain",
			ImpactMS: 10, EffectiveImpactMS: 3, EffectiveImpactPublished: true,
		}}}}}, tracefinding.SeatFrameCausalityAuthority{Applicable: true})
	if err != nil {
		t.Fatal(err)
	}
	contract.Required, contract.RootCauseReportEnabled = false, true
	ctx.Mutable.SetTraceFindingContract(contract)
	text := renderTraceFindingContract(ctx)
	for _, want := range []string{"`description`", types.TraceRootCauseDescriptionTeaching(), "frame_unproven"} {
		if !strings.Contains(text, want) {
			t.Fatalf("selector context must teach the description (%q):\n%s", want, text)
		}
	}
}
