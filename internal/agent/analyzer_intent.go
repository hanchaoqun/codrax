package agent

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// analyzer_intent.go holds a deterministic sanity rule that patches
// the most common intent misclassification we see in production: the
// LLM treats a "count / how many / 统计" question as IntentEnumerate
// and downstream the compiler picks ShapeListOfSymbols with a
// citation_count_ge=1 floor (see
// internal/analysis/compiler/templates.go:516-517). The scalar answer
// that the investigation legitimately produces cannot satisfy either
// the bulleted-symbols shape check or the file:line citation floor,
// and the contract-retry loop burns its budget on a mismatch no
// amount of re-investigation can fix.
//
// The rule mirrors the discipline of analyzer_complexity.go: fire only
// on strong structural signals, leave the LLM's choice untouched in
// every borderline case. Specifically:
//
//   - Triggers ONLY when the LLM picked IntentEnumerate; other intents
//     are left alone (the rule cannot create an enumerate false negative).
//   - Triggers ONLY when the request's leading clause matches a count-
//     verb cue — not any position mid-prompt. "list handlers that
//     count requests" is not touched because "list" is the prefix;
//     "count" appears mid-clause.
//   - Triggers ONLY when Complexity is simple (after reconcileComplexity
//     has already had its say). Moderate / complex enumerations can
//     legitimately include counting language ("list all files and
//     report sizes") and downgrading would lose the enumeration half.

// isMeasurementScalarRequest reports whether the request is asking for
// a single scalar produced by a tool query — "how many X", "统计 …",
// "total of Y" — where the answer has no file:line to cite.
//
// The check is independent of whether the LLM initially picked
// IntentEnumerate (then reconcileIntent downgrades) or IntentReturnValue
// (already correct) — both populations need the three-citation-gate
// carve-out in buildAnalysisIR. Previously the carve-out keyed off
// "reconcileIntent fired" which missed the case where the LLM nailed
// the intent directly; the citation gates stayed enabled and the
// retry budget looped the same way as the original bug.
//
// Three gates in force, all fire equally for both populations:
//
//   1. ComplexitySimple  — moderate/complex count-style questions
//                          can have legitimate file:line evidence
//                          ("list all files > N lines and their
//                          counts") so we don't strip gates there.
//   2. Intent in {enumerate, return_value} — other intents
//                          (explain / trace / root_cause) are never
//                          measurement-scalar even if the prose
//                          starts with a count verb.
//   3. Leading count-verb prefix — "list handlers that count requests"
//                          does not fire because "list" is the prefix;
//                          only first-verb cues count.
func isMeasurementScalarRequest(rm types.RequestModel, rawRequest string) bool {
	if rm.Complexity != types.ComplexitySimple {
		return false
	}
	if rm.Intent != types.IntentEnumerate && rm.Intent != types.IntentReturnValue {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(rawRequest))
	lower = stripPolitenessPrefix(lower)
	return hasLeadingCountVerb(lower)
}

// reconcileIntent returns the intent that should travel downstream and
// a short reason string. When resolved == declared the rule did not
// fire and reason is empty. Runs AFTER reconcileComplexity so the
// complexity input is post-reconcile.
//
// This is the strict downgrade contract (enumerate → return_value on
// count-verb prefix). For the broader "is this a measurement scalar
// question regardless of declared intent" check that drives the
// citation-gate carve-out, see isMeasurementScalarRequest above.
func reconcileIntent(declared types.Intent, rawRequest string, complexity types.Complexity) (types.Intent, string) {
	if declared != types.IntentEnumerate {
		return declared, ""
	}
	if complexity != types.ComplexitySimple {
		return declared, ""
	}
	lower := strings.ToLower(strings.TrimSpace(rawRequest))
	lower = stripPolitenessPrefix(lower)
	if !hasLeadingCountVerb(lower) {
		return declared, ""
	}
	return types.IntentReturnValue,
		"leading count-verb cue (how many / 统计 / count / total) — answer is a scalar, not a list"
}

// countVerbPrefixes are matched at the start of the (trimmed,
// politeness-stripped) request. HasPrefix matching is what keeps
// "list handlers that count requests" safe — "list" is the prefix,
// "count" is mid-clause and does not fire.
//
// Keep this list short on purpose — every entry must be a phrase a
// real user has been observed to ask as a count question. Curation
// discipline: do NOT add a token because it "could" be count-like;
// add it when a failing eval run shows a miss. This matches the
// curation pattern of simpleLookupCues / crossComponentCues in
// analyzer_complexity.go.
var countVerbPrefixes = []string{
	// Chinese — note no trailing space: Chinese tokens are not
	// space-delimited, so HasPrefix against the concatenated verb
	// is the correct shape.
	"统计",
	"数一下",
	"数下",
	"算一下",
	"算下",
	"有多少",
	"多少个",
	"多少行",
	"总共",
	"一共",
	// English — trailing space forces whole-word prefix match.
	"how many ",
	"how much ",
	"count ",
	"number of ",
	"total ",
	"size of ",
	"tally ",
	"what's the total",
	"what is the total",
	"what's the count",
	"what is the count",
	"what's the size",
	"what is the size",
}

func hasLeadingCountVerb(lower string) bool {
	for _, p := range countVerbPrefixes {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}

// politenessPrefixes are stripped once before the leading-verb check
// so "please count X" and "请统计 X" still fire. Kept deliberately
// minimal — no recursive strip (if a user writes "please could you
// count ..." we accept the miss rather than chain strips).
var politenessPrefixes = []string{
	"please ",
	"could you ",
	"can you ",
	"请帮我",
	"请帮",
	"请",
	"帮我",
	"帮忙",
	"麻烦",
}

func stripPolitenessPrefix(lower string) string {
	for _, p := range politenessPrefixes {
		if strings.HasPrefix(lower, p) {
			return strings.TrimSpace(lower[len(p):])
		}
	}
	return lower
}

// logIntentReconcile is the twin of logComplexityReconcile — one
// warning line when the rule overrode the LLM's pick, silent no-op
// otherwise. Matching log levels let operators grep a single trace
// for "[analyzer] * reconciled:" to find every automatic override.
func logIntentReconcile(before, after types.Intent, reason string) {
	if before == after || reason == "" {
		return
	}
	logging.Warning("[analyzer] intent reconciled: %s → %s (%s)", before, after, reason)
}
