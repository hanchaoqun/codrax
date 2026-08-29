package agent

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

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
