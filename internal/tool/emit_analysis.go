package tool

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/hanchaoqun/codrax/internal/analysis/prescan"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/skill"
	repomap "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

// EmitAnalysis is the analyzer's single structured exit channel: it
// deposits the classified RequestModel onto BusContext.Mutable so
// analyzer.ParseOutput can run the deterministic IR builder against
// it (normalizer → compiler → risk → hdp → counterfactual → gate).
//
// Contract surface:
//
//   - Parameters() is a pure interface contract — type + enum +
//     required — and carries NO teaching copy. Everything the LLM
//     needs to decide what each enum value means lives in the
//     analysis-skill system prompt built from
//     internal/skill/analysis_contract.go. The schema-drift test in
//     this package pins the enum arrays to the SSOT.
//   - Description() is one sentence: what the tool does and its
//     one-call-per-dispatch constraint. Strategy guidance ("extract
//     entities verbatim", "≥8 keywords", "bilingual for Chinese
//     questions") lives in the skill prompt, not here.
//   - Execute() is the quality gate: json.Unmarshal → runtime
//     validation (AnalysisLimits) → normalization → write to
//     Mutable.RequestModel. The ToolResult.Summary reports the
//     POST-NORMALIZATION state so a trace reader can see exactly
//     what the system persisted versus what the LLM actually sent.
//
// Classified ReadOnly because IsWrite() is the filesystem-write
// boundary; mutating BusContext is not a filesystem write.
type EmitAnalysis struct {
	ReadOnly
	NonEvidenceTool
}

type emitAnalysisParams struct {
	Intent            string                  `json:"intent"`
	Scenario          string                  `json:"scenario"`
	Complexity        string                  `json:"complexity"`
	Keywords          []string                `json:"keywords"`
	Entities          []string                `json:"entities"`
	QuestionKind      string                  `json:"question_kind"`
	Language          string                  `json:"language,omitempty"`
	SubTopics         []types.SubTopic        `json:"sub_topics,omitempty"`
	AnswerSubject     *emitAnswerSubjectParam `json:"answer_subject,omitempty"`
	ExactTargets      []string                `json:"exact_targets,omitempty"`
	ExactContextTerms []string                `json:"exact_context_terms,omitempty"`
	ExactContextRoles []string                `json:"exact_context_roles,omitempty"`

	// Schema v4 — confidence + alternatives + LLM semantic predicates +
	// LLM-emitted predicate axis. These replace the historical prose-cue
	// reconciliation tables (countVerbPrefixes / crossComponentCues /
	// enumerationCuePrefixes / relationalVerbCues / categoryEnumerationCues
	// / predicateVerbMap) with cross-language LLM judgement. All
	// predicate fields are required to be explicit (true OR false) — a
	// missing field is fail-loud, not a silent default.
	IntentConfidence             float64                                `json:"intent_confidence"`
	ComplexityConfidence         float64                                `json:"complexity_confidence"`
	KindConfidence               float64                                `json:"kind_confidence"`
	Predicates                   *emitPredicatesParam                   `json:"predicates"`
	DiagnosticProfile            *emitDiagnosticProfileParam            `json:"diagnostic_profile"`
	ConversationReferenceProfile *emitConversationReferenceProfileParam `json:"conversation_reference_profile,omitempty"`
	SourceScopeProfile           *emitSourceScopeProfileParam           `json:"source_scope_profile,omitempty"`
	AnswerVisibilityProfile      *emitAnswerVisibilityProfileParam      `json:"answer_visibility_profile,omitempty"`
	ChangeImpactProfile          *emitChangeImpactProfileParam          `json:"change_impact_profile,omitempty"`
	FieldValueProfile            *emitFieldValueProfileParam            `json:"field_value_profile,omitempty"`
	AnswerExclusionPolicy        *emitAnswerExclusionPolicyParam        `json:"answer_exclusion_policy,omitempty"`
	AnswerRoleProfile            *emitAnswerRoleProfileParam            `json:"answer_role_profile,omitempty"`
	ErrorGranularityProfile      *emitErrorGranularityProfileParam      `json:"error_granularity_profile,omitempty"`
	PredicateAxis                string                                 `json:"predicate_axis,omitempty"`
	DiagramHint                  *emitDiagramHintParam                  `json:"diagram_hint,omitempty"`
	EnumerationBoundary          *emitEnumerationBoundaryParam          `json:"enumeration_boundary,omitempty"`
	// Plan E (2026-05-02) — additional structural-obligation axes.
	CompletenessObligation *emitCompletenessObligationParam `json:"completeness_obligation,omitempty"`
	Buckets                []emitQuestionBucketParam        `json:"buckets,omitempty"`
	// L3 (2026-05-10) — analyzer-emitted per-file recommendations
	// with confidence + rationale. The downstream explorer threshold-
	// gates each entry: ≥0.8 → primary file + pre-read; 0.5–0.79 →
	// pre-read only; <0.5 → discarded. Empty/omitted falls through to
	// the post-emit entity→file resolver.
	RequiredFiles []emitRequiredFileParam `json:"required_files,omitempty"`
	// L4 (2026-05-10) — analyzer-declared irrelevant files. The
	// negative-channel counterpart of required_files: paths the
	// analyzer LLM has read in pre-scan and judged off-topic.
	// Downstream agents respect this as a hard exclusion across
	// pre-read pools, mid-loop hints, and primary-file selection.
	IrrelevantFiles []string `json:"irrelevant_files,omitempty"`
}

// emitRequiredFileParam is the wire shape of one required_files entry.
// Path + Confidence are required; Rationale is recommended for
// confidence ≥ 0.5 entries (the threshold below which the entry is
// dropped anyway).
type emitRequiredFileParam struct {
	Path       string  `json:"path"`
	Confidence float64 `json:"confidence"`
	Rationale  string  `json:"rationale,omitempty"`
}

// emitPredicatesParam is the wire shape of the required `predicates`
// object. Pointer-typed so missing object can be detected via nil
// (validation rejects nil with a fail-loud error). Each bool field
// must be explicitly emitted by the LLM — JSON omission would parse
// to false silently which would mask a classification miss.
type emitPredicatesParam struct {
	IsScalarAnswer        *bool `json:"is_scalar_answer"`
	IsRoleLocateLookup    *bool `json:"is_role_locate_lookup"`
	IsCountQuestion       *bool `json:"is_count_question"`
	IsCrossComponent      *bool `json:"is_cross_component"`
	IsRelationalLookup    *bool `json:"is_relational_lookup"`
	IsCategoryEnumeration *bool `json:"is_category_enumeration"`
	IsHistoryLookup       *bool `json:"is_history_lookup"`
	IsDiagnosticQuestion  *bool `json:"is_diagnostic_question"`
}

// emitDiagnosticProfileParam is the second typed diagnostic lane.
// Predicates.IsDiagnosticQuestion is the primary route; this profile
// carries richer diagnostic facets. Pointer fields let the parser
// distinguish explicit false from omitted so it can conservatively
// default missing mirror data without inventing current-risk /
// historical-regression / current-version obligations.
type emitDiagnosticProfileParam struct {
	IsDiagnostic         *bool    `json:"is_diagnostic"`
	CurrentRisk          *bool    `json:"current_risk"`
	HistoricalRegression *bool    `json:"historical_regression"`
	CurrentVersionCheck  *bool    `json:"current_version_check"`
	ObservationSummary   string   `json:"observation_summary,omitempty"`
	Confidence           *float64 `json:"confidence"`
}

type emitConversationReferenceProfileParam struct {
	RequiresPriorContext  *bool                                  `json:"requires_prior_context"`
	NeedsRepoVerification *bool                                  `json:"needs_repo_verification"`
	Ambiguity             string                                 `json:"ambiguity"`
	ResolvedSubjects      []emitResolvedConversationSubjectParam `json:"resolved_subjects,omitempty"`
}

type emitResolvedConversationSubjectParam struct {
	Surface          string   `json:"surface"`
	Kind             string   `json:"kind,omitempty"`
	Source           string   `json:"source"`
	Role             string   `json:"role,omitempty"`
	UseAsExactTarget *bool    `json:"use_as_exact_target"`
	Confidence       *float64 `json:"confidence"`
}

type emitSourceScopeProfileParam struct {
	RequestedScope              string   `json:"requested_scope"`
	IncludeAuxiliaryAsPrincipal *bool    `json:"include_auxiliary_as_principal,omitempty"`
	Confidence                  *float64 `json:"confidence"`
	Rationale                   string   `json:"rationale,omitempty"`
}

type emitAnswerVisibilityProfileParam struct {
	SymbolVisibility string   `json:"symbol_visibility"`
	SourceQuotes     []string `json:"source_quotes,omitempty"`
	Confidence       *float64 `json:"confidence"`
	Rationale        string   `json:"rationale,omitempty"`
}

type emitChangeImpactProfileParam struct {
	IsChangeImpact    *bool    `json:"is_change_impact"`
	Target            string   `json:"target,omitempty"`
	TargetKind        string   `json:"target_kind,omitempty"`
	Scope             string   `json:"scope,omitempty"`
	RequestedOutput   string   `json:"requested_output,omitempty"`
	AffectedSiteKinds []string `json:"affected_site_kinds,omitempty"`
	Confidence        *float64 `json:"confidence"`
	Rationale         string   `json:"rationale,omitempty"`
}

type emitFieldValueProfileParam struct {
	IsFieldValueLookup *bool    `json:"is_field_value_lookup"`
	Target             string   `json:"target,omitempty"`
	Literal            string   `json:"literal,omitempty"`
	LiteralKind        string   `json:"literal_kind,omitempty"`
	SourceQuote        string   `json:"source_quote,omitempty"`
	Confidence         *float64 `json:"confidence"`
	Rationale          string   `json:"rationale,omitempty"`
}

type emitAnswerExclusionPolicyParam struct {
	IsExclusionRequested   *bool    `json:"is_exclusion_requested"`
	ExcludedCandidateRoles []string `json:"excluded_candidate_roles,omitempty"`
	SourceQuotes           []string `json:"source_quotes,omitempty"`
	Confidence             *float64 `json:"confidence"`
	Rationale              string   `json:"rationale,omitempty"`
}

type emitAnswerRoleProfileParam struct {
	IsRoleBindingRequested *bool    `json:"is_role_binding_requested"`
	RequiredCandidateRoles []string `json:"required_candidate_roles,omitempty"`
	SourceQuotes           []string `json:"source_quotes,omitempty"`
	Confidence             *float64 `json:"confidence"`
	Rationale              string   `json:"rationale,omitempty"`
}

type emitErrorGranularityProfileParam struct {
	IsGranularityQuestion   *bool    `json:"is_granularity_question"`
	RequestedVerdictOptions []string `json:"requested_verdict_options,omitempty"`
	SourceQuotes            []string `json:"source_quotes,omitempty"`
	Confidence              *float64 `json:"confidence"`
	Rationale               string   `json:"rationale,omitempty"`
}

// emitAnswerSubjectParam is the wire shape of the optional
// answer_subject field. The LLM may set kind explicitly when the
// question resolves to a clear answer-literal type; otherwise the
// system's deterministic inferAnswerSubject in analyzer_intent.go
// fills the field at IR-build time.
type emitAnswerSubjectParam struct {
	Kind       string   `json:"kind"`
	EntityAxes []string `json:"entity_axes,omitempty"`
	Confidence float64  `json:"confidence,omitempty"`
}

// emitDiagramHintParam is the wire shape of the optional
// diagram_hint field. It lets the analyzer LLM suggest the diagram
// family that best matches the question, while the deterministic
// compiler still derives the final contract from stronger signals.
type emitDiagramHintParam struct {
	Kind string `json:"kind"`
}

type emitEnumerationBoundaryParam struct {
	DeclaredCount int    `json:"declared_count"`
	SourceQuote   string `json:"source_quote"`
}

// emitCompletenessObligationParam is Plan E's completeness-axis wire
// shape. Required=true means the user asked for an exhaustive answer
// ("all the X" / "every X"); SourceQuote is the verbatim phrase from
// the question that triggered the obligation. Validated at parse
// time via NormalizeCompletenessObligation — fabricated quotes are
// dropped silently (mirrors EnumerationBoundary's grounding).
type emitCompletenessObligationParam struct {
	Required    bool   `json:"required"`
	SourceQuote string `json:"source_quote,omitempty"`
}

// emitQuestionBucketParam is Plan E's bucket-partition wire shape.
// Triggered when the user explicitly partitions the answer into
// labeled groups. Label is verbatim from RawRequest. Anchors[] are
// LLM-resolved entities representing this bucket. Index is 1-based
// in the order the buckets appear in the question.
type emitQuestionBucketParam struct {
	Label   string   `json:"label"`
	Anchors []string `json:"anchors,omitempty"`
	Index   int      `json:"index,omitempty"`
}

func (t *EmitAnalysis) Name() string { return "emit_analysis" }

// Description is a single sentence: what the tool does and its
// one-call-per-dispatch constraint. Strategy guidance — how to pick
// an enum value, how many keywords to emit, what not to put in
// entities — lives in the analysis-skill system prompt, not here.
func (t *EmitAnalysis) Description() string {
	return "Records the classified request model for this dispatch so the " +
		"deterministic analyzer pipeline can assemble the full analysis. " +
		"Call at most once per dispatch."
}

// Parameters returns a purely structural JSON schema: type, enum,
// and required. Enum arrays are pulled from skill.Analysis*Values()
// so the schema cannot drift from the skill prompt; a consistency
// test pins the two sides. There are no "description" fields on
// properties — a field-level teaching surface is precisely what this
// refactor collapsed onto the analysis-skill.
func (t *EmitAnalysis) Parameters() json.RawMessage {
	emitAnalysisSchemaOnce.Do(buildEmitAnalysisSchema)
	return emitAnalysisSchemaCache
}

var (
	emitAnalysisSchemaOnce  sync.Once
	emitAnalysisSchemaCache json.RawMessage
)

func buildEmitAnalysisSchema() {
	type stringProp struct {
		Type string   `json:"type"`
		Enum []string `json:"enum,omitempty"`
	}
	type arrayProp struct {
		Type  string            `json:"type"`
		Items map[string]string `json:"items"`
	}

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"intent":        stringProp{Type: "string", Enum: skill.AnalysisIntentValues()},
			"scenario":      stringProp{Type: "string", Enum: skill.AnalysisScenarioValues()},
			"complexity":    stringProp{Type: "string", Enum: skill.AnalysisComplexityValues()},
			"keywords":      arrayProp{Type: "array", Items: map[string]string{"type": "string"}},
			"entities":      arrayProp{Type: "array", Items: map[string]string{"type": "string"}},
			"question_kind": stringProp{Type: "string", Enum: skill.AnalysisQuestionKindValues()},
			"language":      stringProp{Type: "string", Enum: []string{"zh", "en"}},
			"sub_topics": map[string]any{
				"type":        "array",
				"description": "Independent sub-topics in the user's question. Empty for single-topic questions. Do NOT split topics with causal dependencies.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"summary":  map[string]string{"type": "string", "description": "One-sentence sub-topic description"},
						"entities": map[string]any{"type": "array", "items": map[string]string{"type": "string"}, "description": "Code entities for this sub-topic"},
					},
					"required": []string{"summary"},
				},
			},
			"exact_targets": map[string]any{
				"type":        "array",
				"description": "Optional exact user-asked targets, copied verbatim from the current request text. Use when the request mentions neighboring context items but asks about one specific key/path/symbol/literal. Every item must be explicitly present in the current request text before later steps use it.",
				"items":       map[string]string{"type": "string"},
			},
			"exact_context_terms": map[string]any{
				"type":        "array",
				"description": "Optional narrow same-scope terms for exact-resolution questions. Leave this unset by default. Use only alongside exact_targets when nearby context should stay within one identifier family / module subtree, and only when you can copy 1-2 narrow identifier/path stems directly from the exact target lane itself. Do not use layer labels, precedence words, or generic context terms here. The system treats these as LLM suggestions and silently drops any item that is not validated against the request-mentioned exact-target lane.",
				"items":       map[string]string{"type": "string"},
			},
			"exact_context_roles": map[string]any{
				"type":        "array",
				"description": "Optional abstract nearby-context roles the user explicitly asked to see for an exact-target precedence / lineage answer. Use only alongside exact_targets when the request names conceptual layers such as code defaults, repo/user config files, runtime state, or override channels. Use enum values default / config / runtime / override; omit when unsure.",
				"items": map[string]any{
					"type": "string",
					"enum": skill.AnalysisExactContextRoleValues(),
				},
			},
			"answer_subject": map[string]any{
				"type":        "object",
				"description": "Optional. Classifies what kind of source-code literal the answer should be (skill_name, agent_name, config_key, ...). The chain ranker uses this to demote chains whose terminal token is the wrong kind. Leave unset when the answer kind is ambiguous; an automatic fallback infers from question_kind.",
				"properties": map[string]any{
					"kind":        stringProp{Type: "string", Enum: skill.AnalysisAnswerSubjectValues()},
					"entity_axes": arrayProp{Type: "array", Items: map[string]string{"type": "string"}},
					"confidence":  map[string]string{"type": "number"},
				},
			},
			"intent_confidence":     map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0, "description": "Your certainty about the intent classification, in [0, 1]. 0.9+ = unambiguous; 0.5-0.7 = plausible alternative exists; < 0.5 = guessing."},
			"complexity_confidence": map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0, "description": "Your certainty about the complexity classification, in [0, 1]."},
			"kind_confidence":       map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0, "description": "Your certainty about the question_kind classification, in [0, 1]."},
			"predicates": map[string]any{
				"type":        "object",
				"description": "Cross-language semantic self-assessment of the question. Every field is required; emit true OR false explicitly. A missing field is fail-loud rejection, not a silent default.",
				"properties": map[string]any{
					"is_scalar_answer":        map[string]any{"type": "boolean", "description": "True if the answer is a single scalar (a number, a literal, a path) rather than a set or sequence."},
					"is_role_locate_lookup":   map[string]any{"type": "boolean", "description": "True only for scalar lookups where the request names a clue / output / context entity, but the answer is one DIFFERENT literal that plays a role relative to that clue (entry function, defining file, config key, route, owner symbol, etc.). For set-valued tables such as every package -> entry function, set this false and use category/relational enumeration predicates. Implies is_scalar_answer=true and requires answer_subject.kind."},
					"is_count_question":       map[string]any{"type": "boolean", "description": "True when the answer is a single number that must be computed by aggregating values across multiple source units — e.g. counting items, summing lines of code, summing file sizes, totalling bytes across a directory tree. Implies is_scalar_answer. Set false when the user asks to list/enumerate members and include counts per list/group; those counts are attributes of an enumeration, not a scalar count question. Also set false when the answer is a number that already exists as a single source-code literal (a const declaration, a default value, an enum ordinal) — that case is is_scalar_answer=true without is_count_question."},
					"is_cross_component":      map[string]any{"type": "boolean", "description": "True if the question genuinely spans multiple distinct components / subsystems / independently-answerable code regions. Leave false for a single named target that merely needs nearby context, precedence layers, or override stages, and also leave false for one ordered source-to-sink call/flow trace even when that chain crosses files or packages."},
					"is_relational_lookup":    map[string]any{"type": "boolean", "description": "True if filtering set X by a relationship to Y ('functions that return Z', 'agents that use skill Y')."},
					"is_category_enumeration": map[string]any{"type": "boolean", "description": "True if asking 'what kinds / types / categories of X exist'."},
					"is_history_lookup":       map[string]any{"type": "boolean", "description": "True when the authoritative evidence source is repository history / authorship metadata (git log / blame / commit history), not a repo file:line. This is an evidence-source flag, not an answer-shape flag: pair it with is_scalar_answer=true only when the principal answer is one literal such as a commit hash/date/author/count; leave is_scalar_answer=false for feature summaries, recent-commit lists, commit comparisons, locating the changed code, explaining the code behind a commit, drawing logic/sequence diagrams from a commit, or history-backed diagnostics."},
					"is_diagnostic_question":  map[string]any{"type": "boolean", "description": "True when the current request asks to diagnose a failure, regression, runtime symptom, observed bad behaviour, or whether a similar problem still exists, and expects cause / current-risk / remediation analysis. Applies with or without an attached runtime artifact. False for ordinary architecture tours, code walkthroughs, or log/trace parser mechanism questions. This is the primary diagnostic routing predicate; the system aligns diagnostic_profile.is_diagnostic to it unless independent current-risk / historical-regression signals are present."},
				},
				"required": []string{"is_scalar_answer", "is_role_locate_lookup", "is_count_question", "is_cross_component", "is_relational_lookup", "is_category_enumeration", "is_history_lookup", "is_diagnostic_question"},
			},
			"diagnostic_profile": map[string]any{
				"type":        "object",
				"description": "Required typed diagnostic-intent profile. Use this as a second safety lane for diagnosis, current-risk, historical-regression, and current-version verification questions. Every boolean field should be emitted true OR false explicitly. Do not infer from raw artifact presence alone. The system tolerates mirror drift by aligning is_diagnostic to predicates.is_diagnostic_question unless current_risk or historical_regression is true; if this mirror profile is accidentally omitted, runtime defaults it conservatively from predicates without inventing current-risk/regression/current-version flags.",
				"properties": map[string]any{
					"is_diagnostic":         map[string]any{"type": "boolean", "description": "Profile-level mirror of predicates.is_diagnostic_question. Set both consistently; the system can auto-align this mirror when it drifts."},
					"current_risk":          map[string]any{"type": "boolean", "description": "True when the current request asks whether a known or observed issue can still happen in the current checkout."},
					"historical_regression": map[string]any{"type": "boolean", "description": "True when the request compares a historical observed symptom against the current version."},
					"current_version_check": map[string]any{"type": "boolean", "description": "True only for diagnostic current-status questions where the answer must verify current code separately from historical artifact observations; false for ordinary exact/config/value/location lookups."},
					"observation_summary":   map[string]any{"type": "string", "description": "Optional compact summary of the historical or user-described symptom. Fill this when no log/trace is attached or when the request uses prior conversation to refer to the issue being checked."},
					"confidence":            map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0, "description": "Your confidence in this diagnostic profile in [0,1]."},
				},
				"required": []string{"is_diagnostic", "current_risk", "historical_regression", "current_version_check", "confidence"},
			},
			"conversation_reference_profile": map[string]any{
				"type":        "object",
				"description": "Optional typed resolution for current requests that rely on Prior Conversation. Use this for ordinary follow-up questions whose concrete subject is not verbatim in the current request. Do not use it for attached log/trace observations; those use diagnostic/artifact profiles.",
				"properties": map[string]any{
					"requires_prior_context":  map[string]any{"type": "boolean", "description": "True when the current request cannot be resolved without Prior Conversation."},
					"needs_repo_verification": map[string]any{"type": "boolean", "description": "True when answering still requires reading repository files after resolving the prior subject."},
					"ambiguity":               map[string]any{"type": "string", "enum": conversationReferenceAmbiguityValues(), "description": "none when the prior subject is resolved uniquely; ambiguous when multiple prior subjects fit; missing when no prior subject is available."},
					"resolved_subjects": map[string]any{
						"type":        "array",
						"description": "Concrete subjects resolved from Prior Conversation or mixed current/prior context.",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"surface":             map[string]any{"type": "string", "description": "Concrete identifier, path, config key, route, or symbol surface copied from Prior Conversation or current+prior context. Current-request-only subjects belong in entities / exact_targets, not here."},
								"kind":                map[string]any{"type": "string", "enum": skill.AnalysisAnswerSubjectValues(), "description": "What kind of target this resolved subject is."},
								"source":              map[string]any{"type": "string", "enum": conversationReferenceSourceValues(), "description": "Where this resolved surface came from."},
								"role":                map[string]any{"type": "string", "description": "Short role label such as primary_subject, exact_answer_target, context, comparator."},
								"use_as_exact_target": map[string]any{"type": "boolean", "description": "True only when this prior-derived subject is the exact target the answer must resolve; false for contextual hints."},
								"confidence":          map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0, "description": "Your confidence in this specific subject resolution."},
							},
							"required": []string{"surface", "source", "use_as_exact_target", "confidence"},
						},
					},
				},
				"required": []string{"requires_prior_context", "needs_repo_verification", "ambiguity"},
			},
			"source_scope_profile": map[string]any{
				"type":        "object",
				"description": "Optional typed path-scope intent. Emit when tests, docs, fixtures, examples, or all repo material are principal answer scope; otherwise production is the default for code-behavior questions. This is user intent, not path classification.",
				"properties": map[string]any{
					"requested_scope":                map[string]any{"type": "string", "enum": sourceScopeValues(), "description": "production, test, documentation, auxiliary, all, or unknown."},
					"include_auxiliary_as_principal": map[string]any{"type": "boolean", "description": "True only when test/docs/fixture/example files may be principal answer members rather than supporting context."},
					"confidence":                     map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0, "description": "Your confidence in this source scope in [0,1]."},
					"rationale":                      map[string]any{"type": "string", "description": "Short audit rationale for the selected scope."},
				},
				"required": []string{"requested_scope", "confidence"},
			},
			"answer_visibility_profile": map[string]any{
				"type":        "object",
				"description": "Typed symbol-visibility intent for code-symbol inventory/list/count/API-surface questions. Emit this whenever the current request expresses a public/exported/all/private/internal visibility scope; omit it only when no symbol visibility scope is expressed. Use public_exported when the current request asks for public/exported symbols or API surface only; use all when private/internal members are explicitly included; use private_only when only private/internal symbols are requested. This controls graph Exported filtering downstream and is separate from source_scope_profile path filtering.",
				"properties": map[string]any{
					"symbol_visibility": map[string]any{"type": "string", "enum": answerSymbolVisibilityValues(), "description": "public_exported, all, private_only, or unknown."},
					"source_quotes":     map[string]any{"type": "array", "items": map[string]string{"type": "string"}, "description": "Verbatim current-request phrase(s) that state the symbol visibility scope."},
					"confidence":        map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0, "description": "Your confidence in this symbol visibility scope in [0,1]."},
					"rationale":         map[string]any{"type": "string", "description": "Short audit rationale for why this visibility scope applies."},
				},
				"required": []string{"symbol_visibility", "confidence"},
			},
			"change_impact_profile": map[string]any{
				"type":        "object",
				"description": "Optional typed profile for migration / affected-site questions. Use only when the CURRENT request asks which code sites, files, symbols, APIs, config locations, or downstream artifacts would need changes if a named target changed. Do not use for ordinary mechanism explanations, current-status diagnostics, or simple value lookups.",
				"properties": map[string]any{
					"is_change_impact": map[string]any{"type": "boolean", "description": "True when the answer set is affected sites/files/symbols under a target change; false or omit for ordinary enumeration."},
					"target":           map[string]any{"type": "string", "description": "The changed target: symbol, field, type, API, config key, file path, module, or package surface."},
					"target_kind":      map[string]any{"type": "string", "enum": skill.AnalysisAnswerSubjectValues(), "description": "What kind of target is being changed, when known."},
					"scope":            map[string]any{"type": "string", "enum": impactScopeValues(), "description": "Requested affected-code boundary: production, test, all, or unknown."},
					"requested_output": map[string]any{"type": "string", "enum": impactRequestedOutputValues(), "description": "Principal member surface the user asked for: files, sites, symbols, steps, or unknown."},
					"affected_site_kinds": map[string]any{
						"type":        "array",
						"description": "Structural site roles expected to be principal affected members. Pick language-neutral roles; include multiple roles when the change affects writers and readers/validators/builders.",
						"items": map[string]any{
							"type": "string",
							"enum": impactAffectedSiteKindValues(),
						},
					},
					"confidence": map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0, "description": "Your confidence in this impact profile in [0,1]."},
					"rationale":  map[string]any{"type": "string", "description": "Short audit rationale for why this is or is not a change-impact question."},
				},
				"required": []string{"is_change_impact", "confidence"},
			},
			"field_value_profile": map[string]any{
				"type":        "object",
				"description": "Optional typed profile for exact field/member/config literal lookups. Use when the CURRENT request asks how many sites, which sites, or what scalar answer depends on a named target being set/equal to a specific literal value. Do not emit for unrelated counts or ordinary mechanism explanations.",
				"properties": map[string]any{
					"is_field_value_lookup": map[string]any{"type": "boolean", "description": "True only when the current request explicitly binds a named target to an exact literal value."},
					"target":                map[string]any{"type": "string", "description": "The exact field/member/config surface, such as CitationReq.Required, server.port, Namespace::Option, or Owner.member."},
					"literal":               map[string]any{"type": "string", "description": "The exact requested literal value, copied without paraphrase, such as false, nil, 30, or \"debug\"."},
					"literal_kind":          map[string]any{"type": "string", "enum": fieldValueLiteralKindValues(), "description": "Kind of literal value."},
					"source_quote":          map[string]any{"type": "string", "description": "Verbatim phrase from the current request that contains both target and literal, for provenance validation."},
					"confidence":            map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0, "description": "Your confidence in this exact target/literal binding in [0,1]."},
					"rationale":             map[string]any{"type": "string", "description": "Short audit rationale for why this is a field/member literal lookup."},
				},
				"required": []string{"is_field_value_lookup", "confidence"},
			},
			"answer_exclusion_policy": map[string]any{
				"type":        "object",
				"description": "Optional typed profile for current-request candidate categories the user explicitly excludes from the principal answer, such as variables, tests, generated files, private helpers, or private symbols excluded by an explicit public/exported-only request. Explicit Chinese exclusions such as `不要列变量`, `不包含测试`, or `排除生成文件` should use roles variable/test/generated with the exact phrase in source_quotes. Do not infer categories from keywords; emit only when the request states the exclusion or public/exported-only export scope.",
				"properties": map[string]any{
					"is_exclusion_requested":   map[string]any{"type": "boolean", "description": "True only when the current request explicitly excludes one or more candidate roles from the answer."},
					"excluded_candidate_roles": map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": answerCandidateRoleValues()}, "description": "Candidate roles excluded from principal answer rows."},
					"source_quotes":            map[string]any{"type": "array", "items": map[string]string{"type": "string"}, "description": "Verbatim current-request phrase(s) that state the exclusion."},
					"confidence":               map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0, "description": "Your confidence in this exclusion profile in [0,1]."},
					"rationale":                map[string]any{"type": "string", "description": "Short audit rationale for why these candidate roles are excluded."},
				},
				"required": []string{"is_exclusion_requested", "confidence"},
			},
			"answer_role_profile": map[string]any{
				"type":        "object",
				"description": "Required typed profile for positive role-binding constraints on principal answer rows. Set is_role_binding_requested=false when the current request has no positive answer role binding.",
				"properties": map[string]any{
					"is_role_binding_requested": map[string]any{"type": "boolean", "description": "True only when the current request explicitly asks for one or more answer roles to be returned as the principal answer."},
					"required_candidate_roles":  map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": answerCandidateRoleValues()}, "description": "Positive principal answer role(s) required by the current request."},
					"source_quotes":             map[string]any{"type": "array", "items": map[string]string{"type": "string"}, "description": "Verbatim current-request phrase(s) that state the positive role binding."},
					"confidence":                map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0, "description": "Your confidence in this positive answer-role profile in [0,1]."},
					"rationale":                 map[string]any{"type": "string", "description": "Short audit rationale for why these candidate roles are required."},
				},
				"required": []string{"is_role_binding_requested", "confidence"},
			},
			"error_granularity_profile": map[string]any{
				"type":        "object",
				"description": "Required typed profile for requests about failure scope across an item, record, call, batch, or transaction. Set is_granularity_question=false when no canonical failure-scope verdict is needed.",
				"properties": map[string]any{
					"is_granularity_question":   map[string]any{"type": "boolean", "description": "True only when the current request asks how failures are scoped, such as per-item rejection, whole-batch failure, partial success, fail-fast stop, or collected errors."},
					"requested_verdict_options": map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": errorGranularityRequestedOptionValues()}, "description": "Optional enum options explicitly contrasted by the current request. This is not the answer verdict; it constrains the final decision to one of the request's alternatives when evidence supports one."},
					"source_quotes":             map[string]any{"type": "array", "items": map[string]string{"type": "string"}, "description": "Current-request phrase(s) that state the failure-scope question. Exact verbatim is preferred; the system performs deterministic normalization and may ignore unanchored optional quotes instead of forcing a retry."},
					"confidence":                map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0, "description": "Your confidence in this failure-scope profile in [0,1]."},
					"rationale":                 map[string]any{"type": "string", "description": "Short audit rationale for why a canonical failure-scope verdict is or is not required."},
				},
				"required": []string{"is_granularity_question", "confidence"},
			},
			"predicate_axis": map[string]any{
				"type":        "string",
				"enum":        skill.AnalysisPredicateAxisValues(),
				"description": "Action-direction axis of the question (call / register / define / return / configure / condition / implement). Empty when no clear verb cue. Used by the evidence ranker to bias items whose anchor matches the axis.",
			},
			"diagram_hint": map[string]any{
				"type":        "object",
				"description": "Optional. Emit only when the CURRENT request explicitly asks for a diagram / visual / drawing, or when the answer would be structurally incomplete without a visual. Omit for ordinary call-chain, architecture, log, or trace questions where prose/list/table blocks answer the user directly.",
				"properties": map[string]any{
					"kind": stringProp{Type: "string", Enum: skill.AnalysisDiagramKindValues()},
				},
				"required": []string{"kind"},
			},
			"enumeration_boundary": map[string]any{
				"type":        "object",
				"description": "Optional. Use only when the user explicitly declares a bounded principal set with a numeric count such as 'the 7 checks', 'the first 3 handlers', or 'top 5 stages'. Copy the evidence-bearing phrase verbatim from the current request into source_quote and set declared_count to that same user-declared number. Do not emit this for scalar count answers where the number is only a search/window/scope constraint, such as 'how many of the last 20 commits ...'. Do not infer a count for all/every/complete questions that do not name a number; use completeness_obligation instead.",
				"properties": map[string]any{
					"declared_count": map[string]any{"type": "integer", "minimum": 1},
					"source_quote":   map[string]string{"type": "string"},
				},
				"required": []string{"declared_count", "source_quote"},
			},
			"completeness_obligation": map[string]any{
				"type":        "object",
				"description": "Optional. Use when the user explicitly demands an EXHAUSTIVE answer — phrases that signal universal coverage of the answer set (e.g. quantifiers like 'all'/'every'/'all the' in any language, or explicit completeness markers like 'complete list'/'exhaustive'/'full inventory'). Decision rule: if the question would NOT be considered fully answered while one valid item is still missing, set required=true and copy the verbatim trigger phrase into source_quote. Distinct from enumeration_boundary which carries a count: completeness demands every item without naming a number. Leave omitted when the user is satisfied with a representative subset, a top-N, or any partial answer.",
				"properties": map[string]any{
					"required":     map[string]any{"type": "boolean"},
					"source_quote": map[string]any{"type": "string", "description": "Verbatim phrase from the current request that signals the completeness demand."},
				},
				"required": []string{"required", "source_quote"},
			},
			"buckets": map[string]any{
				"type":        "array",
				"description": "Optional. Emit when the user EXPLICITLY partitions the answer into named groups — phrases that pair multiple labels with parallel asks (e.g. 'X for A, Y for B' / 'A 和 B 分别...' / 'list ... separately for A and B' / 'compare A vs B'). Each bucket's label MUST be verbatim from the question; your anchor entities go in anchors[]. Decision rule: if the answer would naturally split into N sections each titled by a user-named label, emit one bucket per label in the order they appear. Leave omitted for single-topic questions or for multi-topic questions where the user did NOT name the partitions.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"label":   map[string]any{"type": "string", "description": "Verbatim label copied from the user's question (e.g. when the question pairs two named modes / phases / sides, copy each name as the bucket label)."},
						"anchors": map[string]any{"type": "array", "items": map[string]string{"type": "string"}, "description": "Entities you resolved as members of this bucket."},
						"index":   map[string]any{"type": "integer", "minimum": 1, "description": "1-based ordinal in the order labels appear in the question."},
					},
					"required": []string{"label"},
				},
			},
			"required_files": map[string]any{
				"type":        "array",
				"description": "Optional. When you can identify specific files structurally needed to answer the user's question, list them here with confidence and a short rationale. Confidence ≥ 0.8 means the file is treated as a primary file AND its content is pre-read into the prompt; 0.5 ≤ conf < 0.8 means the file is a soft hint (pre-read eligible only); below 0.5 the entry is discarded — leave the recommendation to the deterministic resolver. Use repo-relative POSIX paths copied verbatim from the prescan results. Empty list is fine: omit when you do not have file-level conviction.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path":       map[string]any{"type": "string", "description": "Repo-relative POSIX path copied from the prescan results (e.g. 'internal/analysis/gate/gate.go')."},
						"confidence": map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0, "description": "Recommendation strength in [0,1]. Threshold bands: ≥0.8 primary + pre-read; 0.5–0.79 pre-read only; <0.5 discarded."},
						"rationale":  map[string]any{"type": "string", "description": "Short reason for the recommendation. Why this file is structurally needed for the answer (e.g. 'directly implements the entry function the question asks about')."},
					},
					"required": []string{"path", "confidence"},
				},
			},
			"irrelevant_files": map[string]any{
				"type":        "array",
				"description": "Optional. Negative-channel counterpart of required_files. When you have READ a candidate file in pre-scan and judged it OFF-TOPIC for the user's question, list its repo-relative POSIX path here. The file will NOT be re-injected via pre-read content, mid-loop reading suggestions, or primary-file selection — saves prompt tokens and prevents the system from contradicting your judgment on later iterations. Use sparingly: at most 10 paths, only files you actually inspected. Empty list is fine when no candidates need explicit exclusion.",
				"items":       map[string]any{"type": "string"},
				"maxItems":    10,
			},
		},
		"required": []string{
			"intent", "scenario", "complexity", "keywords", "entities", "question_kind",
			"intent_confidence", "complexity_confidence", "kind_confidence",
			"predicates", "diagnostic_profile", "answer_role_profile", "error_granularity_profile",
		},
	}

	raw, err := json.Marshal(schema)
	if err != nil {
		// emit_analysis schema is built from static SSOT data; a
		// marshal failure here is a programmer error, not a runtime
		// input problem. Fall back to the minimal well-formed schema
		// so the tool still registers and the caller sees a clear
		// compile-time-style error on first use.
		emitAnalysisSchemaCache = json.RawMessage(fmt.Sprintf(
			`{"type":"object","description":"emit_analysis schema build failed: %s"}`, err))
		return
	}
	emitAnalysisSchemaCache = raw
}

func conversationReferenceSourceValues() []string {
	values := types.AllConversationReferenceSources()
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, string(v))
	}
	return out
}

func conversationReferenceAmbiguityValues() []string {
	values := types.AllConversationReferenceAmbiguities()
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, string(v))
	}
	return out
}

func impactScopeValues() []string {
	values := types.AllImpactScopes()
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, string(v))
	}
	return out
}

func sourceScopeValues() []string {
	values := types.AllSourceScopes()
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, string(v))
	}
	return out
}

func answerSymbolVisibilityValues() []string {
	values := types.AllAnswerSymbolVisibilities()
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, string(v))
	}
	return out
}

func impactRequestedOutputValues() []string {
	values := types.AllImpactRequestedOutputs()
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, string(v))
	}
	return out
}

func impactAffectedSiteKindValues() []string {
	values := types.AllImpactAffectedSiteKinds()
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, string(v))
	}
	return out
}

func fieldValueLiteralKindValues() []string {
	values := types.AllFieldValueLiteralKinds()
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, string(v))
	}
	return out
}

func answerCandidateRoleValues() []string {
	values := types.AllAnswerCandidateRoles()
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, string(v))
	}
	return out
}

func errorGranularityVerdictValues() []string {
	values := types.AllErrorGranularityVerdicts()
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, string(v))
	}
	return out
}

func errorGranularityRequestedOptionValues() []string {
	values := types.AllErrorGranularityVerdicts()
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v == types.ErrorGranularityNotEnoughEvidence {
			continue
		}
		out = append(out, string(v))
	}
	return out
}

// Execute is the runtime quality gate. The JSON schema caught the
// structural problems (wrong type, unknown enum, missing required
// field); this method handles everything the schema cannot express:
//
//  1. Mutable gating — sub-agent contexts that never wire Mutable
//     are rejected up front.
//  2. JSON unmarshal — structural shape.
//  3. Normalization — LLM-phrased enum variants ("root-cause") are
//     coerced to the canonical typed values.
//  4. validateAnalysisInput — keyword floor + generic-entity filter
//     driven by the runtime AnalysisLimits policy.
//  5. Persist — the normalized RequestModel is written to
//     Mutable.RequestModel.
//  6. Summary — reports the POST-NORMALIZATION state plus any
//     warnings or normalization deltas so a trace reader can
//     diff the LLM's raw output against the system's interpretation
//     in one line.
//
// Rejection paths keep the error channel machine-readable: Success=
// false with a short reason, err returned when the failure is a
// caller contract violation (bad JSON) rather than a policy breach.
func (t *EmitAnalysis) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	if ctx == nil || ctx.Mutable == nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis requires a writable context; the caller did not provide one (sub-agents are not supported)",
			Timestamp: time.Now(),
		}, nil
	}

	if repaired, fields, ok := repairStringWrappedArrayFields(params); ok {
		logging.Warning("[emit_analysis] top-level arrays arrived as JSON-encoded strings (fields: %s); re-parsed via flat-mode tolerance path",
			strings.Join(fields, ", "))
		params = repaired
	}
	if repaired, fields, ok := repairStringWrappedObjectFields(params,
		"answer_subject",
		"predicates",
		"diagnostic_profile",
		"conversation_reference_profile",
		"source_scope_profile",
		"change_impact_profile",
		"field_value_profile",
		"answer_exclusion_policy",
		"answer_role_profile",
		"error_granularity_profile",
		"diagram_hint",
		"enumeration_boundary",
		"completeness_obligation",
	); ok {
		logging.Warning("[emit_analysis] top-level objects arrived as JSON-encoded strings (fields: %s); re-parsed via flat-mode tolerance path",
			strings.Join(fields, ", "))
		params = repaired
	}
	if repaired, report := toolparam.Normalize(params, t.Parameters(), types.DefaultToolParamCompatConfig()); report.Changed() {
		logging.Warning("[emit_analysis] params schema-normalized at tool boundary: %s", report.Summary(8))
		params = repaired
	}

	var p emitAnalysisParams
	if err := json.Unmarshal(params, &p); err != nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("invalid params: %v", err),
			Timestamp: time.Now(),
		}, err
	}

	// Normalize enum fields first so validateAnalysisInput and the
	// Summary operate on a single canonical view of the classification.
	intent := normalizeIntent(p.Intent)
	scenario := normalizeScenario(p.Scenario)
	complexity := normalizeComplexity(p.Complexity)
	kind := normalizeQuestionKind(p.QuestionKind)

	keywords := trimStringSlice(p.Keywords)
	entities := trimStringSlice(p.Entities)

	// Runtime validation — keyword floor + entity blocklist + quality
	// probe. Config lives in AnalysisLimits (see analysis_limits.go).
	// Two new inputs vs the pre-refactor signature:
	//
	//   - seenBlob: lowercased concatenation of every pre-scan
	//     ToolResult.Summary the analyzer saw this dispatch, read
	//     from Mutable.PrescanSummaryBlob() which
	//     analyzerEvaluator.Observe populates at runtime. Feeds the
	//     verified-entity whitelist (so `Agent` / `Handler` are
	//     KEPT when the pre-scan saw them in the target repo) and
	//     the hit-ratio quality probe.
	//   - prescanRounds: derived from the number of newline-separated
	//     entries in seenBlob. The validator threads this into the
	//     returned Probe so downstream consumers (analyzer.ParseOutput,
	//     eval harnesses) see rounds + hits in one struct without
	//     pulling from two sources.
	//
	// The validator returns the filtered entity slice so a dropped
	// generic noun never reaches the persisted RequestModel.
	limits := CurrentAnalysisLimits()
	seenBlob := ctx.Mutable.PrescanSummaryBlob()
	prescanRounds := countPrescanRounds(seenBlob)
	val := validateAnalysisInput(keywords, entities, limits, seenBlob, prescanRounds)
	if val.BlocklistShadowSummary != "" {
		logging.Info("[emit_analysis] %s", val.BlocklistShadowSummary)
	}
	if val.RejectReason != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: " + val.RejectReason,
			Timestamp: time.Now(),
		}, nil
	}
	entities = val.FilteredEntities
	if reason := rejectDegenerateClassification(intent, kind, keywords, entities); reason != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: " + reason,
			Timestamp: time.Now(),
		}, nil
	}
	// Schema v4 fail-loud: predicates is the cross-language replacement
	// for prose-cue tables. Every field must be present and explicit;
	// the analyzer LLM cannot silently default to "no" and have the
	// downstream dispatch silently degrade. Parse + validate before
	// building the RequestModel.
	predicates, predErr := parsePredicates(p.Predicates)
	if predErr != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: " + predErr,
			Timestamp: time.Now(),
		}, nil
	}
	if reconciled, reason := reconcileSetValuedRoleLocatePredicates(intent, predicates, p.SubTopics); reason != "" {
		predicates = reconciled
		val.Warnings = append(val.Warnings, reason)
	}
	if reconciled, reason := reconcileSetValuedCountPredicates(intent, predicates); reason != "" {
		predicates = reconciled
		val.Warnings = append(val.Warnings, reason)
	}
	diagnosticProfile, diagnosticErr, diagnosticWarnings := parseDiagnosticProfile(p.DiagnosticProfile, predicates)
	if diagnosticErr != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: " + diagnosticErr,
			Timestamp: time.Now(),
		}, nil
	}
	for _, warning := range diagnosticWarnings {
		logging.Warning("[emit_analysis] %s", warning)
		val.Warnings = append(val.Warnings, warning)
	}
	var mirrorWarnings []string
	predicates, diagnosticProfile, mirrorWarnings = normalizeDiagnosticMirrorSignals(predicates, diagnosticProfile)
	for _, warning := range mirrorWarnings {
		logging.Warning("[emit_analysis] %s", warning)
		val.Warnings = append(val.Warnings, warning)
	}
	var diagnosticRouteWarnings []string
	intent, scenario, diagnosticRouteWarnings = normalizeDiagnosticRoute(intent, scenario, predicates)
	for _, warning := range diagnosticRouteWarnings {
		logging.Warning("[emit_analysis] %s", warning)
		val.Warnings = append(val.Warnings, warning)
	}
	conversationReferenceProfile, conversationReferenceErr := parseConversationReferenceProfile(p.ConversationReferenceProfile)
	if conversationReferenceErr != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: " + conversationReferenceErr,
			Timestamp: time.Now(),
		}, nil
	}
	sourceScopeProfile, sourceScopeErr := parseSourceScopeProfile(p.SourceScopeProfile)
	if sourceScopeErr != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: " + sourceScopeErr,
			Timestamp: time.Now(),
		}, nil
	}
	changeImpactProfile, changeImpactErr := parseChangeImpactProfile(p.ChangeImpactProfile)
	if changeImpactErr != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: " + changeImpactErr,
			Timestamp: time.Now(),
		}, nil
	}
	if reason := validateConfidenceRange(p.IntentConfidence, p.ComplexityConfidence, p.KindConfidence); reason != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: " + reason,
			Timestamp: time.Now(),
		}, nil
	}
	axis, axisErr := parsePredicateAxis(p.PredicateAxis)
	if axisErr != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: " + axisErr,
			Timestamp: time.Now(),
		}, nil
	}
	diagramHint, diagramHintErr := parseDiagramHint(p.DiagramHint)
	if diagramHintErr != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: " + diagramHintErr,
			Timestamp: time.Now(),
		}, nil
	}
	// Raw objective — the analyzer gets it from Mutable seeded by
	// the REPL/orchestrator before dispatch. Normalizer builds the
	// TermGraph from this in analyzer.ParseOutput, so we strip the
	// REPL conversation prefix first — otherwise every CamelCase /
	// file-path token from the prior-turn memory block leaks into
	// the TermGraph and downstream TaskNode.SearchHints, polluting
	// the explorer's retry-directive hints with tokens unrelated to
	// the current question. In single-shot mode the strip is a
	// no-op (no marker present).
	raw := types.StripConversationPrefix(ctx.Mutable.Objective())
	if types.IsREPLControlInput(raw) {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("emit_analysis rejected: current user request %q looks like a local REPL control command, not a code question. Do NOT mine entities from Prior Conversation for control inputs.", raw),
			Timestamp: time.Now(),
		}, nil
	}
	answerExclusionPolicy, answerExclusionErr := parseAnswerExclusionPolicy(raw, p.AnswerExclusionPolicy)
	if answerExclusionErr != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: " + answerExclusionErr,
			Timestamp: time.Now(),
		}, nil
	}
	answerVisibilityProfile, answerVisibilityErr, answerVisibilityWarnings := parseAnswerVisibilityProfile(raw, p.AnswerVisibilityProfile)
	if answerVisibilityErr != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: " + answerVisibilityErr,
			Timestamp: time.Now(),
		}, nil
	}
	for _, warning := range answerVisibilityWarnings {
		logging.Warning("[emit_analysis] %s", warning)
		val.Warnings = append(val.Warnings, warning)
	}
	answerRoleProfile, answerRoleErr := parseAnswerRoleProfile(raw, p.AnswerRoleProfile)
	if answerRoleErr != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: " + answerRoleErr,
			Timestamp: time.Now(),
		}, nil
	}
	errorGranularityProfile, errorGranularityErr, errorGranularityWarnings := parseErrorGranularityProfile(raw, p.ErrorGranularityProfile)
	if errorGranularityErr != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: " + errorGranularityErr,
			Timestamp: time.Now(),
		}, nil
	}
	for _, warning := range errorGranularityWarnings {
		logging.Warning("[emit_analysis] %s", warning)
		val.Warnings = append(val.Warnings, warning)
	}
	fieldValueProfile, fieldValueErr := parseFieldValueProfile(raw, p.FieldValueProfile)
	if fieldValueErr != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: " + fieldValueErr,
			Timestamp: time.Now(),
		}, nil
	}
	var enumerationBoundary *types.RequestedEnumerationBoundary
	if p.EnumerationBoundary != nil &&
		types.ErrorGranularityCountsAreContextual(intent, predicates, errorGranularityProfile) {
		val.Warnings = append(val.Warnings, "ignored enumeration_boundary because error_granularity_profile makes count-like phrases contextual")
	} else if p.EnumerationBoundary != nil && scalarCountBoundaryIsScopeOnly(predicates) {
		val.Warnings = append(val.Warnings, "ignored enumeration_boundary because scalar count answers treat numeric phrases as scope windows, not principal answer-member boundaries")
	} else {
		var enumerationBoundaryErr, enumerationBoundaryWarn string
		enumerationBoundary, enumerationBoundaryErr, enumerationBoundaryWarn = parseEnumerationBoundary(raw, p.EnumerationBoundary)
		if enumerationBoundaryErr != "" {
			return types.ToolResult{
				ToolName:  t.Name(),
				Success:   false,
				Summary:   "emit_analysis rejected: " + enumerationBoundaryErr,
				Timestamp: time.Now(),
			}, nil
		}
		if enumerationBoundaryWarn != "" {
			val.Warnings = append(val.Warnings, enumerationBoundaryWarn)
		}
	}
	// Plan E (2026-05-02) — completeness + buckets parsing.
	// Completeness remains load-bearing when required=true. Buckets are
	// an optional comparison partition: malformed bucket labels are
	// stripped with an audit warning so analyzer retries are reserved
	// for missing primary typed signals.
	completenessObligation, completenessErr := parseCompletenessObligation(raw, p.CompletenessObligation)
	if completenessErr != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: " + completenessErr,
			Timestamp: time.Now(),
		}, nil
	}
	buckets, bucketsErr, bucketsWarn := parseQuestionBuckets(raw, p.Buckets)
	if bucketsErr != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: " + bucketsErr,
			Timestamp: time.Now(),
		}, nil
	}
	if bucketsWarn != "" {
		val.Warnings = append(val.Warnings, bucketsWarn)
	}
	exactTargets, exactTargetErr := validateExactTargets(raw, p.ExactTargets)
	if exactTargetErr != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: " + exactTargetErr,
			Timestamp: time.Now(),
		}, nil
	}
	mentionedEntities := types.MentionedEntitiesFromRawRequest(raw, entities)
	answerSubject := parseAnswerSubject(p.AnswerSubject)
	if normalized, warning := normalizeMissingAnswerSubjectForNonScalarExplain(axis, intent, predicates, entities, p.SubTopics, answerSubject); warning != "" {
		answerSubject = normalized
		val.Warnings = append(val.Warnings, warning)
	}
	// Self-consistency: after typed, deterministic normalizers have
	// absorbed safe drift, reject only contradictions that still need
	// the LLM to reconcile its own classification.
	if issue := validateSelfConsistencyDetailed(intent, scenario, kind, predicates, diagnosticProfile, axis, entities, p.SubTopics, answerSubject); issue.Reason != "" {
		if writeModeAnalysisRootCauseTolerance(ctx, issue.Kind) {
			val.Warnings = append(val.Warnings, "write-mode tolerated read-analyzer root_cause without diagnostic typed signal; write_analyzer will provide the code-change task framing")
		} else {
			return types.ToolResult{
				ToolName:  t.Name(),
				Success:   false,
				Summary:   "emit_analysis rejected: " + issue.Reason,
				Timestamp: time.Now(),
			}, nil
		}
	}
	exactContextTerms, exactContextWarn := sanitizeExactContextTerms(exactTargets, mentionedEntities, p.ExactContextTerms)
	exactContextRoles, exactContextRoleWarn := sanitizeExactContextRoles(
		exactTargets,
		answerSubject.Kind,
		scenario,
		kind,
		p.ExactContextRoles,
	)
	if exactContextWarn != "" {
		val.Warnings = append(val.Warnings, exactContextWarn)
	}
	if exactContextRoleWarn != "" {
		val.Warnings = append(val.Warnings, exactContextRoleWarn)
	}

	// Sub-topic summaries sometimes copy chunks of the REPL
	// conversation prefix verbatim when the LLM is over-eager to
	// "summarise". That pollution then surfaces in the renderer's
	// per-topic row as "## Prior conversation ..." text. Sanitize
	// every summary before it reaches downstream consumers.
	sanitizedSubTopics := sanitizeSubTopics(p.SubTopics)

	rm := types.RequestModel{
		RawRequest: raw,
		Language:   p.Language,
		Intent:     intent,
		Scenario:   scenario,
		Complexity: complexity,
		SubTopics:  sanitizedSubTopics,
		AnalyzerHints: types.AnalyzerHints{
			Keywords:          keywords,
			Entities:          entities,
			PrimaryEntities:   append([]string(nil), entities...),
			MentionedEntities: mentionedEntities,
			ExactTargets:      exactTargets,
			ExactContextTerms: exactContextTerms,
			ExactContextRoles: exactContextRoles,
			RequiredFileHints: validateAndBuildRequiredFileHints(p.RequiredFiles, &val),
			IrrelevantFiles:   validateAndBuildIrrelevantFiles(p.IrrelevantFiles, &val),
			Kind:              kind,
		},
		AnswerSubject:                answerSubject,
		IntentConfidence:             p.IntentConfidence,
		ComplexityConfidence:         p.ComplexityConfidence,
		KindConfidence:               p.KindConfidence,
		Predicates:                   predicates,
		DiagnosticProfile:            diagnosticProfile,
		ConversationReferenceProfile: conversationReferenceProfile,
		SourceScopeProfile:           sourceScopeProfile,
		AnswerVisibilityProfile:      answerVisibilityProfile,
		ChangeImpactProfile:          changeImpactProfile,
		FieldValueProfile:            fieldValueProfile,
		AnswerExclusionPolicy:        answerExclusionPolicy,
		AnswerRoleProfile:            answerRoleProfile,
		ErrorGranularityProfile:      errorGranularityProfile,
		PredicateAxis:                axis,
		DiagramHint:                  diagramHint,
		EnumerationBoundary:          enumerationBoundary,
		CompletenessObligation:       completenessObligation,
		Buckets:                      buckets,
	}
	ctx.Mutable.SetRequestModel(rm)
	recordExactTargetPrescanFindings(ctx, rm, seenBlob)

	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   buildEmitAnalysisSummary(p, rm, val),
		Timestamp: time.Now(),
	}, nil
}

func recordExactTargetPrescanFindings(ctx *types.BusContext, rm types.RequestModel, seenBlob string) {
	if ctx == nil || ctx.Mutable == nil {
		return
	}
	contract := types.BuildExactResolutionContract(rm)
	if contract == nil || len(contract.Targets) == 0 {
		return
	}
	graph, _ := ctx.Mutable.SearchGraph().(*repomap.Graph)
	closure := ctx.Mutable.EvidenceClosure()
	recorded := 0
	for _, candidate := range contract.Targets {
		token := strings.TrimSpace(candidate)
		if token == "" {
			continue
		}
		finding := prescan.ClassifyToken(graph, seenBlob, token, types.ExactResolutionRequiresDefiningPrimaryProof(contract))
		if finding.Status == prescan.TokenStatusPrimary {
			continue
		}
		if finding.Status == prescan.TokenStatusUnresolved && !finding.Observed {
			continue
		}
		reason := "exact target has no current production-defining prescan hit"
		if finding.Status == prescan.TokenStatusAuxiliaryOnly {
			reason = "exact target matched only auxiliary prescan files"
		}
		closure.AppendUnverifiedFinding(types.UnverifiedFinding{
			Token:  token,
			Kind:   exactTargetFindingKind(rm),
			Reason: reason,
		})
		recorded++
	}
	if recorded > 0 {
		closure.BumpUnverifiedFinds(recorded)
	}
}

func exactTargetFindingKind(rm types.RequestModel) string {
	switch rm.AnswerSubject.Kind {
	case types.SubjectFilePath:
		return "path"
	case types.SubjectHandlerRoute:
		return "route"
	default:
		return "symbol"
	}
}

// rejectDegenerateClassification blocks the fully-collapsed
// classification that otherwise passes emit_analysis and only dies
// later in the analyzer quality gate. The check is intentionally
// narrow so genuinely ambiguous but term-bearing requests still flow.
func rejectDegenerateClassification(
	intent types.Intent,
	kind string,
	keywords []string,
	entities []string,
) string {
	if intent != types.IntentUnknown {
		return ""
	}
	if !strings.EqualFold(strings.TrimSpace(kind), "unknown") {
		return ""
	}
	if len(keywords) > 0 || len(entities) > 0 {
		return ""
	}
	return "degenerate classification (intent=unknown, question_kind=unknown, keywords=0, entities=0). Re-read the User Request section only and emit at least one real keyword/entity or choose a concrete question_kind."
}

// validateSelfConsistency cross-checks the LLM's intent / kind /
// predicates / answer_subject and rejects internally-contradictory
// classifications. The downstream pipeline trusts the LLM's predicates
// as the cross-language signal that replaces the deleted prose-cue
// tables; if the LLM emits "is_count_question=true" but also picks
// intent=enumerate, it has answered the question wrong in two places
// at once and the retry hint should force the LLM to reconcile its
// own answer rather than have a Go rule silently override one of them
// (which is what session 6 reconcileIntent used to do via the
// prose-cue table that's now deleted).
//
// Each check fires only on a clear contradiction. We deliberately do
// NOT enforce the full Cartesian product (e.g. is_cross_component
// implies complexity=complex) because a few of those would trip on
// legitimate edge cases; callers should add a check here only when a
// real failure is observed.
func reconcileSetValuedRoleLocatePredicates(
	intent types.Intent,
	preds types.SemanticPredicates,
	subTopics []types.SubTopic,
) (types.SemanticPredicates, string) {
	if !preds.IsRoleLocateLookup || preds.IsScalarAnswer {
		return preds, ""
	}
	if !setValuedRoleLocateShape(intent, preds, subTopics) {
		return preds, ""
	}
	preds.IsRoleLocateLookup = false
	if !preds.IsCategoryEnumeration {
		preds.IsRelationalLookup = true
	}
	return preds, "set-valued role-locate normalized to relational/category enumeration; scalar role-locate is reserved for one located literal"
}

func setValuedRoleLocateShape(intent types.Intent, preds types.SemanticPredicates, subTopics []types.SubTopic) bool {
	if preds.IsCategoryEnumeration || preds.IsRelationalLookup {
		return true
	}
	if intent == types.IntentEnumerate {
		return true
	}
	return len(trimSubTopicsForConsistency(subTopics)) > 1
}

func reconcileSetValuedCountPredicates(intent types.Intent, preds types.SemanticPredicates) (types.SemanticPredicates, string) {
	if intent != types.IntentEnumerate || !preds.IsCountQuestion {
		return preds, ""
	}
	preds.IsCountQuestion = false
	preds.IsScalarAnswer = false
	return preds, "set-valued enumeration normalized count predicate to false; requested counts remain per-list attributes, not a scalar count answer"
}

func trimSubTopicsForConsistency(subTopics []types.SubTopic) []types.SubTopic {
	if len(subTopics) == 0 {
		return nil
	}
	out := make([]types.SubTopic, 0, len(subTopics))
	for _, st := range subTopics {
		if strings.TrimSpace(st.Summary) == "" && len(trimStringSlice(st.Entities)) == 0 {
			continue
		}
		out = append(out, st)
	}
	return out
}

type selfConsistencyIssueKind string

const (
	selfConsistencyIssueNone                       selfConsistencyIssueKind = ""
	selfConsistencyIssueRootCauseMissingDiagnostic selfConsistencyIssueKind = "root_cause_missing_diagnostic"
	selfConsistencyIssueOther                      selfConsistencyIssueKind = "other"
)

type selfConsistencyIssue struct {
	Kind   selfConsistencyIssueKind
	Reason string
}

func validateSelfConsistency(
	intent types.Intent,
	scenario types.Scenario,
	kind string,
	preds types.SemanticPredicates,
	diagnostic types.DiagnosticIntentProfile,
	axis types.PredicateAxis,
	entities []string,
	subTopics []types.SubTopic,
	answerSubject types.AnswerSubject,
) string {
	return validateSelfConsistencyDetailed(intent, scenario, kind, preds, diagnostic, axis, entities, subTopics, answerSubject).Reason
}

func validateSelfConsistencyDetailed(
	intent types.Intent,
	scenario types.Scenario,
	kind string,
	preds types.SemanticPredicates,
	diagnostic types.DiagnosticIntentProfile,
	axis types.PredicateAxis,
	entities []string,
	subTopics []types.SubTopic,
	answerSubject types.AnswerSubject,
) selfConsistencyIssue {
	if preds.IsRoleLocateLookup {
		if !preds.IsScalarAnswer {
			return selfConsistencyIssue{Kind: selfConsistencyIssueOther, Reason: "is_role_locate_lookup=true requires is_scalar_answer=true — a role-locate question still resolves to one literal answer"}
		}
		if !roleLocateSubjectKindAllowed(answerSubject.Kind) {
			return selfConsistencyIssue{Kind: selfConsistencyIssueOther, Reason: "is_role_locate_lookup=true requires answer_subject.kind to name the located literal kind (function_name / type_name / file_path / handler_route / config_key / interface_name / struct_field / enum_value)"}
		}
	}
	// Count question must resolve to a scalar answer, not a list. The
	// normal parse path clears this predicate for intent=enumerate, where
	// counts are per-list attributes. Keep a defensive reject for direct
	// callers that bypass the normalizer.
	if preds.IsCountQuestion {
		if intent == types.IntentEnumerate {
			return selfConsistencyIssue{Kind: selfConsistencyIssueOther, Reason: "is_count_question=true is inconsistent with intent=enumerate — set is_count_question=false and is_scalar_answer=false when the answer is a member list with counts per category"}
		}
	}
	// is_count_question implies is_scalar_answer (the prompt says so).
	// LLM can still get this wrong; reject so it has to fix one or
	// the other.
	if preds.IsCountQuestion && !preds.IsScalarAnswer {
		return selfConsistencyIssue{Kind: selfConsistencyIssueOther, Reason: "is_count_question=true requires is_scalar_answer=true — a count question always yields a single scalar"}
	}
	// Category enumeration ("what kinds of X exist") implies a list
	// answer, not a single scalar. If both predicates are set the
	// question is contradictory.
	if preds.IsCategoryEnumeration && preds.IsScalarAnswer {
		return selfConsistencyIssue{Kind: selfConsistencyIssueOther, Reason: "is_category_enumeration=true and is_scalar_answer=true are mutually exclusive — a 'what kinds of X' question yields a list, not a scalar"}
	}
	if preds.IsHistoryLookup {
		if preds.IsRoleLocateLookup && !preds.IsScalarAnswer {
			return selfConsistencyIssue{Kind: selfConsistencyIssueOther, Reason: "is_role_locate_lookup=true requires is_scalar_answer=true — if the history question asks for one role-bearing literal, keep both predicates scalar; otherwise set is_role_locate_lookup=false and let the history answer use summary/list/comparison shape"}
		}
		if preds.IsScalarAnswer && !preds.IsCountQuestion && intent != types.IntentReturnValue {
			return selfConsistencyIssue{Kind: selfConsistencyIssueOther, Reason: "principal history scalar answers must use intent=return_value — if the user asks to explain, trace corresponding code, compare, list, diagnose, draw diagrams, or summarize repository history, keep is_history_lookup=true but set is_scalar_answer=false and choose the non-scalar intent"}
		}
	}
	diagnosticRequired := preds.IsDiagnosticQuestion || diagnostic.RequiresDiagnosticRootCause()
	if diagnostic.CurrentVersionCheck && !preds.IsDiagnosticQuestion &&
		!diagnostic.IsDiagnostic && !diagnostic.CurrentRisk && !diagnostic.HistoricalRegression {
		return selfConsistencyIssue{Kind: selfConsistencyIssueOther, Reason: "diagnostic_profile.current_version_check=true is only valid for diagnostic current-status questions — pair it with predicates.is_diagnostic_question=true or diagnostic_profile.is_diagnostic/current_risk/historical_regression=true when the user asks whether an observed issue is still present; for ordinary current-code exact/config/value/location lookup, set current_version_check=false"}
	}
	if preds.IsDiagnosticQuestion {
		if intent != types.IntentRootCause {
			return selfConsistencyIssue{Kind: selfConsistencyIssueOther, Reason: "is_diagnostic_question=true requires intent=root_cause — diagnostic questions ask for cause / current-risk analysis, not a general mechanism tour or scalar lookup"}
		}
		switch scenario {
		case types.ScenarioRootCause, types.ScenarioPerformanceBottleneck:
			// ok
		default:
			return selfConsistencyIssue{Kind: selfConsistencyIssueOther, Reason: "is_diagnostic_question=true requires scenario=root_cause or scenario=performance_bottleneck — architecture_explain is for ordinary mechanism tours, not failure diagnosis"}
		}
	}
	if intent == types.IntentRootCause && !diagnosticRequired {
		return selfConsistencyIssue{Kind: selfConsistencyIssueRootCauseMissingDiagnostic, Reason: "intent=root_cause requires a diagnostic typed signal — set predicates.is_diagnostic_question=true or diagnostic_profile.is_diagnostic/current_risk/historical_regression=true"}
	}
	if needsRoleLocateDisambiguation(axis, intent, preds, entities, subTopics) &&
		answerSubject.Kind == types.SubjectUnknown {
		return selfConsistencyIssue{Kind: selfConsistencyIssueOther, Reason: "single-target define-axis lookup is under-specified: set answer_subject.kind explicitly so the system can tell whether this is a role-locate scalar lookup (function / type / file / route / config key) or an explanation of the named entity itself; also set predicates.is_role_locate_lookup to true or false explicitly"}
	}
	return selfConsistencyIssue{}
}

func writeModeAnalysisRootCauseTolerance(ctx *types.BusContext, kind selfConsistencyIssueKind) bool {
	if kind != selfConsistencyIssueRootCauseMissingDiagnostic {
		return false
	}
	if ctx == nil {
		return false
	}
	return ctx.Mode.Normalize().IsWrite()
}

func roleLocateSubjectKindAllowed(kind types.AnswerSubjectKind) bool {
	switch kind {
	case types.SubjectFunctionName,
		types.SubjectTypeName,
		types.SubjectInterface,
		types.SubjectHandlerRoute,
		types.SubjectConfigKey,
		types.SubjectFilePath,
		types.SubjectStructField,
		types.SubjectEnumValue,
		types.SubjectStringLiteral:
		return true
	}
	return false
}

func needsRoleLocateDisambiguation(
	axis types.PredicateAxis,
	intent types.Intent,
	preds types.SemanticPredicates,
	entities []string,
	subTopics []types.SubTopic,
) bool {
	if axis != types.AxisDefine {
		return false
	}
	if len(subTopics) > 1 {
		return false
	}
	if preds.IsCountQuestion ||
		preds.IsCategoryEnumeration ||
		preds.IsCrossComponent ||
		preds.IsRelationalLookup ||
		preds.IsHistoryLookup {
		return false
	}
	if len(trimStringSlice(entities)) > 1 {
		return false
	}
	switch intent {
	case types.IntentExplain, types.IntentUnknown, types.IntentReturnValue:
	default:
		return false
	}
	return true
}

func normalizeMissingAnswerSubjectForNonScalarExplain(
	axis types.PredicateAxis,
	intent types.Intent,
	preds types.SemanticPredicates,
	entities []string,
	subTopics []types.SubTopic,
	answerSubject types.AnswerSubject,
) (types.AnswerSubject, string) {
	if answerSubject.Kind != types.SubjectUnknown {
		return answerSubject, ""
	}
	if intent != types.IntentExplain || preds.IsScalarAnswer || preds.IsRoleLocateLookup {
		return answerSubject, ""
	}
	if !needsRoleLocateDisambiguation(axis, intent, preds, entities, subTopics) {
		return answerSubject, ""
	}
	answerSubject.Kind = types.SubjectGeneric
	if answerSubject.Confidence <= 0 {
		answerSubject.Confidence = 0.55
	}
	return answerSubject, "answer_subject.kind defaulted to generic for non-scalar explain classification"
}

// parsePredicates enforces the schema-v4 fail-loud contract for the
// `predicates` object: the LLM must explicitly emit every field as
// true OR false. A nil pointer means the LLM omitted the whole object;
// a non-nil pointer with any nil bool field means the LLM forgot one.
// Both cases reject and trigger analyzer retry — the rationale is
// that we deleted the prose-cue tables which used to detect these
// classifications from surface text, so the LLM judgment is now the
// only signal. Silently defaulting any field to false would mask a
// classification miss the same way the old prose-cue tables silently
// missed non-ZH/EN questions.
func parsePredicates(p *emitPredicatesParam) (types.SemanticPredicates, string) {
	if p == nil {
		return types.SemanticPredicates{},
			"predicates object missing — emit `predicates` with is_scalar_answer / is_role_locate_lookup / is_count_question / is_cross_component / is_relational_lookup / is_category_enumeration / is_history_lookup / is_diagnostic_question each set to true or false"
	}
	missing := []string{}
	if p.IsScalarAnswer == nil {
		missing = append(missing, "is_scalar_answer")
	}
	if p.IsRoleLocateLookup == nil {
		missing = append(missing, "is_role_locate_lookup")
	}
	if p.IsCountQuestion == nil {
		missing = append(missing, "is_count_question")
	}
	if p.IsCrossComponent == nil {
		missing = append(missing, "is_cross_component")
	}
	if p.IsRelationalLookup == nil {
		missing = append(missing, "is_relational_lookup")
	}
	if p.IsCategoryEnumeration == nil {
		missing = append(missing, "is_category_enumeration")
	}
	if p.IsHistoryLookup == nil {
		missing = append(missing, "is_history_lookup")
	}
	if p.IsDiagnosticQuestion == nil {
		missing = append(missing, "is_diagnostic_question")
	}
	if len(missing) > 0 {
		return types.SemanticPredicates{}, fmt.Sprintf(
			"predicates missing required field(s): %s — every field must be set explicitly to true or false (no silent default)",
			strings.Join(missing, ", "),
		)
	}
	return types.SemanticPredicates{
		IsScalarAnswer:        *p.IsScalarAnswer,
		IsRoleLocateLookup:    *p.IsRoleLocateLookup,
		IsCountQuestion:       *p.IsCountQuestion,
		IsCrossComponent:      *p.IsCrossComponent,
		IsRelationalLookup:    *p.IsRelationalLookup,
		IsCategoryEnumeration: *p.IsCategoryEnumeration,
		IsHistoryLookup:       *p.IsHistoryLookup,
		IsDiagnosticQuestion:  *p.IsDiagnosticQuestion,
	}, ""
}

func parseDiagnosticProfile(p *emitDiagnosticProfileParam, preds types.SemanticPredicates) (types.DiagnosticIntentProfile, string, []string) {
	const defaultConfidence = 0.5
	if p == nil {
		return types.DiagnosticIntentProfile{
				IsDiagnostic: preds.IsDiagnosticQuestion,
				Confidence:   defaultConfidence,
			}, "",
			[]string{"diagnostic_profile auto-defaulted: object missing; mirrored predicates.is_diagnostic_question and left current_risk/historical_regression/current_version_check=false"}
	}
	missing := []string{}
	currentRisk := false
	if p.IsDiagnostic == nil {
		missing = append(missing, "is_diagnostic")
	}
	if p.CurrentRisk == nil {
		missing = append(missing, "current_risk")
	} else {
		currentRisk = *p.CurrentRisk
	}
	historicalRegression := false
	if p.HistoricalRegression == nil {
		missing = append(missing, "historical_regression")
	} else {
		historicalRegression = *p.HistoricalRegression
	}
	currentVersionCheck := false
	if p.CurrentVersionCheck == nil {
		missing = append(missing, "current_version_check")
	} else {
		currentVersionCheck = *p.CurrentVersionCheck
	}
	confidence := defaultConfidence
	if p.Confidence == nil {
		missing = append(missing, "confidence")
	} else {
		confidence = *p.Confidence
	}
	if confidence < 0 || confidence > 1 {
		return types.DiagnosticIntentProfile{}, fmt.Sprintf(
			"diagnostic_profile.confidence %.2f out of [0,1]",
			confidence,
		), nil
	}
	isDiagnostic := preds.IsDiagnosticQuestion || currentRisk || historicalRegression
	if p.IsDiagnostic != nil {
		isDiagnostic = *p.IsDiagnostic
	}
	var warnings []string
	if len(missing) > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"diagnostic_profile auto-defaulted missing field(s): %s; used typed predicates/profile defaults without inferring from prose",
			strings.Join(missing, ", ")))
	}
	return types.DiagnosticIntentProfile{
		IsDiagnostic:         isDiagnostic,
		CurrentRisk:          currentRisk,
		HistoricalRegression: historicalRegression,
		CurrentVersionCheck:  currentVersionCheck,
		ObservationSummary:   strings.TrimSpace(p.ObservationSummary),
		Confidence:           confidence,
	}, "", warnings
}

// normalizeDiagnosticMirrorSignals consumes the two typed diagnostic
// lanes after both have parsed successfully. The predicate is the
// primary routing signal; the profile mirror is a support lane. Strong
// diagnostic facts (current risk / historical regression) may promote
// the predicate, but an isolated current_version_check never does: many
// ordinary code/config lookups also inspect the current checkout.
func normalizeDiagnosticMirrorSignals(
	preds types.SemanticPredicates,
	diagnostic types.DiagnosticIntentProfile,
) (types.SemanticPredicates, types.DiagnosticIntentProfile, []string) {
	var warnings []string
	addWarning := func(format string, args ...any) {
		warnings = append(warnings, fmt.Sprintf(format, args...))
	}

	strongDiagnostic := diagnostic.CurrentRisk || diagnostic.HistoricalRegression
	if !preds.IsDiagnosticQuestion && strongDiagnostic {
		preds.IsDiagnosticQuestion = true
		addWarning("mirror auto-align: predicates.is_diagnostic_question false→true because diagnostic_profile.current_risk/historical_regression is true")
		if !diagnostic.IsDiagnostic {
			diagnostic.IsDiagnostic = true
			addWarning("mirror auto-align: diagnostic_profile.is_diagnostic false→true because a strong diagnostic profile signal is true")
		}
		return preds, diagnostic, warnings
	}

	if preds.IsDiagnosticQuestion {
		if !diagnostic.IsDiagnostic {
			diagnostic.IsDiagnostic = true
			addWarning("mirror auto-align: diagnostic_profile.is_diagnostic false→true to match predicates.is_diagnostic_question")
		}
		return preds, diagnostic, warnings
	}

	if diagnostic.IsDiagnostic {
		diagnostic.IsDiagnostic = false
		addWarning("mirror auto-align: diagnostic_profile.is_diagnostic true→false to match predicates.is_diagnostic_question")
	}
	if diagnostic.CurrentVersionCheck {
		diagnostic.CurrentVersionCheck = false
		addWarning("mirror auto-align: diagnostic_profile.current_version_check true→false because no diagnostic predicate/current-risk/regression signal is active")
	}
	return preds, diagnostic, warnings
}

// normalizeDiagnosticRoute absorbs a narrow, typed self-consistency
// drift: the analyzer already emitted the diagnostic predicate, but
// left the generic explain/architecture route in place. The existing
// validator still enforces this invariant for direct callers; the
// normal Execute path repairs it before validation so an otherwise
// valid analysis does not burn another model round.
func normalizeDiagnosticRoute(
	intent types.Intent,
	scenario types.Scenario,
	preds types.SemanticPredicates,
) (types.Intent, types.Scenario, []string) {
	if !preds.IsDiagnosticQuestion {
		return intent, scenario, nil
	}
	var warnings []string
	if intent != types.IntentRootCause {
		warnings = append(warnings, fmt.Sprintf(
			"diagnostic route auto-align: intent %q→%q because predicates.is_diagnostic_question=true",
			intent, types.IntentRootCause,
		))
		intent = types.IntentRootCause
	}
	switch scenario {
	case types.ScenarioRootCause, types.ScenarioPerformanceBottleneck:
		// Keep a specific diagnostic/performance route when the model
		// already chose one of the allowed typed scenarios.
	default:
		warnings = append(warnings, fmt.Sprintf(
			"diagnostic route auto-align: scenario %q→%q because predicates.is_diagnostic_question=true",
			scenario, types.ScenarioRootCause,
		))
		scenario = types.ScenarioRootCause
	}
	return intent, scenario, warnings
}

func parseConversationReferenceProfile(p *emitConversationReferenceProfileParam) (*types.ConversationReferenceProfile, string) {
	if p == nil {
		return nil, ""
	}
	var missing []string
	if p.RequiresPriorContext == nil {
		missing = append(missing, "requires_prior_context")
	}
	if p.NeedsRepoVerification == nil {
		missing = append(missing, "needs_repo_verification")
	}
	if strings.TrimSpace(p.Ambiguity) == "" {
		missing = append(missing, "ambiguity")
	}
	if len(missing) > 0 {
		return nil, fmt.Sprintf(
			"conversation_reference_profile missing required field(s): %s",
			strings.Join(missing, ", "),
		)
	}
	ambiguity := types.ConversationReferenceAmbiguity(strings.TrimSpace(p.Ambiguity))
	if !ambiguity.IsValid() {
		return nil, fmt.Sprintf(
			"conversation_reference_profile.ambiguity %q is invalid; use one of %s",
			p.Ambiguity, strings.Join(conversationReferenceAmbiguityValues(), ", "),
		)
	}
	out := &types.ConversationReferenceProfile{
		RequiresPriorContext:  *p.RequiresPriorContext,
		NeedsRepoVerification: *p.NeedsRepoVerification,
		Ambiguity:             ambiguity,
	}
	for i, raw := range p.ResolvedSubjects {
		surface := strings.TrimSpace(raw.Surface)
		if surface == "" {
			return nil, fmt.Sprintf("conversation_reference_profile.resolved_subjects[%d].surface is required", i)
		}
		if raw.UseAsExactTarget == nil {
			return nil, fmt.Sprintf("conversation_reference_profile.resolved_subjects[%d].use_as_exact_target is required", i)
		}
		if raw.Confidence == nil {
			return nil, fmt.Sprintf("conversation_reference_profile.resolved_subjects[%d].confidence is required", i)
		}
		if *raw.Confidence < 0 || *raw.Confidence > 1 {
			return nil, fmt.Sprintf(
				"conversation_reference_profile.resolved_subjects[%d].confidence %.2f out of [0,1]",
				i, *raw.Confidence,
			)
		}
		source := types.ConversationReferenceSource(strings.TrimSpace(raw.Source))
		if !source.IsValid() {
			return nil, fmt.Sprintf(
				"conversation_reference_profile.resolved_subjects[%d].source %q is invalid; use one of %s",
				i, raw.Source, strings.Join(conversationReferenceSourceValues(), ", "),
			)
		}
		if source == types.ConversationReferenceSourceCurrentRequest {
			// Current-request subjects are already represented by
			// entities/exact_targets. Treat this as a redundant local-model
			// emission rather than forcing an analyzer retry or polluting the
			// prior-conversation lane.
			continue
		}
		kind := types.AnswerSubjectKind(strings.TrimSpace(raw.Kind))
		if !kind.IsValid() {
			return nil, fmt.Sprintf(
				"conversation_reference_profile.resolved_subjects[%d].kind %q is invalid",
				i, raw.Kind,
			)
		}
		if !out.RequiresPriorContext {
			out.RequiresPriorContext = true
			logging.Warning("[emit_analysis] normalized conversation_reference_profile.requires_prior_context false→true because resolved_subjects source=%s", source)
		}
		out.ResolvedSubjects = append(out.ResolvedSubjects, types.ResolvedConversationSubject{
			Surface:          surface,
			Kind:             kind,
			Source:           source,
			Role:             strings.TrimSpace(raw.Role),
			UseAsExactTarget: *raw.UseAsExactTarget,
			Confidence:       *raw.Confidence,
		})
	}
	if out.RequiresPriorContext &&
		out.Ambiguity == types.ConversationReferenceAmbiguityNone &&
		len(out.ResolvedSubjects) == 0 {
		return nil, "conversation_reference_profile.ambiguity=none requires at least one resolved_subject"
	}
	if !out.RequiresPriorContext {
		return nil, ""
	}
	return out, ""
}

func parseSourceScopeProfile(p *emitSourceScopeProfileParam) (*types.SourceScopeProfile, string) {
	if p == nil {
		return nil, ""
	}
	var missing []string
	if strings.TrimSpace(p.RequestedScope) == "" {
		missing = append(missing, "requested_scope")
	}
	if p.Confidence == nil {
		missing = append(missing, "confidence")
	}
	if len(missing) > 0 {
		return nil, fmt.Sprintf(
			"source_scope_profile missing required field(s): %s",
			strings.Join(missing, ", "),
		)
	}
	if *p.Confidence < 0 || *p.Confidence > 1 {
		return nil, fmt.Sprintf("source_scope_profile.confidence %.2f out of [0,1]", *p.Confidence)
	}
	scope := types.SourceScope(strings.TrimSpace(p.RequestedScope))
	if !scope.IsValid() {
		return nil, fmt.Sprintf(
			"source_scope_profile.requested_scope %q is invalid; use one of %s",
			p.RequestedScope, strings.Join(sourceScopeValues(), ", "),
		)
	}
	includeAux := false
	if p.IncludeAuxiliaryAsPrincipal != nil {
		includeAux = *p.IncludeAuxiliaryAsPrincipal
	}
	return &types.SourceScopeProfile{
		RequestedScope:              scope,
		IncludeAuxiliaryAsPrincipal: includeAux,
		Confidence:                  *p.Confidence,
		Rationale:                   strings.TrimSpace(p.Rationale),
	}, ""
}

func parseAnswerVisibilityProfile(raw string, p *emitAnswerVisibilityProfileParam) (*types.AnswerVisibilityProfile, string, []string) {
	if p == nil {
		return nil, "", nil
	}
	var missing []string
	if strings.TrimSpace(p.SymbolVisibility) == "" {
		missing = append(missing, "symbol_visibility")
	}
	if p.Confidence == nil {
		missing = append(missing, "confidence")
	}
	if len(missing) > 0 {
		return nil, fmt.Sprintf(
			"answer_visibility_profile missing required field(s): %s",
			strings.Join(missing, ", "),
		), nil
	}
	if *p.Confidence < 0 || *p.Confidence > 1 {
		return nil, fmt.Sprintf("answer_visibility_profile.confidence %.2f out of [0,1]", *p.Confidence), nil
	}
	visibility := types.AnswerSymbolVisibility(strings.TrimSpace(p.SymbolVisibility))
	if !visibility.IsValid() {
		return nil, fmt.Sprintf(
			"answer_visibility_profile.symbol_visibility %q is invalid; use one of %s",
			p.SymbolVisibility, strings.Join(answerSymbolVisibilityValues(), ", "),
		), nil
	}
	sourceQuotes := trimStringSlice(p.SourceQuotes)
	var warnings []string
	keptQuotes := sourceQuotes[:0]
	for _, quote := range sourceQuotes {
		if sourceQuotePresentInCurrentRequest(raw, quote) {
			keptQuotes = append(keptQuotes, quote)
			continue
		}
		warnings = append(warnings, "answer_visibility_profile.source_quotes entry ignored because it is not copied verbatim from the current request")
	}
	sourceQuotes = keptQuotes
	if visibility == types.AnswerSymbolVisibilityUnknown {
		return nil, "", warnings
	}
	if visibility != types.AnswerSymbolVisibilityAll && len(sourceQuotes) == 0 {
		warnings = append(warnings, "answer_visibility_profile kept without source_quotes; downstream will treat it as advisory typed scope only")
	}
	return &types.AnswerVisibilityProfile{
		SymbolVisibility: visibility,
		SourceQuotes:     append([]string(nil), sourceQuotes...),
		Confidence:       *p.Confidence,
		Rationale:        strings.TrimSpace(p.Rationale),
	}, "", warnings
}

func parseChangeImpactProfile(p *emitChangeImpactProfileParam) (*types.ChangeImpactProfile, string) {
	if p == nil {
		return nil, ""
	}
	var missing []string
	if p.IsChangeImpact == nil {
		missing = append(missing, "is_change_impact")
	}
	if p.Confidence == nil {
		missing = append(missing, "confidence")
	}
	if len(missing) > 0 {
		return nil, fmt.Sprintf(
			"change_impact_profile missing required field(s): %s",
			strings.Join(missing, ", "),
		)
	}
	if *p.Confidence < 0 || *p.Confidence > 1 {
		return nil, fmt.Sprintf("change_impact_profile.confidence %.2f out of [0,1]", *p.Confidence)
	}
	if !*p.IsChangeImpact {
		return nil, ""
	}
	target := strings.TrimSpace(p.Target)
	if target == "" {
		return nil, "change_impact_profile.target is required when is_change_impact=true"
	}
	targetKind := types.AnswerSubjectKind(strings.TrimSpace(p.TargetKind))
	if !targetKind.IsValid() {
		return nil, fmt.Sprintf(
			"change_impact_profile.target_kind %q is invalid; use one of %s",
			p.TargetKind, strings.Join(skill.AnalysisAnswerSubjectValues(), ", "),
		)
	}
	scope := types.ImpactScope(strings.TrimSpace(p.Scope))
	if scope == "" {
		scope = types.ImpactScopeUnknown
	}
	if !scope.IsValid() {
		return nil, fmt.Sprintf(
			"change_impact_profile.scope %q is invalid; use one of %s",
			p.Scope, strings.Join(impactScopeValues(), ", "),
		)
	}
	requestedOutput := types.ImpactRequestedOutput(strings.TrimSpace(p.RequestedOutput))
	if requestedOutput == "" {
		requestedOutput = types.ImpactOutputUnknown
	}
	if !requestedOutput.IsValid() {
		return nil, fmt.Sprintf(
			"change_impact_profile.requested_output %q is invalid; use one of %s",
			p.RequestedOutput, strings.Join(impactRequestedOutputValues(), ", "),
		)
	}
	seenKinds := map[types.ImpactAffectedSiteKind]bool{}
	var kinds []types.ImpactAffectedSiteKind
	for i, raw := range p.AffectedSiteKinds {
		kind := types.ImpactAffectedSiteKind(strings.TrimSpace(raw))
		if !kind.IsValid() {
			return nil, fmt.Sprintf(
				"change_impact_profile.affected_site_kinds[%d] %q is invalid; use one of %s",
				i, raw, strings.Join(impactAffectedSiteKindValues(), ", "),
			)
		}
		if seenKinds[kind] {
			continue
		}
		seenKinds[kind] = true
		kinds = append(kinds, kind)
	}
	return &types.ChangeImpactProfile{
		IsChangeImpact:    true,
		Target:            target,
		TargetKind:        targetKind,
		Scope:             scope,
		RequestedOutput:   requestedOutput,
		AffectedSiteKinds: kinds,
		Confidence:        *p.Confidence,
		Rationale:         strings.TrimSpace(p.Rationale),
	}, ""
}

func parseFieldValueProfile(raw string, p *emitFieldValueProfileParam) (*types.FieldValueLookupProfile, string) {
	if p == nil {
		return nil, ""
	}
	var missing []string
	if p.IsFieldValueLookup == nil {
		missing = append(missing, "is_field_value_lookup")
	}
	if p.Confidence == nil {
		missing = append(missing, "confidence")
	}
	if len(missing) > 0 {
		return nil, fmt.Sprintf(
			"field_value_profile missing required field(s): %s",
			strings.Join(missing, ", "),
		)
	}
	if *p.Confidence < 0 || *p.Confidence > 1 {
		return nil, fmt.Sprintf("field_value_profile.confidence %.2f out of [0,1]", *p.Confidence)
	}
	if !*p.IsFieldValueLookup {
		return nil, ""
	}
	target := strings.TrimSpace(p.Target)
	literal := strings.TrimSpace(p.Literal)
	sourceQuote := strings.TrimSpace(p.SourceQuote)
	if target == "" {
		return nil, "field_value_profile.target is required when is_field_value_lookup=true"
	}
	if literal == "" {
		return nil, "field_value_profile.literal is required when is_field_value_lookup=true"
	}
	if sourceQuote == "" {
		return nil, "field_value_profile.source_quote is required when is_field_value_lookup=true"
	}
	if !sourceQuotePresentInCurrentRequest(raw, sourceQuote) {
		return nil, "field_value_profile.source_quote must be copied verbatim from the current request"
	}
	if !sourceQuoteContainsTargetAndLiteral(sourceQuote, target, literal) {
		return nil, "field_value_profile.source_quote must include both target and literal"
	}
	full, owner, field, ok := types.ParseFieldValueTarget(target)
	if !ok {
		return nil, "field_value_profile.target must be an owner-qualified field/member/config surface"
	}
	literalKind := types.FieldValueLiteralKind(strings.TrimSpace(p.LiteralKind))
	if !literalKind.IsValid() {
		return nil, fmt.Sprintf(
			"field_value_profile.literal_kind %q is invalid; use one of %s",
			p.LiteralKind, strings.Join(fieldValueLiteralKindValues(), ", "),
		)
	}
	return &types.FieldValueLookupProfile{
		IsFieldValueLookup: true,
		Target:             full,
		Owner:              owner,
		Field:              field,
		Literal:            literal,
		LiteralKind:        literalKind,
		SourceQuote:        sourceQuote,
		Confidence:         *p.Confidence,
		Rationale:          strings.TrimSpace(p.Rationale),
	}, ""
}

func parseAnswerExclusionPolicy(raw string, p *emitAnswerExclusionPolicyParam) (*types.AnswerExclusionPolicy, string) {
	if p == nil {
		return nil, ""
	}
	var missing []string
	if p.IsExclusionRequested == nil {
		missing = append(missing, "is_exclusion_requested")
	}
	if p.Confidence == nil {
		missing = append(missing, "confidence")
	}
	if len(missing) > 0 {
		return nil, fmt.Sprintf(
			"answer_exclusion_policy missing required field(s): %s",
			strings.Join(missing, ", "),
		)
	}
	if *p.Confidence < 0 || *p.Confidence > 1 {
		return nil, fmt.Sprintf("answer_exclusion_policy.confidence %.2f out of [0,1]", *p.Confidence)
	}
	if !*p.IsExclusionRequested {
		return nil, ""
	}
	roles := make([]types.AnswerCandidateRole, 0, len(p.ExcludedCandidateRoles))
	seen := make(map[types.AnswerCandidateRole]struct{}, len(p.ExcludedCandidateRoles))
	for _, rawRole := range p.ExcludedCandidateRoles {
		role, ok := types.NormalizeAnswerCandidateRole(rawRole)
		if !ok || role == types.AnswerCandidateRoleUnknown {
			return nil, fmt.Sprintf(
				"answer_exclusion_policy.excluded_candidate_roles contains invalid role %q; use one of %s",
				rawRole, strings.Join(answerCandidateRoleValues(), ", "),
			)
		}
		if _, dup := seen[role]; dup {
			continue
		}
		seen[role] = struct{}{}
		roles = append(roles, role)
	}
	if len(roles) == 0 {
		return nil, "answer_exclusion_policy.excluded_candidate_roles is required when is_exclusion_requested=true"
	}
	sourceQuotes := trimStringSlice(p.SourceQuotes)
	if len(sourceQuotes) == 0 {
		return nil, "answer_exclusion_policy.source_quotes is required when is_exclusion_requested=true"
	}
	for _, quote := range sourceQuotes {
		if !sourceQuotePresentInCurrentRequest(raw, quote) {
			return nil, "answer_exclusion_policy.source_quotes entries must be copied verbatim from the current request"
		}
	}
	return &types.AnswerExclusionPolicy{
		IsExclusionRequested:   true,
		ExcludedCandidateRoles: roles,
		SourceQuotes:           sourceQuotes,
		Confidence:             *p.Confidence,
		Rationale:              strings.TrimSpace(p.Rationale),
	}, ""
}

func parseAnswerRoleProfile(raw string, p *emitAnswerRoleProfileParam) (*types.AnswerRoleProfile, string) {
	if p == nil {
		return nil, "answer_role_profile object missing — emit `answer_role_profile` with is_role_binding_requested set to true or false, plus confidence in [0,1]"
	}
	var missing []string
	if p.IsRoleBindingRequested == nil {
		missing = append(missing, "is_role_binding_requested")
	}
	if p.Confidence == nil {
		missing = append(missing, "confidence")
	}
	if len(missing) > 0 {
		return nil, fmt.Sprintf(
			"answer_role_profile missing required field(s): %s",
			strings.Join(missing, ", "),
		)
	}
	if *p.Confidence < 0 || *p.Confidence > 1 {
		return nil, fmt.Sprintf("answer_role_profile.confidence %.2f out of [0,1]", *p.Confidence)
	}
	if !*p.IsRoleBindingRequested {
		return nil, ""
	}
	roles := make([]types.AnswerCandidateRole, 0, len(p.RequiredCandidateRoles))
	seen := make(map[types.AnswerCandidateRole]struct{}, len(p.RequiredCandidateRoles))
	for _, rawRole := range p.RequiredCandidateRoles {
		role, ok := types.NormalizeAnswerCandidateRole(rawRole)
		if !ok || role == types.AnswerCandidateRoleUnknown {
			return nil, fmt.Sprintf(
				"answer_role_profile.required_candidate_roles contains invalid role %q; use one of %s",
				rawRole, strings.Join(answerCandidateRoleValues(), ", "),
			)
		}
		if _, dup := seen[role]; dup {
			continue
		}
		seen[role] = struct{}{}
		roles = append(roles, role)
	}
	if len(roles) == 0 {
		return nil, "answer_role_profile.required_candidate_roles is required when is_role_binding_requested=true"
	}
	sourceQuotes := trimStringSlice(p.SourceQuotes)
	if len(sourceQuotes) == 0 {
		return nil, "answer_role_profile.source_quotes is required when is_role_binding_requested=true"
	}
	for _, quote := range sourceQuotes {
		if !sourceQuotePresentInCurrentRequest(raw, quote) {
			return nil, "answer_role_profile.source_quotes entries must be copied verbatim from the current request"
		}
	}
	return &types.AnswerRoleProfile{
		IsRoleBindingRequested: true,
		RequiredCandidateRoles: roles,
		SourceQuotes:           sourceQuotes,
		Confidence:             *p.Confidence,
		Rationale:              strings.TrimSpace(p.Rationale),
	}, ""
}

func parseErrorGranularityProfile(raw string, p *emitErrorGranularityProfileParam) (*types.ErrorGranularityProfile, string, []string) {
	if p == nil {
		return nil, "error_granularity_profile object missing — emit `error_granularity_profile` with is_granularity_question set to true or false, plus confidence in [0,1]", nil
	}
	var missing []string
	if p.IsGranularityQuestion == nil {
		missing = append(missing, "is_granularity_question")
	}
	if p.Confidence == nil {
		missing = append(missing, "confidence")
	}
	if len(missing) > 0 {
		return nil, fmt.Sprintf(
			"error_granularity_profile missing required field(s): %s",
			strings.Join(missing, ", "),
		), nil
	}
	if *p.Confidence < 0 || *p.Confidence > 1 {
		return nil, fmt.Sprintf("error_granularity_profile.confidence %.2f out of [0,1]", *p.Confidence), nil
	}
	if !*p.IsGranularityQuestion {
		return nil, "", nil
	}
	var warnings []string
	sourceQuotes := trimStringSlice(p.SourceQuotes)
	if len(sourceQuotes) == 0 {
		return nil, "", []string{"error_granularity_profile auto-softened: source_quotes missing while is_granularity_question=true; optional profile ignored"}
	}
	anchoredQuotes := make([]string, 0, len(sourceQuotes))
	for _, quote := range sourceQuotes {
		if sourceQuoteAnchoredInCurrentRequest(raw, quote) {
			anchoredQuotes = append(anchoredQuotes, quote)
			continue
		}
		warnings = append(warnings, fmt.Sprintf("error_granularity_profile ignored unanchored source_quote %q", quote))
	}
	if len(anchoredQuotes) == 0 {
		warnings = append(warnings, "error_granularity_profile auto-softened: no source_quote anchors the current request; optional profile ignored")
		return nil, "", warnings
	}
	options := make([]types.ErrorGranularityVerdict, 0, len(p.RequestedVerdictOptions))
	seenOptions := make(map[types.ErrorGranularityVerdict]struct{}, len(p.RequestedVerdictOptions))
	for _, rawOption := range p.RequestedVerdictOptions {
		option, ok := types.NormalizeErrorGranularityVerdict(rawOption)
		if !ok || option == types.ErrorGranularityUnknown {
			return nil, fmt.Sprintf(
				"error_granularity_profile.requested_verdict_options contains invalid verdict %q; use one of %s",
				rawOption, strings.Join(errorGranularityRequestedOptionValues(), ", "),
			), nil
		}
		if option == types.ErrorGranularityNotEnoughEvidence {
			return nil, "error_granularity_profile.requested_verdict_options must list user-stated alternatives, not not_enough_evidence", nil
		}
		if _, dup := seenOptions[option]; dup {
			continue
		}
		seenOptions[option] = struct{}{}
		options = append(options, option)
	}
	return &types.ErrorGranularityProfile{
		IsGranularityQuestion:   true,
		RequestedVerdictOptions: options,
		SourceQuotes:            anchoredQuotes,
		Confidence:              *p.Confidence,
		Rationale:               strings.TrimSpace(p.Rationale),
	}, "", warnings
}

func sourceQuotePresentInCurrentRequest(raw, quote string) bool {
	raw = strings.TrimSpace(raw)
	quote = strings.TrimSpace(quote)
	return raw != "" && quote != "" && strings.Contains(raw, quote)
}

func sourceQuoteAnchoredInCurrentRequest(raw, quote string) bool {
	if sourceQuotePresentInCurrentRequest(raw, quote) {
		return true
	}
	normalizedRaw := normalizeForSourceQuoteMatch(raw)
	normalizedQuote := normalizeForSourceQuoteMatch(quote)
	return normalizedRaw != "" && normalizedQuote != "" && strings.Contains(normalizedRaw, normalizedQuote)
}

func normalizeForSourceQuoteMatch(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		r = normalizeFullWidthASCII(r)
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

func normalizeFullWidthASCII(r rune) rune {
	if r == '\u3000' {
		return ' '
	}
	if r >= '\uff01' && r <= '\uff5e' {
		return r - 0xfee0
	}
	return r
}

func sourceQuoteContainsTargetAndLiteral(sourceQuote, target, literal string) bool {
	if len(types.MentionedEntitiesFromRawRequest(sourceQuote, []string{target})) == 0 {
		return false
	}
	return fieldValueLiteralAppearsInQuote(sourceQuote, literal)
}

func fieldValueLiteralAppearsInQuote(sourceQuote, literal string) bool {
	sourceQuote = strings.TrimSpace(sourceQuote)
	literal = strings.Trim(strings.TrimSpace(literal), "`")
	if sourceQuote == "" || literal == "" {
		return false
	}
	start := 0
	for {
		idx := strings.Index(sourceQuote[start:], literal)
		if idx < 0 {
			return false
		}
		pos := start + idx
		if fieldValueQuoteBoundary(sourceQuote, pos-1) &&
			fieldValueQuoteBoundary(sourceQuote, pos+len(literal)) {
			return true
		}
		start = pos + len(literal)
		if start >= len(sourceQuote) {
			return false
		}
	}
}

func fieldValueQuoteBoundary(s string, idx int) bool {
	if idx < 0 || idx >= len(s) {
		return true
	}
	r := rune(s[idx])
	return !(r == '_' || r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z')
}

func validateExactTargets(raw string, in []string) ([]string, string) {
	if len(in) == 0 {
		return nil, ""
	}
	validated := types.MentionedEntitiesFromRawRequest(raw, in)
	if len(validated) == len(in) {
		return validated, ""
	}
	if len(validated) == 0 {
		return nil, "exact_targets were provided, but none are explicitly present in the current request text; omit the field when unsure"
	}
	var invalid []string
	seen := make(map[string]bool, len(validated))
	for _, item := range validated {
		seen[strings.ToLower(strings.TrimSpace(item))] = true
	}
	for _, item := range in {
		key := strings.ToLower(strings.TrimSpace(item))
		if key == "" || seen[key] {
			continue
		}
		invalid = append(invalid, strings.TrimSpace(item))
	}
	sort.Strings(invalid)
	return nil, fmt.Sprintf(
		"exact_targets must be copied verbatim from the current request text; these item(s) were not validated: %s",
		strings.Join(invalid, ", "),
	)
}

func sanitizeExactContextTerms(exactTargets, mentionedEntities, in []string) ([]string, string) {
	if len(in) == 0 {
		return nil, ""
	}
	if len(exactTargets) == 0 {
		return nil, "dropped exact_context_terms because exact_targets were absent or ambiguous"
	}
	allowed := make(map[string]bool)
	for _, source := range append(append([]string(nil), exactTargets...), mentionedEntities...) {
		for _, term := range types.ExactResolutionIdentifierTerms(source) {
			if len(term) >= 3 {
				allowed[term] = true
			}
		}
	}
	if len(allowed) == 0 {
		return nil, "dropped exact_context_terms because no request-grounded exact-target vocabulary was available"
	}
	seen := make(map[string]bool)
	var out []string
	var dropped []string
	for _, item := range in {
		norm := strings.TrimSpace(strings.ToLower(item))
		if norm == "" {
			continue
		}
		if !allowed[norm] {
			dropped = append(dropped, strings.TrimSpace(item))
			continue
		}
		if seen[norm] {
			continue
		}
		seen[norm] = true
		out = append(out, norm)
	}
	if len(dropped) == 0 {
		return out, ""
	}
	sort.Strings(dropped)
	return out, fmt.Sprintf(
		"ignored exact_context_terms outside the request-mentioned exact-target lane: %s",
		strings.Join(dropped, ", "),
	)
}

func sanitizeExactContextRoles(exactTargets []string, subjectKind types.AnswerSubjectKind, scenario types.Scenario, analyzerKind string, in []string) ([]types.EvidenceDiagramRole, string) {
	if len(in) == 0 {
		return nil, ""
	}
	if len(exactTargets) == 0 {
		return nil, "dropped exact_context_roles because exact_targets were absent or ambiguous"
	}
	if scenario != types.ScenarioConfigTrace {
		return nil, "dropped exact_context_roles because this request is not a config-trace exact-target question"
	}
	if subjectKind != types.SubjectUnknown &&
		subjectKind != types.SubjectConfigKey &&
		!strings.EqualFold(strings.TrimSpace(analyzerKind), "config_mapping") {
		return nil, "dropped exact_context_roles because this request does not resolve to a config-key subject"
	}
	raw := trimStringSlice(in)
	if len(raw) == 0 {
		return nil, ""
	}
	seen := make(map[types.EvidenceDiagramRole]bool, len(raw))
	var out []types.EvidenceDiagramRole
	var dropped []string
	for _, item := range raw {
		role := types.CanonicalEvidenceDiagramRole(item)
		switch role {
		case types.EvidenceDiagramRoleDefault,
			types.EvidenceDiagramRoleConfig,
			types.EvidenceDiagramRoleRuntime,
			types.EvidenceDiagramRoleOverride:
			if !seen[role] {
				seen[role] = true
				out = append(out, role)
			}
		default:
			dropped = append(dropped, item)
		}
	}
	if len(out) == 0 {
		return nil, "dropped exact_context_roles because none mapped to valid abstract precedence roles"
	}
	if len(dropped) == 0 {
		return out, ""
	}
	sort.Strings(dropped)
	return out, fmt.Sprintf(
		"ignored exact_context_roles outside the validated precedence-role enum: %s",
		strings.Join(dropped, ", "),
	)
}

// validateConfidenceRange enforces the [0, 1] domain on every
// confidence field. Out-of-range values reject the call so a
// prompt-injection or schema-misread cannot smuggle a 99.0 confidence
// past the downstream guards (which gate aggressive narrowing on
// confidence ≥ 0.7).
func validateConfidenceRange(intentConf, compConf, kindConf float64) string {
	for _, c := range [...]struct {
		name string
		val  float64
	}{
		{"intent_confidence", intentConf},
		{"complexity_confidence", compConf},
		{"kind_confidence", kindConf},
	} {
		if c.val < 0.0 || c.val > 1.0 {
			return fmt.Sprintf("%s = %.2f out of [0.0, 1.0]", c.name, c.val)
		}
	}
	return ""
}

// parsePredicateAxis coerces the optional predicate_axis enum string
// into a typed PredicateAxis. Empty string is a legitimate "no axis
// extracted" signal (AxisUnknown). An unrecognised non-empty value
// is rejected so a typo cannot silently downgrade to AxisUnknown
// and disable the axis-aware ranker boost.
func parsePredicateAxis(s string) (types.PredicateAxis, string) {
	axis := types.PredicateAxis(strings.TrimSpace(s))
	if !axis.IsValid() {
		return types.AxisUnknown, fmt.Sprintf(
			"predicate_axis = %q is not a recognised axis — use one of the enum values or omit the field",
			s,
		)
	}
	return axis, ""
}

// parseDiagramHint coerces the optional emit_analysis.diagram_hint
// field into a typed DiagramHint. Empty / nil means "no hint". An
// unrecognised non-empty value is rejected so a typo cannot silently
// degrade to no hint.
func parseDiagramHint(p *emitDiagramHintParam) (*types.DiagramHint, string) {
	if p == nil {
		return nil, ""
	}
	kind := types.DiagramKind(strings.TrimSpace(p.Kind))
	if kind == types.DiagramNone {
		return nil, ""
	}
	if !kind.IsValid() {
		return nil, fmt.Sprintf(
			"diagram_hint.kind = %q is not a recognised diagram kind — use one of the enum values or omit the field",
			p.Kind,
		)
	}
	return &types.DiagramHint{Kind: kind}, ""
}

func scalarCountBoundaryIsScopeOnly(predicates types.SemanticPredicates) bool {
	return predicates.IsCountQuestion &&
		predicates.IsScalarAnswer &&
		!predicates.IsCategoryEnumeration
}

// parseEnumerationBoundary validates and normalises the optional
// enumeration_boundary handoff. Returns (boundary, rejectMsg, warnMsg).
//
// Field is schema-OPTIONAL (omitempty pointer). Downstream consumers
// (answer_surface_plan, facet_plan, request_traits, erm_completeness,
// question_structure) all guard on nil + DeclaredCount > 0, so a
// missing value is benign.
//
// Reject vs warn policy:
//   - DeclaredCount <= 0 AND empty SourceQuote → soft strip + warn
//     because the LLM emitted an inactive optional object. Downstream
//     already treats nil as no boundary, so a retry just to delete this
//     object is waste.
//   - DeclaredCount <= 0 with a non-empty SourceQuote → hard reject
//     (partial active payload; the LLM emitted an ambiguous boundary
//     and must fix it).
//   - DeclaredCount > 0 with an empty SourceQuote → soft strip + warn.
//     The count lane is optional and not load-bearing unless it carries
//     a verbatim user quote; spending a full analyzer retry just to
//     delete an inert optional object is waste.
//   - source_quote fails NormalizeRequestedEnumerationBoundary (quote
//     not in raw OR quote does not contain the count literal) → soft
//     strip + warn (the field is optional, downstream tolerates nil,
//     and a hard reject costs a full LLM retry round just to drop
//     one optional handoff). Mirrors the strip+warn precedent used
//     for ErrorGranularityCountsAreContextual (line ~912) and the
//     sanitizeExactContextTerms / sanitizeExactContextRoles pattern.
func parseEnumerationBoundary(raw string, p *emitEnumerationBoundaryParam) (*types.RequestedEnumerationBoundary, string, string) {
	if p == nil {
		return nil, "", ""
	}
	if p.DeclaredCount <= 0 && strings.TrimSpace(p.SourceQuote) == "" {
		return nil, "", "ignored inactive enumeration_boundary with declared_count<=0 and empty source_quote"
	}
	if p.DeclaredCount <= 0 {
		return nil, "enumeration_boundary.declared_count must be >= 1", ""
	}
	if strings.TrimSpace(p.SourceQuote) == "" {
		return nil, "", fmt.Sprintf(
			"ignored enumeration_boundary because source_quote is empty and cannot prove the declared count %d came from the current request",
			p.DeclaredCount,
		)
	}
	boundary := types.NormalizeRequestedEnumerationBoundary(raw, &types.RequestedEnumerationBoundary{
		DeclaredCount: p.DeclaredCount,
		SourceQuote:   p.SourceQuote,
	})
	if boundary == nil {
		warn := fmt.Sprintf(
			"ignored enumeration_boundary because source_quote %q must appear verbatim in the request and contain the declared count %d",
			strings.TrimSpace(p.SourceQuote), p.DeclaredCount,
		)
		return nil, "", warn
	}
	return boundary, "", ""
}

// parseCompletenessObligation (Plan E, 2026-05-02) validates the
// LLM-emitted completeness obligation. Mirrors
// parseEnumerationBoundary's contract:
//   - nil input → no obligation, no error
//   - required=true MUST come with a non-empty source_quote that
//     verbatim-appears in raw; missing or unmatched quote → reject
//     loudly (not silent) so the LLM gets corrective feedback
//   - required=false → silently treated as "no obligation"
func parseCompletenessObligation(raw string, p *emitCompletenessObligationParam) (*types.CompletenessObligation, string) {
	if p == nil {
		return nil, ""
	}
	if !p.Required {
		// LLM explicitly said "no completeness demand" — silently
		// treated as nil. No error.
		return nil, ""
	}
	if strings.TrimSpace(p.SourceQuote) == "" {
		return nil, "completeness_obligation.source_quote must be copied verbatim from the current request when required=true"
	}
	out := types.NormalizeCompletenessObligation(raw, &types.CompletenessObligation{
		Required:    true,
		SourceQuote: p.SourceQuote,
	})
	if out == nil {
		return nil, "completeness_obligation.source_quote must appear in the current request text (whitespace-insensitive match allowed)"
	}
	return out, ""
}

// parseQuestionBuckets (Plan E, 2026-05-02) validates the optional
// LLM-emitted bucket partition. The shared NormalizeBuckets helper
// still enforces precise user provenance by dropping labels that do
// not verbatim-appear in the current request.
//
// Commercial retry policy: buckets are a helpful comparison scaffold,
// not the primary analyzer classification. If a non-empty bucket array
// normalizes to zero or one usable bucket, strip the optional partition
// and warn. Downstream can still infer buckets from user-mentioned
// entities / required files through RequestModel.QuestionStructure().
// Re-prompting the analyzer just to delete or rename optional buckets
// is too expensive and can degrade otherwise-good intent analysis.
func parseQuestionBuckets(raw string, in []emitQuestionBucketParam) ([]types.QuestionBucket, string, string) {
	if len(in) == 0 {
		return nil, "", ""
	}
	prelim := make([]types.QuestionBucket, 0, len(in))
	for _, b := range in {
		prelim = append(prelim, types.QuestionBucket{
			Label:   b.Label,
			Anchors: b.Anchors,
			Index:   b.Index,
		})
	}
	out := types.NormalizeBuckets(raw, prelim)
	if out == nil {
		return nil, "", "ignored buckets because no label survived current-request provenance validation"
	}
	if len(out) < 2 {
		return nil, "", "ignored buckets because a comparison partition needs at least 2 current-request labels"
	}
	return out, "", ""
}

// parseAnswerSubject coerces the optional emit_analysis.answer_subject
// field into a typed AnswerSubject. Returns the zero value when the
// LLM omitted the field; the analyzer's deterministic
// inferAnswerSubject fallback fills it later from cues +
// question_kind. An unrecognised kind value is silently coerced to
// SubjectUnknown — invalid kinds should be caught by the schema's
// enum constraint, but defensive parsing keeps a malformed call from
// short-circuiting downstream stages.
func parseAnswerSubject(p *emitAnswerSubjectParam) types.AnswerSubject {
	if p == nil {
		return types.AnswerSubject{}
	}
	kind := types.AnswerSubjectKind(strings.TrimSpace(p.Kind))
	if !kind.IsValid() {
		kind = types.SubjectUnknown
	}
	return types.AnswerSubject{
		Kind:       kind,
		EntityAxes: trimStringSlice(p.EntityAxes),
		Confidence: p.Confidence,
	}
}

// buildEmitAnalysisSummary renders a one-line, trace-friendly summary
// that reports the POST-normalization state of every classification
// field, flags any LLM→canonical coercion (e.g. "root-cause" →
// "root_cause"), lists dropped generic entities, and surfaces
// validator warnings. The single-line format matches the rest of the
// tool package's Summary conventions and keeps the REPL trace
// readable, while still letting the operator diff LLM input against
// the system's normalized interpretation at a glance.
//
// Format:
//
//	analysis emitted: intent=<canonical> scenario=<canonical> ...
//	 | normalized: <field> <raw>→<canonical>, ...
//	 | warn: <warning>; <warning>
//
// The "normalized" and "warn" clauses only appear when they have
// content, so happy-path calls still produce a compact summary.
func buildEmitAnalysisSummary(raw emitAnalysisParams, rm types.RequestModel, val analysisValidationResult) string {
	var b strings.Builder
	h := rm.AnalyzerHints

	fmt.Fprintf(&b, "analysis emitted: intent=%s scenario=%s complexity=%s kw=%d ent=%d kind=%s",
		rm.Intent, rm.Scenario, rm.Complexity,
		len(h.Keywords), len(h.Entities),
		h.Kind)
	if len(h.ExactTargets) > 0 {
		fmt.Fprintf(&b, " exact=%d", len(h.ExactTargets))
	}
	if len(h.ExactContextTerms) > 0 {
		fmt.Fprintf(&b, " exact_ctx=%d", len(h.ExactContextTerms))
	}
	if len(h.ExactContextRoles) > 0 {
		fmt.Fprintf(&b, " exact_roles=%d", len(h.ExactContextRoles))
	}
	if rm.PredicateAxis != types.AxisUnknown {
		fmt.Fprintf(&b, " axis=%s", rm.PredicateAxis)
	}
	if rm.DiagramHint != nil && rm.DiagramHint.Kind != types.DiagramNone {
		fmt.Fprintf(&b, " diagram_hint=%s", rm.DiagramHint.Kind)
	}
	if rm.EnumerationBoundary != nil {
		fmt.Fprintf(&b, " boundary=%s", types.EnumerationBoundaryCountString(rm.EnumerationBoundary))
	}
	if rm.Predicates.IsHistoryLookup {
		b.WriteString(" history_lookup=true")
	}
	if rm.Predicates.IsRoleLocateLookup {
		b.WriteString(" role_locate=true")
	}
	if rm.DiagnosticProfile.RequiresDiagnosticRootCause() {
		b.WriteString(" diagnostic_profile=true")
	}
	if rm.DiagnosticProfile.RequiresCurrentStatusDiagnostic() {
		b.WriteString(" current_status_check=true")
	}
	if rm.ConversationReferenceProfile != nil && rm.ConversationReferenceProfile.RequiresPriorContext {
		fmt.Fprintf(&b, " conversation_refs=%d", len(rm.ConversationReferenceProfile.ResolvedSubjects))
	}
	if rm.SourceScopeProfile != nil {
		fmt.Fprintf(&b, " source_scope=%s", rm.SourceScopeProfile.RequestedScope)
	}
	if rm.AnswerVisibilityProfile != nil && rm.AnswerVisibilityProfile.Active() {
		fmt.Fprintf(&b, " symbol_visibility=%s", rm.AnswerVisibilityProfile.SymbolVisibility)
	}
	if rm.ChangeImpactProfile != nil && rm.ChangeImpactProfile.IsChangeImpact {
		fmt.Fprintf(&b, " change_impact=%s", rm.ChangeImpactProfile.Target)
	}
	if rm.FieldValueProfile != nil && rm.FieldValueProfile.Active() {
		fmt.Fprintf(&b, " field_value=%s=%s", rm.FieldValueProfile.Target, rm.FieldValueProfile.Literal)
	}
	if rm.AnswerExclusionPolicy != nil && rm.AnswerExclusionPolicy.Active() {
		roles := make([]string, 0, len(rm.AnswerExclusionPolicy.ExcludedCandidateRoles))
		for _, role := range rm.AnswerExclusionPolicy.ExcludedCandidateRoles {
			roles = append(roles, string(role))
		}
		fmt.Fprintf(&b, " excluded_roles=%s", strings.Join(roles, ","))
	}
	if rm.AnswerRoleProfile != nil && rm.AnswerRoleProfile.Active() {
		roles := make([]string, 0, len(rm.AnswerRoleProfile.RequiredCandidateRoles))
		for _, role := range rm.AnswerRoleProfile.RequiredCandidateRoles {
			roles = append(roles, string(role))
		}
		fmt.Fprintf(&b, " required_roles=%s", strings.Join(roles, ","))
	}
	if rm.ErrorGranularityProfile != nil && rm.ErrorGranularityProfile.Active() {
		b.WriteString(" error_granularity=true")
		if len(rm.ErrorGranularityProfile.RequestedVerdictOptions) > 0 {
			options := make([]string, 0, len(rm.ErrorGranularityProfile.RequestedVerdictOptions))
			for _, option := range rm.ErrorGranularityProfile.RequestedVerdictOptions {
				options = append(options, string(option))
			}
			fmt.Fprintf(&b, " error_options=%s", strings.Join(options, ","))
		}
	}

	// Normalization delta — only fields where raw ≠ canonical get
	// listed, so a clean classification stays silent.
	deltas := collectNormalizationDeltas(raw, rm)
	if len(deltas) > 0 {
		b.WriteString(" | normalized: ")
		b.WriteString(strings.Join(deltas, ", "))
	}

	if len(val.Warnings) > 0 {
		b.WriteString(" | warn: ")
		b.WriteString(strings.Join(val.Warnings, "; "))
	}

	return b.String()
}

// collectNormalizationDeltas returns one "field raw→canonical" entry
// for every enum field the normalizer coerced. Unknown-collapse cases
// (LLM emitted "banana", normalizer returned "unknown") are listed
// explicitly so they surface in the trace — those are the prompts
// where the skill guidance missed. Empty-input cases stay silent:
// the field was simply absent, not coerced.
func collectNormalizationDeltas(raw emitAnalysisParams, rm types.RequestModel) []string {
	var deltas []string
	check := func(field, before, after string) {
		before = strings.TrimSpace(before)
		if before == "" {
			return
		}
		if !strings.EqualFold(before, after) {
			deltas = append(deltas, fmt.Sprintf("%s %q→%q", field, before, after))
		}
	}
	check("intent", raw.Intent, string(rm.Intent))
	check("scenario", raw.Scenario, string(rm.Scenario))
	check("complexity", raw.Complexity, string(rm.Complexity))
	check("question_kind", raw.QuestionKind, rm.AnalyzerHints.Kind)
	return deltas
}

// normalizeIntent coerces LLM-emitted intent strings to the typed
// v3 enum. Unknown values collapse to IntentUnknown.
func normalizeIntent(s string) types.Intent {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "explain":
		return types.IntentExplain
	case "root_cause", "rootcause", "root-cause":
		return types.IntentRootCause
	case "trace":
		return types.IntentTrace
	case "enumerate", "enumeration":
		return types.IntentEnumerate
	case "config_query", "config-query", "config":
		return types.IntentConfigQuery
	case "return_value", "return-value", "return":
		return types.IntentReturnValue
	}
	return types.IntentUnknown
}

// normalizeScenario coerces LLM-emitted scenario strings to the
// typed v3 Scenario enum. Unknown values collapse to ScenarioGeneric
// so the compiler's generic template is always a valid fallback.
func normalizeScenario(s string) types.Scenario {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "architecture_explain", "explain":
		return types.ScenarioArchitectureExplain
	case "root_cause", "rootcause":
		return types.ScenarioRootCause
	case "config_trace", "config":
		return types.ScenarioConfigTrace
	case "performance_bottleneck", "performance":
		return types.ScenarioPerformanceBottleneck
	}
	return types.ScenarioGeneric
}

// normalizeComplexity maps LLM-emitted complexity strings to the
// typed v3 Complexity enum. Unknown values default to moderate.
func normalizeComplexity(s string) types.Complexity {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "simple":
		return types.ComplexitySimple
	case "complex":
		return types.ComplexityComplex
	}
	return types.ComplexityModerate
}

// normalizeQuestionKind keeps the legacy ERM enum strings stable
// across the v3 migration so downstream ERM predicate whitelisting
// does not need to change. Unknown values fall back to "unknown".
//
// Delegates to types.NormalizeRequirementKind so the synonym table
// lives next to the typed enum — historically this function had its
// own switch table that drifted apart from the ERM consumer side.
func normalizeQuestionKind(s string) string {
	k := types.NormalizeRequirementKind(s)
	if k == types.ReqUnknown {
		return "unknown"
	}
	return string(k)
}

// countPrescanRounds derives the number of pre-scan rounds from
// the seen-blob. `MutableState.AppendPrescanSummary` appends each
// summary followed by a `\n`, so the number of newlines equals the
// round count. This lets emit_analysis.Execute report the full
// quality-probe triple (keyword hits / entity hits / rounds) in
// one call without a separate counter field on Mutable.
//
// Returns 0 when the blob is empty (no pre-scan fired) or the
// caller forgot the trailing newline.
func countPrescanRounds(seenBlob string) int {
	if seenBlob == "" {
		return 0
	}
	return strings.Count(seenBlob, "\n")
}

// trimStringSlice drops empty/whitespace-only entries and trims
// each remaining element. Returns nil for an empty input so the
// field stays omitempty-friendly.
func trimStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// subTopicPollutionMarkers are substrings that, if found in a
// sub-topic summary, mean the LLM copied a chunk of the REPL
// conversation prefix verbatim instead of producing a genuine
// one-line synopsis. Matching is case-sensitive — the markers are
// structural, not prose — and exact (no regex) so the check stays
// cheap and predictable.
var subTopicPollutionMarkers = []string{
	"## Prior conversation",
	"## Current request",
	"### Recent conversation",
	"(previous attempt ended in error",
}

// sanitizeSubTopics scrubs every sub-topic summary through:
//
//  1. types.StripConversationPrefix — strips a full REPL prefix
//     when the LLM pasted the whole effective-request wrapper.
//  2. Pollution-marker check on what remains. When any marker is
//     still present, the summary is considered unusable prose and
//     is replaced by a comma-joined entity list if available, else
//     emptied. An empty summary causes compiler.expandEvidenceNodes
//     and the renderer to fall back to the node ID — ugly, but less
//     misleading than rendering "## Prior conversation ..." as a
//     sub-topic title.
//  3. Whitespace + length cap at subTopicSummaryMaxChars so a
//     pasted paragraph cannot dominate the live task-row width.
//
// The original slice is not mutated — callers get a freshly-allocated
// sub-topic list with the cleaned Summary fields.
func sanitizeSubTopics(in []types.SubTopic) []types.SubTopic {
	if len(in) == 0 {
		return in
	}
	out := make([]types.SubTopic, len(in))
	for i, st := range in {
		out[i] = types.SubTopic{
			Summary:  sanitizeSubTopicSummary(st.Summary, st.Entities),
			Entities: st.Entities,
		}
	}
	return out
}

func sanitizeSubTopicSummary(summary string, entities []string) string {
	s := strings.TrimSpace(summary)
	if s == "" {
		return ""
	}
	// Tier 1: strip a well-formed REPL prefix.
	s = strings.TrimSpace(types.StripConversationPrefix(s))
	// Tier 2: if any pollution marker survives, the summary is
	// structurally bad — prefer the entity list as a fallback label.
	for _, m := range subTopicPollutionMarkers {
		if strings.Contains(s, m) {
			return strings.Join(nonEmptyStrings(entities), " / ")
		}
	}
	// Tier 3: length cap. Long paragraphs force the renderer to
	// truncate every frame and make the task row unreadable.
	if len(s) > subTopicSummaryMaxChars {
		s = strings.TrimSpace(s[:subTopicSummaryMaxChars]) + "…"
	}
	return s
}

// subTopicSummaryMaxChars is chosen so the sub-topic label stays
// readable in a typical 80-120 col terminal after the "[topic N] "
// prefix. Anything longer is almost certainly a pasted paragraph.
const subTopicSummaryMaxChars = 120

func nonEmptyStrings(xs []string) []string {
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		if t := strings.TrimSpace(x); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// validateAndBuildRequiredFileHints converts the LLM-emitted
// required_files array into the typed []types.RequiredFileHint
// slice the IR consumes. Validation is per-item, fail-soft: invalid
// entries are dropped silently with a warning appended to the
// validation result; the rest pass through. This keeps the
// emit_analysis call from failing hard on a single malformed entry
// while still surfacing the issue to the operator.
//
// Per-entry rules:
//   - path must be non-empty after TrimSpace + ToSlash + TrimPrefix("./");
//     empty paths are dropped
//   - confidence must be in [0,1]; out-of-range entries are clamped
//     and a warning is recorded; NaN entries are dropped
//   - rationale is capped at requiredFileHintRationaleMaxChars to
//     keep the prompt budget bounded; over-cap entries are truncated
//     with an ellipsis
//   - confidence < 0.5 entries are kept (the threshold gating happens
//     at the consumer, not here — preserves "I'm unsure" signal)
//   - hard cap: at most requiredFileHintsMax entries; excess is dropped
//
// Cross-language: paths are POSIX-canonicalised (Windows backslash
// converted to forward slash) so the same hint set works regardless
// of the model's path-style preference.
func validateAndBuildRequiredFileHints(in []emitRequiredFileParam, val *analysisValidationResult) []types.RequiredFileHint {
	if len(in) == 0 {
		return nil
	}
	out := make([]types.RequiredFileHint, 0, len(in))
	dropped := 0
	clamped := 0
	for _, e := range in {
		if len(out) >= requiredFileHintsMax {
			dropped++
			continue
		}
		canon := canonicalRequiredFilePath(e.Path)
		if canon == "" {
			dropped++
			continue
		}
		conf := e.Confidence
		if conf != conf { // NaN
			dropped++
			continue
		}
		if conf < 0 {
			conf = 0
			clamped++
		}
		if conf > 1 {
			conf = 1
			clamped++
		}
		rationale := strings.TrimSpace(e.Rationale)
		if len(rationale) > requiredFileHintRationaleMaxChars {
			rationale = rationale[:requiredFileHintRationaleMaxChars-1] + "…"
		}
		out = append(out, types.RequiredFileHint{
			Path:       canon,
			Confidence: conf,
			Rationale:  rationale,
		})
	}
	if val != nil {
		if dropped > 0 {
			val.Warnings = append(val.Warnings,
				fmt.Sprintf("required_files: %d entries dropped (empty path / NaN confidence / over cap of %d)", dropped, requiredFileHintsMax))
		}
		if clamped > 0 {
			val.Warnings = append(val.Warnings,
				fmt.Sprintf("required_files: %d entries had confidence clamped into [0,1]", clamped))
		}
	}
	return out
}

// canonicalRequiredFilePath normalises an LLM-emitted file path to
// the repo-relative POSIX form the explorer's path lookup uses. Same
// rules as canonicalExplorerPath: backslash → forward slash, trim
// "./" prefix, trim whitespace.
func canonicalRequiredFilePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimPrefix(p, "./")
	if p == "" || p == "." {
		return ""
	}
	return p
}

const (
	// requiredFileHintsMax caps the number of analyzer file hints
	// the system will accept per emit_analysis call. Far above any
	// realistic LLM output (rare to see >10) but defends against
	// pathological "list every file" emissions that would bloat the
	// IR and downstream prompt budgets.
	requiredFileHintsMax = 20
	// requiredFileHintRationaleMaxChars caps the rationale length so
	// a populated 20-entry hint list contributes ~4 KB at most when
	// echoed back through the retry hint composer.
	requiredFileHintRationaleMaxChars = 200
	// irrelevantFilesMax caps the L4 negative-channel list. Smaller
	// than required_files because declaring a file irrelevant is
	// cheaper for the LLM than recommending one (no rationale
	// required) and abuse here is more impactful — a wrong
	// declaration silently hides answer-bearing files.
	irrelevantFilesMax = 10
)

// validateAndBuildIrrelevantFiles canonicalises and caps the
// LLM-emitted irrelevant_files array. Same rules as the per-entry
// path canonicaliser used for required_files (forward-slash, strip
// leading "./", trim whitespace). Empty / duplicate / over-cap
// entries are dropped silently with a single aggregated warning.
//
// Cross-language: paths are POSIX-canonical so the same hint set
// works regardless of the model's path-style preference.
//
// L4 (2026-05-10).
func validateAndBuildIrrelevantFiles(in []string, val *analysisValidationResult) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]bool, len(in))
	dropped := 0
	for _, p := range in {
		if len(out) >= irrelevantFilesMax {
			dropped++
			continue
		}
		canon := canonicalRequiredFilePath(p)
		if canon == "" {
			dropped++
			continue
		}
		if seen[canon] {
			dropped++
			continue
		}
		seen[canon] = true
		out = append(out, canon)
	}
	if val != nil && dropped > 0 {
		val.Warnings = append(val.Warnings,
			fmt.Sprintf("irrelevant_files: %d entries dropped (empty / duplicate / over cap of %d)", dropped, irrelevantFilesMax))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
