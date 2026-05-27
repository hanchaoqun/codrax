package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRenderAnswerDocRelationSurfaceHandoffRequiresStructuredRows(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest:    "Which worker calls the helper?",
			PredicateAxis: types.AxisCall,
		}},
	}
	if got := renderAnswerDocRelationSurfaceHandoff(ctx); got != "" {
		t.Fatalf("raw request / typed axis without structured relation rows must not render handoff, got:\n%s", got)
	}

	ctx.EvidenceItems = []types.EvidenceItem{{
		ID:              "rel-1",
		Kind:            types.EvidenceRelationship,
		Source:          "internal/worker.go",
		LineStart:       42,
		LineEnd:         42,
		AnchorKind:      types.AnchorCall,
		AnchorSymbol:    "Run",
		Subject:         "Worker",
		Object:          "Helper",
		Summary:         "Worker calls Helper through Run.",
		GroundingStatus: types.GroundingGrounded,
		Producer:        "test",
	}}
	got := renderAnswerDocRelationSurfaceHandoff(ctx)
	for _, want := range []string{
		"Relation Role Handoff (Advisory)",
		"structured exploration carriers",
		"not a system-authored final answer set",
		"Worker -> Helper",
		"internal/worker.go:42",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("relation handoff missing %q:\n%s", want, got)
		}
	}
}

func TestRenderAnswerDocRelationSurfaceHandoffPrincipalMemberSetWins(t *testing.T) {
	mu := types.NewMutableState("Which caller reaches Helper?")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "Helper callers",
		Members:     []string{"Worker -> Helper"},
		SupportRefs: []string{"Worker: internal/worker.go:42"},
		Role:        types.AnswerAggregateRolePrincipalAnswer,
	}})
	mu.SetInvestigationComplete("relation member set accepted")
	ctx := &types.AgentContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsRelationalLookup:    true,
				IsCategoryEnumeration: true,
			},
			PredicateAxis: types.AxisCall,
		}},
		EvidenceItems: []types.EvidenceItem{{
			ID:              "rel-1",
			Kind:            types.EvidenceRelationship,
			Source:          "internal/worker.go",
			LineStart:       42,
			LineEnd:         42,
			AnchorKind:      types.AnchorCall,
			AnchorSymbol:    "Run",
			Subject:         "Worker",
			Object:          "Helper",
			Summary:         "Worker calls Helper through Run.",
			GroundingStatus: types.GroundingGrounded,
			Producer:        "test",
		}},
	}
	got := renderAnswerDocRelationSurfaceHandoff(ctx)
	if !strings.Contains(got, "A principal relation `member_set` is already present above") {
		t.Fatalf("principal relation member_set precedence not surfaced:\n%s", got)
	}
	if !strings.Contains(got, "use these rows only for per-member explanation") {
		t.Fatalf("advisory boundary missing:\n%s", got)
	}
}

func TestRenderAnswerDocRelationSurfaceHandoffIncludesStructuredBoundaryRows(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			PredicateAxis: types.AxisCall,
		}},
		EvidenceItems: []types.EvidenceItem{{
			ID:              "boundary-1",
			Kind:            types.EvidenceMechanism,
			Source:          "internal/orchestrator.go",
			LineStart:       12,
			LineEnd:         12,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "Orchestrator",
			Subject:         "Orchestrator",
			Summary:         "Orchestrator is the scheduler boundary for this relation.",
			GroundingStatus: types.GroundingGrounded,
			Producer:        "test",
		}},
	}
	got := renderAnswerDocRelationSurfaceHandoff(ctx)
	for _, want := range []string{
		"definition_or_boundary",
		"Orchestrator",
		"internal/orchestrator.go:12",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("boundary relation handoff missing %q:\n%s", want, got)
		}
	}
}

func TestRequiresRelationMemberSetHandoffStillSkipsMechanismOnlyRelation(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentExplain,
		Predicates: types.SemanticPredicates{
			IsRelationalLookup: true,
		},
	}
	if types.RequiresRelationMemberSetHandoff(rm) {
		t.Fatal("mechanism-only relation explanation must remain advisory, not a hard relation member-set gate")
	}
}
