package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestAppendInactiveScopeSystemCaveat_AppendsSupplementalNote(t *testing.T) {
	busCtx := &types.BusContext{
		Language:        "zh",
		PendingSubRepos: []string{"repo-tools-py"},
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentExplain,
				AnalyzerHints: types.AnalyzerHints{
					ExactTargets: []string{"process_request"},
				},
			},
		},
	}
	doc := &types.AnswerDocumentV2{
		ExactResolution: &types.AnswerExactResolution{
			Status: types.AnswerExactResolutionAbsent,
		},
	}
	got := appendInactiveScopeSystemCaveat("正文", doc, busCtx)
	for _, want := range []string{"**补充说明：**", "当前已启用的子仓范围", "`repo-tools-py`"} {
		if !strings.Contains(got, want) {
			t.Fatalf("system caveat missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "scope_disclosure") || strings.Contains(got, "multi-repo workspace") {
		t.Fatalf("system caveat should not leak schema/internal wording:\n%s", got)
	}
}

func TestAppendInactiveScopeSystemCaveat_ExistingDisclosureNoops(t *testing.T) {
	busCtx := &types.BusContext{
		Language:        "zh",
		PendingSubRepos: []string{"repo-tools-py"},
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentExplain,
				AnalyzerHints: types.AnalyzerHints{
					ExactTargets: []string{"process_request"},
				},
			},
		},
	}
	doc := &types.AnswerDocumentV2{
		ExactResolution: &types.AnswerExactResolution{
			Status: types.AnswerExactResolutionAbsent,
		},
		Blocks: []types.AnswerBlock{{
			ID:              "c1",
			Kind:            types.BlockCaveat,
			Text:            "需要调整仓库关注范围。",
			ScopeDisclosure: types.ScopeDisclosureRequiresWorkspaceAdjust,
		}},
	}
	got := appendInactiveScopeSystemCaveat("正文", doc, busCtx)
	if got != "正文" {
		t.Fatalf("existing typed disclosure should suppress system caveat, got:\n%s", got)
	}
}

func TestInactiveScopeDisclosureSoftByDefault(t *testing.T) {
	if !isSoftViolationKind(types.ViolInactiveScopeDisclosureMissing) {
		t.Fatal("inactive-scope disclosure should be system-caveat telemetry, not a default finalizer retry")
	}
}
