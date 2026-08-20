package agent

import (
	"fmt"
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

func TestRenderAnswerDocRelationSurfaceHandoffPreservesTextReferenceAuthority(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			PredicateAxis: types.AxisCall,
		}},
		EvidenceItems: []types.EvidenceItem{{
			ID:                 "teaching-1",
			Kind:               types.EvidenceMechanism,
			Source:             "internal/skill/defaults.go",
			LineStart:          1203,
			AnchorKind:         types.AnchorTextReference,
			AnchorSymbol:       "span parsing rule",
			Subject:            "span parsing rule",
			Summary:            "Walk every B/E span pair.",
			LoadBearingSummary: true,
			GroundingStatus:    types.GroundingGrounded,
		}},
	}
	got := renderAnswerDocRelationSurfaceHandoff(ctx)
	for _, want := range []string{
		"internal/skill/defaults.go:1203",
		"source_shape_authority=visible_text_only executable_mechanism=unproven",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("text-reference authority missing %q:\n%s", want, got)
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
		"direction=`member_to_source`",
		`body_json="tr1[\"prodEvaluator\"] -->|implements| tr2[\"LoopController\"]"`,
		`"from_identity":"prodEvaluator"`,
		`"to_identity":"LoopController"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed source-role handoff missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `body_json="tr2[\"LoopController\"] -->|implements| tr1[\"prodEvaluator\"]"`) {
		t.Fatalf("typed relation authoring recipe reversed lookup direction into display direction:\n%s", got)
	}
	if strings.Contains(got, `"from_identity":"stubEvaluator"`) || strings.Contains(got, `"from_identity":"pathlessEvaluator"`) {
		t.Fatalf("auxiliary/unknown rows must not be promoted into principal diagram recipes:\n%s", got)
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

func TestRenderAnswerDocTypedRelationSourceRoleHandoffUsesVerifiedPrincipalMemberBoundary(t *testing.T) {
	mu := types.NewMutableState("show the main LoopController implementations")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:       types.AnswerAggregateMemberSet,
		Role:       types.AnswerAggregateRolePrincipalAnswer,
		Provenance: types.TypedRelationPrincipalMemberSetAggregateProvenance,
		Members:    []string{"prodEvaluator"},
	}})
	mu.SetInvestigationComplete("typed principal set accepted")
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.AgentContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:             types.IntentEnumerate,
			PredicateAxis:      types.AxisImplement,
			SourceScopeProfile: &types.SourceScopeProfile{RequestedScope: types.SourceScopeAll},
			Predicates: types.SemanticPredicates{
				IsRelationalLookup:    true,
				IsCategoryEnumeration: true,
			},
		}},
		TypedRelationHints: []types.TypedRelationHint{{
			Relation:   types.TypedRelationImplements,
			SourceName: "LoopController",
			Members: []types.TypedRelationMember{
				{Name: "prodEvaluator", File: "internal/agent/prod.go"},
				{Name: "testEvaluator", File: "internal/agent/prod_test.go"},
			},
		}},
	}
	got := renderAnswerDocTypedRelationSourceRoleHandoff(ctx)
	for _, want := range []string{
		"exact_principal_member_boundary=`completion_verified_typed_relation_member_set`",
		"member=`prodEvaluator` answer_membership=`principal`",
		"member=`testEvaluator` answer_membership=`support_only`",
		`"from_identity":"prodEvaluator"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("verified principal member boundary missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `"from_identity":"testEvaluator"`) {
		t.Fatalf("broader all-source candidate leaked into principal type-relation recipe:\n%s", got)
	}

	edges := []answerDocMechanismRelationEdge{
		{from: "prodEvaluator", to: "LoopController", relation: types.DiagramRelTypeRelation},
		{from: "testEvaluator", to: "LoopController", relation: types.DiagramRelTypeRelation},
		{from: "Factory", to: "Builder", relation: types.DiagramRelCall},
	}
	bounded := answerDocPrincipalTypedRelationBoundaryEdges(ctx, edges)
	if len(bounded) != 2 || bounded[0].from != "prodEvaluator" || bounded[1].relation != types.DiagramRelCall {
		t.Fatalf("principal boundary must remove only outside type members and retain other relation families: %+v", bounded)
	}
}

func TestRenderAnswerDocTypedRelationDirectionRecipesAreLanguageNeutral(t *testing.T) {
	files := []string{
		"src/GoImpl.go",
		"src/JavaScriptImpl.js",
		"src/TypeScriptImpl.ts",
		"src/JavaImpl.java",
		"src/KotlinImpl.kt",
		"src/CImpl.c",
		"src/CppImpl.cpp",
		"src/RustImpl.rs",
		"src/PythonImpl.py",
		"src/RubyImpl.rb",
		"src/SwiftImpl.swift",
		"src/LuaImpl.lua",
		"src/ProtoImpl.proto",
		"src/ArkImpl.ets",
		"src/CangjieImpl.cj",
	}
	members := make([]types.TypedRelationMember, 0, len(files))
	for i, file := range files {
		members = append(members, types.TypedRelationMember{
			Name: fmt.Sprintf("Impl%d", i+1), File: file, Line: i + 1,
		})
	}
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{}},
		TypedRelationHints: []types.TypedRelationHint{
			{Relation: types.TypedRelationImplements, SourceName: "Contract", Members: members},
			{Relation: types.TypedRelationExtends, SourceName: "Base", Members: []types.TypedRelationMember{{Name: "Child", File: "src/child.ts"}}},
			{Relation: types.TypedRelationOverrides, SourceName: "Base.render", Members: []types.TypedRelationMember{{Name: "Child.render", File: "src/child.kt"}}},
		},
	}
	got := renderAnswerDocTypedRelationSourceRoleHandoff(ctx)
	for i := range files {
		member := fmt.Sprintf("Impl%d", i+1)
		if !strings.Contains(got, `"from_identity":"`+member+`"`) ||
			!strings.Contains(got, `"to_identity":"Contract"`) {
			t.Fatalf("cross-language member %s lost member->contract recipe:\n%s", member, got)
		}
	}
	for _, want := range []string{
		"relation=`extends`", `"from_identity":"Child"`, `"to_identity":"Base"`,
		"relation=`overrides`", `"from_identity":"Child.render"`, `"to_identity":"Base.render"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("declared type recipe missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `"from_identity":"Contract"`) ||
		strings.Contains(got, `"from_identity":"Base"`) ||
		strings.Contains(got, `"from_identity":"Base.render"`) {
		t.Fatalf("contract/supertype must never become the recipe source endpoint:\n%s", got)
	}
}

func TestRenderAnswerDocTypedRelationDirectionRecipesDoNotRelabelOtherRelations(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{}},
		TypedRelationHints: []types.TypedRelationHint{{
			Relation: types.TypedRelationCalledBy, SourceName: "target",
			Members: []types.TypedRelationMember{{Name: "caller", File: "src/caller.go", Line: 10}},
		}},
	}
	got := renderAnswerDocTypedRelationSourceRoleHandoff(ctx)
	if !strings.Contains(got, "relation=`called-by`") {
		t.Fatalf("non-type relation roster row should remain visible:\n%s", got)
	}
	if strings.Contains(got, "type_relation_recipe") || strings.Contains(got, "declared_type_relation_authoring=`available`") {
		t.Fatalf("called-by must not be relabeled as a declared type relation recipe:\n%s", got)
	}
}
