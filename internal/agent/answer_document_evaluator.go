package agent

// answer_document_evaluator.go — finalizer evaluator that emits a
// structured AnswerDocument via the emit_answer_document tool and
// renders it to user prose through internal/render/answerdoc.go.
//
// Design contract: the LLM emits AnswerDocument via one batched
// tool call, the evaluator runs shape-level structural validation
// plus the cardinality cross-check for list_of_symbols/complete
// slates, and the renderer produces deterministic prose. The four
// fake-green patterns are structurally impossible on this path.
//
// Cardinality cross-check: the emit_answer_document tool already
// validates per-item grounding (line > 0, file not in WorkDir,
// citation_ref in range). The baseline (TerminalEvidenceCount +
// AnalysisIR.AnswerContract.MustInclude) is not part of the tool
// schema, so the evaluator applies validateCompletenessClaim here
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

// maxFinalizerCorrectionRetries caps how many times the finalizer
// is allowed to re-invoke the LLM with a correction prompt when
// the LLM forgot to call emit_answer_document or produced an
// invalid document. Two is enough to resolve occasional slips
// without turning a stubborn model into an infinite loop.
const maxFinalizerCorrectionRetries = 2

// answerDocumentEvaluator is the Evaluator implementation for the
// finalize stage.
type answerDocumentEvaluator struct {
	// language is captured at BuildInitialInstruction time so ParseOutput
	// can pick the renderer locale without re-deriving it.
	language string

	// mu is captured at BuildInitialInstruction so ContinuationPrompt can
	// check whether the AnswerDocument tool call has landed before
	// issuing a correction retry. Without this, a content-only turn
	// that followed a successful tool call would burn a retry and
	// re-prompt the LLM to emit the tool call a second time,
	// clobbering the first document. The evaluator Evaluator
	// interface does not pass ctx to ContinuationPrompt, so the
	// pointer has to be stashed on the struct.
	mu *types.MutableState

	// retriesUsed tracks correction rounds across the ReAct loop,
	// bounded by maxFinalizerCorrectionRetries.
	retriesUsed int
}

// BuildInitialInstruction renders ONLY the dynamic per-dispatch data the
// answer-document-skill system sections cannot carry:
//
//   - Resolved target shape (depends on ctx.AnalysisIR / ctx.AnswerSymbols)
//   - Cardinality baseline for list_of_symbols (MustInclude list)
//   - Prior extraction slate (from explorer / extractor output)
//   - User question echo
//
// All STATIC contract — tool name, shape-field dispatch table,
// citation pool semantics, completeness honesty contract,
// prohibitions — lives in `answer-document-skill` in
// internal/skill/defaults.go and is rendered as system sections
// (Goal / Workflow / OutputFormat / Prohibitions) by
// context/builder.go before this dynamic block runs.
//
// Rationale: matches the pattern used by extractor.go where the
// evaluator stays slim (dynamic data only) and the static contract
// lives in the skill config. Contradicting directives between a
// declarative skill OutputFormat and a Go-string-builder prompt
// are a known footgun, so this evaluator deliberately never
// repeats the skill's contract text.
func (e *answerDocumentEvaluator) BuildInitialInstruction(ctx *types.AgentContext, sk *skill.Config) string {
	e.retriesUsed = 0
	e.language = extractAnswerDocLang(ctx)
	if ctx != nil {
		e.mu = ctx.Mutable
	}

	var b strings.Builder

	// User question is already rendered by builder.go as "User Request"
	// section — no need to repeat it here.

	shape := resolveAnswerDocShape(ctx)
	fmt.Fprintf(&b, "## Target shape (resolved from AnalysisIR)\n\n`%s`\n\n", shape)

	if shape == string(types.ShapeListOfSymbols) {
		must := []string(nil)
		if ctx != nil && ctx.AnalysisIR != nil {
			must = ctx.AnalysisIR.AnswerContract.MustInclude
		}
		b.WriteString("## Cardinality baseline (symbols_completeness floor)\n\n")
		if len(must) > 0 {
			fmt.Fprintf(&b, "Analyzer MustInclude (γ): **%d name(s)** — %s\n\n",
				len(must), strings.Join(must, ", "))
			fmt.Fprintf(&b,
				"A `symbols_completeness=complete` claim with fewer than %d items will be "+
					"DOWNGRADED to `lower_bound` automatically with a visible caveat in the "+
					"rendered answer. If you cannot reach the floor, choose `lower_bound` up "+
					"front — it is the honest terminal state.\n\n", len(must))
		} else {
			b.WriteString("Analyzer MustInclude (γ) is empty. No floor is enforced for this dispatch — ")
			b.WriteString("choose `complete` / `lower_bound` / `unknown` based on your own recall confidence.\n\n")
		}

		if ctx != nil && len(ctx.AnswerSymbols) > 0 {
			b.WriteString("## Prior slate from the extraction pipeline\n\n")
			b.WriteString("The upstream deterministic pipeline produced this answer-symbol list. ")
			b.WriteString("Use it as the starting point; adding items requires evidence from the ")
			b.WriteString("Evidence Items section, removing items requires a rationale in the ")
			b.WriteString("symbol's `rationale` field.\n\n")
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

// ShouldStop always returns false so the BaseAgent's soft-stop path
// delegates the accept/retry decision to ContinuationPrompt.
func (e *answerDocumentEvaluator) ShouldStop(resp llm.Response, iteration int) bool {
	return false
}

// Observe implements LoopController. The finalizer only has a
// soft-stop branch — the answer-document stage runs a handful of
// one-shot LLM turns, so there is nothing to detect mid-loop.
//
// Soft-stop policy: a populated AnswerDocument in Mutable means the
// emit_answer_document tool call has already landed and ParseOutput
// will render the prose — accept the soft-stop. A missing document
// within the retry budget triggers a correction hint; beyond the
// budget the evaluator stays silent and the stage's ParseOutput
// writes a fail-loud warning from the raw last LLM content.
//
// Evaluator-specific retry budget (retriesUsed vs
// maxFinalizerCorrectionRetries) is INTENTIONALLY kept here rather
// than delegated to LoopPolicy.MaxContinuations: it is a
// finalizer-specific contract ("try at most N correction prompts
// before fail-loud"), not a generic loop-wide cap. LoopPolicy's
// continuation cap still applies as an outer safety net.
func (e *answerDocumentEvaluator) Observe(_ *types.AgentContext, obs LoopObservation) LoopSignal {
	if obs.Phase == PhaseMidLoop {
		// emit_answer_document is the finalizer's terminal action —
		// once it fires, stop immediately instead of burning one extra
		// LLM round that would just produce a content-only soft-stop.
		if e.mu != nil {
			if doc := e.mu.AnswerDocument(); doc != nil && !doc.IsZero() {
				return LoopSignal{StopRequested: true, StopReason: "emit_answer_document called"}
			}
		}
		return LoopSignal{}
	}
	if obs.Phase != PhaseSoftStop {
		return LoopSignal{}
	}
	if e.mu != nil {
		if doc := e.mu.AnswerDocument(); doc != nil && !doc.IsZero() {
			return LoopSignal{}
		}
	}
	if e.retriesUsed >= maxFinalizerCorrectionRetries {
		logging.Debug("[finalizer/answer_document] correction retries exhausted (%d); accepting response",
			e.retriesUsed)
		return LoopSignal{}
	}
	e.retriesUsed++
	logging.Debug("[finalizer/answer_document] correction retry #%d: requesting emit_answer_document",
		e.retriesUsed)
	// HintKey embeds the retry counter so the LoopPolicy's dedup
	// window does NOT swallow the second retry. The finalizer's
	// retry budget is an evaluator-owned contract; dedup by a
	// shared key would silently truncate it to the first attempt.
	return LoopSignal{
		HintRequested: true,
		HintKey:       fmt.Sprintf("finalizer.missing_document.%d", e.retriesUsed),
		Hint: "You must produce the final answer by calling `emit_answer_document` exactly once. " +
			"Do not write prose outside the tool call. Review the shape-specific instructions in the " +
			"initial prompt and emit the tool call now.",
	}
}

// answerDocumentStageData is the JSON payload shape written into
// StageOutput.Data. Marshaling via a typed struct (rather than
// fmt.Sprintf with %q) keeps unicode escapes JSON-safe.
type answerDocumentStageData struct {
	FinalAnswer    string                `json:"final_answer"`
	AnswerDocument *types.AnswerDocument `json:"answer_document"`
}

// ParseOutput reads the AnswerDocument from Mutable, runs the
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
			caveat := "symbols list was marked complete but did not meet the cardinality baseline; downgraded to lower_bound"
			if e.language == "zh" {
				caveat = "符号列表声称完整但未达到基数基线，已自动降级为 lower_bound"
			}
			doc.Caveats = append(doc.Caveats, caveat)
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
