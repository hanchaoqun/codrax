package agent

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// analyzer_predicate.go extracts a PredicateAxis from the user's
// question by matching against a multi-language verb-cue table.
// The axis is a deterministic, project-generic classification of the
// question's action direction — "how does X CALL Y" → AxisCall, "how
// is X REGISTERED" → AxisRegister — and is consumed downstream by
// the evidence ranker (via internal/analysis/axis.Affinity) to bias
// evidence items whose AnchorKind matches the axis.
//
// The implementation mirrors the curation discipline of
// relationalVerbCues in analyzer_intent.go: English cues are padded
// with leading+trailing spaces for whole-word matching; Chinese cues
// are bare substrings because tokens are not space-delimited. The
// cue list is intentionally short — each entry must be an observed
// real-user question pattern, not a speculative addition.

// predicateVerbMap maps verb cues to the axis they signal.
// Duplicates across axes are allowed — the axisPriority tie-breaker
// picks the most specific match. English cues carry leading AND
// trailing spaces so a padded haystack scan never fires inside a
// larger word.
var predicateVerbMap = map[string]types.PredicateAxis{
	// AxisCall — dispatch/invocation verbs. Highest priority because
	// "how does X call Y" is the canonical mechanism-question shape
	// and the axis a verb-agnostic extractor is most likely to miss.
	" calls ":       types.AxisCall,
	" call ":        types.AxisCall,
	" calling ":     types.AxisCall,
	" invokes ":     types.AxisCall,
	" invoke ":      types.AxisCall,
	" invoking ":    types.AxisCall,
	" invoked ":     types.AxisCall,
	" dispatches ":  types.AxisCall,
	" dispatch ":    types.AxisCall,
	" dispatching ": types.AxisCall,
	"调用":            types.AxisCall,
	"分发":            types.AxisCall,
	"调度":            types.AxisCall,

	// AxisRegister — registration/binding verbs.
	" registers ":  types.AxisRegister,
	" register ":   types.AxisRegister,
	" registered ": types.AxisRegister,
	" binds ":      types.AxisRegister,
	" bind ":       types.AxisRegister,
	" wires ":      types.AxisRegister,
	" wire ":       types.AxisRegister,
	"注册":           types.AxisRegister,
	"绑定":           types.AxisRegister,

	// AxisDefine — definition/declaration verbs.
	" defines ":  types.AxisDefine,
	" define ":   types.AxisDefine,
	" defined ":  types.AxisDefine,
	" declares ": types.AxisDefine,
	" declare ":  types.AxisDefine,
	" declared ": types.AxisDefine,
	" what is ":  types.AxisDefine,
	"定义":         types.AxisDefine,
	"声明":         types.AxisDefine,
	"什么是":        types.AxisDefine,

	// AxisReturn — return-value verbs.
	" returns ":  types.AxisReturn,
	" return ":   types.AxisReturn,
	" produces ": types.AxisReturn,
	" produce ":  types.AxisReturn,
	" yields ":   types.AxisReturn,
	"返回":         types.AxisReturn,
	"产生":         types.AxisReturn,

	// AxisConfigure — configuration verbs.
	" configures ": types.AxisConfigure,
	" configure ":  types.AxisConfigure,
	" configured ": types.AxisConfigure,
	" sets ":       types.AxisConfigure,
	"配置":          types.AxisConfigure,
	"设置":          types.AxisConfigure,

	// AxisCondition — branch/guard verbs. "when" is ambiguous in
	// English so we key off whole-word padded forms; Chinese cues
	// are bare substrings. " set " is omitted here because it
	// overlaps with AxisConfigure too heavily in short prompts.
	" when ":       types.AxisCondition,
	" whether ":    types.AxisCondition,
	" conditions ": types.AxisCondition,
	" condition ":  types.AxisCondition,
	" triggered ":  types.AxisCondition,
	" triggers ":   types.AxisCondition,
	"条件":           types.AxisCondition,
	"什么时候":         types.AxisCondition,
	"何时":           types.AxisCondition,
	"触发":           types.AxisCondition,

	// AxisImplement — implementation/provide verbs.
	" implements ":   types.AxisImplement,
	" implement ":    types.AxisImplement,
	" implementing ": types.AxisImplement,
	" implemented ":  types.AxisImplement,
	" provides ":     types.AxisImplement,
	" provide ":      types.AxisImplement,
	" handles ":      types.AxisImplement,
	" handle ":       types.AxisImplement,
	"实现":             types.AxisImplement,
	"提供":             types.AxisImplement,
	"处理":             types.AxisImplement,
}

// axisPriority breaks ties when the request matches cues from
// multiple axes. Order expresses specificity — the most specific
// action-question wins. Rationale: "how does X call Y" (AxisCall)
// beats "how is X implemented" (AxisImplement) because the former
// asks about a specific runtime action while the latter is a
// broader definitional question.
var axisPriority = []types.PredicateAxis{
	types.AxisCall,
	types.AxisRegister,
	types.AxisCondition,
	types.AxisReturn,
	types.AxisConfigure,
	types.AxisDefine,
	types.AxisImplement,
}

// extractPredicateAxis scans the request for verb cues and returns
// the highest-priority axis. Returns AxisUnknown when no cue
// matches — downstream consumers key off this to skip the axis
// boost entirely and preserve historical ranking.
//
// Case-insensitive for English (request is lowercased before
// scanning). Chinese matching is literal (token case does not apply).
// The haystack is padded with a space on both sides so English cue
// whole-word matches work at the request boundaries.
func extractPredicateAxis(rawRequest string) types.PredicateAxis {
	if strings.TrimSpace(rawRequest) == "" {
		return types.AxisUnknown
	}
	lower := strings.ToLower(strings.TrimSpace(rawRequest))
	padded := " " + lower + " "

	hits := make(map[types.PredicateAxis]bool, len(axisPriority))
	for cue, axis := range predicateVerbMap {
		if strings.Contains(padded, cue) {
			hits[axis] = true
		}
	}
	for _, axis := range axisPriority {
		if hits[axis] {
			return axis
		}
	}
	return types.AxisUnknown
}

// reconcilePredicateAxis returns the axis that should travel
// downstream along with a short reason string. When declared is
// already non-zero, it is preserved (the analyzer schema does not
// emit an axis today but reserving the precedence rule keeps a
// future LLM-emitted value safe). When declared is zero, the
// deterministic verb-extractor fills it; when the extractor
// returns AxisUnknown too, the field stays empty.
//
// Unlike reconcileIntent / reconcileComplexity this function never
// OVERRIDES a filled value — it only FILLS an empty one. Axis is
// purely additive; a wrong LLM-emitted axis would not corrupt any
// existing gate because the ranker simply returns 1.0 for unknown
// combos in the affinity matrix.
func reconcilePredicateAxis(declared types.PredicateAxis, rawRequest string) (types.PredicateAxis, string) {
	if declared != types.AxisUnknown {
		return declared, ""
	}
	axis := extractPredicateAxis(rawRequest)
	if axis == types.AxisUnknown {
		return declared, ""
	}
	return axis, "verb cue in request mapped to axis=" + string(axis)
}

// logPredicateAxisReconcile mirrors logIntentReconcile: one info
// line when the rule filled an empty axis, silent otherwise. The
// "[analyzer] * reconciled:" prefix matches the other analyzer
// reconciliation log lines so operators can grep a single trace
// for every deterministic override.
func logPredicateAxisReconcile(before, after types.PredicateAxis, reason string) {
	if before == after || reason == "" {
		return
	}
	logging.Info("[analyzer] predicate_axis reconciled: %q → %q (%s)", before, after, reason)
}
