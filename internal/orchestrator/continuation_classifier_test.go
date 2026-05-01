package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// stubContinuationAdapter is a minimal llm.Adapter for unit tests.
type stubContinuationAdapter struct {
	resp llm.Response
	err  error
}

func (s *stubContinuationAdapter) Chat(_ context.Context, _ []llm.Message, _ []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	return s.resp, s.err
}
func (s *stubContinuationAdapter) ModelID() string                  { return "stub" }
func (s *stubContinuationAdapter) MaxContextTokens() int            { return 128000 }
func (s *stubContinuationAdapter) MaxOutputTokens() int             { return 0 }
func (s *stubContinuationAdapter) RequestTimeout() time.Duration    { return 0 }
func (s *stubContinuationAdapter) RetryMaxAttempts() int            { return 1 }

// TestContinuationClassifier_NilAdapterErrors pins commit 48
// graceful-degradation contract: a nil adapter errors so the
// caller falls back to the keyword heuristic.
func TestContinuationClassifier_NilAdapterErrors(t *testing.T) {
	c := NewContinuationClassifier(nil)
	_, err := c.Classify(context.Background(), "current", "prior")
	if err == nil {
		t.Errorf("nil adapter should error; got nil")
	}
}

// TestContinuationClassifier_EmptyPriorIsFresh pins the
// short-circuit: when prior is empty there's nothing to
// continue from. Returns (false, nil) without an LLM round-
// trip.
func TestContinuationClassifier_EmptyPriorIsFresh(t *testing.T) {
	stub := &stubContinuationAdapter{}
	c := NewContinuationClassifier(stub)
	got, err := c.Classify(context.Background(), "what is X?", "")
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if got {
		t.Errorf("empty prior should yield fresh; got continuation")
	}
}

// TestContinuationClassifier_HappyPathDecision covers both
// "continuation" and "fresh" branches via stub responses.
func TestContinuationClassifier_HappyPathDecision(t *testing.T) {
	for _, c := range []struct {
		decision string
		want     bool
	}{
		{"continuation", true},
		{"fresh", false},
	} {
		stub := &stubContinuationAdapter{
			resp: llm.Response{
				ToolCalls: []llm.ToolCall{{
					Name:   continuationClassifierTool.Name,
					Params: json.RawMessage(`{"decision": "` + c.decision + `", "reason": "test"}`),
				}},
			},
		}
		got, err := NewContinuationClassifier(stub).Classify(context.Background(), "x", "y")
		if err != nil {
			t.Fatalf("decision=%s: %v", c.decision, err)
		}
		if got != c.want {
			t.Errorf("decision=%s: want %v got %v", c.decision, c.want, got)
		}
	}
}

// TestContinuationClassifier_ChatErrorDegrades pins the
// fallback path: chat error → caller falls back to keyword
// heuristic via err return.
func TestContinuationClassifier_ChatErrorDegrades(t *testing.T) {
	stub := &stubContinuationAdapter{err: errors.New("boom")}
	_, err := NewContinuationClassifier(stub).Classify(context.Background(), "x", "y")
	if err == nil {
		t.Errorf("chat error should propagate; got nil")
	}
}

// TestOrchestratorPriorConvVisibleForStage_LLMTakesPriority
// pins the integration: when the orchestrator has an LLM
// classifier wired AND PolicyContinue is active, the LLM
// decision overrides the keyword heuristic.
func TestOrchestratorPriorConvVisibleForStage_LLMTakesPriority(t *testing.T) {
	// Build an objective with prior + current where the
	// keyword heuristic would say "fresh" (current names its
	// own subject "Foo" + no continuation prefix), but the
	// LLM stub returns "continuation".
	objective := "## Prior conversation\nWe were discussing config.\n\n" +
		types.ConversationCurrentMarker + "How is Foo defined?"
	stub := &stubContinuationAdapter{
		resp: llm.Response{
			ToolCalls: []llm.ToolCall{{
				Name:   continuationClassifierTool.Name,
				Params: json.RawMessage(`{"decision": "continuation", "reason": "stub"}`),
			}},
		},
	}
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.continuationClassifier = NewContinuationClassifier(stub)
	visible := o.orchestratorPriorConvVisibleForStage(types.PriorConvPolicyContinue,
		types.StageExplore, objective)
	if !visible {
		t.Errorf("LLM said continuation, prior should be visible; got hidden")
	}
}

// TestOrchestratorPriorConvVisibleForStage_NoClassifierDefaultsToFresh
// pins the simplified fallback (commit 48 update): when the LLM
// classifier is not wired, the policy defaults to fresh (false) —
// downstream stages don't see Prior. Pre-commit-48 there was a
// keyword-list fallback (types.IsContinuation), but operator
// feedback flagged it as dead code + red-line violation.
func TestOrchestratorPriorConvVisibleForStage_NoClassifierDefaultsToFresh(t *testing.T) {
	objective := "## Prior conversation\nold stuff\n\n" +
		types.ConversationCurrentMarker + "再详细说说 X" // would have triggered keyword path pre-commit-48
	o := New(types.PipelineSettings{}, nil, nil, nil)
	// continuationClassifier intentionally nil → defaults to fresh.
	visible := o.orchestratorPriorConvVisibleForStage(types.PriorConvPolicyContinue,
		types.StageExplore, objective)
	if visible {
		t.Errorf("no-classifier path should default to fresh (hidden); got visible")
	}
}
