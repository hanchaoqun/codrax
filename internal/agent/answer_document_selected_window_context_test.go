package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func answerDocExplicitWindowTestContext() *types.AgentContext {
	start, end := 1.0, 1.01
	return &types.AgentContext{
		AgentName: types.AgentFinalizer,
		Stage:     types.StageFinalize,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{
				RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
				TimeStart:      &start,
				TimeEnd:        &end,
				SourceQuote:    "1.000000..1.010000",
			},
		}},
	}
}

func answerDocWindowObservation(id, window string) types.ObservationRecord {
	return types.ObservationRecord{
		ID:        id,
		Origin:    types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:  "trace_query",
		Role:      types.AnswerAggregateRolePrincipalAnswer,
		Subject:   "app-20",
		Predicate: "root_cause_rank",
		RichNotes: []string{types.TraceNoteKeySelectedWindow + "=" + window},
	}
}

func TestAnswerDocSelectedWindowObservationRecordsKeepsOnlyContainedTypedQueryWindows(t *testing.T) {
	ctx := answerDocExplicitWindowTestContext()
	records := []types.ObservationRecord{
		answerDocWindowObservation("exact", "1.000000..1.010000"),
		answerDocWindowObservation("contained", "1.001000..1.009500"),
		answerDocWindowObservation("post", "1.010000..1.010020"),
		answerDocWindowObservation("wider", "1.000000..1.020000"),
		{
			ID:       "no-window",
			Origin:   types.AnswerEvidenceOriginRuntimeArtifact,
			Producer: "trace_query",
		},
		{
			ID:        "model-runtime",
			Origin:    types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:  "explorer_model",
			RichNotes: []string{types.TraceNoteKeySelectedWindow + "=1.010000..1.020000"},
		},
		{
			ID:        "repo-row",
			Origin:    types.AnswerEvidenceOriginCurrentSource,
			Producer:  "trace_query",
			RichNotes: []string{types.TraceNoteKeySelectedWindow + "=1.010000..1.020000"},
		},
	}

	got, omitted := answerDocSelectedWindowObservationRecords(ctx, records)
	if omitted != 2 {
		t.Fatalf("omitted=%d, want 2: %#v", omitted, got)
	}
	want := []string{"exact", "contained", "no-window", "model-runtime", "repo-row"}
	if len(got) != len(want) {
		t.Fatalf("kept=%d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].ID != want[i] {
			t.Fatalf("kept[%d].ID=%q, want %q: %#v", i, got[i].ID, want[i], got)
		}
	}
}

func TestAnswerDocSelectedWindowObservationRecordsIsFinalizerOnly(t *testing.T) {
	ctx := answerDocExplicitWindowTestContext()
	ctx.AgentName = types.AgentExplorer
	ctx.Stage = types.StageExplore
	records := []types.ObservationRecord{answerDocWindowObservation("post", "1.010000..1.010020")}

	got, omitted := answerDocSelectedWindowObservationRecords(ctx, records)
	if omitted != 0 || len(got) != 1 || got[0].ID != "post" {
		t.Fatalf("non-finalizer evidence pool must stay lossless: omitted=%d got=%#v", omitted, got)
	}
}

func TestAnswerDocToolHandoffCarriersProjectKnownOutsideWindowRefsOnly(t *testing.T) {
	ctx := answerDocExplicitWindowTestContext()
	ctx.Mutable = types.NewMutableState("trace window")
	records := []types.ObservationRecord{
		answerDocWindowObservation("exact", "1.000000..1.010000"),
		answerDocWindowObservation("post", "1.010000..1.010020"),
		answerDocWindowObservation("wider", "1.000000..1.020000"),
	}
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName:     "trace_query",
		Success:      true,
		Observations: records,
	})
	carriers := []types.ToolHandoffCarrier{{
		Version:    types.ToolHandoffCarrierVersion,
		ToolName:   "trace_query",
		ReasonCode: "tool_observation_handoff",
		ObservationRefs: []types.ToolObservationRef{
			{ID: "exact"},
			{ID: "post"},
			{ID: "wider"},
			{ID: "unknown-from-another-carrier"},
		},
	}}

	got := answerDocToolHandoffCarriersForFinalizer(ctx, carriers)
	if len(got) != 1 {
		t.Fatalf("carrier repair/evidence shell must survive: %#v", got)
	}
	refs := got[0].ObservationRefs
	if len(refs) != 2 || refs[0].ID != "exact" || refs[1].ID != "unknown-from-another-carrier" {
		t.Fatalf("handoff refs=%#v, want exact + unknown only", refs)
	}
}

func TestAnswerDocInvestigationNarrativeHandoffDoesNotReplayBesideDeterministicRuntimeRows(t *testing.T) {
	ctx := answerDocExplicitWindowTestContext()
	ctx.Mutable = types.NewMutableState("trace window")
	ctx.Mutable.SetPerfTrace(&types.PerfBundle{Observations: []types.PerfObservation{{
		Kind:    "state_churn",
		Subject: "app-20",
		Summary: "attached trace runtime observation",
	}}})
	ctx.AnalysisIR.RequestModel.PerfTrace = ctx.Mutable.PerfTrace()
	ctx.AnalysisIR.RequestModel.ExternalObservationPolicy = &types.ExternalObservationPolicy{
		CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
		ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary,
		Confidence:        0.9,
	}
	const stale = "stale broad-window conclusion at 1.010000..1.010020"
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{InvestigationNotes: []string{stale}})
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "trace_query",
		Success:  true,
		Observations: []types.ObservationRecord{
			answerDocWindowObservation("exact", "1.000000..1.010000"),
		},
	})

	if !runtimeObservationOnlyForAnswerDoc(ctx) {
		t.Fatal("fixture must exercise the runtime-observation-only finalizer lane")
	}
	if got := renderAnswerDocInvestigationNarrativeHandoff(ctx); got != "" {
		t.Fatalf("deterministic runtime facts must not receive a second narrative authority:\n%s", got)
	}
	if ta := ctx.Mutable.TurnAArtifacts(); ta == nil || len(ta.InvestigationNotes) != 1 || !strings.Contains(ta.InvestigationNotes[0], stale) {
		t.Fatalf("audit artifacts must retain the original investigation note: %#v", ta)
	}
}
