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

// reflectorStubAdapter is a minimal llm.Adapter for unit tests. Only
// implements Chat — every other method returns harmless defaults so
// the type satisfies the interface without dragging real provider
// state in.
type reflectorStubAdapter struct {
	resp llm.Response
	err  error
}

func (s *reflectorStubAdapter) Chat(_ context.Context, _ []llm.Message, _ []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	return s.resp, s.err
}
func (s *reflectorStubAdapter) ModelID() string                { return "stub" }
func (s *reflectorStubAdapter) MaxContextTokens() int          { return 128000 }
func (s *reflectorStubAdapter) MaxOutputTokens() int           { return 0 }
func (s *reflectorStubAdapter) RequestTimeout() time.Duration  { return 0 }
func (s *reflectorStubAdapter) RetryMaxAttempts() int          { return 1 }

// TestReflector_Disabled covers the nil-adapter path: NewReflector(nil)
// must return ("", nil) so clearForReplan falls back to heuristic
// hint without aborting the retry.
func TestReflector_Disabled(t *testing.T) {
	r := NewReflector(nil)
	got, err := r.Reflect(context.Background(), ReflectorInput{Attempt: 1})
	if err != nil {
		t.Errorf("nil adapter Reflect should return (\"\", nil); got err=%v", err)
	}
	if got != "" {
		t.Errorf("nil adapter Reflect should return empty critique; got %q", got)
	}
}

// TestReflector_HappyPath covers the canonical structured response:
// adapter returns one tool_call with both required fields populated;
// Reflect assembles them into the "Root cause: ... Next attempt: ..."
// critique string.
func TestReflector_HappyPath(t *testing.T) {
	stub := &reflectorStubAdapter{
		resp: llm.Response{
			ToolCalls: []llm.ToolCall{{
				Name: "emit_failure_critique",
				Params: json.RawMessage(`{
					"root_cause": "off-by-one in overflow check",
					"corrective_direction": "remove the per-iteration overflow guard; only check after multiplication"
				}`),
			}},
		},
	}
	r := NewReflector(stub)
	got, err := r.Reflect(context.Background(), ReflectorInput{
		Attempt:         1,
		OriginalRequest: "implement parse_trinary",
		PlanSummary:     "trinary parser",
		FailingTests: []ReflectorFailedTest{{
			Suite:       "trinary",
			AssertionID: "TestParseTrinary",
			Detail:      "ParseTrinary(\"0000201\") returned error \"overflow\"",
		}},
	})
	if err != nil {
		t.Fatalf("happy path Reflect err: %v", err)
	}
	if !strings.Contains(got, "Root cause:") || !strings.Contains(got, "Next attempt:") {
		t.Errorf("critique should contain both Root cause + Next attempt sections; got %q", got)
	}
}

// TestReflector_ChatErrorDegrades covers the retry-must-not-block
// invariant: when the LLM errors, Reflect returns ("", err) so the
// caller falls through to the heuristic hint.
func TestReflector_ChatErrorDegrades(t *testing.T) {
	stub := &reflectorStubAdapter{err: errors.New("provider timeout")}
	r := NewReflector(stub)
	got, err := r.Reflect(context.Background(), ReflectorInput{Attempt: 1, FailingTests: []ReflectorFailedTest{{Suite: "x", AssertionID: "y"}}})
	if err == nil {
		t.Errorf("chat error should surface as Reflect error; got nil")
	}
	if got != "" {
		t.Errorf("chat error should yield empty critique; got %q", got)
	}
}

// TestReflector_NoToolCallDegrades covers the malformed-output path:
// LLM returned content but no structured tool call. Reflect surfaces
// the error so the heuristic hint takes over.
func TestReflector_NoToolCallDegrades(t *testing.T) {
	stub := &reflectorStubAdapter{resp: llm.Response{Content: "I think the bug is..."}}
	r := NewReflector(stub)
	_, err := r.Reflect(context.Background(), ReflectorInput{Attempt: 1, FailingTests: []ReflectorFailedTest{{Suite: "x", AssertionID: "y"}}})
	if err == nil {
		t.Errorf("no-tool-call response should error; got nil")
	}
}

// TestReflector_PreserveWhatWorked covers the optional third field:
// when the LLM emits preserve_what_worked, the assembled critique
// includes a "Preserve:" suffix so the next planner knows what NOT
// to throw away.
func TestReflector_PreserveWhatWorked(t *testing.T) {
	stub := &reflectorStubAdapter{
		resp: llm.Response{ToolCalls: []llm.ToolCall{{
			Name: "emit_failure_critique",
			Params: json.RawMessage(`{
				"root_cause": "missing edge case for empty string",
				"corrective_direction": "add a guard at the top of the function",
				"preserve_what_worked": "the file targeting (modify on stub.py) was correct"
			}`),
		}}},
	}
	r := NewReflector(stub)
	got, err := r.Reflect(context.Background(), ReflectorInput{Attempt: 1, FailingTests: []ReflectorFailedTest{{Suite: "x", AssertionID: "y"}}})
	if err != nil {
		t.Fatalf("Reflect err: %v", err)
	}
	if !strings.Contains(got, "Preserve:") {
		t.Errorf("critique should include Preserve section when LLM provided preserve_what_worked; got %q", got)
	}
}
