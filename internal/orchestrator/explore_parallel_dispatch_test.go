package orchestrator

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestDispatchExploreWindowsParallel_CancelsSiblingAfterConvergence(t *testing.T) {
	var slowStarted sync.Once
	slowStartedCh := make(chan struct{})
	var slowCanceled int32

	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			switch ctx.ExploreDispatchKey {
			case "done":
				<-slowStartedCh
				ctx.Mutable.SetInvestigationComplete("parallel branch reached terminal evidence")
				return &agent.StageOutput{
					MissingPiece:  types.MissingNone,
					SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: true},
				}, nil
			case "slow":
				slowStarted.Do(func() { close(slowStartedCh) })
				<-ctx.Context().Done()
				atomic.StoreInt32(&slowCanceled, 1)
				return nil, ctx.Context().Err()
			default:
				t.Fatalf("unexpected dispatch key %q", ctx.ExploreDispatchKey)
				return nil, nil
			}
		},
	})
	o := New(types.PipelineSettings{MaxParallelism: 2}, ar, sr, sar)
	o.busCtx = &types.BusContext{
		Mutable: types.NewMutableState("parallel explore cancellation"),
		Signals: types.ExecutionSignals{},
	}

	out, err := o.dispatchExploreWindowsParallel([][]*types.TaskNode{
		{{ID: "done"}},
		{{ID: "slow"}},
	}, nil, 2)
	if err != nil {
		t.Fatalf("terminal branch should absorb canceled sibling, got error: %v", err)
	}
	if out == nil || out.SignalUpdates == nil || !out.SignalUpdates.HasEnoughFacts {
		t.Fatalf("merged output should preserve terminal enough-facts signal, got %+v", out)
	}
	if !o.busCtx.Mutable.IsInvestigationComplete() {
		t.Fatal("terminal fork state was not merged into parent mutable state")
	}
	if atomic.LoadInt32(&slowCanceled) != 1 {
		t.Fatal("running sibling explorer did not observe cancellation after convergence")
	}
}

func TestDispatchExploreWindowsParallel_ScopesLanePlanPerEvidenceWindow(t *testing.T) {
	seen := map[string][]string{}
	var mu sync.Mutex

	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			var units []string
			for _, lane := range ctx.ExploreLanePlan.Lanes {
				units = append(units, lane.InvestigationUnitID)
			}
			mu.Lock()
			seen[ctx.ExploreDispatchKey] = append([]string(nil), units...)
			mu.Unlock()
			return &agent.StageOutput{
				MissingPiece:  types.MissingNone,
				SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: true},
			}, nil
		},
	})
	o := New(types.PipelineSettings{MaxParallelism: 2}, ar, sr, sar)
	o.busCtx = &types.BusContext{
		Mutable: types.NewMutableState("parallel lane ownership"),
		Signals: types.ExecutionSignals{},
		ExploreLanePlan: types.ExploreLanePlan{Lanes: []types.ExploreLane{
			{ID: "lane-a", Label: "current_source", Origin: types.AnswerEvidenceOriginCurrentSource, InvestigationUnitID: "subtopic-1"},
			{ID: "lane-b", Label: "current_source", Origin: types.AnswerEvidenceOriginCurrentSource, InvestigationUnitID: "subtopic-2"},
		}},
	}

	_, err := o.dispatchExploreWindowsParallel([][]*types.TaskNode{
		{{ID: "n1_evidence_t0", Type: types.NodeEvidence}},
		{{ID: "n1_evidence_t1", Type: types.NodeEvidence}},
	}, nil, 2)
	if err != nil {
		t.Fatalf("dispatchExploreWindowsParallel: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if got := seen["n1_evidence_t0"]; len(got) != 1 || got[0] != "subtopic-1" {
		t.Fatalf("first window should see only subtopic-1 lane, got %#v (all=%#v)", got, seen)
	}
	if got := seen["n1_evidence_t1"]; len(got) != 1 || got[0] != "subtopic-2" {
		t.Fatalf("second window should see only subtopic-2 lane, got %#v (all=%#v)", got, seen)
	}
}

func TestScopeExploreLanePlansForWindows_DemotesExactDuplicateLane(t *testing.T) {
	plan := types.ExploreLanePlan{Lanes: []types.ExploreLane{{
		ID:                  "lane-request",
		Label:               "current_source",
		Origin:              types.AnswerEvidenceOriginCurrentSource,
		InvestigationUnitID: "request",
		Role:                types.ExploreLaneRolePrincipal,
		HandoffPolicy:       types.ExploreLaneHandoffOwn,
	}}}
	got := scopeExploreLanePlansForWindows(plan, [][]*types.TaskNode{
		{{ID: "evidence_a", Type: types.NodeEvidence}},
		{{ID: "evidence_b", Type: types.NodeEvidence}},
	})
	if len(got) != 2 {
		t.Fatalf("plans=%d want 2", len(got))
	}
	if len(got[0].Lanes) != 1 || got[0].Lanes[0].HandoffPolicy != types.ExploreLaneHandoffOwn ||
		got[0].Lanes[0].Role != types.ExploreLaneRolePrincipal {
		t.Fatalf("first exact lane should remain principal owner: %+v", got[0].Lanes)
	}
	if len(got[1].Lanes) != 1 || got[1].Lanes[0].HandoffPolicy != types.ExploreLaneHandoffSupport ||
		got[1].Lanes[0].Role != types.ExploreLaneRoleSupport {
		t.Fatalf("second exact duplicate should become support handoff: %+v", got[1].Lanes)
	}
	if plan.Lanes[0].HandoffPolicy != types.ExploreLaneHandoffOwn || plan.Lanes[0].Role != types.ExploreLaneRolePrincipal {
		t.Fatalf("scoping must not mutate source plan: %+v", plan.Lanes)
	}
}

func TestApplyExploreLaneHandoffIterationCap_SupportOnlyLane(t *testing.T) {
	ctx := &types.AgentContext{
		MaxIterOverride: 30,
		ExploreLanePlan: types.ExploreLanePlan{Lanes: []types.ExploreLane{{
			ID:            "lane-support",
			Label:         "current_source",
			Origin:        types.AnswerEvidenceOriginCurrentSource,
			Role:          types.ExploreLaneRoleSupport,
			HandoffPolicy: types.ExploreLaneHandoffSupport,
		}}},
	}
	applyExploreLaneHandoffIterationCap(ctx)
	if ctx.MaxIterOverride != 8 {
		t.Fatalf("support-only duplicate lane should be capped to 8 iterations, got %d", ctx.MaxIterOverride)
	}

	ctx.MaxIterOverride = 5
	applyExploreLaneHandoffIterationCap(ctx)
	if ctx.MaxIterOverride != 5 {
		t.Fatalf("handoff cap must not raise an already tighter limit, got %d", ctx.MaxIterOverride)
	}

	ctx = &types.AgentContext{
		ExploreLanePlan: types.ExploreLanePlan{Lanes: []types.ExploreLane{{
			ID:            "lane-owner",
			Label:         "current_source",
			Origin:        types.AnswerEvidenceOriginCurrentSource,
			Role:          types.ExploreLaneRolePrincipal,
			HandoffPolicy: types.ExploreLaneHandoffOwn,
		}}},
	}
	applyExploreLaneHandoffIterationCap(ctx)
	if ctx.MaxIterOverride != 0 {
		t.Fatalf("principal owner lane must keep normal explorer budget, got %d", ctx.MaxIterOverride)
	}
}

func TestDispatchExploreWindowsParallel_SkipsNonWinningPartialSiblingAfterConvergence(t *testing.T) {
	partialDoneCh := make(chan struct{})

	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			switch ctx.ExploreDispatchKey {
			case "partial":
				ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
					UserQuestion:          "q",
					AcceptedClosureReason: "partial sibling never closed",
					AcceptedAggregateFacts: []types.AnswerAggregateFact{{
						Kind:    types.AnswerAggregateMemberSet,
						Label:   "broad partial members",
						Value:   "2",
						Role:    types.AnswerAggregateRolePrincipalAnswer,
						Members: []string{"HelperA", "HelperB"},
					}},
				})
				close(partialDoneCh)
				return &agent.StageOutput{
					MissingPiece: types.MissingFacts,
					StageReport:  "partial branch report",
					Repairs: []types.RepairDirective{{
						Kind:      types.RepairEmitEvidence,
						Files:     []string{"support_only.go"},
						Rationale: "partial sibling support debt",
						Origin:    "test_partial_sibling",
					}},
					SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: false},
				}, nil
			case "winner":
				<-partialDoneCh
				facts := []types.AnswerAggregateFact{{
					Kind:    types.AnswerAggregateMemberSet,
					Label:   "accepted members",
					Value:   "1",
					Role:    types.AnswerAggregateRolePrincipalAnswer,
					Members: []string{"AnswerA"},
				}}
				ctx.Mutable.SetInvestigationAggregateFacts(facts)
				ctx.Mutable.SetInvestigationComplete("winning branch accepted closure")
				ctx.Mutable.RetainInvestigationAggregateFacts()
				ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
					UserQuestion:                     "q",
					AcceptedClosureReason:            "winning branch accepted closure",
					AcceptedAggregateFacts:           facts,
					TerminalEvidenceCount:            1,
					RuntimeObservationOnlyCompletion: false,
				})
				return &agent.StageOutput{
					MissingPiece:  types.MissingNone,
					StageReport:   "winner branch report",
					SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: true},
				}, nil
			default:
				t.Fatalf("unexpected dispatch key %q", ctx.ExploreDispatchKey)
				return nil, nil
			}
		},
	})
	o := New(types.PipelineSettings{MaxParallelism: 2}, ar, sr, sar)
	o.busCtx = &types.BusContext{
		Mutable: types.NewMutableState("parallel winner owns principal closure"),
		Signals: types.ExecutionSignals{},
	}

	out, err := o.dispatchExploreWindowsParallel([][]*types.TaskNode{
		{{ID: "partial"}},
		{{ID: "winner"}},
	}, nil, 2)
	if err != nil {
		t.Fatalf("winning branch should absorb non-winning partial sibling, got error: %v", err)
	}
	if out == nil || out.SignalUpdates == nil || !out.SignalUpdates.HasEnoughFacts {
		t.Fatalf("merged output should preserve winning enough-facts signal, got %+v", out)
	}
	if len(o.busCtx.StageReports) != 1 || o.busCtx.StageReports[0].Findings != "winner branch report" {
		t.Fatalf("stage reports = %+v, want only winning branch report", o.busCtx.StageReports)
	}
	if repairs := o.busCtx.Mutable.EvidenceClosure().PendingRepairs(); len(repairs) != 0 {
		t.Fatalf("non-winning partial repairs leaked into parent: %+v", repairs)
	}
	facts := o.busCtx.Mutable.StableInvestigationAggregateFacts()
	if len(facts) != 1 || strings.Join(facts[0].Members, ",") != "AnswerA" {
		t.Fatalf("stable aggregate facts = %+v, want only winning closure members", facts)
	}
	ta := o.busCtx.Mutable.TurnAArtifacts()
	if ta == nil || strings.TrimSpace(ta.AcceptedClosureReason) != "winning branch accepted closure" {
		t.Fatalf("TurnA accepted closure = %+v, want winning closure only", ta)
	}
	if len(ta.AcceptedAggregateFacts) != 1 || strings.Join(ta.AcceptedAggregateFacts[0].Members, ",") != "AnswerA" {
		t.Fatalf("TurnA aggregate facts = %+v, want only winning facts", ta.AcceptedAggregateFacts)
	}
}

func TestDispatchExploreWindowsParallel_HistoryNarrativeSubtopicsCanConvergeEarly(t *testing.T) {
	var slowStarted sync.Once
	slowStartedCh := make(chan struct{})
	var slowCanceled int32

	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			switch ctx.ExploreDispatchKey {
			case "history-done":
				<-slowStartedCh
				ctx.Mutable.SetInvestigationComplete("history topic search converged with VCS evidence")
				return &agent.StageOutput{
					MissingPiece:  types.MissingNone,
					SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: true},
				}, nil
			case "source-slow":
				slowStarted.Do(func() { close(slowStartedCh) })
				<-ctx.Context().Done()
				atomic.StoreInt32(&slowCanceled, 1)
				return nil, ctx.Context().Err()
			default:
				t.Fatalf("unexpected dispatch key %q", ctx.ExploreDispatchKey)
				return nil, nil
			}
		},
	})
	o := New(types.PipelineSettings{MaxParallelism: 2}, ar, sr, sar)
	o.busCtx = &types.BusContext{
		PipelineStage: types.StageAnalyze,
		ActiveAgent:   types.AgentAnalyzer,
		Mutable:       types.NewMutableState("parallel history narrative"),
		Signals:       types.ExecutionSignals{},
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain,
			Predicates: types.SemanticPredicates{
				IsHistoryLookup: true,
			},
			SubTopics: []types.SubTopic{
				{Summary: "commit topic A", Entities: []string{"ScalarAnswer"}},
				{Summary: "commit topic B", Entities: []string{"AggregateScalar"}},
			},
		}},
	}

	out, err := o.dispatchExploreWindowsParallel([][]*types.TaskNode{
		{{ID: "history-done"}},
		{{ID: "source-slow"}},
	}, nil, 2)
	if err != nil {
		t.Fatalf("history narrative convergence should absorb canceled sibling, got error: %v", err)
	}
	if out == nil || out.SignalUpdates == nil || !out.SignalUpdates.HasEnoughFacts {
		t.Fatalf("merged output should preserve enough-facts signal, got %+v", out)
	}
	if atomic.LoadInt32(&slowCanceled) != 1 {
		t.Fatal("history narrative sibling was not canceled after convergence")
	}
}

func TestDispatchExploreWindowsParallel_EnumerationWaitsForSiblingHandoffs(t *testing.T) {
	var slowStarted sync.Once
	slowStartedCh := make(chan struct{})
	doneFinishedCh := make(chan struct{})
	var slowCanceled int32

	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			switch ctx.ExploreDispatchKey {
			case "done":
				<-slowStartedCh
				ctx.Mutable.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
					Kind:    types.AnswerAggregateMemberSet,
					Label:   "public functions",
					Value:   "1",
					Role:    types.AnswerAggregateRolePrincipalAnswer,
					Members: []string{"Eval"},
				}})
				ctx.Mutable.SetInvestigationComplete("parallel branch reached partial enum closure")
				ctx.Mutable.RetainInvestigationAggregateFacts()
				close(doneFinishedCh)
				return &agent.StageOutput{
					MissingPiece:  types.MissingNone,
					StageReport:   "done branch report",
					SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: true},
				}, nil
			case "slow":
				slowStarted.Do(func() { close(slowStartedCh) })
				select {
				case <-ctx.Context().Done():
					atomic.StoreInt32(&slowCanceled, 1)
					return nil, ctx.Context().Err()
				case <-doneFinishedCh:
				}
				ctx.Mutable.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
					Kind:    types.AnswerAggregateMemberSet,
					Label:   "public functions",
					Value:   "2",
					Role:    types.AnswerAggregateRolePrincipalAnswer,
					Members: []string{"EvalAll", "RegisteredKinds"},
				}})
				ctx.Mutable.SetInvestigationComplete("parallel sibling completed enum closure")
				ctx.Mutable.RetainInvestigationAggregateFacts()
				return &agent.StageOutput{
					MissingPiece:  types.MissingNone,
					StageReport:   "slow branch report",
					SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: true},
				}, nil
			default:
				t.Fatalf("unexpected dispatch key %q", ctx.ExploreDispatchKey)
				return nil, nil
			}
		},
	})
	o := New(types.PipelineSettings{MaxParallelism: 2}, ar, sr, sar)
	o.busCtx = &types.BusContext{
		PipelineStage: types.StageAnalyze,
		ActiveAgent:   types.AgentAnalyzer,
		Mutable:       types.NewMutableState("parallel explore enumeration"),
		Signals:       types.ExecutionSignals{},
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
		}},
	}

	_, err := o.dispatchExploreWindowsParallel([][]*types.TaskNode{
		{{ID: "done"}},
		{{ID: "slow"}},
	}, nil, 2)
	if err != nil {
		t.Fatalf("enumeration siblings should finish instead of being canceled: %v", err)
	}
	if atomic.LoadInt32(&slowCanceled) != 0 {
		t.Fatal("enumeration sibling was canceled even though its member_set handoff is required")
	}
	facts := o.busCtx.Mutable.StableInvestigationAggregateFacts()
	if len(facts) != 1 {
		t.Fatalf("stable aggregate facts = %+v, want merged function member_set", facts)
	}
	got := strings.Join(facts[0].Members, ",")
	want := "Eval,EvalAll,RegisteredKinds"
	if got != want {
		t.Fatalf("merged members = %q, want %q", got, want)
	}
	if len(o.busCtx.StageReports) != 2 {
		t.Fatalf("stage reports = %+v, want one per completed parallel branch", o.busCtx.StageReports)
	}
	for _, report := range o.busCtx.StageReports {
		if report.Stage != types.StageExplore || report.Agent != types.AgentExplorer {
			t.Fatalf("parallel stage report metadata = %+v, want explore/explorer", report)
		}
	}
}

func TestParallelExploreAllowsEarlyConvergence_HistoryDiagramStaysMixed(t *testing.T) {
	o := &Orchestrator{busCtx: &types.BusContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentExplain,
				Scenario: types.ScenarioGeneric,
				Predicates: types.SemanticPredicates{
					IsHistoryLookup: true,
				},
				SubTopics: []types.SubTopic{
					{Summary: "commit"},
					{Summary: "current flow"},
				},
				DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow},
			},
			AnswerContract: types.AnswerContract{
				Diagram: &types.DiagramContract{Required: true, RequiredKind: types.DiagramFlow},
			},
		},
	}}

	if o.parallelExploreAllowsEarlyConvergence() {
		t.Fatal("history + required diagram/current-flow evidence must wait for sibling handoffs")
	}
}

func TestParallelExploreAllowsEarlyConvergence_HistoryCurrentCodeMechanism(t *testing.T) {
	o := &Orchestrator{busCtx: &types.BusContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentExplain,
				Scenario: types.ScenarioArchitectureExplain,
				Predicates: types.SemanticPredicates{
					IsHistoryLookup: true,
				},
				AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
				SubTopics: []types.SubTopic{
					{Summary: "commit clue"},
					{Summary: "current implementation"},
					{Summary: "gate relationship"},
				},
			},
		},
	}}

	if o.parallelExploreAllowsEarlyConvergence() {
		t.Fatal("history-backed current-code mechanism must wait for both VCS and current-source handoffs")
	}
}

func TestAcceptedClosureAutoCompleteBlocksUntilMixedHistoryCurrentSourceOriginsPresent(t *testing.T) {
	mut := types.NewMutableState("mixed history plus current source")
	mut.SetInvestigationComplete("history lane finished first")
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentExplain,
			Scenario: types.ScenarioArchitectureExplain,
			Predicates: types.SemanticPredicates{
				IsHistoryLookup: true,
			},
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
		}},
		ToolResults: []types.ToolResult{{
			ToolName: "exec_command",
			Success:  true,
			Summary:  "[git_show: evidence_origin=vcs_metadata diff_origin=vcs_diff repo=. commit=abc123]\ncommit abc123 changed retry behavior",
		}},
	}}

	if o.shouldAutoCompleteExploreWindowFromAcceptedClosure(nil, "", "") {
		t.Fatal("accepted closure must not skip remaining mixed-origin windows when current-source lane is still missing")
	}

	o.busCtx.EvidenceItems = []types.EvidenceItem{{
		ID:              "current-source",
		Source:          "internal/orchestrator/orchestrator.go",
		LineStart:       42,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "shouldAutoCompleteExploreWindowFromAcceptedClosure",
		Scope:           types.ScopeLine,
		GroundingStatus: types.GroundingGrounded,
		Summary:         "current implementation anchor",
	}}
	if !o.shouldAutoCompleteExploreWindowFromAcceptedClosure(nil, "", "") {
		t.Fatal("accepted closure may auto-complete after all typed mixed-origin lanes have accepted observations")
	}
}

func TestAcceptedClosureAutoCompleteBlocksUntilRuntimeCurrentSourceOriginsPresent(t *testing.T) {
	mut := types.NewMutableState("runtime plus current source")
	mut.SetInvestigationComplete("runtime artifact lane finished first")
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentExplain,
			Scenario: types.ScenarioArchitectureExplain,
			LogTriage: &types.LogBundle{Observations: []types.LogObservation{{
				Kind:      types.LogObservationRetryCycle,
				Subject:   "retry status",
				Summary:   "runtime log shows a retry loop",
				LineStart: 7,
			}}},
			CurrentSourceExplanationProfile: &types.CurrentSourceExplanationProfile{
				IsCurrentSourceExplanationRequested: true,
				Modes:                               []types.CurrentSourceExplanationMode{types.CurrentSourceExplanationExplainCurrentMechanism},
				SourceQuotes:                        []string{"结合当前代码"},
				Confidence:                          0.9,
			},
		}},
	}}

	if o.shouldAutoCompleteExploreWindowFromAcceptedClosure(nil, "", "") {
		t.Fatal("runtime+current-source request must not auto-complete before current-source evidence is accepted")
	}

	o.busCtx.EvidenceItems = []types.EvidenceItem{{
		ID:              "retry-code",
		Source:          "internal/llm/openai.go",
		LineStart:       88,
		AnchorKind:      types.AnchorCondition,
		AnchorSymbol:    "firstByteTimeout",
		Scope:           types.ScopeLine,
		GroundingStatus: types.GroundingGrounded,
		Summary:         "current retry condition",
	}}
	if !o.shouldAutoCompleteExploreWindowFromAcceptedClosure(nil, "", "") {
		t.Fatal("runtime+current-source request may auto-complete after runtime and current-source lanes are both present")
	}
}

func TestAcceptedClosureAutoCompleteObservationOnlyRuntimeDoesNotBlock(t *testing.T) {
	mut := types.NewMutableState("runtime artifact only")
	mut.SetInvestigationComplete("runtime artifact answered locally")
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentRootCause,
			Scenario: types.ScenarioRootCause,
			Predicates: types.SemanticPredicates{
				IsDiagnosticQuestion: true,
			},
			LogTriage: &types.LogBundle{Observations: []types.LogObservation{{
				Kind:      types.LogObservationContractViolation,
				Severity:  types.LogObservationFailure,
				Subject:   "validator retry",
				Summary:   "the log itself identifies the failed validator",
				LineStart: 12,
			}}},
		}},
	}}

	if !o.shouldAutoCompleteExploreWindowFromAcceptedClosure(nil, "", "") {
		t.Fatal("observation-only runtime artifact should not be forced to open a current-source lane")
	}
}

func TestAcceptedClosureAutoCompleteHistoryScalarDoesNotBlockOnCurrentSource(t *testing.T) {
	mut := types.NewMutableState("history scalar")
	mut.SetInvestigationComplete("scalar commit id found")
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentReturnValue,
			Predicates: types.SemanticPredicates{
				IsHistoryLookup: true,
				IsScalarAnswer:  true,
			},
		}},
		ToolResults: []types.ToolResult{{
			ToolName: "exec_command",
			Success:  true,
			Summary:  "[git_log: evidence_origin=vcs_metadata repo=. ref=HEAD count=1]\nabc123 latest commit",
		}},
	}}

	if !o.shouldAutoCompleteExploreWindowFromAcceptedClosure(nil, "", "") {
		t.Fatal("scalar history lookup should not be widened into a current-source sibling obligation")
	}
}

func TestDispatchExploreWindowsParallel_MixedOriginMechanismWaitsForSourceSibling(t *testing.T) {
	var slowStarted sync.Once
	slowStartedCh := make(chan struct{})
	doneFinishedCh := make(chan struct{})
	var slowCanceled int32

	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			switch ctx.ExploreDispatchKey {
			case "history-done":
				<-slowStartedCh
				ctx.Mutable.SetInvestigationComplete("VCS lane collected the merge narrative")
				close(doneFinishedCh)
				return &agent.StageOutput{
					MissingPiece:  types.MissingNone,
					SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: true},
				}, nil
			case "current-source":
				slowStarted.Do(func() { close(slowStartedCh) })
				select {
				case <-ctx.Context().Done():
					atomic.StoreInt32(&slowCanceled, 1)
					return nil, ctx.Context().Err()
				case <-doneFinishedCh:
				}
				ctx.Mutable.SetInvestigationComplete("current-source lane collected present implementation anchors")
				return &agent.StageOutput{
					MissingPiece:  types.MissingNone,
					SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: true},
				}, nil
			default:
				t.Fatalf("unexpected dispatch key %q", ctx.ExploreDispatchKey)
				return nil, nil
			}
		},
	})
	o := New(types.PipelineSettings{MaxParallelism: 2}, ar, sr, sar)
	o.busCtx = &types.BusContext{
		PipelineStage: types.StageAnalyze,
		ActiveAgent:   types.AgentAnalyzer,
		Mutable:       types.NewMutableState("mixed history plus current code"),
		Signals:       types.ExecutionSignals{},
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentExplain,
			Scenario: types.ScenarioArchitectureExplain,
			Predicates: types.SemanticPredicates{
				IsHistoryLookup: true,
			},
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
		}},
	}

	_, err := o.dispatchExploreWindowsParallel([][]*types.TaskNode{
		{{ID: "history-done"}},
		{{ID: "current-source"}},
	}, nil, 2)
	if err != nil {
		t.Fatalf("mixed-origin siblings should finish instead of being canceled: %v", err)
	}
	if atomic.LoadInt32(&slowCanceled) != 0 {
		t.Fatal("current-source sibling was canceled even though mixed-origin mechanism needs both lanes")
	}
}

func TestDispatchExploreWindowsParallel_CollectiveLaneConvergenceCancelsSupportSibling(t *testing.T) {
	var supportStarted sync.Once
	supportStartedCh := make(chan struct{})
	vcsDoneCh := make(chan struct{})
	var supportCanceled int32

	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			switch ctx.ExploreDispatchKey {
			case "n1_evidence_t0":
				<-supportStartedCh
				ctx.Mutable.SetInvestigationComplete("VCS lane accepted commit evidence")
				close(vcsDoneCh)
				return &agent.StageOutput{
					MissingPiece:  types.MissingNone,
					StageReport:   "vcs owner report",
					SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: true},
				}, nil
			case "n1_evidence_t1":
				<-vcsDoneCh
				ctx.Mutable.SetInvestigationComplete("current-source lane accepted implementation evidence")
				return &agent.StageOutput{
					MissingPiece:  types.MissingNone,
					StageReport:   "source owner report",
					SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: true},
				}, nil
			case "n1_evidence_t2":
				supportStarted.Do(func() { close(supportStartedCh) })
				<-ctx.Context().Done()
				atomic.StoreInt32(&supportCanceled, 1)
				return nil, ctx.Context().Err()
			default:
				t.Fatalf("unexpected dispatch key %q", ctx.ExploreDispatchKey)
				return nil, nil
			}
		},
	})
	o := New(types.PipelineSettings{MaxParallelism: 3}, ar, sr, sar)
	o.busCtx = &types.BusContext{
		PipelineStage: types.StageAnalyze,
		ActiveAgent:   types.AgentAnalyzer,
		Mutable:       types.NewMutableState("collective lane convergence"),
		Signals:       types.ExecutionSignals{},
		ExploreLanePlan: types.ExploreLanePlan{Lanes: []types.ExploreLane{
			{
				ID:                  "vcs-lane",
				Label:               "vcs_history",
				Origin:              types.AnswerEvidenceOriginVCSMetadata,
				InvestigationUnitID: "subtopic-1",
				Role:                types.ExploreLaneRolePrincipal,
				HandoffPolicy:       types.ExploreLaneHandoffOwn,
			},
			{
				ID:                  "source-lane",
				Label:               "current_source",
				Origin:              types.AnswerEvidenceOriginCurrentSource,
				InvestigationUnitID: "subtopic-2",
				Role:                types.ExploreLaneRolePrincipal,
				HandoffPolicy:       types.ExploreLaneHandoffOwn,
			},
		}},
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentExplain,
			Scenario: types.ScenarioArchitectureExplain,
			Predicates: types.SemanticPredicates{
				IsHistoryLookup: true,
			},
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
		}},
	}

	out, err := o.dispatchExploreWindowsParallel([][]*types.TaskNode{
		{{ID: "n1_evidence_t0", Type: types.NodeEvidence}},
		{{ID: "n1_evidence_t1", Type: types.NodeEvidence}},
		{{ID: "n1_evidence_t2", Type: types.NodeEvidence}},
	}, nil, 3)
	if err != nil {
		t.Fatalf("collective typed-lane convergence should absorb canceled support sibling: %v", err)
	}
	if out == nil || out.SignalUpdates == nil || !out.SignalUpdates.HasEnoughFacts {
		t.Fatalf("merged output should preserve enough-facts after collective convergence, got %+v", out)
	}
	if atomic.LoadInt32(&supportCanceled) != 1 {
		t.Fatal("support-only sibling was not canceled after all required typed lanes converged")
	}
	if len(o.busCtx.StageReports) != 2 {
		t.Fatalf("stage reports = %+v, want only the two required owner reports", o.busCtx.StageReports)
	}
	got := []string{o.busCtx.StageReports[0].Findings, o.busCtx.StageReports[1].Findings}
	if strings.Join(got, "|") != "vcs owner report|source owner report" {
		t.Fatalf("stage reports = %+v, want owner reports only", got)
	}
}

func TestParallelExploreAllowsEarlyConvergence_HistoryCrossComponentMechanismWaits(t *testing.T) {
	o := &Orchestrator{busCtx: &types.BusContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentExplain,
				Scenario: types.ScenarioArchitectureExplain,
				Predicates: types.SemanticPredicates{
					IsHistoryLookup:  true,
					IsCrossComponent: true,
				},
				AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
				SubTopics: []types.SubTopic{
					{Summary: "component A"},
					{Summary: "component B"},
				},
			},
		},
	}}

	if o.parallelExploreAllowsEarlyConvergence() {
		t.Fatal("history-backed cross-component mechanism must wait for both VCS and current-source handoffs")
	}
}

func TestParallelExploreAllowsEarlyConvergence_BareCurrentCrossComponentMechanismConverges(t *testing.T) {
	o := &Orchestrator{busCtx: &types.BusContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentExplain,
				Scenario: types.ScenarioArchitectureExplain,
				Predicates: types.SemanticPredicates{
					IsCrossComponent: true,
				},
				AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
				SubTopics: []types.SubTopic{
					{Summary: "component A"},
					{Summary: "component B"},
				},
			},
		},
	}}

	if !o.parallelExploreAllowsEarlyConvergence() {
		t.Fatal("current-source-only cross-component/sub-topic breadth is advisory; accepted closure should converge")
	}
}

func TestParallelExploreAllowsEarlyConvergence_BucketedComparisonWaits(t *testing.T) {
	o := &Orchestrator{busCtx: &types.BusContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentExplain,
				Scenario: types.ScenarioArchitectureExplain,
				Predicates: types.SemanticPredicates{
					IsCrossComponent: true,
				},
				AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
				Buckets: []types.QuestionBucket{
					{Label: "component A", Index: 1},
					{Label: "component B", Index: 2},
				},
			},
		},
	}}

	if o.parallelExploreAllowsEarlyConvergence() {
		t.Fatal("explicit user bucket partitions should wait for sibling handoffs")
	}
}

func TestParallelExploreAllowsEarlyConvergence_DiagnosticFlagAloneConverges(t *testing.T) {
	o := &Orchestrator{busCtx: &types.BusContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentExplain,
				Scenario: types.ScenarioArchitectureExplain,
				Predicates: types.SemanticPredicates{
					IsDiagnosticQuestion: true,
					IsCrossComponent:     true,
				},
				DiagnosticProfile: types.DiagnosticIntentProfile{
					IsDiagnostic:        true,
					CurrentVersionCheck: true,
				},
				AnalyzerHints: types.AnalyzerHints{
					Kind: string(types.ReqMechanism),
					RequiredFileHints: []types.RequiredFileHint{
						{Path: "internal/llm/openai.go", Confidence: 0.9},
						{Path: "internal/orchestrator/read_stage_retry.go", Confidence: 0.8},
					},
				},
				SubTopics: []types.SubTopic{
					{Summary: "runtime artifact symptom", Entities: []string{"first_byte_timeout"}},
					{Summary: "current source retry behavior", Entities: []string{"retry"}},
				},
			},
		},
	}}

	if !o.parallelExploreAllowsEarlyConvergence() {
		t.Fatal("diagnostic/current-check flags are answer semantics; accepted closure plus required-file prechecks should converge")
	}
}

func TestParallelExploreAllowsEarlyConvergence_DiagnosticExplicitBucketsWait(t *testing.T) {
	o := &Orchestrator{busCtx: &types.BusContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentExplain,
				Scenario: types.ScenarioArchitectureExplain,
				Predicates: types.SemanticPredicates{
					IsDiagnosticQuestion: true,
				},
				DiagnosticProfile: types.DiagnosticIntentProfile{
					IsDiagnostic:        true,
					CurrentVersionCheck: true,
				},
				Buckets: []types.QuestionBucket{
					{Label: "日志现象", Index: 1},
					{Label: "当前源码", Index: 2},
				},
			},
		},
	}}

	if o.parallelExploreAllowsEarlyConvergence() {
		t.Fatal("explicit user buckets remain sibling handoff obligations even for diagnostic answers")
	}
}

func TestParallelExploreAllowsEarlyConvergence_InferredBucketsStayAdvisory(t *testing.T) {
	o := &Orchestrator{busCtx: &types.BusContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: "对比 Foo 和 Bar 在重试机制中的职责差异",
				Intent:     types.IntentExplain,
				Scenario:   types.ScenarioArchitectureExplain,
				Predicates: types.SemanticPredicates{
					IsCrossComponent: true,
				},
				AnalyzerHints: types.AnalyzerHints{
					Kind:              string(types.ReqMechanism),
					MentionedEntities: []string{"Foo", "Bar"},
				},
				SubTopics: []types.SubTopic{
					{Summary: "Foo", Entities: []string{"Foo"}},
					{Summary: "Bar", Entities: []string{"Bar"}},
				},
			},
		},
	}}

	if buckets := o.busCtx.AnalysisIR.RequestModel.QuestionStructure().Buckets; len(buckets) < 2 {
		t.Fatalf("fixture should exercise inferred bucket fallback, got %+v", buckets)
	}
	if !o.parallelExploreAllowsEarlyConvergence() {
		t.Fatal("system-inferred buckets are useful for rendering, but must not hard-block accepted closure convergence")
	}
}

func TestDispatchExploreWindowsParallel_PropagatesErrorWithoutConvergence(t *testing.T) {
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return nil, context.Canceled
		},
	})
	o := New(types.PipelineSettings{MaxParallelism: 2}, ar, sr, sar)
	o.busCtx = &types.BusContext{
		Mutable: types.NewMutableState("parallel explore cancellation"),
		Signals: types.ExecutionSignals{},
	}

	if _, err := o.dispatchExploreWindowsParallel([][]*types.TaskNode{
		{{ID: "a"}},
		{{ID: "b"}},
	}, nil, 2); err == nil {
		t.Fatal("non-converged parallel dispatch should still propagate worker errors")
	}
}

func TestDispatchExploreWindowsParallel_PreservesPartialOutputOnErrorWithoutConvergence(t *testing.T) {
	streamErr := errors.New("stream stalled")
	ev := types.EvidenceItem{
		ID:           "partial-buildAnalysisIR",
		Kind:         types.EvidenceDirect,
		Subject:      "buildAnalysisIR",
		Source:       "internal/agent/analyzer.go",
		LineStart:    1505,
		Scope:        types.ScopeLine,
		AnchorKind:   types.AnchorDefinition,
		AnchorSymbol: "buildAnalysisIR",
	}
	tr := types.ToolResult{ToolName: "emit_evidence", Success: true, Summary: "accepted evidence"}

	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			switch ctx.ExploreDispatchKey {
			case "partial":
				ctx.Mutable.AppendEvidence([]types.EvidenceItem{ev})
				ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
					UserQuestion:       "q",
					ToolResults:        []types.ToolResult{tr},
					EvidenceItems:      []types.EvidenceItem{ev},
					InvestigationNotes: []string{"accepted before stream error"},
				})
				return &agent.StageOutput{
					EvidenceItems: []types.EvidenceItem{ev},
					ToolResults:   []types.ToolResult{tr},
				}, streamErr
			case "ok":
				return &agent.StageOutput{
					MissingPiece:  types.MissingNone,
					SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: true},
				}, nil
			default:
				t.Fatalf("unexpected dispatch key %q", ctx.ExploreDispatchKey)
				return nil, nil
			}
		},
	})
	o := New(types.PipelineSettings{MaxParallelism: 2}, ar, sr, sar)
	o.busCtx = &types.BusContext{
		Mutable: types.NewMutableState("parallel partial preservation"),
		Signals: types.ExecutionSignals{},
	}

	out, err := o.dispatchExploreWindowsParallel([][]*types.TaskNode{
		{{ID: "partial"}},
		{{ID: "ok"}},
	}, nil, 2)
	if err == nil {
		t.Fatal("non-converged parallel dispatch should still return the worker error")
	}
	if out == nil {
		t.Fatal("partial StageOutput should be returned with the error")
	}
	if len(o.busCtx.EvidenceItems) != 1 || o.busCtx.EvidenceItems[0].ID != ev.ID {
		t.Fatalf("bus evidence = %+v, want preserved partial evidence", o.busCtx.EvidenceItems)
	}
	if len(o.busCtx.ToolResults) != 1 || o.busCtx.ToolResults[0].ToolName != "emit_evidence" {
		t.Fatalf("tool results = %+v, want preserved partial tool result", o.busCtx.ToolResults)
	}
	if emitted := o.busCtx.Mutable.EmittedEvidence(); len(emitted) != 1 || emitted[0].ID != ev.ID {
		t.Fatalf("mutable emitted evidence = %+v, want fork side-effect preserved", emitted)
	}
	if ta := o.busCtx.Mutable.TurnAArtifacts(); ta == nil || len(ta.EvidenceItems) != 1 || ta.EvidenceItems[0].ID != ev.ID {
		t.Fatalf("TurnAArtifacts = %+v, want fork handoff preserved", ta)
	}
}

func TestDispatchExploreWindowsParallel_PreservesConvergedPartialOutputOnError(t *testing.T) {
	streamErr := errors.New("post-completion stream stalled")
	doneStarted := make(chan struct{})
	ev := types.EvidenceItem{
		ID:           "converged-buildAnalysisIR",
		Kind:         types.EvidenceDirect,
		Subject:      "buildAnalysisIR",
		Source:       "internal/agent/analyzer.go",
		LineStart:    1505,
		Scope:        types.ScopeLine,
		AnchorKind:   types.AnchorDefinition,
		AnchorSymbol: "buildAnalysisIR",
	}

	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			switch ctx.ExploreDispatchKey {
			case "done":
				close(doneStarted)
				ctx.Mutable.AppendEvidence([]types.EvidenceItem{ev})
				ctx.Mutable.SetInvestigationComplete("closure accepted before stream error")
				return &agent.StageOutput{
					EvidenceItems: []types.EvidenceItem{ev},
					MissingPiece:  types.MissingNone,
					SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: true},
				}, streamErr
			case "slow":
				<-doneStarted
				<-ctx.Context().Done()
				return nil, ctx.Context().Err()
			default:
				t.Fatalf("unexpected dispatch key %q", ctx.ExploreDispatchKey)
				return nil, nil
			}
		},
	})
	o := New(types.PipelineSettings{MaxParallelism: 2}, ar, sr, sar)
	o.busCtx = &types.BusContext{
		Mutable: types.NewMutableState("parallel converged partial preservation"),
		Signals: types.ExecutionSignals{},
	}

	out, err := o.dispatchExploreWindowsParallel([][]*types.TaskNode{
		{{ID: "done"}},
		{{ID: "slow"}},
	}, nil, 2)
	if err == nil {
		t.Fatal("winning branch transport error should still propagate for retry policy")
	}
	if out == nil || out.MissingPiece != types.MissingNone {
		t.Fatalf("output = %+v, want preserved converged output", out)
	}
	if len(o.busCtx.EvidenceItems) != 1 || o.busCtx.EvidenceItems[0].ID != ev.ID {
		t.Fatalf("bus evidence = %+v, want converged partial evidence preserved", o.busCtx.EvidenceItems)
	}
	if !o.busCtx.Mutable.IsInvestigationComplete() {
		t.Fatal("fork completion marker should be merged even when the winning branch returns an error")
	}
}
