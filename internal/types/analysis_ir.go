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
const AnalysisIRVersion = "v4"

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

	// IntentConfidence / ComplexityConfidence / KindConfidence /
	// ShapeConfidence are LLM-emitted certainty scores in [0.0, 1.0]
	// for each classification dimension. Zero = "no opinion" (unusual
	// — the schema asks the LLM to rate explicitly). Downstream
	// consumers gate aggressive narrowing on confidence (e.g. the
	// explorer's tightenDeclarativeFrontier only fires when
	// KindConfidence ≥ 0.7) so a hesitant classification cannot
	// over-narrow the read set.
	IntentConfidence     float64 `json:"intent_confidence,omitempty"`
	ComplexityConfidence float64 `json:"complexity_confidence,omitempty"`
	KindConfidence       float64 `json:"kind_confidence,omitempty"`
	ShapeConfidence      float64 `json:"shape_confidence,omitempty"`

	// Predicates carries the LLM's semantic self-assessment of the
	// question along axes that the prose-cue tables used to detect
	// (count question, scalar answer, cross-component, …). Replaces
	// language-specific keyword tables with cross-language LLM judgement.
	// Schema requires every field present (true or false) so a missing
	// field is a fail-loud signal, not a silent false.
	Predicates SemanticPredicates `json:"predicates"`

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
	SubjectUnknown        AnswerSubjectKind = ""
	SubjectFunctionName   AnswerSubjectKind = "function_name"
	SubjectTypeName       AnswerSubjectKind = "type_name"
	SubjectHandlerRoute   AnswerSubjectKind = "handler_route"
	SubjectConfigKey      AnswerSubjectKind = "config_key"
	SubjectReturnValue    AnswerSubjectKind = "return_value"
	SubjectFilePath       AnswerSubjectKind = "file_path"
	SubjectStringLiteral  AnswerSubjectKind = "string_literal"
	SubjectNumeric        AnswerSubjectKind = "numeric"
	SubjectEnumValue      AnswerSubjectKind = "enum_value"
	SubjectStructField    AnswerSubjectKind = "struct_field"
	SubjectInterface      AnswerSubjectKind = "interface_name"
	SubjectGeneric        AnswerSubjectKind = "generic"
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

	// IsCountQuestion: the user is asking "how many X" / "X 的数量" /
	// equivalent in any language. Implies IsScalarAnswer=true.
	// Drives intent downgrade enumerate→return_value and the
	// measurement-scalar carve-out.
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
}

// AnalyzerHints is the raw LLM-extracted analyzer output, mirrored onto
// the IR so downstream consumers have a single canonical read path.
type AnalyzerHints struct {
	Keywords []string `json:"keywords,omitempty"`
	Entities []string `json:"entities,omitempty"`
	// PrimaryEntities snapshots the pre-merge top-level Entities list
	// from the LLM's emit_analysis call, before sub-topic entities
	// (SubTopics[].Entities) are unioned into Entities for breadth
	// consumers (normalizer / keyword_search ranker / entity boost).
	// Consumers that need "user-named entities only" semantics — most
	// notably keyword_search's exactEntityAnchors, which should never
	// anchor on a planner-added sub_topic descriptor — read this
	// instead of Entities. Empty when no sub-topics triggered the
	// merge, in which case consumers fall back to Entities.
	PrimaryEntities []string `json:"primary_entities,omitempty"`
	Kind            string   `json:"kind,omitempty"`
	Shape           string   `json:"shape,omitempty"`
}

type Intent string

const (
	IntentExplain       Intent = "explain"
	IntentRootCause     Intent = "root_cause"
	IntentTrace         Intent = "trace"
	IntentEnumerate     Intent = "enumerate"
	IntentConfigQuery   Intent = "config_query"
	IntentReturnValue   Intent = "return_value"
	IntentUnknown       Intent = "unknown"
)

type Scenario string

const (
	ScenarioArchitectureExplain   Scenario = "architecture_explain"
	ScenarioRootCause             Scenario = "root_cause"
	ScenarioConfigTrace           Scenario = "config_trace"
	ScenarioPerformanceBottleneck Scenario = "performance_bottleneck"
	ScenarioGeneric               Scenario = "generic"
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
}

// ── TermGraph ───────────────────────────────────────────────────────────

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
}

type TaskNodeType string

const (
	NodeProbe     TaskNodeType = "probe"
	NodeEvidence  TaskNodeType = "evidence"
	NodeValidate  TaskNodeType = "validate"
	NodeReconcile TaskNodeType = "reconcile"
	NodeFinalize  TaskNodeType = "finalize"
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
	RequiredAnswerShape AnswerShape `json:"required_answer_shape"`
	MustInclude         []string    `json:"must_include,omitempty"`
	MustExclude         []string    `json:"must_exclude,omitempty"`
	CitationReq         CitationReq `json:"citation_requirements"`
	AcceptanceTests     []Criterion `json:"acceptance_tests,omitempty"`
	Language            string      `json:"language"`
}

type AnswerShape string

const (
	ShapeListOfSymbols AnswerShape = "list_of_symbols"
	ShapeStepList      AnswerShape = "step_list"
	ShapeValue         AnswerShape = "value"
	ShapeBoolean       AnswerShape = "boolean"
	ShapeConfigValue   AnswerShape = "config_value"
	ShapeExplanation   AnswerShape = "explanation"
	ShapeNone          AnswerShape = "none"
)

// IsEmittable reports whether the shape is one a producer (finalizer
// emit_answer_document, analyzer AnswerContract) can actually emit.
// ShapeNone is accepted by the type system as "no shape declared"
// but MUST NOT land in an AnswerDocument — the tool schema rejects
// it so a zero-value drift cannot reach the renderer.
func (s AnswerShape) IsEmittable() bool {
	switch s {
	case ShapeListOfSymbols, ShapeStepList, ShapeValue,
		ShapeBoolean, ShapeConfigValue, ShapeExplanation:
		return true
	}
	return false
}

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
}

type GateCheck struct {
	Name      string  `json:"name"`
	Passed    bool    `json:"passed"`
	Score     float32 `json:"score"`
	Threshold float32 `json:"threshold"`
	Detail    string  `json:"detail,omitempty"`
}
