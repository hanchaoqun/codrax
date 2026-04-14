package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestAnswerDocumentEvaluator_BuildInitialPrompt_RendersResolvedShape
// pins that the dynamic prompt surfaces the resolved target shape
// (for operator visibility + diagnostic logs). The STATIC shape
// dispatch table — tool name, required fields, forbidden fields —
// lives in answer-document-skill.OutputFormat and is rendered as a
// system section by context/builder.go, NOT here. Asserting those
// substrings in the dynamic prompt would resurrect the pre-cleanup
// contradiction between the skill's declarative contract and the
// evaluator's baked-in instructions.
func TestAnswerDocumentEvaluator_BuildInitialPrompt_RendersResolvedShape(t *testing.T) {
	shapes := []types.AnswerShape{
		types.ShapeListOfSymbols, types.ShapeStepList, types.ShapeValue,
		types.ShapeConfigValue, types.ShapeBoolean, types.ShapeExplanation,
	}
	for _, shape := range shapes {
		t.Run(string(shape), func(t *testing.T) {
			ctx := &types.AgentContext{
				AnalysisIR: &types.AnalysisIR{
					AnswerContract: types.AnswerContract{RequiredAnswerShape: shape},
				},
			}
			prompt := (&answerDocumentEvaluator{}).BuildInitialPrompt(ctx, nil)
			// The dynamic prompt carries the shape name for operator
			// visibility — this is the one substring the evaluator
			// still owns after the static contract moved to the skill.
			if !strings.Contains(prompt, string(shape)) {
				t.Errorf("shape=%s: dynamic prompt missing resolved shape name: %q", shape, prompt)
			}
			// Guard against drift back to the pre-cleanup pattern:
			// the static contract MUST NOT resurface here.
			for _, banned := range []string{"emit_answer_document", "Prohibitions", "Citation pool"} {
				if strings.Contains(prompt, banned) {
					t.Errorf("shape=%s: dynamic prompt leaked static contract substring %q — "+
						"that content belongs in answer-document-skill, not the evaluator", shape, banned)
				}
			}
		})
	}
}

// TestAnswerDocumentEvaluator_BuildInitialPrompt_SurfacesCardinalityBaseline
// checks that when MustInclude is populated and the resolved shape
// is list_of_symbols, the dynamic prompt renders the γ floor so the
// LLM can compute its completeness claim without re-deriving it from
// the IR.
func TestAnswerDocumentEvaluator_BuildInitialPrompt_SurfacesCardinalityBaseline(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				RequiredAnswerShape: types.ShapeListOfSymbols,
				MustInclude:         []string{"Alpha", "Beta"},
			},
		},
		AnswerSymbols: []types.AnswerSymbol{
			{Name: "Alpha", File: "a.go", Line: 10},
			{Name: "Beta", File: "b.go", Line: 20},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialPrompt(ctx, nil)
	if !strings.Contains(prompt, "Alpha") || !strings.Contains(prompt, "Beta") {
		t.Errorf("prior slate not surfaced: %q", prompt)
	}
	if !strings.Contains(prompt, "MustInclude (γ): **2 name(s)**") {
		t.Errorf("MustInclude γ floor not surfaced: %q", prompt)
	}
	if !strings.Contains(prompt, "fewer than 2 items will be DOWNGRADED") {
		t.Errorf("downgrade warning not surfaced: %q", prompt)
	}
}

// TestAnswerDocumentEvaluator_BuildInitialPrompt_NoFloorWithoutMustInclude
// checks the other branch: when MustInclude is empty, the prompt
// says "no floor is enforced" so the LLM picks the claim from its
// own recall confidence.
func TestAnswerDocumentEvaluator_BuildInitialPrompt_NoFloorWithoutMustInclude(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{RequiredAnswerShape: types.ShapeListOfSymbols},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialPrompt(ctx, nil)
	if !strings.Contains(prompt, "MustInclude (γ) is empty") {
		t.Errorf("no-floor branch missing: %q", prompt)
	}
}

// TestAnswerDocumentEvaluator_LanguageCapture pulls zh from
// Preferences. Mirrors orchestrator.languageDirective output.
func TestAnswerDocumentEvaluator_LanguageCapture(t *testing.T) {
	ctx := &types.AgentContext{
		Preferences: []string{
			"Respond to the user in Simplified Chinese (简体中文) by default. " +
				"Keep technical terms …",
		},
	}
	e := &answerDocumentEvaluator{}
	e.BuildInitialPrompt(ctx, nil)
	if e.language != "zh" {
		t.Errorf("language = %q, want zh", e.language)
	}

	ctx2 := &types.AgentContext{
		Preferences: []string{"Respond to the user in English by default."},
	}
	e2 := &answerDocumentEvaluator{}
	e2.BuildInitialPrompt(ctx2, nil)
	if e2.language != "en" {
		t.Errorf("language = %q, want en", e2.language)
	}

	ctx3 := &types.AgentContext{} // no preferences
	e3 := &answerDocumentEvaluator{}
	e3.BuildInitialPrompt(ctx3, nil)
	if e3.language != "en" {
		t.Errorf("default language = %q, want en", e3.language)
	}
}

// TestAnswerDocumentEvaluator_ContinuationPrompt_RetryBounded
// exercises the retry budget — after maxFinalizerCorrectionRetries
// retries, the evaluator stops issuing corrections.
func TestAnswerDocumentEvaluator_ContinuationPrompt_RetryBounded(t *testing.T) {
	e := &answerDocumentEvaluator{}
	for i := 0; i < maxFinalizerCorrectionRetries; i++ {
		_, cont := e.ContinuationPrompt(llm.Response{}, 0, i, nil)
		if !cont {
			t.Errorf("retry %d: cont = false, want true (still within budget)", i)
		}
	}
	_, cont := e.ContinuationPrompt(llm.Response{}, 0, maxFinalizerCorrectionRetries, nil)
	if cont {
		t.Error("after budget: cont = true, want false")
	}
}

// TestAnswerDocumentEvaluator_ContinuationPrompt_AcceptsWhenDocPresent
// pins the simplify-round bug fix: when the LLM emits a tool call
// then soft-stops with a content-only turn, ContinuationPrompt must
// see the populated AnswerDocument in Mutable and NOT burn a retry.
// Without this guard, every successful emit followed by a free-text
// closer would trigger a correction retry that clobbers the first
// document on the second call.
func TestAnswerDocumentEvaluator_ContinuationPrompt_AcceptsWhenDocPresent(t *testing.T) {
	mu := types.NewMutableState(types.TaskList{})
	mu.SetAnswerDocument(&types.AnswerDocument{
		Shape:   types.ShapeExplanation,
		Summary: "landed tool call",
	})
	e := &answerDocumentEvaluator{mu: mu}
	_, cont := e.ContinuationPrompt(llm.Response{}, 0, 0, nil)
	if cont {
		t.Error("doc present in Mutable: cont = true, want false (no retry)")
	}
	if e.retriesUsed != 0 {
		t.Errorf("doc-present path burned a retry: retriesUsed = %d, want 0", e.retriesUsed)
	}
}

// TestAnswerDocumentEvaluator_ContinuationPrompt_RetriesWhenDocMissing
// is the complement: no doc in Mutable → retry.
func TestAnswerDocumentEvaluator_ContinuationPrompt_RetriesWhenDocMissing(t *testing.T) {
	mu := types.NewMutableState(types.TaskList{}) // empty Mutable
	e := &answerDocumentEvaluator{mu: mu}
	_, cont := e.ContinuationPrompt(llm.Response{}, 0, 0, nil)
	if !cont {
		t.Error("doc missing: cont = false, want true")
	}
	if e.retriesUsed != 1 {
		t.Errorf("retriesUsed = %d, want 1", e.retriesUsed)
	}
}

// TestAnswerDocumentEvaluator_ParseOutput_Happy — a fully-populated
// AnswerDocument in Mutable is rendered into FinalAnswer.
func TestAnswerDocumentEvaluator_ParseOutput_Happy(t *testing.T) {
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState(types.TaskList{}),
	}
	doc := &types.AnswerDocument{
		Shape:   types.ShapeValue,
		Value:   &types.AnswerValue{Literal: "explorer", CitationRef: 0},
		Citations: []types.Citation{{File: "a.go", Line: 42}},
	}
	ctx.Mutable.SetAnswerDocument(doc)

	e := &answerDocumentEvaluator{language: "en"}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if out.FinalAnswer == "" {
		t.Fatal("FinalAnswer empty")
	}
	if !strings.Contains(out.FinalAnswer, "`explorer`") {
		t.Errorf("FinalAnswer missing literal: %q", out.FinalAnswer)
	}
	if !strings.Contains(out.FinalAnswer, "a.go:42") {
		t.Errorf("FinalAnswer missing citation: %q", out.FinalAnswer)
	}
	// Data payload carries the structured doc for debugging.
	var payload struct {
		FinalAnswer    string          `json:"final_answer"`
		AnswerDocument json.RawMessage `json:"answer_document"`
	}
	if err := json.Unmarshal(out.Data, &payload); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if len(payload.AnswerDocument) == 0 || string(payload.AnswerDocument) == "null" {
		t.Error("Data.answer_document is empty/null")
	}
}

// TestAnswerDocumentEvaluator_ParseOutput_MissingDoc_FailLoud covers
// the fail-loud path: no document, retries exhausted, ParseOutput
// surfaces a warning banner prefixed to the raw content.
func TestAnswerDocumentEvaluator_ParseOutput_MissingDoc_FailLoud(t *testing.T) {
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState(types.TaskList{}),
	}
	messages := []llm.Message{
		{Role: "assistant", Content: "raw fallback text"},
	}
	e := &answerDocumentEvaluator{language: "en"}
	out, err := e.ParseOutput(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if !strings.Contains(out.FinalAnswer, "answer_document emission missing") {
		t.Errorf("fail-loud warning missing: %q", out.FinalAnswer)
	}
	if !strings.Contains(out.FinalAnswer, "raw fallback text") {
		t.Errorf("raw content lost: %q", out.FinalAnswer)
	}
}

// TestAnswerDocumentEvaluator_ParseOutput_CardinalityDowngrade is the
// critical P2.2 test: when Shape == list_of_symbols + Completeness ==
// complete + len(Symbols) < baseline, ParseOutput downgrades to
// lower_bound and appends a caveat. Reuses the same validator as
// extractorEvaluator so this test pins the cross-stage contract.
func TestAnswerDocumentEvaluator_ParseOutput_CardinalityDowngrade(t *testing.T) {
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState(types.TaskList{}),
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				MustInclude: []string{"Alpha", "Beta", "Gamma", "Delta"}, // baseline = 4
			},
		},
	}
	doc := &types.AnswerDocument{
		Shape:               types.ShapeListOfSymbols,
		SymbolsCompleteness: types.CompletenessComplete,
		Symbols: []types.AnswerSymbol{ // only 2 — below baseline of 4
			{Name: "Alpha", File: "a.go", Line: 10},
			{Name: "Beta", File: "b.go", Line: 20},
		},
	}
	ctx.Mutable.SetAnswerDocument(doc)

	e := &answerDocumentEvaluator{language: "en"}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if out.AnswerSymbolCompleteness != types.CompletenessLowerBound {
		t.Errorf("completeness = %q, want lower_bound (downgrade)", out.AnswerSymbolCompleteness)
	}
	if !strings.Contains(out.FinalAnswer, "At least the following symbols") {
		t.Errorf("downgraded rendering not applied: %q", out.FinalAnswer)
	}
	if !strings.Contains(out.FinalAnswer, "downgraded to lower_bound") {
		t.Errorf("downgrade caveat missing: %q", out.FinalAnswer)
	}
}

// TestAnswerDocumentEvaluator_ParseOutput_NoDowngrade — completeness
// complete with enough symbols passes through unchanged.
func TestAnswerDocumentEvaluator_ParseOutput_NoDowngrade(t *testing.T) {
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState(types.TaskList{}),
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				MustInclude: []string{"Alpha"}, // baseline = 1
			},
		},
	}
	doc := &types.AnswerDocument{
		Shape:               types.ShapeListOfSymbols,
		SymbolsCompleteness: types.CompletenessComplete,
		Symbols: []types.AnswerSymbol{
			{Name: "Alpha", File: "a.go", Line: 10},
			{Name: "Beta", File: "b.go", Line: 20},
		},
	}
	ctx.Mutable.SetAnswerDocument(doc)
	e := &answerDocumentEvaluator{language: "en"}
	out, _ := e.ParseOutput(ctx, nil, nil, nil)
	if out.AnswerSymbolCompleteness != types.CompletenessComplete {
		t.Errorf("completeness = %q, want complete (no downgrade)", out.AnswerSymbolCompleteness)
	}
	if !strings.Contains(out.FinalAnswer, "Complete answer") {
		t.Errorf("complete rendering not applied: %q", out.FinalAnswer)
	}
}

// TestAnswerDocumentEvaluator_DetermineMissingPiece — always returns
// MissingNone, matching the legacy finalizer contract.
func TestAnswerDocumentEvaluator_DetermineMissingPiece(t *testing.T) {
	e := &answerDocumentEvaluator{}
	if got := e.DetermineMissingPiece(nil, nil); got != types.MissingNone {
		t.Errorf("DetermineMissingPiece = %q, want MissingNone", got)
	}
}

// TestAnswerDocumentSkill_DeclaresEmitTool pins the P2.2 cleanup
// contract: the declarative answer-document-skill in
// internal/skill/defaults.go MUST declare emit_answer_document in
// its ToolSuggestions. This replaces the pre-cleanup approach of
// patching the two legacy finalize skills' ToolSuggestions at runtime
// in cmd/root.go, which would have left their contradictory
// Answer/Evidence markdown OutputFormat in the prompt.
func TestAnswerDocumentSkill_DeclaresEmitTool(t *testing.T) {
	reg := skill.NewRegistry()
	skill.RegisterDefaults(reg)
	sk, err := reg.Get("answer-document-skill")
	if err != nil {
		t.Fatalf("answer-document-skill not registered by RegisterDefaults: %v", err)
	}
	found := false
	for _, name := range sk.ToolSuggestions {
		if name == "emit_answer_document" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("answer-document-skill.ToolSuggestions missing emit_answer_document: %v",
			sk.ToolSuggestions)
	}
	// Sanity checks: the skill must NOT accidentally declare the
	// legacy finalize skills' tools (todo_write etc.) which would
	// reintroduce the prose-writing pathway.
	if len(sk.ToolSuggestions) != 1 {
		t.Errorf("answer-document-skill should only declare emit_answer_document, got %v",
			sk.ToolSuggestions)
	}
}
