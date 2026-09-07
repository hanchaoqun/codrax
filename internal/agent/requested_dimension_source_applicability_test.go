package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestExplorerDimensionOwnershipGuideRespectsExactSourceApplicability(t *testing.T) {
	for _, tc := range []struct {
		name       string
		adjust     func(*types.AgentContext)
		wantSource bool
	}{
		{name: "explicit runtime-only boundary"},
		{name: "attached trace without waiver", wantSource: true, adjust: func(ctx *types.AgentContext) {
			ctx.AnalysisIR.RequestModel.ExternalObservationPolicy = nil
		}},
		{name: "exclude lacks provenance", wantSource: true, adjust: func(ctx *types.AgentContext) {
			ctx.AnalysisIR.RequestModel.ExternalObservationPolicy.SourceQuotes = nil
		}},
		{name: "mixed runtime and source", wantSource: true, adjust: func(ctx *types.AgentContext) {
			ctx.AnalysisIR.RequestModel.ExternalObservationPolicy.CurrentSourceMode = types.ExternalObservationCurrentSourceAllow
			ctx.AnalysisIR.RequestModel.AnalyzerHints.RequiredFileHints = []types.RequiredFileHint{{
				Path: "worker.go", Confidence: 1, RequestedDimensionIndices: []int{3},
			}}
		}},
		{name: "independent exact source target", wantSource: true, adjust: func(ctx *types.AgentContext) {
			ctx.AnalysisIR.RequestModel.ExternalObservationPolicy.CurrentSourceMode = types.ExternalObservationCurrentSourceDefault
			ctx.AnalysisIR.RequestModel.AnalyzerHints.ExactTargets = []string{"worker.go:9"}
		}},
		{name: "validated exclusion retains existing authority precedence", adjust: func(ctx *types.AgentContext) {
			ctx.AnalysisIR.RequestModel.AnalyzerHints.ExactTargets = []string{"worker.go:9"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := requestedDimensionEvidenceOwnershipContext()
			ctx.AnalysisIR.RequestModel.PerfTrace = &types.PerfBundle{Observations: []types.PerfObservation{{Kind: "root_cause_rank", Subject: "app-17"}}}
			ctx.AnalysisIR.RequestModel.ExternalObservationPolicy = &types.ExternalObservationPolicy{
				CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
				ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary,
				SourceQuotes:      []string{"opaque anchored user boundary"},
			}
			if tc.adjust != nil {
				tc.adjust(ctx)
			}
			before, _ := json.Marshal(ctx.AnalysisIR.RequestModel)
			got := renderExplorerRequestedDimensionEvidenceOwnershipGuide(ctx)
			if (got != "") != tc.wantSource {
				t.Fatalf("ownership guide present=%v want=%v: %s", got != "", tc.wantSource, got)
			}
			prompt := (&explorerEvaluator{}).BuildInitialInstruction(ctx, nil)
			if strings.Contains(prompt, "### Requested Explanation Evidence Ownership") != tc.wantSource {
				t.Fatal("actual explorer prompt differs from shared source applicability")
			}
			after, _ := json.Marshal(ctx.AnalysisIR.RequestModel)
			if string(before) != string(after) {
				t.Fatal("teaching must preserve the analyzer's roles and required dimensions")
			}
		})
	}
}
