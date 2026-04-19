package agent

import (
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// analyzer_intent.go holds the schema-v4 reconcile rules for Intent /
// AnswerSubject / AnswerShape. After the v4 rewrite, every prose-cue
// table that this file used to host has been deleted — the
// classification signal now comes from the LLM-emitted
// SemanticPredicates and PredicateAxis on RequestModel, which work
// across every language the user might write in. The deleted tables
// (countVerbPrefixes, enumerationCuePrefixes, categoryEnumerationCues,
// relationalVerbCues, politenessPrefixes, predicateVerbMap) only
// covered ZH+EN and required per-language curation.
//
// Each reconcile function below dispatches on RequestModel.Predicates
// and complementary structural signals (entity counts, sub-topic
// counts) — never on raw prose. Self-consistency between predicates
// and the LLM's intent / shape is enforced upstream by
// validateSelfConsistency in emit_analysis.

// isMeasurementScalarRequest reports whether the request is asking for
// a single scalar produced by a tool query (count / total / size)
// where the answer has no file:line to cite, and the citation gate
// should be lifted. The driving signal is the LLM-emitted
// is_count_question predicate — the LLM judges this for any language.
//
// Why is_count_question and not is_scalar_answer: a scalar answer
// like "the version string is 1.2.3" CAN be cited (the const lives
// at a file:line), so the gate should stay on. Only count / measurement
// questions ("how many X", "total bytes of Y") have no file:line to
// cite — that's exactly what is_count_question marks.
//
// The complexity / intent gates the v3 implementation layered on top
// of the prose check are gone — self-consistency in emit_analysis
// already rejects (is_count_question=true + intent=enumerate) and
// (is_count_question=true + shape=list_of_symbols), so by the time
// this function runs the combination is internally consistent.
func isMeasurementScalarRequest(rm types.RequestModel) bool {
	return rm.Predicates.IsCountQuestion
}

// reconcileIntent is preserved as a thin sanity check that traps the
// rare case where the LLM marked is_count_question=true but still
// picked intent=enumerate. validateSelfConsistency in emit_analysis
// already rejects this combination upstream, so by the time
// reconcileIntent runs it should be a no-op. The function survives
// only as a defense-in-depth assertion: if a future schema change
// loosens the upstream check, the analyzer still produces the
// correct downstream behaviour.
//
// Returns the resolved intent + a short reason string. When resolved
// == declared, the rule did not fire and reason is empty.
func reconcileIntent(declared types.Intent, preds types.SemanticPredicates) (types.Intent, string) {
	if declared == types.IntentEnumerate && preds.IsCountQuestion {
		return types.IntentReturnValue,
			"predicates.is_count_question=true overrides intent=enumerate (defense-in-depth; should be caught by self-consistency)"
	}
	return declared, ""
}

// inferSecondaryKinds returns RequirementKinds that should be tracked
// alongside the analyzer's primary question_kind when the LLM's
// predicates indicate the question structurally implies multiple
// kinds of investigation.
//
// The analyzer schema forces a single question_kind per emit, so a
// hybrid question like "pipeline has how many states" gets tagged
// either enumeration or return_value but not both. The
// is_category_enumeration predicate marks the case where the user
// wants each category named even though the literal ask is a count;
// in that case ERM thresholds for ReqEnumeration apply alongside
// whatever primary kind the LLM picked.
//
// Returns kinds in a stable order; the caller is responsible for
// de-duping against whatever primary kind is already in the ERM.
func inferSecondaryKinds(preds types.SemanticPredicates) []types.RequirementKind {
	var out []types.RequirementKind
	if preds.IsCategoryEnumeration {
		out = append(out, types.ReqEnumeration)
	}
	return out
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

// ── AnswerSubject inference + Shape reconciliation ───────────────────
//
// inferAnswerSubject and reconcileShape are the CGEC additions to the
// analyzer's deterministic post-processing chain. They mirror the
// reconcileIntent / reconcileComplexity pattern: fire only on strong
// signals, log every override under "[analyzer] * reconciled:" so
// operators can grep one trace for every automatic decision, and
// leave the LLM's choice untouched in every borderline case.
//
// Why both rules:
//
//   inferAnswerSubject classifies WHAT KIND of code-literal the
//   answer is supposed to be (skill_name, agent_name, config_key,
//   ...). The chain ranker uses this to demote chains whose terminal
//   token is the wrong kind ("SubExplorer.Name() returns 'explorer'"
//   should not rank highest when the question asks for a SKILL).
//
//   reconcileShape handles the corner where the LLM picked
//   ShapeConfigValue (key=value pair) but the actual answer is a Go
//   struct-field literal — e.g. the topology.go map literal that
//   binds "explore-skill" to the explorer agent. Forcing config_value
//   shape on a Go literal manufactures a fake "key" the LLM has to
//   invent, which downstream contract checks then reject.

// inferAnswerSubject derives AnswerSubject when the LLM left it zero
// (SubjectUnknown). Returns the resolved subject + a short reason
// string for the log line. An empty Kind in the result means no rule
// fired and the caller should keep the LLM's value.
//
// Schema-v4 simplification: the cue-match step is gone (the table was
// already empty after the session-11 audit). Inference now reads the
// LLM's typed question_kind enum and maps to a kind via a small case
// table — enum-to-enum, no prose substring matching, language-neutral.
//
// Order:
//  1. Honour LLM-supplied AnswerSubject when present (high signal).
//  2. Fallback by question_kind enum.
//  3. Hard fallback to SubjectGeneric so downstream consumers stay
//     in "passive-on" mode (the weakest kind that still keeps the
//     ranker / reconcile / pre-complete code paths active).
func inferAnswerSubject(rm types.RequestModel) (types.AnswerSubject, string) {
	if rm.AnswerSubject.Kind != types.SubjectUnknown {
		return rm.AnswerSubject, ""
	}
	switch rm.AnalyzerHints.Kind {
	case "config_mapping":
		return types.AnswerSubject{Kind: types.SubjectConfigKey, EntityAxes: []string{"key → value"}, Confidence: 0.4},
			"question_kind=config_mapping → ConfigKey"
	case "registration":
		return types.AnswerSubject{Kind: types.SubjectGeneric, Confidence: 0.3},
			"question_kind=registration → Generic"
	case "return_value":
		return types.AnswerSubject{Kind: types.SubjectReturnValue, EntityAxes: []string{"function → value"}, Confidence: 0.4},
			"question_kind=return_value → ReturnValue"
	case "call_chain":
		return types.AnswerSubject{Kind: types.SubjectFunctionName, EntityAxes: []string{"behavior → function"}, Confidence: 0.4},
			"question_kind=call_chain → FunctionName"
	case "enumeration":
		return types.AnswerSubject{Kind: types.SubjectGeneric, Confidence: 0.3},
			"question_kind=enumeration → Generic"
	case "mechanism", "conditional":
		return types.AnswerSubject{Kind: types.SubjectGeneric, Confidence: 0.2},
			"question_kind=" + rm.AnalyzerHints.Kind + " → Generic"
	}
	return types.AnswerSubject{Kind: types.SubjectGeneric, Confidence: 0.1},
		"hard fallback: question_kind missing — defaulting to Generic (weakest kind)"
}

// reconcileShape consolidates three shape-rewriting rules that used
// to live in separate prose-cue checks. Inputs are the LLM-emitted
// shape, the LLM predicates, and the resolved AnswerSubject. The
// raw request is no longer consulted — every signal comes from
// typed fields.
//
// Rules (in priority order):
//
//   - Rule 1a: conditional-enumeration lift. LLM picked Value /
//     ConfigValue but predicates indicate a filtered count (count or
//     category enumeration combined with a relational lookup). The
//     answer is a list whose length is the count; ShapeListOfSymbols
//     gives the finalizer a symbols[] to count and name.
//   - Rule 1b: category enumeration on a scalar shape. LLM picked
//     Value / ConfigValue but predicates.is_category_enumeration is
//     true. Each category deserves a named row.
//   - Rule 2: config_value → value when the AnswerSubject is a
//     source-code literal (function / type / interface / handler /
//     return value) rather than a YAML key.
//
// Returns resolved shape + reason; empty reason means no override.
func reconcileShape(declared types.AnswerShape, subject types.AnswerSubject, preds types.SemanticPredicates) (types.AnswerShape, string) {
	// Rule 1: scalar shapes lifted to list when predicates say the
	// answer is really a filtered set or a discrete category list.
	if declared == types.ShapeValue || declared == types.ShapeConfigValue {
		// Rule 1a: filtered count — count + relational lookup, OR
		// category + relational lookup. Either combination implies
		// the answer is a list whose len is the count.
		if (preds.IsCountQuestion || preds.IsCategoryEnumeration) && preds.IsRelationalLookup {
			return types.ShapeListOfSymbols,
				"predicates indicate a filtered count (count/category + relational lookup) — use list_of_symbols so count = len(symbols)"
		}
		// Rule 1b: category enumeration — each kind deserves a row
		// even without a relational filter.
		if preds.IsCategoryEnumeration {
			return types.ShapeListOfSymbols,
				"predicates.is_category_enumeration=true — each kind deserves a named table row"
		}
	}
	// Rule 2: config_value → value when the answer subject is a
	// source-code literal rather than a YAML key.
	if declared != types.ShapeConfigValue {
		return declared, ""
	}
	switch subject.Kind {
	case types.SubjectFunctionName, types.SubjectTypeName,
		types.SubjectInterface, types.SubjectHandlerRoute,
		types.SubjectReturnValue:
		return types.ShapeValue,
			"answer subject is source-code literal (" + string(subject.Kind) + ") not a YAML config key"
	}
	return declared, ""
}

// logSubjectInferred + logShapeReconciled are the structural twins of
// logIntentReconcile. One warning line per actual override. Silent
// when the rule did not fire.
func logSubjectInferred(subject types.AnswerSubject, reason string) {
	if subject.Kind == types.SubjectUnknown || reason == "" {
		return
	}
	logging.Warning("[CGEC] E1 subject_inferred: kind=%s axes=%v conf=%.2f (%s)",
		subject.Kind, subject.EntityAxes, subject.Confidence, reason)
}

func logShapeReconciled(before, after types.AnswerShape, reason string) {
	if before == after || reason == "" {
		return
	}
	logging.Warning("[analyzer] shape reconciled: %s → %s (%s)", before, after, reason)
}
