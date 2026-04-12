package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

type finalizerEvaluator struct{}

func (e *finalizerEvaluator) BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string {
	var b strings.Builder
	b.WriteString("Compile all results into a clear, actionable final answer for the user.")

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
	// Finalizer typically doesn't need tools — stop after first response
	return true
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
