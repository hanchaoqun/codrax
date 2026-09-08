package tracefinding

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestCandidateWindowScopeUsesRankDonorNotSeedOrOccurrence(t *testing.T) {
	start, end := 2.0, 2.02
	profile := &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
		TimeStart: &start, TimeEnd: &end, SourceQuote: "2.0..2.02"}
	projection := types.TraceCausalProjection{WindowStartTs: 2, WindowEndTs: 2.021,
		WindowScope: types.ResolveTraceQueryWindowScope(profile, 2, 2.021)}
	for _, tc := range []struct {
		name string
		node types.TraceCausalProjectionNode
		want types.TraceQueryWindowScopeRole
		end  float64
	}{
		{"rank donor requested", types.TraceCausalProjectionNode{QueryWindowStartTs: 2, QueryWindowEndTs: 2.021, RankQueryWindowStartTs: 2, RankQueryWindowEndTs: 2.02}, types.TraceQueryWindowScopeRequestedPrincipal, 2.02},
		{"rank donor supplementary", types.TraceCausalProjectionNode{QueryWindowStartTs: 2, QueryWindowEndTs: 2.02, RankQueryWindowStartTs: 2, RankQueryWindowEndTs: 2.021}, types.TraceQueryWindowScopeSupportingExploration, 2.021},
		{"ordinary query", types.TraceCausalProjectionNode{QueryWindowStartTs: 2, QueryWindowEndTs: 2.021}, types.TraceQueryWindowScopeSupportingExploration, 2.021},
		{"occurrence is not query", types.TraceCausalProjectionNode{StartTs: 2, EndTs: 2.02}, types.TraceQueryWindowScopeUnknownQueryWindow, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			facts := candidateEvidenceFacts(projection, tc.node, "", "")
			if facts.WindowScope == nil || facts.WindowScope.Role != tc.want || facts.WindowScope.QueryWindowEndTs != tc.end {
				t.Fatalf("wrong measurement source: %+v", facts.WindowScope)
			}
		})
	}
}

func TestRootCauseWindowScopeDoesNotDisplacePhysicalLocatorUnderEvidenceCap(t *testing.T) {
	start, end := 2.0, 2.02
	profile := &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
		TimeStart: &start, TimeEnd: &end, SourceQuote: "2.0..2.02"}
	scope := types.ResolveTraceQueryWindowScope(profile, 2, 2.021)
	facts := &types.TraceCauseEvidenceFacts{WindowScope: &scope, WindowStartTs: 2, WindowEndTs: 2.021,
		SeatStartTs: 2.020, SeatEndTs: 2.021, LineStart: 70, LineEnd: 80, ArtifactLabel: strings.Repeat("长名称", 30)}
	locator := rootCauseEvidenceFit(rootCauseEvidenceLocatorSentence(facts))
	for _, want := range []string{"第 70–80 行", "发生 2.020000–2.021000 s", "投影范围 2.000000–2.021000 s"} {
		if !strings.Contains(locator, want) {
			t.Fatalf("scope displaced physical locator %q: %s", want, locator)
		}
	}
	if strings.Contains(locator, "分析窗") || scope.RequestedWindowEndTs != 2.02 {
		t.Fatalf("lost ruler role or full typed scope: %s / %+v", locator, scope)
	}
	if got := rootCauseEvidenceLocatorSentence(&types.TraceCauseEvidenceFacts{WindowScope: &scope}); got != scope.Format("zh") {
		t.Fatalf("scope-only evidence was lost: %s", got)
	}
}
