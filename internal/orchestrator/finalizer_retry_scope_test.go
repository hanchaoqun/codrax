package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRunTaskGraph_FinalizerOnlyRetryReusesHypothesisVerdicts(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, []string{string(types.ViolMustInclude)})

	ir := dagIR(types.AnswerContract{
		Language:    "en",
		MustInclude: []string{"SENTINEL_REQUIRES_FINALIZER_RETRY"},
	})
	requireExtractEnumerationSlate(ir)
	ir.TaskGraph.ExecutionPolicy.RetryBudget = 2
	ir.HypothesisSet = []types.Hypothesis{{
		ID:        "h1",
		Statement: "typed Turn-B verdict is available",
		Status:    types.HypUnknown,
	}}

	var extractorCalls, finalizerCalls int
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{
				MissingPiece: types.MissingFacts,
				EvidenceItems: []types.EvidenceItem{{
					ID:           "ev",
					Source:       "src.go",
					LineStart:    1,
					AnchorKind:   types.AnchorDefinition,
					AnchorSymbol: "Source",
				}},
			}, nil
		},
		types.AgentExtractor: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			extractorCalls++
			ctx.Mutable.AppendEmittedHypothesisVerdicts([]types.HypothesisVerdict{{
				HypothesisID: "h1",
				Status:       types.HypConfirmed,
				Rationale:    "confirmed from typed evidence",
				Citation:     "src.go:1",
			}})
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
		types.AgentFinalizer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			finalizerCalls++
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "answer body without the sentinel",
			}, nil
		},
	}

	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)

	done := make(chan struct{})
	go func() {
		_, _ = o.Run("explain X", "/tmp/repo", "main")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not terminate within 5s")
	}

	if extractorCalls != 1 {
		t.Fatalf("finalizer-only retry must reuse typed Turn-B verdicts instead of returning to extract; extractorCalls=%d", extractorCalls)
	}
	if finalizerCalls < 2 {
		t.Fatalf("finalizer should retry on the contract violation; finalizerCalls=%d", finalizerCalls)
	}
}

func TestRunTaskGraph_FinalizerOnlyRetryDoesNotDispatchEmptyExtract(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, []string{string(types.ViolMustInclude)})

	ir := dagIR(types.AnswerContract{
		Language:    "en",
		MustInclude: []string{"SENTINEL_REQUIRES_FINALIZER_RETRY"},
	})
	ir.TaskGraph.ExecutionPolicy.RetryBudget = 2

	var extractorCalls, finalizerCalls int
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		types.AgentExtractor: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			extractorCalls++
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
		types.AgentFinalizer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			finalizerCalls++
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "answer body without the sentinel",
			}, nil
		},
	}

	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)

	done := make(chan struct{})
	go func() {
		_, _ = o.Run("explain X", "/tmp/repo", "main")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not terminate within 5s")
	}

	if extractorCalls != 0 {
		t.Fatalf("finalizer-only retry must not dispatch an empty extract when no typed extract work is pending; extractorCalls=%d", extractorCalls)
	}
	if finalizerCalls < 2 {
		t.Fatalf("finalizer should retry on the contract violation; finalizerCalls=%d", finalizerCalls)
	}
}

func TestRunTaskGraph_ForcedFinalizeReusesTurnBSlateAfterDispatchError(t *testing.T) {
	ir := dagIR(types.AnswerContract{Language: "en"})
	requireExtractAnswerSymbols(ir)

	var extractorCalls, finalizerCalls int
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{
				MissingPiece: types.MissingFacts,
				EvidenceItems: []types.EvidenceItem{{
					ID:           "ev",
					Source:       "src.go",
					LineStart:    1,
					AnchorKind:   types.AnchorDefinition,
					AnchorSymbol: "Source",
				}},
			}, nil
		},
		types.AgentExtractor: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			extractorCalls++
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				AnswerSymbols: []types.AnswerSymbol{{
					Name:      "Source",
					File:      "src.go",
					Line:      1,
					Rationale: "typed Turn-B slate",
				}},
				AnswerSymbolCompleteness: types.CompletenessLowerBound,
			}, nil
		},
		types.AgentFinalizer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			finalizerCalls++
			if finalizerCalls == 1 {
				return nil, errors.New("provider closed connection after finalizer prompt")
			}
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "forced finalizer answer mentions Source at src.go:1",
			}, nil
		},
	}

	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)
	o.SetForceFinalizeAttempts(1)

	bus, err := o.Run("explain X", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if extractorCalls != 1 {
		t.Fatalf("forced-finalize escape must not replay extract when typed Turn-B slate is already present; extractorCalls=%d", extractorCalls)
	}
	if finalizerCalls != 2 {
		t.Fatalf("finalizer calls = %d, want initial failure + forced-finalize success", finalizerCalls)
	}
	if got := bus.Mutable.Result(); !strings.Contains(got, "Source") {
		t.Fatalf("forced final answer was not recorded: %q", got)
	}
}

func TestRunTaskGraph_FinalizerNoVisibleOutputUsesDegradedTurnBSlate(t *testing.T) {
	ir := dagIR(types.AnswerContract{Language: "zh"})
	requireExtractAnswerSymbols(ir)
	ir.TaskGraph.ExecutionPolicy.RetryBudget = 0

	var extractorCalls, finalizerCalls int
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{
				MissingPiece: types.MissingFacts,
				EvidenceItems: []types.EvidenceItem{{
					ID:              "ev-source",
					Source:          "src.go",
					LineStart:       7,
					Summary:         "Source explains the answer",
					GroundingStatus: types.GroundingGrounded,
				}},
			}, nil
		},
		types.AgentExtractor: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			extractorCalls++
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				AnswerSymbols: []types.AnswerSymbol{{
					Name:      "Source",
					File:      "src.go",
					Line:      7,
					Rationale: "typed Turn-B slate survived finalizer transport failure",
				}},
				AnswerSymbolCompleteness: types.CompletenessLowerBound,
			}, nil
		},
		types.AgentFinalizer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			finalizerCalls++
			return nil, &llm.StreamNoVisibleOutputTimeoutError{
				IdleFor: 4 * time.Minute,
				Cause:   context.Canceled,
			}
		},
	}

	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)
	o.SetForceFinalizeAttempts(3)

	bus, err := o.Run("解释 Source", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if extractorCalls != 1 {
		t.Fatalf("extractor calls = %d, want 1", extractorCalls)
	}
	if finalizerCalls != 1 {
		t.Fatalf("no-visible-output finalizer fallback must not retry/force-finalize; finalizerCalls=%d", finalizerCalls)
	}
	if bus.TaskState.LastError != "" {
		t.Fatalf("LastError = %q, want empty after degraded fallback", bus.TaskState.LastError)
	}
	got := bus.Mutable.Result()
	for _, want := range []string{"降级答案", "Source", "src.go:7", "最终成文模型"} {
		if !strings.Contains(got, want) {
			t.Fatalf("degraded fallback answer missing %q:\n%s", want, got)
		}
	}
}
