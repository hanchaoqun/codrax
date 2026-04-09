package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestFinalizerParseOutputPopulatesFinalAnswer locks in the contract
// that finalizerEvaluator copies the last assistant message into
// StageOutput.FinalAnswer so the orchestrator (and main.go) can render
// it without re-parsing the Data JSON blob.
func TestFinalizerParseOutputPopulatesFinalAnswer(t *testing.T) {
	e := &finalizerEvaluator{}
	answer := "The project is a 5-layer multi-agent AI system."
	msgs := []llm.Message{
		{Role: "assistant", Content: answer},
	}

	out, err := e.ParseOutput(&types.AgentContext{Stage: types.StageFinalize}, msgs, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.FinalAnswer != answer {
		t.Errorf("FinalAnswer = %q, want %q", out.FinalAnswer, answer)
	}
	if len(out.Data) == 0 {
		t.Error("Data should still carry the JSON-wrapped answer for downstream consumers")
	}
}

func TestFinalizerParseOutputEmptyMessages(t *testing.T) {
	e := &finalizerEvaluator{}
	out, err := e.ParseOutput(&types.AgentContext{Stage: types.StageFinalize}, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.FinalAnswer != "" {
		t.Errorf("FinalAnswer for empty messages = %q, want empty", out.FinalAnswer)
	}
}
