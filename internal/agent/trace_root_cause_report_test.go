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
	contract.RootCauseReportEnabled = true
	ctx.Mutable.SetTraceFindingContract(contract)
	description := tracefinding.RootCauseValueDescription(contract.Candidates[0].Decision)
	got := renderTraceFindingContract(ctx)
	if !strings.Contains(got, `"value_description": "`+description+`"`) || !strings.Contains(got, `"impact_ms": 3`) {
		t.Fatalf("selector omitted the exact value meaning: %s", got)
	}
}

func TestRootCauseSelectorContextCarriesMechanismIndependentOfFrameQualifier(t *testing.T) {
	ctx := traceRootCauseTestContext("analyze this trace root cause")
	contract, err := tracefinding.CompileCandidateContract(types.ObservationLedger{}, types.TraceCausalProjectionSet{
		Projections: []types.TraceCausalProjection{{RankedSeats: []types.TraceCausalProjectionNode{{
			EvidenceID: "E1", Subject: "worker", Rank: 1, TypeToken: "priority_inversion_candidate", ChainRelevance: "on_chain",
			ImpactMS: 12, EffectiveImpactMS: 9, EffectiveImpactPublished: true, GatedRunnableMS: 5, GatedRunningDeficitMS: 4,
		}}}}}, tracefinding.SeatFrameCausalityAuthority{})
	if err != nil {
		t.Fatal(err)
	}
	contract.RootCauseReportEnabled = true
	ctx.Mutable.SetTraceFindingContract(contract)
	got := renderTraceFindingContract(ctx)
	for _, want := range []string{`"causal_qualifier": "not_applicable"`, `"mechanism_qualifier": "lower_priority_dependency_candidate"`,
		tracefinding.RootCauseValueDescription(contract.Candidates[0].Decision), types.TraceRootCauseMechanismTeaching()} {
		if !strings.Contains(got, want) {
			t.Fatalf("selector lacks the same mechanism boundary taught by its schema: missing %q in %s", want, got)
		}
	}
}

func TestPrepareTraceFindingContractDoesNotEnableReportWithoutTypedCandidates(t *testing.T) {
	ctx := traceRootCauseTestContext("analyze this trace root cause")
	if err := prepareTraceFindingContract(ctx); err != nil {
		t.Fatalf("prepareTraceFindingContract: %v", err)
	}
	contract := ctx.Mutable.TraceFindingContract()
	if contract == nil || contract.RootCauseReportEnabled {
		t.Fatalf("empty typed roster must not expose root-cause selection: %+v", contract)
	}
	if got := renderAnswerDocTraceDecisionHandoff(ctx); strings.Contains(got, "Trace Root Cause JSON") {
		t.Fatalf("empty roster leaked a root-cause report contract:\n%s", got)
	}
	if report := ctx.Mutable.TraceRootCauseReport(); report != nil {
		t.Fatalf("runtime must not mint a model conclusion: %+v", report)
	}
}

func TestPrepareTraceFindingContractSidecarFlagCannotBypassTypedRoster(t *testing.T) {
	ctx := traceRootCauseTestContext("analyze this trace root cause")
	if err := prepareTraceFindingContract(ctx); err != nil {
		t.Fatal(err)
	}
	if contract := ctx.Mutable.TraceFindingContract(); contract == nil || contract.RootCauseReportEnabled {
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

// V1-1 (§40.25): the caliber closed set in the roster teaching is rendered
// from the types list, never hand-typed.
func TestRenderTraceRootCauseContractTeachesEveryImpactCaliber(t *testing.T) {
	ctx := traceRootCauseTestContext("analyze this trace root cause")
	ctx.Mutable.SetTraceFindingContract(testRosterContract([]string{"a.systrace"}, "a.systrace"))
	got := renderAnswerDocTraceDecisionHandoff(ctx)
	for _, caliber := range types.AllTraceImpactCalibers() {
		if !strings.Contains(got, caliber) {
			t.Fatalf("roster teaching missing caliber %q:\n%s", caliber, got)
		}
	}
	if !strings.Contains(got, "`impact_caliber` ("+strings.Join(types.AllTraceImpactCalibers(), " vs ")+")") {
		t.Fatalf("caliber parenthetical must render from the closed-set list:\n%s", got)
	}
}

func testRosterContract(artifactLabels []string, candidateLabels ...string) *types.TraceFindingContract {
	contract := &types.TraceFindingContract{RootCauseReportEnabled: true, CandidateSetID: "set-1", ArtifactLabels: artifactLabels}
	for i, label := range candidateLabels {
		contract.Candidates = append(contract.Candidates, types.TraceFindingCandidateV1{
			PrimaryEligible: true,
			Decision: types.TraceCauseDecision{
				CandidateID: "candidate-" + string(rune('1'+i)), SubjectName: "RenderThread", ArtifactLabel: label, Rank: 1,
				Token:           types.TraceCausalTokenSnapshot{Token: "scheduler_latency", Lane: "scheduling_demand"},
				Magnitude:       &types.TypedMagnitude{Value: 4, Unit: "ms", Additivity: "wall_clock_per_thread", Caliber: types.TraceImpactCaliberEffectiveAttribution},
				CausalQualifier: types.TraceCausalQualifierProven, EvidenceRefs: []string{"E" + string(rune('1'+i))},
			},
		})
	}
	return contract
}

// V1-4 (§40.26 ②): a multi-artifact roster is grouped by trace file, every
// item carries its partition key and the teaching names the discipline.
func TestRosterGroupsCandidatesByTraceFileWhenMultiArtifact(t *testing.T) {
	ctx := traceRootCauseTestContext("analyze this trace root cause")
	ctx.Mutable.SetTraceFindingContract(testRosterContract([]string{"a.systrace", "b.systrace"}, "a.systrace", "b.systrace"))
	got := renderAnswerDocTraceDecisionHandoff(ctx)
	if strings.Count(got, "```json") != 2 {
		t.Fatalf("two trace files must render two roster fences:\n%s", got)
	}
	for _, want := range []string{
		"**Trace file: a.systrace**", "**Trace file: b.systrace**",
		`"artifact_label": "a.systrace"`, `"artifact_label": "b.systrace"`,
		"Candidates come from 2 trace files", "same name in two trace files is two different candidates",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("multi-artifact roster missing %q:\n%s", want, got)
		}
	}
	if strings.Index(got, "**Trace file: a.systrace**") > strings.Index(got, "**Trace file: b.systrace**") {
		t.Fatalf("groups must follow the contract's partition order:\n%s", got)
	}
}

// §40.48 fold-in (S3/S9): the teaching counts the contract's PARTITION
// ROSTER (ArtifactLabels), not the rendered groups — a trace file whose
// seats yield no selectable candidate is still a partition of the fold and
// is named as contributing none, and an out-of-roster or unlabeled group is
// never counted as a trace file.
func TestRosterTeachingCountsPartitionRosterNotRenderedGroups(t *testing.T) {
	ctx := traceRootCauseTestContext("analyze this trace root cause")
	ctx.Mutable.SetTraceFindingContract(testRosterContract([]string{"a.systrace", "b.systrace"}, "a.systrace"))
	got := renderAnswerDocTraceDecisionHandoff(ctx)
	if strings.Count(got, "```json") != 1 || strings.Count(got, "**Trace file: ") != 1 {
		t.Fatalf("one selectable partition renders one fence under one heading:\n%s", got)
	}
	for _, want := range []string{"Candidates come from 2 trace files", "no selectable candidate: b.systrace"} {
		if !strings.Contains(got, want) {
			t.Fatalf("lone-group multi-artifact roster missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Candidates come from 1 trace files") {
		t.Fatalf("count must read the partition roster, not the rendered groups:\n%s", got)
	}
	// An unlabeled candidate beside two partitions: still 2 trace files.
	ctx = traceRootCauseTestContext("analyze this trace root cause")
	ctx.Mutable.SetTraceFindingContract(testRosterContract([]string{"a.systrace", "b.systrace"}, "a.systrace", "b.systrace", ""))
	got = renderAnswerDocTraceDecisionHandoff(ctx)
	if !strings.Contains(got, "Candidates come from 2 trace files") || !strings.Contains(got, "**Trace file: unlabeled**") || strings.Contains(got, "no selectable candidate") {
		t.Fatalf("the unlabeled trailing group is not a trace file:\n%s", got)
	}
}

func TestSingleArtifactRosterUnchanged(t *testing.T) {
	ctx := traceRootCauseTestContext("analyze this trace root cause")
	ctx.Mutable.SetTraceFindingContract(testRosterContract([]string{"a.systrace"}, "a.systrace"))
	got := renderAnswerDocTraceDecisionHandoff(ctx)
	if strings.Count(got, "```json") != 1 || strings.Contains(got, "Trace file:") || strings.Contains(got, "trace files.") {
		t.Fatalf("single-artifact roster must stay one ungrouped fence:\n%s", got)
	}
	if !strings.Contains(got, `"artifact_label": "a.systrace"`) {
		t.Fatalf("the partition key still rides each item:\n%s", got)
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
	contract.RootCauseReportEnabled = true
	ctx.Mutable.SetTraceFindingContract(contract)
	text := renderTraceFindingContract(ctx)
	for _, want := range []string{"`description`", types.TraceRootCauseDescriptionTeaching(), "frame_unproven"} {
		if !strings.Contains(text, want) {
			t.Fatalf("selector context must teach the description (%q):\n%s", want, text)
		}
	}
}
