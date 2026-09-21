package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1790RendererTargetAuthorityCannotFallBackToGenericEntities(t *testing.T) {
	for _, tc := range []struct {
		name       string
		profile    *types.RuntimeTargetProfile
		targets    []types.RuntimeTarget
		wantAnchor bool
		wantAlias  string
	}{
		{"legacy_nil", nil, nil, false, ""},
		{"no_named", &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNoNamedTarget}, nil, true, ""},
		{"unspecified", &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationUnspecified}, nil, true, ""},
		{"empty", &types.RuntimeTargetProfile{}, nil, true, ""},
		{"named_without_targets", &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: "named"}, nil, true, ""},
		{"named_elsewhere", &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: "named"}, []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 900, Source: "user_explicit"}}, true, ""},
		{"named_target", &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: "named"}, []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 200, Source: "user_explicit"}}, false, ""},
		{"named_alias", &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: "named"}, []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, Thread: "user-worker-200", Source: "user_explicit"}}, false, "user-worker-200"},
		{"named_diagnostic_full", &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: "named"}, []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, Thread: "worker-200 [200]", Source: "user_explicit"}}, false, ""},
		{"named_diagnostic_name", &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: "named"}, []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, Thread: "worker [200]", Source: "user_explicit"}}, false, ""},
		{"named_diagnostic_parens", &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: "named"}, []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, Thread: "worker-200 (200)", Source: "user_explicit"}}, false, ""},
		{"named_canonical_comm", &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: "named"}, []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, Thread: "WORKER", Source: "user_explicit"}}, false, ""},
		{"named_diagnostic_contradiction", &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: "named"}, []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, Thread: "worker-999 [200]", Source: "user_explicit"}}, true, ""},
		{"named_different_tid", &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: "named"}, []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, Thread: "worker-999", Source: "user_explicit"}}, true, ""},
		{"named_cursor", &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: "named"}, []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 200, Source: types.RuntimeTargetSourceExplicitToolCall}}, true, ""},
	} {
		for _, zh := range []bool{true, false} {
			t.Run(tc.name+map[bool]string{true: "/zh", false: "/en"}[zh], func(t *testing.T) {
				ctx := &types.BusContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
					RuntimeTargetProfile: tc.profile, RuntimeTargets: tc.targets,
				}}}
				ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{"worker", "1.000", "1.100"}
				ctx.AnalysisIR.RequestModel.AnalyzerHints.ExactTargets = []string{"worker-200"}
				focus := runtimeTraceProjUserFocusFromBusContext(ctx)
				// An old elected flag cannot override the current profile's
				// authorized target list. This is a renderer defensive fixture.
				model := runtimeTraceProjTreeModel{Target: "worker-200", TargetUserElected: true}
				runtimeTraceProjApplyUserFocus(&model, focus)
				if model.RootFocusAnchorOnly != tc.wantAnchor || model.TargetUserAliasEntity != tc.wantAlias {
					t.Fatalf("wrong authority/alias: anchor=%v alias=%q", model.RootFocusAnchorOnly, model.TargetUserAliasEntity)
				}
				header := runtimeTraceProjTreeHeaderLabel(model, zh)
				anchorWord, focusWord := "分析锚点线程", "用户关注线程"
				if !zh {
					anchorWord, focusWord = "analysis anchor thread", "user-focused thread"
				}
				if tc.wantAnchor && (!strings.Contains(header, anchorWord) || strings.Contains(header, focusWord)) {
					t.Fatalf("untrusted hint acquired user label: %s", header)
				}
				if model.UserWindowStart != 1 || model.UserWindowEnd != 1.1 {
					t.Fatalf("identity policy changed the separate legacy time-window face: %g..%g", model.UserWindowStart, model.UserWindowEnd)
				}
				for _, entity := range model.RootFocusUserEntities {
					if tc.profile != nil && entity == "worker-200" {
						t.Fatal("generic exact target was presented as a user target")
					}
				}
			})
		}
	}
}

func TestB1790EmptyRendererFocusDistinguishesPresentProfileFromLegacy(t *testing.T) {
	for _, present := range []bool{false, true} {
		ctx := &types.BusContext{AnalysisIR: &types.AnalysisIR{}}
		if present {
			ctx.AnalysisIR.RequestModel.RuntimeTargetProfile = &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNoNamedTarget}
		}
		model := runtimeTraceProjTreeModel{Target: "worker-200"}
		runtimeTraceProjApplyUserFocus(&model, runtimeTraceProjUserFocusFromBusContext(ctx))
		if model.RootFocusAnchorOnly != present || len(model.RootFocusUserEntities) != 0 || model.FlatAnchorMismatch {
			t.Fatalf("empty focus authority: present=%v anchor=%v roster=%v flat=%v", present, model.RootFocusAnchorOnly, model.RootFocusUserEntities, model.FlatAnchorMismatch)
		}
	}
}
