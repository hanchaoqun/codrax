package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestCountTier1Evidence covers the tier1/total classification
// independently of the orchestrator wiring.
func TestCountTier1Evidence(t *testing.T) {
	items := []types.EvidenceItem{
		{GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText},
		{GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierSymbolTable},
		{GroundingStatus: types.GroundingRecovered, GroundingTier: types.TierFQNameSameFile},
		{GroundingStatus: types.GroundingUngrounded},
		{}, // legacy empty-status → counts as Tier-1
	}
	tier1, total := countTier1Evidence(items, nil, types.ScenarioGeneric, false, nil, types.RequestModel{})
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	// Tier-1 (line_text): 1
	// Tier-2 grounded (symbol_table): 0 (NOT tier 1)
	// Recovered / Ungrounded: 0
	// Legacy empty: 1
	if tier1 != 2 {
		t.Errorf("tier1 = %d, want 2", tier1)
	}
}

func TestCountTier1EvidenceIgnoresSalience(t *testing.T) {
	items := []types.EvidenceItem{
		{GroundingStatus: types.GroundingRecovered, Salience: types.SalienceLoadBearing},
		{GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText, Salience: types.SalienceContext},
	}
	tier1, total := countTier1Evidence(items, nil, types.ScenarioGeneric, false, nil, types.RequestModel{})
	if tier1 != 1 || total != 2 {
		t.Fatalf("tier1/total = %d/%d, want 1/2; salience must not affect grounding floor", tier1, total)
	}
}

// TestCountTier1Evidence_AllRecovered — pure-recovery investigation
// (the log 172408 scenario): zero Tier-1 against total evidence.
func TestCountTier1Evidence_AllRecovered(t *testing.T) {
	items := []types.EvidenceItem{
		{GroundingStatus: types.GroundingRecovered},
		{GroundingStatus: types.GroundingRecovered},
		{GroundingStatus: types.GroundingRecovered},
		{GroundingStatus: types.GroundingRecovered},
	}
	tier1, total := countTier1Evidence(items, nil, types.ScenarioGeneric, false, nil, types.RequestModel{})
	if tier1 != 0 {
		t.Errorf("tier1 = %d, want 0", tier1)
	}
	if total != 4 {
		t.Errorf("total = %d, want 4", total)
	}
}

func TestCountTier1Evidence_SkipsAuxiliaryExactResolutionContext(t *testing.T) {
	items := []types.EvidenceItem{
		{
			GroundingStatus: types.GroundingUngrounded,
			ContextRole:     types.EvidenceContextRoleIllustrativeOnly,
			Kind:            types.EvidenceUnresolved,
		},
		{
			GroundingStatus: types.GroundingUngrounded,
			ContextRole:     types.EvidenceContextRoleAbsenceSupport,
			Kind:            types.EvidenceDirect,
		},
		{
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			Kind:            types.EvidenceDirect,
		},
	}
	tier1, total := countTier1Evidence(items, nil, types.ScenarioGeneric, false, nil, types.RequestModel{})
	if total != 1 {
		t.Fatalf("auxiliary illustrative/absence-support items should be excluded from tier1 denominator, got total=%d", total)
	}
	if tier1 != 1 {
		t.Fatalf("remaining grounded related-context item should count, got tier1=%d", tier1)
	}
}

func TestCountTier1Evidence_ExactAbsenceSkipsNonClosureRelatedContext(t *testing.T) {
	contract := &types.ExactResolutionContract{
		AllowAbsence:         true,
		TargetKind:           types.SubjectConfigKey,
		Targets:              []string{"explore_mid_loop_hint_budget"},
		RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
	}
	items := []types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       848,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleDefault,
			GroundingStatus: types.GroundingRecovered,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/config/runtime.go",
			LineStart:       296,
			AnchorKind:      types.AnchorAssignment,
			AnchorSymbol:    "ExploreMidLoopMinIteration",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	}
	tier1, total := countTier1Evidence(items, contract, types.ScenarioConfigTrace, true, []string{"internal/types/config.go"}, types.RequestModel{})
	if total != 0 || tier1 != 0 {
		t.Fatalf("non-closure related_context items should not count during exact-absence closure, got tier1=%d total=%d", tier1, total)
	}
}

func TestCountGroundingHealth(t *testing.T) {
	items := []types.EvidenceItem{
		{Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText},
		{Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierSymbolTable},
		{Kind: types.EvidenceDirect, GroundingStatus: types.GroundingRecovered},
		{Kind: types.EvidenceDirect, GroundingStatus: types.GroundingUngrounded},
		{Kind: types.EvidenceDirect},
	}
	health := countGroundingHealth(items, nil, types.ScenarioGeneric, false, nil, types.RequestModel{})
	if health.total != 5 {
		t.Fatalf("total = %d, want 5", health.total)
	}
	if health.accepted != 4 {
		t.Fatalf("accepted = %d, want 4", health.accepted)
	}
	if health.tier1 != 2 {
		t.Fatalf("tier1 = %d, want 2", health.tier1)
	}
	if health.recovered != 1 {
		t.Fatalf("recovered = %d, want 1", health.recovered)
	}
}

func TestWarnLowGroundingIfNeededEmitsAdvisoryNotice(t *testing.T) {
	prev := tool.CurrentGroundingPolicy()
	tool.SetGroundingPolicy(tool.DefaultGroundingPolicy())
	t.Cleanup(func() { tool.SetGroundingPolicy(prev) })

	mu := types.NewMutableState("explain weak evidence")
	mu.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Source:          "a.go",
		LineStart:       1,
		GroundingStatus: types.GroundingUngrounded,
	}})
	var events []render.Event
	o := &Orchestrator{
		busCtx: &types.BusContext{
			Mutable:  mu,
			Language: "en",
		},
		emit: func(ev render.Event) {
			events = append(events, ev)
		},
	}
	warned := false
	o.warnLowGroundingIfNeeded(&types.AnalysisIR{
		RequestModel: types.RequestModel{Scenario: types.ScenarioGeneric},
	}, &warned)

	if !warned {
		t.Fatal("warning flag was not set")
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if events[0].Kind != render.EventOrchestratorNotice || events[0].NoticeKind != render.NoticeLowGrounding {
		t.Fatalf("unexpected event: %+v", events[0])
	}
	if !strings.Contains(events[0].Reasoning, "Evidence grounding is weak") {
		t.Fatalf("warning message missing user-facing grounding cue: %q", events[0].Reasoning)
	}

	o.warnLowGroundingIfNeeded(&types.AnalysisIR{
		RequestModel: types.RequestModel{Scenario: types.ScenarioGeneric},
	}, &warned)
	if len(events) != 1 {
		t.Fatalf("warning should emit once, events=%d", len(events))
	}
}

// TestCheckTier1Floor_RejectMessageR6 — the reject message flows
// through pendingViolation back into the next dispatch's prompt as
// LLM-facing context. R6 forbids internal pipeline stage names from
// LLM-facing strings; the message used to read "explorer must call
// read_file ..." (2026-05-10 audit). Pin the reworded form so a
// future edit cannot regress.
func TestCheckTier1Floor_RejectMessageR6(t *testing.T) {
	prev := tool.CurrentGroundingPolicy()
	tool.SetGroundingPolicy(tool.StrictGroundingPolicy())
	t.Cleanup(func() { tool.SetGroundingPolicy(prev) })

	mu := types.NewMutableState("any q")
	mu.AppendEvidence([]types.EvidenceItem{{
		Kind: types.EvidenceDirect, Source: "a.go", LineStart: 1,
		GroundingStatus: types.GroundingRecovered,
	}})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu}}
	ir := &types.AnalysisIR{RequestModel: types.RequestModel{Scenario: types.ScenarioGeneric}}
	state := &graphState{}
	msg, proceed, _ := o.checkTier1Floor(ir, state)
	if proceed {
		t.Skip("strict policy did not reject this evidence shape; nothing to audit")
	}
	if msg == "" {
		t.Fatal("reject message empty when proceed=false")
	}
	for _, banned := range []string{
		"explorer must",
		"extractor must",
		"finalizer must",
		"analyzer must",
		"line_text",
		"grounded-via",
		"answer-render",
		"BusContext",
		"MutableState",
		"AnalysisIR",
	} {
		if strings.Contains(msg, banned) {
			t.Errorf("R6 leak: reject message contains internal term %q: %q", banned, msg)
		}
	}
	if !strings.Contains(msg, "read_file") {
		t.Errorf("reject message must still tell the model to call read_file: %q", msg)
	}
}

func TestCheckTier1Floor_ReadLocalizerFollowupRequeuesMissingCoverage(t *testing.T) {
	prev := tool.CurrentGroundingPolicy()
	tool.SetGroundingPolicy(tool.DefaultGroundingPolicy())
	t.Cleanup(func() { tool.SetGroundingPolicy(prev) })

	mu := types.NewMutableState("where is the request dispatched")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles: []string{"pkg/handler.py"},
	})
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentTrace},
		},
	}}
	ir := o.busCtx.AnalysisIR
	state := newGraphState(types.TaskGraph{
		ExecutionPolicy: types.ExecutionPolicy{RetryBudget: 1},
	})

	msg, proceed, exhausted := o.checkTier1Floor(ir, state)
	if proceed || exhausted {
		t.Fatalf("expected non-exhausted localizer retry, proceed=%v exhausted=%v msg=%q", proceed, exhausted, msg)
	}
	for _, want := range []string{"repo_map", "read_file", "pkg/handler.py"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("localizer retry message missing %q:\n%s", want, msg)
		}
	}
}

func TestCheckTier1Floor_ReadLocalizerFollowupDemotesNavigationAfterOwnerEvidence(t *testing.T) {
	prev := tool.CurrentGroundingPolicy()
	tool.SetGroundingPolicy(tool.DefaultGroundingPolicy())
	t.Cleanup(func() { tool.SetGroundingPolicy(prev) })

	mu := types.NewMutableState("why did the runtime observation happen in current code")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles: []string{"pkg/handler.py", "pkg/nearby.py"},
		EvidenceItems: []types.EvidenceItem{{
			ID:              "ev-owner",
			Source:          "pkg/handler.py",
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			LineStart:       42,
			Subject:         "Handler.Dispatch",
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
			OwnerSymbol:     "Handler.Dispatch",
		}},
	})
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentTrace},
		},
	}}
	ir := o.busCtx.AnalysisIR
	state := newGraphState(types.TaskGraph{
		ExecutionPolicy: types.ExecutionPolicy{RetryBudget: 1},
	})

	msg, proceed, exhausted := o.checkTier1Floor(ir, state)
	if !proceed || exhausted || msg != "" {
		t.Fatalf("navigation-only localizer debt should be advisory after owner evidence, proceed=%v exhausted=%v msg=%q", proceed, exhausted, msg)
	}
}

func TestCheckTier1Floor_ReadLocalizerFollowupDemotesSupportAfterPrincipalMemberSetCoverage(t *testing.T) {
	prev := tool.CurrentGroundingPolicy()
	tool.SetGroundingPolicy(tool.DefaultGroundingPolicy())
	t.Cleanup(func() { tool.SetGroundingPolicy(prev) })

	mu := types.NewMutableState("list source inventory members")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "@Entry members",
		Value: "2",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"Index @ internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets:7",
			"ParentComponent @ internal/thirdparty/tree-sitter-arkts/corpus/sources/03_state_management.ets:34",
		},
	}})
	mu.SetInvestigationComplete("principal member set accepted")
	mu.RetainInvestigationAggregateFacts()
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles: []string{
			"internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
			"internal/thirdparty/tree-sitter-arkts/corpus/sources/03_state_management.ets",
			"internal/tool/repomap/index/extract_arkts.go",
		},
	})
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
					HasPerMemberTable:     true,
				},
				SourceInventoryProfile: &types.SourceInventoryProfile{
					IsSourceInventory: true,
					TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleType},
					Confidence:        0.9,
				},
			},
		},
	}}
	state := newGraphState(types.TaskGraph{
		ExecutionPolicy: types.ExecutionPolicy{RetryBudget: 1},
	})

	msg, proceed, exhausted := o.checkTier1Floor(o.busCtx.AnalysisIR, state)
	if !proceed || exhausted || msg != "" {
		t.Fatalf("read-backed principal member set should suppress support/navigation localizer debt, proceed=%v exhausted=%v msg=%q", proceed, exhausted, msg)
	}
}

func TestCheckTier1Floor_ReadLocalizerFollowupDemotesNavigationForRelativePrincipalMemberPaths(t *testing.T) {
	prev := tool.CurrentGroundingPolicy()
	tool.SetGroundingPolicy(tool.DefaultGroundingPolicy())
	t.Cleanup(func() { tool.SetGroundingPolicy(prev) })

	mu := types.NewMutableState("list source inventory members")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "extend blocks",
		Value: "2",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"extend String @ 04_extend_operator.cj:6 (package demo.stringext)",
			"highlight @ 04_styles_extend.ets:11",
		},
	}, {
		Kind:  types.AnswerAggregateMemberSet,
		Label: "foreign func",
		Value: "1",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"native_add @ 07_foreign_ffi.cj:6 (package demo.ffi)",
		},
	}})
	mu.SetInvestigationComplete("principal source-inventory member set accepted")
	mu.RetainInvestigationAggregateFacts()
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles: []string{
			"internal/thirdparty/tree-sitter-cangjie/corpus/sources/04_extend_operator.cj",
			"internal/thirdparty/tree-sitter-arkts/corpus/sources/04_styles_extend.ets",
			"internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj",
		},
	})
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
					HasPerMemberTable:     true,
				},
				SourceInventoryProfile: &types.SourceInventoryProfile{
					IsSourceInventory: true,
					TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction, types.AnswerCandidateRoleType},
					Confidence:        0.9,
				},
			},
		},
	}}
	state := newGraphState(types.TaskGraph{
		ExecutionPolicy: types.ExecutionPolicy{RetryBudget: 1},
	})

	msg, proceed, exhausted := o.checkTier1Floor(o.busCtx.AnalysisIR, state)
	if !proceed || exhausted || msg != "" {
		t.Fatalf("scope-relative principal member anchors should suppress navigation-only localizer debt, proceed=%v exhausted=%v msg=%q", proceed, exhausted, msg)
	}
}

func TestCheckTier1Floor_ReadLocalizerFollowupDemotesCaseDriftForReadPrincipalMemberPaths(t *testing.T) {
	prev := tool.CurrentGroundingPolicy()
	tool.SetGroundingPolicy(tool.DefaultGroundingPolicy())
	t.Cleanup(func() { tool.SetGroundingPolicy(prev) })

	mu := types.NewMutableState("list cangjie source inventory members")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "extend blocks",
		Value: "1",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"Cart @ cart/cart.cj:30 (package demo.cart)",
		},
	}, {
		Kind:  types.AnswerAggregateMemberSet,
		Label: "public classes",
		Value: "1",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"Bridge @ bridge/bridge.cj:15 (package demo.bridge)",
		},
	}})
	mu.SetInvestigationComplete("principal source-inventory member set accepted")
	mu.RetainInvestigationAggregateFacts()
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles: []string{
			"cart/Cart.cj",
			"bridge/Bridge.cj",
		},
		EvidenceItems: []types.EvidenceItem{{
			Source:          "cart/Cart.cj",
			LineStart:       30,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		}, {
			Source:          "bridge/Bridge.cj",
			LineStart:       15,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		}},
	})
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
					HasPerMemberTable:     true,
				},
				SourceInventoryProfile: &types.SourceInventoryProfile{
					IsSourceInventory: true,
					TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction, types.AnswerCandidateRoleType},
					Confidence:        0.9,
				},
			},
		},
	}}
	state := newGraphState(types.TaskGraph{
		ExecutionPolicy: types.ExecutionPolicy{RetryBudget: 1},
	})

	msg, proceed, exhausted := o.checkTier1Floor(o.busCtx.AnalysisIR, state)
	if !proceed || exhausted || msg != "" {
		t.Fatalf("case-drifted principal member anchors should match read-backed repo paths, proceed=%v exhausted=%v msg=%q", proceed, exhausted, msg)
	}
}

func TestCheckTier1Floor_ReadLocalizerFollowupKeepsMissingPrincipalMemberAnchor(t *testing.T) {
	prev := tool.CurrentGroundingPolicy()
	tool.SetGroundingPolicy(tool.DefaultGroundingPolicy())
	t.Cleanup(func() { tool.SetGroundingPolicy(prev) })

	mu := types.NewMutableState("list source inventory members")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "@Entry members",
		Value:   "1",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"Index"},
	}})
	mu.SetInvestigationComplete("principal member set missing anchor")
	mu.RetainInvestigationAggregateFacts()
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles: []string{"internal/tool/repomap/index/extract_arkts.go"},
	})
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
				},
				SourceInventoryProfile: &types.SourceInventoryProfile{
					IsSourceInventory: true,
					TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleType},
					Confidence:        0.9,
				},
			},
		},
	}}
	state := newGraphState(types.TaskGraph{
		ExecutionPolicy: types.ExecutionPolicy{RetryBudget: 1},
	})

	msg, proceed, exhausted := o.checkTier1Floor(o.busCtx.AnalysisIR, state)
	if proceed || exhausted {
		t.Fatalf("missing principal member anchor should still requeue, proceed=%v exhausted=%v msg=%q", proceed, exhausted, msg)
	}
	if !strings.Contains(msg, "repo_map") || !strings.Contains(msg, "read_file") {
		t.Fatalf("retry message should preserve typed localizer recovery guidance, got:\n%s", msg)
	}
}

func TestCheckTier1Floor_RuntimeTraceObservationOnlySkipsNavigationFollowup(t *testing.T) {
	prev := tool.CurrentGroundingPolicy()
	tool.SetGroundingPolicy(tool.DefaultGroundingPolicy())
	t.Cleanup(func() { tool.SetGroundingPolicy(prev) })

	mu := types.NewMutableState("analyze attached trace root cause")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{})
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable:         mu,
		AttachedHitrace: "app-100 sched_switch prev_state=S",
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentTrace,
			Scenario: types.ScenarioPerformanceBottleneck,
			AnalyzerHints: types.AnalyzerHints{
				PrimaryEntities: []string{"app-100"},
			},
		}},
	}}
	state := newGraphState(types.TaskGraph{
		ExecutionPolicy: types.ExecutionPolicy{RetryBudget: 1},
	})

	msg, proceed, exhausted := o.checkTier1Floor(o.busCtx.AnalysisIR, state)
	if !proceed || exhausted || msg != "" {
		t.Fatalf("runtime-only trace should skip source-navigation follow-up, proceed=%v exhausted=%v msg=%q", proceed, exhausted, msg)
	}
}

func TestCheckTier1Floor_RuntimeTraceQueryObservationSkipsNavigationFollowupWithoutAttachment(t *testing.T) {
	prev := tool.CurrentGroundingPolicy()
	tool.SetGroundingPolicy(tool.DefaultGroundingPolicy())
	t.Cleanup(func() { tool.SetGroundingPolicy(prev) })

	mu := types.NewMutableState("analyze trace path root cause")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{})
	mu.AppendDispatchToolResult(tier1TraceQueryRuntimeToolResult())
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentTrace,
			Scenario: types.ScenarioPerformanceBottleneck,
			AnalyzerHints: types.AnalyzerHints{
				PrimaryEntities: []string{"app-100"},
			},
		}},
	}}
	state := newGraphState(types.TaskGraph{
		ExecutionPolicy: types.ExecutionPolicy{RetryBudget: 1},
	})

	msg, proceed, exhausted := o.checkTier1Floor(o.busCtx.AnalysisIR, state)
	if !proceed || exhausted || msg != "" {
		t.Fatalf("trace_query runtime observation should skip source-navigation follow-up, proceed=%v exhausted=%v msg=%q", proceed, exhausted, msg)
	}
}

func TestCheckTier1Floor_RuntimeTraceQueryClosureSkipsMechanismDimensionRepoMapDebt(t *testing.T) {
	prev := tool.CurrentGroundingPolicy()
	tool.SetGroundingPolicy(tool.DefaultGroundingPolicy())
	t.Cleanup(func() { tool.SetGroundingPolicy(prev) })

	mu := types.NewMutableState("analyze trace path root cause")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{})
	mu.AppendDispatchToolResult(tier1TraceQueryRuntimeToolResult())
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable: mu,
		TurnRouteHint: types.TurnRouteHint{
			Route:           "repo",
			Source:          "artifact",
			NeedsRepoAccess: true,
			Confidence:      0.9,
		},
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentRootCause,
			Scenario: types.ScenarioPerformanceBottleneck,
			AnalyzerHints: types.AnalyzerHints{
				PrimaryEntities: []string{"OHTrace_20260626_16.32.34.ftrace", "android.haitong-56023"},
			},
			ExternalObservationPolicy: &types.ExternalObservationPolicy{
				ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
				CurrentSourceMode:    types.ExternalObservationCurrentSourceDefault,
				Confidence:           0.9,
			},
			RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
				IsDimensionedAnswer: true,
				Dimensions: []types.RequestedAnswerDimension{{
					Label:       "deep runtime mechanism",
					Role:        types.RequestedAnswerDimensionFunctionOrPurpose,
					SourceQuote: "长时间运行的深层次原因",
					Required:    true,
					Index:       1,
				}},
				Confidence: 0.9,
			},
		}},
	}}
	state := newGraphState(types.TaskGraph{
		ExecutionPolicy: types.ExecutionPolicy{RetryBudget: 1},
	})

	msg, proceed, exhausted := o.checkTier1Floor(o.busCtx.AnalysisIR, state)
	if !proceed || exhausted || msg != "" {
		t.Fatalf("answer-grade trace_query observations should suppress stale repo_map debt for runtime-only mechanism dimensions, proceed=%v exhausted=%v msg=%q", proceed, exhausted, msg)
	}
}

func TestCheckTier1Floor_RuntimeTraceQuerySoftCurrentSourceObligationDowngradesToCaveat(t *testing.T) {
	prev := tool.CurrentGroundingPolicy()
	tool.SetGroundingPolicy(tool.DefaultGroundingPolicy())
	t.Cleanup(func() { tool.SetGroundingPolicy(prev) })

	mu := types.NewMutableState("explain trace parser mechanism from current source")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{})
	mu.AppendDispatchToolResult(tier1TraceQueryRuntimeToolResult())
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentExplain,
			Scenario: types.ScenarioPerformanceBottleneck,
			PerfTrace: &types.PerfBundle{Observations: []types.PerfObservation{{
				Kind:       "trace_mark",
				Subject:    "H:RenderService:DoFrame",
				Summary:    "runtime trace span is janky",
				LineStart:  5,
				LineEnd:    6,
				DurationMs: 86.111,
			}}},
			ExternalObservationPolicy: &types.ExternalObservationPolicy{
				ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
				CurrentSourceMode:    types.ExternalObservationCurrentSourceDefault,
				Confidence:           0.9,
			},
			CurrentSourceObligationSignals: []types.CurrentSourceObligationSignal{{
				Kind:  types.CurrentSourceObligationSignalDroppedRequestedDimension,
				Role:  types.RequestedAnswerDimensionFunctionOrPurpose,
				Index: 1,
			}},
		}},
	}}
	state := newGraphState(types.TaskGraph{
		ExecutionPolicy: types.ExecutionPolicy{RetryBudget: 1},
	})

	msg, proceed, exhausted := o.checkTier1Floor(o.busCtx.AnalysisIR, state)
	if !proceed || exhausted || msg != "" {
		t.Fatalf("soft typed current-source obligation with runtime proof should proceed with caveat instead of repo_map follow-up, proceed=%v exhausted=%v msg=%q", proceed, exhausted, msg)
	}
}

func TestCheckTier1Floor_RuntimeTraceQuerySoftCurrentSourceProfileSuppressesLocalizer(t *testing.T) {
	prev := tool.CurrentGroundingPolicy()
	tool.SetGroundingPolicy(tool.DefaultGroundingPolicy())
	t.Cleanup(func() { tool.SetGroundingPolicy(prev) })

	mu := types.NewMutableState("explain trace parser mechanism from current source")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{})
	mu.AppendDispatchToolResult(tier1TraceQueryRuntimeToolResult())
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentExplain,
			Scenario: types.ScenarioPerformanceBottleneck,
			PerfTrace: &types.PerfBundle{Observations: []types.PerfObservation{{
				Kind:       "trace_mark",
				Subject:    "H:RenderService:DoFrame",
				Summary:    "runtime trace span is janky",
				LineStart:  5,
				LineEnd:    6,
				DurationMs: 86.111,
			}}},
			ExternalObservationPolicy: &types.ExternalObservationPolicy{
				ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
				CurrentSourceMode:    types.ExternalObservationCurrentSourceDefault,
				Confidence:           0.9,
			},
			CurrentSourceExplanationProfile: &types.CurrentSourceExplanationProfile{
				IsCurrentSourceExplanationRequested: true,
				Modes: []types.CurrentSourceExplanationMode{
					types.CurrentSourceExplanationExplainCurrentMechanism,
				},
				SourceQuotes: []string{"current parser mechanism"},
				Confidence:   0.9,
			},
		}},
	}}
	state := newGraphState(types.TaskGraph{
		ExecutionPolicy: types.ExecutionPolicy{RetryBudget: 1},
	})

	msg, proceed, exhausted := o.checkTier1Floor(o.busCtx.AnalysisIR, state)
	if !proceed || exhausted || msg != "" {
		t.Fatalf("soft current-source profile with runtime proof should proceed with caveat instead of localizer follow-up, proceed=%v exhausted=%v msg=%q", proceed, exhausted, msg)
	}
}

func TestCheckTier1Floor_AttachedLogObservationOnlySkipsNavigationFollowup(t *testing.T) {
	prev := tool.CurrentGroundingPolicy()
	tool.SetGroundingPolicy(tool.DefaultGroundingPolicy())
	t.Cleanup(func() { tool.SetGroundingPolicy(prev) })

	mu := types.NewMutableState("analyze attached log")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{})
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable:     mu,
		AttachedLog: "WARN request timed out at artifact line 42",
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentRootCause,
			Scenario: types.ScenarioRootCause,
			AnalyzerHints: types.AnalyzerHints{
				PrimaryEntities: []string{"request timeout"},
			},
		}},
	}}
	state := newGraphState(types.TaskGraph{
		ExecutionPolicy: types.ExecutionPolicy{RetryBudget: 1},
	})

	msg, proceed, exhausted := o.checkTier1Floor(o.busCtx.AnalysisIR, state)
	if !proceed || exhausted || msg != "" {
		t.Fatalf("runtime-only log should skip source-navigation follow-up, proceed=%v exhausted=%v msg=%q", proceed, exhausted, msg)
	}
}

func TestCheckTier1Floor_RuntimeTraceCurrentSourceRequirementKeepsNavigationFollowup(t *testing.T) {
	prev := tool.CurrentGroundingPolicy()
	tool.SetGroundingPolicy(tool.DefaultGroundingPolicy())
	t.Cleanup(func() { tool.SetGroundingPolicy(prev) })

	mu := types.NewMutableState("analyze trace and current implementation")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{})
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable:         mu,
		AttachedHitrace: "app-100 sched_switch prev_state=S",
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentTrace,
			Scenario: types.ScenarioPerformanceBottleneck,
			AnalyzerHints: types.AnalyzerHints{
				PrimaryEntities: []string{"app-100"},
			},
			SourceScopeProfile: &types.SourceScopeProfile{
				RequestedScope: types.SourceScopeProduction,
				Confidence:     0.8,
			},
			RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
				IsDimensionedAnswer: true,
				Dimensions: []types.RequestedAnswerDimension{{
					Label:    "current implementation",
					Role:     types.RequestedAnswerDimensionCurrentKeyCode,
					Required: true,
					Index:    1,
				}},
				Confidence: 0.9,
			},
		}},
	}}
	state := newGraphState(types.TaskGraph{
		ExecutionPolicy: types.ExecutionPolicy{RetryBudget: 1},
	})

	msg, proceed, exhausted := o.checkTier1Floor(o.busCtx.AnalysisIR, state)
	if proceed || exhausted || !strings.Contains(msg, "repo_map") {
		t.Fatalf("current-source requirement should keep navigation follow-up, proceed=%v exhausted=%v msg=%q", proceed, exhausted, msg)
	}
}

func TestCheckTier1Floor_RuntimeTraceQueryCurrentSourceRequirementKeepsNavigationFollowup(t *testing.T) {
	prev := tool.CurrentGroundingPolicy()
	tool.SetGroundingPolicy(tool.DefaultGroundingPolicy())
	t.Cleanup(func() { tool.SetGroundingPolicy(prev) })

	mu := types.NewMutableState("analyze trace and current implementation")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{})
	mu.AppendDispatchToolResult(tier1TraceQueryRuntimeToolResult())
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentTrace,
			Scenario: types.ScenarioPerformanceBottleneck,
			RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
				IsDimensionedAnswer: true,
				Dimensions: []types.RequestedAnswerDimension{{
					Label:    "current implementation",
					Role:     types.RequestedAnswerDimensionCurrentKeyCode,
					Required: true,
					Index:    1,
				}},
				Confidence: 0.9,
			},
		}},
	}}
	state := newGraphState(types.TaskGraph{
		ExecutionPolicy: types.ExecutionPolicy{RetryBudget: 1},
	})

	msg, proceed, exhausted := o.checkTier1Floor(o.busCtx.AnalysisIR, state)
	if proceed || exhausted || !strings.Contains(msg, "repo_map") {
		t.Fatalf("trace_query with current-source requirement should keep navigation follow-up, proceed=%v exhausted=%v msg=%q", proceed, exhausted, msg)
	}
}

func TestCheckTier1Floor_ReadLocalizerFollowupCoveredDoesNotBlock(t *testing.T) {
	prev := tool.CurrentGroundingPolicy()
	tool.SetGroundingPolicy(tool.DefaultGroundingPolicy())
	t.Cleanup(func() { tool.SetGroundingPolicy(prev) })

	mu := types.NewMutableState("where is the request dispatched")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles: []string{"pkg/handler.py"},
		EvidenceItems: []types.EvidenceItem{{
			ID:              "ev-owner",
			Source:          "pkg/handler.py",
			Kind:            types.EvidenceDirect,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
			OwnerSymbol:     "Handler",
		}},
		ToolResults: []types.ToolResult{{
			ToolName: "repo_map",
			Success:  true,
			Observations: []types.ObservationRecord{
				{
					ID:        "repo_map:file#navigation:file_map",
					Producer:  "repo_map",
					Predicate: types.RepoMapNavigationObservationPredicate,
					Object:    string(types.RepoMapNavigationRouteFileMap),
				},
				{
					ID:        "repo_map:relation#navigation:relation_map",
					Producer:  "repo_map",
					Predicate: types.RepoMapNavigationObservationPredicate,
					Object:    string(types.RepoMapNavigationRouteRelationMap),
				},
				{
					ID:        "repo_map:call#navigation:call_path",
					Producer:  "repo_map",
					Predicate: types.RepoMapNavigationObservationPredicate,
					Object:    string(types.RepoMapNavigationRouteCallPath),
				},
			},
		}},
	})
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentTrace},
		},
	}}
	ir := o.busCtx.AnalysisIR
	state := newGraphState(types.TaskGraph{
		ExecutionPolicy: types.ExecutionPolicy{RetryBudget: 1},
	})

	msg, proceed, exhausted := o.checkTier1Floor(ir, state)
	if !proceed || exhausted || msg != "" {
		t.Fatalf("covered localization/navigation should proceed, proceed=%v exhausted=%v msg=%q", proceed, exhausted, msg)
	}
}

func tier1TraceQueryRuntimeToolResult() types.ToolResult {
	return types.ToolResult{
		ToolName: "trace_query",
		Success:  true,
		Observations: []types.ObservationRecord{{
			ID:              "trace_query:window#root_cause_rank:1",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: types.ClaimGroundingHard,
			SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact},
			Subject:         "app-100",
			Predicate:       "root_cause_primary",
			Object:          "runnable",
		}},
	}
}
