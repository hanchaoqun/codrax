package agent

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// renderAnalyzerPeerErrorClassificationBoundary exposes the validated
// top-level error topology before the analyzer chooses a request shape.  It is
// deliberately soft guidance: the model still owns intent and breadth.  The
// carrier prevents independently printed stacks from being mistaken for one
// source-code call chain merely because their messages or frame names look
// related.
func renderAnalyzerPeerErrorClassificationBoundary(ctx *types.AgentContext) string {
	if ctx == nil {
		return ""
	}
	bundle := ctx.LogTriage
	if bundle == nil && ctx.Mutable != nil {
		bundle = ctx.Mutable.LogTriage()
	}
	if bundle == nil || len(bundle.Errors) < 2 {
		return ""
	}
	return strings.Join([]string{
		"## Validated Runtime Error Topology",
		"",
		"- The attached artifact contains multiple top-level peer error occurrences. This topology is already validated; frames inside one occurrence preserve only that occurrence's stack order.",
		"- Do not classify a source-code call chain, wrapper chain, propagation path, or cross-stack root cause from adjacency, similar messages, language-bridge names, or frame order across peer occurrences.",
		"- Choose answer breadth from the current request. A request for each occurrence's type, message, or frame location is a coherent `bounded_fact_set` with `other_observed_value`; a genuine causal-diagnosis request may remain broad but must preserve that the cross-occurrence relation is unproven unless an explicit artifact relation exists.",
		"",
	}, "\n")
}

// runtimePeerErrorBoundedFactSet is the shared typed carrier for a finite
// per-occurrence runtime-log question. It deliberately reads neither the user
// request nor any model/final prose: the analyzer-owned runtime scope says the
// requested answer is a bounded fact set, while the validated LogBundle says
// the artifact contains multiple peer error occurrences.
func runtimePeerErrorBoundedFactSet(ctx *types.AgentContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil ||
		ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile == nil ||
		!ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.BoundedFactSet() {
		return false
	}
	bundle := ctx.LogTriage
	if bundle == nil && ctx.Mutable != nil {
		bundle = ctx.Mutable.LogTriage()
	}
	return bundle != nil && len(bundle.Errors) > 1
}

// renderExplorerPeerErrorFactScope keeps a finite peer-error investigation
// from widening its model-authored closure into a causal diagnosis. This is
// soft workflow guidance: it does not reject a tool call, inspect completion
// prose, or write any user-facing conclusion.
func renderExplorerPeerErrorFactScope(ctx *types.AgentContext) string {
	if !runtimePeerErrorBoundedFactSet(ctx) {
		return ""
	}
	return strings.Join([]string{
		"### Separate Runtime Error Stacks",
		"",
		"- The user requested a finite set of facts. Complete them independently for each top-level error stack, including that stack's own message, first observed frame, and within-stack callers when available.",
		"- No explicit artifact marker connects the peer stacks. Do not turn adjacency, similar wording, shared identifiers, or bridge-like names into a cross-stack root cause, propagation, caller/callee, or wrapper chain. If the relationship must be discussed, say that the log does not prove it.",
		"- Literal error-message wording remains an observation of that occurrence, not a relation edge to a peer occurrence.",
		"",
	}, "\n")
}
