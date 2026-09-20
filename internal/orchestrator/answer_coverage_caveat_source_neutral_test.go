package orchestrator

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

const sourceNeutralCoverageZH = "答案在某些维度的覆盖度可能不充分，建议结合相关证据进一步核对。"
const sourceNeutralCoverageEN = "Coverage on some dimensions of the answer may be incomplete; cross-check the relevant evidence."

func TestAnswerCoverageCaveatSourceNeutralPublicExits(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)
	// A surviving coverage obligation, not a suppressed metadata-only warning.
	violations := []types.Violation{{Kind: types.ViolMustInclude}}
	before := append([]types.Violation(nil), violations...)
	body := "正文仍保留 / Original answer remains.\nvalue=17; window=1..14"
	for _, tc := range []struct{ lang, want string }{
		{"", sourceNeutralCoverageZH},
		{"zh", sourceNeutralCoverageZH},
		{"zh-cn", sourceNeutralCoverageZH},
		{"cn", sourceNeutralCoverageZH},
		{"Chinese", sourceNeutralCoverageZH},
		{"en", sourceNeutralCoverageEN},
		{"fr", sourceNeutralCoverageEN}, // Preserve the existing unknown-language fallback.
	} {
		t.Run("lang="+tc.lang, func(t *testing.T) {
			got := MaterializeUnresolvedViolationsAsCaveats(violations, tc.lang)
			if !reflect.DeepEqual(got, []string{tc.want}) {
				t.Errorf("materializer must emit one source-neutral disclosure: %v", got)
			}
			for _, exit := range []struct {
				name string
				run  func(string, []types.Violation, string) string
			}{
				{"user", AppendUserCaveatsToAnswer},
				{"soft", AppendSoftContractCaveatsToAnswer},
			} {
				t.Run(exit.name, func(t *testing.T) {
					assertSourceNeutralCoverageOutput(t, exit.run(body, violations, tc.lang), body, tc.want)
				})
			}
			residual := MaterializeResidualConcernDetails(violations, tc.lang)
			if len(residual) != 2 || residual[1] != tc.want {
				t.Errorf("hard-cap generic residual must reuse the same disclosure: %v", residual)
			}
			if !reflect.DeepEqual(violations, before) {
				t.Fatalf("display must not mutate the surviving obligation: %+v", violations)
			}
		})
	}
}

func TestAnswerCoverageCaveatSourceNeutralScopesAndReplay(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)
	violations := []types.Violation{{Kind: types.ViolMustInclude}}
	for _, scope := range []string{"no_context", "trace_only", "current_source"} {
		for _, tc := range []struct{ lang, want string }{{"zh", sourceNeutralCoverageZH}, {"en", sourceNeutralCoverageEN}} {
			t.Run(scope+"/"+tc.lang, func(t *testing.T) {
				var ctx *types.BusContext
				if scope != "no_context" {
					rm := types.RequestModel{Intent: types.IntentExplain}
					wantLane := types.CurrentSourceLaneRequired
					if scope == "trace_only" {
						rm.ExternalObservationPolicy = &types.ExternalObservationPolicy{
							CurrentSourceMode:    types.ExternalObservationCurrentSourceExclude,
							ExclusionKind:        types.ExternalObservationSourceExclusionExplicitUserBoundary,
							ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
							SourceQuotes:         []string{"只分析附加 trace，不分析代码"},
						}
						wantLane = types.CurrentSourceLaneExcluded
					}
					if got := rm.CurrentSourceLaneDecision(); got != wantLane {
						t.Fatalf("fixture source authority mismatch: got %s, want %s", got, wantLane)
					}
					// Identical raw text across opposite typed scopes: no prose routing.
					ctx = &types.BusContext{Mutable: types.NewMutableState("unchanged request"), Language: tc.lang,
						AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
				}
				body := "原答案 / Original answer: measured value=17."
				assertSourceNeutralCoverageOutput(t, AppendUserCaveatsToAnswerForBus(body, violations, tc.lang, ctx), body, tc.want)
				assertSourceNeutralCoverageOutput(t, AppendSoftContractCaveatsToAnswerForBus(body, violations, tc.lang, ctx), body, tc.want)
				if ctx == nil {
					return
				}
				o := &Orchestrator{busCtx: ctx}
				out := o.appendUserCaveatsTracked(body, violations)
				out = o.appendUserCaveatsTracked(out, violations)
				assertSourceNeutralCoverageOutput(t, out, body, tc.want)
				for i := 0; i < 2; i++ {
					fresh := "重新渲染正文 / Fresh answer: value=17."
					assertSourceNeutralCoverageOutput(t, o.replayRegisteredAnswerCaveats(fresh), fresh, tc.want)
				}
			})
		}
	}
}

func TestAnswerCoverageCaveatSourceNeutralKeepsExistingBoundaries(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		body := "original body"
		if got := AppendUserCaveatsToAnswer(body, nil, lang); got != body {
			t.Fatalf("no violation must leave the body unchanged: %s", got)
		}
		if got := AppendUserCaveatsToAnswer(body, []types.Violation{{Kind: types.ViolDemotionStorm}}, lang); got != body {
			t.Fatalf("operator-only telemetry must remain hidden: %s", got)
		}
		got := MaterializeUnresolvedViolationsAsCaveats([]types.Violation{{Kind: types.ViolFacetUncovered,
			ClusterKey: types.FacetClusterKey(string(types.FacetCurrentCodePath), "answer_facet_coverage")}}, lang)
		want := "当前代码路径"
		if lang == "en" {
			want = "current code path"
		}
		if len(got) != 1 || !strings.Contains(got[0], want) || got[0] == sourceNeutralCoverageZH || got[0] == sourceNeutralCoverageEN {
			t.Fatalf("an exact current-source facet must retain its specific disclosure: %v", got)
		}
	}
}

func assertSourceNeutralCoverageOutput(t *testing.T, got, body, want string) {
	t.Helper()
	if !strings.HasPrefix(got, body+"\n\n") || strings.Count(got, body) != 1 {
		t.Errorf("disclosure must preserve the answer body exactly once: %s", got)
	}
	if strings.Count(got, want) != 1 {
		t.Errorf("expected one shared source-neutral disclosure %q: %s", want, got)
	}
	for _, banned := range []string{"结合源码", "相关组件", "cross-check with source", "affected components", "answer_coverage"} {
		if strings.Contains(got, banned) {
			t.Errorf("generic disclosure must not prescribe a source or leak metadata %q: %s", banned, got)
		}
	}
}
