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

type stubAnswerAdapter struct {
	resp llm.Response
	err  error
}

func (s *stubAnswerAdapter) Chat(_ context.Context, _ []llm.Message, _ []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	return s.resp, s.err
}
func (s *stubAnswerAdapter) ModelID() string               { return "stub" }
func (s *stubAnswerAdapter) MaxContextTokens() int         { return 128000 }
func (s *stubAnswerAdapter) MaxOutputTokens() int          { return 0 }
func (s *stubAnswerAdapter) RequestTimeout() time.Duration { return 0 }
func (s *stubAnswerAdapter) RetryMaxAttempts() int         { return 1 }

func TestAnswerReviewer_NilAdapterReturnsNoOp(t *testing.T) {
	r := NewAnswerReviewer(nil)
	got, err := r.ReviewAnswer(context.Background(), AnswerReviewerInput{
		OriginalRequest: "x",
	})
	if err != nil {
		t.Errorf("nil adapter should not error; got %v", err)
	}
	if got != nil {
		t.Error("nil adapter should yield nil pattern")
	}
}

func TestAnswerReviewer_HappyPath_EmitsPattern(t *testing.T) {
	resp := llm.Response{
		ToolCalls: []llm.ToolCall{{
			Name: answerPatternTool.Name,
			Params: json.RawMessage(`{
                "name": "enumeration counts interfaces alpha",
                "description": "Past Runs counted interfaces when the user wanted implementations; the trap recurs because handler types both end in -er.",
                "trigger": "when the user asks how many of an -er type",
                "confidence": 0.8
            }`),
		}},
	}
	r := NewAnswerReviewer(&stubAnswerAdapter{resp: resp})
	pattern, err := r.ReviewAnswer(context.Background(), AnswerReviewerInput{
		OriginalRequest: "how many handlers",
		FinalAnswer:     "There are 12 handler interfaces.",
		Scenario:        "enumerate",
		Shape:           "list",
		RetryEvents: []AnswerRetryEvent{
			{Stage: "analyze", Reason: "shape mismatch"},
		},
	})
	if err != nil {
		t.Fatalf("ReviewAnswer: %v", err)
	}
	if pattern == nil {
		t.Fatal("expected pattern; got nil")
	}
	if !strings.Contains(pattern.Name, "enumeration") {
		t.Errorf("pattern name unexpected: %q", pattern.Name)
	}
	if pattern.Confidence < 0.5 {
		t.Errorf("pattern confidence below floor: %.2f", pattern.Confidence)
	}
}

func TestAnswerReviewer_NoToolCallReturnsNil(t *testing.T) {
	// LLM declined to emit — that's the legitimate skip case, NOT an error.
	r := NewAnswerReviewer(&stubAnswerAdapter{resp: llm.Response{}})
	pattern, err := r.ReviewAnswer(context.Background(), AnswerReviewerInput{
		OriginalRequest: "x",
		FinalAnswer:     "y",
	})
	if err != nil {
		t.Errorf("no-tool-call should not error: %v", err)
	}
	if pattern != nil {
		t.Error("no tool call should yield nil pattern")
	}
}

func TestAnswerReviewer_ChatErrorPropagates(t *testing.T) {
	r := NewAnswerReviewer(&stubAnswerAdapter{err: errors.New("boom")})
	pattern, err := r.ReviewAnswer(context.Background(), AnswerReviewerInput{
		OriginalRequest: "x",
		FinalAnswer:     "y",
	})
	if err == nil {
		t.Error("chat error should propagate")
	}
	if pattern != nil {
		t.Error("error path should yield nil pattern")
	}
}

func TestAnswerReviewer_LowConfidenceRejected(t *testing.T) {
	resp := llm.Response{
		ToolCalls: []llm.ToolCall{{
			Name: answerPatternTool.Name,
			Params: json.RawMessage(`{
                "name": "low confidence pattern alpha beta",
                "description": "abstract description that is over thirty characters long for validation",
                "trigger": "fires when X meets Y precondition",
                "confidence": 0.3
            }`),
		}},
	}
	r := NewAnswerReviewer(&stubAnswerAdapter{resp: resp})
	pattern, err := r.ReviewAnswer(context.Background(), AnswerReviewerInput{
		OriginalRequest: "x",
		FinalAnswer:     "y",
	})
	if err == nil {
		t.Error("low-confidence emit should error")
	}
	if pattern != nil {
		t.Error("low-confidence emit should yield nil pattern")
	}
}

func TestAnswerReviewer_NonAsciiNameRejected(t *testing.T) {
	resp := llm.Response{
		ToolCalls: []llm.ToolCall{{
			Name: answerPatternTool.Name,
			Params: json.RawMessage(`{
                "name": "中文名字 should be rejected",
                "description": "abstract description that is over thirty characters long for validation",
                "trigger": "fires when X meets Y precondition",
                "confidence": 0.7
            }`),
		}},
	}
	r := NewAnswerReviewer(&stubAnswerAdapter{resp: resp})
	pattern, err := r.ReviewAnswer(context.Background(), AnswerReviewerInput{
		OriginalRequest: "x",
		FinalAnswer:     "y",
	})
	if err == nil {
		t.Error("non-ASCII name should error")
	}
	if pattern != nil {
		t.Error("non-ASCII name should yield nil pattern")
	}
}

func TestRenderAnswerReviewerUserMessage_TruncatesLargeAnswer(t *testing.T) {
	bigAnswer := strings.Repeat("a", 5000)
	rendered := renderAnswerReviewerUserMessage(AnswerReviewerInput{
		OriginalRequest: "q",
		FinalAnswer:     bigAnswer,
	})
	if !strings.Contains(rendered, "(truncated)") {
		t.Error("expected (truncated) marker in rendered prompt for >4KB answer")
	}
	if len(rendered) > 8000 { // sanity: 4KB cap + headers + ellipsis
		t.Errorf("rendered prompt unexpectedly large: %d bytes", len(rendered))
	}
}
