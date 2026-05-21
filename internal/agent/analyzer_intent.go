package agent

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// analyzer_intent.go holds the schema-v4 reconcile rules for Intent /
// AnswerSubject / Predicates. After the v4 rewrite, every prose-cue
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
// is_count_question=false but two independent structural signals
// coincide — intent=return_value, and answer_subject.kind=numeric —
// the pair together describes a single numeric answer to a
// return-value question, which is the same population the primary
// signal targets. The fallback catches the case where the LLM was
// internally inconsistent (emitted both structural signals for a
// measurement-scalar question but still picked is_count_question=
// false). Adds is_scalar_answer=true as a sanity gate so a
// non-scalar return value can't trip the carve-out.
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
// intent=enumerate); this function runs on an LLM output that has
// already cleared that check.
func isMeasurementScalarRequest(rm types.RequestModel) bool {
	if rm.Predicates.IsCountQuestion {
		return true
	}
	return rm.Predicates.IsScalarAnswer &&
		rm.Intent == types.IntentReturnValue &&
		rm.AnswerSubject.Kind == types.SubjectNumeric
}

// isHistoryLookupRequest reports whether the request has repository-history /
// authorship metadata as an answer-grade evidence source. The flag is
// orthogonal to answer shape: a history question may be a scalar commit lookup,
// a feature-summary explanation, a recent-merge list, or a comparison. All of
// those need VCS metadata to flow without fake file:line citation floors.
//
// Primary signal: analyzer declared question_kind=history.
//
// Secondary signal: analyzer emitted predicates.is_history_lookup=true. This
// keeps the semantic judgment in the LLM lane while preserving a deterministic
// fail-closed validator keyed on typed fields rather than request keywords.
func isHistoryLookupRequest(rm types.RequestModel) bool {
	if types.NormalizeRequirementKind(rm.AnalyzerHints.Kind) == types.ReqHistory {
		return true
	}
	return rm.Predicates.IsHistoryLookup
}

func isCitationFreeToolValueRequest(rm types.RequestModel) bool {
	return isMeasurementScalarRequest(rm) || isHistoryLookupRequest(rm)
}

func isScalarSourceLiteralLookup(rm types.RequestModel) bool {
	return types.IsScalarSourceLiteralLookup(rm)
}

func reconcileNonScalarExplanationSubject(rm types.RequestModel) (types.RequestModel, string) {
	if !scalarAnswerSubjectKind(rm.AnswerSubject.Kind) {
		return rm, ""
	}
	if rm.Predicates.IsScalarAnswer ||
		rm.Predicates.IsCountQuestion ||
		rm.Predicates.IsRoleLocateLookup ||
		rm.Intent == types.IntentReturnValue {
		return rm, ""
	}
	if !nonScalarExplanationRoute(rm) {
		return rm, ""
	}
	before := rm.AnswerSubject.Kind
	rm.AnswerSubject = types.AnswerSubject{}
	return rm, "cleared scalar answer_subject.kind=" + string(before) + " because the typed route is an explanation-shaped answer, not a scalar literal lookup"
}

func reconcileHistoryMultiTargetScalar(rm types.RequestModel) (types.RequestModel, string) {
	if !rm.Predicates.IsHistoryLookup ||
		!rm.Predicates.IsScalarAnswer ||
		rm.Predicates.IsCountQuestion ||
		rm.Predicates.IsRoleLocateLookup {
		return rm, ""
	}
	targetCount := types.HistoryLookupScalarTargetCount(rm)
	if targetCount <= 1 {
		return rm, ""
	}
	rm.Predicates.IsScalarAnswer = false
	if rm.Intent == types.IntentReturnValue {
		rm.Intent = types.IntentEnumerate
	}
	if scalarAnswerSubjectKind(rm.AnswerSubject.Kind) {
		rm.AnswerSubject = types.AnswerSubject{}
	}
	return rm, "history lookup has multiple typed targets; answer is a per-target result set, not one scalar literal"
}

func scalarAnswerSubjectKind(kind types.AnswerSubjectKind) bool {
	switch kind {
	case types.SubjectStringLiteral, types.SubjectNumeric, types.SubjectReturnValue:
		return true
	default:
		return false
	}
}

func nonScalarExplanationRoute(rm types.RequestModel) bool {
	switch rm.Intent {
	case types.IntentRootCause, types.IntentExplain:
		return true
	}
	switch rm.Scenario {
	case types.ScenarioRootCause,
		types.ScenarioArchitectureExplain,
		types.ScenarioPerformanceBottleneck:
		return true
	}
	switch strings.TrimSpace(rm.AnalyzerHints.Kind) {
	case "mechanism", "call_chain", "conditional":
		return true
	default:
		return false
	}
}

func reconcileScenario(rm types.RequestModel) (types.Scenario, string) {
	if isScalarSourceLiteralLookup(rm) {
		return types.ScenarioGeneric,
			"scalar source-literal lookup uses generic scenario (avoid architecture-only diagram/reconcile overhead)"
	}
	if types.IsSingleTopicStructuralTrace(rm) && rm.Scenario != types.ScenarioGeneric {
		return types.ScenarioGeneric,
			"single-topic structural trace uses generic scenario (avoid architecture reconcile window for linear call/flow walkthroughs)"
	}
	if types.IsFailureScopeDecisionAnswer(rm) && rm.Scenario != types.ScenarioGeneric {
		return types.ScenarioGeneric,
			"failure-scope verdict question uses generic scenario (avoid architecture/enumeration scaffold for contextual counts)"
	}
	return rm.Scenario, ""
}

// reconcileDiagnosticQuestionProfile consumes the analyzer LLM's
// language-neutral diagnostic predicate. It deliberately does not read
// raw request text: the semantic judgment lives in emit_analysis, and
// emit_analysis has already normalized the diagnostic profile mirror to
// this predicate unless an independent strong diagnostic signal was
// present. This helper accepts the predicate or those strong signals
// (current risk / historical regression) for direct RequestModel callers,
// but deliberately ignores profile-only IsDiagnostic drift; profile OR
// semantics would let a noisy support lane steal ordinary architecture
// questions into the root-cause family.
func reconcileDiagnosticQuestionProfile(rm types.RequestModel) (types.RequestModel, string) {
	strongProfileSignal := rm.DiagnosticProfile.CurrentRisk || rm.DiagnosticProfile.HistoricalRegression
	if !rm.Predicates.IsDiagnosticQuestion && !strongProfileSignal {
		return rm, ""
	}
	var changes []string
	if !rm.Predicates.IsDiagnosticQuestion {
		changes = append(changes, "predicate.is_diagnostic_question→true")
		rm.Predicates.IsDiagnosticQuestion = true
	}
	if !rm.DiagnosticProfile.IsDiagnostic {
		changes = append(changes, "diagnostic_profile.is_diagnostic→true")
		rm.DiagnosticProfile.IsDiagnostic = true
	}
	if rm.Intent != types.IntentRootCause {
		changes = append(changes, "intent→root_cause")
		rm.Intent = types.IntentRootCause
	}
	targetScenario := types.ScenarioRootCause
	if rm.PerfTrace != nil {
		targetScenario = types.ScenarioPerformanceBottleneck
	}
	switch rm.Scenario {
	case types.ScenarioRootCause, types.ScenarioPerformanceBottleneck:
		// Keep an already-diagnostic scenario. PerfTrace prefers the
		// performance template only when the LLM left the scenario in a
		// non-diagnostic lane; do not override a deliberate root_cause
		// classification.
	default:
		changes = append(changes, "scenario→"+string(targetScenario))
		rm.Scenario = targetScenario
	}
	if rm.Predicates.IsScalarAnswer || rm.Predicates.IsRoleLocateLookup {
		// A diagnostic can still ask for the single failing function or
		// line. The important part is that role/scalar predicates do not
		// steal the family after root_cause intent is set, so leave the
		// predicates intact and let the answer surface choose the
		// diagnostic scaffold.
	}
	if len(changes) == 0 {
		return rm, ""
	}
	return rm, "diagnostic semantic predicate aligned " + strings.Join(changes, ", ")
}

func reconcileExternalOnlyRuntimeDiagnosticProfile(rm types.RequestModel) (types.RequestModel, string) {
	if !rm.HasExternalOnlyRuntimeArtifact() {
		return rm, ""
	}
	if !rm.DiagnosticProfile.RequiresCurrentStatusDiagnostic() {
		return rm, ""
	}
	if rm.HasRuntimeArtifactCurrentVerificationAnchor() {
		return rm, ""
	}
	rm.DiagnosticProfile.CurrentRisk = false
	rm.DiagnosticProfile.CurrentVersionCheck = false
	rm.DiagnosticProfile.HistoricalRegression = false
	return rm, "external-only runtime artifact has no resolved frame, exact target, or required file for current-checkout verification; keep the request observation-only instead of opening current-repo verification"
}

// reconcileIntent is preserved as a thin sanity check. Count predicates used to
// downgrade intent=enumerate to return_value, but that overrode list-with-counts
// questions where the user wants members plus per-category totals. The normal
// emit_analysis path now clears is_count_question for enumerate-shaped answers
// before the RequestModel is stored, so this layer must not steal user intent.
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
		return declared,
			"predicates.is_count_question=true left advisory for intent=enumerate; per-list counts do not downgrade enumeration intent"
	}
	// Commit 61 Batch F.3 (audit MEDIUM #3, red line "no system
	// hard-cap"): pre-fix this branch forced IntentRootCause when
	// log_triage's IntentHint was RootCause. That overrode the user's
	// declared intent based on a derived signal (panic in log) — but
	// "user attached a panic log AND asked 'explain how X works'"
	// has user-intent=explain, not root_cause. The system was
	// substituting its judgment for the user's. Removed.
	//
	// The LLM's emit_analysis already reads the raw log via the
	// formatAttachedLog prompt section, so it can independently
	// classify root_cause when the user genuinely asked for one.
	// Trust the LLM's intent decision.
	_ = bundle
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

// reconcileStrictMode is a legacy compatibility switch for reconcile
// rules that still have an advisory/strict split. The retired
// reconcileShape path no longer exists; new reconcile logic should use
// typed consistency repair and preserve the LLM's emit_analysis
// judgment instead of adding new hard overrides. Default false keeps
// legacy advisory-only behaviour.
var reconcileStrictMode bool

// SetReconcileStrictMode flips the global mode. Called from
// cmd/root.go at startup. Mutex is unnecessary because production
// only writes once at startup and tests use t.Cleanup; the var is
// a single bool.
func SetReconcileStrictMode(on bool) { reconcileStrictMode = on }

// reconcileStrictModeEnabled is the read-side predicate.
func reconcileStrictModeEnabled() bool { return reconcileStrictMode }

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

// ── AnswerSubject inference ───────────────────────────────────────────
//
// inferAnswerSubject is the remaining CGEC analyzer post-processor
// for subject kind. It classifies WHAT KIND of code literal the answer
// is supposed to be (skill_name, agent_name, config_key, ...), while
// leaving the LLM's supplied AnswerSubject untouched when present. The
// retired shape reconciler is intentionally not part of this path:
// answer presentation now flows through AnswerDocumentV2 +
// AnswerPresentationContract rather than legacy shape overrides.

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

func multiAxisStructuralSubject(subject types.AnswerSubject) bool {
	if len(subject.EntityAxes) < 2 {
		return false
	}
	switch subject.Kind {
	case types.SubjectStructField,
		types.SubjectTypeName,
		types.SubjectInterface,
		types.SubjectGeneric:
		return true
	}
	return false
}

// reconcileDiagramContract derives the finalizer-facing diagram
// obligation from structural signals. Diagram requirement is
// orthogonal to the principal-payload kind: scalar / list / explanation
// answers may all require a grounded diagram when the user is
// asking about flow, call relationships, timing, or architecture.
func reconcileDiagramContract(rm types.RequestModel, bundle *types.LogBundle) *types.DiagramContract {
	var preferred []types.DiagramKind
	var reasons []string
	required := false
	requiredKind := types.DiagramNone

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
			if requiredKind == types.DiagramNone && kind != types.DiagramNone && kind.IsValid() {
				requiredKind = kind
			}
			addKind(kind)
		}
	}
	prefer := func(reason string, kinds ...types.DiagramKind) {
		addReason(reason)
		for _, kind := range kinds {
			addKind(kind)
		}
	}

	if rm.DiagramHint != nil && rm.DiagramHint.Kind != types.DiagramNone {
		require("explicit_diagram_request", rm.DiagramHint.Kind)
	}
	if rm.Intent == types.IntentTrace {
		prefer("trace_intent", types.DiagramCallDAG, types.DiagramSequence)
	}
	if rm.Predicates.IsCrossComponent {
		prefer("cross_component", types.DiagramArchitecture)
	}
	if resolvedLogFrameCount(bundle) >= 2 {
		prefer("log_call_chain", types.DiagramCallDAG, types.DiagramSequence)
	}

	switch rm.PredicateAxis {
	case types.AxisCall:
		prefer("axis_call", types.DiagramCallDAG, types.DiagramSequence)
	case types.AxisCondition:
		prefer("axis_condition", types.DiagramFlow)
	case types.AxisRegister:
		prefer("axis_register", types.DiagramArchitecture, types.DiagramCallDAG)
	case types.AxisConfigure:
		if rm.Scenario == types.ScenarioConfigTrace {
			prefer("axis_configure", types.DiagramArchitecture, types.DiagramFlow)
		}
	case types.AxisImplement:
		prefer("axis_implement", types.DiagramArchitecture)
	}

	switch rm.AnalyzerHints.Kind {
	case "call_chain":
		prefer("question_kind_call_chain", types.DiagramCallDAG, types.DiagramSequence)
	case "conditional":
		prefer("question_kind_conditional", types.DiagramFlow)
	case "registration":
		prefer("question_kind_registration", types.DiagramArchitecture, types.DiagramCallDAG)
	}
	if rm.Scenario == types.ScenarioArchitectureExplain && !isScalarSourceLiteralLookup(rm) {
		prefer("architecture_scenario", types.DiagramArchitecture)
	}
	if len(preferred) == 0 {
		return nil
	}
	scope := types.DiagramScopeOverall
	if len(rm.SubTopics) > 1 {
		scope = types.DiagramScopePerSubTopic
		addReason("multi_topic_scope")
	}
	return &types.DiagramContract{
		Required:       required,
		Minimum:        mapDiagramMinimum(required),
		RequiredKind:   requiredKind,
		PreferredKinds: preferred,
		ScopeHint:      scope,
		Reasons:        reasons,
	}
}

func mapDiagramMinimum(required bool) int {
	if required {
		return 1
	}
	return 0
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
