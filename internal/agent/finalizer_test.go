package agent

import (
	"strings"
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

// TestFinalizerPrompt_L02SymbolsTriggerTranslationMode verifies that a
// populated AnswerSymbols list switches the finalizer into translation
// mode: the prompt must contain the "Translation mode" header and an
// explicit instruction that the LLM must list exactly those symbols.
// This is the L0-2 headline behaviour.
func TestFinalizerPrompt_L02SymbolsTriggerTranslationMode(t *testing.T) {
	e := &finalizerEvaluator{}
	ctx := &types.AgentContext{
		Stage: types.StageFinalize,
		AnswerSymbols: []types.AnswerSymbol{
			{Name: "Explorer", File: "internal/agent/explorer.go", Line: 48, Kind: "registration"},
			{Name: "SubExplorer", File: "internal/agent/sub_explorer.go", Line: 12, Kind: "registration"},
		},
		CurrentTaskAnswerShape: "list_of_symbols",
	}
	prompt := e.BuildInitialPrompt(ctx, nil)

	if !strings.Contains(prompt, "Translation mode") {
		t.Error("prompt should contain 'Translation mode' header when AnswerSymbols is populated")
	}
	if !strings.Contains(prompt, "EXACTLY") {
		t.Error("prompt should stress 'EXACTLY' to force subset discipline")
	}
	// The legacy shape-constraint block must NOT appear — translation
	// mode is terminal; it returns before the shape switch.
	if strings.Contains(prompt, "Hard constraint: list_of_symbols answer") {
		t.Error("translation mode should bypass the legacy shape constraint block")
	}
}

// TestFinalizerPrompt_EmptySymbolsFallsBackToShape verifies that an
// empty AnswerSymbols list leaves the legacy shape-constraint path
// active — important for kinds (mechanism, enumeration) where L0-2
// is intentionally a no-op.
func TestFinalizerPrompt_EmptySymbolsFallsBackToShape(t *testing.T) {
	e := &finalizerEvaluator{}
	ctx := &types.AgentContext{
		Stage:                  types.StageFinalize,
		CurrentTaskAnswerShape: "step_list",
		// No AnswerSymbols set.
	}
	prompt := e.BuildInitialPrompt(ctx, nil)

	if strings.Contains(prompt, "Translation mode") {
		t.Error("translation mode should NOT appear when AnswerSymbols is empty")
	}
	// The legacy step_list shape block must still be emitted.
	if !strings.Contains(prompt, "step_list") {
		t.Error("empty AnswerSymbols + step_list shape should produce the legacy shape constraint")
	}
}
