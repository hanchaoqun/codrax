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
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Shape: "list_of_symbols"},
			},
		},
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
		Stage: types.StageFinalize,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Shape: "step_list"},
			},
		},
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

// TestFinalizerPrompt_StepListEnumeratesConditionalBranches verifies
// that the step_list Hard constraint contains the Plan D clause
// telling the LLM to enumerate distinct [CONDITIONAL] branches as
// separate steps rather than collapsing them into a narrative.
// See memory/project_fake_green_audit_2026_04_14.md Pattern 1 and
// memory/project_df3_push_strategy_enumeration_gap.md direction 3.
func TestFinalizerPrompt_StepListEnumeratesConditionalBranches(t *testing.T) {
	e := &finalizerEvaluator{}
	ctx := &types.AgentContext{
		Stage: types.StageFinalize,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Shape: "step_list"},
			},
		},
	}
	prompt := e.BuildInitialPrompt(ctx, nil)

	// Structural assertion: the prompt must acknowledge CONDITIONAL
	// branches as a distinct enumeration unit. The exact wording is
	// free to evolve; the test checks for the two load-bearing
	// terms — `[CONDITIONAL]` (the tag name) and `ALTERNATIVE
	// BRANCHES` (the contract).
	if !strings.Contains(prompt, "[CONDITIONAL]") {
		t.Error("step_list prompt should mention [CONDITIONAL] evidence tag")
	}
	if !strings.Contains(prompt, "ALTERNATIVE BRANCHES") {
		t.Error("step_list prompt should instruct enumeration of alternative branches")
	}
	// /ungrounded contract from Plan B — when grounding tagged a
	// cite as unreliable, the step must name function/file without
	// a line number. The prompt must advertise this contract so the
	// LLM knows what to do when it sees the tag in evidence.
	if !strings.Contains(prompt, "/ungrounded") {
		t.Error("step_list prompt should teach how to handle /ungrounded evidence")
	}
}

// TestOutOfListSymbols covers the S3 token extractor. It only flags
// tokens that STRUCTURALLY look like Go identifiers (CamelCase,
// snake_case, or ≥8-char single-word caps). Common English prose
// capitalisations like "The", "Answer", "Summary" (all ≤7 chars and
// no mid-word uppercase) are structurally filtered out — no
// explicit word list required.
func TestOutOfListSymbols(t *testing.T) {
	allowed := map[string]bool{
		"SubExplorer":  true,
		"Orchestrator": true, // used in a test case below to verify allowed skip
	}
	cases := []struct {
		in       string
		expected []string
	}{
		// Prose capitalisations filtered structurally.
		{"The agent is SubExplorer", nil},
		{"**Answer:** SubExplorer", nil},
		{"Summary: SubExplorer", nil},
		{"Context Content Example Caveat", nil}, // all <8, no CamelCase
		// CamelCase code names that are out of list.
		{"BaseAgent and SubAgentRuntime", []string{"BaseAgent", "SubAgentRuntime"}},
		// Orchestrator is allowed here → not flagged.
		{"Orchestrator and BaseAgent", []string{"BaseAgent"}},
		// snake_case out of list.
		{"a call to base_agent_init()", []string{"base_agent_init"}},
		// SubExplorer is allowed → not flagged.
		{"SubExplorer (internal/agent/sub_explorer.go:20)", []string{"sub_explorer"}},
		// Duplicate violations reported once.
		{"BaseAgent X BaseAgent Y BaseAgent", []string{"BaseAgent"}},
		// Short tokens (<3 chars) ignored.
		{"IO ID Go", nil},
		// Empty string → nil.
		{"", nil},
	}
	for _, c := range cases {
		got := outOfListSymbols(c.in, allowed)
		if !equalStringSlice(got, c.expected) {
			t.Errorf("outOfListSymbols(%q) = %v, want %v", c.in, got, c.expected)
		}
	}
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestFinalizerContinuationPromptValidResponseStops verifies that a
// response containing only allowed symbols does NOT trigger a retry.
// ContinuationPrompt must return shouldContinue=false and no prompt.
func TestFinalizerContinuationPromptValidResponseStops(t *testing.T) {
	e := &finalizerEvaluator{}
	ctx := &types.AgentContext{
		Stage: types.StageFinalize,
		AnswerSymbols: []types.AnswerSymbol{
			{Name: "SubExplorer", File: "x.go", Line: 10},
		},
	}
	e.BuildInitialPrompt(ctx, nil) // seeds allowedSymbolSet
	resp := llm.Response{Content: "The agent is SubExplorer (x.go:10)."}
	prompt, cont := e.ContinuationPrompt(resp, 0, 0, nil)
	if cont {
		t.Errorf("expected shouldContinue=false for valid response, got true (prompt=%q)", prompt)
	}
}

// TestFinalizerContinuationPromptInvalidResponseRetries verifies that
// a response with out-of-list symbols triggers a correction prompt
// and shouldContinue=true, and that the correction prompt names the
// violations explicitly.
func TestFinalizerContinuationPromptInvalidResponseRetries(t *testing.T) {
	e := &finalizerEvaluator{}
	ctx := &types.AgentContext{
		Stage: types.StageFinalize,
		AnswerSymbols: []types.AnswerSymbol{
			{Name: "SubExplorer", File: "x.go", Line: 10},
		},
	}
	e.BuildInitialPrompt(ctx, nil)
	resp := llm.Response{Content: "The agents are Orchestrator, BaseAgent, and SubAgentRuntime."}
	prompt, cont := e.ContinuationPrompt(resp, 0, 0, nil)
	if !cont {
		t.Fatal("expected shouldContinue=true for invalid response")
	}
	for _, v := range []string{"Orchestrator", "BaseAgent", "SubAgentRuntime"} {
		if !strings.Contains(prompt, v) {
			t.Errorf("correction prompt should name violation %q, prompt was: %s", v, prompt)
		}
	}
	if !strings.Contains(prompt, "SubExplorer") {
		t.Error("correction prompt should list allowed symbol SubExplorer")
	}
	if e.retriesUsed != 1 {
		t.Errorf("retriesUsed = %d, want 1", e.retriesUsed)
	}
}

// TestFinalizerContinuationPromptRetryCapExhaustion verifies that
// after maxFinalizerCorrectionRetries the evaluator accepts even a
// still-invalid response (no infinite loop). Returns shouldContinue
// =false to let the ReAct loop stop.
func TestFinalizerContinuationPromptRetryCapExhaustion(t *testing.T) {
	e := &finalizerEvaluator{}
	ctx := &types.AgentContext{
		Stage: types.StageFinalize,
		AnswerSymbols: []types.AnswerSymbol{
			{Name: "SubExplorer"},
		},
	}
	e.BuildInitialPrompt(ctx, nil)
	badResp := llm.Response{Content: "Orchestrator."}

	// Burn through the retry budget.
	for i := 0; i < maxFinalizerCorrectionRetries; i++ {
		_, cont := e.ContinuationPrompt(badResp, i, i, nil)
		if !cont {
			t.Fatalf("retry %d should have requested continuation", i)
		}
	}
	// One more call should hit the cap and return shouldContinue=false.
	_, cont := e.ContinuationPrompt(badResp, maxFinalizerCorrectionRetries, maxFinalizerCorrectionRetries, nil)
	if cont {
		t.Error("expected retry cap to stop continuation, got true")
	}
}

// TestFinalizerShouldStopTranslationMode verifies the S3 ShouldStop
// behaviour: when AnswerSymbols is populated, ShouldStop returns
// false so the ReAct loop consults ContinuationPrompt. Otherwise
// (legacy path) it returns true to preserve one-shot behaviour.
func TestFinalizerShouldStopTranslationMode(t *testing.T) {
	e := &finalizerEvaluator{}
	// Translation mode ON.
	ctx := &types.AgentContext{
		Stage: types.StageFinalize,
		AnswerSymbols: []types.AnswerSymbol{
			{Name: "SubExplorer"},
		},
	}
	e.BuildInitialPrompt(ctx, nil)
	if e.ShouldStop(llm.Response{}, 0) {
		t.Error("translation mode: ShouldStop should return false to allow ContinuationPrompt")
	}

	// Translation mode OFF — legacy one-shot behaviour.
	eLegacy := &finalizerEvaluator{}
	eLegacy.BuildInitialPrompt(&types.AgentContext{Stage: types.StageFinalize}, nil)
	if !eLegacy.ShouldStop(llm.Response{}, 0) {
		t.Error("legacy mode: ShouldStop should return true for one-shot")
	}
}

// TestFinalizerDisarm_SlateNarrowerThanChains verifies the S3 disarm
// fires when the extracted slate is less than half of the upstream
// AnswerChains count — the extractAnswerSymbols enumeration-gap
// symptom documented in project_p2_2_deferred_items.md #13.
func TestFinalizerDisarm_SlateNarrowerThanChains(t *testing.T) {
	e := &finalizerEvaluator{}
	ctx := &types.AgentContext{
		Stage: types.StageFinalize,
		AnswerSymbols: []types.AnswerSymbol{
			// 2 symbols — method-name-shaped, the bug signature.
			{Name: "BuildInitialPrompt"},
			{Name: "ContinuationPrompt"},
		},
		// 9 chains — slate covers < half → disarm fires.
		AnswerChains: []string{"c1", "c2", "c3", "c4", "c5", "c6", "c7", "c8", "c9"},
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Shape: "list_of_symbols"},
			},
		},
	}
	prompt := e.BuildInitialPrompt(ctx, nil)

	if e.allowedSymbolSet != nil {
		t.Error("disarm should clear allowedSymbolSet")
	}
	if len(e.answerSymbols) != 0 {
		t.Error("disarm should clear answerSymbols so ContinuationPrompt's S3 path is dead")
	}
	if strings.Contains(prompt, "Translation mode") {
		t.Error("disarm: prompt must NOT render Translation mode header")
	}
	if !strings.Contains(prompt, "Hard constraint: list_of_symbols answer") {
		t.Error("disarm: prompt must fall through to the shape-based constraint block")
	}
}

// TestFinalizerDisarm_SlateBelowMustInclude verifies the disarm fires
// when the analyzer declared MustInclude[] cardinality the slate fails
// to meet — independent of chain count.
func TestFinalizerDisarm_SlateBelowMustInclude(t *testing.T) {
	e := &finalizerEvaluator{}
	ctx := &types.AgentContext{
		Stage: types.StageFinalize,
		AnswerSymbols: []types.AnswerSymbol{
			{Name: "Foo"},
			{Name: "Bar"},
		},
		// No chains — the MustInclude trigger is independent.
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				RequiredAnswerShape: types.ShapeListOfSymbols,
				MustInclude:         []string{"A", "B", "C", "D", "E"},
			},
		},
	}
	prompt := e.BuildInitialPrompt(ctx, nil)

	if e.allowedSymbolSet != nil {
		t.Error("disarm should clear allowedSymbolSet when slate < MustInclude")
	}
	if strings.Contains(prompt, "Translation mode") {
		t.Error("disarm: prompt must NOT render Translation mode header")
	}
}

// TestFinalizerDisarm_ProportionalSlateNoDisarm verifies the disarm
// does NOT fire when the slate is proportional to upstream evidence.
// A 5-symbol slate against 5 chains and 5 MustInclude is the happy
// path — Translation Mode must stay on.
func TestFinalizerDisarm_ProportionalSlateNoDisarm(t *testing.T) {
	e := &finalizerEvaluator{}
	ctx := &types.AgentContext{
		Stage: types.StageFinalize,
		AnswerSymbols: []types.AnswerSymbol{
			{Name: "A"}, {Name: "B"}, {Name: "C"}, {Name: "D"}, {Name: "E"},
		},
		AnswerChains: []string{"c1", "c2", "c3", "c4", "c5"},
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				RequiredAnswerShape: types.ShapeListOfSymbols,
				MustInclude:         []string{"A", "B", "C", "D", "E"},
			},
		},
	}
	prompt := e.BuildInitialPrompt(ctx, nil)

	if e.allowedSymbolSet == nil {
		t.Error("proportional slate: allowedSymbolSet should remain populated")
	}
	if len(e.allowedSymbolSet) != 5 {
		t.Errorf("allowedSymbolSet size = %d, want 5", len(e.allowedSymbolSet))
	}
	if !strings.Contains(prompt, "Translation mode") {
		t.Error("proportional slate: Translation mode should render")
	}
}

// TestFinalizerDisarm_EmptySlateNoDisarm verifies empty AnswerSymbols
// does not mis-trigger the disarm — empty slate goes through the
// legacy no-symbols path, not the disarm path (which is only
// meaningful against non-empty but wrong slates).
func TestFinalizerDisarm_EmptySlateNoDisarm(t *testing.T) {
	ctx := &types.AgentContext{
		Stage: types.StageFinalize,
		// No AnswerSymbols, no chains, large MustInclude.
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				MustInclude: []string{"A", "B", "C"},
			},
		},
	}
	if shouldDisarmS3(ctx) {
		t.Error("empty slate should not trigger disarm — the legacy path handles it")
	}
}

// TestFinalizerDisarm_NoChainsNoMustIncludeNoDisarm verifies the
// disarm stays silent when the pipeline produced a slate but no
// independent cardinality evidence — we only disarm when the
// upstream has actual data to contradict the slate.
func TestFinalizerDisarm_NoChainsNoMustIncludeNoDisarm(t *testing.T) {
	e := &finalizerEvaluator{}
	ctx := &types.AgentContext{
		Stage: types.StageFinalize,
		AnswerSymbols: []types.AnswerSymbol{
			{Name: "OnlyOne"},
		},
		// No AnswerChains. No MustInclude.
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				RequiredAnswerShape: types.ShapeListOfSymbols,
			},
		},
	}
	e.BuildInitialPrompt(ctx, nil)
	if e.allowedSymbolSet == nil || len(e.allowedSymbolSet) != 1 {
		t.Error("no upstream cardinality evidence: Translation Mode must stay on")
	}
}
