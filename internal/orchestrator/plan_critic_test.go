package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// stubLLM is a minimal llm.Adapter used by the plan_critic tests.
// Returns a fixed response and records the most recent Chat call so
// tests can assert on the user message and tools shape.
type stubLLM struct {
	resp        llm.Response
	err         error
	lastUser    string
	lastSystem  string
	lastTools   []llm.ToolSchema
	lastOptions llm.ChatOptions
	callCount   int
}

func (s *stubLLM) Chat(_ context.Context, messages []llm.Message, tools []llm.ToolSchema, opts llm.ChatOptions) (llm.Response, error) {
	s.callCount++
	for _, m := range messages {
		switch m.Role {
		case "user":
			s.lastUser = m.Content
		case "system":
			s.lastSystem = m.Content
		}
	}
	s.lastTools = tools
	s.lastOptions = opts
	return s.resp, s.err
}

func (s *stubLLM) RequestTimeout() time.Duration { return 0 }
func (s *stubLLM) ModelID() string               { return "stub-model" }
func (s *stubLLM) MaxContextTokens() int         { return 128000 }
func (s *stubLLM) MaxOutputTokens() int          { return 4096 }
func (s *stubLLM) RetryMaxAttempts() int         { return 0 }

// TestPlanCritic_NilAdapterIsDisabled covers the no-op path: when
// adapter is nil, Review returns (\"\", nil) so the caller falls
// through silently.
func TestPlanCritic_NilAdapterIsDisabled(t *testing.T) {
	c := NewPlanCritic(nil)
	out, err := c.Review(context.Background(), PlanCriticInput{})
	if err != nil {
		t.Errorf("nil adapter must not produce err; got %v", err)
	}
	if !out.IsEmpty() {
		t.Errorf("nil adapter must return empty verdict; got %+v", out)
	}
}

// TestPlanCritic_HappyPath verifies the critic dispatches one
// Chat call with the right tool, parses the LLM response, and
// returns a formatted critique paragraph.
func TestPlanCritic_HappyPath(t *testing.T) {
	stub := &stubLLM{
		resp: llm.Response{
			ToolCalls: []llm.ToolCall{{
				Name:   planCriticTool.Name,
				Params: []byte(`{"risks":["plan changes auth.go but does not update auth_test.go","summary mentions migration but no .sql file in changes"],"confidence":"high"}`),
			}},
		},
	}
	c := &llmPlanCritic{adapter: stub}
	in := PlanCriticInput{
		RawRequest:  "add a login flow",
		TaskKind:    "feature",
		Scope:       "package",
		Summary:     "wire login through auth handler",
		PlanSummary: "modify auth.go to register a new handler and add migration 0001_users.sql",
		Changes: []PlanCriticChangeRef{
			{Path: "auth.go", Kind: "modify", Rationale: "add login endpoint"},
		},
	}
	out, err := c.Review(context.Background(), in)
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if stub.callCount != 1 {
		t.Errorf("expected 1 Chat call; got %d", stub.callCount)
	}
	if len(out.Risks) != 2 {
		t.Errorf("expected 2 risks; got %d (%+v)", len(out.Risks), out.Risks)
	}
	if !strings.Contains(out.Risks[0], "auth_test.go") {
		t.Errorf("first risk should mention auth_test.go; got %q", out.Risks[0])
	}
	if out.Confidence != "high" {
		t.Errorf("expected confidence=high; got %q", out.Confidence)
	}
	prose := AssemblePlanCritiqueProse(out)
	if !strings.Contains(prose, "auth_test.go") || !strings.Contains(prose, "(confidence: high)") {
		t.Errorf("rendered prose missing risk/confidence; got %q", prose)
	}
	// Tools list should carry exactly emit_plan_review.
	if len(stub.lastTools) != 1 || stub.lastTools[0].Name != "emit_plan_review" {
		t.Errorf("expected emit_plan_review tool; got %+v", stub.lastTools)
	}
	if stub.lastOptions.ToolChoice != "required" {
		t.Errorf("expected tool_choice=required; got %q", stub.lastOptions.ToolChoice)
	}
	// User message should mention the request and the changes.
	if !strings.Contains(stub.lastUser, "add a login flow") {
		t.Errorf("user message missing raw request; got %q", stub.lastUser)
	}
	if !strings.Contains(stub.lastUser, "auth.go") {
		t.Errorf("user message missing change path; got %q", stub.lastUser)
	}
}

// TestPlanCritic_EmptyRisksReturnsEmpty pins the "plan looks clean"
// path: zero risks → empty critique (caller skips storing).
func TestPlanCritic_EmptyRisksReturnsEmpty(t *testing.T) {
	stub := &stubLLM{
		resp: llm.Response{
			ToolCalls: []llm.ToolCall{{
				Name:   planCriticTool.Name,
				Params: []byte(`{"risks":[],"confidence":"high"}`),
			}},
		},
	}
	c := &llmPlanCritic{adapter: stub}
	out, err := c.Review(context.Background(), PlanCriticInput{
		RawRequest: "x", PlanSummary: "y",
		Changes: []PlanCriticChangeRef{{Path: "a.go", Kind: "create", Rationale: "z"}},
	})
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if !out.IsEmpty() {
		t.Errorf("empty risks should yield empty verdict; got %+v", out)
	}
}

// TestPlanCritic_ChatErrorPropagates verifies a Chat-level error
// flows up so the caller can log + degrade.
func TestPlanCritic_ChatErrorPropagates(t *testing.T) {
	stub := &stubLLM{err: errors.New("network blip")}
	c := &llmPlanCritic{adapter: stub}
	out, err := c.Review(context.Background(), PlanCriticInput{
		RawRequest: "x", PlanSummary: "y",
		Changes: []PlanCriticChangeRef{{Path: "a.go", Kind: "create"}},
	})
	if err == nil {
		t.Error("expected error to propagate; got nil")
	}
	if !out.IsEmpty() {
		t.Errorf("error path should return empty verdict; got %+v", out)
	}
}

// TestPlanCritic_NoToolCallReportsErr covers the LLM-misbehave
// path: response without a tool call.
func TestPlanCritic_NoToolCallReportsErr(t *testing.T) {
	stub := &stubLLM{resp: llm.Response{Content: "no tool"}}
	c := &llmPlanCritic{adapter: stub}
	_, err := c.Review(context.Background(), PlanCriticInput{
		RawRequest: "x", PlanSummary: "y",
		Changes: []PlanCriticChangeRef{{Path: "a.go"}},
	})
	if err == nil || !strings.Contains(err.Error(), "no tool_call") {
		t.Errorf("expected 'no tool_call' error; got %v", err)
	}
}

// TestBuildPlanCriticInput_FromBusContext pins the wiring that
// flows BusContext data into the critic's input — RawRequest from
// Mutable.Objective, IR fields from WriteAnalysisIR, plan changes
// from ChangePlan.
func TestBuildPlanCriticInput_FromBusContext(t *testing.T) {
	mu := types.NewMutableState("user wants a quiet flag")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{
		Request: types.WriteRequestModel{
			Task: types.WriteTask{
				Kind:    types.WriteTaskFeature,
				Scope:   types.ScopeMicro,
				Summary: "add --quiet flag",
			},
			ExpectedOutcomes: []string{"INFO logs suppressed when --quiet is set"},
		},
	})
	mu.SetChangePlan(&types.ChangePlan{
		Summary: "modify cmd/root.go to register --quiet flag",
		Changes: []types.FileChange{
			{Path: "cmd/root.go", Kind: "modify", Rationale: "add flag parsing"},
		},
	})
	bus := &types.BusContext{Mutable: mu}
	in := buildPlanCriticInput(bus)
	if in.RawRequest != "user wants a quiet flag" {
		t.Errorf("RawRequest = %q", in.RawRequest)
	}
	if in.TaskKind != "feature" || in.Scope != "micro" {
		t.Errorf("Task fields drift: kind=%q scope=%q", in.TaskKind, in.Scope)
	}
	if in.Summary != "add --quiet flag" {
		t.Errorf("Summary = %q", in.Summary)
	}
	if len(in.ExpectedOutcomes) != 1 {
		t.Errorf("ExpectedOutcomes len = %d", len(in.ExpectedOutcomes))
	}
	if in.PlanSummary != "modify cmd/root.go to register --quiet flag" {
		t.Errorf("PlanSummary = %q", in.PlanSummary)
	}
	if len(in.Changes) != 1 || in.Changes[0].Path != "cmd/root.go" {
		t.Errorf("Changes = %+v", in.Changes)
	}
}

// TestBuildPlanCriticInput_NilSafe pins the defensive paths.
func TestBuildPlanCriticInput_NilSafe(t *testing.T) {
	if got := buildPlanCriticInput(nil); got.RawRequest != "" {
		t.Errorf("nil bus should yield empty input; got %+v", got)
	}
	bus := &types.BusContext{}
	if got := buildPlanCriticInput(bus); got.RawRequest != "" {
		t.Errorf("nil Mutable should yield empty RawRequest; got %+v", got)
	}
}

// TestMutableState_PlanCritique covers the accessor pair.
func TestMutableState_PlanCritique(t *testing.T) {
	mu := types.NewMutableState("x")
	if got := mu.PlanCritique(); got != "" {
		t.Errorf("zero-value PlanCritique should be empty; got %q", got)
	}
	mu.SetPlanCritique("- risk one\n(confidence: high)")
	if got := mu.PlanCritique(); got != "- risk one\n(confidence: high)" {
		t.Errorf("PlanCritique round-trip drift; got %q", got)
	}
}
