package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
)

type stubSelfConsistencyAdapter struct {
	resp llm.Response
	err  error
}

func (s *stubSelfConsistencyAdapter) Chat(_ context.Context, _ []llm.Message, _ []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	return s.resp, s.err
}
func (s *stubSelfConsistencyAdapter) ModelID() string               { return "stub" }
func (s *stubSelfConsistencyAdapter) MaxContextTokens() int         { return 128000 }
func (s *stubSelfConsistencyAdapter) MaxOutputTokens() int          { return 0 }
func (s *stubSelfConsistencyAdapter) RequestTimeout() time.Duration { return 0 }
func (s *stubSelfConsistencyAdapter) RetryMaxAttempts() int         { return 1 }

func TestSelfConsistencyReviewer_NilAdapterIsNoOp(t *testing.T) {
	r := NewSelfConsistencyReviewer(nil)
	got, err := r.Review(context.Background(), SelfConsistencyInput{
		AnswerSummary: "foo", AnswerBody: "bar",
	})
	if err != nil {
		t.Errorf("nil adapter should not error; got %v", err)
	}
	if got != nil {
		t.Error("nil adapter should yield nil result")
	}
}

// TestSelfConsistencyReviewer_HappyPathConsistent pins the
// common case: reviewer says consistent=true, no violations.
func TestSelfConsistencyReviewer_HappyPathConsistent(t *testing.T) {
	resp := llm.Response{
		ToolCalls: []llm.ToolCall{{
			Name: selfConsistencyTool.Name,
			Params: json.RawMessage(`{
                "consistent": true,
                "confidence": 0.9,
                "reasoning": "summary and body align on all key claims"
            }`),
		}},
	}
	r := NewSelfConsistencyReviewer(&stubSelfConsistencyAdapter{resp: resp})
	got, err := r.Review(context.Background(), SelfConsistencyInput{
		OriginalRequest: "what is X",
		AnswerSummary:   "X is foo",
		AnswerBody:      "1. X = foo (file:1)",
	})
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if got == nil || !got.Consistent {
		t.Errorf("expected consistent=true; got %+v", got)
	}
	if len(got.Contradictions) != 0 {
		t.Errorf("consistent verdict should have no contradictions; got %v", got.Contradictions)
	}
}

// TestSelfConsistencyReviewer_ContradictionEmitted pins the s1a-
// style case: read↔write reversal between summary and body.
func TestSelfConsistencyReviewer_ContradictionEmitted(t *testing.T) {
	resp := llm.Response{
		ToolCalls: []llm.ToolCall{{
			Name: selfConsistencyTool.Name,
			Params: json.RawMessage(`{
                "consistent": false,
                "confidence": 0.9,
                "contradictions": [{
                    "topic": "read vs write mode check count",
                    "summary_claim": "write mode runs 9 checks, read mode runs 5",
                    "body_claim": "if !isWrite (read mode) adds checks 4-7, write skips them"
                }],
                "reasoning": "summary inverts the read/write count assignment"
            }`),
		}},
	}
	r := NewSelfConsistencyReviewer(&stubSelfConsistencyAdapter{resp: resp})
	got, err := r.Review(context.Background(), SelfConsistencyInput{
		OriginalRequest: "list checks",
		AnswerSummary:   "write mode runs 9 checks, read mode runs 5",
		AnswerBody:      "1. if !isWrite (read mode) adds checks 4-7, write skips them",
	})
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if got.Consistent {
		t.Error("expected consistent=false")
	}
	if len(got.Contradictions) != 1 {
		t.Fatalf("expected 1 contradiction; got %d", len(got.Contradictions))
	}
	c := got.Contradictions[0]
	if !strings.Contains(c.Topic, "read") || !strings.Contains(c.Topic, "write") {
		t.Errorf("topic missing read/write framing: %q", c.Topic)
	}
}

// TestSelfConsistencyReviewer_InconsistentNoContradictionsRejected
// pins the cross-check: emit declares inconsistent but lists no
// contradictions = malformed; caller should see error.
func TestSelfConsistencyReviewer_InconsistentNoContradictionsRejected(t *testing.T) {
	resp := llm.Response{
		ToolCalls: []llm.ToolCall{{
			Name: selfConsistencyTool.Name,
			Params: json.RawMessage(`{
                "consistent": false,
                "confidence": 0.9,
                "contradictions": []
            }`),
		}},
	}
	r := NewSelfConsistencyReviewer(&stubSelfConsistencyAdapter{resp: resp})
	_, err := r.Review(context.Background(), SelfConsistencyInput{
		AnswerSummary: "x", AnswerBody: "y",
	})
	if err == nil {
		t.Error("inconsistent + no contradictions should error")
	}
}

// TestSelfConsistencyReviewer_ChatErrorPropagates pins the
// graceful-degradation contract: LLM error is non-fatal but
// returned to caller for LearningFailure recording.
func TestSelfConsistencyReviewer_ChatErrorPropagates(t *testing.T) {
	r := NewSelfConsistencyReviewer(&stubSelfConsistencyAdapter{err: errors.New("boom")})
	_, err := r.Review(context.Background(), SelfConsistencyInput{
		AnswerSummary: "x", AnswerBody: "y",
	})
	if err == nil {
		t.Error("Chat error must propagate")
	}
}

// TestSelfConsistencyReviewer_NoToolCallErrors pins the strict
// schema: tool_choice=required + LLM returned no tool call =
// schema violation; caller treats as failure.
func TestSelfConsistencyReviewer_NoToolCallErrors(t *testing.T) {
	r := NewSelfConsistencyReviewer(&stubSelfConsistencyAdapter{resp: llm.Response{}})
	_, err := r.Review(context.Background(), SelfConsistencyInput{
		AnswerSummary: "x", AnswerBody: "y",
	})
	if err == nil {
		t.Error("missing tool call should error")
	}
}

// TestSelfConsistencyReviewer_OutOfRangeConfidenceRejected pins
// the schema-validity gate: confidence > 1 is an LLM bug; caller
// should see error rather than mis-compare against floor.
func TestSelfConsistencyReviewer_OutOfRangeConfidenceRejected(t *testing.T) {
	resp := llm.Response{
		ToolCalls: []llm.ToolCall{{
			Name: selfConsistencyTool.Name,
			Params: json.RawMessage(`{
                "consistent": true,
                "confidence": 1.5
            }`),
		}},
	}
	r := NewSelfConsistencyReviewer(&stubSelfConsistencyAdapter{resp: resp})
	_, err := r.Review(context.Background(), SelfConsistencyInput{
		AnswerSummary: "x", AnswerBody: "y",
	})
	if err == nil {
		t.Error("confidence > 1 should error")
	}
}
