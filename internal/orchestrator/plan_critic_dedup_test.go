package orchestrator

import (
	"context"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
)

// TestPlanCritic_DedupSameInput pins commit 15 #5: when Review is
// called twice with identical input, the second call returns the
// cached critique without dispatching a Chat call. Spares the
// LLM cost on verify→plan retry iterations where the planner
// re-emits a structurally identical plan.
func TestPlanCritic_DedupSameInput(t *testing.T) {
	stub := &stubLLM{
		resp: llm.Response{
			ToolCalls: []llm.ToolCall{{
				Name:   planCriticTool.Name,
				Params: []byte(`{"risks":["one risk"],"confidence":"high"}`),
			}},
		},
	}
	c := &llmPlanCritic{adapter: stub}
	in := PlanCriticInput{
		RawRequest:  "user request",
		PlanSummary: "summary",
		Changes: []PlanCriticChangeRef{
			{Path: "a.go", Kind: "modify", Rationale: "fix bug"},
		},
	}
	out1, err := c.Review(context.Background(), in)
	if err != nil {
		t.Fatalf("first Review: %v", err)
	}
	if stub.callCount != 1 {
		t.Errorf("first call should dispatch; callCount=%d", stub.callCount)
	}
	out2, err := c.Review(context.Background(), in)
	if err != nil {
		t.Fatalf("second Review: %v", err)
	}
	if stub.callCount != 1 {
		t.Errorf("second call with identical input should hit cache; callCount=%d (want 1)", stub.callCount)
	}
	if out1 != out2 {
		t.Errorf("cached critique drift: first=%q second=%q", out1, out2)
	}
}

// TestPlanCritic_DedupDifferentInputDispatches pins the negative:
// different inputs (different plan summary, different changes)
// produce a fresh dispatch.
func TestPlanCritic_DedupDifferentInputDispatches(t *testing.T) {
	stub := &stubLLM{
		resp: llm.Response{
			ToolCalls: []llm.ToolCall{{
				Name:   planCriticTool.Name,
				Params: []byte(`{"risks":["one risk"],"confidence":"high"}`),
			}},
		},
	}
	c := &llmPlanCritic{adapter: stub}
	in1 := PlanCriticInput{RawRequest: "x", PlanSummary: "first plan", Changes: []PlanCriticChangeRef{{Path: "a.go", Kind: "modify"}}}
	in2 := PlanCriticInput{RawRequest: "x", PlanSummary: "second plan", Changes: []PlanCriticChangeRef{{Path: "a.go", Kind: "modify"}}}
	if _, err := c.Review(context.Background(), in1); err != nil {
		t.Fatalf("Review in1: %v", err)
	}
	if _, err := c.Review(context.Background(), in2); err != nil {
		t.Fatalf("Review in2: %v", err)
	}
	if stub.callCount != 2 {
		t.Errorf("different inputs should each dispatch; callCount=%d (want 2)", stub.callCount)
	}
}

// TestPlanCritic_DedupCachesEmptyOutput pins that cache hits
// preserve the "no risks" answer too — repeating the same plan
// must not blow up to "no result, fall through to fresh
// dispatch" on subsequent calls.
func TestPlanCritic_DedupCachesEmptyOutput(t *testing.T) {
	stub := &stubLLM{
		resp: llm.Response{
			ToolCalls: []llm.ToolCall{{
				Name:   planCriticTool.Name,
				Params: []byte(`{"risks":[],"confidence":"high"}`),
			}},
		},
	}
	c := &llmPlanCritic{adapter: stub}
	in := PlanCriticInput{RawRequest: "x", PlanSummary: "y", Changes: []PlanCriticChangeRef{{Path: "a.go"}}}
	out1, _ := c.Review(context.Background(), in)
	out2, _ := c.Review(context.Background(), in)
	if out1 != "" || out2 != "" {
		t.Errorf("expected empty critique both calls; got out1=%q out2=%q", out1, out2)
	}
	if stub.callCount != 1 {
		t.Errorf("expected 1 dispatch (cache hit on second); callCount=%d", stub.callCount)
	}
}
