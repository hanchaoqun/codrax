package repomap

import (
	"testing"

	ctypes "github.com/hanchaoqun/codrax/internal/types"
)

func TestRepoMapSourceInventoryRefinementRequiresScopeForBroadBudgetTruncation(t *testing.T) {
	obs := ctypes.SourceInventoryObservation{
		Active:     true,
		Complete:   false,
		Scopes:     []string{"."},
		Provenance: []string{"repo_lens:tool_query", "repo_lens:candidate_budget_truncated"},
		Execution:  &ctypes.SourceInventoryExecutionState{CandidateBudgetTruncated: true},
		SourceClasses: []ctypes.SourceInventorySourceClassCount{
			{Role: ctypes.SourcePathRoleProduction, Count: 10},
			{Role: ctypes.SourcePathRoleThirdParty, Count: 3},
		},
		Sets: []ctypes.SourceInventoryObservationSet{{
			Role:     ctypes.AnswerCandidateRoleFunction,
			Complete: false,
			Count:    1,
			Total:    200,
			Members:  []ctypes.SourceInventoryObservationMember{{Name: "main", File: "cmd/main.go"}},
		}},
	}
	hint := repoMapSourceInventoryRefinement(obs, ctypes.SourceInventoryLensQuery{
		Path:   ".",
		Scopes: []string{"."},
		Roles: []ctypes.AnswerCandidateRole{
			ctypes.AnswerCandidateRoleFunction,
			ctypes.AnswerCandidateRoleType,
		},
		TopN: 24,
	})
	if hint == nil {
		t.Fatal("expected source-inventory refinement")
	}
	if hint.ReasonCode != "source_inventory_candidate_budget_truncated" || !hint.CandidateBudgetTruncated {
		t.Fatalf("unexpected hint: %+v", hint)
	}
	if hint.PreferredNextTool != "repo_map" || hint.PreferredParams["view"] != "source_inventory" {
		t.Fatalf("preferred route mismatch: %+v", hint)
	}
	if _, ok := hint.PreferredParams["scope"]; ok {
		t.Fatalf("broad budget truncation should require a narrower scope, not repeat root scope: %+v", hint.PreferredParams)
	}
	if _, ok := hint.PreferredParams["cursor"]; ok {
		t.Fatalf("broad budget truncation should not prefer cursor paging over narrowing: %+v", hint.PreferredParams)
	}
	if len(hint.RequiredFields) != 1 || hint.RequiredFields[0] != "scope" {
		t.Fatalf("required fields = %+v", hint.RequiredFields)
	}
	if len(hint.TopSourceClasses) != 2 || hint.TopSourceClasses[1] != ctypes.SourcePathRoleThirdParty {
		t.Fatalf("top source classes = %+v", hint.TopSourceClasses)
	}
}

func TestRepoMapSourceInventoryRefinementCarriesCursorForScopedPage(t *testing.T) {
	obs := ctypes.SourceInventoryObservation{
		Active:   true,
		Complete: false,
		Scopes:   []string{"src/runtime"},
		Page: &ctypes.SourceInventoryObservationPage{
			Offset:     0,
			Limit:      24,
			Total:      64,
			Emitted:    24,
			NextCursor: "24",
			Complete:   false,
		},
		Sets: []ctypes.SourceInventoryObservationSet{{
			Role:     ctypes.AnswerCandidateRoleType,
			Complete: false,
			Count:    24,
			Total:    64,
			Members:  []ctypes.SourceInventoryObservationMember{{Name: "Runtime", File: "src/runtime/runtime.ts"}},
		}},
	}
	hint := repoMapSourceInventoryRefinement(obs, ctypes.SourceInventoryLensQuery{
		Path:   ".",
		Scopes: []string{"src/runtime"},
		Roles:  []ctypes.AnswerCandidateRole{ctypes.AnswerCandidateRoleType},
		TopN:   24,
	})
	if hint == nil {
		t.Fatal("expected cursor refinement")
	}
	if hint.ReasonCode != "source_inventory_page_incomplete" || hint.CandidateBudgetTruncated {
		t.Fatalf("unexpected hint: %+v", hint)
	}
	if hint.PreferredParams["scope"] != "src/runtime" || hint.PreferredParams["cursor"] != "24" {
		t.Fatalf("preferred params = %+v", hint.PreferredParams)
	}
	if len(hint.RequiredFields) != 0 {
		t.Fatalf("scoped pagination should not require a new scope: %+v", hint.RequiredFields)
	}
}
