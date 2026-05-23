package orchestrator

import (
	"context"
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
