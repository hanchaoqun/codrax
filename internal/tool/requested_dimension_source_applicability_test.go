package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func runtimeDimensionSourceContext() *types.BusContext {
	ctx := dimensionOwnershipContext(
		types.RequestedAnswerDimension{Index: 1, Role: types.RequestedAnswerDimensionCausalAttribution, Required: true},
		types.RequestedAnswerDimension{Index: 2, Role: types.RequestedAnswerDimensionCausalContributorSet, Required: true},
		types.RequestedAnswerDimension{Index: 3, Role: types.RequestedAnswerDimensionFunctionOrPurpose, Required: true},
		types.RequestedAnswerDimension{Index: 4, Role: types.RequestedAnswerDimensionFunctionOrPurpose, Required: true},
	)
	ctx.AttachedHitrace = "runtime trace attachment"
	ctx.AnalysisIR.RequestModel.PerfTrace = &types.PerfBundle{Observations: []types.PerfObservation{{
		Kind: "root_cause_rank", Subject: "app-17", LineStart: 10, LineEnd: 20,
	}}}
	ctx.AnalysisIR.RequestModel.ExternalObservationPolicy = &types.ExternalObservationPolicy{
		CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
		ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary,
		SourceQuotes:      []string{"opaque anchored user boundary"},
	}
	return ctx
}

func TestRequestedDimensionSourceApplicabilityCompletionAndAdvisory(t *testing.T) {
	for _, tc := range []struct {
		name       string
		adjust     func(*types.BusContext)
		wantSource bool
	}{
		{name: "explicit runtime-only boundary"},
		{name: "attached trace alone", wantSource: true, adjust: func(ctx *types.BusContext) {
			ctx.AnalysisIR.RequestModel.ExternalObservationPolicy = nil
		}},
		{name: "exclude without quote", wantSource: true, adjust: func(ctx *types.BusContext) {
			ctx.AnalysisIR.RequestModel.ExternalObservationPolicy.SourceQuotes = nil
		}},
		{name: "exclude without explicit kind", wantSource: true, adjust: func(ctx *types.BusContext) {
			ctx.AnalysisIR.RequestModel.ExternalObservationPolicy.ExclusionKind = ""
		}},
		{name: "mixed source allowed", wantSource: true, adjust: func(ctx *types.BusContext) {
			ctx.AnalysisIR.RequestModel.ExternalObservationPolicy.CurrentSourceMode = types.ExternalObservationCurrentSourceAllow
			ctx.AnalysisIR.RequestModel.AnalyzerHints.RequiredFileHints = []types.RequiredFileHint{{
				Path: "worker.go", Confidence: 1, RequestedDimensionIndices: []int{3},
			}}
		}},
		{name: "independent precise source obligation", wantSource: true, adjust: func(ctx *types.BusContext) {
			ctx.AnalysisIR.RequestModel.ExternalObservationPolicy.CurrentSourceMode = types.ExternalObservationCurrentSourceDefault
			ctx.AnalysisIR.RequestModel.AnalyzerHints.RequiredFileHints = []types.RequiredFileHint{{
				Path: "worker.go", Confidence: 1, RequestedDimensionIndices: []int{3},
			}}
		}},
		{name: "validated exclusion retains existing authority precedence", adjust: func(ctx *types.BusContext) {
			ctx.AnalysisIR.RequestModel.AnalyzerHints.RequiredFileHints = []types.RequiredFileHint{{
				Path: "worker.go", Confidence: 1, RequestedDimensionIndices: []int{3},
			}}
		}},
		{name: "ordinary current-source request", wantSource: true, adjust: func(ctx *types.BusContext) {
			ctx.AttachedHitrace = ""
			ctx.AnalysisIR.RequestModel.PerfTrace = nil
			ctx.AnalysisIR.RequestModel.ExternalObservationPolicy = nil
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := runtimeDimensionSourceContext()
			if tc.adjust != nil {
				tc.adjust(ctx)
			}
			before, _ := json.Marshal(ctx.AnalysisIR.RequestModel)
			operation := []types.EvidenceItem{{
				Kind: types.EvidenceMechanism, AnchorKind: types.AnchorCall, Source: "other.go",
				GroundingStatus: types.GroundingGrounded,
			}}
			for name, got := range map[string]string{
				"completion":             requestedDimensionEvidenceOwnershipDowngrade(ctx, nil),
				"emit_evidence advisory": renderRequestedDimensionOperationOwnershipAdvisory(ctx, operation, operation),
			} {
				if (got != "") != tc.wantSource {
					t.Errorf("%s source requirement=%v want=%v: %s", name, got != "", tc.wantSource, got)
				}
				if tc.wantSource && (!strings.Contains(got, "3 (function_or_purpose)") || !strings.Contains(got, "4 (function_or_purpose)")) {
					t.Errorf("%s dropped required source dimensions: %s", name, got)
				}
			}
			skip := renderEmitEvidenceExternalObservationSoftSkipSummary(ctx, []string{"runtime trace row"})
			if tc.wantSource && strings.Contains(skip, "call emit_investigation_complete directly") {
				t.Error("a waived citation floor must not promise completion while independent source operation seats remain")
			}
			if !tc.wantSource && !strings.Contains(skip, "call emit_investigation_complete directly") {
				t.Error("runtime-only completion guidance should retain the existing direct handoff")
			}
			after, _ := json.Marshal(ctx.AnalysisIR.RequestModel)
			if string(before) != string(after) {
				t.Fatal("applicability must not rewrite analysis roles, required dimensions or file ownership")
			}
		})
	}
}

// r1024 reached this source-only gate even after emit_evidence explicitly said
// runtime observations could complete without current-source citations.
func TestEmitInvestigationCompleteRuntimeDimensionSourceExclusionClosesOnFirstAttempt(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })
	ctx := runtimeDimensionSourceContext()
	ctx.AnalysisIR.AnswerContract.CitationReq = types.CitationReq{Required: true, MinCitations: 2, Granularity: "file_line"}
	if !emitEvidenceCompletionCitationFloorWaived(ctx) {
		t.Fatal("fixture must exercise the existing runtime-only completion promise")
	}
	res, err := (&EmitInvestigationComplete{}).Execute(ctx, json.RawMessage(`{
		"reason":"The runtime observations support the requested directions; relationships without interval proof remain unresolved.",
		"confidence":"high", "result_kind":"resolved"
	}`))
	if err != nil || !res.Success || strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("explicit runtime-only dimensions must close without source-ownership retries: err=%v result=%+v", err, res)
	}
	if ctx.Mutable.InvestigationCompleteReason() == "" {
		t.Fatal("the first completion must be stored, not accepted only by convergence escape")
	}
}

func TestEmitEvidenceDimensionSourcePromiseDoesNotConflateFloorWithOperationOwnership(t *testing.T) {
	ctx := runtimeDimensionSourceContext()
	ctx.AnalysisIR.RequestModel.ExternalObservationPolicy = nil
	ctx.Mutable.SetEvidenceFloorWaiver(&types.EvidenceFloorWaiver{
		Reason: types.EvidenceFloorWaiverExternalTrace, Rationale: "runtime observation citation-count waiver",
	})
	if !emitEvidenceCompletionCitationFloorWaived(ctx) {
		t.Fatal("fixture must reach the existing citation-floor waiver promise")
	}
	if got := requestedDimensionEvidenceOwnershipDowngrade(ctx, nil); got == "" {
		t.Fatal("a citation-count waiver alone must not broaden this fix's source-ownership applicability")
	}
	got := renderEmitEvidenceExternalObservationSoftSkipSummary(ctx, []string{"runtime trace row"})
	if strings.Contains(got, "call emit_investigation_complete directly") ||
		!strings.Contains(got, "citation-count floor is waived") ||
		!strings.Contains(got, "operation evidence is still required") {
		t.Fatalf("the hint must distinguish the waived floor from operation ownership: %s", got)
	}
}
