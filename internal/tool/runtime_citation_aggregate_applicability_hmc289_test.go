package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Real native proof and incidental reads must not turn advisory aggregates into
// source-count obligations or promote those aggregates to principal evidence.
func TestHMC289CitationAuditLegacyAggregateCannotCreateSourceCount(t *testing.T) {
	for _, tc := range []struct {
		name            string
		fact            *types.AnswerAggregateFact
		wantSourceFloor bool
	}{
		{name: "no_aggregate"},
		{name: "catalog_support_token", fact: &types.AnswerAggregateFact{Kind: types.AnswerAggregateTotalCount, Label: "Capture count", Value: "4", Unit: "captures", SupportRefs: []string{"trace_catalog:discover captures root, candidate_count=4, discovery complete=true, regular_files_seen=4"}}},
		{name: "untyped_advisory", fact: &types.AnswerAggregateFact{Kind: types.AnswerAggregateScalar, Label: "Model-only retained note", Value: "4"}},
		{name: "explicit_current_source", wantSourceFloor: true, fact: &types.AnswerAggregateFact{Kind: types.AnswerAggregateScalar, Label: "Source value", Value: "4", Provenance: string(types.AnswerEvidenceOriginCurrentSource)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := nativeMeasurementHandoffContext(t)
			ctx.AnalysisIR.AnswerContract.CitationReq = types.CitationReq{Required: true, MinCitations: 3, Granularity: "file_line"}
			path := filepath.Join(ctx.RepoRoot, "incidental.go")
			if err := os.WriteFile(path, []byte("package fixture\n"), 0600); err != nil {
				t.Fatal(err)
			}
			args, _ := json.Marshal(map[string]any{"path": path})
			read, err := (&ReadFile{}).Execute(ctx, args)
			if err != nil || !read.Success {
				t.Fatalf("public incidental read: %+v %v", read, err)
			}
			ctx.Mutable.AppendDispatchToolResult(read)
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
			a := runtimeSourceAnswerAuthorityForCompletion(ctx)
			if a.CurrentSourceRequired || !a.CurrentSourceSatisfied || !types.NativeRuntimeAnswerSourceCitationFloorNotApplicable(ctx, nil) {
				t.Fatalf("no source duty plus genuine native proof prerequisite: %+v", a)
			}
			var facts []types.AnswerAggregateFact
			if tc.fact != nil {
				facts = append(facts, *tc.fact)
			}
			for _, f := range facts {
				t.Logf("origins=%v explicit=%v origin_requires_source=%v", types.AnswerAggregateFactEvidenceOrigins(f, &ctx.AnalysisIR.RequestModel), aggregateFactExplicitCompletionOrigins(f), aggregateMemberSetOriginRequiresCurrentSource(ctx, &ctx.AnalysisIR.RequestModel, []types.AnswerAggregateFact{f}))
			}
			label, bypass := traceQueryRuntimeObservationCompletionBypassLabel(ctx, facts)
			t.Logf("typed_source_floor_not_applicable=true compatible=%v bypass=%v label=%q", traceQueryRuntimeObservationBypassCompatibleWithAggregateFacts(ctx, facts), bypass, label)
			if bypass == tc.wantSourceFloor {
				t.Fatalf("typed bypass disagrees with expected source duty: bypass=%v want_floor=%v", bypass, tc.wantSourceFloor)
			}
			before, _ := json.Marshal([]any{ctx.Mutable.DispatchToolResults(), ctx.Mutable.TurnAArtifacts()})
			params, _ := json.Marshal(map[string]any{"reason": "Retain accepted observations within their own scope", "confidence": "high", "result_kind": "resolved", "aggregate_facts": facts})
			result, err := (&EmitInvestigationComplete{}).Execute(ctx, params)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("public result success=%v complete=%v summary=%s", result.Success, ctx.Mutable.IsInvestigationComplete(), result.Summary)
			gotSourceFloor := strings.Contains(result.Summary, "cite-eligible") || strings.Contains(result.Summary, "citation preflight failed")
			if gotSourceFloor != tc.wantSourceFloor {
				t.Fatalf("source citation floor=%v want %v: no typed source obligation must not be created by legacy/advisory aggregate origin", gotSourceFloor, tc.wantSourceFloor)
			}
			if !result.Success || ctx.Mutable.IsInvestigationComplete() == tc.wantSourceFloor {
				t.Fatalf("public lifecycle changed or positive case failed for a different reason: success=%v complete=%v want_floor=%v", result.Success, ctx.Mutable.IsInvestigationComplete(), tc.wantSourceFloor)
			}
			after, _ := json.Marshal([]any{ctx.Mutable.DispatchToolResults(), ctx.Mutable.TurnAArtifacts()})
			if string(before) != string(after) || ctx.Mutable.StableEvidenceFloorWaiver().IsActive() {
				t.Fatal("source-floor applicability changed original native proof or required a waiver")
			}
			if !tc.wantSourceFloor && tc.fact != nil {
				if len(ctx.Mutable.StableInvestigationAggregateFacts()) != 1 {
					t.Fatal("completion discarded the retained aggregate instead of correcting applicability")
				}
				f := ctx.Mutable.StableInvestigationAggregateFacts()[0]
				if f.Label != tc.fact.Label || f.Value != tc.fact.Value || types.AnswerAggregateFactAuthorizesPrincipalContractWithSourceContext(f, &ctx.AnalysisIR.RequestModel, types.AnswerAggregateSourceContextFromBusContext(ctx)) {
					t.Fatal("retained legacy/advisory aggregate changed value or gained principal authority")
				}
			}
		})
	}
}

func TestHMC289CitationAuditPreciseBoundaryNegativeControls(t *testing.T) {
	for _, tc := range []struct {
		name             string
		edit             func(*types.BusContext)
		fact             *types.AnswerAggregateFact
		onlyNativeBranch bool
	}{
		{"required_route_with_precise_source_dimension", func(ctx *types.BusContext) {
			ctx.TurnRouteHint.CurrentSourceEvidenceMode = types.TurnRouteCurrentSourceEvidenceRequired
			ctx.TurnRouteHint.RequiredOutcomes = types.TurnOutcomeMeasurement | types.TurnOutcomeSourceExplanation
			ctx.AnalysisIR.RequestModel.RequestedAnswerDimensions = &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{{Role: types.RequestedAnswerDimensionSourceLocation, Required: true, SourceQuote: "current source location"}}}
		}, nil, false},
		{"current_version_native_branch_stays_inapplicable", func(ctx *types.BusContext) { ctx.AnalysisIR.RequestModel.DiagnosticProfile.CurrentVersionCheck = true }, nil, true},
		{"explicit_other_origin_keeps_separate_origin_path", nil, &types.AnswerAggregateFact{Kind: types.AnswerAggregateScalar, Label: "external claim", Value: "4", Provenance: string(types.AnswerEvidenceOriginExternalDocument)}, true},
		{"unsuccessful_native", func(ctx *types.BusContext) {
			results := ctx.Mutable.DispatchToolResults()
			ctx.Mutable = types.NewMutableState("failed query does not waive floor")
			for _, r := range results {
				r.Success = false
				ctx.Mutable.AppendDispatchToolResult(r)
			}
		}, nil, false},
		{"unaddressable_native", func(ctx *types.BusContext) {
			results := ctx.Mutable.DispatchToolResults()
			ctx.Mutable = types.NewMutableState("unaddressed query does not waive floor")
			for _, r := range results {
				for i := range r.Observations {
					r.Observations[i].SourceRef = types.ObservationSourceRef{}
				}
				ctx.Mutable.AppendDispatchToolResult(r)
			}
		}, nil, false},
		{"summary_json_is_not_native_proof", func(ctx *types.BusContext) {
			ctx.Mutable = types.NewMutableState("summary JSON cannot mint native proof")
			ctx.Mutable.AppendDispatchToolResult(types.ToolResult{ToolName: "trace_query", Success: true, Summary: `{"origin":"runtime_artifact","producer":"trace_query","resource_stack":{"count":4}}`})
		}, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := nativeMeasurementHandoffContext(t)
			ctx.AnalysisIR.AnswerContract.CitationReq = types.CitationReq{Required: true, MinCitations: 3}
			if tc.edit != nil {
				tc.edit(ctx)
			}
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
			var facts []types.AnswerAggregateFact
			if tc.fact != nil {
				facts = append(facts, *tc.fact)
			}
			nativeApplicable := types.NativeRuntimeAnswerSourceCitationFloorNotApplicable(ctx, nil)
			label, bypass := traceQueryRuntimeObservationCompletionBypassLabel(ctx, facts)
			t.Logf("native_source_floor_not_applicable=%v bypass=%v label=%q authority=%+v", nativeApplicable, bypass, label, runtimeSourceAnswerAuthorityForCompletion(ctx))
			if tc.fact == nil && nativeApplicable {
				t.Fatal("precise native applicability accepted an independent duty or missing proof")
			}
			if tc.fact != nil && (len(aggregateFactExplicitCompletionOrigins(*tc.fact)) == 0 || traceQueryRuntimeObservationBypassCompatibleWithAggregateFacts(ctx, facts)) {
				t.Fatal("explicit other origin lost its independent grounding path")
			}
			if strings.HasPrefix(label, "native_runtime_source_citation_not_applicable:") {
				t.Fatal("precise-only branch accepted an independent duty, other origin, or missing verified proof")
			}
			// These cases have separate legacy landing semantics. The baseline
			// control log proves those are unchanged; this patch only owns the
			// precise native citation-applicability branch, not other validators.
			if tc.onlyNativeBranch {
				return
			}
			if bypass {
				t.Fatal("native bypass accepted independent duty or unverified native proof")
			}
			args, _ := json.Marshal(map[string]any{"reason": "candidate records retain their original authority", "confidence": "high", "result_kind": "resolved", "aggregate_facts": facts})
			r, err := (&EmitInvestigationComplete{}).Execute(ctx, args)
			if err != nil {
				t.Fatal(err)
			}
			if ctx.Mutable.IsInvestigationComplete() {
				t.Fatalf("completion lost proof boundary: %+v", r)
			}
		})
	}
}
