package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRunTypedAnswerExclusionPolicyCheck_RejectsExcludedCandidateRole(t *testing.T) {
	rm := &types.RequestModel{
		AnswerExclusionPolicy: &types.AnswerExclusionPolicy{
			IsExclusionRequested: true,
			ExcludedCandidateRoles: []types.AnswerCandidateRole{
				types.AnswerCandidateRoleVariable,
			},
			Confidence: 0.9,
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:          "exports",
			Kind:        types.BlockOrderedList,
			SurfaceRole: types.SurfacePrincipal,
			Items: []types.AnswerBlockItem{{
				ID:            "var1",
				Label:         "debugEnabled",
				CandidateRole: types.AnswerCandidateRoleVariable,
			}},
		}},
	}
	got := runTypedAnswerExclusionPolicyCheck(doc, rm)
	if len(got) != 1 {
		t.Fatalf("violations = %d, want 1: %+v", len(got), got)
	}
	if got[0].Kind != types.ViolMustExclude ||
		!strings.Contains(got[0].Detail, `candidate_role="variable"`) {
		t.Fatalf("unexpected violation: %+v", got[0])
	}
}

func TestRunTypedAnswerExclusionPolicyCheck_DoesNotReadProse(t *testing.T) {
	rm := &types.RequestModel{
		RawRequest: "不要列变量",
		AnswerExclusionPolicy: &types.AnswerExclusionPolicy{
			IsExclusionRequested: true,
			ExcludedCandidateRoles: []types.AnswerCandidateRole{
				types.AnswerCandidateRoleVariable,
			},
			Confidence: 0.9,
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:          "scope",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "变量 debugEnabled 已排除。",
		}},
	}
	if got := runTypedAnswerExclusionPolicyCheck(doc, rm); len(got) != 0 {
		t.Fatalf("prose-only mentions must not trigger typed exclusion check: %+v", got)
	}
}
