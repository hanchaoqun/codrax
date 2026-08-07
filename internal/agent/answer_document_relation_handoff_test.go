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
		Provenance:  types.TypedRelationPrincipalMemberSetAggregateProvenance,
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

func TestRenderAnswerDocRelationSurfaceHandoffSoftCallChainSetRemainsAdvisory(t *testing.T) {
	mu := types.NewMutableState("How does Logger reach the console sink?")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind: types.AnswerAggregateMemberSet, Label: "complete call chain",
		Members: []string{"Logger.log", "Sink.write", "ConsoleSink.write", "std.fputs"},
		SupportRefs: []string{
			"Logger.log @ src/logger.cpp:36",
			"Sink.write @ include/logx/sink.hpp:8",
			"ConsoleSink.write @ include/logx/console_sink.hpp:10",
			"std.fputs @ include/logx/console_sink.hpp:11",
		},
		Role: types.AnswerAggregateRolePrincipalAnswer,
	}})
	mu.SetInvestigationComplete("model proposed a path")
	ctx := &types.AgentContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			PredicateAxis: types.AxisCall,
			AnalyzerHints: types.AnalyzerHints{Kind: "call_chain"},
		}},
		EvidenceItems: []types.EvidenceItem{{
			ID: "call", Kind: types.EvidenceRelationship,
			Source: "src/logger.cpp", LineStart: 36, AnchorKind: types.AnchorCall,
			Subject: "Logger.log", Object: "Sink.write", Predicate: "calls",
			GroundingStatus: types.GroundingGrounded,
		}},
	}
	got := renderAnswerDocRelationSurfaceHandoff(ctx)
	for _, forbidden := range []string{
		"That required member set is the answer-member carrier",
		"A principal relation `member_set` is already present above",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("soft call-chain set was upgraded by relation handoff via %q:\n%s", forbidden, got)
		}
	}
	for _, want := range []string{
		"No evidence-authorized principal relation `member_set` is active",
		"keep it advisory",
		"preserve component boundaries",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("soft call-chain boundary missing %q:\n%s", want, got)
		}
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

func TestRenderAnswerDocRelationSurfaceHandoffPrioritizesStructuredGateRows(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			PredicateAxis: types.AxisCall,
		}},
		EvidenceItems: []types.EvidenceItem{{
			ID:              "gate-1",
			Kind:            types.EvidenceDataflowPath,
			Source:          "internal/consumer.go",
			LineStart:       24,
			LineEnd:         24,
			Subject:         "`Consumer.Build()` gates on Registry.Get(...) — registry populated by `RegisterDefaults()` binding NewWorker → `Worker.Name()` returns \"worker\"",
			Summary:         "`Consumer.Build()` gates on Registry.Get(...) — registry populated by `RegisterDefaults()` binding NewWorker → `Worker.Name()` returns \"worker\"",
			GroundingStatus: types.GroundingGrounded,
			Producer:        "consumer_gate",
		}, {
			ID:              "reg-1",
			Kind:            types.EvidenceRegistration,
			Source:          "internal/registry.go",
			LineStart:       17,
			LineEnd:         17,
			Subject:         "RegisterDefaults",
			Object:          "NewWorker",
			Summary:         "RegisterDefaults binds NewWorker.",
			GroundingStatus: types.GroundingGrounded,
			Producer:        "test",
		}, {
			ID:              "runtime-1",
			Kind:            types.EvidenceRelationship,
			Source:          "internal/runtime.go",
			LineStart:       41,
			LineEnd:         41,
			AnchorKind:      types.AnchorCall,
			Subject:         "Runtime",
			Object:          "Worker.Run",
			Summary:         "Runtime dispatches Worker.Run after a proposal is accepted.",
			GroundingStatus: types.GroundingGrounded,
			Producer:        "test",
		}},
	}
	got := renderAnswerDocRelationSurfaceHandoff(ctx)
	for _, want := range []string{
		"Role priority",
		"role=`qualifying_gate`",
		"role=`registration_or_binding`",
		"role=`call_or_invocation`",
		"global registry, tool catalog, dispatcher, or runtime row",
		"must not replace qualifying members",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("gate-priority relation handoff missing %q:\n%s", want, got)
		}
	}
	gateIdx := strings.Index(got, "role=`qualifying_gate`")
	regIdx := strings.Index(got, "role=`registration_or_binding`")
	if gateIdx < 0 || regIdx < 0 || gateIdx > regIdx {
		t.Fatalf("qualifying gate row should sort before registration support row:\n%s", got)
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

func TestRenderAnswerDocTypedRelationSourceRoleHandoffPartitionsCompleteRoster(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{}},
		TypedRelationHints: []types.TypedRelationHint{{
			Relation:   types.TypedRelationImplements,
			SourceName: "LoopController",
			Members: []types.TypedRelationMember{
				{Name: "prodEvaluator", File: "internal/agent/prod.go", Line: 10},
				{Name: "stubEvaluator", File: "internal/agent/prod_test.go", Line: 20},
				{Name: "pathlessEvaluator"},
			},
		}},
	}
	got := renderAnswerDocTypedRelationSourceRoleHandoff(ctx)
	for _, want := range []string{
		"Typed Relation Source-Role Projection",
		"complete_relation_roster=3; principal=1; auxiliary=1; unknown=1",
		"lane=`principal` source_role=`production`",
		"lane=`auxiliary` source_role=`test`",
		"lane=`unknown` source_role=`unknown`",
		"does not scan the raw request or answer prose",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed source-role handoff missing %q:\n%s", want, got)
		}
	}
}

func TestRenderAnswerDocTypedRelationSourceRoleHandoffHonorsTypedTestScope(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			SourceScopeProfile: &types.SourceScopeProfile{RequestedScope: types.SourceScopeTest},
		}},
		TypedRelationHints: []types.TypedRelationHint{{
			Relation:   types.TypedRelationImplements,
			SourceName: "LoopController",
			Members: []types.TypedRelationMember{
				{Name: "prodEvaluator", File: "internal/agent/prod.go"},
				{Name: "stubEvaluator", File: "internal/agent/prod_test.go"},
			},
		}},
	}
	got := renderAnswerDocTypedRelationSourceRoleHandoff(ctx)
	if !strings.Contains(got, "lane=`principal` source_role=`test`") ||
		!strings.Contains(got, "lane=`auxiliary` source_role=`production`") {
		t.Fatalf("typed test scope not honored:\n%s", got)
	}
}
