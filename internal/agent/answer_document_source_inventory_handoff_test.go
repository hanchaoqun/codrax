package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRenderAnswerDocSourceInventoryHandoffUsesPrincipalRowSet(t *testing.T) {
	mu := types.NewMutableState("list Cangjie declarations")
	mu.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Active:     true,
		Complete:   true,
		Scopes:     []string{"."},
		Provenance: []string{"repo_lens:tool_query"},
		SourceClasses: []types.SourceInventorySourceClassCount{{
			Role:  types.SourcePathRoleProduction,
			Count: 4,
		}, {
			Role:  types.SourcePathRoleThirdParty,
			Count: 1,
		}},
		Sets: []types.SourceInventoryObservationSet{{
			Role: types.AnswerCandidateRoleType,
			Members: []types.SourceInventoryObservationMember{
				{Name: "ProdA", Role: types.AnswerCandidateRoleType, File: "src/a.cj", Line: 1, Language: "cangjie", CoverageState: types.SourceInventoryCoverageObserved},
				{Name: "ProdB", Role: types.AnswerCandidateRoleType, File: "src/b.cj", Line: 1, Language: "cangjie", CoverageState: types.SourceInventoryCoverageObserved},
				{Name: "ProdC", Role: types.AnswerCandidateRoleType, File: "src/c.cj", Line: 1, Language: "cangjie", CoverageState: types.SourceInventoryCoverageObserved},
				{Name: "CorpusA", Role: types.AnswerCandidateRoleType, File: "internal/thirdparty/tree-sitter-cangjie/corpus/sources/a.cj", Line: 3, Language: "cangjie", CoverageState: types.SourceInventoryCoverageObserved},
			},
		}, {
			Role: types.AnswerCandidateRoleFunction,
			Members: []types.SourceInventoryObservationMember{{
				Name: "supportParser", Role: types.AnswerCandidateRoleFunction, File: "internal/tool/repomap/index/extract_cangjie.go", Line: 50, Language: "go", CoverageState: types.SourceInventoryCoverageObserved,
			}},
		}},
	})
	ctx := &types.AgentContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			SourceInventoryProfile: &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleType},
			},
		}},
	}

	out := renderAnswerDocSourceInventoryHandoff(ctx)
	for _, want := range []string{
		"Principal candidate rows",
		"principal_roles: `type`",
		"source_classes: production:4, thirdparty:1",
		"member=`ProdA`",
		"member=`CorpusA`",
		"source_class=thirdparty",
		"Audit-only rows",
		"member=`supportParser`",
		"lane=audit reason=non_principal_role",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("source-inventory handoff missing %q:\n%s", want, out)
		}
	}
	if strings.Index(out, "member=`CorpusA`") > strings.Index(out, "member=`ProdB`") {
		t.Fatalf("rare thirdparty family should be rendered before repeated production rows:\n%s", out)
	}
	if strings.Index(out, "Audit-only rows") < strings.Index(out, "member=`CorpusA`") {
		t.Fatalf("audit rows must not appear before principal rows:\n%s", out)
	}
}

func TestRenderAnswerDocSourceInventoryHandoffSplitsSupportScope(t *testing.T) {
	mu := types.NewMutableState("list production handlers")
	mu.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Active:   true,
		Complete: true,
		Scopes:   []string{"."},
		Sets: []types.SourceInventoryObservationSet{{
			Role: types.AnswerCandidateRoleFunction,
			Members: []types.SourceInventoryObservationMember{
				{Name: "Serve", Role: types.AnswerCandidateRoleFunction, File: "src/server.go", Line: 10, Language: "go", CoverageState: types.SourceInventoryCoverageObserved},
				{Name: "ServeFixture", Role: types.AnswerCandidateRoleFunction, File: "fixtures/server_fixture.go", Line: 11, Language: "go", CoverageState: types.SourceInventoryCoverageObserved},
			},
		}},
	})
	ctx := &types.AgentContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			SourceInventoryProfile: &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
			},
			SourceScopeProfile: &types.SourceScopeProfile{RequestedScope: types.SourceScopeProduction},
		}},
	}

	out := renderAnswerDocSourceInventoryHandoff(ctx)
	for _, want := range []string{
		"Principal candidate rows",
		"member=`Serve`",
		"Support/navigation rows",
		"member=`ServeFixture`",
		"lane=support reason=support_scope",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("source-inventory handoff missing %q:\n%s", want, out)
		}
	}
	if strings.Index(out, "member=`ServeFixture`") < strings.Index(out, "Support/navigation rows") {
		t.Fatalf("support row leaked into principal section:\n%s", out)
	}
}
