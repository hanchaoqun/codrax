package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// reflectorStubAdapter is a minimal llm.Adapter for unit tests. Only
// implements Chat — every other method returns harmless defaults so
// the type satisfies the interface without dragging real provider
// state in.
type reflectorStubAdapter struct {
	resp        llm.Response
	err         error
	lastSystem  string
	lastUser    string
	lastTools   []llm.ToolSchema
}

func (s *reflectorStubAdapter) Chat(_ context.Context, msgs []llm.Message, tools []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	for _, m := range msgs {
		switch m.Role {
		case "system":
			s.lastSystem = m.Content
		case "user":
			s.lastUser = m.Content
		}
	}
	s.lastTools = tools
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

// TestReflector_HappyPath covers the canonical structured response
// after the Module G upgrade: adapter returns one
// emit_failure_observation tool call; Reflect assembles the
// observation into "Reviewer observation: ..." text.
func TestReflector_HappyPath(t *testing.T) {
	stub := &reflectorStubAdapter{
		resp: llm.Response{
			ToolCalls: []llm.ToolCall{{
				Name: "emit_failure_observation",
				Params: json.RawMessage(`{
					"observation": "the plan modified parser.go but the failing test test_overflow exercises the multiplication path which the plan left unchanged"
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
	if !strings.Contains(got, "Reviewer observation:") {
		t.Errorf("output should carry observation label; got %q", got)
	}
	if !strings.Contains(got, "the plan modified parser.go") {
		t.Errorf("output should carry observation text verbatim; got %q", got)
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

// TestReflector_UncertaintySurfaces covers the optional uncertainty
// field. When the reviewer says it's not sure about something, the
// planner should see that uncertainty alongside the observation.
func TestReflector_UncertaintySurfaces(t *testing.T) {
	stub := &reflectorStubAdapter{
		resp: llm.Response{ToolCalls: []llm.ToolCall{{
			Name: "emit_failure_observation",
			Params: json.RawMessage(`{
				"observation": "test_x failed with AssertionError on stub.py:42",
				"uncertainty": "I cannot tell from the input whether stub.py was previously passing or has always failed in this fixture"
			}`),
		}}},
	}
	r := NewReflector(stub)
	got, err := r.Reflect(context.Background(), ReflectorInput{Attempt: 1, FailingTests: []ReflectorFailedTest{{Suite: "x", AssertionID: "y"}}})
	if err != nil {
		t.Fatalf("Reflect err: %v", err)
	}
	if !strings.Contains(got, "Reviewer uncertainty:") {
		t.Errorf("output should include uncertainty section; got %q", got)
	}
}

// TestReflector_FullLedgerPropagatesToUserPrompt verifies Module G's
// load-bearing wiring: the iteration ledger is rendered into the
// reviewer's user message verbatim, so the reviewer can observe
// patterns across attempts. The system never pre-classifies the
// patterns; the reviewer reads the data.
func TestReflector_FullLedgerPropagatesToUserPrompt(t *testing.T) {
	stub := &reflectorStubAdapter{
		resp: llm.Response{ToolCalls: []llm.ToolCall{{
			Name:   "emit_failure_observation",
			Params: json.RawMessage(`{"observation": "ok"}`),
		}}},
	}
	r := NewReflector(stub)
	_, err := r.Reflect(context.Background(), ReflectorInput{
		Attempt: 3,
		IterationLedger: []types.IterationRecord{
			{Attempt: 1, PlanSummary: "first attempt summary", FailureSummary: "first failure stderr"},
			{Attempt: 2, PlanSummary: "second attempt summary", FailureSummary: "second failure stderr"},
		},
		FailingTests: []ReflectorFailedTest{{Suite: "x", AssertionID: "y", Detail: "raw stderr line"}},
	})
	if err != nil {
		t.Fatalf("Reflect err: %v", err)
	}
	// Ledger content propagates verbatim.
	for _, want := range []string{
		"first attempt summary", "first failure stderr",
		"second attempt summary", "second failure stderr",
		"## Iteration history",
	} {
		if !strings.Contains(stub.lastUser, want) {
			t.Errorf("user prompt should include %q; got:\n%s", want, stub.lastUser)
		}
	}
}

// TestReflector_SystemPromptHasNoPrescriptiveGuards locks the
// Module G discipline: the system prompt must NOT contain
// "DO NOT classify X as Y" rule lists. The reviewer is a model and
// is asked to OBSERVE; rule lists pre-encode the system's view of
// what's defensible, which is exactly the anti-pattern this batch
// removes.
func TestReflector_SystemPromptHasNoPrescriptiveGuards(t *testing.T) {
	stub := &reflectorStubAdapter{
		resp: llm.Response{ToolCalls: []llm.ToolCall{{
			Name:   "emit_failure_observation",
			Params: json.RawMessage(`{"observation": "ok"}`),
		}}},
	}
	r := NewReflector(stub)
	_, _ = r.Reflect(context.Background(), ReflectorInput{Attempt: 1, FailingTests: []ReflectorFailedTest{{Suite: "x", AssertionID: "y"}}})
	for _, banned := range []string{
		"DO NOT classify",
		"DO NOT speculate",
		"DO NOT blame",
		"Tests are authoritative", // pre-encodes a stance the reviewer should reach itself
	} {
		if strings.Contains(stub.lastSystem, banned) {
			t.Errorf("system prompt must not contain prescriptive guard %q; got:\n%s", banned, stub.lastSystem)
		}
	}
}
