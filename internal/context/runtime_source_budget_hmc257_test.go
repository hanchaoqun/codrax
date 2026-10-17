package context

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC257RuntimeDomainDoesNotShrinkEvidenceBudget(t *testing.T) {
	for _, currentVersion := range []bool{false, true} {
		for _, excluded := range []bool{false, true} {
			rm := types.RequestModel{
				Intent:                 types.IntentExplain,
				RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet},
				DiagnosticProfile:      types.DiagnosticIntentProfile{IsDiagnostic: true, CurrentVersionCheck: currentVersion},
			}
			if excluded {
				rm.ExternalObservationPolicy = &types.ExternalObservationPolicy{
					CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
					ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary, SourceQuotes: []string{"do not analyze current source"},
				}
			}
			mu := types.NewMutableState("structured runtime request")
			mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
				ToolName: "trace_query", Success: true,
				Observations: []types.ObservationRecord{{
					ID: "runtime:budget:query", Producer: "trace_query", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
					ClaimAuthority: types.ObservationClaimAuthorityDirectObservation,
					SourceRef:      types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: "/captures/session.trace", PayloadRef: "query-generation-1"},
					Span:           types.ObservationSpan{LineStart: 1}, Summary: "native bounded facts",
				}},
			}}})
			ctx := &types.AgentContext{AgentName: types.AgentFinalizer, Stage: types.StageFinalize, Mutable: mu, AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
			want := defaultEvidenceRenderLimit
			if currentVersion && !excluded {
				want = mixedRuntimeCurrentSourceFinalizerEvidenceLimit
			}
			if got := evidenceRenderLimitForAgentContext(ctx); got != want {
				t.Errorf("current_version=%t source_excluded=%t evidence budget=%d want %d", currentVersion, excluded, got, want)
			}
		}
	}
}
