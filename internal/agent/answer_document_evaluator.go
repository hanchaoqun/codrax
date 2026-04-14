package agent

// answer_document_evaluator.go — P2.2 finalizer evaluator that emits
// a structured AnswerDocument via the emit_answer_document tool and
// renders it to user prose through internal/render/answerdoc.go.
//
// Flag=on replacement for finalizerEvaluator; the two coexist until
// flip-default (gated on grid + manual inspection).
//
// Design contract (docs/architecture-root-cause-remediation.md §6
// P2.2): the LLM emits AnswerDocument via one batched tool call, the
// evaluator runs shape-level structural validation plus the P2.1
// cardinality cross-check for list_of_symbols/complete slates, and
// the renderer produces deterministic prose. Patterns 1/2/3/4 become
// structurally impossible on the flag=on path.
//
// Cardinality cross-check: the emit_answer_document tool already
// validates per-item grounding (line > 0, file not in WorkDir,
// citation_ref in range). The P2.1 baseline (TerminalEvidenceCount +
// AnalysisIR.AnswerContract.MustInclude) is not part of the tool
// schema, so the evaluator applies validateCompletenessClaim HERE
// at ParseOutput time — the same function extractorEvaluator uses
// for emit_answer_symbol, so one audited implementation covers
// both stages.

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// answerDocumentEvaluator is the Evaluator implementation for the
// finalize stage when answer_document_mode=on.
type answerDocumentEvaluator struct {
	// language is captured at BuildInitialPrompt time so ParseOutput
	// can pick the renderer locale without re-deriving it.
	language string

	// mu is captured at BuildInitialPrompt so ContinuationPrompt can
	// check whether the AnswerDocument tool call has landed before
	// issuing a correction retry. Without this, a content-only turn
	// that followed a successful tool call would burn a retry and
	// re-prompt the LLM to emit the tool call a second time,
	// clobbering the first document. The evaluator Evaluator
	// interface does not pass ctx to ContinuationPrompt, so the
	// pointer has to be stashed on the struct.
	mu *types.MutableState

	// retriesUsed tracks correction rounds across the ReAct loop,
	// bounded by maxFinalizerCorrectionRetries (the shared budget
	// with the legacy finalizerEvaluator).
	retriesUsed int
}

func (e *answerDocumentEvaluator) BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string {
	e.retriesUsed = 0
	e.language = extractAnswerDocLang(ctx)
	if ctx != nil {
		e.mu = ctx.Mutable
	}

	var b strings.Builder
	b.WriteString("Compile your findings into a STRUCTURED final answer by calling the ")
	b.WriteString("`emit_answer_document` tool exactly once. Do NOT write prose outside the tool ")
	b.WriteString("call — the tool's output is the entire final answer, and a deterministic ")
	b.WriteString("renderer turns it into user-visible text in the configured language.\n\n")

	shape := resolveAnswerDocShape(ctx)

	fmt.Fprintf(&b, "## Target shape: `%s`\n\n", shape)
	b.WriteString(answerDocShapeInstructions(shape))
	b.WriteString("\n\n")

	// Citation pool contract: enforced at the tool layer, but the LLM
	// needs to know why it exists so it does not try to duplicate
	// citations inline per step.
	b.WriteString("## Citation pool\n\n")
	b.WriteString("Every file:line you cite MUST be declared once in the `citations` array, and every ")
	b.WriteString("`citation_ref` elsewhere is a zero-based integer index into that array (or `-1` when ")
	b.WriteString("no citation backs the entry). This lets one cited line serve multiple steps without ")
	b.WriteString("re-rendering — the renderer resolves the indices for you.\n\n")

	// Completeness honesty contract is only meaningful for
	// list_of_symbols, but we surface it unconditionally so the model
	// never forgets to set it when the shape does apply.
	if shape == string(types.ShapeListOfSymbols) {
		b.WriteString("## Completeness honesty contract (list_of_symbols)\n\n")
		b.WriteString("Set `symbols_completeness` to `complete` ONLY if the slate contains every answer. ")
		if ctx != nil && ctx.AnalysisIR != nil {
			must := ctx.AnalysisIR.AnswerContract.MustInclude
			if len(must) > 0 {
				fmt.Fprintf(&b, "The analyzer requires at least %d name(s): %s. A `complete` claim with fewer items will be DOWNGRADED to `lower_bound` by the finalizer's cardinality validator. ",
					len(must), strings.Join(must, ", "))
			}
		}
		b.WriteString("If you are not certain the slate is complete, use `lower_bound` instead — it ")
		b.WriteString("preserves your findings as a floor without lying to the user.\n\n")

		if len(ctx.AnswerSymbols) > 0 {
			b.WriteString("### Prior slate from the extraction pipeline\n\n")
			b.WriteString("The deterministic pipeline already produced an answer-symbol list. Use it ")
			b.WriteString("as the starting point; adding items requires evidence from the Evidence Items ")
			b.WriteString("section, and removing items requires a rationale.\n\n")
			for _, s := range ctx.AnswerSymbols {
				if s.File != "" && s.Line > 0 {
					fmt.Fprintf(&b, "- %s (%s:%d)\n", s.Name, s.File, s.Line)
				} else {
					fmt.Fprintf(&b, "- %s\n", s.Name)
				}
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("## Prohibitions\n\n")
	b.WriteString("- Do not write prose outside the `emit_answer_document` tool call.\n")
	b.WriteString("- Do not cite a file or line that is not in the evidence / read-files list from prior stages.\n")
	b.WriteString("- Do not inflate `summary` past 500 characters — it is a 1-2 sentence lead-in, not the answer body.\n")
	b.WriteString("- Do not invent `citation_ref` values; use `-1` when there is genuinely no backing cite.\n")

	return b.String()
}

// extractAnswerDocLang pulls the language preference from the
// orchestrator's injected BusContext.Preferences. The directive
// wording is stable (languageDirective in orchestrator.go emits
// "Respond to the user in Simplified Chinese …" or "Respond to the
// user in English …"), so a cheap substring match recovers the locale
// without plumbing a dedicated field through AgentContext.
//
// When no directive is present (flagLang=off / none / "") we default
// to "en" so the renderer has a defined locale.
func extractAnswerDocLang(ctx *types.AgentContext) string {
	if ctx == nil {
		return "en"
	}
	for _, p := range ctx.Preferences {
		lower := strings.ToLower(p)
		if strings.Contains(lower, "simplified chinese") || strings.Contains(lower, "简体中文") {
			return "zh"
		}
		if strings.Contains(lower, "respond to the user in english") {
			return "en"
		}
	}
	return "en"
}

// resolveAnswerDocShape picks the shape string the finalizer prompt
// should target. Preference order:
//
//   1. AnalysisIR.AnswerContract.RequiredAnswerShape — the canonical
//      source wired by P1.3. Typed + non-empty means the analyzer
//      reached a decision.
//   2. irAnswerShape(ctx) — the legacy AnalyzerHints.Shape field,
//      kept as a fallback for pre-P1.3 call paths and REPL turns
//      where the IR is nil.
//   3. Presence of ctx.AnswerSymbols → list_of_symbols (an upstream
//      extraction pipeline found candidates, so the shape is clearly
//      a symbol list).
//   4. Explanation — the safe default.
//
// Returning a string (rather than types.AnswerShape) keeps the callers
// — prompt assembly + shape-specific instructions — uniform against
// both the typed and legacy paths without a per-call coercion.
func resolveAnswerDocShape(ctx *types.AgentContext) string {
	if ctx != nil && ctx.AnalysisIR != nil {
		if shape := ctx.AnalysisIR.AnswerContract.RequiredAnswerShape; shape != "" && shape != types.ShapeNone {
			return string(shape)
		}
	}
	if s := irAnswerShape(ctx); s != "" {
		return s
	}
	if ctx != nil && len(ctx.AnswerSymbols) > 0 {
		return string(types.ShapeListOfSymbols)
	}
	return string(types.ShapeExplanation)
}

// answerDocShapeInstructions returns the shape-specific required-field
// block rendered into the LLM prompt. Each branch is short because
// the tool's JSON schema carries the full per-field contract — this
// block is the natural-language summary that tells the LLM what to
// produce, not the authoritative spec.
func answerDocShapeInstructions(shape string) string {
	switch shape {
	case string(types.ShapeListOfSymbols):
		return "Emit `symbols[]` with one entry per answer (name, file, line, kind), " +
			"and set `symbols_completeness` to `complete` / `lower_bound` / `unknown`. " +
			"Do NOT emit `steps[]`, `value`, or `boolean` — they are forbidden for this shape."
	case string(types.ShapeStepList):
		return "Emit `steps[]` with one entry per distinct branch or mechanism hop. " +
			"Each step needs a positive `index`, a one-sentence `description`, and a " +
			"`citation_ref` (or -1). Distinct [CONDITIONAL] evidence branches MUST become distinct steps. " +
			"Do NOT emit `symbols[]`, `value`, or `boolean`."
	case string(types.ShapeValue):
		return "Emit `value{literal, citation_ref}` with the verbatim value literal from evidence. " +
			"Leave `key` empty. Do NOT emit `steps[]`, `symbols[]`, or `boolean`."
	case string(types.ShapeConfigValue):
		return "Emit `value{key, literal, citation_ref}` with the config key path AND the " +
			"resolved literal. Both come from evidence. Do NOT emit `steps[]`, `symbols[]`, or `boolean`."
	case string(types.ShapeBoolean):
		return "Emit `boolean{decision, rationale, citation_ref}` — decision is exactly one of " +
			"true / false / yes / no / 是 / 否, rationale is a one-sentence evidence citation. " +
			"Do NOT hedge. Do NOT emit `steps[]`, `symbols[]`, or `value`."
	case string(types.ShapeExplanation):
		return "Emit a non-empty `summary` (1-2 sentences, ≤500 chars) with the natural-language " +
			"answer. Add `citations[]` entries for any file:line the summary references. " +
			"Do NOT emit `steps[]`, `symbols[]`, `value`, or `boolean`."
	}
	return "The analyzer did not declare a specific shape; emit `explanation` with a " +
		"non-empty summary plus backing citations."
}

// ShouldStop always returns false so the BaseAgent's soft-stop path
// delegates the accept/retry decision to ContinuationPrompt.
func (e *answerDocumentEvaluator) ShouldStop(resp llm.Response, iteration int) bool {
	return false
}

// ContinuationPrompt fires when the LLM soft-stops. A populated
// AnswerDocument in Mutable means the tool call has already landed
// and the prose is assembled by ParseOutput — accept. A missing
// document within the retry budget triggers a correction; beyond
// the budget we give up and fall through to the fail-loud path in
// ParseOutput.
func (e *answerDocumentEvaluator) ContinuationPrompt(resp llm.Response, iteration int, continuationCount int, _ []types.ToolResult) (string, bool) {
	if e.mu != nil {
		if doc := e.mu.AnswerDocument(); doc != nil && !doc.IsZero() {
			return "", false
		}
	}
	if e.retriesUsed >= maxFinalizerCorrectionRetries {
		logging.Debug("[finalizer/answer_document] correction retries exhausted (%d); accepting response",
			e.retriesUsed)
		return "", false
	}
	e.retriesUsed++
	logging.Debug("[finalizer/answer_document] correction retry #%d: requesting emit_answer_document",
		e.retriesUsed)
	return "You must produce the final answer by calling `emit_answer_document` exactly once. " +
		"Do not write prose outside the tool call. Review the shape-specific instructions in the " +
		"initial prompt and emit the tool call now.", true
}

// answerDocumentStageData is the JSON payload shape written into
// StageOutput.Data. Marshaling via a typed struct (rather than
// fmt.Sprintf with %q) keeps unicode escapes JSON-safe.
type answerDocumentStageData struct {
	FinalAnswer    string                `json:"final_answer"`
	AnswerDocument *types.AnswerDocument `json:"answer_document"`
}

// ParseOutput reads the AnswerDocument from Mutable, runs the P2.1
// cardinality cross-check on list_of_symbols + complete claims,
// renders the document to prose, and packages the result into a
// StageOutput. On a zero/missing document, emits a fail-loud warning
// prefixed to the raw last LLM content.
func (e *answerDocumentEvaluator) ParseOutput(ctx *types.AgentContext, messages []llm.Message, _ []types.ToolResult, _ []types.MCPResponse) (*StageOutput, error) {
	out := &StageOutput{}

	var doc *types.AnswerDocument
	if ctx != nil && ctx.Mutable != nil {
		doc = ctx.Mutable.AnswerDocument()
	}

	if doc == nil || doc.IsZero() {
		var lastContent string
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == "assistant" && messages[i].Content != "" {
				lastContent = messages[i].Content
				break
			}
		}
		warning := "⚠️ answer_document emission missing — the finalizer could not produce a " +
			"structured AnswerDocument. The text below is the raw LLM response; no schema-level " +
			"validation ran on it."
		combined := warning
		if lastContent != "" {
			combined = warning + "\n\n" + lastContent
		}
		logging.Warning("[finalizer/answer_document] emit_answer_document missing after retries; falling back to raw content (len=%d)",
			len(lastContent))
		out.Data = marshalStageData(answerDocumentStageData{FinalAnswer: combined})
		out.FinalAnswer = combined
		return out, nil
	}

	if doc.Shape == types.ShapeListOfSymbols && doc.SymbolsCompleteness == types.CompletenessComplete {
		validated := validateCompletenessClaim(ctx, doc.Symbols, doc.SymbolsCompleteness)
		if validated != doc.SymbolsCompleteness {
			logging.Warning("[finalizer/answer_document] symbols_completeness DOWNGRADED %s → %s for list_of_symbols answer",
				doc.SymbolsCompleteness, validated)
			doc.SymbolsCompleteness = validated
			// Caveat surfaces the downgrade in the rendered prose
			// instead of letting it happen silently.
			doc.Caveats = append(doc.Caveats,
				"symbols list was marked complete but did not meet the cardinality baseline; downgraded to lower_bound")
		}
	}

	prose := render.RenderAnswerDocument(doc, e.language)

	out.Data = marshalStageData(answerDocumentStageData{
		FinalAnswer:    prose,
		AnswerDocument: doc,
	})
	out.FinalAnswer = prose

	if len(doc.Symbols) > 0 {
		out.AnswerSymbols = doc.Symbols
		out.AnswerSymbolCompleteness = doc.SymbolsCompleteness
	}

	return out, nil
}

func marshalStageData(payload answerDocumentStageData) json.RawMessage {
	b, err := json.Marshal(payload)
	if err != nil {
		logging.Error("[finalizer/answer_document] marshal stage data failed: %v", err)
		return json.RawMessage(`{}`)
	}
	return b
}

func (e *answerDocumentEvaluator) DetermineMissingPiece(_ *types.AgentContext, _ *StageOutput) types.MissingPiece {
	return types.MissingNone
}
