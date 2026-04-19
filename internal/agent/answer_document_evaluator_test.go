package agent

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersResolvedShape
// pins that the dynamic prompt surfaces the resolved target shape
// (for operator visibility + diagnostic logs). The STATIC shape
// dispatch table — tool name, required fields, forbidden fields —
// lives in answer-document-skill.OutputFormat and is rendered as a
// system section by context/builder.go, NOT here. Asserting those
// substrings in the dynamic prompt would resurrect the pre-cleanup
// contradiction between the skill's declarative contract and the
// evaluator's baked-in instructions.
func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersResolvedShape(t *testing.T) {
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
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
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

// TestAnswerDocumentEvaluator_BuildInitialInstruction_SurfacesCardinalityBaseline
// checks that when MustInclude is populated and the resolved shape
// is list_of_symbols, the dynamic prompt renders the γ floor so the
// LLM can compute its completeness claim without re-deriving it from
// the IR.
func TestAnswerDocumentEvaluator_BuildInitialInstruction_SurfacesCardinalityBaseline(t *testing.T) {
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
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
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

// TestAnswerDocumentEvaluator_BuildInitialInstruction_NoFloorWithoutMustInclude
// checks the other branch: when MustInclude is empty, the prompt
// says "no floor is enforced" so the LLM picks the claim from its
// own recall confidence.
func TestAnswerDocumentEvaluator_BuildInitialInstruction_NoFloorWithoutMustInclude(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{RequiredAnswerShape: types.ShapeListOfSymbols},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "MustInclude (γ) is empty") {
		t.Errorf("no-floor branch missing: %q", prompt)
	}
}

// TestAnswerDocumentEvaluator_LanguageCapture reads language from
// AgentContext.Language (set by BuildAgentContext from -lang flag).
func TestAnswerDocumentEvaluator_LanguageCapture(t *testing.T) {
	ctx := &types.AgentContext{Language: "zh"}
	e := &answerDocumentEvaluator{}
	e.BuildInitialInstruction(ctx, nil)
	if e.language != "zh" {
		t.Errorf("language = %q, want zh", e.language)
	}

	ctx2 := &types.AgentContext{Language: "en"}
	e2 := &answerDocumentEvaluator{}
	e2.BuildInitialInstruction(ctx2, nil)
	if e2.language != "en" {
		t.Errorf("language = %q, want en", e2.language)
	}

	ctx3 := &types.AgentContext{} // no language set
	e3 := &answerDocumentEvaluator{}
	e3.BuildInitialInstruction(ctx3, nil)
	if e3.language != "en" {
		t.Errorf("default language = %q, want en", e3.language)
	}
}

// softStopObs builds a minimal PhaseSoftStop LoopObservation for the
// Observe tests — all the answer-document evaluator cares about is
// Phase; the rest of the fields can stay zero.
func softStopObs(continuationCount int) LoopObservation {
	return LoopObservation{
		Phase:             PhaseSoftStop,
		Iteration:         0,
		ContinuationsUsed: continuationCount,
	}
}

// TestAnswerDocumentEvaluator_Observe_RetryBounded exercises the
// evaluator-owned retry budget. After maxFinalizerCorrectionRetries
// retries, Observe must stop returning HintRequested so the policy
// accepts the soft-stop. The retries counter is an
// evaluator-internal contract (fail-loud when the LLM stays off-
// contract after N corrections), distinct from LoopPolicy's
// MaxContinuations which applies loop-wide.
func TestAnswerDocumentEvaluator_Observe_RetryBounded(t *testing.T) {
	maxRetries := types.DefaultAgentSettings().FinalizerMaxCorrectionRetries
	e := &answerDocumentEvaluator{maxRetries: maxRetries}
	for i := 0; i < maxRetries; i++ {
		sig := e.Observe(nil, softStopObs(i))
		if !sig.HintRequested {
			t.Errorf("retry %d: HintRequested = false, want true (still within budget)", i)
		}
	}
	sig := e.Observe(nil, softStopObs(maxRetries))
	if sig.HintRequested {
		t.Error("after budget: HintRequested = true, want false")
	}
}

// TestAnswerDocumentEvaluator_Observe_AcceptsWhenDocPresent pins
// the simplify-round bug fix: when the LLM emits a tool call then
// soft-stops with a content-only turn, Observe must see the
// populated AnswerDocument in Mutable and NOT burn a retry.
// Without this guard, every successful emit followed by a
// free-text closer would trigger a correction retry that clobbers
// the first document on the second call.
func TestAnswerDocumentEvaluator_Observe_AcceptsWhenDocPresent(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetAnswerDocument(&types.AnswerDocument{
		Shape:   types.ShapeExplanation,
		Summary: "landed tool call",
	})
	e := &answerDocumentEvaluator{mu: mu}
	sig := e.Observe(nil, softStopObs(0))
	if sig.HintRequested {
		t.Error("doc present in Mutable: HintRequested = true, want false (no retry)")
	}
	if e.retriesUsed != 0 {
		t.Errorf("doc-present path burned a retry: retriesUsed = %d, want 0", e.retriesUsed)
	}
}

// TestAnswerDocumentEvaluator_Observe_RetriesWhenDocMissing is the
// complement: no doc in Mutable → Observe returns HintRequested.
func TestAnswerDocumentEvaluator_Observe_RetriesWhenDocMissing(t *testing.T) {
	mu := types.NewMutableState("") // empty Mutable
	e := &answerDocumentEvaluator{mu: mu, maxRetries: types.DefaultAgentSettings().FinalizerMaxCorrectionRetries}
	sig := e.Observe(nil, softStopObs(0))
	if !sig.HintRequested {
		t.Error("doc missing: HintRequested = false, want true")
	}
	if e.retriesUsed != 1 {
		t.Errorf("retriesUsed = %d, want 1", e.retriesUsed)
	}
}

// TestAnswerDocumentEvaluator_ParseOutput_Happy — a fully-populated
// AnswerDocument in Mutable is rendered into FinalAnswer.
func TestAnswerDocumentEvaluator_ParseOutput_Happy(t *testing.T) {
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState(""),
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
		Mutable: types.NewMutableState(""),
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
		Mutable: types.NewMutableState(""),
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
	if !strings.Contains(out.FinalAnswer, "confirmed items") {
		t.Errorf("downgraded rendering footer tag missing: %q", out.FinalAnswer)
	}
	if !strings.Contains(out.FinalAnswer, "downgraded to lower_bound") {
		t.Errorf("downgrade caveat missing: %q", out.FinalAnswer)
	}
}

// TestAnswerDocumentEvaluator_ParseOutput_NoDowngrade — completeness
// complete with enough symbols passes through unchanged.
func TestAnswerDocumentEvaluator_ParseOutput_NoDowngrade(t *testing.T) {
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState(""),
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
	// Complete answers have no completeness tag — symbols listed directly.
	if strings.Contains(out.FinalAnswer, "Complete answer") {
		t.Errorf("complete should not have header in body: %q", out.FinalAnswer)
	}
	if !strings.Contains(out.FinalAnswer, "**Alpha**") {
		t.Errorf("symbol missing: %q", out.FinalAnswer)
	}
}

// richDraftProse is a ~1500-char explanation the LLM writes as plain
// prose in its first attempt — the kind of answer the shrinkage
// salvage must preserve when the correction retry emits a compressed
// paraphrase. Kept as a test helper so every shrinkage case shares
// one fixture and the length floor matches the 400-char threshold.
func richDraftProse() string {
	return strings.Repeat(
		"The dispatcher walks the handler chain and delegates to the registered listener. ",
		20) // 20 * ~78 chars = ~1560 chars, well above the 400-char floor
}

// parseStageDoc reads the mutated AnswerDocument back out of the
// StageOutput's JSON Data payload. MutableState.AnswerDocument()
// returns a defensive clone on every call, so the mutations the
// salvage applies inside ParseOutput are only observable through the
// StageOutput it returns.
func parseStageDoc(t *testing.T, out *StageOutput) *types.AnswerDocument {
	t.Helper()
	var payload struct {
		AnswerDocument *types.AnswerDocument `json:"answer_document"`
	}
	if err := json.Unmarshal(out.Data, &payload); err != nil {
		t.Fatalf("unmarshal stage data: %v", err)
	}
	if payload.AnswerDocument == nil {
		t.Fatal("stage data.answer_document is nil")
	}
	return payload.AnswerDocument
}

// TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage — the
// positive case. Explanation shape + rich pre-tool-call draft + a
// short compressed summary in the emitted AnswerDocument. ParseOutput
// must overwrite Summary with the prior draft (trimmed to the
// explanation cap) and append a user-visible caveat.
func TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage(t *testing.T) {
	ctx := &types.AgentContext{Mutable: types.NewMutableState("")}
	doc := &types.AnswerDocument{
		Shape:   types.ShapeExplanation,
		Summary: "The dispatcher delegates to the listener.", // ~41 chars, way under 50% of ~1560
	}
	ctx.Mutable.SetAnswerDocument(doc)

	messages := []llm.Message{
		{Role: "user", Content: "explain the dispatch flow"},
		{Role: "assistant", Content: richDraftProse()}, // no ToolCalls — pre-tool-call draft
		{Role: "user", Content: "[correction hint elided]"},
		{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "1", Name: "emit_answer_document"}}},
	}

	e := &answerDocumentEvaluator{language: "en"}
	out, err := e.ParseOutput(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	got := parseStageDoc(t, out)
	if len(got.Summary) < 400 {
		t.Errorf("summary not salvaged: len=%d, expected prior draft copied in", len(got.Summary))
	}
	if !strings.Contains(got.Summary, "dispatcher") {
		t.Errorf("summary does not contain prior draft content: %q", got.Summary)
	}
	foundCaveat := false
	for _, c := range got.Caveats {
		if strings.Contains(c, "richer prior draft") {
			foundCaveat = true
		}
	}
	if !foundCaveat {
		t.Errorf("salvage caveat missing: %v", got.Caveats)
	}
	if !strings.Contains(out.FinalAnswer, "dispatcher") {
		t.Errorf("FinalAnswer missing salvaged content: %q", out.FinalAnswer)
	}
}

// TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage_OnlyExplanation —
// negative scope check. The salvage is deliberately scoped to
// explanation shape because other shapes carry the answer in
// structured fields, not in Summary. A list_of_symbols answer with a
// rich prior prose draft must NOT have its Summary replaced.
func TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage_OnlyExplanation(t *testing.T) {
	otherShapes := []types.AnswerShape{
		types.ShapeListOfSymbols, types.ShapeStepList,
		types.ShapeValue, types.ShapeConfigValue, types.ShapeBoolean,
	}
	for _, shape := range otherShapes {
		t.Run(string(shape), func(t *testing.T) {
			ctx := &types.AgentContext{Mutable: types.NewMutableState("")}
			origSummary := "one-line lead-in"
			doc := &types.AnswerDocument{Shape: shape, Summary: origSummary}
			// Populate a minimum-required structured field so the
			// renderer doesn't reject the doc for missing payload —
			// we only care that Summary stays untouched.
			switch shape {
			case types.ShapeListOfSymbols:
				doc.Symbols = []types.AnswerSymbol{{Name: "X", File: "a.go", Line: 1}}
			case types.ShapeStepList:
				doc.Steps = []types.AnswerStep{{Index: 1, Description: "d", CitationRef: types.CitationRefUnset}}
			case types.ShapeValue, types.ShapeConfigValue:
				doc.Value = &types.AnswerValue{Literal: "v", CitationRef: types.CitationRefUnset}
			case types.ShapeBoolean:
				doc.Boolean = &types.AnswerBoolean{Decision: true, Rationale: "r", CitationRef: types.CitationRefUnset}
			}
			ctx.Mutable.SetAnswerDocument(doc)
			messages := []llm.Message{
				{Role: "assistant", Content: richDraftProse()},
				{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "1", Name: "emit_answer_document"}}},
			}
			e := &answerDocumentEvaluator{language: "en"}
			out, err := e.ParseOutput(ctx, messages, nil, nil)
			if err != nil {
				t.Fatalf("ParseOutput err: %v", err)
			}
			got := parseStageDoc(t, out)
			if got.Summary != origSummary {
				t.Errorf("shape=%s: Summary was overwritten (%q → %q) — salvage must be scoped to explanation",
					shape, origSummary, got.Summary)
			}
		})
	}
}

// TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage_ShortPriorDraft —
// negative length-floor check. When the prior draft is below the
// minimum prose length (400 chars), the salvage must not fire.
// Otherwise a one-line placeholder pre-tool-call content would start
// overwriting every answer.
func TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage_ShortPriorDraft(t *testing.T) {
	ctx := &types.AgentContext{Mutable: types.NewMutableState("")}
	origSummary := "The dispatcher delegates to the listener."
	doc := &types.AnswerDocument{Shape: types.ShapeExplanation, Summary: origSummary}
	ctx.Mutable.SetAnswerDocument(doc)
	messages := []llm.Message{
		{Role: "assistant", Content: "short draft, well under 400 chars"},
		{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "1", Name: "emit_answer_document"}}},
	}
	e := &answerDocumentEvaluator{language: "en"}
	out, err := e.ParseOutput(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	got := parseStageDoc(t, out)
	if got.Summary != origSummary {
		t.Errorf("Summary was overwritten despite short prior draft: %q", got.Summary)
	}
}

// TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage_NotShrunk —
// negative ratio check. When the emitted Summary is already at least
// half the length of the prior draft, the salvage must not fire —
// the LLM did NOT compress this time.
func TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage_NotShrunk(t *testing.T) {
	ctx := &types.AgentContext{Mutable: types.NewMutableState("")}
	// Emitted summary is 80% of prior draft — within tolerance, no salvage.
	prior := richDraftProse()
	origSummary := prior[:int(float64(len(prior))*0.8)]
	doc := &types.AnswerDocument{Shape: types.ShapeExplanation, Summary: origSummary}
	ctx.Mutable.SetAnswerDocument(doc)
	messages := []llm.Message{
		{Role: "assistant", Content: prior},
		{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "1", Name: "emit_answer_document"}}},
	}
	e := &answerDocumentEvaluator{language: "en"}
	out, err := e.ParseOutput(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	got := parseStageDoc(t, out)
	if got.Summary != origSummary {
		t.Errorf("Summary was overwritten despite ratio above threshold: len=%d → %d", len(origSummary), len(got.Summary))
	}
}

// TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage_CapTrim —
// the prior draft exceeding SummaryCapFor(ShapeExplanation) must be
// trimmed, not rejected — the salvage's job is best-effort recovery.
func TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage_CapTrim(t *testing.T) {
	ctx := &types.AgentContext{Mutable: types.NewMutableState("")}
	cap := types.SummaryCapFor(types.ShapeExplanation)
	// Build a draft 1.5x the cap.
	prior := strings.Repeat("a", cap*3/2)
	doc := &types.AnswerDocument{Shape: types.ShapeExplanation, Summary: "x"}
	ctx.Mutable.SetAnswerDocument(doc)
	messages := []llm.Message{
		{Role: "assistant", Content: prior},
		{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "1", Name: "emit_answer_document"}}},
	}
	e := &answerDocumentEvaluator{language: "en"}
	out, err := e.ParseOutput(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	got := parseStageDoc(t, out)
	if len(got.Summary) != cap {
		t.Errorf("Summary not trimmed to cap: len=%d, cap=%d", len(got.Summary), cap)
	}
}

// TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage_CJKRuneBoundary —
// the cap trim must land on a rune boundary so the tail of a CJK
// answer does not end mid-codepoint (which would display as an
// invalid glyph). Build a prior draft of 3-byte CJK characters long
// enough to overshoot the cap, then verify the trimmed summary is
// still valid UTF-8.
func TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage_CJKRuneBoundary(t *testing.T) {
	ctx := &types.AgentContext{Mutable: types.NewMutableState("")}
	// 中 is 3 bytes UTF-8. 1000 copies = 3000 bytes, overshoots the 2500 cap.
	prior := strings.Repeat("中", 1000)
	doc := &types.AnswerDocument{Shape: types.ShapeExplanation, Summary: "短"}
	ctx.Mutable.SetAnswerDocument(doc)
	messages := []llm.Message{
		{Role: "assistant", Content: prior},
		{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "1", Name: "emit_answer_document"}}},
	}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	got := parseStageDoc(t, out)
	if !utf8.ValidString(got.Summary) {
		t.Errorf("trimmed summary has invalid UTF-8 tail: len=%d last8=%x",
			len(got.Summary), got.Summary[max(0, len(got.Summary)-8):])
	}
	// Trimmed length should be close to 2500, never exceed it.
	if len(got.Summary) > types.SummaryCapFor(types.ShapeExplanation) {
		t.Errorf("trimmed summary exceeds cap: len=%d", len(got.Summary))
	}
	// At 3 bytes/rune, trimming 2500 bytes should preserve at least
	// 833 runes (2500 / 3 = 833.33).
	if got := utf8.RuneCountInString(got.Summary); got < 830 {
		t.Errorf("trimmed summary lost too many runes: got %d runes, expected ~833", got)
	}
}

// TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage_ZhCaveat —
// bilingual caveat check: when language=zh, the appended caveat is
// rendered in Chinese.
func TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage_ZhCaveat(t *testing.T) {
	ctx := &types.AgentContext{Mutable: types.NewMutableState("")}
	doc := &types.AnswerDocument{Shape: types.ShapeExplanation, Summary: "short"}
	ctx.Mutable.SetAnswerDocument(doc)
	messages := []llm.Message{
		{Role: "assistant", Content: richDraftProse()},
		{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "1", Name: "emit_answer_document"}}},
	}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	got := parseStageDoc(t, out)
	foundZh := false
	for _, c := range got.Caveats {
		if strings.Contains(c, "更丰富的前一轮草稿") {
			foundZh = true
		}
	}
	if !foundZh {
		t.Errorf("zh caveat missing: %v", got.Caveats)
	}
}

// TestFindLastPreToolCallDraft_IgnoresToolCallTurns — the helper
// must SKIP assistant messages that have tool calls, since those
// represent "tool call fired" turns, not pre-tool-call drafts. The
// target is the draft from BEFORE the emit landed.
func TestFindLastPreToolCallDraft_IgnoresToolCallTurns(t *testing.T) {
	messages := []llm.Message{
		{Role: "user", Content: "q"},
		{Role: "assistant", Content: "pre-tool-call draft"}, // no ToolCalls
		{Role: "user", Content: "hint"},
		{Role: "assistant", Content: "tool-call turn preamble", ToolCalls: []llm.ToolCall{{ID: "1", Name: "emit"}}},
	}
	got := findLastPreToolCallDraft(messages)
	if got != "pre-tool-call draft" {
		t.Errorf("findLastPreToolCallDraft = %q, want pre-tool-call draft", got)
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
