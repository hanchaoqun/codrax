package orchestrator

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRunTaskGraph_FinalizerOnlyRetryReusesHypothesisVerdicts(t *testing.T) {
	ir := dagIR(types.AnswerContract{
		Language:    "en",
		MustInclude: []string{"SENTINEL_REQUIRES_FINALIZER_RETRY"},
	})
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

func TestRunTaskGraph_FinalizerOnlyRetryDoesNotReplayEmptyExtract(t *testing.T) {
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

	if extractorCalls != 1 {
		t.Fatalf("finalizer-only retry must not replay an already-successful empty extract; extractorCalls=%d", extractorCalls)
	}
	if finalizerCalls < 2 {
		t.Fatalf("finalizer should retry on the contract violation; finalizerCalls=%d", finalizerCalls)
	}
}

func TestRunTaskGraph_ForcedFinalizeReusesTurnBSlateAfterDispatchError(t *testing.T) {
	ir := dagIR(types.AnswerContract{Language: "en"})

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
