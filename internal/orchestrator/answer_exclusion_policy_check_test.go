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

func TestRunTypedAnswerRoleProfileCheck_RejectsMissingRequiredRole(t *testing.T) {
	rm := &types.RequestModel{
		AnswerRoleProfile: &types.AnswerRoleProfile{
			IsRoleBindingRequested: true,
			RequiredCandidateRoles: []types.AnswerCandidateRole{
				types.AnswerCandidateRoleBudgetCap,
			},
			Confidence: 0.9,
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:          "scalar",
			Kind:        types.BlockScalar,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "The nearby attempt counter is EmitStageRetryAttempt.",
			Items: []types.AnswerBlockItem{{
				ID:            "attempt",
				CandidateRole: types.AnswerCandidateRoleAttemptCounter,
			}},
		}},
	}
	got := runTypedAnswerRoleProfileCheck(doc, rm)
	if len(got) != 1 {
		t.Fatalf("violations = %d, want 1: %+v", len(got), got)
	}
	if got[0].Kind != types.ViolMustInclude ||
		!strings.Contains(got[0].Detail, "budget_cap") {
		t.Fatalf("unexpected violation: %+v", got[0])
	}
	doc.Blocks[0].Items = append(doc.Blocks[0].Items, types.AnswerBlockItem{
		ID:            "cap",
		CandidateRole: types.AnswerCandidateRoleBudgetCap,
	})
	if got := runTypedAnswerRoleProfileCheck(doc, rm); len(got) != 0 {
		t.Fatalf("typed role coverage should pass: %+v", got)
	}
}

func TestRunTypedAnswerRoleProfileCheck_DoesNotReadProse(t *testing.T) {
	rm := &types.RequestModel{
		RawRequest: "what is the retry budget parameter called",
		AnswerRoleProfile: &types.AnswerRoleProfile{
			IsRoleBindingRequested: true,
			RequiredCandidateRoles: []types.AnswerCandidateRole{
				types.AnswerCandidateRoleBudgetCap,
			},
			Confidence: 0.9,
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:          "summary",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "The retry budget parameter is MaxRetriesPerStage.",
		}},
	}
	got := runTypedAnswerRoleProfileCheck(doc, rm)
	if len(got) != 1 {
		t.Fatalf("prose-only role wording must not satisfy typed role contract: %+v", got)
	}
}
