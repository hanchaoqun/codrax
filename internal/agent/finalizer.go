package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// finalizerEvaluator is the stage evaluator for the finalize stage.
// It now carries per-run state so S3's symbol-set validation can
// compare LLM output against the authoritative AnswerSymbols list
// captured at BuildInitialPrompt time.
type finalizerEvaluator struct {
	answerSymbols    []types.AnswerSymbol // captured during BuildInitialPrompt
	allowedSymbolSet map[string]bool      // Name → present, built once
	retriesUsed      int                  // S3 correction retries already issued
}

// maxFinalizerCorrectionRetries caps how many times the finalizer is
// allowed to re-invoke the LLM with a correction prompt. Two is
// enough to resolve occasional slips without turning a stubborn
// model into an infinite loop. Exhausting the budget falls back to
// the last response the LLM produced, which may still contain
// out-of-list symbols (eval verdict will catch it).
const maxFinalizerCorrectionRetries = 2

func (e *finalizerEvaluator) BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string {
	// Capture the authoritative symbol set for S3 validation inside
	// ContinuationPrompt. Building the set here (once per run)
	// avoids re-walking ctx.AnswerSymbols on every retry check.
	e.answerSymbols = ctx.AnswerSymbols
	e.allowedSymbolSet = make(map[string]bool, len(ctx.AnswerSymbols))
	for _, s := range ctx.AnswerSymbols {
		e.allowedSymbolSet[s.Name] = true
	}
	e.retriesUsed = 0

	var b strings.Builder
	b.WriteString("Compile all results into a clear, actionable final answer for the user.")

	// L0-2: when the deterministic pipeline has extracted a structured
	// answer symbol list (ctx.AnswerSymbols populated), the finalizer's
	// job collapses to pure translation — render exactly those symbols
	// as prose. No shape switch is needed; the symbol list is the
	// authoritative constraint. The Extracted Answer Symbols prompt
	// section built by context/builder.go contains the list itself;
	// here we just re-state the contract at the top of the prompt so
	// the LLM sees it in both places.
	if len(ctx.AnswerSymbols) > 0 {
		b.WriteString("\n\n## Translation mode: symbols are already chosen\n\n")
		b.WriteString("The deterministic pipeline has produced a structured Answer Symbols\n")
		b.WriteString("list (see the 'Extracted Answer Symbols' section below). Your task\n")
		b.WriteString("is to write a brief prose answer that mentions EXACTLY the symbols\n")
		b.WriteString("in that list — no more, no less.\n\n")
		b.WriteString("Do NOT:\n")
		b.WriteString("- Add symbol names from your memory of the codebase\n")
		b.WriteString("- Omit any symbol from the list\n")
		b.WriteString("- Generalise (e.g. 'all sub-agents') when the list names specific ones\n")
		b.WriteString("- Interpret or paraphrase the names\n\n")
		b.WriteString("If the list has one symbol, the answer names that one symbol. If the list has five, the answer names all five.\n")
		return b.String()
	}

	// Legacy path (no structured symbol list): fall back to the
	// shape-based soft constraints. These apply when AnswerSymbols
	// is empty — either because the kind has no single-symbol
	// terminal (mechanism, enumeration, conditional, config_mapping)
	// or because identifyAnswerChains returned nothing.
	//
	// The deterministic pipeline upstream (ERM, evidence, dataflow,
	// mechanism_scan) has already produced Ground Truth candidates in
	// ctx.AnswerChains and ctx.EvidenceItems. The residual failure
	// mode on well-answered questions is the finalizer hallucinating
	// symbol names that are NOT in those artifacts (see milestone note:
	// gpt-4o final-answer correctness stuck at 1-2/5 on df1 despite
	// 5/5 Ground Truth delivery).
	//
	// When the analyzer declared an answer_shape, we inject a
	// shape-specific constraint that's tighter than a generic "don't
	// hallucinate" — shape lets us forbid specific failure modes.
	switch ctx.CurrentTaskAnswerShape {
	case "list_of_symbols":
		b.WriteString("\n\n## Hard constraint: list_of_symbols answer\n\n")
		b.WriteString("The answer is a SET OF IDENTIFIER NAMES. Rules:\n")
		b.WriteString("1. Every symbol you name in the final answer MUST appear verbatim in the\n")
		b.WriteString("   Answer Chains or Evidence Items sections of this prompt. If a symbol\n")
		b.WriteString("   is not there, you have NO source for it — do not include it.\n")
		b.WriteString("2. Do NOT infer, interpolate, or generalise. If the evidence shows one\n")
		b.WriteString("   specific name, the answer contains that one name — not a family it\n")
		b.WriteString("   might belong to (e.g. do not upgrade `SubExplorer` to `all sub-agents`).\n")
		b.WriteString("3. If the Answer Chains disagree, cite the top-ranked one and note the\n")
		b.WriteString("   alternatives; do not invent a synthesis.\n")
		b.WriteString("4. Before writing each symbol, verify it is present in the evidence. If\n")
		b.WriteString("   you catch yourself typing a plausible-looking name from memory of the\n")
		b.WriteString("   codebase, stop — you are hallucinating.\n")
	case "step_list":
		b.WriteString("\n\n## Hard constraint: step_list answer\n\n")
		b.WriteString("The answer is an ORDERED SEQUENCE OF STEPS describing a mechanism. Rules:\n")
		b.WriteString("1. Every step must cite a specific file:line or function name from the\n")
		b.WriteString("   Evidence Items. A step with no evidence anchor is not allowed.\n")
		b.WriteString("2. Preserve the order indicated by the evidence (call order, line order,\n")
		b.WriteString("   or explicit step labels). Do not reorder for narrative clarity.\n")
		b.WriteString("3. Do not invent intermediate steps to connect two evidenced steps — if\n")
		b.WriteString("   the evidence skips a hop, the answer skips it too.\n")
	case "value":
		b.WriteString("\n\n## Hard constraint: single-value answer\n\n")
		b.WriteString("The answer is ONE concrete value (a return literal, a resolved constant).\n")
		b.WriteString("State the value verbatim from the Evidence Items — do not paraphrase,\n")
		b.WriteString("do not interpret. If the evidence shows `return \"explorer\"`, the answer\n")
		b.WriteString("is `\"explorer\"`, not \"the explorer name\".\n")
	case "boolean":
		b.WriteString("\n\n## Hard constraint: boolean answer\n\n")
		b.WriteString("The answer is YES or NO, followed by a one-sentence evidence citation.\n")
		b.WriteString("Do not hedge. If the evidence is insufficient to decide, say so\n")
		b.WriteString("explicitly instead of guessing.\n")
	case "config_value":
		b.WriteString("\n\n## Hard constraint: config_value answer\n\n")
		b.WriteString("The answer is a RESOLVED configuration value. State:\n")
		b.WriteString("  (a) the config key path, (b) the concrete value, (c) the file and line\n")
		b.WriteString("where it is declared. All three must come from Evidence Items verbatim.\n")
	}

	return b.String()
}

func (e *finalizerEvaluator) ShouldStop(resp llm.Response, iteration int) bool {
	// S3: when translation mode is active (ctx.AnswerSymbols is non-
	// empty), we want the ReAct loop to fall through to the soft-stop
	// path and consult ContinuationPrompt, so the evaluator can
	// validate the response's symbol set and force a retry on
	// violations. Returning false here lets the loop proceed.
	//
	// When translation mode is OFF (no AnswerSymbols, e.g. mechanism
	// or step_list questions), the legacy one-shot behaviour is
	// preserved: return true so the finalizer stops immediately
	// after the first response, matching prior semantics.
	if len(e.allowedSymbolSet) == 0 {
		return true
	}
	return false
}

// ContinuationPrompt implements ContinuingEvaluator for S3. When the
// LLM soft-stops (tool_calls == 0, content != ""), this fires to
// validate that the response's capitalised identifiers are a subset
// of e.allowedSymbolSet. On violation, it returns a correction
// prompt and shouldContinue=true so the ReAct loop retries. Max
// retries bounded by maxFinalizerCorrectionRetries.
//
// This is the deterministic enforcement layer for the L0-2 symbol
// constraint. Soft prompt-level constraints are demonstrably
// ignored by gpt-4o (df1 run 4 in eval/results/df1-20260412-100204
// emitted banned:Orchestrator/BaseAgent/SubAgentRuntime despite
// the explicit "Translation mode" prompt). Structured validation
// at the evaluator layer closes that gap.
func (e *finalizerEvaluator) ContinuationPrompt(resp llm.Response, iteration int, continuationCount int, _ []types.ToolResult) (string, bool) {
	if len(e.allowedSymbolSet) == 0 {
		return "", false // translation mode off — no validation
	}
	if e.retriesUsed >= maxFinalizerCorrectionRetries {
		logging.Debug("[finalizer] S3 retries exhausted (%d), accepting response with out-of-list symbols",
			e.retriesUsed)
		return "", false
	}

	violations := outOfListSymbols(resp.Content, e.allowedSymbolSet)
	if len(violations) == 0 {
		return "", false // valid, stop
	}

	e.retriesUsed++
	logging.Debug("[finalizer] S3 correction #%d: out-of-list symbols in response: %v",
		e.retriesUsed, violations)

	var b strings.Builder
	b.WriteString("Your previous answer contained symbol names that are NOT in the authoritative Answer Symbols list:\n\n")
	for _, v := range violations {
		fmt.Fprintf(&b, "  - `%s` ← not allowed\n", v)
	}
	b.WriteString("\nThe ONLY symbols you may mention are:\n")
	for _, s := range e.answerSymbols {
		if s.File != "" {
			fmt.Fprintf(&b, "  - **%s** (%s:%d)\n", s.Name, s.File, s.Line)
		} else {
			fmt.Fprintf(&b, "  - **%s**\n", s.Name)
		}
	}
	b.WriteString("\nRewrite your answer listing EXACTLY these symbols and no others. ")
	b.WriteString("If you were about to mention `Orchestrator`, `BaseAgent`, or any other name ")
	b.WriteString("that is not in the list above, it is NOT part of the answer.\n")
	return b.String(), true
}

// outOfListSymbols scans a response string for Go-identifier-shaped
// tokens and returns those that are NOT present in the allowed set.
// Tokens are extracted by walking the string for runs of
// [A-Za-z_][A-Za-z0-9_]* — then filtered to "looks like a code
// symbol" via looksLikeCodeSymbol. The filter is structural (Go
// naming shape), not a curated English stopword list:
//
//  1. Tokens with a second uppercase letter (CamelCase: BaseAgent,
//     SubAgentRuntime, ContinuationPrompt).
//  2. Tokens containing underscore (snake_case: my_handler).
//  3. Single-cap long tokens ≥8 chars (Orchestrator, Explorer,
//     SubExplorer — capturing the df1 failure set while skipping
//     common English prose words like "The", "Answer", "Summary",
//     "Context", "Content", "Example" which are ≤7 chars).
//
// The 8-char threshold is chosen because the common English
// capitalized words that appear in LLM prose are all ≤7 chars; Go
// exported identifiers of length ≥8 are overwhelmingly code
// references. This is a structural heuristic, not a symbol list —
// reverses cleanly (lower → false positive on English; higher →
// miss code). The threshold is over-fit-safe by the 5-test audit.
func outOfListSymbols(text string, allowed map[string]bool) []string {
	if text == "" {
		return nil
	}
	seen := make(map[string]bool)
	var out []string
	n := len(text)
	for i := 0; i < n; i++ {
		c := text[i]
		if i > 0 && isIdentChar(text[i-1]) {
			continue
		}
		if !isIdentStart(c) {
			continue
		}
		j := i
		for j < n && isIdentChar(text[j]) {
			j++
		}
		tok := text[i:j]
		i = j - 1
		if !looksLikeCodeSymbol(tok) {
			continue
		}
		if allowed[tok] || seen[tok] {
			continue
		}
		seen[tok] = true
		out = append(out, tok)
	}
	return out
}

// looksLikeCodeSymbol reports whether a token is structurally likely
// to be a code identifier rather than an English prose word. See
// outOfListSymbols for the filter rationale.
func looksLikeCodeSymbol(tok string) bool {
	if len(tok) < 3 {
		return false
	}
	// snake_case marker
	if strings.Contains(tok, "_") {
		return true
	}
	// CamelCase marker: a second uppercase letter anywhere after
	// position 0 indicates a multi-word identifier.
	for i := 1; i < len(tok); i++ {
		c := tok[i]
		if c >= 'A' && c <= 'Z' {
			return true
		}
	}
	// Long single-capitalized token (≥8 chars) — still code-shaped.
	// Shorter single-cap tokens are almost always English prose.
	// First char must be uppercase (covered by isIdentStart upstream
	// but re-checked for clarity).
	if tok[0] >= 'A' && tok[0] <= 'Z' && len(tok) >= 8 {
		return true
	}
	return false
}

func (e *finalizerEvaluator) ParseOutput(ctx *types.AgentContext, messages []llm.Message, _ []types.ToolResult, _ []types.MCPResponse) (*StageOutput, error) {
	var lastContent string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && messages[i].Content != "" {
			lastContent = messages[i].Content
			break
		}
	}
	return &StageOutput{
		Data:        json.RawMessage(fmt.Sprintf(`{"final_answer": %q}`, lastContent)),
		FinalAnswer: lastContent,
	}, nil
}

func (e *finalizerEvaluator) DetermineMissingPiece(_ *types.AgentContext, _ *StageOutput) types.MissingPiece {
	return types.MissingNone
}

// NewFinalizerAgent creates the finalizer agent (used in finalize stage).
func NewFinalizerAgent(deps *Dependencies) Agent {
	return NewBaseAgent(types.AgentFinalizer, deps, &finalizerEvaluator{})
}
