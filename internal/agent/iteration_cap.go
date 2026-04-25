package agent

import "github.com/hanchaoqun/codrax/internal/llm"

// iterationCapDecision encodes the two-stage iteration cap pattern
// used by every evaluator that drives a "must call emit_X to fill a
// Mutable slot" loop (planner / verifier / coder / extractor).
//
// Background: BaseAgent.Execute calls Evaluator.ShouldStop BEFORE
// running the tools the LLM emitted in the current iteration. A flat
// `iteration >= cap` therefore discards the current iter's tool calls
// when the cap is hit. That is wrong for the streaming-truncation
// recovery scenario observed in production: streaming gateways
// occasionally truncate the function.arguments string of a large
// emit_X call (e.g. emit_change_plan with a 5 KB JSON payload), the
// tool rejects with "unexpected EOF", and the LLM's NEXT iteration
// is a clean retry. If that retry happens to land on the cap iter, a
// flat cap throws away the recovery before it ever runs.
//
// The fix is a soft+hard cap pair: at the soft cap, spare one
// execution when the LLM is actively calling one of the agent's
// recoveryTools; at the hard cap, stop unconditionally.
//
// Returns true when the loop SHOULD STOP at the given iteration.
func iterationCapShouldStop(resp llm.Response, iteration, softCap, hardCap int, recoveryTools ...string) bool {
	if iteration >= hardCap {
		return true
	}
	if iteration < softCap {
		return false
	}
	// At/past the soft cap: spare one more iteration when the LLM is
	// retrying one of the agent's structured-emit tools. The hard cap
	// above bounds the worst case so a stuck LLM cannot loop forever.
	for _, tc := range resp.ToolCalls {
		for _, name := range recoveryTools {
			if tc.Name == name {
				return false
			}
		}
	}
	return true
}
