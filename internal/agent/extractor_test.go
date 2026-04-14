package agent

// P2.1 Session 1 Phase 7 — extractorEvaluator skeleton tests.
//
// These tests pin the SKELETON behavior so Session 2 can extend each
// method without accidentally breaking the contract that Phase 5's
// orchestrator dispatch hook relies on.
//
// Phase 7 deliberately keeps the surface minimal: 4 tests, one per
// Evaluator method. Session 2 will add cardinality / drain / retry
// tests as it implements those branches.

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestExtractor_BuildInitialPromptMentionsAllowedTools(t *testing.T) {
	// The skeleton prompt MUST list the three emit_* tools and MUST
	// forbid read_file / grep / repo_map. This is the structural
	// contract Session 2 inherits — even when the prompt body is
	// rewritten, the allow/forbid list stays the same shape.
	e := &extractorEvaluator{}
	ctx := &types.AgentContext{CurrentTask: "what does Foo return?"}
	prompt := e.BuildInitialPrompt(ctx, nil)

	allowed := []string{"emit_evidence", "emit_answer_symbol", "emit_hypothesis_verdict"}
	for _, tool := range allowed {
		if !contains(prompt, tool) {
			t.Errorf("prompt must mention allowed tool %q", tool)
		}
	}
	forbidden := []string{"read_file", "grep", "repo_map"}
	for _, tool := range forbidden {
		if !contains(prompt, tool) {
			t.Errorf("prompt must explicitly forbid %q so the LLM does not attempt it", tool)
		}
	}
	if !contains(prompt, "what does Foo return?") {
		t.Error("prompt should echo the user question")
	}
}

func TestExtractor_BuildInitialPromptHandlesNilCtx(t *testing.T) {
	// Defensive: Session 2 wiring may call BuildInitialPrompt before
	// AgentContext is fully populated. The skeleton must not panic.
	e := &extractorEvaluator{}
	prompt := e.BuildInitialPrompt(nil, nil)
	if prompt == "" {
		t.Error("nil ctx must still produce a non-empty prompt")
	}
}

func TestExtractor_ShouldStopFiresAfterOneIteration(t *testing.T) {
	// Turn B is fundamentally one-shot — there is no soft-stop, no
	// continuation, no ReAct loop. ShouldStop must return true at
	// iteration >= 1 so BaseAgent.Execute exits cleanly after the
	// first LLM response is produced.
	e := &extractorEvaluator{}
	if e.ShouldStop(llm.Response{}, 0) {
		t.Error("must NOT stop at iteration 0 (we need at least one LLM response)")
	}
	if !e.ShouldStop(llm.Response{}, 1) {
		t.Error("must stop at iteration 1 (one-shot policy)")
	}
	if !e.ShouldStop(llm.Response{}, 5) {
		t.Error("must stop at any iteration >= 1")
	}
}

func TestExtractor_ParseOutputDrainsEmittedBuffers(t *testing.T) {
	// The skeleton's job is to drain MutableState's emit_evidence and
	// emit_answer_symbol buffers into StageOutput so the
	// orchestrator's applyStageOutput merge sees whatever Turn B
	// produced. Today the buffers will normally be empty (no prompt
	// teaches the LLM to call the tools yet), but the drain must
	// work end-to-end so Session 2 can populate them and immediately
	// see them flow through.
	mu := types.NewMutableState(types.TaskList{})
	mu.AppendEvidence([]types.EvidenceItem{
		{ID: "ev1", Kind: types.EvidenceDirect, Source: "a.go", LineStart: 5},
	})
	mu.AppendEmittedAnswerSymbols([]types.AnswerSymbol{
		{Name: "Foo", File: "a.go", Line: 5, Kind: "function"},
	})
	ctx := &types.AgentContext{
		CurrentTask: "test",
		Mutable:     mu,
	}

	e := &extractorEvaluator{}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput: %v", err)
	}
	if len(out.EvidenceItems) != 1 || out.EvidenceItems[0].ID != "ev1" {
		t.Errorf("EvidenceItems not drained: %+v", out.EvidenceItems)
	}
	if len(out.AnswerSymbols) != 1 || out.AnswerSymbols[0].Name != "Foo" {
		t.Errorf("AnswerSymbols not drained: %+v", out.AnswerSymbols)
	}
}

func TestExtractor_ParseOutputHandlesNilMutable(t *testing.T) {
	// Defensive: sub-agent dispatch passes a context with Mutable nil.
	// The extractor is not a sub-agent target today, but the symmetry
	// with emit_evidence's "no Mutable → fail-safe" is worth pinning.
	e := &extractorEvaluator{}
	out, err := e.ParseOutput(&types.AgentContext{}, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput should not error on nil Mutable, got: %v", err)
	}
	if out == nil {
		t.Fatal("ParseOutput must return non-nil StageOutput even on nil Mutable")
	}
	if len(out.EvidenceItems) != 0 || len(out.AnswerSymbols) != 0 {
		t.Errorf("nil Mutable must produce empty slices, got: %+v", out)
	}
}

func TestExtractor_DetermineMissingPieceAlwaysNone(t *testing.T) {
	// Turn B never triggers a backtrack — the contract checker
	// downstream of finalize owns retry decisions. Returning
	// MissingNone keeps the extractor out of the orchestrator's
	// stage-routing branch.
	e := &extractorEvaluator{}
	if got := e.DetermineMissingPiece(nil, &StageOutput{}); got != types.MissingNone {
		t.Errorf("got %v, want MissingNone", got)
	}
}

func TestExtractor_RegisteredAsAgent(t *testing.T) {
	// The factory must produce something that satisfies the Agent
	// interface so registry.go can store it. This catches a typo in
	// NewExtractorAgent's return shape immediately, before any
	// orchestrator-level integration test runs.
	deps := &Dependencies{LLM: nil, Tools: nil, MaxIterations: 1}
	a := NewExtractorAgent(deps)
	if a == nil {
		t.Fatal("NewExtractorAgent returned nil")
	}
	if a.Name() != types.AgentExtractor {
		t.Errorf("name = %q, want %q", a.Name(), types.AgentExtractor)
	}
}

// (uses the package-level `contains` helper from explorer_test.go)
