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
// should be lifted. The primary signal is the LLM-emitted
// is_count_question predicate — the LLM judges this for any language.
//
// Structural coherence fallback. When the LLM emits
// is_count_question=false but three independent structural signals
// coincide — answer_shape=value, intent=return_value, and
// answer_subject.kind=numeric — the triple together describes a single
// numeric answer to a return-value question, which is the same
// population the primary signal targets. The fallback catches the
// case where the LLM was internally inconsistent (emitted all three
// structural signals for a measurement-scalar question but still
// picked is_count_question=false). Pattern-consistent with
// reconcileShape, which already fuses multiple LLM signals to override
// a single field; no surface-word matching.
//
// Over-trigger tradeoff. A citable-numeric-constant question
// ("default value of MAX_STEPS") also satisfies the triple; for that
// population the carve-out strips citation enforcement even though a
// file:line exists. The LLM typically self-cites when it can, so the
// net effect is a soft gate, not a corrupted answer. The alternative
// (no fallback) makes a misclassified measurement-scalar question
// exhaust the retry budget on an unfixable citation gap, which is
// strictly worse.
//
// Upstream self-consistency (validateSelfConsistency in
// emit_analysis) already rejects (is_count_question=true +
// intent=enumerate) and (is_count_question=true +
// shape=list_of_symbols); this function runs on a LLM output that has
// already cleared those checks.
func isMeasurementScalarRequest(rm types.RequestModel) bool {
	if rm.Predicates.IsCountQuestion {
		return true
	}
	shape := mapLegacyAnswerShape(rm.AnalyzerHints.Shape)
	return shape == types.ShapeValue &&
		rm.Intent == types.IntentReturnValue &&
		rm.AnswerSubject.Kind == types.SubjectNumeric
}

// isHistoryLookupRequest reports whether the request is a citation-free
// repository-history/authorship lookup whose literal comes from VCS
// metadata rather than from a repo file:line.
//
// Primary signal: analyzer declared question_kind=history.
//
// Secondary signal: analyzer emitted predicates.is_history_lookup=true.
// The system then validates that the rest of the classification is
// structurally coherent for a scalar history lookup before stripping
// citations. This keeps the semantic judgment in the LLM lane while
// preserving a deterministic fail-closed validator.
func isHistoryLookupRequest(rm types.RequestModel) bool {
	if types.NormalizeRequirementKind(rm.AnalyzerHints.Kind) == types.ReqHistory {
		return true
	}
	if !rm.Predicates.IsHistoryLookup {
		return false
	}
	shape := mapLegacyAnswerShape(rm.AnalyzerHints.Shape)
	if shape != types.ShapeValue && rm.Intent != types.IntentReturnValue {
		return false
	}
	if !rm.Predicates.IsScalarAnswer && shape != types.ShapeValue {
		return false
	}
	return true
}

func isCitationFreeToolValueRequest(rm types.RequestModel) bool {
	return isMeasurementScalarRequest(rm) || isHistoryLookupRequest(rm)
}

func isScalarSourceLiteralLookup(rm types.RequestModel) bool {
	if len(rm.SubTopics) > 1 {
		return false
	}
	if !rm.Predicates.IsScalarAnswer ||
		rm.Predicates.IsCountQuestion ||
		rm.Predicates.IsCategoryEnumeration ||
		rm.Predicates.IsCrossComponent ||
		rm.Predicates.IsRelationalLookup {
		return false
	}
	switch rm.AnswerSubject.Kind {
	case types.SubjectFunctionName,
		types.SubjectTypeName,
		types.SubjectInterface,
		types.SubjectHandlerRoute,
		types.SubjectFilePath,
		types.SubjectStringLiteral,
		types.SubjectEnumValue,
		types.SubjectStructField:
		return true
	}
	return false
}

func reconcileScenario(rm types.RequestModel) (types.Scenario, string) {
	if isScalarSourceLiteralLookup(rm) {
		return types.ScenarioGeneric,
			"scalar source-literal lookup uses generic scenario (avoid architecture-only diagram/reconcile overhead)"
	}
	return rm.Scenario, ""
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
// Log-triage override: when the log_triage pre-stage emitted a
// bundle whose IntentHint is IntentRootCause (i.e. the log carried
// at least one real stack frame with file+line, or a panic/crash/oom
// signal), force IntentRootCause regardless of the LLM's guess. The
// LLM sees the raw log in its prompt via formatAttachedLog and could
// classify correctly on its own; this override is defence-in-depth
// for the case where it misses. Ordered AFTER the count-question
// rule so a hypothetical count-about-a-log question still downgrades
// to return_value; log + count is exotic enough that we do not need
// to optimise it.
//
// Returns the resolved intent + a short reason string. When resolved
// == declared, the rule did not fire and reason is empty.
// bundle MAY be nil — the typical case for questions without an
// attached log; the function nil-checks and skips the override.
func reconcileIntent(declared types.Intent, preds types.SemanticPredicates, bundle *types.LogBundle) (types.Intent, string) {
	if declared == types.IntentEnumerate && preds.IsCountQuestion {
		return types.IntentReturnValue,
			"predicates.is_count_question=true overrides intent=enumerate (defense-in-depth; should be caught by self-consistency)"
	}
	if bundle != nil && bundle.IntentHint == types.IntentRootCause && declared != types.IntentRootCause {
		return types.IntentRootCause,
			"log_triage bundle.intent_hint=root_cause overrides LLM intent (log-triage override)"
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
func reconcileShape(rm types.RequestModel, declared types.AnswerShape, subject types.AnswerSubject, preds types.SemanticPredicates) (types.AnswerShape, string) {
	// Rule 0a: scalar source-literal lookup.
	//
	// Questions whose answer is "one named source-code literal" (entry
	// function, defining type, route path, file path, etc.) should not
	// drift into explanation/step/list shapes even when the analyzer's
	// question_kind says mechanism. The answer is still a single token.
	if isScalarSourceLiteralLookup(rm) && declared != types.ShapeValue {
		return types.ShapeValue,
			"scalar source-literal lookup resolves to one named token, not a prose/list shape"
	}

	// Rule 0b: single exact config-trace question.
	//
	// When the request names one exact config key and asks for lineage /
	// precedence, a symbol set is the wrong carrier. Scalar asks belong
	// to config_value; non-scalar precedence questions belong to
	// explanation so the finalizer can render the lineage/diagram
	// without inventing a symbols[] payload.
	if subject.Kind == types.SubjectConfigKey &&
		(rm.Scenario == types.ScenarioConfigTrace || rm.Intent == types.IntentConfigQuery) &&
		len(types.ExactResolutionTargets(rm)) == 1 {
		want := types.ShapeExplanation
		if preds.IsScalarAnswer {
			want = types.ShapeConfigValue
		}
		if declared != want {
			return want,
				"single exact config-trace question needs narrative/config-value shape, not a symbol set"
		}
	}

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

// reconcileDiagramContract derives the finalizer-facing diagram
// obligation from structural signals. The key design rule is that
// diagram requirement is orthogonal to answer shape: step_list,
// explanation, boolean, value, and config_value may all require a
// grounded diagram when the user is really asking about flow, call
// relationships, timing, or architecture.
func reconcileDiagramContract(rm types.RequestModel, shape types.AnswerShape, bundle *types.LogBundle) *types.DiagramContract {
	var preferred []types.DiagramKind
	var reasons []string
	required := false

	addKind := func(kind types.DiagramKind) {
		if kind == types.DiagramNone || !kind.IsValid() {
			return
		}
		for _, existing := range preferred {
			if existing == kind {
				return
			}
		}
		preferred = append(preferred, kind)
	}
	addReason := func(reason string) {
		if reason == "" {
			return
		}
		for _, existing := range reasons {
			if existing == reason {
				return
			}
		}
		reasons = append(reasons, reason)
	}
	require := func(reason string, kinds ...types.DiagramKind) {
		required = true
		addReason(reason)
		for _, kind := range kinds {
			addKind(kind)
		}
	}

	if rm.DiagramHint != nil && rm.DiagramHint.Kind != types.DiagramNone {
		require("llm_hint", rm.DiagramHint.Kind)
	}
	if rm.Intent == types.IntentTrace {
		require("trace_intent", types.DiagramCallDAG, types.DiagramSequence)
	}
	if rm.Predicates.IsCrossComponent {
		require("cross_component", types.DiagramArchitecture)
	}
	if resolvedLogFrameCount(bundle) >= 2 {
		require("log_call_chain", types.DiagramCallDAG, types.DiagramSequence)
	}

	switch rm.PredicateAxis {
	case types.AxisCall:
		require("axis_call", types.DiagramCallDAG, types.DiagramSequence)
	case types.AxisCondition:
		require("axis_condition", types.DiagramFlow)
	case types.AxisRegister:
		require("axis_register", types.DiagramArchitecture, types.DiagramCallDAG)
	case types.AxisConfigure:
		if rm.Scenario == types.ScenarioConfigTrace {
			require("axis_configure", types.DiagramArchitecture, types.DiagramFlow)
		}
	case types.AxisImplement:
		require("axis_implement", types.DiagramArchitecture)
	}

	switch rm.AnalyzerHints.Kind {
	case "call_chain":
		require("question_kind_call_chain", types.DiagramCallDAG, types.DiagramSequence)
	case "conditional":
		require("question_kind_conditional", types.DiagramFlow)
	case "registration":
		require("question_kind_registration", types.DiagramArchitecture, types.DiagramCallDAG)
	}
	if rm.Scenario == types.ScenarioArchitectureExplain && !isScalarSourceLiteralLookup(rm) {
		require("architecture_scenario", types.DiagramArchitecture)
	}
	if !required {
		return nil
	}
	if len(preferred) == 0 {
		addKind(types.DiagramFlow)
	}
	scope := types.DiagramScopeOverall
	if len(rm.SubTopics) > 1 {
		scope = types.DiagramScopePerSubTopic
		addReason("multi_topic_scope")
	}
	return &types.DiagramContract{
		Required:       true,
		Minimum:        1,
		PreferredKinds: preferred,
		ScopeHint:      scope,
		Reasons:        reasons,
	}
}

func resolvedLogFrameCount(bundle *types.LogBundle) int {
	if bundle == nil {
		return 0
	}
	count := 0
	var walk func(err *types.LogError)
	walk = func(err *types.LogError) {
		if err == nil {
			return
		}
		for _, frame := range err.Frames {
			if frame.File != "" && frame.Line > 0 {
				count++
			}
		}
		walk(err.Cause)
	}
	for i := range bundle.Errors {
		walk(&bundle.Errors[i])
	}
	return count
}

func logSubjectInferred(subject types.AnswerSubject, reason string) {
	if subject.Kind == types.SubjectUnknown || reason == "" {
		return
	}
	logging.Warning("[CGEC] E1 subject_inferred: kind=%s axes=%v conf=%.2f (%s)",
		subject.Kind, subject.EntityAxes, subject.Confidence, reason)
}

func logScenarioReconcile(before, after types.Scenario, reason string) {
	if reason == "" {
		return
	}
	logging.Warning("[analyzer] scenario reconciled: %s → %s (%s)", before, after, reason)
}

func logShapeReconciled(before, after types.AnswerShape, reason string) {
	if before == after || reason == "" {
		return
	}
	logging.Warning("[analyzer] shape reconciled: %s → %s (%s)", before, after, reason)
}
