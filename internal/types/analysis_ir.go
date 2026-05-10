package types

import "fmt"

// AnalysisIR is the sole structured output of the analyze stage under the
// Analyzer v3 design. It is a single source of truth for everything the
// downstream pipeline needs to know about the user's request: intent and
// scenario classification, a canonical term graph, a task DAG, a falsifiable
// hypothesis set, a multi-dimensional risk matrix, an evidence plan, and a
// machine-checkable answer contract.
//
// Design invariants:
//
//  1. Analyzer is the *only* writer. Other stages may mutate
//     HypothesisSet[*].Status and TaskGraph.Nodes[*] execution state via
//     dedicated APIs but must not rewrite the structural fields.
//  2. Every non-probe TaskNode must bind at least one Hypothesis.
//  3. TaskGraph must be a well-formed DAG whose hard dependencies are
//     satisfiable — enforced by the analyzer quality gate, not by consumers.
//
// The deterministic pipeline that fills every field on this struct
// — normalizer, scenario compiler, hypothesis planner, risk
// evaluator, counterfactual expander, quality gate — lives under
// internal/analysis/ and is orchestrated from internal/agent/
// analyzer.go:buildAnalysisIR.
type AnalysisIR struct {
	Version        string         `json:"version"`
	TraceID        string         `json:"trace_id,omitempty"`
	RequestModel   RequestModel   `json:"request_model"`
	TaskGraph      TaskGraph      `json:"task_graph"`
	EvidencePlan   EvidencePlan   `json:"evidence_plan"`
	AnswerContract AnswerContract `json:"answer_contract"`
	HypothesisSet  []Hypothesis   `json:"hypothesis_set,omitempty"`
	QualityGate    GateReport     `json:"quality_gate"`
}

// AnalysisIRVersion is the current schema version string. Bump on any
// breaking change to the wire format so downstream consumers can refuse
// to parse IRs they do not understand.
const AnalysisIRVersion = "v9"

// ── RequestModel ────────────────────────────────────────────────────────

type RequestModel struct {
	RawRequest  string      `json:"raw_request"`
	Language    string      `json:"language"`
	Intent      Intent      `json:"intent"`
	Scenario    Scenario    `json:"scenario"`
	Complexity  Complexity  `json:"complexity"`
	TermGraph   TermGraph   `json:"term_graph"`
	Ambiguities []Ambiguity `json:"ambiguities,omitempty"`
	RiskMatrix  RiskMatrix  `json:"risk_matrix"`

	// IntentConfidence / ComplexityConfidence / KindConfidence are
	// LLM-emitted certainty scores in [0.0, 1.0] for each
	// classification dimension. Zero = "no opinion" (unusual — the
	// schema asks the LLM to rate explicitly). Downstream consumers
	// gate aggressive narrowing on confidence (e.g. the explorer's
	// tightenDeclarativeFrontier only fires when KindConfidence ≥
	// 0.7) so a hesitant classification cannot over-narrow the read
	// set. (Pre-PR5 a fourth ShapeConfidence rode alongside; it was
	// retired with AnswerShape.)
	IntentConfidence     float64 `json:"intent_confidence,omitempty"`
	ComplexityConfidence float64 `json:"complexity_confidence,omitempty"`
	KindConfidence       float64 `json:"kind_confidence,omitempty"`

	// Predicates carries the LLM's semantic self-assessment of the
	// question along axes that the prose-cue tables used to detect
	// (count question, scalar answer, cross-component, …). Replaces
	// language-specific keyword tables with cross-language LLM judgement.
	// Schema requires every field present (true or false) so a missing
	// field is a fail-loud signal, not a silent false.
	Predicates SemanticPredicates `json:"predicates"`

	// DiagnosticProfile is the analyzer LLM's second typed
	// diagnostic-intent lane. Predicates.IsDiagnosticQuestion remains
	// the broad "is this a diagnosis?" bit; this profile splits the
	// cases that previously collapsed when the LLM picked the wrong
	// broad predicate: current-risk checks, historical regression
	// checks, and current-version verification. Downstream routing
	// consumes this profile as a typed safety net and never scans raw
	// request text for diagnostic keywords.
	DiagnosticProfile DiagnosticIntentProfile `json:"diagnostic_profile,omitempty"`

	// ArtifactObservationProfile is system-derived from log / trace
	// triage bundles after emit_analysis. It preserves non-exception
	// observations (retry loops, line-mapping drift, topic mismatch,
	// performance symptoms, etc.) as typed facts that TaskGraph and
	// finalizer prompts can consume without treating unknown_chunks or
	// raw trace prose as authority. Empty when no structured artifact
	// observation exists.
	ArtifactObservationProfile *ArtifactObservationProfile `json:"artifact_observation_profile,omitempty"`

	// SubTopics lists independently-answerable sub-topics detected by
	// the analyzer. When non-empty, the compiler generates one evidence
	// DAG node per sub-topic. Empty for single-topic questions.
	SubTopics []SubTopic `json:"sub_topics,omitempty"`

	// AnalyzerHints captures the raw LLM-extracted hints from the
	// analyze stage's structured output. Intent/Scenario/Complexity
	// above are typed, mapped, and sometimes lossy projections of the
	// same LLM output; AnalyzerHints preserves the verbatim strings so
	// explorer/finalizer can read them without going back to the legacy
	// TaskItem fields. This field is the downstream read surface — not
	// a "sidecar" — because v3 Intent has fewer distinct values than
	// the LLM's question_kind enum (e.g. both "registration" and
	// "enumeration" collapse to IntentEnumerate), so the mapped form
	// is not a lossless substitute for the raw LLM string.
	AnalyzerHints AnalyzerHints `json:"analyzer_hints"`

	// AnswerSubject classifies what kind of source-code literal the
	// answer should be (skill name, agent name, config key, return
	// value, ...). Used by the chain ranker to prefer chains whose
	// terminal token matches the expected subject kind, by the
	// shape reconciler to swap config_value→value when the subject
	// is a Go-literal rather than a YAML key, and by the retry-hint
	// renderer's RebindSubject directive. Either filled by the LLM
	// via emit_analysis.answer_subject or derived deterministically
	// by analyzer_intent.go::inferAnswerSubject when the LLM left
	// the field zero. Zero-value (Kind=SubjectUnknown) is safe and
	// degrades gracefully to the pre-CGEC behaviour.
	AnswerSubject AnswerSubject `json:"answer_subject"`

	// PredicateAxis captures the action verb of the user's question
	// ("how does X CALL Y" → AxisCall; "how is X REGISTERED" →
	// AxisRegister). Orthogonal to AnswerSubject: subject asks
	// "what kind of token is the answer?"; axis asks "what
	// action-direction is the question about?". Used by the
	// evidence ranker to bias items whose AnchorKind matches the
	// axis (AxisCall × AnchorCall → 1.5x; AxisCall × AnchorDefinition
	// → 0.7x). Populated by analyzer_predicate.go::reconcilePredicateAxis
	// from a verb-cue table; zero-value (AxisUnknown) disables the
	// axis boost entirely and preserves historical ranking for
	// questions without a clear verb cue.
	PredicateAxis PredicateAxis `json:"predicate_axis,omitempty"`

	// DiagramHint is the analyzer LLM's optional suggestion for the
	// kind of structural diagram that would best communicate the
	// answer. The deterministic analyzer compiler reconciles this hint
	// with the resolved shape, predicate axis, intent, cross-component
	// signals, and log-triage data to derive AnswerContract.Diagram.
	DiagramHint *DiagramHint `json:"diagram_hint,omitempty"`

	// EnumerationBoundary is the analyzer LLM's request-grounded hint
	// that the user explicitly asked for a bounded set size ("the 7
	// checks", "the first 3 handlers", "top 5 stages"). The system
	// validates SourceQuote against RawRequest before downstream stages
	// use it, so this remains an LLM recommendation rather than an
	// authority lane.
	EnumerationBoundary *RequestedEnumerationBoundary `json:"enumeration_boundary,omitempty"`

	// CompletenessObligation (Plan E, 2026-05-02) captures the
	// analyzer's finding that the user asked for an EXHAUSTIVE
	// answer. Trigger: a verbatim phrase from the question that
	// signals universal coverage ("all the X" / "every X" / "complete
	// list" / "exhaustive"). When Required=true, downstream gates
	// refuse premature emit_investigation_complete (G1) and reject
	// completeness=lower_bound on the answer slate (G2). Honest
	// fallback: completeness=unknown when the investigation
	// legitimately cannot determine the full set. Distinct from
	// EnumerationBoundary which carries a count; this carries a
	// completeness demand without a count.
	CompletenessObligation *CompletenessObligation `json:"completeness_obligation,omitempty"`

	// Buckets (Plan E, 2026-05-02) lists user-named partitions of
	// the answer. Triggered by questions that explicitly carve the
	// response into labeled groups ("X for A, Y for B" / "分别列出
	// A 和 B 的 ..." / "compare A vs B"). Each bucket's Label is
	// verbatim from RawRequest; downstream rendering reuses the
	// labels as section headers so the user's mental partition
	// survives end-to-end. The bucket-alignment gate (G3) verifies
	// that the rendered answer's per-bucket items are disjoint
	// across siblings.
	//
	// SubTopics is the analyzer's existing "I split this into N
	// independent topics" surface; Buckets is the stronger claim
	// "the user PARTITIONED the answer along these N specific named
	// groups". A multi-topic question can have SubTopics without
	// Buckets (the LLM split topics for investigation parallelism
	// but the user didn't impose a partition). A bucket-partitioned
	// question always has both: each Bucket maps 1:1 to one or more
	// SubTopics by entity overlap.
	Buckets []QuestionBucket `json:"buckets,omitempty"`

	// LogTriage / PerfTrace are NOT emitted by the analyzer LLM. They
	// are bundle pointers that buildAnalysisIR attaches AFTER
	// log/perf-triage validation, exposed here so downstream
	// sub-packages (compiler / hdp / gate / criterion / request_traits)
	// can read multi-frame artifact evidence without re-threading
	// bundle pointers through every signature.
	//
	// The fields are excluded from JSON serialisation (`json:"-"`) to
	// preserve the LLM-emit contract on the wire — these are
	// system-derived attachments, not part of what emit_analysis
	// returned. nil when the user did not attach --log / --htrace /
	// --atrace.
	//
	// Read by: IsScalarSourceLiteralLookup (multi-frame artifact
	// evidence reverses the role-locate scalar short-circuit), and
	// any future cross-package decider that needs to disambiguate
	// "single literal answer" from "multi-step mechanism" using the
	// objective artifact context the user supplied.
	LogTriage *LogBundle  `json:"-"`
	PerfTrace *PerfBundle `json:"-"`
}

// AnswerSubjectKind enumerates the distinct kinds of source-code
// literals an answer can resolve to. The chain ranker, shape
// reconciler, and retry-hint renderer all dispatch on this enum so
// the LLM's "what kind of token am I looking for" intuition is
// represented as a typed value rather than as English prose in
// prompts.
//
// Add new kinds here when a real bug surfaces a missing one — do
// not pre-populate speculative kinds. Each kind that callers can
// rely on has a corresponding judge in
// internal/analysis/subject/taxonomy.go that scores tokens against
// it. SubjectUnknown is the explicit "I don't know" sentinel and
// is the zero value.
type AnswerSubjectKind string

const (
	SubjectUnknown       AnswerSubjectKind = ""
	SubjectFunctionName  AnswerSubjectKind = "function_name"
	SubjectTypeName      AnswerSubjectKind = "type_name"
	SubjectHandlerRoute  AnswerSubjectKind = "handler_route"
	SubjectConfigKey     AnswerSubjectKind = "config_key"
	SubjectReturnValue   AnswerSubjectKind = "return_value"
	SubjectFilePath      AnswerSubjectKind = "file_path"
	SubjectStringLiteral AnswerSubjectKind = "string_literal"
	SubjectNumeric       AnswerSubjectKind = "numeric"
	SubjectEnumValue     AnswerSubjectKind = "enum_value"
	SubjectStructField   AnswerSubjectKind = "struct_field"
	SubjectInterface     AnswerSubjectKind = "interface_name"
	SubjectGeneric       AnswerSubjectKind = "generic"
)

// AllAnswerSubjectKinds returns every declared kind in declaration
// order. The emit_analysis schema enum and the gate's
// criterion_resolvable check both consume this list so adding a new
// constant above is a single source of truth.
func AllAnswerSubjectKinds() []AnswerSubjectKind {
	return []AnswerSubjectKind{
		SubjectUnknown,
		SubjectFunctionName,
		SubjectTypeName,
		SubjectHandlerRoute,
		SubjectConfigKey,
		SubjectReturnValue,
		SubjectFilePath,
		SubjectStringLiteral,
		SubjectNumeric,
		SubjectEnumValue,
		SubjectStructField,
		SubjectInterface,
		SubjectGeneric,
	}
}

// IsValid returns true when k is one of the declared constants.
// SubjectUnknown is treated as valid because the zero value is the
// explicit "no classification yet" state.
func (k AnswerSubjectKind) IsValid() bool {
	for _, declared := range AllAnswerSubjectKinds() {
		if k == declared {
			return true
		}
	}
	return false
}

// AnswerSubject is the classifier the analyzer attaches to each
// RequestModel. EntityAxes captures the relational structure of
// the question (e.g. ["agent → skill"] for "what skill does the
// explorer agent use"); the chain ranker uses both Kind and
// EntityAxes to score candidate chains. Confidence is in [0, 1]
// and reflects how sure the producer (LLM or deterministic
// fallback) is about the classification — low-confidence
// classifications get a softer ranker boost.
type AnswerSubject struct {
	Kind       AnswerSubjectKind `json:"kind"`
	EntityAxes []string          `json:"entity_axes,omitempty"`
	Confidence float64           `json:"confidence,omitempty"`
}

// PredicateAxis enumerates the action-direction axis of the user's
// question. Extracted deterministically from verb cues in the raw
// request ("调用" / "call" / "invoke" → AxisCall; "注册" /
// "register" → AxisRegister; ...). Paired at the evidence ranker
// with each item's AnchorKind via a static PredicateAxis × AnchorKind
// affinity matrix (see internal/analysis/axis). Zero value
// (AxisUnknown) disables the axis-aware boost and preserves
// historical ranking for questions without a verb cue.
//
// Orthogonal to AnswerSubjectKind: subject asks "what kind of
// token is the answer?"; axis asks "what direction is the
// question about?". Both can coexist: a "what does X.Y() return?"
// question is AnswerSubject=SubjectReturnValue + PredicateAxis=AxisReturn.
//
// Add new axes only when a failing run surfaces a missing one —
// do not pre-populate speculative kinds. Each new constant must be
// paired with at least one AnchorKind affinity entry in
// internal/analysis/axis/matrix.go or the axis is a no-op.
type PredicateAxis string

const (
	AxisUnknown   PredicateAxis = ""
	AxisCall      PredicateAxis = "call"
	AxisRegister  PredicateAxis = "register"
	AxisDefine    PredicateAxis = "define"
	AxisReturn    PredicateAxis = "return"
	AxisConfigure PredicateAxis = "configure"
	AxisCondition PredicateAxis = "condition"
	AxisImplement PredicateAxis = "implement"
)

var allPredicateAxes = []PredicateAxis{
	AxisCall, AxisRegister, AxisDefine, AxisReturn,
	AxisConfigure, AxisCondition, AxisImplement,
}

// AllPredicateAxes returns every declared axis in declaration
// order, excluding the zero-value AxisUnknown. Used by tests and
// the axis package's matrix completeness check.
func AllPredicateAxes() []PredicateAxis {
	out := make([]PredicateAxis, len(allPredicateAxes))
	copy(out, allPredicateAxes)
	return out
}

// IsValid reports whether a is one of the declared axes including
// AxisUnknown (which is the explicit "no axis extracted" sentinel
// and is valid).
func (a PredicateAxis) IsValid() bool {
	if a == AxisUnknown {
		return true
	}
	for _, declared := range allPredicateAxes {
		if a == declared {
			return true
		}
	}
	return false
}

// SemanticPredicates carries the LLM's semantic self-assessment of the
// user's question along orthogonal axes that are language-neutral. It
// replaces the historical prose-cue tables (countVerbPrefixes,
// crossComponentCues, enumerationCuePrefixes, relationalVerbCues,
// categoryEnumerationCues) which only covered ZH+EN and required
// per-language curation. The LLM, by contrast, can read any language
// the user writes in.
//
// All fields are required in the emit_analysis schema (LLM must
// explicitly emit true OR false) — a missing field rejects the call
// and triggers analyzer retry. This is the fail-loud contract: we
// never silently default a predicate to false (which could mask a
// classification miss) — the LLM must affirm one way or the other.
//
// Each predicate is consumed by exactly one downstream dispatch site;
// add a new predicate only when an existing prose-cue table needs to
// be deleted and an LLM-judgement is the replacement signal.
type SemanticPredicates struct {
	// IsScalarAnswer: the answer is a single scalar value (a number, a
	// literal, a path), not a set or sequence. Drives the
	// measurement-scalar carve-out that strips the citation gate.
	IsScalarAnswer bool `json:"is_scalar_answer"`

	// IsRoleLocateLookup: the request names a clue / output / context
	// entity, but the answer is a DIFFERENT single literal that plays a
	// role relative to that clue — e.g. the entry function that
	// produces a type, the file that defines a route, or the config key
	// that controls a behaviour. This lets the analyzer keep the answer
	// in the scalar-literal lane even when the request text itself names
	// a neighboring entity, avoiding architecture/mechanism drift.
	IsRoleLocateLookup bool `json:"is_role_locate_lookup"`

	// IsCountQuestion: the answer is a single number that must be
	// computed by aggregating values across multiple source units —
	// counting items, summing lines of code, totalling bytes across a
	// directory tree, or any similar aggregation where no single
	// source-code literal holds the answer. Implies IsScalarAnswer=true.
	// False when the answer is a number that already exists as a single
	// literal (a const declaration, a default value, an enum ordinal):
	// that case is IsScalarAnswer=true without IsCountQuestion. Drives
	// intent downgrade enumerate→return_value and the measurement-scalar
	// carve-out (which strips the citation gate because an aggregated
	// number has no file:line to cite).
	IsCountQuestion bool `json:"is_count_question"`

	// IsCrossComponent: the user is comparing or relating two distinct
	// subsystems / components / types. Drives complexity upgrade to
	// Complex regardless of declared complexity.
	IsCrossComponent bool `json:"is_cross_component"`

	// IsRelationalLookup: the user is filtering set X by a relationship
	// to Y ("functions that return Z", "agents that use skill Y").
	// Combined with IsCategoryEnumeration drives the conditional-
	// enumeration shape rule.
	IsRelationalLookup bool `json:"is_relational_lookup"`

	// IsCategoryEnumeration: the user is asking "what kinds / types /
	// categories of X exist". Drives shape selection toward
	// list_of_symbols even when declared shape was value/config_value.
	IsCategoryEnumeration bool `json:"is_category_enumeration"`

	// IsHistoryLookup: the answer is a repository-history / authorship
	// lookup whose literal should come from VCS metadata (git log /
	// blame / commit history), not from a repo file:line. Replaces the
	// old VCS keyword tables with an explicit analyzer judgment that the
	// system can then validate against the rest of the classification.
	IsHistoryLookup bool `json:"is_history_lookup"`

	// IsDiagnosticQuestion: the user is asking to diagnose a failure,
	// regression, runtime symptom, or observed bad behaviour and wants
	// a cause / still-present-risk answer rather than an architecture
	// mechanism tour or scalar role lookup. This is a language-neutral
	// LLM judgment, not a keyword table. It applies with or without an
	// attached runtime artifact; log / trace bundles are evidence when
	// present, not the trigger source for this predicate.
	IsDiagnosticQuestion bool `json:"is_diagnostic_question"`
}

// DiagnosticIntentProfile splits diagnostic intent into the cases that
// need different downstream treatment. It is emitted by the analyzer LLM
// as a required structured object and then reconciled with
// SemanticPredicates. The booleans are deliberately simple typed flags:
// hard routing may read them directly without similarity scores or raw
// text scans.
type DiagnosticIntentProfile struct {
	// IsDiagnostic is the profile-level mirror of
	// SemanticPredicates.IsDiagnosticQuestion. Either lane being true
	// is enough to route into diagnostic root-cause handling.
	IsDiagnostic bool `json:"is_diagnostic"`

	// CurrentRisk is true when the user asks whether a known or
	// observed issue can still happen in the current checkout.
	CurrentRisk bool `json:"current_risk"`

	// HistoricalRegression is true when the user asks about an issue
	// observed in a previous run/log/trace and whether the current
	// code still has that regression class.
	HistoricalRegression bool `json:"historical_regression"`

	// CurrentVersionCheck is true when the answer must explicitly
	// verify current code rather than only explain the historical
	// artifact or prior conversation.
	CurrentVersionCheck bool `json:"current_version_check"`

	// Confidence is the analyzer LLM's confidence in this profile in
	// [0,1]. Zero means unset/unknown and does not block the typed
	// booleans.
	Confidence float64 `json:"confidence,omitempty"`
}

// RequiresDiagnosticRootCause reports whether the typed profile says
// downstream should use the diagnostic/root-cause lane. Artifact facts
// alone do not trigger this; the current request must carry one of the
// diagnostic intent flags.
func (p DiagnosticIntentProfile) RequiresDiagnosticRootCause() bool {
	return p.IsDiagnostic || p.CurrentRisk || p.HistoricalRegression || p.CurrentVersionCheck
}

// RequiresCurrentStatusDiagnostic reports whether the answer needs the
// fixed two-lane current-status scaffold: historical observation plus
// current-code verification plus a bounded verdict.
func (p DiagnosticIntentProfile) RequiresCurrentStatusDiagnostic() bool {
	return p.CurrentRisk || p.HistoricalRegression || p.CurrentVersionCheck
}

// AnalyzerHints is the raw LLM-extracted analyzer output, mirrored onto
// the IR so downstream consumers have a single canonical read path.
type AnalyzerHints struct {
	Keywords []string `json:"keywords,omitempty"`
	Entities []string `json:"entities,omitempty"`
	// PrimaryEntities snapshots the analyzer LLM's top-level Entities
	// list before deterministic augmentation (log/perf entities) and
	// before sub-topic entities are unioned into Entities for breadth
	// consumers. This preserves the analyzer-authored shortlist even when
	// downstream stages widen Entities for search breadth.
	PrimaryEntities []string `json:"primary_entities,omitempty"`
	// PrimaryScopes is the typed sub-repo-name lane parallel to
	// PrimaryEntities. When the analyzer LLM emits an entity whose
	// surface form matches an active sub-repo's RootRel verbatim
	// (NormalizeCodeKey-equivalent), the post-emit projection at
	// internal/agent/scope_projection.go::projectPrimaryScopes COPIES
	// that entity here while keeping it in PrimaryEntities. Empty in
	// single-repo posture (mg.IsSingle()); empty in multi-repo when
	// no emitted entity matches an active sub-repo. The
	// internal/analysis/gate/coherence.go scope-aware path (R1.3
	// widened to (entities ∪ scopes); R1.5 carve-out for scope-only
	// sub-topics; new R1.8 scope-anchor distribution advisory) reads
	// this lane to distinguish "structural scope" (sub-repo container,
	// active-set membership is authoritative) from "code symbol"
	// (must round-trip through resolver.LookupSymbol).
	//
	// Design doc: docs/design/multirepo_entity_scope_separation.md §4.1.
	PrimaryScopes []string `json:"primary_scopes,omitempty"`
	// MentionedEntities is the deterministic subset of PrimaryEntities
	// whose surface forms are explicitly present in RawRequest. This is
	// the provenance-carrying "user mentioned it" lane: exact-resolution
	// contracts and exact-anchor ranking should prefer this over broader
	// entity lists so analyzer-derived context cannot be upgraded into an
	// exact target accidentally.
	MentionedEntities []string `json:"mentioned_entities,omitempty"`
	// DerivedEntities is the complement lane: entities available for
	// related-context / breadth ranking that are NOT explicitly named in
	// RawRequest. These may come from analyzer expansion, deterministic
	// log/perf augmentation, or sub-topic merge.
	DerivedEntities []string `json:"derived_entities,omitempty"`
	// ExactTargets is the analyzer's optional shortlist of exact
	// user-asked targets (config keys / file paths / symbols / literals).
	// The system validates every item against RawRequest provenance
	// before downstream contracts consume it, so this is an LLM
	// recommendation lane rather than an authority lane.
	ExactTargets []string `json:"exact_targets,omitempty"`
	// ExactContextTerms is the analyzer's optional shortlist of narrow
	// same-scope terms for exact-resolution questions (for example, the
	// identifier-family stem that should guide nearby-context reading).
	// The system validates every term against the request-mentioned
	// exact target lane before using it downstream, so this remains an
	// LLM recommendation rather than a source of truth.
	ExactContextTerms []string `json:"exact_context_terms,omitempty"`
	// ExactContextRoles is the analyzer's optional shortlist of
	// abstract nearby-context roles the user explicitly asked to see
	// when explaining an exact target's precedence / lineage. These
	// are abstract layers such as default/config/runtime/override, not
	// repo-specific file names. The system validates and normalizes the
	// roles before using them for closure and final-answer coverage.
	ExactContextRoles []EvidenceDiagramRole `json:"exact_context_roles,omitempty"`
	// CapabilitySurface is the compiled authority lane for stage/tool
	// capability questions. It tells downstream stages which stage ->
	// skill -> tool exposure surface answers the question, and which
	// source files are the canonical authority set for closure.
	CapabilitySurface *CapabilitySurfaceHint `json:"capability_surface,omitempty"`
	// RequiredFileHints is the analyzer LLM's optional per-file
	// relevance recommendation. Each entry pairs a repo-relative
	// path with a confidence ∈ [0,1] and a short rationale. The
	// downstream explorer threshold-gates on confidence:
	//
	//   confidence ≥ 0.8   → file is treated as effective primary
	//                         AND eligible for prompt pre-read
	//                         injection (high signal: LLM judged
	//                         this file structurally needed).
	//   0.5 ≤ conf < 0.8   → soft hint only; file is eligible for
	//                         pre-read injection at lower priority
	//                         but does NOT augment primary-files.
	//   confidence < 0.5   → discarded (LLM is unsure; let the
	//                         deterministic resolver decide).
	//
	// The field is OPTIONAL. Empty slice falls through to the
	// existing post-emit entity→file resolver
	// (analyzer.go::analyzerRequiredFiles) — no migration needed.
	//
	// L3 (2026-05-10): added to bypass the entity→file resolver's
	// failure modes for qualified names + over-broad RequiredFiles.
	// The LLM has full context (user question + repo summary +
	// pre-scan results) so it can judge file relevance better than
	// any post-emit heuristic.
	//
	// Design doc: docs/design/forced_read_remediation.md §4.4.
	RequiredFileHints []RequiredFileHint `json:"required_file_hints,omitempty"`
	// IrrelevantFiles is the negative-channel counterpart of
	// RequiredFileHints. The analyzer LLM may explicitly list paths
	// it has already inspected during pre-scan and judged
	// off-topic for the user's question. Downstream agents will
	// not re-inject these files via pre-read pools, mid-loop
	// "Read these next:" hints, or evidence-filter primary set.
	//
	// Use case: when the analyzer's pre-scan harvested many
	// candidate files but only a few are actually about the user's
	// subject, declaring the misses keeps subsequent dispatches
	// from wasting prompt tokens re-pushing them. The customer-
	// reported phenomenon: model complained about being forced to
	// read the same 3 unrelated files repeatedly even after it
	// had judged them irrelevant.
	//
	// Optional throughout. Empty list → no negative gating, the
	// deterministic resolver decides. Cap of 10 entries prevents
	// abuse; paths are POSIX-canonicalised by the decoder.
	//
	// Cross-language: paths are repo-relative POSIX (forward-slash,
	// no leading "./") and language-agnostic; the explorer's
	// canonicalExplorerPath normalises before comparison.
	//
	// L4 (2026-05-10). Design doc:
	// docs/design/forced_read_remediation.md §4.5.
	IrrelevantFiles []string `json:"irrelevant_files,omitempty"`
	Kind            string   `json:"kind,omitempty"`
}

// RequiredFileHint is one entry in AnalyzerHints.RequiredFileHints —
// the analyzer's per-file relevance recommendation with confidence
// and rationale. Cross-language: paths are repo-relative POSIX-style
// (canonicalised via canonicalExplorerPath at consume sites); rationale
// is free-form natural language.
type RequiredFileHint struct {
	// Path is the repo-relative file path (e.g.
	// "internal/analysis/gate/gate.go"). Must be non-empty after
	// strict-decode validation. Forward-slash normalised.
	Path string `json:"path"`
	// Confidence is the analyzer's recommendation strength ∈ [0,1].
	// 0 = "I'm not confident", 1 = "this file is definitely needed".
	// Rejected by strict-decode if outside the range or NaN.
	Confidence float64 `json:"confidence"`
	// Rationale is a short natural-language reason for the
	// recommendation (e.g. "directly implements the gate.Run with
	// 9 sequential check calls"). Required when confidence ≥ 0.5
	// (the threshold below which the field is discarded anyway).
	// Length cap 200 chars to keep prompt budget bounded.
	Rationale string `json:"rationale,omitempty"`
}

type Intent string

const (
	IntentExplain     Intent = "explain"
	IntentRootCause   Intent = "root_cause"
	IntentTrace       Intent = "trace"
	IntentEnumerate   Intent = "enumerate"
	IntentConfigQuery Intent = "config_query"
	IntentReturnValue Intent = "return_value"
	IntentUnknown     Intent = "unknown"
	// Write-mode note: there is no IntentWriteCode constant today.
	// Write-mode dispatch is signalled entirely by BusContext.Mode
	// (ModePlan / ModeApply / ModeVerify) — the analyzer stage runs
	// unchanged in write mode and its Intent classification describes
	// the user's request in read-mode terms. When a future B3 wires
	// ScenarioWriteProposal into the compiler, add a "write_code"
	// Intent case back alongside normalizeIntent + analyzer prompt
	// updates; until then, absence of the constant is deliberate.
)

type Scenario string

const (
	ScenarioArchitectureExplain   Scenario = "architecture_explain"
	ScenarioRootCause             Scenario = "root_cause"
	ScenarioConfigTrace           Scenario = "config_trace"
	ScenarioPerformanceBottleneck Scenario = "performance_bottleneck"
	ScenarioGeneric               Scenario = "generic"
	// Write-mode note: there is no ScenarioWriteProposal constant
	// today. Write mode bypasses compiler.Compile entirely; the
	// analyzer's scenario selection applies to read-mode TaskGraph
	// generation. Adding a write scenario requires templateWriteProposal
	// + scheduler stageMapping + readyExplorerWindow integration —
	// see the B3 follow-up in project_session34_b2_shipped.md.
)

type Complexity string

const (
	ComplexitySimple   Complexity = "simple"
	ComplexityModerate Complexity = "moderate"
	ComplexityComplex  Complexity = "complex"
)

type Ambiguity struct {
	Clause     string   `json:"clause"`
	Options    []string `json:"options,omitempty"`
	Resolution string   `json:"resolution,omitempty"`
}

// SubTopic is one independently-answerable sub-topic within a multi-topic
// user question. Produced by the analyzer LLM in the sub_topics field of
// emit_analysis. The compiler maps each SubTopic to a dedicated evidence
// DAG node so the scheduler tracks per-topic investigation progress.
type SubTopic struct {
	Summary  string   `json:"summary"`
	Entities []string `json:"entities,omitempty"`
	// Scopes is the per-sub-topic typed sub-repo-name lane parallel
	// to Entities. Same projection rule as AnalyzerHints.PrimaryScopes:
	// any SubTopic.Entities surface that matches an active sub-repo's
	// RootRel (NormalizeCodeKey-equivalent) is COPIED here at
	// post-emit time by internal/agent/scope_projection.go::projectSubTopicScopes.
	// Coherence R1.3 considers (Entities ∪ Scopes) when checking
	// PrimaryEntities overlap; R1.5 skips per-entity resolver lookup
	// for scope-only sub-topics (active-set membership is authoritative).
	//
	// Design doc: docs/design/multirepo_entity_scope_separation.md §4.1.
	Scopes []string `json:"scopes,omitempty"`
}

// ── TermGraph ───────────────────────────────────────────────────────────

// TermGraph is the canonical surface graph built by
// internal/analysis/normalizer.Normalize from the surfaces
// EXTRACTED FROM rm.RawRequest TEXT. Plus repomap-resolved Kind /
// Confidence / aliases.
//
// CAVEAT for downstream readers (especially amplifier-style rules
// that derive structure from "what entities did the LLM
// identify?"): TermGraph is NOT a mirror of the LLM's emit_analysis
// output. Entity names that the LLM names in its analysis but that
// the user never typed verbatim into the question text WILL NOT
// appear in TermGraph.Canonical, and they WILL NOT carry
// TermKind=TermSymbol. Chinese / natural-language questions almost
// always fit this shape. For "subjects the LLM identified" use
// rm.AnalyzerHints.Entities instead.
//
// This caveat bit Phase 2.1 of the analyzer amplifier (commit
// 8dfd27b → 7bf5ce5 → bc340a4); see
// internal/analysis/amplifier/trap_fixture_test.go for the
// regression-defending fixture.
type TermGraph struct {
	Canonical []CanonicalTerm `json:"canonical"`
	Aliases   []TermAlias     `json:"aliases,omitempty"`
}

type CanonicalTerm struct {
	ID         string   `json:"id"`
	Surface    string   `json:"surface"`
	Language   string   `json:"language"`
	Kind       TermKind `json:"kind"`
	Domain     string   `json:"domain,omitempty"`
	Confidence float32  `json:"confidence"`
}

type TermKind string

const (
	TermSymbol  TermKind = "symbol"
	TermConcept TermKind = "concept"
	TermConfig  TermKind = "config"
	TermCommand TermKind = "command"
	TermLiteral TermKind = "literal"
)

type TermAlias struct {
	Source     string  `json:"source"`
	Target     string  `json:"target"`
	Relation   string  `json:"relation"`
	Confidence float32 `json:"confidence"`
}

// ── TaskGraph ───────────────────────────────────────────────────────────

type TaskGraph struct {
	Nodes           []TaskNode      `json:"nodes"`
	Edges           []TaskEdge      `json:"edges,omitempty"`
	ExecutionPolicy ExecutionPolicy `json:"execution_policy"`
}

type TaskNode struct {
	ID        string       `json:"id"`
	Type      TaskNodeType `json:"type"`
	Objective string       `json:"objective"`

	// pending(artifact-exchange): declared input artifact type slots
	// for a future per-node typed data-flow contract. Compiler
	// templates fill them with snake_case identifiers so the values
	// remain human-readable, but no runtime consumer reads this
	// field today. Gate's pending_fields_wellformed check enforces
	// the identifier shape; criterion_resolvable does not scan this
	// field because it is not a Criterion.
	Inputs []string `json:"inputs,omitempty"`

	// pending(artifact-exchange): declared output artifact type
	// slots. Paired with ExitArtifacts which will hold the concrete
	// produced IDs at runtime. Same pending semantics as Inputs.
	Outputs []string `json:"outputs,omitempty"`

	EntryConditions []Criterion `json:"entry_conditions,omitempty"`

	// pending(artifact-exchange): runtime-populated artifact IDs the
	// node actually produced, keyed to Outputs slot names. Will be
	// read by a future artifact store; today nothing populates or
	// reads it at runtime.
	ExitArtifacts []string `json:"exit_artifacts,omitempty"`

	SuccessCriteria  []Criterion `json:"success_criteria,omitempty"`
	Hypotheses       []string    `json:"hypotheses,omitempty"`
	SearchHints      SearchHints `json:"search_hints"`
	IsCounterfactual bool        `json:"is_counterfactual,omitempty"`
	MaxRetries       int         `json:"max_retries,omitempty"`

	// OneShot caps the node at one successful dispatch. After
	// markDone the scheduler skips it on subsequent ready-window
	// scans even when the node still satisfies its EntryConditions.
	// Used for terminal-shape nodes: NodeFinalize (read mode) and
	// NodePlan (write mode); Apply/Verify are NOT OneShot so the
	// verify→plan retry cycle can re-queue them via
	// EdgeValidationFeedback. Defaults to false to preserve the
	// pre-T4 yield-on-EntryConditions semantics for read explorer
	// nodes.
	OneShot bool `json:"one_shot,omitempty"`

	// SkipOnFirstVisit causes the scheduler to mark the node as done
	// without dispatching the agent on the FIRST entry, but allows
	// later visits (triggered via EdgeValidationFeedback retry) to
	// dispatch normally. Used by the write-mode plan node when the
	// user supplied --plan-file: the disk plan is loaded into Mutable
	// by applyPreHook, so re-emitting on the first iteration would be
	// wasted work. On retry (verify failed → clearForReplan wipes
	// planPath + ChangePlan), the second visit DOES dispatch and
	// regenerates the plan informed by Mutable.PlanningHint.
	//
	// Implemented in the write scheduler via a per-node visit counter
	// so the same field works whether or not the node is OneShot.
	// Defaults to false to preserve pre-existing behaviour for every
	// node not explicitly opted in.
	SkipOnFirstVisit bool `json:"skip_on_first_visit,omitempty"`
}

type TaskNodeType string

const (
	NodeProbe     TaskNodeType = "probe"
	NodeEvidence  TaskNodeType = "evidence"
	NodeValidate  TaskNodeType = "validate"
	NodeReconcile TaskNodeType = "reconcile"
	NodeFinalize  TaskNodeType = "finalize"

	// Write-mode node types. Each maps to a dedicated PipelineStage
	// via orchestrator/scheduler.go::stageMapping. Plan/Apply/Verify
	// run sequentially in the linear graph that
	// orchestrator/write_graph.go::BuildWriteTaskGraph emits when
	// BusContext.Mode is ModePlan / ModeApply / ModeVerify, and never
	// fire in read-only scenarios.
	NodePlan   TaskNodeType = "plan"   // planner emits ChangePlan artifact
	NodeApply  TaskNodeType = "apply"  // coder applies ChangeUnits inside worktree
	NodeVerify TaskNodeType = "verify" // verifier runs tests + asserts no regression
)

type TaskEdge struct {
	From     string   `json:"from"`
	To       string   `json:"to"`
	EdgeType EdgeType `json:"edge_type"`
	Guard    string   `json:"guard,omitempty"`
}

type EdgeType string

const (
	EdgeHardDependency     EdgeType = "hard_dependency"
	EdgeSoftDependency     EdgeType = "soft_dependency"
	EdgeValidationFeedback EdgeType = "validation_feedback"
	// EdgeValidationFeedback also drives the write-mode verify→plan
	// retry cycle: BuildWriteTaskGraph emits an edge from the verify
	// node back to the plan node so a verify SuccessCriteria failure
	// (TestsPass / NoRegression) requeues the plan via
	// requeueValidationTargets. The orchestrator's clearForReplan
	// helper resets ChangePlan / WriteClosure.AppliedSet / PlanPath
	// before the requeue and seeds Mutable.PlanningHint with the
	// failure summary so the planner's next dispatch sees the
	// retry rationale.
)

type ExecutionPolicy struct {
	MaxParallelism int      `json:"max_parallelism"`
	CriticalPath   []string `json:"critical_path,omitempty"`
	RetryBudget    int      `json:"retry_budget"`
}

type Criterion struct {
	Kind string `json:"kind"`
	Expr string `json:"expr"`
}

// CriterionKind constants — the closed namespace of legal Kind
// values for Criterion fields. Every raw string that appears in a
// Criterion{Kind: ...} literal, a switch case, or a compiler
// template MUST use one of these constants so a typo is caught at
// compile time. The criterion package's evaluator dispatch table
// and the gate's criterion_resolvable check both reference this
// same set.
const (
	CritSymbolPresent                 = "symbol_present"
	CritNoCallSites                   = "no_call_sites"
	CritAnswerSetBounded              = "answer_set_bounded"
	CritAnswerSetUnbounded            = "answer_set_unbounded"
	CritMultipleResolutionChains      = "multiple_resolution_chains"
	CritUserClauseUnresolved          = "user_clause_unresolved"
	CritUntrustedReachesSink          = "untrusted_reaches_sink"
	CritInvariantBroken               = "invariant_broken"
	CritNoRelevantEvidence            = "no_relevant_evidence"
	CritSignalPresent                 = "signal_present"
	CritHasEnoughFacts                = "has_enough_facts"
	CritAllHypothesesDecided          = "all_hypotheses_decided"
	CritContractSatisfied             = "contract_satisfied"
	CritBudgetExhausted               = "budget_exhausted"
	CritEvidenceCount                 = "evidence_count"
	CritCitationCountGE               = "citation_count_ge"
	CritContainsSymbol                = "contains_symbol"
	CritRegexMatch                    = "regex_match"
	CritCounterfactualBranchesDecided = "counterfactual_branches_decided"
	CritRelationAbsent                = "relation_absent"

	// CritExternalArtifactDecoded fires when an external artifact
	// (LogBundle / PerfBundle on MutableState) was successfully
	// triaged AND the final answer's summary + body references at
	// least cgec_external_artifact_decoded_floor (default 0.4) of
	// the bundle-extracted non-path tokens. Used in
	// AnswerContract.AcceptanceTests so the contract gate enforces
	// that user-pasted diagnostic content actually appears in the
	// answer rather than being silently dropped in favour of repo
	// citations alone.
	//
	// Trigger discipline: STRUCTURAL only — analyzer adds the
	// criterion when bus.Mutable.LogTriage() != nil OR
	// PerfTrace() != nil. No keyword classification, no intent
	// matching. Generalises to any future "structured artifact"
	// surface by appending the new bundle to
	// criterion.collectExternalArtifactTokens.
	CritExternalArtifactDecoded = "external_artifact_decoded"

	// Write-mode criteria (B0). Evaluators live in
	// internal/analysis/criterion/eval.go and read
	// MutableState.WriteClosure for their ground truth. B0 ships
	// grammar constants + registered entries only; real evaluator
	// bodies land with emit_change_plan / apply_patch / run_tests
	// (Day 5, plus B2/B3 for the verify-level assertions).
	//
	//   - CritPlanReady: a ChangePlan artifact exists (either in
	//     WriteClosure.pendingApplies or on disk at PlanPath) and
	//     its Status is "approved". Used as EntryCondition on
	//     NodeApply to gate Apply on Plan completion.
	//
	//   - CritPatchApplies: every ChangeUnit in the current plan
	//     has landed successfully in the worktree (WriteClosure
	//     .AppliedSet ⊇ plan.TargetPaths). Used as SuccessCriterion
	//     on NodeApply.
	//
	//   - CritTestsPass: every assertion named in plan.AcceptanceTests
	//     has a WriteClosure.VerifyResult with Passed=true. Used as
	//     SuccessCriterion on NodeVerify.
	//
	//   - CritNoRegression: no MetricDelta in ChangeReport regressed
	//     past its configured threshold. Used as SuccessCriterion on
	//     NodeVerify. Expr is either empty (check all metrics) or a
	//     specific metric name.
	CritPlanReady    = "plan_ready"
	CritPatchApplies = "patch_applies"
	CritTestsPass    = "tests_pass"
	CritNoRegression = "no_regression"
)

type SearchHints struct {
	KeywordIDs []string `json:"keyword_ids,omitempty"`
	EntityIDs  []string `json:"entity_ids,omitempty"`
}

// ── EvidencePlan ────────────────────────────────────────────────────────

type EvidencePlan struct {
	Budget EvidenceBudget `json:"budget"`

	// SourceMix is the template-level declaration of per-tool
	// budget ratios (e.g. grep:30, repomap:40, read:30). The
	// sourcemix package compiles it into NodeBudgetHints at analyze
	// time; explorer consumes the compiled form, never this raw map
	// directly. Kept as the declaration source so future template
	// edits stay human-readable and diffable.
	SourceMix map[string]int `json:"source_mix,omitempty"`

	// NodeBudgetHints is the compiled, machine-consumable form of
	// SourceMix. explorerEvaluator reads it on every tool dispatch
	// to throttle per-tool calls inside the explore window.
	NodeBudgetHints NodeBudgetHints `json:"node_budget_hints"`

	StopConditions []StopCondition `json:"stop_conditions,omitempty"`

	// RequiredFiles is the analyzer's up-front list of repo-relative
	// file paths that downstream agents SHOULD consider relevant to
	// answering the user's question. Populated at analyze-time by
	// running the repo_map graph query with the analyzer-extracted
	// entities as the query and pulling the top-N scored files.
	//
	// Unlike SourceMix (which is a budget hint) and StopConditions
	// (which are termination gates), RequiredFiles carries a
	// declarative coverage signal: "these are the files I expect
	// you to read or grep to answer the question". The explorer
	// merges this list into its own keyword_search ranking so
	// high-confidence analyzer-picked files get priority alongside
	// grep IDF scores — and the CGEC phase1-unread gate counts
	// them against ReadSet for the pre-complete check.
	//
	// T3a (added after the 2026-04-18 "explorer是怎么调用subagent的？"
	// debug): closes the gap where the analyzer's repo_map query
	// identified the right files but the results were only rendered
	// into prose prompt text and never carried structurally to the
	// explorer's coverage tracker. Zero-value-safe: an empty slice
	// is the fallback when the analyzer runs without a repo_map
	// graph (sub-agent contexts, unit tests, or repo_map build
	// failures) and downstream code treats it as "no hint".
	RequiredFiles []string `json:"required_files,omitempty"`
}

// NodeBudgetHints is the compiled shape of EvidencePlan.SourceMix.
// PerToolCap is keyed by tool name (e.g. "grep", "read_file") and
// gives the maximum number of times that tool may fire inside one
// explore window. OverallCap is a ceiling on total tool calls across
// all tools — 0 means "fall through to EvidenceBudget.MaxToolCalls".
type NodeBudgetHints struct {
	PerToolCap map[string]int `json:"per_tool_cap,omitempty"`
	OverallCap int            `json:"overall_cap,omitempty"`
}

type EvidenceBudget struct {
	MaxFiles      int `json:"max_files"`
	MaxBytes      int `json:"max_bytes"`
	MaxReactIters int `json:"max_react_iters"`
	MaxToolCalls  int `json:"max_tool_calls"`
}

type StopCondition struct {
	Kind string `json:"kind"`
	Expr string `json:"expr,omitempty"`
}

// ── AnswerContract ──────────────────────────────────────────────────────

type AnswerContract struct {
	Diagram                 *DiagramContract                 `json:"diagram,omitempty"`
	ExactResolution         *ExactResolutionContract         `json:"exact_resolution,omitempty"`
	CurrentStatusDiagnostic *CurrentStatusDiagnosticContract `json:"current_status_diagnostic,omitempty"`
	MustInclude             []string                         `json:"must_include,omitempty"`
	MustIncludeTerms        []ContractTerm                   `json:"must_include_terms,omitempty"`
	MustExclude             []string                         `json:"must_exclude,omitempty"`
	MustExcludeTerms        []ContractTerm                   `json:"must_exclude_terms,omitempty"`
	CitationReq             CitationReq                      `json:"citation_requirements"`
	AcceptanceTests         []Criterion                      `json:"acceptance_tests,omitempty"`
	Language                string                           `json:"language"`
}

// ContractTermKind tells the answer-contract checker what a required
// term represents. The checker can then choose the right matching
// oracle: source-code symbols use SymbolOracle, tool names and file
// stems use literal/token-form matching, and user phrases use phrase
// containment. This prevents tool/file-stem terms such as
// "emit_evidence" from being treated as ordinary Go symbols.
type ContractTermKind string

const (
	ContractTermSymbol     ContractTermKind = "symbol"
	ContractTermToolName   ContractTermKind = "tool_name"
	ContractTermFileStem   ContractTermKind = "file_stem"
	ContractTermUserPhrase ContractTermKind = "user_phrase"
)

func (k ContractTermKind) IsValid() bool {
	switch k {
	case ContractTermSymbol, ContractTermToolName, ContractTermFileStem, ContractTermUserPhrase:
		return true
	}
	return false
}

type ContractTerm struct {
	Text string           `json:"text"`
	Kind ContractTermKind `json:"kind"`
}

type CurrentStatusVerdict string

const (
	CurrentStatusStillPresent      CurrentStatusVerdict = "still_present"
	CurrentStatusFixed             CurrentStatusVerdict = "fixed"
	CurrentStatusNotEnoughEvidence CurrentStatusVerdict = "not_enough_evidence"
)

// CurrentStatusDiagnosticContract is set when the user's diagnostic
// request asks whether a historical / observed issue still exists in
// the current checkout. It pins the final answer to three surfaces:
// historical observation, current-code verification, and a bounded
// verdict.
type CurrentStatusDiagnosticContract struct {
	Required                     bool                   `json:"required"`
	AllowedVerdicts              []CurrentStatusVerdict `json:"allowed_verdicts,omitempty"`
	RequireHistoricalObservation bool                   `json:"require_historical_observation"`
	RequireCurrentVerification   bool                   `json:"require_current_verification"`
}

type DiagramKind string

const (
	DiagramNone         DiagramKind = ""
	DiagramFlow         DiagramKind = "flow"
	DiagramSequence     DiagramKind = "sequence"
	DiagramCallDAG      DiagramKind = "call_dag"
	DiagramArchitecture DiagramKind = "architecture"
)

func AllDiagramKinds() []DiagramKind {
	return []DiagramKind{
		DiagramNone,
		DiagramFlow,
		DiagramSequence,
		DiagramCallDAG,
		DiagramArchitecture,
	}
}

func (k DiagramKind) IsValid() bool {
	for _, declared := range AllDiagramKinds() {
		if k == declared {
			return true
		}
	}
	return false
}

type DiagramScope string

const (
	DiagramScopeOverall     DiagramScope = "overall"
	DiagramScopePerSubTopic DiagramScope = "per_subtopic"
)

func AllDiagramScopes() []DiagramScope {
	return []DiagramScope{
		DiagramScopeOverall,
		DiagramScopePerSubTopic,
	}
}

func (s DiagramScope) IsValid() bool {
	for _, declared := range AllDiagramScopes() {
		if s == declared {
			return true
		}
	}
	return false
}

// DiagramHint is the analyzer LLM's optional pre-contract suggestion.
// The compiler consumes it as one signal among several stronger
// structural signals when deriving the final DiagramContract.
type DiagramHint struct {
	Kind DiagramKind `json:"kind"`
}

// DiagramContract is the finalizer-facing presentation contract for
// answers that need a grounded structural diagram in summary. It is
// intentionally orthogonal to the principal answer block — a scalar
// or boolean answer may still require a diagram when the question
// is fundamentally about flow, call relationships, timing, or
// architecture.
type DiagramContract struct {
	Required       bool          `json:"required"`
	Minimum        int           `json:"minimum,omitempty"`
	PreferredKinds []DiagramKind `json:"preferred_kinds,omitempty"`
	ScopeHint      DiagramScope  `json:"scope_hint,omitempty"`
	Reasons        []string      `json:"reasons,omitempty"`
}

type ExactResolutionContextPolicy string

const (
	ExactContextGroundedOnly          ExactResolutionContextPolicy = "grounded_only"
	ExactContextSameFamilyGrounded    ExactResolutionContextPolicy = "same_family_grounded"
	ExactContextSameDirectoryGrounded ExactResolutionContextPolicy = "same_directory_grounded"
)

func AllExactResolutionContextPolicies() []ExactResolutionContextPolicy {
	return []ExactResolutionContextPolicy{
		ExactContextGroundedOnly,
		ExactContextSameFamilyGrounded,
		ExactContextSameDirectoryGrounded,
	}
}

func (p ExactResolutionContextPolicy) IsValid() bool {
	for _, declared := range AllExactResolutionContextPolicies() {
		if p == declared {
			return true
		}
	}
	return false
}

// ExactResolutionContract is the finalizer-facing contract for
// exact-target questions where the user named a concrete config key,
// file path, symbol, route, or literal and the answer must resolve
// THAT exact target before any nearby context is discussed.
//
// The contract is intentionally orthogonal to the principal answer
// block. A scalar or explanation answer may still need to (a)
// explicitly name the requested target, (b) allow honest "absent"
// outcomes, and (c) require explicit proof before claiming a
// nearby item is an alias / equivalent / substitute.
type ExactResolutionContract struct {
	TargetKind              AnswerSubjectKind            `json:"target_kind,omitempty"`
	TargetLabel             string                       `json:"target_label,omitempty"`
	Targets                 []string                     `json:"targets,omitempty"`
	AllowAbsence            bool                         `json:"allow_absence,omitempty"`
	RequireTargetMention    bool                         `json:"require_target_mention,omitempty"`
	AliasRequiresProof      bool                         `json:"alias_requires_proof,omitempty"`
	RelatedContextPolicy    ExactResolutionContextPolicy `json:"related_context_policy,omitempty"`
	RelatedContextScopeHint string                       `json:"related_context_scope_hint,omitempty"`
	RelatedContextTerms     []string                     `json:"related_context_terms,omitempty"`
	RequestedContextRoles   []EvidenceDiagramRole        `json:"requested_context_roles,omitempty"`
}

// type AnswerShape and the seven Shape* constants (ShapeListOfSymbols /
// ShapeStepList / ShapeValue / ShapeBoolean / ShapeConfigValue /
// ShapeExplanation / ShapeNone) were retired with the AnswerShape
// terminal-retirement migration (PR5, 2026-05-03). The V2 block-only
// AnswerDocumentV2 carrier derives rendering from
// AnswerSemanticView (compiled per QuestionFamily) — there is no
// longer a typed shape enum. See docs/migration/
// answer_shape_terminal_retirement.md for the full rationale.

type CitationReq struct {
	Required     bool   `json:"required"`
	Granularity  string `json:"granularity"`
	MinCitations int    `json:"min_citations"`
}

// ── HypothesisSet ───────────────────────────────────────────────────────

type Hypothesis struct {
	ID                     string           `json:"id"`
	Statement              string           `json:"statement"`
	RequiredEvidence       []Criterion      `json:"required_evidence,omitempty"`
	FalsificationCondition Criterion        `json:"falsification_condition"`
	Priority               int              `json:"priority"`
	Status                 HypothesisStatus `json:"status"`
}

type HypothesisStatus string

const (
	HypUnknown      HypothesisStatus = "unknown"
	HypConfirmed    HypothesisStatus = "confirmed"
	HypRejected     HypothesisStatus = "rejected"
	HypInconclusive HypothesisStatus = "inconclusive"
)

// MarkHypothesis is the P2.1 D7 carve-out documented in the IR
// header invariant: "Other stages may mutate
// HypothesisSet[*].Status and TaskGraph.Nodes[*] execution state via
// dedicated APIs but must not rewrite the structural fields." This
// method is the dedicated API for the Status mutation. It is the
// ONLY supported way to write back per-hypothesis verdicts produced
// by the extractor (Turn B) — direct field assignment from outside
// internal/types is forbidden by convention so the carve-out stays
// auditable from a single grep.
//
// The method takes id and status by value, validates both, and
// rejects unknown IDs and unknown statuses loudly. Rationale and
// citation are intentionally NOT carried into the IR — they live on
// MutableState's verdict buffer (see HypothesisVerdict struct in
// context.go) so the structural Hypothesis type stays minimal. The
// orchestrator's drain hook in Session 2 reads both halves: it
// calls MarkHypothesis to update the IR and renders the rationale /
// citation via the finalizer prompt builder.
//
// memory/project_p1_3_deferred_items.md D7 names this as
// "MutableState.MarkHypothesis" — implementing it on AnalysisIR
// instead keeps the IR write encapsulated with the type that owns
// the field, and avoids storing an aliased *AnalysisIR pointer on
// MutableState (which would reach across the analyzer's
// single-writer invariant). The deferred memo will be updated to
// point at this location when P2.1 ships.
func (ir *AnalysisIR) MarkHypothesis(id string, status HypothesisStatus) error {
	if ir == nil {
		return fmt.Errorf("MarkHypothesis: nil AnalysisIR")
	}
	if id == "" {
		return fmt.Errorf("MarkHypothesis: empty hypothesis id")
	}
	switch status {
	case HypUnknown, HypConfirmed, HypRejected, HypInconclusive:
		// known status
	default:
		return fmt.Errorf("MarkHypothesis: unknown status %q (allowed: unknown, confirmed, rejected, inconclusive)", status)
	}
	for i := range ir.HypothesisSet {
		if ir.HypothesisSet[i].ID == id {
			ir.HypothesisSet[i].Status = status
			return nil
		}
	}
	return fmt.Errorf("MarkHypothesis: hypothesis id %q not found in HypothesisSet (size %d)", id, len(ir.HypothesisSet))
}

// ── RiskMatrix ──────────────────────────────────────────────────────────

type RiskMatrix struct {
	Security      RiskLevel `json:"security"`
	DataIntegrity RiskLevel `json:"data_integrity"`
	Compatibility RiskLevel `json:"compatibility"`
	Performance   RiskLevel `json:"performance"`
	Ops           RiskLevel `json:"ops"`
	Compliance    RiskLevel `json:"compliance"`
}

// RiskLevel is a 0..5 score with supporting free-text evidence.
// 0 = not relevant, 1 = trivial, 2 = minor, 3 = moderate,
// 4 = high, 5 = critical.
type RiskLevel struct {
	Level    int      `json:"level"`
	Evidence []string `json:"evidence,omitempty"`
}

// ── Quality Gate ────────────────────────────────────────────────────────

type GateReport struct {
	Passed    bool        `json:"passed"`
	Rejected  bool        `json:"rejected"`
	Retryable bool        `json:"retryable"`
	Checks    []GateCheck `json:"checks,omitempty"`
	// Fingerprint is a stable hash of the failure SHAPE (rule names
	// + rule prefixes from each failed check Detail). Empty when
	// Rejected==false. The orchestrator's analyze retry loop
	// (orchestrator.go::runAnalyzePhase) compares fingerprints
	// across attempts to detect a retry storm: when the SAME
	// fingerprint repeats ≥ ceil(maxRetries/2) consecutive
	// attempts, the LLM is stuck in a loop (each attempt
	// produces a structurally distinct emit but each new shape
	// fails the same gate combination). Early-exit to the
	// degraded path saves LLM round-trips and surfaces a
	// user-actionable diagnostic.
	//
	// Forensic anchor: 2026-05-10 chatpp-7d46dee4 log L1320 +
	// L2714 — TWO turns each burning the full 3-attempt budget on
	// the same subtopic_coherence gate. Pre-B6 the orchestrator
	// could not distinguish "LLM is converging" from "LLM is
	// stuck"; with the fingerprint it can.
	//
	// Design doc: docs/design/multirepo_entity_scope_separation.md §4.6.
	Fingerprint string `json:"fingerprint,omitempty"`
}

type GateCheck struct {
	Name      string  `json:"name"`
	Passed    bool    `json:"passed"`
	Score     float32 `json:"score"`
	Threshold float32 `json:"threshold"`
	Detail    string  `json:"detail,omitempty"`
}
