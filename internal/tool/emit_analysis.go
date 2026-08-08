package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/hanchaoqun/codrax/internal/analysis/prescan"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/skill"
	repomap "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
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
	Intent             string                          `json:"intent"`
	Scenario           string                          `json:"scenario"`
	Complexity         string                          `json:"complexity"`
	Keywords           []string                        `json:"keywords"`
	Entities           []string                        `json:"entities"`
	QuestionKind       string                          `json:"question_kind"`
	Language           string                          `json:"language,omitempty"`
	SubTopics          []emitAnalysisSubTopic          `json:"sub_topics,omitempty"`
	AnswerSubject      *emitAnswerSubjectParam         `json:"answer_subject,omitempty"`
	ExactTargets       []string                        `json:"exact_targets,omitempty"`
	ExactContextTerms  []string                        `json:"exact_context_terms,omitempty"`
	ExactContextRoles  []string                        `json:"exact_context_roles,omitempty"`
	CallChainEndpoints *types.CallChainEndpointProfile `json:"call_chain_endpoints"`

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
	ReferencedArtifactLines      []emitArtifactLineRefParam             `json:"referenced_artifact_lines,omitempty"`
	SourceScopeProfile           *emitSourceScopeProfileParam           `json:"source_scope_profile,omitempty"`
	AnswerVisibilityProfile      *emitAnswerVisibilityProfileParam      `json:"answer_visibility_profile,omitempty"`
	SourceInventoryProfile       *emitSourceInventoryProfileParam       `json:"source_inventory_profile,omitempty"`
	ChangeImpactProfile          *emitChangeImpactProfileParam          `json:"change_impact_profile,omitempty"`
	FieldValueProfile            *emitFieldValueProfileParam            `json:"field_value_profile,omitempty"`
	RuntimeArtifactValueProfile  *emitRuntimeArtifactValueProfileParam  `json:"artifact_value_profile,omitempty"`
	RuntimeArtifactScopeProfile  *emitRuntimeArtifactScopeProfileParam  `json:"runtime_artifact_scope_profile"`
	RuntimeTargetProfile         *emitRuntimeTargetProfileParam         `json:"runtime_target_profile"`
	RuntimeQuestionProfile       *emitRuntimeQuestionProfileParam       `json:"runtime_question_profile"`
	HistorySelectionProfile      *emitHistorySelectionProfileParam      `json:"history_selection_profile"`
	RuntimeTargets               []emitRuntimeTargetParam               `json:"runtime_targets,omitempty"`
	AnswerExclusionPolicy        *emitAnswerExclusionPolicyParam        `json:"answer_exclusion_policy,omitempty"`
	AnswerRoleProfile            *emitAnswerRoleProfileParam            `json:"answer_role_profile,omitempty"`
	ErrorGranularityProfile      *emitErrorGranularityProfileParam      `json:"error_granularity_profile,omitempty"`
	RequestedAnswerDimensions    *emitRequestedAnswerDimensionsParam    `json:"requested_answer_dimensions,omitempty"`
	CurrentSourceExplanation     *emitCurrentSourceExplanationParam     `json:"current_source_explanation_profile,omitempty"`
	ExternalObservationPolicy    *emitExternalObservationPolicyParam    `json:"external_observation_policy,omitempty"`
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

type emitAnalysisSubTopic struct {
	Summary  string   `json:"summary"`
	Entities []string `json:"entities,omitempty"`
}

func emitAnalysisSubTopics(in []emitAnalysisSubTopic) []types.SubTopic {
	if len(in) == 0 {
		return nil
	}
	out := make([]types.SubTopic, 0, len(in))
	for _, st := range in {
		out = append(out, types.SubTopic{
			Summary:  st.Summary,
			Entities: append([]string(nil), st.Entities...),
		})
	}
	return out
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
	HasPerMemberTable     *bool `json:"has_per_member_table"`
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

type emitArtifactLineRefParam struct {
	Source    string `json:"source"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line,omitempty"`
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
	SourceQuotes                []string `json:"source_quotes,omitempty"`
	Confidence                  *float64 `json:"confidence"`
	Rationale                   string   `json:"rationale,omitempty"`
}

type emitAnswerVisibilityProfileParam struct {
	SymbolVisibility string   `json:"symbol_visibility"`
	SourceQuotes     []string `json:"source_quotes,omitempty"`
	Confidence       *float64 `json:"confidence"`
	Rationale        string   `json:"rationale,omitempty"`
}

type emitSourceInventoryProfileParam struct {
	IsSourceInventory *bool    `json:"is_source_inventory"`
	TargetRoles       []string `json:"target_roles,omitempty"`
	TypeUnderlying    string   `json:"type_underlying,omitempty"`
	RequiresConstSet  *bool    `json:"requires_const_set,omitempty"`
	RequestedFields   []string `json:"requested_fields,omitempty"`
	SourceQuotes      []string `json:"source_quotes,omitempty"`
	Confidence        *float64 `json:"confidence"`
	Rationale         string   `json:"rationale,omitempty"`
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

type emitRuntimeArtifactValueProfileParam struct {
	IsArtifactValueLookup *bool    `json:"is_artifact_value_lookup"`
	Target                string   `json:"target,omitempty"`
	Value                 string   `json:"value,omitempty"`
	Unit                  string   `json:"unit,omitempty"`
	LiteralKind           string   `json:"literal_kind,omitempty"`
	ArtifactRefs          []string `json:"artifact_refs,omitempty"`
	ObservationRefs       []string `json:"observation_refs,omitempty"`
	Confidence            *float64 `json:"confidence"`
	Rationale             string   `json:"rationale,omitempty"`
}

type emitRuntimeArtifactScopeProfileParam struct {
	RequestedScope string   `json:"requested_scope"`
	TimeStart      *float64 `json:"time_start,omitempty"`
	TimeEnd        *float64 `json:"time_end,omitempty"`
	SourceQuote    string   `json:"source_quote,omitempty"`
	Confidence     *float64 `json:"confidence"`
	Rationale      string   `json:"rationale,omitempty"`
}

type emitRuntimeTargetProfileParam struct {
	Declaration string   `json:"declaration"`
	SourceQuote string   `json:"source_quote,omitempty"`
	Confidence  *float64 `json:"confidence"`
	Rationale   string   `json:"rationale,omitempty"`
}

type emitRuntimeQuestionProfileParam struct {
	Scope        string   `json:"scope"`
	FactFamilies []string `json:"fact_families,omitempty"`
	SourceQuote  string   `json:"source_quote,omitempty"`
	Confidence   *float64 `json:"confidence"`
	Rationale    string   `json:"rationale,omitempty"`
}

type emitHistorySelectionProfileParam struct {
	Mode        string   `json:"mode"`
	ItemKind    string   `json:"item_kind"`
	Count       int      `json:"count,omitempty"`
	SourceQuote string   `json:"source_quote,omitempty"`
	Confidence  *float64 `json:"confidence"`
	Rationale   string   `json:"rationale,omitempty"`
}

type emitRuntimeTargetParam struct {
	Kind        string   `json:"kind,omitempty"`
	PID         *int     `json:"pid,omitempty"`
	Thread      string   `json:"thread,omitempty"`
	Source      string   `json:"source,omitempty"`
	Confidence  *float64 `json:"confidence,omitempty"`
	Description string   `json:"description,omitempty"`
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

type emitRequestedAnswerDimensionsParam struct {
	IsDimensionedAnswer *bool                               `json:"is_dimensioned_answer"`
	Dimensions          []emitRequestedAnswerDimensionParam `json:"dimensions,omitempty"`
	Confidence          *float64                            `json:"confidence"`
	Rationale           string                              `json:"rationale,omitempty"`
}

type emitRequestedAnswerDimensionParam struct {
	Label       string `json:"label"`
	Role        string `json:"role,omitempty"`
	SourceQuote string `json:"source_quote,omitempty"`
	Required    *bool  `json:"required,omitempty"`
	Index       int    `json:"index,omitempty"`
}

type emitCurrentSourceExplanationParam struct {
	IsCurrentSourceExplanationRequested *bool    `json:"is_current_source_explanation_requested"`
	Modes                               []string `json:"modes,omitempty"`
	SourceQuotes                        []string `json:"source_quotes,omitempty"`
	TargetTerms                         []string `json:"target_terms,omitempty"`
	Confidence                          *float64 `json:"confidence"`
	Rationale                           string   `json:"rationale,omitempty"`
}

type emitExternalObservationPolicyParam struct {
	CurrentSourceMode           string   `json:"current_source_mode,omitempty"`
	ExclusionKind               string   `json:"exclusion_kind,omitempty"`
	ArtifactCitationMode        string   `json:"artifact_citation_mode,omitempty"`
	CurrentSourceExclusionQuote string   `json:"current_source_exclusion_quote,omitempty"`
	ArtifactCitationQuotes      []string `json:"artifact_citation_quotes,omitempty"`
	// SourceQuotes is a pre-role-split compatibility carrier. It remains
	// decodable for persisted/direct callers, but parseExternalObservationPolicy
	// never accepts it as sufficient proof for a newly minted source exclusion.
	SourceQuotes []string `json:"source_quotes,omitempty"`
	Confidence   *float64 `json:"confidence"`
	Rationale    string   `json:"rationale,omitempty"`
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
	Kind     string `json:"kind"`
	Required *bool  `json:"required"`
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
				"description": "Optional exact user-asked targets, copied verbatim from the current request text. Use when the request mentions neighboring context items but asks about one specific key/path/symbol/literal. For a source-code call_chain with more than two symbol-like entities, this field is required and must contain the caller/source and callee/sink (intermediate/context symbols stay in entities). Every item must be explicitly present in the current request text before later steps use it.",
				"items":       map[string]string{"type": "string"},
			},
			"call_chain_endpoints": map[string]any{
				"type":        "object",
				"description": types.CallChainEndpointProfileTeaching + " This is the ONLY field whose exact endpoint order is directional; entities and exact_targets remain unordered identity sets. For all other shapes emit source=\"\", sink=\"\", sink_mode=exact.",
				"properties": map[string]any{
					"source":    map[string]string{"type": "string", "description": "Exact caller/start copied from the current request in exact/discover; empty in discover_path and when not applicable."},
					"sink":      map[string]string{"type": "string", "description": "Exact current-request destination in exact; empty in discover/discover_path and when not applicable."},
					"sink_mode": map[string]any{"type": "string", "enum": types.CallChainSinkResolutionModeValues(), "description": "exact=both identities named; discover=exact source with runtime destination to find; discover_path=both role-bound endpoint identities to find."},
				},
				"required":             []string{"source", "sink", "sink_mode"},
				"additionalProperties": false,
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
					"is_role_locate_lookup":   map[string]any{"type": "boolean", "description": "True only for scalar lookups where the request names a clue / output / context entity, but the answer is one DIFFERENT literal that plays a role relative to that clue (line number / event row, entry function, defining file, config key, route, owner symbol, etc.). For set-valued tables such as every package -> entry function, set this false and use category/relational enumeration predicates. Implies is_scalar_answer=true and requires answer_subject.kind."},
					"is_count_question":       map[string]any{"type": "boolean", "description": "True when the answer is a single number that must be computed by aggregating values across multiple source units — e.g. counting items, summing lines of code, summing file sizes, totalling bytes across a directory tree. Implies is_scalar_answer. Set false when the user asks to list/enumerate members and include counts per list/group; those counts are attributes of an enumeration, not a scalar count question. Also set false when the answer is a number that already exists as a single source-code literal (a const declaration, a default value, an enum ordinal) — that case is is_scalar_answer=true without is_count_question."},
					"is_cross_component":      map[string]any{"type": "boolean", "description": "True if the question genuinely spans multiple distinct components / subsystems / independently-answerable code regions. Leave false for a single named target that merely needs nearby context, precedence layers, or override stages, and also leave false for one ordered source-to-sink call/flow trace even when that chain crosses files or packages."},
					"is_relational_lookup":    map[string]any{"type": "boolean", "description": "True if filtering/selecting/counting members of source set X by a relationship to target Y, such as functions that return Z, callers that reach service Y, or role/category members that can invoke capability Y. Pair concrete member-name relation answers with is_category_enumeration=true; pair pure relation counts with is_count_question=true."},
					"is_category_enumeration": map[string]any{"type": "boolean", "description": "True if asking 'what kinds / types / categories of X exist' or asking for concrete members of a closed/structural set, including relation-qualified members when is_relational_lookup=true."},
					"has_per_member_table":    map[string]any{"type": "boolean", "description": "True when the request demands a per-member table / per-member rows over a bounded set (\"每个 X 一行\" / \"one row per X from A to B\"), even when the overall intent is an explanation. The member set becomes a completion obligation."},
					"is_history_lookup":       map[string]any{"type": "boolean", "description": "True when the authoritative evidence source is repository history / authorship metadata (git log / blame / commit history), not a repo file:line. This is an evidence-source flag, not an answer-shape flag: pair it with is_scalar_answer=true only when the principal answer is one literal such as a commit hash/date/author/count; leave is_scalar_answer=false for feature summaries, recent-commit lists, commit comparisons, locating the changed code, explaining the code behind a commit, drawing logic/sequence diagrams from a commit, or history-backed diagnostics."},
					"is_diagnostic_question":  map[string]any{"type": "boolean", "description": "True when the current request asks to diagnose a failure, regression, runtime symptom, observed bad behaviour, or whether a similar problem still exists, and expects cause / current-risk / remediation analysis. Applies with or without an attached runtime artifact. False for ordinary architecture tours, code walkthroughs, or log/trace parser mechanism questions. This is the primary diagnostic routing predicate; the system aligns diagnostic_profile.is_diagnostic to it unless independent current-risk / historical-regression signals are present."},
				},
				"required": []string{"is_scalar_answer", "is_role_locate_lookup", "is_count_question", "is_cross_component", "is_relational_lookup", "is_category_enumeration", "is_history_lookup", "is_diagnostic_question", "has_per_member_table"},
			},
			"diagnostic_profile": map[string]any{
				"type":        "object",
				"description": "Required typed diagnostic-intent profile. Use this as a second safety lane for diagnosis, current-risk, historical-regression, and current-version verification questions. Every boolean field should be emitted true OR false explicitly. Do not infer from raw artifact presence alone. The system tolerates mirror drift by aligning is_diagnostic to predicates.is_diagnostic_question unless current_risk or historical_regression is true; if this mirror profile is accidentally omitted, runtime defaults it conservatively from predicates without inventing current-risk/regression/current-version flags.",
				"properties": map[string]any{
					"is_diagnostic":         map[string]any{"type": "boolean", "description": "Profile-level mirror of predicates.is_diagnostic_question. Set both consistently; the system can auto-align this mirror when it drifts."},
					"current_risk":          map[string]any{"type": "boolean", "description": "True when the current request asks whether a known or observed issue can still happen in the current checkout. Set false when external_observation_policy.current_source_mode=exclude, because the user explicitly forbids current checkout/source verification."},
					"historical_regression": map[string]any{"type": "boolean", "description": "True when the request compares a historical observed symptom against the current version."},
					"current_version_check": map[string]any{"type": "boolean", "description": "True only for diagnostic current-status questions where the answer must verify current code separately from historical artifact observations; false for ordinary exact/config/value/location lookups and false when external_observation_policy.current_source_mode=exclude."},
					"observation_summary":   map[string]any{"type": "string", "description": "Optional compact summary of the historical or user-described symptom. Fill this when no log/trace is attached or when the request uses prior conversation to refer to the issue being checked."},
					"confidence":            map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0, "description": "Your confidence in this diagnostic profile in [0,1]."},
				},
				"required": []string{"is_diagnostic", "current_risk", "historical_regression", "current_version_check", "confidence"},
			},
			"referenced_artifact_lines": map[string]any{
				"type":        "array",
				"description": "Artifact-local line coordinates the QUESTION itself references (for example 'log line 3' / '日志第 3 行' / 'trace 第 5-6 行'). Declare one entry per referenced span so the answer keeps the user's coordinate anchored. Only for attached log/trace lines the user pointed at — not for stack-frame file:line tokens inside the artifact text.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"source":     map[string]any{"type": "string", "enum": []string{"log", "trace"}, "description": "Which attached artifact the span addresses."},
						"start_line": map[string]any{"type": "integer", "minimum": 1, "description": "1-based artifact-local line the user referenced."},
						"end_line":   map[string]any{"type": "integer", "minimum": 1, "description": "Inclusive span end; omit for a single line."},
					},
					"required": []string{"source", "start_line"},
				},
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
				"description": "Optional typed REPOSITORY PATH-scope intent. Emit only when the current request explicitly makes production, tests, docs, fixtures, examples, or all repo material the principal answer scope. Never use an attached log/trace/artifact phrase as this profile's source quote; artifact range belongs in runtime_artifact_scope_profile. This profile cannot substitute for current_source_explanation_profile in a mixed artifact+current-source request.",
				"properties": map[string]any{
					"requested_scope":                map[string]any{"type": "string", "enum": sourceScopeValues(), "description": "production, test, documentation, auxiliary, all, or unknown."},
					"include_auxiliary_as_principal": map[string]any{"type": "boolean", "description": "True only when test/docs/fixture/example files may be principal answer members rather than supporting context."},
					"source_quotes":                  map[string]any{"type": "array", "items": map[string]string{"type": "string"}, "description": "Verbatim current-request phrase(s) that state the requested path/source scope. Leave empty when the scope is inferred only from repository layout or model judgment."},
					"confidence":                     map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0, "description": "Your confidence in this source scope in [0,1]."},
					"rationale":                      map[string]any{"type": "string", "description": "Short audit rationale for the selected scope."},
				},
				"required": []string{"requested_scope", "confidence"},
			},
			"answer_visibility_profile": map[string]any{
				"type":        "object",
				"description": "Typed language-level symbol-visibility intent for code-symbol inventory/list/count/API-surface questions. Emit only when the current request explicitly expresses public/exported/all/private/internal visibility. Importance, centrality, runtime use, or source-path scope such as main/primary/core/production implementations does not mean public_exported; use source_scope_profile for an explicit path boundary and otherwise omit this profile. This controls graph Exported filtering downstream and is separate from source_scope_profile path filtering.",
				"properties": map[string]any{
					"symbol_visibility": map[string]any{"type": "string", "enum": answerSymbolVisibilityValues(), "description": "public_exported, all, private_only, or unknown."},
					"source_quotes":     map[string]any{"type": "array", "items": map[string]string{"type": "string"}, "description": "Verbatim current-request phrase(s) that state the symbol visibility scope."},
					"confidence":        map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0, "description": "Your confidence in this symbol visibility scope in [0,1]."},
					"rationale":         map[string]any{"type": "string", "description": "Short audit rationale for why this visibility scope applies."},
				},
				"required": []string{"symbol_visibility", "confidence"},
			},
			"source_inventory_profile": map[string]any{
				"type":        "object",
				"description": "Optional typed source-inventory intent. Emit when the current request asks for bounded structural source members such as public functions, public types, constants, enum-like types, fields, or methods under a path/package/file scope. This is the user's requested membership shape, not evidence. Do not emit it for conceptual stages, phases, steps, modes, actors, or components in an architecture/mechanism explanation even when code represents them as enums, types, or constants. In particular, an explain request with a required requested_answer_dimensions role=stage_or_workflow uses source declarations only as supporting evidence and must not also emit source_inventory_profile merely because the stages are constants or types. Do not emit this for runtime artifact identifiers such as trace/log inode, dev, entry_name, pid, timestamp, line, event, span, thread, or trace-local file-like labels; keep those in external_observation_policy / runtime artifact lanes. Downstream may use parser/repo-map facts to recover missing members, but it must keep model summaries as enrichment.",
				"properties": map[string]any{
					"is_source_inventory": map[string]any{"type": "boolean", "description": "True only when the answer's principal payload is a bounded inventory of source-code members."},
					"target_roles":        map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": answerCandidateRoleValues()}, "description": "Principal source-member roles requested by the user, such as function, method, type, constant, variable, or field. This carries the requested answer shape and accepts the full role enum; the source-inventory navigation lens later enumerates members for the structural-carrier subset of these roles."},
					"type_underlying":     map[string]any{"type": "string", "enum": sourceInventoryTypeUnderlyingValues(), "description": "Optional structural facet for type inventories. Use string for requests like Go `type X string`; use unknown when no underlying type facet is requested."},
					"requires_const_set":  map[string]any{"type": "boolean", "description": "True when a type inventory requires an associated const/enum declaration set, such as string enum types. False or omit for ordinary type inventories."},
					"requested_fields":    map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": sourceInventoryRequestedFieldValues()}, "description": "Display fields the user wants shown: name, location, summary, values, count, package, module, namespace. Package/module/namespace here are per-row display attributes such as Go package, Java/Kotlin/Cangjie package, Python module, C++ namespace, or ArkTS module labels; use target_roles only when the package/module/import itself is the principal inventory member. Do not put construct/member roles such as file, route, import_path, function, type, method, field, or constant here; put those in target_roles and preserve the user's construct wording in source_quotes. Do not include values unless the request asks for enum/member literal values."},
					"source_quotes":       map[string]any{"type": "array", "items": map[string]string{"type": "string"}, "description": "Verbatim current-request phrase(s) that state the inventory shape or structural facet."},
					"confidence":          map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0, "description": "Your confidence in this source-inventory profile in [0,1]."},
					"rationale":           map[string]any{"type": "string", "description": "Short audit rationale for why this inventory profile applies."},
				},
				"required": []string{"is_source_inventory", "confidence"},
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
				"description": "Optional typed profile for exact current-request field/member/config literal lookups. Use when the CURRENT request itself asks how many sites, which sites, or what scalar answer depends on a named source/config target being set/equal to a specific literal value. Do not emit for unrelated counts, ordinary mechanism explanations, or values observed in attached logs/traces/perf artifacts; use artifact_value_profile for artifact-derived values.",
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
			"artifact_value_profile": map[string]any{
				"type":        "object",
				"description": "Optional typed profile for exact values observed in attached logs, traces, perf artifacts, or trace_query/log-triage facts. Use this when the value is supported by runtime artifact observations rather than by a verbatim current-request field=value phrase. This is a soft artifact lane; later stages must verify against typed runtime observations before treating the value as factual.",
				"properties": map[string]any{
					"is_artifact_value_lookup": map[string]any{"type": "boolean", "description": "True only when the scalar/key-value target is an exact value observed in a runtime artifact."},
					"target":                   map[string]any{"type": "string", "description": "Runtime-artifact value target, such as GC span duration, frame jank duration, binder latency, thread state, or trace line number. It does not need to be an owner-qualified source member."},
					"value":                    map[string]any{"type": "string", "description": "Exact observed value from the runtime artifact, copied without paraphrase."},
					"unit":                     map[string]any{"type": "string", "description": "Optional unit such as ms, s, %, kHz, line, count, or state."},
					"literal_kind":             map[string]any{"type": "string", "enum": fieldValueLiteralKindValues(), "description": "Kind of observed value when known."},
					"artifact_refs":            map[string]any{"type": "array", "items": map[string]string{"type": "string"}, "description": "Runtime artifact ids, paths, or typed source refs supporting the value. Prefer stable refs exposed by log/trace triage or trace_query."},
					"observation_refs":         map[string]any{"type": "array", "items": map[string]string{"type": "string"}, "description": "Typed observation ids supporting this value when available."},
					"confidence":               map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0, "description": "Your confidence in this artifact value profile in [0,1]."},
					"rationale":                map[string]any{"type": "string", "description": "Short audit rationale for why the value is artifact-derived."},
				},
				"required": []string{"is_artifact_value_lookup", "confidence"},
			},
			"runtime_artifact_scope_profile": map[string]any{
				"type":        "object",
				"description": "Required user-scope authority for runtime artifacts. This is NOT a trace_query/exploration window. Use not_applicable when no runtime artifact is attached or referenced. For an attached artifact, use full_artifact when the current request asks about the supplied artifact without a narrower user boundary (for example 'this trace' / '这份 trace'); use explicit_time_window only when the current request itself states exact trace time bounds, copying them to time_start/time_end; use bounded_selector when the current request names a narrower artifact selector such as a frame/span/event but does not state exact time bounds; use unspecified when the current request's artifact scope cannot be determined. A model-chosen query window never changes this field.",
				"properties": map[string]any{
					"requested_scope": map[string]any{"type": "string", "enum": runtimeArtifactRequestedScopeValues(), "description": "not_applicable, full_artifact, explicit_time_window, bounded_selector, or unspecified."},
					"time_start":      map[string]any{"type": "number", "minimum": 0.0, "description": "Exact user-stated trace start in seconds; required only for explicit_time_window."},
					"time_end":        map[string]any{"type": "number", "minimum": 0.0, "description": "Exact user-stated trace end in seconds; required only for explicit_time_window and greater than time_start."},
					"source_quote":    map[string]any{"type": "string", "description": "Verbatim current-request phrase that establishes full_artifact, explicit_time_window, or bounded_selector scope. Do not copy model/tool prose."},
					"confidence":      map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0, "description": "Classification confidence in [0,1]."},
					"rationale":       map[string]any{"type": "string", "description": "Short audit rationale."},
				},
				"required": []string{"requested_scope", "confidence"},
			},
			"runtime_target_profile": map[string]any{
				"type":        "object",
				"description": "Required typed declaration of whether the current runtime-artifact request names a concrete process/thread identity. This is independent of the artifact time/range. Use named_target only with an exact current-request source_quote and one or more matching runtime_targets; use no_named_target when the artifact question names no process/thread identity; use unspecified when genuinely unclear; use not_applicable when no runtime artifact is attached or referenced. This declaration prevents ordinary analyzer entities or model exploration cursors from silently becoming user target authority.",
				"properties": map[string]any{
					"declaration":  map[string]any{"type": "string", "enum": runtimeTargetDeclarationValues(), "description": "not_applicable, no_named_target, named_target, or unspecified."},
					"source_quote": map[string]any{"type": "string", "description": "For named_target, an exact verbatim current-request phrase containing the named process/thread identity."},
					"confidence":   map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0, "description": "Classification confidence in [0,1]."},
					"rationale":    map[string]any{"type": "string", "description": "Short audit rationale."},
				},
				"required": []string{"declaration", "confidence"},
			},
			"runtime_question_profile": map[string]any{
				"type":        "object",
				"description": "Required typed declaration of the answer breadth requested from a runtime artifact. This is independent of artifact range, target identity, and relation shape. Use bounded_fact_set for a finite set of observed state/value/count/time/recorded-reason facts or directly named relation fields such as one peer, transaction id, or waker, without a requested causal report; causal_diagnosis for why/root-cause/jank diagnosis; relation_analysis for broader caller/wakeup/IPC/dependency path or topology analysis; system_overview for a broad hotspot/health/summary report; unspecified only when genuinely unclear; not_applicable outside runtime artifacts. A kernel-recorded reason or direct relation field remains a bounded observed fact unless the request asks to expand or diagnose it. Downstream uses this enum instead of unstable intent/scenario combinations and never scans request or answer prose.",
				"allOf": []any{
					map[string]any{
						"if": map[string]any{
							"properties": map[string]any{
								"scope": map[string]any{"enum": []string{string(types.RuntimeQuestionScopeBoundedFactSet)}},
							},
							"required": []string{"scope"},
						},
						"then": map[string]any{"required": []string{"fact_families"}},
						"else": map[string]any{
							"not": map[string]any{"required": []string{"fact_families"}},
						},
					},
				},
				"properties": map[string]any{
					"scope":         map[string]any{"type": "string", "enum": runtimeQuestionScopeValues(), "description": "not_applicable, bounded_fact_set, causal_diagnosis, relation_analysis, system_overview, or unspecified."},
					"fact_families": map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"type": "string", "enum": runtimeQuestionFactFamilyValues()}, "description": "Principal observed fact families requested by a bounded_fact_set. Use target_scheduler_state only for the target's scheduler-state presence/partition; target_wait_occurrences only for a requested count/list/total of scheduler wait intervals (never merely because a direct waker, wakeup event, or wakeup latency is requested); recorded_reason for a kernel/tool reason; occurrence_time; count_or_duration; relation_peer; transaction_id; direct_waker; resource_pressure; frequency_residency; or other_observed_value. Required and non-empty for bounded_fact_set; the schema forbids this field for every broader scope. These enums control only which exact fact cards may accompany the model answer; they never authorize a causal conclusion."},
					"source_quote":  map[string]any{"type": "string", "description": "Optional audit anchor for a concrete runtime scope. Prefer the shortest contiguous exact current-request phrase that expresses the requested facts, diagnosis, relation, or overview (for example, copy `卡顿原因`, not a paraphrase assembled from separated words). An empty or unanchored quote is dropped with a warning; scope/fact_families remain the typed contract."},
					"confidence":    map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0, "description": "Classification confidence in [0,1]."},
					"rationale":     map[string]any{"type": "string", "description": "Short audit rationale."},
				},
				"required": []string{"scope", "confidence"},
			},
			"history_selection_profile": map[string]any{
				"type":        "object",
				"description": "Required typed declaration of ordinal/cardinality selection over repository history. Use not_applicable when is_history_lookup=false. For history questions, use latest_one or earliest_one for one endpoint, recent_n or oldest_n for a user-requested ordered N, bounded_range for an explicit ref/date/range whose endpoints are not one ordinal, and unspecified only when the current request genuinely does not select an order. item_kind is commit, merge, non_merge, matching_commit, unspecified, or not_applicable. This is soft request guidance: downstream conjoins it with tool-carried order/filter fields and never scans request or answer prose.",
				"properties": map[string]any{
					"mode":         map[string]any{"type": "string", "enum": historySelectionModeValues(), "description": "not_applicable, latest_one, earliest_one, recent_n, oldest_n, bounded_range, or unspecified."},
					"item_kind":    map[string]any{"type": "string", "enum": historySelectionItemKindValues(), "description": "not_applicable, commit, merge, non_merge, matching_commit, or unspecified."},
					"count":        map[string]any{"type": "integer", "minimum": 1, "description": "Required only for recent_n/oldest_n; the requested number of principal history rows."},
					"source_quote": map[string]any{"type": "string", "description": "For every concrete history selection, one exact current-request phrase that states the order/range selection."},
					"confidence":   map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0, "description": "Classification confidence in [0,1]."},
					"rationale":    map[string]any{"type": "string", "description": "Short audit rationale."},
				},
				"required": []string{"mode", "item_kind", "confidence"},
			},
			"runtime_targets": map[string]any{
				"type":        "array",
				"description": "Optional typed runtime-artifact target list. Emit only when the current request explicitly identifies trace/log/perf targets as structured process IDs, thread IDs, or concrete thread labels. This is the only lane downstream trace tools may use to preserve omitted pid/thread filters; do not put timestamps, file paths, span names, generic entities, or guessed values here.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"kind":        map[string]any{"type": "string", "enum": runtimeTargetKindValues(), "description": "process for process-level pid targets; thread for thread id or concrete thread label targets."},
						"pid":         map[string]any{"type": "integer", "minimum": 1, "description": "Runtime pid/tid from the request when explicitly structured as the target."},
						"thread":      map[string]any{"type": "string", "description": "Concrete runtime thread label when explicitly named, such as Thread-10 [56284] or android.haitong [56023]."},
						"source":      map[string]any{"type": "string", "enum": []string{"user_explicit", "artifact_metadata", "tool_handoff"}, "description": "Typed provenance for this runtime target. Use user_explicit when the user named it in the current request."},
						"confidence":  map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0, "description": "Confidence that this is the user's requested runtime target."},
						"description": map[string]any{"type": "string", "description": "Short audit note. Do not include free-form evidence that is not already reflected by pid/thread/source."},
					},
					"required": []string{"kind", "confidence"},
				},
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
				"description": "Required typed profile for positive principal-item role selection. candidate_role is one scalar category for the item represented by a principal row; relation endpoints or row-local attributes belong in answer_subject/entity_axes, cells/text, and grounded relation evidence, not as extra required_candidate_roles. Set is_role_binding_requested=false for ordinary member-to-attribute tables and whenever the current request has no positive principal-role selection.",
				"properties": map[string]any{
					"is_role_binding_requested": map[string]any{"type": "boolean", "description": "True only when the current request explicitly selects one or more principal answer-item categories. False for an implementation/member row whose route, owner, value, or other relation attribute is merely another column."},
					"required_candidate_roles":  map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": answerCandidateRoleValues()}, "description": "Category of the principal answer item(s), never a list of all attributes or relation endpoints displayed on the same row."},
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
			"requested_answer_dimensions": map[string]any{
				"type":        "object",
				"description": "Optional soft typed profile for visible answer dimensions the CURRENT request explicitly asks the final answer to preserve, such as diff clues, current key code, purpose/function, impact, comparison axes, total count, complete member set, evidence source, boundary notes, stage/workflow tables, or diagram/table surfaces. This is presentation guidance only, not an evidence origin and not a hard validation gate.",
				"properties": map[string]any{
					"is_dimensioned_answer": map[string]any{"type": "boolean", "description": "True when the user explicitly asks the answer to cover named visible dimensions; false otherwise."},
					"dimensions": map[string]any{
						"type":        "array",
						"description": "Requested answer dimensions in the order they appear in the current request. Use the user's own label when possible.",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"label":        map[string]any{"type": "string", "description": "User-facing dimension label, preferably copied from the current request, such as `diff 线索`, `当前关键代码`, `作用`, `影响`, `阶段表`, or `sequenceDiagram`."},
								"role":         map[string]any{"type": "string", "enum": requestedAnswerDimensionRoleValues(), "description": "Language-neutral dimension role. stage_or_workflow denotes a conceptual stage/phase/step sequence or handoff surface; for explain requests it does not imply a source declaration inventory."},
								"source_quote": map[string]any{"type": "string", "description": "Verbatim current-request phrase that states this dimension. If the label itself is verbatim, reuse it."},
								"required":     map[string]any{"type": "boolean", "description": "True for dimensions the user directly requested; false for optional stylistic preferences."},
								"index":        map[string]any{"type": "integer", "minimum": 1, "description": "1-based order in the current request."},
							},
							"required": []string{"label"},
						},
					},
					"confidence": map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0, "description": "Your confidence in this presentation profile in [0,1]."},
					"rationale":  map[string]any{"type": "string", "description": "Short audit rationale for why these answer dimensions were requested."},
				},
				"required": []string{"is_dimensioned_answer", "confidence"},
			},
			"current_source_explanation_profile": map[string]any{
				"type":        "object",
				"description": "Dedicated soft typed profile for mixed external-observation + current-checkout requests. You MUST emit it when the CURRENT request asks to explain, verify, trace, compare, locate, or assess an external/non-source observation against current source. source_scope_profile, diagnostic_profile.current_risk/current_version_check, and external_observation_policy.current_source_mode=allow do not substitute for this carrier. It opens the current-source evidence lane; it is not a display dimension and not a hard answer gate.",
				"properties": map[string]any{
					"is_current_source_explanation_requested": map[string]any{"type": "boolean", "description": "True only when the current request explicitly asks to relate external/non-source evidence to the current checkout/source."},
					"modes": map[string]any{
						"type":        "array",
						"description": "Why current source is needed for this mixed request.",
						"items":       map[string]any{"type": "string", "enum": currentSourceExplanationModeValues()},
					},
					"source_quotes": map[string]any{"type": "array", "items": map[string]string{"type": "string"}, "description": "Verbatim current-request phrase(s) that ask for current-source explanation / verification / trace / comparison / location / impact."},
					"target_terms":  map[string]any{"type": "array", "items": map[string]string{"type": "string"}, "description": "Optional user-visible terms that should guide current-source ranking. These are not evidence and do not create hard gates."},
					"confidence":    map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0, "description": "Your confidence in this mixed current-source explanation profile in [0,1]."},
					"rationale":     map[string]any{"type": "string", "description": "Short audit rationale for why current-source explanation is or is not requested."},
				},
				"required": []string{"is_current_source_explanation_requested", "confidence"},
			},
			"external_observation_policy": map[string]any{
				"type":        "object",
				"description": "Optional typed source/citation policy for external observations such as logs, traces, MCP resources, connector rows, command output, web pages, or external documents. Omit it or set current_source_mode=default/allow unless the CURRENT request explicitly says not to read or analyze current checkout/source evidence at all. If current_source_mode=exclude is emitted, also set exclusion_kind=explicit_user_exclusion and current_source_exclusion_quote to one minimal verbatim phrase that forbids current-source evidence; otherwise the tool downgrades the exclusion to default/allow. A request saying only that artifact line numbers are not current-source citations must use artifact_citation_mode=external_only plus artifact_citation_quotes and must not exclude current source.",
				"properties": map[string]any{
					"current_source_mode":    map[string]any{"type": "string", "enum": externalObservationCurrentSourceModeValues(), "description": "default/allow permits current-source analysis but does not require it. Mixed artifact+current-source obligation must use current_source_explanation_profile. exclude suppresses current-source exploration only when the current request explicitly forbids source/current-checkout analysis."},
					"exclusion_kind":         map[string]any{"type": "string", "enum": externalObservationCurrentSourceExclusionKindValues(), "description": "Set to explicit_user_exclusion only when current_source_mode=exclude is justified by a user-authored phrase that forbids current checkout/source evidence. Leave empty otherwise."},
					"artifact_citation_mode": map[string]any{"type": "string", "enum": externalObservationArtifactCitationModeValues(), "description": "default leaves citation policy unchanged. external_only means external artifact line/row refs stay external-observation evidence and must not be re-rendered as current-source file:line citations; it does NOT suppress current-source exploration. allow_current_source is only for external material that has been resolved to current source by another typed signal."},
					"current_source_exclusion_quote": map[string]any{
						"type":        "string",
						"description": "Exactly one minimal verbatim CURRENT-request phrase that forbids reading or analyzing current checkout/source evidence itself. Required for active current_source_mode=exclude. Do not put citation-identity boundaries here.",
					},
					"artifact_citation_quotes": map[string]any{
						"type":        "array",
						"items":       map[string]string{"type": "string"},
						"description": "Verbatim CURRENT-request phrase(s) saying that external artifact line/row references must remain external-observation citations. These quotes never suppress current-source exploration.",
					},
					"confidence": map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0, "description": "Your confidence in this source/citation policy in [0,1]."},
					"rationale":  map[string]any{"type": "string", "description": "Short audit rationale."},
				},
				"required": []string{"confidence"},
			},
			"predicate_axis": map[string]any{
				"type":        "string",
				"enum":        skill.AnalysisPredicateAxisValues(),
				"description": "Action-direction axis of the question (call / register / define / return / configure / condition / implement). For a source-code question_kind=call_chain emit call; the tool repairs an omitted axis from that typed kind but rejects an explicitly conflicting axis. Empty when no other clear action exists. Used by the evidence ranker to bias items whose anchor matches the axis.",
			},
			"diagram_hint": map[string]any{
				"type":        "object",
				"description": "Optional visual-family hint. `required` is the authority boundary: set it true ONLY when the CURRENT request or typed Presentation Directive explicitly requires a diagram / visual / drawing; otherwise set it false, and the hint remains optional guidance. An explicitly requested visual modality is authoritative: sequence/timeline/interaction view -> sequence even when the topic is a call chain; call graph/DAG/fan-out view -> call_dag. Do not replace an explicit sequence request with call_dag merely because predicate_axis=call. Omit this object for ordinary code, call-chain, architecture, log, or trace questions when prose/list/table blocks answer the user directly.",
				"properties": map[string]any{
					"kind":     stringProp{Type: "string", Enum: skill.AnalysisDiagramKindValues()},
					"required": map[string]any{"type": "boolean", "description": "Hard presentation authority. True only for an explicit current-turn visual request; false for an optional structural aid."},
				},
				"required": []string{"kind", "required"},
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
				"description": "Required typed decision. Always emit this object. Set required=false and source_quote=\"\" when the user does not demand exhaustive coverage. Set required=true only when the user explicitly demands an EXHAUSTIVE answer — phrases that signal universal coverage of the answer set or whole requested mechanism path (e.g. quantifiers like 'all'/'every'/'all the' in any language, or explicit completeness markers like 'complete list'/'exhaustive'/'full inventory'/'complete path'/'full path'/'完整路径'/'完整调用路径') — and copy the verbatim trigger phrase into source_quote. Decision rule: if the question would NOT be considered fully answered while one valid member or one load-bearing path step/guard/branch is still missing, use the true arm. A full mechanism/call path is coverage over that path, not automatically an inventory of every nearby declaration or callee. Distinct from enumeration_boundary which carries a count.",
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
				"description": "Optional. When you can identify specific files structurally needed to answer the user's question, list them here with confidence and a short rationale. Confidence ≥ 0.8 means the file is treated as a primary file AND its content is pre-read into the prompt; 0.5 ≤ conf < 0.8 means the file is a soft hint (pre-read eligible only); below 0.5 the entry is discarded — leave the recommendation to the deterministic resolver. Use repo-relative POSIX paths copied verbatim from the prescan results. For source_inventory / inventory-style questions, do not list guessed sample files as required_files; rely on source_inventory_profile and repo_map unless the user named the exact path. Empty list is fine: omit when you do not have file-level conviction. The field name is required_files, not requested_files — the requested_* fields elsewhere in this schema are separate display/profile settings, not this file list.",
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
				"description": "Optional. Negative-channel counterpart of required_files. When you have READ a candidate file in pre-scan and judged it OFF-TOPIC for the user's question, list its repo-relative POSIX path here. The file will NOT be re-injected via pre-read content, mid-loop reading suggestions, or primary-file selection — saves prompt tokens and prevents the system from contradicting your judgment on later iterations. Use sparingly: at most 10 paths, only files you actually inspected. Empty list is fine when no candidates need explicit exclusion. Emit an array of plain strings only, e.g. [\"internal/foo.go\"]; do not emit objects with confidence/rationale.",
				"items":       map[string]any{"type": "string", "description": "Plain repo-relative POSIX path string only; do not emit an object."},
				"maxItems":    10,
			},
		},
		"required": []string{
			"intent", "scenario", "complexity", "keywords", "entities", "question_kind",
			"intent_confidence", "complexity_confidence", "kind_confidence",
			"predicates", "diagnostic_profile", "answer_role_profile", "error_granularity_profile",
			"runtime_artifact_scope_profile", "runtime_target_profile", "runtime_question_profile", "history_selection_profile", "completeness_obligation", "call_chain_endpoints",
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

func sourceInventoryTypeUnderlyingValues() []string {
	values := types.AllSourceInventoryTypeUnderlyings()
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, string(v))
	}
	return out
}

func sourceInventoryRequestedFieldValues() []string {
	values := types.AllSourceInventoryRequestedFields()
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, string(v))
	}
	return out
}

func answerCandidateRoleListForWarning(roles []types.AnswerCandidateRole) string {
	if len(roles) == 0 {
		return ""
	}
	out := make([]string, 0, len(roles))
	seen := map[types.AnswerCandidateRole]bool{}
	for _, role := range roles {
		if role == types.AnswerCandidateRoleUnknown || seen[role] {
			continue
		}
		seen[role] = true
		out = append(out, string(role))
	}
	return strings.Join(out, ", ")
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

func runtimeTargetKindValues() []string {
	return []string{string(types.RuntimeTargetKindProcess), string(types.RuntimeTargetKindThread)}
}

func runtimeArtifactRequestedScopeValues() []string {
	values := types.AllRuntimeArtifactRequestedScopes()
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func runtimeTargetDeclarationValues() []string {
	values := types.AllRuntimeTargetDeclarations()
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func runtimeQuestionScopeValues() []string {
	values := types.AllRuntimeQuestionScopes()
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func runtimeQuestionFactFamilyValues() []string {
	values := types.AllRuntimeQuestionFactFamilies()
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func historySelectionModeValues() []string {
	values := types.AllHistorySelectionModes()
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func historySelectionItemKindValues() []string {
	values := types.AllHistorySelectionItemKinds()
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
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

func requestedAnswerDimensionRoleValues() []string {
	values := types.AllRequestedAnswerDimensionRoles()
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, string(v))
	}
	return out
}

func currentSourceExplanationModeValues() []string {
	values := types.AllCurrentSourceExplanationModes()
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, string(v))
	}
	return out
}

func externalObservationCurrentSourceModeValues() []string {
	values := types.AllExternalObservationCurrentSourceModes()
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, string(v))
	}
	return out
}

func externalObservationCurrentSourceExclusionKindValues() []string {
	values := types.AllExternalObservationCurrentSourceExclusionKinds()
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, string(v))
	}
	return out
}

func externalObservationArtifactCitationModeValues() []string {
	values := types.AllExternalObservationArtifactCitationModes()
	out := make([]string, 0, len(values))
	for _, v := range values {
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
// emitAnalysisMisplacedHints — known recurring near-miss field names
// for this tool's strict decode (EVALFIX-2A wrong-NAME ledger). Rows
// are added per observed recurrence only — no similarity heuristics.
// Each CanonicalName row obligates the canonical field's schema
// description to pre-teach the wrong token (Tripwire C,
// strict_decode_hint_parity_test.go).
var emitAnalysisMisplacedHints = []MisplacedFieldHint{
	// EVALRUN-1 F3 (2/12 runs): the requested_* profile family sits
	// adjacent to required_files and models cross the two prefixes.
	{Field: "requested_files", CanonicalName: "required_files"},
}

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

	params = applyStructuredPayloadCompatWithLegacyStringFieldRepair(t.Name(), params, t.Parameters(),
		"answer_subject",
		"predicates",
		"diagnostic_profile",
		"conversation_reference_profile",
		"source_scope_profile",
		"change_impact_profile",
		"field_value_profile",
		"artifact_value_profile",
		"runtime_artifact_scope_profile",
		"runtime_target_profile",
		"runtime_question_profile",
		"history_selection_profile",
		"answer_exclusion_policy",
		"answer_role_profile",
		"error_granularity_profile",
		"requested_answer_dimensions",
		"external_observation_policy",
		"diagram_hint",
		"enumeration_boundary",
		"completeness_obligation",
		"call_chain_endpoints",
	)
	var compatWarnings []string
	if repaired, warnings, ok := repairEmitAnalysisIrrelevantFilePathObjects(params); ok {
		params = repaired
		compatWarnings = append(compatWarnings, warnings...)
	}
	if repaired, warnings, ok := repairEmitAnalysisRequiredFileStringEntries(params); ok {
		params = repaired
		compatWarnings = append(compatWarnings, warnings...)
	}
	if repaired, warnings, ok := repairEmitAnalysisNonBoundedFactFamilies(params); ok {
		params = repaired
		compatWarnings = append(compatWarnings, warnings...)
	}

	var p emitAnalysisParams
	if _, decodeFailure, err := decodeStrictNormalizedToolParams(t.Name(), params, &p, emitAnalysisMisplacedHints); err != nil {
		return *decodeFailure, err
	}
	// The completeness decision is deliberately presence-required. A nil
	// profile is not equivalent to false: r193 showed that a model can
	// understand a complete-path request while simply omitting the optional
	// carrier, which silently disables every downstream typed coverage lane.
	// This is a single structured-field presence check; it never scans the
	// request, model prose, thinking, or answer for completeness keywords.
	if p.CompletenessObligation == nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: completeness_obligation is required; emit required=false with source_quote=\"\" when exhaustive coverage is not requested, or required=true with a verbatim current-request source_quote when it is",
			Timestamp: time.Now(),
		}, nil
	}
	raw := types.StripConversationPrefix(ctx.Mutable.Objective())

	// Normalize enum fields first so validateAnalysisInput and the
	// Summary operate on a single canonical view of the classification.
	intent := normalizeIntent(p.Intent)
	scenario := normalizeScenario(p.Scenario)
	complexity := normalizeComplexity(p.Complexity)
	kind := normalizeQuestionKind(p.QuestionKind)

	keywords := trimStringSlice(p.Keywords)
	entities := trimStringSlice(p.Entities)
	subTopics := emitAnalysisSubTopics(p.SubTopics)

	// Runtime validation — keyword floor + entity blocklist + quality
	// probe. Config lives in AnalysisLimits (see analysis_limits.go).
	// Two new inputs vs the pre-refactor signature:
	//
	//   - seenBlob: lowercased typed pre-scan corpus read from
	//     Mutable.PrescanSummaryBlob(), populated by analyzerEvaluator.Observe
	//     through AppendPrescanToolResult. Feeds the verified-entity whitelist
	//     and the hit-ratio quality probe without parsing rendered tool
	//     summaries.
	//   - prescanRounds: copied from Mutable.PrescanRoundCount(), so empty
	//     typed-corpus results still count as pre-scan attempts.
	//
	// The validator returns the filtered entity slice so a dropped
	// generic noun never reaches the persisted RequestModel.
	limits := CurrentAnalysisLimits()
	seenBlob := ctx.Mutable.PrescanSummaryBlob()
	prescanRounds := ctx.Mutable.PrescanRoundCount()
	val := validateAnalysisInput(keywords, entities, limits, seenBlob, prescanRounds)
	val.Warnings = append(val.Warnings, compatWarnings...)
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
	if reconciled, reason := reconcileSetValuedRoleLocatePredicates(intent, predicates, subTopics); reason != "" {
		predicates = reconciled
		val.Warnings = append(val.Warnings, reason)
	}
	if reconciled, reason := reconcileSetValuedCountPredicates(intent, predicates); reason != "" {
		predicates = reconciled
		val.Warnings = append(val.Warnings, reason)
	}
	historySelectionProfile, historySelectionErr, historySelectionWarnings := parseHistorySelectionProfile(raw, predicates.IsHistoryLookup, p.HistorySelectionProfile)
	if historySelectionErr != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: " + historySelectionErr,
			Timestamp: time.Now(),
		}, nil
	}
	for _, warning := range historySelectionWarnings {
		logging.Warning("[emit_analysis] %s", warning)
		val.Warnings = append(val.Warnings, warning)
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
	externalObservationPolicy, externalObservationPolicyErr, externalObservationPolicyWarnings := parseExternalObservationPolicy(raw, p.ExternalObservationPolicy)
	if externalObservationPolicyErr != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: " + externalObservationPolicyErr,
			Timestamp: time.Now(),
		}, nil
	}
	for _, warning := range externalObservationPolicyWarnings {
		logging.Warning("[emit_analysis] %s", warning)
		val.Warnings = append(val.Warnings, warning)
	}
	if repaired, warning := repairMissingExternalObservationExclusionKindFromRouteHint(ctx, p.ExternalObservationPolicy, externalObservationPolicy); warning != "" {
		externalObservationPolicy = repaired
		logging.Warning("[emit_analysis] %s", warning)
		val.Warnings = append(val.Warnings, warning)
	}
	if promoted, warning := promoteInvalidExternalObservationExcludeToAllow(ctx, p.ExternalObservationPolicy, externalObservationPolicy); warning != "" {
		externalObservationPolicy = promoted
		logging.Warning("[emit_analysis] %s", warning)
		val.Warnings = append(val.Warnings, warning)
	}
	if synthesized, warning := synthesizeExternalObservationPolicyFromRouteHint(ctx, externalObservationPolicy); warning != "" {
		externalObservationPolicy = synthesized
		logging.Warning("[emit_analysis] %s", warning)
		val.Warnings = append(val.Warnings, warning)
	}
	currentSourceExplanation, currentSourceExplanationErr, currentSourceExplanationWarnings := parseCurrentSourceExplanationProfile(raw, p.CurrentSourceExplanation)
	if currentSourceExplanationErr != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: " + currentSourceExplanationErr,
			Timestamp: time.Now(),
		}, nil
	}
	for _, warning := range currentSourceExplanationWarnings {
		logging.Warning("[emit_analysis] %s", warning)
		val.Warnings = append(val.Warnings, warning)
	}
	if normalizedPolicy, warning := normalizeExternalObservationPolicyForCurrentSourceExplanation(externalObservationPolicy, currentSourceExplanation); warning != "" {
		externalObservationPolicy = normalizedPolicy
		logging.Warning("[emit_analysis] %s", warning)
		val.Warnings = append(val.Warnings, warning)
	}
	if normalizedDiagnostic, warnings := normalizeDiagnosticProfileForExternalObservationPolicy(diagnosticProfile, externalObservationPolicy); len(warnings) > 0 {
		diagnosticProfile = normalizedDiagnostic
		for _, warning := range warnings {
			logging.Warning("[emit_analysis] %s", warning)
			val.Warnings = append(val.Warnings, warning)
		}
	}
	artifactOnlyRuntime := emitAnalysisObservationOnlyRuntimeArtifact(ctx, externalObservationPolicy)
	if normalizedIntent, normalizedScenario, warning := normalizeRuntimeArtifactScalarIntent(artifactOnlyRuntime, intent, scenario, kind, predicates); warning != "" {
		intent = normalizedIntent
		scenario = normalizedScenario
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
	sourceScopeProfile, sourceScopeErr, sourceScopeWarnings := parseSourceScopeProfile(raw, p.SourceScopeProfile)
	if sourceScopeErr != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: " + sourceScopeErr,
			Timestamp: time.Now(),
		}, nil
	}
	if len(sourceScopeWarnings) > 0 {
		val.Warnings = append(val.Warnings, sourceScopeWarnings...)
	}
	sourceScopeQuotesRejected := p.SourceScopeProfile != nil &&
		len(trimStringSlice(p.SourceScopeProfile.SourceQuotes)) > 0 &&
		(sourceScopeProfile == nil || len(sourceScopeProfile.SourceQuotes) == 0)
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
	sourceInventoryProfile, sourceInventoryErr, sourceInventoryWarnings := parseSourceInventoryProfile(raw, p.SourceInventoryProfile)
	if sourceInventoryErr != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: " + sourceInventoryErr,
			Timestamp: time.Now(),
		}, nil
	}
	for _, warning := range sourceInventoryWarnings {
		logging.Warning("[emit_analysis] %s", warning)
		val.Warnings = append(val.Warnings, warning)
	}
	answerRoleProfile, answerRoleErr, answerRoleWarnings := parseAnswerRoleProfile(raw, p.AnswerRoleProfile)
	if answerRoleErr != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: " + answerRoleErr,
			Timestamp: time.Now(),
		}, nil
	}
	for _, warning := range answerRoleWarnings {
		logging.Warning("[emit_analysis] %s", warning)
		val.Warnings = append(val.Warnings, warning)
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
	requestedAnswerDimensions, currentSourceObligationSignals, requestedAnswerDimensionsErr, requestedAnswerDimensionsWarnings := parseRequestedAnswerDimensions(raw, p.RequestedAnswerDimensions)
	if requestedAnswerDimensionsErr != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: " + requestedAnswerDimensionsErr,
			Timestamp: time.Now(),
		}, nil
	}
	for _, warning := range requestedAnswerDimensionsWarnings {
		logging.Warning("[emit_analysis] %s", warning)
		val.Warnings = append(val.Warnings, warning)
	}
	runtimeArtifactCarrier := emitAnalysisHasRuntimeArtifactCarrier(ctx)
	runtimeArtifactValueProfile, runtimeArtifactValueErr := parseRuntimeArtifactValueProfile(runtimeArtifactCarrier, p.RuntimeArtifactValueProfile)
	if runtimeArtifactValueErr != "" {
		if !runtimeArtifactCarrier && p.RuntimeArtifactValueProfile != nil && p.RuntimeArtifactValueProfile.IsArtifactValueLookup != nil && *p.RuntimeArtifactValueProfile.IsArtifactValueLookup {
			warning := "dropped invalid optional artifact_value_profile outside runtime artifact context: " + runtimeArtifactValueErr
			logging.Warning("[emit_analysis] %s", warning)
			val.Warnings = append(val.Warnings, warning)
		} else {
			return types.ToolResult{
				ToolName:  t.Name(),
				Success:   false,
				Summary:   "emit_analysis rejected: " + runtimeArtifactValueErr,
				Timestamp: time.Now(),
			}, nil
		}
	}
	runtimeArtifactScopeProfile, runtimeArtifactScopeErr, runtimeArtifactScopeWarnings := parseRuntimeArtifactScopeProfile(raw, runtimeArtifactCarrier, p.RuntimeArtifactScopeProfile)
	runtimeTargets, runtimeTargetWarnings, runtimeTargetErr := parseRuntimeTargets(p.RuntimeTargets)
	var runtimeTargetProfile *types.RuntimeTargetProfile
	var runtimeTargetProfileErr string
	var runtimeTargetProfileWarnings []string
	if runtimeTargetErr == "" {
		// The declaration consumes the normalized target roster. Do not run
		// this dependent semantic check when the roster itself is malformed:
		// doing so would add a synthetic "missing target" error that cannot be
		// acted on independently. The other runtime profiles remain independent
		// and are still censused below.
		runtimeTargetProfile, runtimeTargetProfileErr, runtimeTargetProfileWarnings = parseRuntimeTargetProfile(raw, runtimeArtifactCarrier, p.RuntimeTargetProfile, runtimeTargets)
	}
	runtimeQuestionProfile, runtimeQuestionProfileErr, runtimeQuestionProfileWarnings := parseRuntimeQuestionProfile(
		raw,
		runtimeArtifactCarrier,
		p.RuntimeQuestionProfile,
		types.NormalizeRequirementKind(kind) == types.ReqCallChain || axis == types.AxisCall || predicates.IsRelationalLookup,
	)
	// MERGE-AUDIT T6-2: these profiles are independent request-authority
	// declarations. Returning after the first bad declaration made a single
	// payload with several local defects consume one retry per profile. Census
	// every independently actionable error in schema order. This is not a
	// semantic cascade collector: the target declaration is deliberately
	// skipped when its target roster failed, and cross-profile consistency
	// remains below after all individual profiles are valid.
	runtimeProfileErrors := trimNonEmptyStrings([]string{
		runtimeArtifactScopeErr,
		runtimeTargetErr,
		runtimeTargetProfileErr,
		runtimeQuestionProfileErr,
	})
	if len(runtimeProfileErrors) > 0 {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: runtime profile validation failed: " + strings.Join(runtimeProfileErrors, "; "),
			Timestamp: time.Now(),
		}, nil
	}
	for _, warning := range runtimeArtifactScopeWarnings {
		logging.Warning("[emit_analysis] %s", warning)
		val.Warnings = append(val.Warnings, warning)
	}
	for _, warning := range runtimeTargetWarnings {
		logging.Warning("[emit_analysis] %s", warning)
		val.Warnings = append(val.Warnings, warning)
	}
	for _, warning := range runtimeTargetProfileWarnings {
		logging.Warning("[emit_analysis] %s", warning)
		val.Warnings = append(val.Warnings, warning)
	}
	for _, warning := range runtimeQuestionProfileWarnings {
		logging.Warning("[emit_analysis] %s", warning)
		val.Warnings = append(val.Warnings, warning)
	}
	if issue := validateRuntimeQuestionProfileConsistency(
		runtimeQuestionProfile, intent, scenario, predicates, diagnosticProfile,
	); issue != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: " + issue,
			Timestamp: time.Now(),
		}, nil
	}
	fieldValueProfile, fieldValueErr := parseFieldValueProfile(raw, p.FieldValueProfile)
	if fieldValueErr != "" {
		if runtimeArtifactCarrier && runtimeArtifactValueProfile == nil {
			if converted, warning := runtimeArtifactValueProfileFromFieldValueParam(ctx, p.FieldValueProfile, fieldValueErr); converted != nil {
				runtimeArtifactValueProfile = converted
				fieldValueErr = ""
				logging.Warning("[emit_analysis] %s", warning)
				val.Warnings = append(val.Warnings, warning)
			}
		}
	}
	if fieldValueErr != "" {
		if !shouldDropInvalidOptionalFieldValueProfile(artifactOnlyRuntime, predicates, currentSourceExplanation, p.FieldValueProfile) {
			return types.ToolResult{
				ToolName:  t.Name(),
				Success:   false,
				Summary:   "emit_analysis rejected: " + fieldValueErr,
				Timestamp: time.Now(),
			}, nil
		}
		warning := invalidOptionalFieldValueProfileWarning(artifactOnlyRuntime, fieldValueErr)
		logging.Warning("[emit_analysis] %s", warning)
		val.Warnings = append(val.Warnings, warning)
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
	answerSubject := parseAnswerSubject(p.AnswerSubject)
	if normalizedPreds, normalizedSubject, normalizedSubTopics, warning := normalizeRoleBindingScalarShape(
		intent,
		kind,
		axis,
		predicates,
		answerRoleProfile,
		answerSubject,
		subTopics,
	); warning != "" {
		predicates = normalizedPreds
		answerSubject = normalizedSubject
		subTopics = normalizedSubTopics
		val.Warnings = append(val.Warnings, warning)
	}
	if normalized, warning := normalizeRuntimeArtifactRoleLocateAnswerSubject(ctx, artifactOnlyRuntime, predicates, answerSubject); warning != "" {
		answerSubject = normalized
		val.Warnings = append(val.Warnings, warning)
	}
	if normalized, warning := normalizeTypedRoleLocateAnswerSubject(kind, axis, predicates, answerSubject); warning != "" {
		answerSubject = normalized
		val.Warnings = append(val.Warnings, warning)
	}
	if normalized, warning := normalizeMissingAnswerSubjectForNonScalarExplain(axis, intent, predicates, entities, subTopics, answerSubject); warning != "" {
		answerSubject = normalized
		val.Warnings = append(val.Warnings, warning)
	}
	if normalized, warning := normalizeMissingAnswerSubjectForSourceInventory(sourceInventoryProfile, answerSubject); warning != "" {
		answerSubject = normalized
		val.Warnings = append(val.Warnings, warning)
	}
	if warning := normalizeSourceInventoryRequestedFieldsForAnswerSubject(sourceInventoryProfile, answerSubject); warning != "" {
		val.Warnings = append(val.Warnings, warning)
	}
	exactTargets, exactTargetErr, exactTargetWarn := validateExactTargets(raw, p.ExactTargets, p.RequiredFiles, predicates, p.SourceInventoryProfile, answerSubject, entities)
	if exactTargetWarn != "" {
		logging.Warning("[emit_analysis] %s", exactTargetWarn)
		val.Warnings = append(val.Warnings, exactTargetWarn)
	}
	if exactTargetErr != "" {
		if !artifactOnlyRuntime {
			return types.ToolResult{
				ToolName:  t.Name(),
				Success:   false,
				Summary:   "emit_analysis rejected: " + exactTargetErr,
				Timestamp: time.Now(),
			}, nil
		}
		exactTargets = types.MentionedEntitiesFromRawRequest(raw, p.ExactTargets)
		warning := "dropped invalid optional exact_targets for observation-only runtime artifact: " + exactTargetErr
		if len(exactTargets) > 0 {
			warning = fmt.Sprintf("%s; kept %d verbatim target(s)", warning, len(exactTargets))
		}
		logging.Warning("[emit_analysis] %s", warning)
		val.Warnings = append(val.Warnings, warning)
	}
	mentionedEntities := types.MentionedEntitiesFromRawRequest(raw, entities)
	if normalizedAxis, warning, issue := reconcileSourceCallChainAxis(
		kind,
		axis,
		runtimeArtifactCarrier,
		predicates,
	); issue != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: " + issue,
			Timestamp: time.Now(),
		}, nil
	} else {
		axis = normalizedAxis
		if warning != "" {
			val.Warnings = append(val.Warnings, warning)
		}
	}
	if issue := validateCallChainEndpointWireShape(kind, axis, runtimeArtifactCarrier, p.CallChainEndpoints, exactTargets); issue != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: " + issue,
			Timestamp: time.Now(),
		}, nil
	}
	callChainEndpointProfile, callChainEndpointWarning := types.NormalizeCallChainEndpointProfile(
		p.CallChainEndpoints,
		append(append([]string(nil), exactTargets...), mentionedEntities...),
	)
	if normalizedKind, warning := reconcileSourceCallChainKindFromEndpointProfile(
		kind,
		axis,
		runtimeArtifactCarrier,
		predicates,
		callChainEndpointProfile,
		append(append([]string(nil), exactTargets...), mentionedEntities...),
	); warning != "" {
		kind = normalizedKind
		val.Warnings = append(val.Warnings, warning)
	}
	if types.NormalizeRequirementKind(kind) != types.ReqCallChain || axis != types.AxisCall || runtimeArtifactCarrier {
		if callChainEndpointProfile != nil {
			callChainEndpointWarning = "dropped call_chain_endpoints outside a source-code call_chain with predicate_axis=call"
		}
		callChainEndpointProfile = nil
	}
	if callChainEndpointWarning != "" {
		val.Warnings = append(val.Warnings, callChainEndpointWarning)
	}
	if issue := validateSourceCallChainEndpointDeclaration(
		kind,
		axis,
		runtimeArtifactCarrier,
		predicates,
		callChainEndpointProfile,
		exactTargets,
		mentionedEntities,
	); issue != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: " + issue,
			Timestamp: time.Now(),
		}, nil
	}
	// Self-consistency: after typed, deterministic normalizers have
	// absorbed safe drift, reject only contradictions that still need
	// the LLM to reconcile its own classification.
	if issue := validateSelfConsistencyDetailed(intent, scenario, kind, predicates, diagnosticProfile, axis, entities, subTopics, answerSubject); issue.Reason != "" {
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
	if issue := validateRuntimeArtifactCallChainConsistency(kind, predicates, runtimeTargets); issue != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: " + issue,
			Timestamp: time.Now(),
		}, nil
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
	sanitizedSubTopics := sanitizeSubTopics(subTopics)

	requiredFileHints := validateAndBuildRequiredFileHintsWithContext(ctx, p.RequiredFiles, &val)
	irrelevantFiles := validateAndBuildIrrelevantFiles(p.IrrelevantFiles, &val)

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
			RequiredFileHints: requiredFileHints,
			IrrelevantFiles:   irrelevantFiles,
			Kind:              kind,
		},
		AnswerSubject:                   answerSubject,
		IntentConfidence:                p.IntentConfidence,
		ComplexityConfidence:            p.ComplexityConfidence,
		KindConfidence:                  p.KindConfidence,
		Predicates:                      predicates,
		DiagnosticProfile:               diagnosticProfile,
		ReferencedArtifactLines:         normalizeEmitArtifactLineRefs(p.ReferencedArtifactLines),
		ConversationReferenceProfile:    conversationReferenceProfile,
		SourceScopeProfile:              sourceScopeProfile,
		AnswerVisibilityProfile:         answerVisibilityProfile,
		SourceInventoryProfile:          sourceInventoryProfile,
		CallChainEndpointProfile:        callChainEndpointProfile,
		ChangeImpactProfile:             changeImpactProfile,
		FieldValueProfile:               fieldValueProfile,
		RuntimeArtifactValueProfile:     runtimeArtifactValueProfile,
		RuntimeArtifactScopeProfile:     runtimeArtifactScopeProfile,
		RuntimeTargets:                  runtimeTargets,
		RuntimeTargetProfile:            runtimeTargetProfile,
		RuntimeQuestionProfile:          runtimeQuestionProfile,
		HistorySelectionProfile:         historySelectionProfile,
		AnswerExclusionPolicy:           answerExclusionPolicy,
		AnswerRoleProfile:               answerRoleProfile,
		ErrorGranularityProfile:         errorGranularityProfile,
		RequestedAnswerDimensions:       requestedAnswerDimensions,
		CurrentSourceObligationSignals:  currentSourceObligationSignals,
		CurrentSourceExplanationProfile: currentSourceExplanation,
		ExternalObservationPolicy:       externalObservationPolicy,
		PredicateAxis:                   axis,
		DiagramHint:                     diagramHint,
		EnumerationBoundary:             enumerationBoundary,
		CompletenessObligation:          completenessObligation,
		Buckets:                         buckets,
	}
	routeHint := types.TurnRouteHint{}
	if ctx != nil {
		routeHint = ctx.TurnRouteHint
	}
	if types.RouteBackedHistoryCurrentCodeExplanation(rm, routeHint) &&
		!rm.HasCurrentSourceObligationSignal() {
		rm.CurrentSourceObligationSignals = append(rm.CurrentSourceObligationSignals,
			types.CurrentSourceObligationSignal{
				Kind: types.CurrentSourceObligationSignalRouteBackedHistoryExplanation,
			})
		warning := "route-backed history/current-code obligation preserved after optional analyzer profile omission"
		logging.Warning("[emit_analysis] %s", warning)
		val.Warnings = append(val.Warnings, warning)
	}
	projectRuntimeArtifactPathHintsFromRawRequest(&rm, raw)
	attachRuntimeArtifactsToRequestModel(ctx, &rm)
	if types.ErrorGranularityConflictsWithDiagnosticMechanism(rm) {
		rm.ErrorGranularityProfile = nil
		warning := "error_granularity_profile auto-softened: diagnostic/current-source explanation is not a precise failure-scope verdict request"
		logging.Warning("[emit_analysis] %s", warning)
		val.Warnings = append(val.Warnings, warning)
	}
	droppedSourceInventoryForPrincipalConflict := false
	if droppedSourceInventory, warning := dropSourceInventoryProfileForTypedRelation(&rm); droppedSourceInventory {
		droppedSourceInventoryForPrincipalConflict = true
		logging.Warning("[emit_analysis] %s", warning)
		val.Warnings = append(val.Warnings, warning)
	}
	if droppedSourceInventory, warning := dropSourceInventoryProfileForObservationOnlyRuntime(ctx, &rm); droppedSourceInventory {
		logging.Warning("[emit_analysis] %s", warning)
		val.Warnings = append(val.Warnings, warning)
	}
	if softenedAnswerRole, warning := softenAnswerRoleProfileForPerMemberRelation(&rm); softenedAnswerRole {
		logging.Warning("[emit_analysis] %s", warning)
		val.Warnings = append(val.Warnings, warning)
	}
	softenAttemptedSourceInventory := p.SourceInventoryProfile
	if droppedSourceInventoryForPrincipalConflict {
		softenAttemptedSourceInventory = nil
	}
	rm.AnalyzerHints.RequiredFileHints = softenModelAuthoredRequiredFilesForSourceInventory(
		raw,
		rm.SourceInventoryProfile,
		softenAttemptedSourceInventory,
		rm.AnalyzerHints.RequiredFileHints,
		&val,
	)
	rm.AnalyzerHints.RequiredFileHints, rm.AnalyzerHints.IrrelevantFiles, rm.SourceScopeProfile = reconcilePrincipalScopeIrrelevantFiles(
		ctx,
		rm.SourceScopeProfile,
		rm.SourceInventoryProfile,
		rm.AnswerExclusionPolicy,
		rm.Intent,
		rm.Predicates,
		rm.AnalyzerHints.RequiredFileHints,
		rm.AnalyzerHints.IrrelevantFiles,
		&val,
	)
	if !droppedSourceInventoryForPrincipalConflict {
		if warning := synthesizeSourceInventoryProfileForTypedEnumeration(ctx, &rm, raw, p.SourceInventoryProfile); warning != "" {
			logging.Warning("[emit_analysis] %s", warning)
			val.Warnings = append(val.Warnings, warning)
		}
		if warning := enrichSourceInventoryProfileFromAnalyzerPrescan(ctx, &rm, raw); warning != "" {
			logging.Warning("[emit_analysis] %s", warning)
			val.Warnings = append(val.Warnings, warning)
		}
	}
	if warning := normalizeSourceInventoryProductionScope(&rm); warning != "" {
		logging.Warning("[emit_analysis] %s", warning)
		val.Warnings = append(val.Warnings, warning)
	}
	if warning := normalizeSourceInventoryConstructOnlySourceScope(&rm, sourceScopeQuotesRejected); warning != "" {
		logging.Warning("[emit_analysis] %s", warning)
		val.Warnings = append(val.Warnings, warning)
	}
	if warning := normalizeSourceInventoryAuxiliaryExclusion(&rm); warning != "" {
		logging.Warning("[emit_analysis] %s", warning)
		val.Warnings = append(val.Warnings, warning)
	}
	if warning := normalizeSourceInventorySubTopicsFromProfile(&rm); warning != "" {
		logging.Warning("[emit_analysis] %s", warning)
		val.Warnings = append(val.Warnings, warning)
	}
	if warning := normalizeSourceInventoryKeywordsFromProfile(&rm); warning != "" {
		logging.Warning("[emit_analysis] %s", warning)
		val.Warnings = append(val.Warnings, warning)
	}
	if conflict := validateAuxiliaryPrincipalExclusionConflict(rm); conflict != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: " + conflict,
			Timestamp: time.Now(),
		}, nil
	}
	if added := projectAnalyzerPrescanRequiredFileHints(ctx, &rm, &val); added > 0 {
		logging.Warning("[emit_analysis] projected %d required_file hint(s) from deterministic analyzer prescan", added)
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

// reconcileSourceCallChainKindFromEndpointProfile resolves one typed
// self-contradiction in analyzer output: a structurally valid ordered
// source/sink carrier on the call axis cannot remain in the generic mechanism
// family, because that family immediately discards the carrier and loses the
// direction obligation. Promotion is deliberately narrow and provenance
// bound. It never reads keywords, summaries, or answer prose: the source must
// be an exact current-request identity already admitted by the analyzer's
// existing token-boundary provenance pass.
func reconcileSourceCallChainKindFromEndpointProfile(
	kind string,
	axis types.PredicateAxis,
	runtimeArtifactCarrier bool,
	predicates types.SemanticPredicates,
	profile *types.CallChainEndpointProfile,
	requestIdentities []string,
) (string, string) {
	if runtimeArtifactCarrier || axis != types.AxisCall || profile == nil || !profile.Active() ||
		predicates.IsScalarAnswer || predicates.IsRoleLocateLookup {
		return kind, ""
	}
	normalized := types.NormalizeRequirementKind(kind)
	if normalized == types.ReqCallChain {
		return kind, ""
	}
	if normalized != types.ReqMechanism || !callChainProfileSourceHasExactRequestIdentity(profile, requestIdentities) {
		return kind, ""
	}
	return string(types.ReqCallChain),
		"normalized question_kind=mechanism to call_chain because predicate_axis=call carried a structurally valid ordered call_chain_endpoints profile whose source has exact current-request provenance"
}

func callChainProfileSourceHasExactRequestIdentity(profile *types.CallChainEndpointProfile, identities []string) bool {
	if profile == nil {
		return false
	}
	source := strings.TrimSpace(profile.Source)
	if source == "" {
		return false
	}
	for _, identity := range identities {
		if strings.EqualFold(source, strings.TrimSpace(identity)) {
			return true
		}
	}
	return false
}

func normalizeSourceInventoryProductionScope(rm *types.RequestModel) string {
	if rm == nil ||
		rm.SourceScopeProfile == nil ||
		rm.SourceInventoryProfile == nil ||
		!rm.SourceInventoryProfile.Active() ||
		rm.SourceScopeProfile.RequestedScope != types.SourceScopeProduction ||
		SourceInventoryHasExplicitAuxiliaryExclusion(*rm) {
		return ""
	}
	if sourceScopeEchoesInventoryTargets(rm.SourceScopeProfile.SourceQuotes, rm.SourceInventoryProfile.SourceQuotes) {
		rm.SourceScopeProfile = nil
		return "source_scope_profile auto-softened: production scope quote echoed source-inventory target categories without a typed auxiliary exclusion"
	}
	return ""
}

func normalizeSourceInventoryConstructOnlySourceScope(rm *types.RequestModel, sourceScopeQuotesRejected bool) string {
	if rm == nil ||
		rm.SourceScopeProfile == nil ||
		rm.SourceInventoryProfile == nil ||
		!rm.SourceInventoryProfile.Active() ||
		rm.SourceScopeProfile.RequestedScope == "" ||
		rm.SourceScopeProfile.RequestedScope == types.SourceScopeUnknown {
		return ""
	}
	if sourceScopeQuotesRejected && len(rm.SourceScopeProfile.SourceQuotes) == 0 {
		scope := rm.SourceScopeProfile.RequestedScope
		rm.SourceScopeProfile = nil
		return fmt.Sprintf("source_scope_profile auto-softened: %s scope had no validated current-request source quote", scope)
	}
	if rm.SourceScopeProfile.RequestedScope == types.SourceScopeProduction &&
		SourceInventoryHasExplicitAuxiliaryExclusion(*rm) {
		return ""
	}
	if !sourceScopeQuotesOnlyInventoryTargets(rm.SourceScopeProfile.SourceQuotes, rm.SourceInventoryProfile.SourceQuotes) {
		return ""
	}
	scope := rm.SourceScopeProfile.RequestedScope
	rm.SourceScopeProfile = nil
	return fmt.Sprintf("source_scope_profile auto-softened: %s scope quote(s) were covered only by source-inventory construct quote(s)", scope)
}

func normalizeSourceInventoryAuxiliaryExclusion(rm *types.RequestModel) string {
	if rm == nil ||
		rm.SourceInventoryProfile == nil ||
		!rm.SourceInventoryProfile.Active() ||
		rm.AnswerExclusionPolicy == nil ||
		!rm.AnswerExclusionPolicy.ExcludesAuxiliarySourceClasses() {
		return ""
	}
	if rm.SourceScopeProfile != nil {
		if rm.SourceScopeProfile.AllowsAuxiliaryPrincipal() ||
			rm.SourceScopeProfile.RequestedScope == types.SourceScopeProduction {
			return ""
		}
	}
	if !sourceQuoteSetEchoesInventoryTargets(rm.AnswerExclusionPolicy.SourceQuotes, rm.SourceInventoryProfile.SourceQuotes) {
		return ""
	}
	rm.AnswerExclusionPolicy = nil
	return "answer_exclusion_policy auto-softened: auxiliary exclusions echoed source-inventory target categories without a precise production source scope"
}

func normalizeSourceInventorySubTopicsFromProfile(rm *types.RequestModel) string {
	if rm == nil ||
		rm.SourceInventoryProfile == nil ||
		!types.SourceInventoryPrincipalNavigationActive(*rm) ||
		!rm.SourceInventoryProfile.MechanicalRowsOnly() ||
		len(rm.SourceInventoryProfile.SourceQuotes) == 0 {
		return ""
	}
	var topics []types.SubTopic
	seen := map[string]bool{}
	for _, quote := range rm.SourceInventoryProfile.SourceQuotes {
		quote = strings.TrimSpace(quote)
		if quote == "" {
			continue
		}
		key := strings.ToLower(quote)
		if seen[key] {
			continue
		}
		seen[key] = true
		topics = append(topics, types.SubTopic{
			Summary:  "source inventory: " + quote,
			Entities: []string{quote},
		})
	}
	if len(topics) == 0 || sourceInventorySubTopicsEqual(rm.SubTopics, topics) {
		return ""
	}
	rm.SubTopics = topics
	return "source_inventory sub_topics normalized from source_quotes so prescan-only language/source guesses remain advisory"
}

// normalizeSourceInventoryKeywordsFromProfile keeps the mechanical inventory
// lane from carrying prescan guesses into exploration/finalization as if they
// were verified source syntax. For this lane, validated source_quotes plus the
// closed typed roles/fields are sufficient retrieval vocabulary; free-form
// model keywords can otherwise introduce a language, declaration mechanism, or
// scope qualifier that never appeared in either the request or repository.
//
// This is prompt/context hygiene only. It does not reject an analysis, inspect
// final-answer prose, or drive any answer-side hard gate.
func normalizeSourceInventoryKeywordsFromProfile(rm *types.RequestModel) string {
	if rm == nil ||
		rm.SourceInventoryProfile == nil ||
		!types.SourceInventoryPrincipalNavigationActive(*rm) ||
		!rm.SourceInventoryProfile.MechanicalRowsOnly() {
		return ""
	}
	var keywords []string
	keywords = append(keywords, rm.SourceInventoryProfile.SourceQuotes...)
	for _, role := range rm.SourceInventoryProfile.TargetRoles {
		keywords = append(keywords, string(role))
	}
	for _, field := range rm.SourceInventoryProfile.RequestedFields {
		keywords = append(keywords, string(field))
	}
	keywords = stableDistinctTrimmedStrings(keywords)
	if len(keywords) == 0 || reflect.DeepEqual(rm.AnalyzerHints.Keywords, keywords) {
		return ""
	}
	rm.AnalyzerHints.Keywords = keywords
	return "source_inventory keywords normalized from validated source_quotes and typed roles/fields so prescan-only syntax/language guesses do not enter later prompts"
}

func stableDistinctTrimmedStrings(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, raw := range in {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sourceInventorySubTopicsEqual(a, b []types.SubTopic) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.TrimSpace(a[i].Summary) != strings.TrimSpace(b[i].Summary) {
			return false
		}
		if strings.Join(trimStringSlice(a[i].Entities), "\x00") != strings.Join(trimStringSlice(b[i].Entities), "\x00") {
			return false
		}
	}
	return true
}

func sourceScopeEchoesInventoryTargets(scopeQuotes, inventoryQuotes []string) bool {
	normalizedInventory := make([]string, 0, len(inventoryQuotes))
	seen := map[string]bool{}
	for _, quote := range inventoryQuotes {
		key := normalizeSourceScopeEchoQuote(quote)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		normalizedInventory = append(normalizedInventory, key)
	}
	if len(normalizedInventory) < 2 {
		return false
	}
	for _, scopeQuote := range scopeQuotes {
		scope := normalizeSourceScopeEchoQuote(scopeQuote)
		if scope == "" {
			continue
		}
		matches := 0
		for _, inv := range normalizedInventory {
			if strings.Contains(scope, inv) {
				matches++
			}
		}
		if matches >= 2 {
			return true
		}
	}
	return false
}

func sourceQuoteSetEchoesInventoryTargets(scopeQuotes, inventoryQuotes []string) bool {
	if sourceScopeEchoesInventoryTargets(scopeQuotes, inventoryQuotes) {
		return true
	}
	inventory := make([]string, 0, len(inventoryQuotes))
	seenInventory := map[string]bool{}
	for _, quote := range inventoryQuotes {
		key := normalizeSourceScopeEchoQuote(quote)
		if key == "" || seenInventory[key] {
			continue
		}
		seenInventory[key] = true
		inventory = append(inventory, key)
	}
	if len(inventory) < 2 {
		return false
	}
	matched := 0
	for _, inv := range inventory {
		for _, quote := range scopeQuotes {
			scope := normalizeSourceScopeEchoQuote(quote)
			if scope != "" && (strings.Contains(scope, inv) || strings.Contains(inv, scope)) {
				matched++
				break
			}
		}
	}
	return matched >= 2
}

func normalizeSourceScopeEchoQuote(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return ""
	}
	return strings.Join(strings.Fields(s), " ")
}

func sourceScopeQuotesOnlyInventoryTargets(scopeQuotes, inventoryQuotes []string) bool {
	normalizedInventory := make([]string, 0, len(inventoryQuotes))
	seen := map[string]bool{}
	for _, quote := range inventoryQuotes {
		key := normalizeSourceScopeEchoQuote(quote)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		normalizedInventory = append(normalizedInventory, key)
	}
	if len(normalizedInventory) == 0 {
		return false
	}
	checked := 0
	covered := 0
	for _, quote := range scopeQuotes {
		scope := normalizeSourceScopeEchoQuote(quote)
		if scope == "" {
			continue
		}
		checked++
		for _, inv := range normalizedInventory {
			if scope == inv || strings.Contains(inv, scope) {
				covered++
				break
			}
		}
	}
	return checked >= 2 && covered == checked
}

func synthesizeSourceInventoryProfileForTypedEnumeration(ctx *types.BusContext, rm *types.RequestModel, raw string, attempted *emitSourceInventoryProfileParam) string {
	if rm == nil || rm.SourceInventoryProfile != nil && rm.SourceInventoryProfile.Active() {
		return ""
	}
	if !types.IsTypedSourceEnumerationShape(*rm) {
		return ""
	}
	if types.SourceInventoryLaneConflictsWithPrincipalAnswer(*rm) {
		return ""
	}
	if emitAnalysisObservationOnlyRuntimeArtifactForSourceInventoryGuards(ctx, *rm) {
		return ""
	}
	if ctx != nil && ctx.RuntimeArtifactPreflight.ZeroCurrentSourceRepo() {
		// §29.122 LENSBURN 病A 方向A2: the deterministic run-entry census proved
		// this checkout contains zero current-source files (runtime artifacts
		// only), so a source-inventory obligation is structurally
		// unsatisfiable — do not synthesize the lens profile. The census gate
		// is precise: Completed=false keeps this arm inert.
		return ""
	}
	requestedFields := sourceInventoryProfileRepairRequestedFields(attempted)
	if len(requestedFields) == 0 {
		requestedFields = []types.SourceInventoryRequestedField{
			types.SourceInventoryFieldName,
			types.SourceInventoryFieldLocation,
			types.SourceInventoryFieldSummary,
		}
	}
	underlying, requiresConstSet := sourceInventoryProfileRepairTypeFacets(attempted)
	rm.SourceInventoryProfile = &types.SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       sourceInventoryDefaultQueryEnumerationRoles(),
		TypeUnderlying:    underlying,
		RequiresConstSet:  requiresConstSet,
		RequestedFields:   requestedFields,
		SourceQuotes:      sourceInventoryProfileRepairSourceQuotes(raw, attempted),
		Confidence:        0.45,
		Rationale:         "synthesized from typed source-enumeration request shape",
	}
	return "synthesized source_inventory_profile from typed source-enumeration request shape"
}

func validateAuxiliaryPrincipalExclusionConflict(rm types.RequestModel) string {
	if rm.SourceScopeProfile == nil || !rm.SourceScopeProfile.AllowsAuxiliaryPrincipal() {
		return ""
	}
	if !SourceInventoryHasExplicitAuxiliaryExclusion(rm) {
		return ""
	}
	if rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		return ""
	}
	return "source_scope_profile allows repo-owned auxiliary source classes as principal inventory members, but answer_exclusion_policy excludes auxiliary source classes; these typed policies are mutually exclusive. Keep auxiliary classes in scope or remove the auxiliary exclusion before emitting analysis."
}

func sourceInventoryProfileRepairSourceQuotes(raw string, attempted *emitSourceInventoryProfileParam) []string {
	if attempted == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, quote := range trimStringSlice(attempted.SourceQuotes) {
		if !sourceQuotePresentInCurrentRequest(raw, quote) || seen[quote] {
			continue
		}
		seen[quote] = true
		out = append(out, quote)
	}
	return out
}

func sourceInventoryProfileRepairRequestedFields(attempted *emitSourceInventoryProfileParam) []types.SourceInventoryRequestedField {
	if attempted == nil {
		return nil
	}
	seen := map[types.SourceInventoryRequestedField]bool{}
	var out []types.SourceInventoryRequestedField
	for _, rawField := range attempted.RequestedFields {
		field := types.SourceInventoryRequestedField(strings.TrimSpace(rawField))
		if !field.IsValid() || seen[field] {
			continue
		}
		seen[field] = true
		out = append(out, field)
	}
	return out
}

func sourceInventoryProfileRepairTypeFacets(attempted *emitSourceInventoryProfileParam) (types.SourceInventoryTypeUnderlying, bool) {
	if attempted == nil {
		return types.SourceInventoryTypeUnderlyingUnknown, false
	}
	underlying := types.SourceInventoryTypeUnderlying(strings.TrimSpace(attempted.TypeUnderlying))
	if underlying == "" || !underlying.IsValid() {
		underlying = types.SourceInventoryTypeUnderlyingUnknown
	}
	requiresConstSet := attempted.RequiresConstSet != nil && *attempted.RequiresConstSet
	return underlying, requiresConstSet
}

func sourceInventoryRequiredHintsFormBoundedScope(ctx *types.BusContext, rm types.RequestModel, candidates []string) bool {
	var paths []string
	for _, hint := range rm.AnalyzerHints.RequiredFileHints {
		paths = append(paths, hint.Path)
	}
	paths = append(paths, candidates...)
	repoRoot := ""
	if ctx != nil {
		repoRoot = ctx.RepoRoot
	}
	files := types.BoundedSourceEnumerationScopeFiles(rm, paths, repoRoot)
	return len(files) >= types.BoundedSourceEnumerationMinFiles &&
		types.BoundedSourceEnumerationCommonScope(files) != ""
}

func sourceInventoryPrescanPeerSupplementAllowed(ctx *types.BusContext, rm types.RequestModel, candidates []string) bool {
	if len(rm.AnalyzerHints.RequiredFileHints) == 0 || len(candidates) == 0 {
		return false
	}
	var highConfidence []string
	for _, hint := range rm.AnalyzerHints.RequiredFileHints {
		if hint.Confidence >= 0.8 {
			highConfidence = append(highConfidence, hint.Path)
		}
	}
	if len(highConfidence) == 0 {
		return false
	}
	files := analyzerPrescanBoundedRequiredFileCandidates(ctx, rm, append(append([]string{}, highConfidence...), candidates...))
	if len(files) < 2 {
		return false
	}
	return types.BoundedSourceEnumerationCommonScope(files) != ""
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
			return selfConsistencyIssue{Kind: selfConsistencyIssueOther, Reason: "is_role_locate_lookup=true requires answer_subject.kind to name the located literal kind (numeric / string_literal / function_name / type_name / file_path / handler_route / config_key / interface_name / struct_field / enum_value)"}
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
			return selfConsistencyIssue{Kind: selfConsistencyIssueOther, Reason: fmt.Sprintf(
				"is_diagnostic_question=true conflicts with intent=%s — choose from the CURRENT requested answer without system replacement: for cause / remediation / current-risk analysis set intent=root_cause (and root_cause or performance_bottleneck scenario); for an ordinary mechanism, architecture, implementation, or attached-log conclusion-boundary explanation keep its non-root-cause intent and set predicates.is_diagnostic_question plus diagnostic_profile.is_diagnostic/current_risk/historical_regression/current_version_check=false",
				intent,
			)}
		}
		switch scenario {
		case types.ScenarioRootCause, types.ScenarioPerformanceBottleneck:
			// ok
		default:
			return selfConsistencyIssue{Kind: selfConsistencyIssueOther, Reason: fmt.Sprintf(
				"is_diagnostic_question=true conflicts with scenario=%s — choose from the CURRENT requested answer without system replacement: for cause / remediation / current-risk analysis use scenario=root_cause or performance_bottleneck; for an ordinary mechanism, architecture, implementation, or attached-log conclusion-boundary explanation keep its non-root-cause scenario and clear the diagnostic predicate/profile flags",
				scenario,
			)}
		}
	}
	if intent == types.IntentRootCause && !diagnosticRequired {
		return selfConsistencyIssue{Kind: selfConsistencyIssueRootCauseMissingDiagnostic, Reason: "intent=root_cause requires a diagnostic typed signal — set predicates.is_diagnostic_question=true or diagnostic_profile.is_diagnostic/current_risk/historical_regression=true"}
	}
	// call_chain is a relationship shape, not a generic synonym for
	// "inspect a trace". Require at least one precise relational carrier:
	// an explicit call axis, the relational predicate, or two named
	// endpoints/entities. This rejects the observed single-target runtime
	// state/value classification that otherwise routes a bounded status
	// query into full call-chain/causal-projection contracts. It still
	// admits one-target role-locate and wakeup/caller questions when the
	// analyzer supplies AxisCall, and admits source→sink traces with their
	// two endpoint entities.
	if types.NormalizeRequirementKind(kind) == types.ReqCallChain &&
		axis != types.AxisCall &&
		!preds.IsRelationalLookup &&
		len(trimStringSlice(entities)) < 2 {
		return selfConsistencyIssue{Kind: selfConsistencyIssueOther, Reason: "question_kind=call_chain requires a precise relationship signal — set predicate_axis=call, predicates.is_relational_lookup=true, or provide at least two named caller/source and callee/sink entities; a single runtime target's state, duration, count, reason, or current status is not a call chain"}
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

func validateRuntimeArtifactCallChainConsistency(
	kind string,
	preds types.SemanticPredicates,
	runtimeTargets []types.RuntimeTarget,
) string {
	if types.NormalizeRequirementKind(kind) != types.ReqCallChain ||
		preds.IsRelationalLookup ||
		len(runtimeTargets) == 0 ||
		runtimeArtifactDistinctTargetCount(runtimeTargets) >= 2 {
		return ""
	}
	return "question_kind=call_chain with one runtime target requires predicates.is_relational_lookup=true or at least two distinct runtime targets — predicate_axis=call only names the relationship axis and cannot by itself turn a target's state, duration, count, reason, or current status into a call chain; use question_kind=conditional/mechanism for that fact shape"
}

// validateSourceCallChainEndpointDeclaration prevents unordered identity sets
// from silently becoming directional source/sink authority later in
// exploration. Every non-scalar source-code call chain must carry the ordered,
// structurally valid endpoint profile. When three or more request symbol hints are in
// scope, exact_targets must additionally identify the endpoint pair and keep
// intermediate/context symbols out of the principal target set. Runtime
// artifact call chains use RuntimeTargets and have their own consistency
// validator. This function consumes only normalized typed fields, never
// request or model prose.
func validateSourceCallChainEndpointDeclaration(
	kind string,
	axis types.PredicateAxis,
	runtimeArtifactCarrier bool,
	predicates types.SemanticPredicates,
	profile *types.CallChainEndpointProfile,
	exactTargets, mentionedEntities []string,
) string {
	if runtimeArtifactCarrier ||
		types.NormalizeRequirementKind(kind) != types.ReqCallChain ||
		axis != types.AxisCall ||
		predicates.IsRoleLocateLookup ||
		predicates.IsScalarAnswer {
		return ""
	}
	if !profile.Active() {
		return "source-code question_kind=call_chain with predicate_axis=call requires call_chain_endpoints.source plus an exact sink or discover-mode empty sink, unless sink_mode=discover_path leaves both source and sink empty for grounded role-bound endpoint discovery; entities and exact_targets are unordered identity sets and cannot supply call direction"
	}
	if profile.DiscoverSinkActive() || profile.DiscoverPathActive() {
		return ""
	}
	exactEndpoints := types.CallChainRequestedEndpointHints(types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{MentionedEntities: exactTargets},
	})
	if len(exactEndpoints) == 1 {
		return "source-code question_kind=call_chain exact_targets contains only one symbol endpoint; provide both caller/source and callee/sink, or omit symbol exact_targets when the two endpoint entities are already unambiguous"
	}
	entityEndpoints := types.CallChainRequestedEndpointHints(types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{MentionedEntities: mentionedEntities},
	})
	if len(entityEndpoints) <= 2 {
		return ""
	}
	if len(exactEndpoints) >= 2 {
		return ""
	}
	return fmt.Sprintf(
		"source-code question_kind=call_chain with predicate_axis=call has %d symbol-like entities but only %d exact endpoint target(s); set exact_targets to at least the caller/source and callee/sink copied verbatim from the current request, and keep intermediate/context symbols in entities — entity ordering is not endpoint authority",
		len(entityEndpoints), len(exactEndpoints),
	)
}

// runtimeArtifactDistinctTargetCount counts endpoint identities, not emitted
// rows. A process/thread spelling pair with the same positive pid is one
// focus identity and cannot impersonate source + sink. PID-less thread labels
// remain exact, case-insensitive identities; no name-to-pid guess is made.
func runtimeArtifactDistinctTargetCount(targets []types.RuntimeTarget) int {
	seen := map[string]struct{}{}
	for _, target := range targets {
		key := ""
		if target.PID > 0 {
			key = fmt.Sprintf("pid:%d", target.PID)
		} else if thread := strings.ToLower(strings.TrimSpace(target.Thread)); thread != "" {
			key = "thread:" + thread
		}
		if key != "" {
			seen[key] = struct{}{}
		}
	}
	return len(seen)
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
		types.SubjectStringLiteral,
		types.SubjectNumeric:
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
			"predicates object missing — emit `predicates` with is_scalar_answer / is_role_locate_lookup / is_count_question / is_cross_component / is_relational_lookup / is_category_enumeration / is_history_lookup / is_diagnostic_question / has_per_member_table each set to true or false"
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
	if p.HasPerMemberTable == nil {
		missing = append(missing, "has_per_member_table")
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
		HasPerMemberTable:     *p.HasPerMemberTable,
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

func normalizeDiagnosticProfileForExternalObservationPolicy(
	diagnostic types.DiagnosticIntentProfile,
	policy *types.ExternalObservationPolicy,
) (types.DiagnosticIntentProfile, []string) {
	if policy == nil || !policy.ExcludesCurrentSource() {
		return diagnostic, nil
	}
	var warnings []string
	if diagnostic.CurrentRisk {
		diagnostic.CurrentRisk = false
		warnings = append(warnings, "external_observation_policy current_source_mode=exclude repaired diagnostic_profile.current_risk true→false")
	}
	if diagnostic.CurrentVersionCheck {
		diagnostic.CurrentVersionCheck = false
		warnings = append(warnings, "external_observation_policy current_source_mode=exclude repaired diagnostic_profile.current_version_check true→false")
	}
	if diagnostic.HistoricalRegression {
		diagnostic.HistoricalRegression = false
		warnings = append(warnings, "external_observation_policy current_source_mode=exclude repaired diagnostic_profile.historical_regression true→false")
	}
	return diagnostic, warnings
}

func normalizeExternalObservationPolicyForCurrentSourceExplanation(
	policy *types.ExternalObservationPolicy,
	currentSource *types.CurrentSourceExplanationProfile,
) (*types.ExternalObservationPolicy, string) {
	if policy == nil || currentSource == nil || !currentSource.Active() ||
		policy.CurrentSourceMode != types.ExternalObservationCurrentSourceExclude {
		return policy, ""
	}
	normalized := *policy
	normalized.CurrentSourceMode = types.ExternalObservationCurrentSourceAllow
	normalized.ExclusionKind = types.ExternalObservationSourceExclusionNone
	return &normalized, "external_observation_policy current_source_mode=exclude auto-softened to allow because current_source_explanation_profile is active; citation-only artifact boundaries must use artifact_citation_mode, not source exclusion"
}

func repairMissingExternalObservationExclusionKindFromRouteHint(
	ctx *types.BusContext,
	raw *emitExternalObservationPolicyParam,
	policy *types.ExternalObservationPolicy,
) (*types.ExternalObservationPolicy, string) {
	if raw == nil || policy == nil {
		return policy, ""
	}
	if types.NormalizeExternalObservationCurrentSourceMode(raw.CurrentSourceMode) != types.ExternalObservationCurrentSourceExclude {
		return policy, ""
	}
	if types.NormalizeExternalObservationCurrentSourceExclusionKind(raw.ExclusionKind) != types.ExternalObservationSourceExclusionNone {
		return policy, ""
	}
	if strings.TrimSpace(raw.CurrentSourceExclusionQuote) == "" {
		return policy, ""
	}
	if policy.ExcludesCurrentSource() || len(policy.SourceQuotes) == 0 {
		return policy, ""
	}
	if ctx == nil || !ctx.TurnRouteHint.ExternalObservationParticipates() || ctx.TurnRouteHint.RequiresCurrentSourceEvidence() {
		return policy, ""
	}
	if !emitAnalysisHasRuntimeArtifactCarrier(ctx) {
		return policy, ""
	}
	normalized := *policy
	normalized.CurrentSourceMode = types.ExternalObservationCurrentSourceExclude
	normalized.ExclusionKind = types.ExternalObservationSourceExclusionExplicitUserBoundary
	if normalized.Confidence <= 0 {
		normalized.Confidence = 0.75
	}
	if strings.TrimSpace(normalized.Rationale) == "" {
		normalized.Rationale = "typed route metadata marked current checkout evidence optional for this external-observation turn"
	}
	return &normalized, "external_observation_policy missing exclusion_kind repaired from typed route metadata (current checkout evidence optional)"
}

func promoteInvalidExternalObservationExcludeToAllow(
	ctx *types.BusContext,
	raw *emitExternalObservationPolicyParam,
	policy *types.ExternalObservationPolicy,
) (*types.ExternalObservationPolicy, string) {
	if raw == nil ||
		types.NormalizeExternalObservationCurrentSourceMode(raw.CurrentSourceMode) != types.ExternalObservationCurrentSourceExclude {
		return policy, ""
	}
	if policy != nil && policy.ExcludesCurrentSource() {
		return policy, ""
	}
	if !emitAnalysisHasRuntimeArtifactCarrier(ctx) {
		return policy, ""
	}
	normalized := types.ExternalObservationPolicy{}
	if policy != nil {
		normalized = *policy
	}
	normalized.CurrentSourceMode = types.ExternalObservationCurrentSourceAllow
	normalized.ExclusionKind = types.ExternalObservationSourceExclusionNone
	if normalized.Confidence <= 0 {
		normalized.Confidence = 0.75
	}
	if strings.TrimSpace(normalized.Rationale) == "" {
		normalized.Rationale = "invalid runtime-artifact source exclusion lacked precise current-request provenance"
	}
	return &normalized, "external_observation_policy invalid current_source_mode=exclude auto-softened to allow for runtime artifact: only anchored explicit user exclusions may close the current-source lane"
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

func parseSourceScopeProfile(raw string, p *emitSourceScopeProfileParam) (*types.SourceScopeProfile, string, []string) {
	if p == nil {
		return nil, "", nil
	}
	var warnings []string
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
		), nil
	}
	if *p.Confidence < 0 || *p.Confidence > 1 {
		return nil, fmt.Sprintf("source_scope_profile.confidence %.2f out of [0,1]", *p.Confidence), nil
	}
	scope := types.SourceScope(strings.TrimSpace(p.RequestedScope))
	if !scope.IsValid() {
		return nil, fmt.Sprintf(
			"source_scope_profile.requested_scope %q is invalid; use one of %s",
			p.RequestedScope, strings.Join(sourceScopeValues(), ", "),
		), nil
	}
	includeAux := false
	if p.IncludeAuxiliaryAsPrincipal != nil {
		includeAux = *p.IncludeAuxiliaryAsPrincipal
	}
	sourceQuotes := trimStringSlice(p.SourceQuotes)
	keptQuotes := sourceQuotes[:0]
	for _, quote := range sourceQuotes {
		if sourceQuotePresentInCurrentRequest(raw, quote) {
			keptQuotes = append(keptQuotes, quote)
			continue
		}
		warnings = append(warnings, "source_scope_profile.source_quotes entry ignored because it is not copied verbatim from the current request")
	}
	return &types.SourceScopeProfile{
		RequestedScope:              scope,
		IncludeAuxiliaryAsPrincipal: includeAux,
		SourceQuotes:                append([]string(nil), keptQuotes...),
		Confidence:                  *p.Confidence,
		Rationale:                   strings.TrimSpace(p.Rationale),
	}, "", warnings
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

func parseSourceInventoryProfile(raw string, p *emitSourceInventoryProfileParam) (*types.SourceInventoryProfile, string, []string) {
	if p == nil {
		return nil, "", nil
	}
	var warnings []string
	if p.Confidence != nil && (*p.Confidence < 0 || *p.Confidence > 1) {
		return nil, fmt.Sprintf("source_inventory_profile.confidence %.2f out of [0,1]", *p.Confidence), nil
	}
	confidence := 0.5
	if p.Confidence != nil {
		confidence = *p.Confidence
	} else {
		warnings = append(warnings, "source_inventory_profile.confidence omitted; downstream will treat the profile as low-confidence advisory data")
	}
	if p.IsSourceInventory == nil {
		warnings = append(warnings, "source_inventory_profile.is_source_inventory omitted; profile ignored")
		return nil, "", warnings
	}
	if !*p.IsSourceInventory {
		return nil, "", warnings
	}
	seenRoles := map[types.AnswerCandidateRole]bool{}
	var roles []types.AnswerCandidateRole
	for i, rawRole := range p.TargetRoles {
		role, ok := types.NormalizeAnswerCandidateRole(rawRole)
		if !ok || role == types.AnswerCandidateRoleUnknown {
			warnings = append(warnings, fmt.Sprintf(
				"source_inventory_profile.target_roles[%d] %q ignored because it is not one of %s",
				i, rawRole, strings.Join(answerCandidateRoleValues(), ", "),
			))
			continue
		}
		if seenRoles[role] {
			continue
		}
		seenRoles[role] = true
		roles = append(roles, role)
	}
	if len(roles) == 0 {
		warnings = append(warnings, "source_inventory_profile.target_roles omitted or empty; profile ignored")
		return nil, "", warnings
	}
	underlying := types.SourceInventoryTypeUnderlying(strings.TrimSpace(p.TypeUnderlying))
	if underlying == "" {
		underlying = types.SourceInventoryTypeUnderlyingUnknown
	}
	if !underlying.IsValid() {
		return nil, fmt.Sprintf(
			"source_inventory_profile.type_underlying %q is invalid; use one of %s",
			p.TypeUnderlying, strings.Join(sourceInventoryTypeUnderlyingValues(), ", "),
		), nil
	}
	requiresConstSet := false
	if p.RequiresConstSet != nil {
		requiresConstSet = *p.RequiresConstSet
	}
	seenFields := map[types.SourceInventoryRequestedField]bool{}
	var fields []types.SourceInventoryRequestedField
	for i, rawField := range p.RequestedFields {
		field := types.SourceInventoryRequestedField(strings.TrimSpace(rawField))
		if !field.IsValid() {
			warnings = append(warnings, fmt.Sprintf(
				"source_inventory_profile.requested_fields[%d] %q ignored because it is not one of %s",
				i, rawField, strings.Join(sourceInventoryRequestedFieldValues(), ", "),
			))
			continue
		}
		if seenFields[field] {
			continue
		}
		seenFields[field] = true
		fields = append(fields, field)
	}
	sourceQuotes := trimStringSlice(p.SourceQuotes)
	keptQuotes := sourceQuotes[:0]
	for _, quote := range sourceQuotes {
		if sourceQuotePresentInCurrentRequest(raw, quote) {
			keptQuotes = append(keptQuotes, quote)
			continue
		}
		warnings = append(warnings, "source_inventory_profile.source_quotes entry ignored because it is not copied verbatim from the current request")
	}
	profile := &types.SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       roles,
		TypeUnderlying:    underlying,
		RequiresConstSet:  requiresConstSet,
		RequestedFields:   fields,
		SourceQuotes:      append([]string(nil), keptQuotes...),
		Confidence:        confidence,
		Rationale:         strings.TrimSpace(p.Rationale),
	}
	if removed := types.NormalizeSourceInventoryDisplayAttributeRoles(profile); len(removed) > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"source_inventory_profile.target_roles moved display attribute role(s) %s to requested_fields because structural principal roles are already present",
			answerCandidateRoleListForWarning(removed),
		))
	}
	return profile, "", warnings
}

func normalizeSourceInventoryRequestedFieldsForAnswerSubject(profile *types.SourceInventoryProfile, answerSubject types.AnswerSubject) string {
	if !types.NormalizeSourceInventoryRequestedFieldsForAnswerSubject(profile, answerSubject) {
		return ""
	}
	return "source_inventory_profile.requested_fields removed values because typed answer_subject/source_inventory_profile define an identity inventory; literal/source values are not a mechanical row-set display field"
}

func normalizeMissingAnswerSubjectForSourceInventory(profile *types.SourceInventoryProfile, answerSubject types.AnswerSubject) (types.AnswerSubject, string) {
	if answerSubject.Kind != types.SubjectUnknown || profile == nil || !profile.Active() {
		return answerSubject, ""
	}
	inferred, ok := types.AnswerSubjectForSourceInventoryProfile(profile)
	if !ok {
		return answerSubject, ""
	}
	return inferred, "answer_subject inferred from source_inventory_profile.target_roles before source-inventory field normalization"
}

func dropSourceInventoryProfileForTypedRelation(rm *types.RequestModel) (bool, string) {
	if rm == nil || rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		return false, ""
	}
	if types.HasTypedRelationMemberSetShape(*rm) {
		rm.SourceInventoryProfile = nil
		return true, "source_inventory_profile ignored because predicate_axis / relational predicate declares a typed relation answer; relation member sets must be carried by typed graph/evidence, not source-inventory repair"
	}
	if types.SourceInventoryProfileConflictsWithRelationFlow(*rm) {
		rm.SourceInventoryProfile = nil
		return true, "source_inventory_profile ignored because the typed request is a structural trace / relation-flow answer; source_inventory is for bounded member inventories, while relation-flow navigation should use relation_map plus grounded file reads"
	}
	if types.SourceInventoryProfileConflictsWithRoleBinding(*rm) {
		rm.SourceInventoryProfile = nil
		return true, "source_inventory_profile ignored because the typed request is a registry/binding member-set answer; source_inventory may orient navigation, but binding membership must be proven by registration evidence and structured member_set handoff"
	}
	if types.SourceInventoryLaneConflictsWithArchitectureNarrative(*rm) {
		rm.SourceInventoryProfile = nil
		return true, "source_inventory_profile ignored because the typed request is an architecture/mechanism narrative; conceptual stages/components are bounded by the mechanism and source declarations remain supporting evidence"
	}
	if types.SourceInventoryLaneConflictsWithArchitectureMemberExplanation(*rm) {
		rm.SourceInventoryProfile = nil
		return true, "source_inventory_profile ignored because the typed architecture member set asks for per-member responsibilities; source declarations remain supporting evidence rather than the principal member universe"
	}
	if types.SourceInventoryLaneConflictsWithConceptualWorkflowDimension(*rm) {
		rm.SourceInventoryProfile = nil
		return true, "source_inventory_profile ignored because the typed explanation requires a conceptual stage/workflow dimension; source declarations remain supporting evidence rather than the principal member universe"
	}
	return false, ""
}

// softenAnswerRoleProfileForPerMemberRelation resolves a structural contract
// collision without reading request or answer prose. candidate_role is a
// scalar category for each principal item, whereas a per-member relation table
// has one principal member plus one or more row-local attributes/endpoints.
// Treating every endpoint as a required candidate role creates an impossible
// finalizer obligation and a false coverage caveat. The typed relation shape
// remains intact; only the inapplicable positive-role gate is removed.
func softenAnswerRoleProfileForPerMemberRelation(rm *types.RequestModel) (bool, string) {
	if rm == nil || rm.AnswerRoleProfile == nil || !rm.AnswerRoleProfile.Active() {
		return false, ""
	}
	if !rm.Predicates.HasPerMemberTable || rm.Predicates.IsRoleLocateLookup {
		return false, ""
	}
	relationMemberSet := types.HasTypedRelationMemberSetShape(*rm) ||
		rm.PredicateAxis == types.AxisRegister ||
		types.RequiresRelationMemberSetHandoff(*rm)
	if !relationMemberSet {
		return false, ""
	}
	rm.AnswerRoleProfile = nil
	return true, "answer_role_profile auto-softened: typed per-member relation rows have one principal candidate_role; related endpoints/attributes remain in answer_subject, row cells/text, and grounded relation evidence"
}

func dropSourceInventoryProfileForObservationOnlyRuntime(ctx *types.BusContext, rm *types.RequestModel) (bool, string) {
	if rm == nil || rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		return false, ""
	}
	if !emitAnalysisObservationOnlyRuntimeArtifactForSourceInventoryGuards(ctx, *rm) {
		return false, ""
	}
	rm.SourceInventoryProfile = nil
	return true, "source_inventory_profile ignored because the typed external-observation policy excludes current-source evidence; runtime artifact identifiers must stay in the observation lane, not source-inventory repair"
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

func parseRuntimeArtifactValueProfile(runtimeArtifactCarrier bool, p *emitRuntimeArtifactValueProfileParam) (*types.RuntimeArtifactValueProfile, string) {
	if p == nil {
		return nil, ""
	}
	var missing []string
	if p.IsArtifactValueLookup == nil {
		missing = append(missing, "is_artifact_value_lookup")
	}
	if p.Confidence == nil {
		missing = append(missing, "confidence")
	}
	if len(missing) > 0 {
		return nil, fmt.Sprintf(
			"artifact_value_profile missing required field(s): %s",
			strings.Join(missing, ", "),
		)
	}
	if *p.Confidence < 0 || *p.Confidence > 1 {
		return nil, fmt.Sprintf("artifact_value_profile.confidence %.2f out of [0,1]", *p.Confidence)
	}
	if !*p.IsArtifactValueLookup {
		return nil, ""
	}
	if !runtimeArtifactCarrier {
		return nil, "artifact_value_profile requires an attached runtime artifact or accepted runtime observation"
	}
	value := strings.TrimSpace(p.Value)
	if value == "" {
		return nil, "artifact_value_profile.value is required when is_artifact_value_lookup=true"
	}
	target := strings.TrimSpace(p.Target)
	artifactRefs := trimNonEmptyStrings(p.ArtifactRefs)
	observationRefs := trimNonEmptyStrings(p.ObservationRefs)
	if target == "" && len(artifactRefs) == 0 && len(observationRefs) == 0 {
		return nil, "artifact_value_profile needs target, artifact_refs, or observation_refs"
	}
	literalKind := types.FieldValueLiteralKind(strings.TrimSpace(p.LiteralKind))
	if !literalKind.IsValid() {
		return nil, fmt.Sprintf(
			"artifact_value_profile.literal_kind %q is invalid; use one of %s",
			p.LiteralKind, strings.Join(fieldValueLiteralKindValues(), ", "),
		)
	}
	return &types.RuntimeArtifactValueProfile{
		IsArtifactValueLookup: true,
		Target:                target,
		Value:                 value,
		Unit:                  strings.TrimSpace(p.Unit),
		LiteralKind:           literalKind,
		ArtifactRefs:          artifactRefs,
		ObservationRefs:       observationRefs,
		Confidence:            *p.Confidence,
		Rationale:             strings.TrimSpace(p.Rationale),
	}, ""
}

func parseRuntimeArtifactScopeProfile(raw string, runtimeArtifactCarrier bool, p *emitRuntimeArtifactScopeProfileParam) (*types.RuntimeArtifactScopeProfile, string, []string) {
	if p == nil {
		return nil, "runtime_artifact_scope_profile object missing — emit requested_scope plus confidence; use not_applicable when no runtime artifact is in scope", nil
	}
	if strings.TrimSpace(p.RequestedScope) == "" || p.Confidence == nil {
		var missing []string
		if strings.TrimSpace(p.RequestedScope) == "" {
			missing = append(missing, "requested_scope")
		}
		if p.Confidence == nil {
			missing = append(missing, "confidence")
		}
		return nil, "runtime_artifact_scope_profile missing required field(s): " + strings.Join(missing, ", "), nil
	}
	if *p.Confidence < 0 || *p.Confidence > 1 {
		return nil, fmt.Sprintf("runtime_artifact_scope_profile.confidence %.2f out of [0,1]", *p.Confidence), nil
	}
	scope := types.RuntimeArtifactRequestedScope(strings.TrimSpace(p.RequestedScope))
	if !scope.IsValid() {
		return nil, fmt.Sprintf(
			"runtime_artifact_scope_profile.requested_scope %q is invalid; use one of %s",
			p.RequestedScope, strings.Join(runtimeArtifactRequestedScopeValues(), ", "),
		), nil
	}
	if !runtimeArtifactCarrier {
		if scope != types.RuntimeArtifactScopeNotApplicable {
			return &types.RuntimeArtifactScopeProfile{
				RequestedScope: types.RuntimeArtifactScopeNotApplicable,
				Confidence:     *p.Confidence,
			}, "", []string{"runtime_artifact_scope_profile normalized to not_applicable because no runtime artifact carrier is present"}
		}
		return &types.RuntimeArtifactScopeProfile{
			RequestedScope: scope,
			Confidence:     *p.Confidence,
		}, "", nil
	}

	profile := &types.RuntimeArtifactScopeProfile{
		RequestedScope: scope,
		TimeStart:      p.TimeStart,
		TimeEnd:        p.TimeEnd,
		Confidence:     *p.Confidence,
		Rationale:      strings.TrimSpace(p.Rationale),
	}
	var warnings []string
	quote := strings.TrimSpace(p.SourceQuote)
	anchored := quote != "" && sourceQuotePresentInCurrentRequest(raw, quote)
	switch scope {
	case types.RuntimeArtifactScopeFullArtifact:
		if !anchored {
			profile.RequestedScope = types.RuntimeArtifactScopeUnspecified
			profile.TimeStart = nil
			profile.TimeEnd = nil
			warnings = append(warnings, "runtime_artifact_scope_profile auto-softened to unspecified because source_quote is not verbatim in the current request")
		} else {
			profile.SourceQuote = quote
			profile.TimeStart = nil
			profile.TimeEnd = nil
		}
	case types.RuntimeArtifactScopeBoundedSelector:
		if !anchored {
			profile.RequestedScope = types.RuntimeArtifactScopeUnspecified
			profile.TimeStart = nil
			profile.TimeEnd = nil
			warnings = append(warnings, "runtime_artifact_scope_profile auto-softened to unspecified because source_quote is not verbatim in the current request")
		} else if p.TimeStart != nil && p.TimeEnd != nil &&
			*p.TimeStart >= 0 && *p.TimeEnd > *p.TimeStart {
			// A valid typed start/end pair is the structurally more precise
			// subtype of bounded_selector. Analyzer models occasionally emit
			// both shapes at once. Preserve the precise carrier instead of
			// discarding it based on the noisier enum choice: downstream
			// exact-window authority then remains independent of scenario
			// wording. This consumes only schema-valid typed fields plus the
			// existing exact current-request quote anchor; it does not scan
			// request or answer prose for keywords.
			profile.RequestedScope = types.RuntimeArtifactScopeExplicitWindow
			profile.SourceQuote = quote
			warnings = append(warnings, "runtime_artifact_scope_profile canonicalized bounded_selector with valid typed time_start/time_end to explicit_time_window")
		} else {
			profile.SourceQuote = quote
			profile.TimeStart = nil
			profile.TimeEnd = nil
		}
	case types.RuntimeArtifactScopeExplicitWindow:
		if !anchored || p.TimeStart == nil || p.TimeEnd == nil || *p.TimeStart < 0 || *p.TimeEnd <= *p.TimeStart {
			profile.RequestedScope = types.RuntimeArtifactScopeUnspecified
			profile.TimeStart = nil
			profile.TimeEnd = nil
			warnings = append(warnings, "runtime_artifact_scope_profile auto-softened to unspecified because explicit_time_window lacks an anchored quote or valid time_start/time_end")
		} else {
			profile.SourceQuote = quote
		}
	case types.RuntimeArtifactScopeNotApplicable:
		profile.RequestedScope = types.RuntimeArtifactScopeUnspecified
		profile.TimeStart = nil
		profile.TimeEnd = nil
		warnings = append(warnings, "runtime_artifact_scope_profile normalized to unspecified because a runtime artifact carrier is present")
	case types.RuntimeArtifactScopeUnspecified:
		profile.TimeStart = nil
		profile.TimeEnd = nil
	}
	return profile, "", warnings
}

func parseRuntimeTargets(in []emitRuntimeTargetParam) ([]types.RuntimeTarget, []string, string) {
	if len(in) == 0 {
		return nil, nil, ""
	}
	const maxRuntimeTargets = 8
	out := make([]types.RuntimeTarget, 0, minInt(len(in), maxRuntimeTargets))
	var warnings []string
	seen := map[string]bool{}
	for i, item := range in {
		if len(out) >= maxRuntimeTargets {
			warnings = append(warnings, fmt.Sprintf("runtime_targets: dropped entries over cap of %d", maxRuntimeTargets))
			break
		}
		target, warning, ok := parseRuntimeTarget(item)
		if !ok {
			return nil, warnings, fmt.Sprintf(
				"runtime_targets[%d] is structurally invalid: %s; correct the typed target identity instead of omitting it",
				i, warning,
			)
		}
		if warning != "" {
			warnings = append(warnings, fmt.Sprintf("runtime_targets[%d]: %s", i, warning))
		}
		key := fmt.Sprintf("%s:%d:%s", target.Kind, target.PID, strings.ToLower(target.Thread))
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, target)
	}
	return out, warnings, ""
}

func parseRuntimeTargetProfile(raw string, runtimeArtifactCarrier bool, p *emitRuntimeTargetProfileParam, targets []types.RuntimeTarget) (*types.RuntimeTargetProfile, string, []string) {
	if p == nil {
		if runtimeArtifactCarrier {
			return nil, "runtime_target_profile object missing — declare named_target, no_named_target, or unspecified; do not let entities or exploration cursors stand in for user target authority", nil
		}
		return &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNotApplicable}, "", nil
	}
	if strings.TrimSpace(p.Declaration) == "" || p.Confidence == nil {
		var missing []string
		if strings.TrimSpace(p.Declaration) == "" {
			missing = append(missing, "declaration")
		}
		if p.Confidence == nil {
			missing = append(missing, "confidence")
		}
		return nil, "runtime_target_profile missing required field(s): " + strings.Join(missing, ", "), nil
	}
	if *p.Confidence < 0 || *p.Confidence > 1 {
		return nil, fmt.Sprintf("runtime_target_profile.confidence %.2f out of [0,1]", *p.Confidence), nil
	}
	declaration := types.RuntimeTargetDeclaration(strings.TrimSpace(p.Declaration))
	if !declaration.IsValid() {
		return nil, fmt.Sprintf(
			"runtime_target_profile.declaration %q is invalid; use one of %s",
			p.Declaration, strings.Join(runtimeTargetDeclarationValues(), ", "),
		), nil
	}
	if !runtimeArtifactCarrier {
		if len(targets) == 0 {
			var warnings []string
			if declaration != types.RuntimeTargetDeclarationNotApplicable && declaration != types.RuntimeTargetDeclarationUnspecified {
				warnings = append(warnings, "runtime_target_profile normalized to not_applicable because no runtime artifact carrier is present")
			}
			return &types.RuntimeTargetProfile{
				Declaration: types.RuntimeTargetDeclarationNotApplicable,
				Confidence:  *p.Confidence,
			}, "", warnings
		}
		// A schema-valid named declaration plus typed targets is itself a
		// precise runtime-request carrier. This supports explicit trace/log
		// paths whose preflight attachment is established after analysis and
		// keeps unit/adapter callers from having to manufacture an artifact.
		if declaration != types.RuntimeTargetDeclarationNamedTarget {
			return nil, fmt.Sprintf("runtime_targets conflict with runtime_target_profile %s outside an attached runtime artifact; declare named_target with an anchored source_quote", declaration), nil
		}
	}

	profile := &types.RuntimeTargetProfile{
		Declaration: declaration,
		Confidence:  *p.Confidence,
		Rationale:   strings.TrimSpace(p.Rationale),
	}
	switch declaration {
	case types.RuntimeTargetDeclarationNamedTarget:
		quote := strings.TrimSpace(p.SourceQuote)
		if quote == "" || !sourceQuotePresentInCurrentRequest(raw, quote) {
			return nil, "runtime_target_profile named_target requires source_quote copied verbatim from the current request", nil
		}
		if len(targets) == 0 {
			return nil, "runtime_target_profile named_target requires at least one structurally valid runtime_targets entry; emit the named process/thread instead of omitting it", nil
		}
		for i, target := range targets {
			if strings.TrimSpace(target.Source) != "user_explicit" {
				return nil, fmt.Sprintf("runtime_targets[%d].source must be user_explicit under runtime_target_profile named_target", i), nil
			}
		}
		profile.SourceQuote = quote
	case types.RuntimeTargetDeclarationNoNamedTarget, types.RuntimeTargetDeclarationUnspecified:
		if len(targets) > 0 {
			return nil, fmt.Sprintf("runtime_target_profile %s conflicts with %d runtime_targets entries; declare named_target with an anchored source_quote or remove the targets", declaration, len(targets)), nil
		}
	case types.RuntimeTargetDeclarationNotApplicable:
		return nil, "runtime_target_profile not_applicable conflicts with the attached/referenced runtime request; use named_target, no_named_target, or unspecified", nil
	}
	return profile, "", nil
}

func parseRuntimeQuestionProfile(raw string, runtimeArtifactCarrier bool, p *emitRuntimeQuestionProfileParam, _ bool) (*types.RuntimeQuestionProfile, string, []string) {
	if p == nil {
		if runtimeArtifactCarrier {
			return nil, "runtime_question_profile object missing — declare bounded_fact_set, causal_diagnosis, relation_analysis, system_overview, or unspecified; intent/scenario labels do not substitute for runtime answer breadth", nil
		}
		return &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeNotApplicable}, "", nil
	}
	var missing []string
	if strings.TrimSpace(p.Scope) == "" {
		missing = append(missing, "scope")
	}
	if p.Confidence == nil {
		missing = append(missing, "confidence")
	}
	if len(missing) > 0 {
		return nil, "runtime_question_profile missing required field(s): " + strings.Join(missing, ", "), nil
	}
	if *p.Confidence < 0 || *p.Confidence > 1 {
		return nil, fmt.Sprintf("runtime_question_profile.confidence %.2f out of [0,1]", *p.Confidence), nil
	}
	scope := types.RuntimeQuestionScope(strings.TrimSpace(p.Scope))
	if !scope.IsValid() {
		return nil, fmt.Sprintf(
			"runtime_question_profile.scope %q is invalid; use one of %s",
			p.Scope, strings.Join(runtimeQuestionScopeValues(), ", "),
		), nil
	}
	if !runtimeArtifactCarrier {
		var warnings []string
		if scope != types.RuntimeQuestionScopeNotApplicable && scope != types.RuntimeQuestionScopeUnspecified {
			warnings = append(warnings, "runtime_question_profile normalized to not_applicable because no runtime artifact carrier is present")
		}
		return &types.RuntimeQuestionProfile{
			Scope:      types.RuntimeQuestionScopeNotApplicable,
			Confidence: *p.Confidence,
		}, "", warnings
	}
	if scope == types.RuntimeQuestionScopeNotApplicable {
		return nil, "runtime_question_profile not_applicable conflicts with the attached/referenced runtime request", nil
	}
	profile := &types.RuntimeQuestionProfile{
		Scope:      scope,
		Confidence: *p.Confidence,
		Rationale:  strings.TrimSpace(p.Rationale),
	}
	if scope == types.RuntimeQuestionScopeBoundedFactSet {
		if len(p.FactFamilies) == 0 {
			return nil, "runtime_question_profile bounded_fact_set requires one or more fact_families; declare the requested observed value families instead of leaving principal-value publication ambiguous", nil
		}
		seen := make(map[types.RuntimeQuestionFactFamily]bool, len(p.FactFamilies))
		for _, rawFamily := range p.FactFamilies {
			family := types.RuntimeQuestionFactFamily(strings.TrimSpace(rawFamily))
			if !family.IsValid() {
				return nil, fmt.Sprintf("runtime_question_profile.fact_families contains invalid value %q; use one of %s", rawFamily, strings.Join(runtimeQuestionFactFamilyValues(), ", ")), nil
			}
			if !seen[family] {
				seen[family] = true
				profile.FactFamilies = append(profile.FactFamilies, family)
			}
		}
	} else if len(p.FactFamilies) > 0 {
		return nil, "runtime_question_profile.fact_families is only valid with scope=bounded_fact_set", nil
	}
	if scope == types.RuntimeQuestionScopeUnspecified {
		return profile, "", nil
	}
	quote := strings.TrimSpace(p.SourceQuote)
	if quote == "" {
		return profile, "", []string{"runtime_question_profile.source_quote omitted; retained typed scope because the quote is audit-only"}
	}
	if !sourceQuotePresentInCurrentRequest(raw, quote) {
		return profile, "", []string{"runtime_question_profile ignored unanchored source_quote; retained typed scope because downstream consumers do not use raw quote text"}
	}
	profile.SourceQuote = quote
	return profile, "", nil
}

// validateRuntimeQuestionProfileConsistency keeps the dedicated runtime
// breadth declaration coherent with the analyzer's other typed diagnosis
// lanes. causal_diagnosis is the full root-cause/attribution contract, not a
// synonym for "one of the requested facts is a relation". If every typed
// diagnosis carrier is negative, the analyzer must choose bounded_fact_set
// for finitely named facts or relation_analysis for an actual path/topology.
//
// This is a structural emit-time consistency check over schema-validated
// enums and booleans. It does not inspect the source quote, raw request, model
// reasoning, or answer prose, and it rejects for model retry instead of
// silently replacing the model's classification.
func validateRuntimeQuestionProfileConsistency(
	profile *types.RuntimeQuestionProfile,
	intent types.Intent,
	scenario types.Scenario,
	predicates types.SemanticPredicates,
	diagnostic types.DiagnosticIntentProfile,
) string {
	if profile == nil || profile.Scope != types.RuntimeQuestionScopeCausalDiagnosis {
		return ""
	}
	if intent == types.IntentRootCause ||
		predicates.IsDiagnosticQuestion ||
		diagnostic.RequiresDiagnosticRootCause() {
		return ""
	}
	switch scenario {
	case types.ScenarioRootCause, types.ScenarioPerformanceBottleneck:
		return ""
	}
	return "runtime_question_profile.scope=causal_diagnosis requires a typed diagnosis/attribution carrier (intent=root_cause, predicates.is_diagnostic_question=true, diagnostic_profile diagnostic/risk/regression, or scenario=root_cause/performance_bottleneck); for a finite set of observed fields use bounded_fact_set even when one field is a direct peer/transaction/waker relation, and for a requested caller/wakeup/IPC/dependency path or topology use relation_analysis"
}

func parseHistorySelectionProfile(raw string, isHistoryLookup bool, p *emitHistorySelectionProfileParam) (*types.HistorySelectionProfile, string, []string) {
	if p == nil {
		if !isHistoryLookup {
			return &types.HistorySelectionProfile{
				Mode:     types.HistorySelectionNotApplicable,
				ItemKind: types.HistorySelectionItemNotApplicable,
			}, "", nil
		}
		return nil, "history_selection_profile object missing — declare latest/earliest/recent-N/range selection for history lookup or not_applicable outside repository history", nil
	}
	var missing []string
	if strings.TrimSpace(p.Mode) == "" {
		missing = append(missing, "mode")
	}
	if strings.TrimSpace(p.ItemKind) == "" {
		missing = append(missing, "item_kind")
	}
	if p.Confidence == nil {
		missing = append(missing, "confidence")
	}
	if len(missing) > 0 {
		return nil, "history_selection_profile missing required field(s): " + strings.Join(missing, ", "), nil
	}
	if *p.Confidence < 0 || *p.Confidence > 1 {
		return nil, fmt.Sprintf("history_selection_profile.confidence %.2f out of [0,1]", *p.Confidence), nil
	}
	mode := types.HistorySelectionMode(strings.TrimSpace(p.Mode))
	if !mode.IsValid() {
		return nil, fmt.Sprintf("history_selection_profile.mode %q is invalid; use one of %s", p.Mode, strings.Join(historySelectionModeValues(), ", ")), nil
	}
	itemKind := types.HistorySelectionItemKind(strings.TrimSpace(p.ItemKind))
	if !itemKind.IsValid() {
		return nil, fmt.Sprintf("history_selection_profile.item_kind %q is invalid; use one of %s", p.ItemKind, strings.Join(historySelectionItemKindValues(), ", ")), nil
	}
	if !isHistoryLookup {
		var warnings []string
		if mode != types.HistorySelectionNotApplicable || itemKind != types.HistorySelectionItemNotApplicable {
			warnings = append(warnings, "history_selection_profile normalized to not_applicable because predicates.is_history_lookup=false")
		}
		return &types.HistorySelectionProfile{
			Mode:       types.HistorySelectionNotApplicable,
			ItemKind:   types.HistorySelectionItemNotApplicable,
			Confidence: *p.Confidence,
		}, "", warnings
	}
	if mode == types.HistorySelectionNotApplicable || itemKind == types.HistorySelectionItemNotApplicable {
		return nil, "history_selection_profile not_applicable conflicts with predicates.is_history_lookup=true", nil
	}
	profile := &types.HistorySelectionProfile{
		Mode:       mode,
		ItemKind:   itemKind,
		Count:      p.Count,
		Confidence: *p.Confidence,
		Rationale:  strings.TrimSpace(p.Rationale),
	}
	if mode == types.HistorySelectionUnspecified {
		profile.Count = 0
		return profile, "", nil
	}
	quote := strings.TrimSpace(p.SourceQuote)
	if quote == "" || !sourceQuotePresentInCurrentRequest(raw, quote) {
		return nil, "history_selection_profile concrete mode requires source_quote copied verbatim from the current request", nil
	}
	profile.SourceQuote = quote
	var warnings []string
	switch mode {
	case types.HistorySelectionLatestOne, types.HistorySelectionEarliestOne:
		if profile.Count != 0 && profile.Count != 1 {
			warnings = append(warnings, "history_selection_profile count normalized to 1 for single-endpoint selection")
		}
		profile.Count = 1
	case types.HistorySelectionRecentN, types.HistorySelectionOldestN:
		if profile.Count <= 0 || profile.Count > 100 {
			return nil, "history_selection_profile recent_n/oldest_n requires count in [1,100]", nil
		}
	case types.HistorySelectionBoundedRange:
		profile.Count = 0
	}
	return profile, "", warnings
}

// emitRuntimeTargetMaxPID is the shared Linux PID_MAX_LIMIT sanity cap
// (types.RuntimeTargetMaxPID; F4 教义统一).
const emitRuntimeTargetMaxPID = types.RuntimeTargetMaxPID

func parseRuntimeTarget(p emitRuntimeTargetParam) (types.RuntimeTarget, string, bool) {
	kind := types.NormalizeRuntimeTargetKind(types.RuntimeTargetKind(p.Kind))
	if kind == types.RuntimeTargetKindUnknown {
		return types.RuntimeTarget{}, fmt.Sprintf("kind %q is invalid; use one of %s", p.Kind, strings.Join(runtimeTargetKindValues(), ", ")), false
	}
	if p.Confidence == nil {
		return types.RuntimeTarget{}, "confidence is required", false
	}
	if *p.Confidence < 0 || *p.Confidence > 1 {
		return types.RuntimeTarget{}, fmt.Sprintf("confidence %.2f out of [0,1]", *p.Confidence), false
	}
	pid := 0
	if p.PID != nil {
		pid = *p.PID
	}
	thread := strings.TrimSpace(p.Thread)
	if pid <= 0 && thread == "" {
		return types.RuntimeTarget{}, "pid or thread is required", false
	}
	if pid < 0 || pid > emitRuntimeTargetMaxPID {
		return types.RuntimeTarget{}, fmt.Sprintf("pid %d is out of supported range", pid), false
	}
	switch kind {
	case types.RuntimeTargetKindProcess:
		if pid <= 0 {
			return types.RuntimeTarget{}, "process target requires pid", false
		}
	case types.RuntimeTargetKindThread:
		if pid <= 0 && thread == "" {
			return types.RuntimeTarget{}, "thread target requires pid or thread", false
		}
	}
	source, sourceWarning := normalizeRuntimeTargetSource(p.Source)
	return types.RuntimeTarget{
		Kind:        kind,
		PID:         pid,
		Thread:      thread,
		Source:      source,
		Confidence:  *p.Confidence,
		Description: strings.TrimSpace(p.Description),
	}, sourceWarning, true
}

func normalizeRuntimeTargetSource(raw string) (string, string) {
	source := strings.TrimSpace(raw)
	if source == "" {
		return "", ""
	}
	switch source {
	case "user_explicit", "artifact_metadata", "tool_handoff":
		return source, ""
	default:
		return "", fmt.Sprintf("source %q is invalid; cleared optional provenance; use one of user_explicit, artifact_metadata, tool_handoff", source)
	}
}

func runtimeArtifactValueProfileFromFieldValueParam(ctx *types.BusContext, p *emitFieldValueProfileParam, reason string) (*types.RuntimeArtifactValueProfile, string) {
	if p == nil || p.IsFieldValueLookup == nil || !*p.IsFieldValueLookup {
		return nil, ""
	}
	value := strings.TrimSpace(p.Literal)
	if value == "" {
		return nil, ""
	}
	target := firstNonEmptyEmitAnalysisString(p.Target, p.SourceQuote, p.Rationale)
	if target == "" {
		target = "runtime artifact value"
	}
	literalKind := types.FieldValueLiteralKind(strings.TrimSpace(p.LiteralKind))
	if !literalKind.IsValid() {
		literalKind = types.FieldValueLiteralUnknown
	}
	confidence := 0.5
	if p.Confidence != nil && *p.Confidence >= 0 && *p.Confidence <= 1 {
		confidence = *p.Confidence
	}
	refs := runtimeArtifactValueContextRefs(ctx)
	profile := &types.RuntimeArtifactValueProfile{
		IsArtifactValueLookup: true,
		Target:                target,
		Value:                 value,
		LiteralKind:           literalKind,
		ArtifactRefs:          refs,
		Confidence:            confidence,
		Rationale:             strings.TrimSpace(p.Rationale),
	}
	if !profile.Active() {
		return nil, ""
	}
	return profile, "converted runtime-artifact field_value_profile to artifact_value_profile: " + reason
}

func runtimeArtifactValueContextRefs(ctx *types.BusContext) []string {
	if ctx == nil {
		return nil
	}
	var refs []string
	add := func(ref string) {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			return
		}
		for _, existing := range refs {
			if existing == ref {
				return
			}
		}
		refs = append(refs, ref)
	}
	if strings.TrimSpace(ctx.AttachedLog) != "" {
		add("attached_log")
	}
	if strings.TrimSpace(ctx.AttachedHitrace) != "" {
		add("attached_trace")
	}
	if ctx.Mutable != nil {
		if log := ctx.Mutable.LogTriage(); log != nil {
			add("log_triage")
		}
		if perf := ctx.Mutable.PerfTrace(); perf != nil {
			if strings.TrimSpace(perf.Meta.Source) != "" {
				add(perf.Meta.Source)
			}
			add("perf_trace")
		}
	}
	return refs
}

func trimNonEmptyStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		out = append(out, raw)
	}
	return out
}

func firstNonEmptyEmitAnalysisString(values ...string) string {
	for _, value := range values {
		if s := strings.TrimSpace(value); s != "" {
			return s
		}
	}
	return ""
}

func shouldDropInvalidOptionalFieldValueProfile(artifactOnlyRuntime bool, predicates types.SemanticPredicates, currentSource *types.CurrentSourceExplanationProfile, p *emitFieldValueProfileParam) bool {
	if artifactOnlyRuntime {
		return true
	}
	if p == nil || p.IsFieldValueLookup == nil || !*p.IsFieldValueLookup {
		return false
	}
	if _, _, _, ok := types.ParseFieldValueTarget(p.Target); ok {
		return false
	}
	if predicates.IsScalarAnswer && predicates.IsCountQuestion {
		return true
	}
	return predicates.IsScalarAnswer && currentSource != nil && currentSource.Active()
}

func invalidOptionalFieldValueProfileWarning(artifactOnlyRuntime bool, err string) string {
	if artifactOnlyRuntime {
		return "dropped invalid optional field_value_profile for observation-only runtime artifact: " + err
	}
	return "dropped invalid optional field_value_profile for generic scalar/current-source request: " + err
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

func parseAnswerRoleProfile(raw string, p *emitAnswerRoleProfileParam) (*types.AnswerRoleProfile, string, []string) {
	if p == nil {
		return nil, "answer_role_profile object missing — emit `answer_role_profile` with is_role_binding_requested set to true or false, plus confidence in [0,1]", nil
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
		), nil
	}
	if *p.Confidence < 0 || *p.Confidence > 1 {
		return nil, fmt.Sprintf("answer_role_profile.confidence %.2f out of [0,1]", *p.Confidence), nil
	}
	if !*p.IsRoleBindingRequested {
		return nil, "", nil
	}
	var warnings []string
	sourceQuotes := trimStringSlice(p.SourceQuotes)
	if len(sourceQuotes) == 0 {
		return nil, "", []string{"answer_role_profile auto-softened: source_quotes missing while is_role_binding_requested=true; optional positive role binding ignored"}
	}
	anchoredQuotes := make([]string, 0, len(sourceQuotes))
	for _, quote := range sourceQuotes {
		if sourceQuotePresentInCurrentRequest(raw, quote) {
			anchoredQuotes = append(anchoredQuotes, quote)
			continue
		}
		warnings = append(warnings, fmt.Sprintf("answer_role_profile ignored unanchored source_quote %q", quote))
	}
	if len(anchoredQuotes) == 0 {
		warnings = append(warnings, "answer_role_profile auto-softened: no source_quote is copied verbatim from the current request; optional positive role binding ignored")
		return nil, "", warnings
	}
	roles := make([]types.AnswerCandidateRole, 0, len(p.RequiredCandidateRoles))
	seen := make(map[types.AnswerCandidateRole]struct{}, len(p.RequiredCandidateRoles))
	for _, rawRole := range p.RequiredCandidateRoles {
		role, ok := types.NormalizeAnswerCandidateRole(rawRole)
		if !ok || role == types.AnswerCandidateRoleUnknown {
			return nil, fmt.Sprintf(
				"answer_role_profile.required_candidate_roles contains invalid role %q; use one of %s",
				rawRole, strings.Join(answerCandidateRoleValues(), ", "),
			), nil
		}
		if _, dup := seen[role]; dup {
			continue
		}
		seen[role] = struct{}{}
		roles = append(roles, role)
	}
	if len(roles) == 0 {
		return nil, "answer_role_profile.required_candidate_roles is required when is_role_binding_requested=true", nil
	}
	return &types.AnswerRoleProfile{
		IsRoleBindingRequested: true,
		RequiredCandidateRoles: roles,
		SourceQuotes:           anchoredQuotes,
		Confidence:             *p.Confidence,
		Rationale:              strings.TrimSpace(p.Rationale),
	}, "", warnings
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

func parseRequestedAnswerDimensions(raw string, p *emitRequestedAnswerDimensionsParam) (*types.RequestedAnswerDimensionProfile, []types.CurrentSourceObligationSignal, string, []string) {
	if p == nil {
		return nil, nil, "", nil
	}
	var missing []string
	if p.IsDimensionedAnswer == nil {
		missing = append(missing, "is_dimensioned_answer")
	}
	if p.Confidence == nil {
		missing = append(missing, "confidence")
	}
	if len(missing) > 0 {
		return nil, nil, fmt.Sprintf(
			"requested_answer_dimensions missing required field(s): %s",
			strings.Join(missing, ", "),
		), nil
	}
	if *p.Confidence < 0 || *p.Confidence > 1 {
		return nil, nil, fmt.Sprintf("requested_answer_dimensions.confidence %.2f out of [0,1]", *p.Confidence), nil
	}
	if !*p.IsDimensionedAnswer {
		return nil, nil, "", nil
	}
	dimensions := make([]types.RequestedAnswerDimension, 0, len(p.Dimensions))
	for _, dim := range p.Dimensions {
		required := true
		if dim.Required != nil {
			required = *dim.Required
		}
		dimensions = append(dimensions, types.RequestedAnswerDimension{
			Label:       dim.Label,
			Role:        types.NormalizeRequestedAnswerDimensionRole(dim.Role),
			SourceQuote: dim.SourceQuote,
			Required:    required,
			Index:       dim.Index,
		})
	}
	profile, warnings := types.NormalizeRequestedAnswerDimensionProfile(raw, &types.RequestedAnswerDimensionProfile{
		IsDimensionedAnswer: true,
		Dimensions:          dimensions,
		Confidence:          *p.Confidence,
		Rationale:           p.Rationale,
	})
	signals := types.CurrentSourceObligationSignalsFromRequestedDimensions(dimensions, profile)
	return profile, signals, "", warnings
}

func parseCurrentSourceExplanationProfile(raw string, p *emitCurrentSourceExplanationParam) (*types.CurrentSourceExplanationProfile, string, []string) {
	if p == nil {
		return nil, "", nil
	}
	var warnings []string
	if p.IsCurrentSourceExplanationRequested == nil {
		return nil, "", []string{"current_source_explanation_profile ignored: is_current_source_explanation_requested missing"}
	}
	if !*p.IsCurrentSourceExplanationRequested {
		return nil, "", nil
	}
	confidence := 0.5
	if p.Confidence == nil {
		warnings = append(warnings, "current_source_explanation_profile confidence missing; defaulted to 0.5")
	} else {
		confidence = *p.Confidence
		if confidence < 0 || confidence > 1 {
			warnings = append(warnings, fmt.Sprintf("current_source_explanation_profile confidence %.2f out of [0,1]; clamped", confidence))
		}
	}
	modes := make([]types.CurrentSourceExplanationMode, 0, len(p.Modes))
	for _, mode := range p.Modes {
		modes = append(modes, types.NormalizeCurrentSourceExplanationMode(mode))
	}
	profile, normalizeWarnings := types.NormalizeCurrentSourceExplanationProfile(raw, &types.CurrentSourceExplanationProfile{
		IsCurrentSourceExplanationRequested: true,
		Modes:                               modes,
		SourceQuotes:                        p.SourceQuotes,
		TargetTerms:                         p.TargetTerms,
		Confidence:                          confidence,
		Rationale:                           p.Rationale,
	})
	warnings = append(warnings, normalizeWarnings...)
	return profile, "", warnings
}

func parseExternalObservationPolicy(raw string, p *emitExternalObservationPolicyParam) (*types.ExternalObservationPolicy, string, []string) {
	if p == nil {
		return nil, "", nil
	}
	var warnings []string
	if p.Confidence == nil {
		warnings = append(warnings, "external_observation_policy confidence missing; defaulted to 0.5")
	}
	confidence := 0.5
	if p.Confidence != nil {
		confidence = *p.Confidence
		if confidence < 0 || confidence > 1 {
			warnings = append(warnings, fmt.Sprintf("external_observation_policy confidence %.2f out of [0,1]; clamped", confidence))
		}
	}
	if confidence < 0 {
		confidence = 0
	} else if confidence > 1 {
		confidence = 1
	}
	mode := types.NormalizeExternalObservationCurrentSourceMode(p.CurrentSourceMode)
	exclusionKind := types.NormalizeExternalObservationCurrentSourceExclusionKind(p.ExclusionKind)
	artifactCitationMode := types.NormalizeExternalObservationArtifactCitationMode(p.ArtifactCitationMode)
	var quotes []string
	exclusionQuote := strings.TrimSpace(p.CurrentSourceExclusionQuote)
	exclusionQuoteAnchored := false
	if exclusionQuote != "" {
		if sourceQuoteAnchoredInCurrentRequest(raw, exclusionQuote) {
			quotes = append(quotes, exclusionQuote)
			exclusionQuoteAnchored = true
		} else {
			warnings = append(warnings, "external_observation_policy.current_source_exclusion_quote ignored because it is not copied from the current request")
		}
	}
	for _, quote := range p.ArtifactCitationQuotes {
		trimmed := strings.TrimSpace(quote)
		if trimmed == "" {
			continue
		}
		if sourceQuoteAnchoredInCurrentRequest(raw, trimmed) {
			quotes = append(quotes, trimmed)
			continue
		}
		warnings = append(warnings, "external_observation_policy.artifact_citation_quotes entry ignored because it is not copied from the current request")
	}
	for _, quote := range p.SourceQuotes {
		trimmed := strings.TrimSpace(quote)
		if trimmed == "" {
			continue
		}
		if sourceQuoteAnchoredInCurrentRequest(raw, trimmed) {
			quotes = append(quotes, trimmed)
			continue
		}
		warnings = append(warnings, "external_observation_policy.source_quotes compatibility entry ignored because it is not copied from the current request")
	}
	if mode != types.ExternalObservationCurrentSourceExclude {
		exclusionKind = types.ExternalObservationSourceExclusionNone
	}
	if mode == types.ExternalObservationCurrentSourceExclude &&
		exclusionKind != types.ExternalObservationSourceExclusionExplicitUserBoundary {
		warnings = append(warnings, "external_observation_policy exclude ignored because exclusion_kind is not explicit_user_exclusion")
		mode = types.ExternalObservationCurrentSourceDefault
		exclusionKind = types.ExternalObservationSourceExclusionNone
	}
	if mode == types.ExternalObservationCurrentSourceExclude && !exclusionQuoteAnchored {
		warnings = append(warnings, "external_observation_policy exclude ignored because no current_source_exclusion_quote survived current-request provenance validation; artifact citation or legacy combined quotes cannot close the current-source lane")
		mode = types.ExternalObservationCurrentSourceDefault
		exclusionKind = types.ExternalObservationSourceExclusionNone
	}
	if mode == types.ExternalObservationCurrentSourceDefault &&
		artifactCitationMode == types.ExternalObservationArtifactCitationDefault &&
		exclusionKind == types.ExternalObservationSourceExclusionNone &&
		len(quotes) == 0 &&
		strings.TrimSpace(p.Rationale) == "" {
		return nil, "", warnings
	}
	return &types.ExternalObservationPolicy{
		CurrentSourceMode:    mode,
		ExclusionKind:        exclusionKind,
		ArtifactCitationMode: artifactCitationMode,
		SourceQuotes:         dedupeTrimmedStrings(quotes),
		Confidence:           confidence,
		Rationale:            strings.TrimSpace(p.Rationale),
	}, "", warnings
}

func sourceQuotePresentInCurrentRequest(raw, quote string) bool {
	raw = strings.TrimSpace(raw)
	quote = strings.TrimSpace(quote)
	return raw != "" && quote != "" && strings.Contains(raw, quote)
}

func dedupeTrimmedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	return out
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

func validateExactTargets(raw string, in []string, requiredFiles []emitRequiredFileParam, predicates types.SemanticPredicates, sourceInventory *emitSourceInventoryProfileParam, answerSubject types.AnswerSubject, entities []string) ([]string, string, string) {
	if len(in) == 0 {
		return nil, "", ""
	}
	validated := types.MentionedEntitiesFromRawRequest(raw, in)
	if len(validated) == len(in) {
		if filtered, warning := demoteRequiredFileContextExactTargets(raw, validated, requiredFiles, answerSubject, entities); warning != "" {
			return filtered, "", warning
		}
		return validated, "", ""
	}
	if len(validated) == 0 {
		if exactTargetsAreRequiredFileHints(in, requiredFiles) &&
			(predicates.IsCategoryEnumeration || sourceInventoryRequested(sourceInventory)) {
			return nil, "", "dropped exact_targets that were not current-request verbatim but already appeared in required_files for a scoped inventory/enumeration"
		}
		return nil, "", "dropped invalid optional exact_targets: none were explicitly present in the current request text"
	}
	var invalid []string
	seen := make(map[string]bool, len(validated))
	for _, item := range validated {
		seen[strings.ToLower(strings.TrimSpace(item))] = true
	}
	lowerRaw := strings.ToLower(strings.ReplaceAll(raw, `\`, `/`))
	for _, item := range in {
		trimmed := strings.TrimSpace(item)
		key := strings.ToLower(trimmed)
		if key == "" || seen[key] {
			continue
		}
		// EVALFIX-1 (eval specimen qf_config_precedence 2026-07-30): a
		// target that IS verbatim in the request must never be dropped
		// or reported as "not copied verbatim". MentionedEntities
		// dedupes by normalized resolution key, so "--pipeline-max-steps"
		// and "pipeline_max_steps" collapse to one key and the second
		// surface vanished — then this lane branded the verbatim
		// survivor invalid: a false diagnostic AND lost user intent.
		// The direct raw check is the precise signal.
		if lowerRaw != "" && types.RawRequestExplicitlyMentionsEntity(raw, trimmed) {
			validated = append(validated, trimmed)
			seen[key] = true
			continue
		}
		invalid = append(invalid, trimmed)
	}
	if len(invalid) == 0 {
		return validated, "", ""
	}
	sort.Strings(invalid)
	return validated, "", fmt.Sprintf(
		"dropped invalid optional exact_targets not copied verbatim from the current request text: %s",
		strings.Join(invalid, ", "),
	)
}

func demoteRequiredFileContextExactTargets(raw string, exactTargets []string, requiredFiles []emitRequiredFileParam, answerSubject types.AnswerSubject, entities []string) ([]string, string) {
	if len(exactTargets) == 0 || len(requiredFiles) == 0 {
		return exactTargets, ""
	}
	if !exactTargetHasNonFileFocus(raw, answerSubject, entities) {
		return exactTargets, ""
	}
	required := make(map[string]struct{}, len(requiredFiles))
	for _, file := range requiredFiles {
		canon := canonicalRequiredFilePath(file.Path)
		if canon == "" || !emitAnalysisLooksLikeFilePath(canon) {
			continue
		}
		required[strings.ToLower(canon)] = struct{}{}
	}
	if len(required) == 0 {
		return exactTargets, ""
	}
	out := make([]string, 0, len(exactTargets))
	demoted := 0
	for _, target := range exactTargets {
		canon := canonicalRequiredFilePath(target)
		if canon != "" && emitAnalysisLooksLikeFilePath(canon) {
			if _, ok := required[strings.ToLower(canon)]; ok {
				demoted++
				continue
			}
		}
		out = append(out, target)
	}
	if demoted == 0 {
		return exactTargets, ""
	}
	return out, "demoted exact_targets that duplicate required_files context paths; those files remain navigation hints, not primary exact-resolution targets"
}

func exactTargetHasNonFileFocus(raw string, answerSubject types.AnswerSubject, entities []string) bool {
	switch answerSubject.Kind {
	case types.SubjectUnknown, types.SubjectFilePath:
	default:
		return true
	}
	for _, entity := range types.MentionedEntitiesFromRawRequest(raw, entities) {
		if strings.TrimSpace(entity) == "" {
			continue
		}
		if !emitAnalysisLooksLikeFilePath(entity) {
			return true
		}
	}
	return false
}

func emitAnalysisLooksLikeFilePath(raw string) bool {
	raw = strings.TrimSpace(strings.Trim(raw, "`\"' "))
	if raw == "" {
		return false
	}
	if strings.ContainsAny(raw, `/\`) {
		return true
	}
	lower := strings.ToLower(raw)
	for _, ext := range []string{
		".go", ".java", ".kt", ".kts", ".scala", ".c", ".h", ".cc", ".cpp", ".cxx", ".hpp",
		".rs", ".py", ".rb", ".php", ".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs",
		".swift", ".m", ".mm", ".cs", ".fs", ".fsx", ".erl", ".ex", ".exs", ".clj",
		".yaml", ".yml", ".json", ".jsonc", ".toml", ".ini", ".xml", ".proto", ".graphql",
		".gradle", ".properties", ".cfg", ".conf", ".env", ".md", ".rst", ".txt", ".sh",
		".bash", ".zsh", ".fish", ".sql", ".html", ".css", ".scss", ".less",
	} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	switch lower {
	case "makefile", "dockerfile", "gemfile", "rakefile", "buildfile", "podfile":
		return true
	}
	return false
}

func exactTargetsAreRequiredFileHints(targets []string, requiredFiles []emitRequiredFileParam) bool {
	if len(targets) == 0 || len(requiredFiles) == 0 {
		return false
	}
	required := make(map[string]struct{}, len(requiredFiles))
	for _, file := range requiredFiles {
		if canon := canonicalRequiredFilePath(file.Path); canon != "" {
			required[strings.ToLower(canon)] = struct{}{}
		}
	}
	if len(required) == 0 {
		return false
	}
	for _, target := range targets {
		canon := canonicalRequiredFilePath(target)
		if canon == "" {
			return false
		}
		if _, ok := required[strings.ToLower(canon)]; !ok {
			return false
		}
	}
	return true
}

func sourceInventoryRequested(p *emitSourceInventoryProfileParam) bool {
	return p != nil && p.IsSourceInventory != nil && *p.IsSourceInventory
}

func emitAnalysisObservationOnlyRuntimeArtifact(ctx *types.BusContext, policy *types.ExternalObservationPolicy) bool {
	if ctx == nil {
		return false
	}
	rm := types.RequestModel{}
	if ctx.AnalysisIR != nil {
		rm = ctx.AnalysisIR.RequestModel
	}
	if policy != nil {
		rm.ExternalObservationPolicy = policy
	}
	if ctx.Mutable != nil {
		if bundle := ctx.Mutable.LogTriage(); bundle != nil {
			rm.LogTriage = bundle
		}
		if bundle := ctx.Mutable.PerfTrace(); bundle != nil {
			rm.PerfTrace = bundle
		}
	}
	return rm.HasObservationOnlyRuntimeArtifact()
}

// emitAnalysisObservationOnlyRuntimeArtifactForSourceInventoryGuards is the
// ctx-aware observation-only predicate for the source-inventory synthesis and
// withdrawal guards (§29.122 LENSBURN 病A). The RequestModel-only predicate
// goes blind on large traces: perf_triage skips bundle materialization above
// its size gate (perf_triage_llm_max_bytes, 512KiB default), so rm.PerfTrace
// stays nil for exactly the attached traces that most need the
// observation-only posture, and trace-only runs mint a source-inventory lens
// window over a repo that has no inventory to enumerate. The Run-entry
// RuntimeArtifactPreflight profile is the deterministic same-carrier signal
// TOOLWIN already consumes for trace-tool-window admission; when the
// corresponding triage bundle is ABSENT, preflight artifact presence is the
// external-runtime-artifact carrier equivalent. A materialized bundle keeps
// full authority over its own lane (the preflight arm never overrides a
// bundle that resolved artifact frames into current-source files). The typed
// policy conjunction (ExcludesCurrentSource) and the current-verification
// anchor negation are preserved unchanged — both remain precise typed
// signals, so this hard guard still routes exclusively on precise inputs.
func emitAnalysisObservationOnlyRuntimeArtifactForSourceInventoryGuards(ctx *types.BusContext, rm types.RequestModel) bool {
	if rm.HasObservationOnlyRuntimeArtifact() {
		return true
	}
	if ctx == nil {
		return false
	}
	traceWithoutBundle := rm.PerfTrace == nil && ctx.RuntimeArtifactPreflight.HasTraceArtifact()
	logWithoutBundle := rm.LogTriage == nil && ctx.RuntimeArtifactPreflight.HasLogArtifact()
	if !traceWithoutBundle && !logWithoutBundle {
		return false
	}
	if rm.ExternalObservationPolicy == nil || !rm.ExternalObservationPolicy.ExcludesCurrentSource() {
		return false
	}
	return !rm.HasRuntimeArtifactCurrentVerificationAnchor()
}

func synthesizeExternalObservationPolicyFromRouteHint(ctx *types.BusContext, policy *types.ExternalObservationPolicy) (*types.ExternalObservationPolicy, string) {
	if ctx == nil || !ctx.TurnRouteHint.ExternalObservationParticipates() || !ctx.TurnRouteHint.RequiresCurrentSourceEvidence() {
		return policy, ""
	}
	if !emitAnalysisHasRuntimeArtifactCarrier(ctx) {
		return policy, ""
	}
	if policy != nil {
		if policy.ExcludesCurrentSource() ||
			policy.CurrentSourceMode == types.ExternalObservationCurrentSourceAllow {
			return policy, ""
		}
		clone := *policy
		clone.CurrentSourceMode = types.ExternalObservationCurrentSourceAllow
		if clone.Confidence <= 0 {
			clone.Confidence = 0.75
		}
		if strings.TrimSpace(clone.Rationale) == "" {
			clone.Rationale = "typed route metadata requires current checkout evidence for this external-observation turn"
		}
		return &clone, "external_observation_policy current_source_mode synthesized as allow from typed route metadata"
	}
	return &types.ExternalObservationPolicy{
		CurrentSourceMode: types.ExternalObservationCurrentSourceAllow,
		Confidence:        0.75,
		Rationale:         "typed route metadata requires current checkout evidence for this external-observation turn",
	}, "external_observation_policy current_source_mode synthesized as allow from typed route metadata"
}

func emitAnalysisHasRuntimeArtifactCarrier(ctx *types.BusContext) bool {
	if ctx == nil {
		return false
	}
	if types.NormalizeRuntimeArtifactPreflightProfile(ctx.RuntimeArtifactPreflight).HasRuntimeArtifact() {
		return true
	}
	if strings.TrimSpace(ctx.AttachedLog) != "" || strings.TrimSpace(ctx.AttachedHitrace) != "" {
		return true
	}
	if ctx.Mutable != nil {
		return ctx.Mutable.LogTriage() != nil || ctx.Mutable.PerfTrace() != nil
	}
	return false
}

func attachRuntimeArtifactsToRequestModel(ctx *types.BusContext, rm *types.RequestModel) {
	if ctx == nil || ctx.Mutable == nil || rm == nil {
		return
	}
	if rm.LogTriage == nil {
		rm.LogTriage = ctx.Mutable.LogTriage()
	}
	if rm.PerfTrace == nil {
		rm.PerfTrace = ctx.Mutable.PerfTrace()
	}
}

func normalizeRuntimeArtifactScalarIntent(artifactOnlyRuntime bool, intent types.Intent, scenario types.Scenario, kind string, predicates types.SemanticPredicates) (types.Intent, types.Scenario, string) {
	if !artifactOnlyRuntime || !predicates.IsScalarAnswer || kind != string(types.ReqReturnValue) {
		return intent, scenario, ""
	}
	switch intent {
	case types.IntentReturnValue:
		return intent, scenario, ""
	case types.IntentExplain, types.IntentUnknown:
		return types.IntentReturnValue, types.ScenarioGeneric, "normalized observation-only runtime scalar answer to intent=return_value"
	default:
		return intent, scenario, ""
	}
}

func normalizeRoleBindingScalarShape(
	intent types.Intent,
	kind string,
	axis types.PredicateAxis,
	predicates types.SemanticPredicates,
	profile *types.AnswerRoleProfile,
	answerSubject types.AnswerSubject,
	subTopics []types.SubTopic,
) (types.SemanticPredicates, types.AnswerSubject, []types.SubTopic, string) {
	if !roleBindingScalarShapeEligible(intent, predicates, profile, subTopics) {
		return predicates, answerSubject, subTopics, ""
	}
	role := profile.RequiredCandidateRoles[0]
	var reasons []string
	if !predicates.IsScalarAnswer {
		predicates.IsScalarAnswer = true
		reasons = append(reasons, "set is_scalar_answer=true from single required answer role")
	}
	if !predicates.IsRoleLocateLookup {
		predicates.IsRoleLocateLookup = true
		reasons = append(reasons, "set is_role_locate_lookup=true from single required answer role")
	}
	if predicates.IsRelationalLookup {
		predicates.IsRelationalLookup = false
		reasons = append(reasons, "cleared is_relational_lookup for scalar role binding")
	}
	if predicates.IsCategoryEnumeration {
		predicates.IsCategoryEnumeration = false
		reasons = append(reasons, "cleared is_category_enumeration for scalar role binding")
	}
	if len(trimSubTopicsForConsistency(subTopics)) == 1 {
		subTopics = nil
		reasons = append(reasons, "dropped exploratory sub_topics for scalar role binding")
	}
	if subject, reason := inferAnswerSubjectForRequiredCandidateRole(role, kind, axis); reason != "" {
		if roleBindingShouldRebindAnswerSubject(role, answerSubject.Kind, subject.Kind) {
			if answerSubject.Confidence > subject.Confidence {
				subject.Confidence = answerSubject.Confidence
			}
			answerSubject = subject
			reasons = append(reasons, reason)
		}
	}
	if len(reasons) == 0 {
		return predicates, answerSubject, subTopics, ""
	}
	return predicates, answerSubject, subTopics, strings.Join(reasons, "; ")
}

func roleBindingScalarShapeEligible(
	intent types.Intent,
	predicates types.SemanticPredicates,
	profile *types.AnswerRoleProfile,
	subTopics []types.SubTopic,
) bool {
	if profile == nil || !profile.Active() || len(profile.RequiredCandidateRoles) != 1 {
		return false
	}
	if intent == types.IntentEnumerate || intent == types.IntentRootCause {
		return false
	}
	if predicates.IsCountQuestion ||
		predicates.IsHistoryLookup ||
		predicates.IsDiagnosticQuestion ||
		predicates.HasPerMemberTable {
		return false
	}
	if predicates.IsCategoryEnumeration && !predicates.IsRoleLocateLookup {
		return false
	}
	return len(trimSubTopicsForConsistency(subTopics)) <= 1
}

func inferAnswerSubjectForRequiredCandidateRole(role types.AnswerCandidateRole, kind string, axis types.PredicateAxis) (types.AnswerSubject, string) {
	switch role {
	case types.AnswerCandidateRoleFunction,
		types.AnswerCandidateRoleMethod,
		types.AnswerCandidateRoleHelper:
		return types.AnswerSubject{Kind: types.SubjectFunctionName, EntityAxes: []string{"role → function"}, Confidence: 0.72},
			"defaulted answer_subject.kind=function_name from required candidate role"
	case types.AnswerCandidateRoleType,
		types.AnswerCandidateRoleAgent:
		return types.AnswerSubject{Kind: types.SubjectTypeName, EntityAxes: []string{"role → type"}, Confidence: 0.72},
			"defaulted answer_subject.kind=type_name from required candidate role"
	case types.AnswerCandidateRoleRoute:
		return types.AnswerSubject{Kind: types.SubjectHandlerRoute, EntityAxes: []string{"role → route"}, Confidence: 0.72},
			"defaulted answer_subject.kind=handler_route from required candidate role"
	case types.AnswerCandidateRoleConfigKey:
		return types.AnswerSubject{Kind: types.SubjectConfigKey, EntityAxes: []string{"role → config_key"}, Confidence: 0.72},
			"defaulted answer_subject.kind=config_key from required candidate role"
	case types.AnswerCandidateRoleFile,
		types.AnswerCandidateRoleConfigFile:
		return types.AnswerSubject{Kind: types.SubjectFilePath, EntityAxes: []string{"role → file_path"}, Confidence: 0.72},
			"defaulted answer_subject.kind=file_path from required candidate role"
	case types.AnswerCandidateRoleField:
		return types.AnswerSubject{Kind: types.SubjectStructField, EntityAxes: []string{"role → field"}, Confidence: 0.72},
			"defaulted answer_subject.kind=struct_field from required candidate role"
	case types.AnswerCandidateRoleLiteralValue,
		types.AnswerCandidateRoleToolName,
		types.AnswerCandidateRoleImportPath,
		types.AnswerCandidateRoleCommitHash,
		types.AnswerCandidateRoleGuardCondition:
		return types.AnswerSubject{Kind: types.SubjectStringLiteral, EntityAxes: []string{"role → literal"}, Confidence: 0.68},
			"defaulted answer_subject.kind=string_literal from required candidate role"
	case types.AnswerCandidateRoleBudgetCap,
		types.AnswerCandidateRoleAttemptCounter:
		return types.AnswerSubject{Kind: types.SubjectNumeric, EntityAxes: []string{"role → numeric"}, Confidence: 0.68},
			"defaulted answer_subject.kind=numeric from required candidate role"
	}
	return inferTypedRoleLocateAnswerSubject(kind, axis)
}

func roleBindingShouldRebindAnswerSubject(role types.AnswerCandidateRole, current, inferred types.AnswerSubjectKind) bool {
	if inferred == types.SubjectUnknown {
		return false
	}
	switch current {
	case types.SubjectUnknown, types.SubjectGeneric:
		return true
	}
	if current == inferred {
		return false
	}
	switch role {
	case types.AnswerCandidateRoleFunction,
		types.AnswerCandidateRoleMethod,
		types.AnswerCandidateRoleHelper,
		types.AnswerCandidateRoleAgent,
		types.AnswerCandidateRoleRoute,
		types.AnswerCandidateRoleConfigKey,
		types.AnswerCandidateRoleFile,
		types.AnswerCandidateRoleConfigFile,
		types.AnswerCandidateRoleField,
		types.AnswerCandidateRoleToolName,
		types.AnswerCandidateRoleBudgetCap,
		types.AnswerCandidateRoleAttemptCounter,
		types.AnswerCandidateRoleGuardCondition:
		return true
	default:
		return !roleLocateSubjectKindAllowed(current)
	}
}

func normalizeRuntimeArtifactRoleLocateAnswerSubject(ctx *types.BusContext, artifactOnlyRuntime bool, predicates types.SemanticPredicates, answerSubject types.AnswerSubject) (types.AnswerSubject, string) {
	if !artifactOnlyRuntime || !predicates.IsRoleLocateLookup || !predicates.IsScalarAnswer ||
		roleLocateSubjectKindAllowed(answerSubject.Kind) || !emitAnalysisRuntimeArtifactHasLineAnchors(ctx) {
		return answerSubject, ""
	}
	answerSubject.Kind = types.SubjectNumeric
	if answerSubject.Confidence <= 0 {
		answerSubject.Confidence = 0.8
	}
	return answerSubject, "defaulted answer_subject.kind=numeric for observation-only runtime artifact line/event-row lookup"
}

func normalizeTypedRoleLocateAnswerSubject(kind string, axis types.PredicateAxis, predicates types.SemanticPredicates, answerSubject types.AnswerSubject) (types.AnswerSubject, string) {
	if !predicates.IsRoleLocateLookup || !predicates.IsScalarAnswer ||
		roleLocateSubjectKindAllowed(answerSubject.Kind) {
		return answerSubject, ""
	}
	subject, reason := inferTypedRoleLocateAnswerSubject(kind, axis)
	if subject.Kind == types.SubjectUnknown {
		return answerSubject, ""
	}
	if answerSubject.Confidence > subject.Confidence {
		subject.Confidence = answerSubject.Confidence
	}
	return subject, reason
}

func inferTypedRoleLocateAnswerSubject(kind string, axis types.PredicateAxis) (types.AnswerSubject, string) {
	switch strings.TrimSpace(kind) {
	case "return_value":
		return types.AnswerSubject{Kind: types.SubjectReturnValue, EntityAxes: []string{"function → value"}, Confidence: 0.65},
			"defaulted answer_subject.kind=return_value from typed role-locate question_kind=return_value"
	case "call_chain":
		return types.AnswerSubject{Kind: types.SubjectFunctionName, EntityAxes: []string{"behavior → function"}, Confidence: 0.65},
			"defaulted answer_subject.kind=function_name from typed role-locate question_kind=call_chain"
	case "config_mapping":
		return types.AnswerSubject{Kind: types.SubjectConfigKey, EntityAxes: []string{"key → value"}, Confidence: 0.65},
			"defaulted answer_subject.kind=config_key from typed role-locate question_kind=config_mapping"
	}
	switch axis {
	case types.AxisReturn:
		return types.AnswerSubject{Kind: types.SubjectReturnValue, EntityAxes: []string{"function → value"}, Confidence: 0.6},
			"defaulted answer_subject.kind=return_value from typed role-locate predicate_axis=return"
	case types.AxisCall:
		return types.AnswerSubject{Kind: types.SubjectFunctionName, EntityAxes: []string{"behavior → function"}, Confidence: 0.6},
			"defaulted answer_subject.kind=function_name from typed role-locate predicate_axis=call"
	case types.AxisConfigure:
		return types.AnswerSubject{Kind: types.SubjectConfigKey, EntityAxes: []string{"key → value"}, Confidence: 0.6},
			"defaulted answer_subject.kind=config_key from typed role-locate predicate_axis=configure"
	}
	return types.AnswerSubject{}, ""
}

func emitAnalysisRuntimeArtifactHasLineAnchors(ctx *types.BusContext) bool {
	if ctx == nil {
		return false
	}
	checkLog := func(bundle *types.LogBundle) bool {
		if bundle == nil {
			return false
		}
		for _, obs := range bundle.Observations {
			if obs.LineStart > 0 {
				return true
			}
		}
		return false
	}
	checkPerf := func(bundle *types.PerfBundle) bool {
		if bundle == nil {
			return false
		}
		for _, obs := range bundle.Observations {
			if obs.LineStart > 0 {
				return true
			}
		}
		return false
	}
	if ctx.Mutable != nil {
		if checkLog(ctx.Mutable.LogTriage()) || checkPerf(ctx.Mutable.PerfTrace()) {
			return true
		}
	}
	if ctx.AnalysisIR != nil {
		return checkLog(ctx.AnalysisIR.RequestModel.LogTriage) || checkPerf(ctx.AnalysisIR.RequestModel.PerfTrace)
	}
	return false
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
	if p.Required == nil {
		return nil, "diagram_hint.required is missing — set it true only when the CURRENT request or typed Presentation Directive explicitly requires a visual; otherwise set it false"
	}
	return &types.DiagramHint{Kind: kind, Required: *p.Required}, ""
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
//   - DeclaredCount <= 0 → soft strip + warn because the LLM emitted an
//     inactive/unknown optional object. Downstream already treats nil as no
//     boundary, so a retry just to delete this object is waste.
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
	if p.DeclaredCount <= 0 {
		return nil, "", "ignored inactive enumeration_boundary with declared_count<=0"
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
	if len(h.RequiredFileHints) > 0 {
		if encoded := encodeRequiredFileHintSummaryPaths(h.RequiredFileHints); encoded != "" {
			fmt.Fprintf(&b, " required_files=%s", encoded)
		}
	} else if len(raw.RequiredFiles) > 0 {
		b.WriteString(" required_files=[]")
	}
	if encoded := encodeSubTopicSummaryLabels(rm.SubTopics); encoded != "" {
		fmt.Fprintf(&b, " sub_topics=%s", encoded)
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
	if rm.SourceInventoryProfile != nil && rm.SourceInventoryProfile.Active() {
		roles := make([]string, 0, len(rm.SourceInventoryProfile.TargetRoles))
		for _, role := range rm.SourceInventoryProfile.TargetRoles {
			roles = append(roles, string(role))
		}
		fmt.Fprintf(&b, " source_inventory=%s", strings.Join(roles, ","))
		if rm.SourceInventoryProfile.TypeUnderlying != "" && rm.SourceInventoryProfile.TypeUnderlying != types.SourceInventoryTypeUnderlyingUnknown {
			fmt.Fprintf(&b, " inventory_underlying=%s", rm.SourceInventoryProfile.TypeUnderlying)
		}
		if rm.SourceInventoryProfile.RequiresConstSet {
			b.WriteString(" inventory_const_set=true")
		}
	}
	if rm.ChangeImpactProfile != nil && rm.ChangeImpactProfile.IsChangeImpact {
		fmt.Fprintf(&b, " change_impact=%s", rm.ChangeImpactProfile.Target)
	}
	if rm.FieldValueProfile != nil && rm.FieldValueProfile.Active() {
		fmt.Fprintf(&b, " field_value=%s=%s", rm.FieldValueProfile.Target, rm.FieldValueProfile.Literal)
	}
	if rm.RuntimeArtifactValueProfile != nil && rm.RuntimeArtifactValueProfile.Active() {
		fmt.Fprintf(&b, " artifact_value=%s=%s", rm.RuntimeArtifactValueProfile.Target, rm.RuntimeArtifactValueProfile.Value)
	}
	if len(rm.RuntimeTargets) > 0 {
		fmt.Fprintf(&b, " runtime_targets=%d", len(rm.RuntimeTargets))
	}
	if rm.RuntimeTargetProfile != nil {
		fmt.Fprintf(&b, " runtime_target_profile=%s", rm.RuntimeTargetProfile.Declaration)
	}
	if rm.RuntimeQuestionProfile != nil {
		fmt.Fprintf(&b, " runtime_question_profile=%s", rm.RuntimeQuestionProfile.Scope)
	}
	if rm.HistorySelectionProfile != nil {
		fmt.Fprintf(&b, " history_selection=%s/%s", rm.HistorySelectionProfile.Mode, rm.HistorySelectionProfile.ItemKind)
		if rm.HistorySelectionProfile.Count > 0 {
			fmt.Fprintf(&b, ":%d", rm.HistorySelectionProfile.Count)
		}
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
	if rm.RequestedAnswerDimensions != nil && rm.RequestedAnswerDimensions.Active() {
		if encoded := encodeAnswerDimensionSummaryLabels(rm.RequestedAnswerDimensions.Dimensions); encoded != "" {
			fmt.Fprintf(&b, " answer_dimensions=%s", encoded)
		}
	}
	if len(rm.CurrentSourceObligationSignals) > 0 {
		fmt.Fprintf(&b, " current_source_obligation_signals=%d", len(rm.CurrentSourceObligationSignals))
	}
	if rm.CurrentSourceExplanationProfile != nil && rm.CurrentSourceExplanationProfile.Active() {
		fmt.Fprintf(&b, " current_source_explanation=%d", len(rm.CurrentSourceExplanationProfile.Modes))
	}
	if rm.ExternalObservationPolicy != nil {
		fmt.Fprintf(&b, " external_observation_policy=%s", rm.ExternalObservationPolicy.CurrentSourceMode)
		if rm.ExternalObservationPolicy.ArtifactCitationMode != "" &&
			rm.ExternalObservationPolicy.ArtifactCitationMode != types.ExternalObservationArtifactCitationDefault {
			fmt.Fprintf(&b, "/artifact_citation=%s", rm.ExternalObservationPolicy.ArtifactCitationMode)
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

func encodeSubTopicSummaryLabels(topics []types.SubTopic) string {
	labels := make([]string, 0, len(topics))
	for _, topic := range topics {
		label := strings.TrimSpace(topic.Summary)
		if label == "" && len(topic.Entities) > 0 {
			label = strings.TrimSpace(topic.Entities[0])
		}
		if label != "" {
			labels = append(labels, label)
		}
	}
	return encodeStringListSummary(labels)
}

func encodeAnswerDimensionSummaryLabels(dimensions []types.RequestedAnswerDimension) string {
	if len(dimensions) == 0 {
		return ""
	}
	items := append([]types.RequestedAnswerDimension(nil), dimensions...)
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Index == items[j].Index {
			return i < j
		}
		if items[i].Index <= 0 {
			return false
		}
		if items[j].Index <= 0 {
			return true
		}
		return items[i].Index < items[j].Index
	})
	labels := make([]string, 0, len(items))
	for _, dim := range items {
		if label := strings.TrimSpace(dim.Label); label != "" {
			labels = append(labels, label)
		}
	}
	return encodeStringListSummary(labels)
}

func encodeStringListSummary(values []string) string {
	if len(values) == 0 {
		return ""
	}
	clean := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		clean = append(clean, value)
	}
	if len(clean) == 0 {
		return ""
	}
	raw, err := json.Marshal(clean)
	if err != nil {
		return ""
	}
	return string(raw)
}

func encodeRequiredFileHintSummaryPaths(hints []types.RequiredFileHint) string {
	if len(hints) == 0 {
		return ""
	}
	paths := make([]string, 0, len(hints))
	seen := make(map[string]bool, len(hints))
	for _, hint := range hints {
		path := strings.TrimSpace(hint.Path)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return ""
	}
	raw, err := json.Marshal(paths)
	if err != nil {
		return ""
	}
	return string(raw)
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
	// Rune-safe cut (G18): a raw byte slice here shredded CJK requests
	// into U+FFFD mojibake on the live task row.
	if len(s) > subTopicSummaryMaxChars {
		s = strings.TrimSpace(types.CutPrefixRuneSafe(s, subTopicSummaryMaxChars)) + "…"
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

// validateAndBuildRequiredFileHints converts the LLM-emitted required_files
// array into the typed []types.RequiredFileHint slice the IR consumes.
// Validation is per-item, fail-soft: invalid entries are dropped with a warning
// appended to the validation result; the rest pass through. This keeps the
// emit_analysis call from failing hard on a single malformed entry while still
// surfacing the issue to the operator.
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
// Cross-language: paths are POSIX-canonicalised (Windows backslash converted to
// forward slash) and, when a BusContext is available, resolved through the same
// active-set/file-existence gate used by read_file seeds. That lets analyzer
// suggestions survive harmless prefix mistakes such as omitting the active
// sub-repo label, while unresolvable suggestions stay advisory and are dropped
// before they can send the explorer to a nonexistent file.
func validateAndBuildRequiredFileHints(in []emitRequiredFileParam, val *analysisValidationResult) []types.RequiredFileHint {
	return validateAndBuildRequiredFileHintsWithContext(nil, in, val)
}

func validateAndBuildRequiredFileHintsWithContext(ctx *types.BusContext, in []emitRequiredFileParam, val *analysisValidationResult) []types.RequiredFileHint {
	if len(in) == 0 {
		return nil
	}
	out := make([]types.RequiredFileHint, 0, len(in))
	dropped := 0
	clamped := 0
	normalized := 0
	unresolved := 0
	for _, e := range in {
		if len(out) >= requiredFileHintsMax {
			dropped++
			continue
		}
		canon, changed, ok := resolveRequiredFileHintPath(ctx, e.Path)
		if !ok {
			dropped++
			unresolved++
			continue
		}
		if canon == "" {
			dropped++
			continue
		}
		if changed {
			normalized++
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
			rationale = types.CutPrefixRuneSafe(rationale, requiredFileHintRationaleMaxChars-1) + "…"
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
				fmt.Sprintf("required_files: %d entries dropped (empty path / unresolved path / NaN confidence / over cap of %d)", dropped, requiredFileHintsMax))
		}
		if clamped > 0 {
			val.Warnings = append(val.Warnings,
				fmt.Sprintf("required_files: %d entries had confidence clamped into [0,1]", clamped))
		}
		if normalized > 0 {
			val.Warnings = append(val.Warnings,
				fmt.Sprintf("required_files: %d path(s) normalized to active repo-relative form", normalized))
		}
		if unresolved > 0 {
			val.Warnings = append(val.Warnings,
				fmt.Sprintf("required_files: %d path(s) dropped because they did not resolve to an existing active file", unresolved))
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func softenModelAuthoredRequiredFilesForSourceInventory(raw string, profile *types.SourceInventoryProfile, attempted *emitSourceInventoryProfileParam, in []types.RequiredFileHint, val *analysisValidationResult) []types.RequiredFileHint {
	// A malformed optional role list must not turn a model-guessed navigation
	// file into the hard universe of an otherwise explicit inventory request.
	// IsSourceInventory is a schema-validated boolean, independent from the
	// lossy role enum. Exact paths copied by the user remain hard below.
	inventoryRequested := (profile != nil && profile.Active()) || sourceInventoryRequested(attempted)
	if !inventoryRequested || len(in) == 0 {
		return in
	}
	out := make([]types.RequiredFileHint, 0, len(in))
	dropped := 0
	for _, hint := range in {
		if sourceInventoryRequiredFilePathExplicitInRequest(raw, hint.Path) {
			out = append(out, hint)
			continue
		}
		dropped++
	}
	if dropped > 0 && val != nil {
		val.Warnings = append(val.Warnings,
			fmt.Sprintf("required_files: dropped %d model-authored source-inventory path hint(s) that were not exact paths in the current request; repo_map/source_inventory remains the coverage authority", dropped))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sourceInventoryRequiredFilePathExplicitInRequest(raw, rel string) bool {
	raw = strings.TrimSpace(raw)
	rel = types.CanonicalRequiredFileHintPath(rel, "")
	if raw == "" || rel == "" {
		return false
	}
	lowerRaw := strings.ToLower(strings.ReplaceAll(raw, `\`, `/`))
	lowerRel := strings.ToLower(strings.ReplaceAll(rel, `\`, `/`))
	return strings.Contains(lowerRaw, lowerRel) ||
		strings.Contains(lowerRaw, "./"+lowerRel)
}

func projectAnalyzerPrescanRequiredFileHints(ctx *types.BusContext, rm *types.RequestModel, val *analysisValidationResult) int {
	if ctx == nil || ctx.Mutable == nil || rm == nil || !types.SourceInventoryRequiredFileCoverageShape(*rm) {
		return 0
	}
	maxProjected := types.SourceInventoryRequiredFileHintCoverageMax
	if maxProjected <= 0 {
		return 0
	}
	candidates := analyzerPrescanRequiredFileCandidates(ctx, *rm, maxProjected*2)
	if !sourceInventoryPrescanRequiredFileProjectionAllowed(ctx, *rm, candidates) {
		return 0
	}
	seen := make(map[string]bool, len(rm.AnalyzerHints.RequiredFileHints))
	highConfidence := 0
	for _, hint := range rm.AnalyzerHints.RequiredFileHints {
		key := canonicalRequiredFilePathForRepo(hint.Path, ctx.RepoRoot)
		if key == "" {
			continue
		}
		seen[key] = true
		if hint.Confidence >= 0.8 {
			highConfidence++
		}
	}
	if highConfidence >= maxProjected || len(rm.AnalyzerHints.RequiredFileHints) >= requiredFileHintsMax {
		return 0
	}
	added := 0
	for _, candidate := range candidates {
		if highConfidence >= maxProjected || len(rm.AnalyzerHints.RequiredFileHints) >= requiredFileHintsMax {
			break
		}
		canon, ok := analyzerPrescanRequiredFileCandidate(ctx, *rm, candidate)
		if !ok || seen[canon] {
			continue
		}
		seen[canon] = true
		highConfidence++
		added++
		rm.AnalyzerHints.RequiredFileHints = append(rm.AnalyzerHints.RequiredFileHints, types.RequiredFileHint{
			Path:       canon,
			Confidence: 0.82,
			Rationale:  "deterministic analyzer prescan candidate for typed source-inventory coverage",
		})
	}
	if added > 0 && val != nil {
		val.Warnings = append(val.Warnings,
			fmt.Sprintf("required_files: projected %d deterministic prescan candidate(s) for source-inventory coverage", added))
	}
	return added
}

func sourceInventoryPrescanRequiredFileProjectionAllowed(ctx *types.BusContext, rm types.RequestModel, candidates []string) bool {
	profile := rm.SourceInventoryProfile
	if profile == nil || !profile.Active() {
		return false
	}
	if !types.SourceInventoryRequiresRepoWideLens(rm) {
		return true
	}
	if sourceInventoryPrescanExplicitScopeProjectionAllowed(ctx, rm, candidates) {
		return true
	}
	if sourceInventoryRequiredHintsFormBoundedScope(ctx, rm, candidates) {
		return true
	}
	if sourceInventoryPrescanPeerSupplementAllowed(ctx, rm, candidates) {
		return true
	}
	files := analyzerPrescanBoundedRequiredFileCandidates(ctx, rm, candidates)
	return len(files) >= types.BoundedSourceEnumerationMinFiles &&
		types.BoundedSourceEnumerationCommonScope(files) != ""
}

func sourceInventoryPrescanExplicitScopeProjectionAllowed(ctx *types.BusContext, rm types.RequestModel, candidates []string) bool {
	if rm.SourceScopeProfile == nil ||
		rm.SourceScopeProfile.RequestedScope == types.SourceScopeUnknown ||
		len(rm.SourceScopeProfile.SourceQuotes) == 0 {
		return false
	}
	files := analyzerPrescanBoundedRequiredFileCandidates(ctx, rm, candidates)
	return len(files) >= 2 && types.BoundedSourceEnumerationCommonScope(files) != ""
}

func analyzerPrescanBoundedRequiredFileCandidates(ctx *types.BusContext, rm types.RequestModel, candidates []string) []string {
	seen := make(map[string]bool, len(candidates))
	files := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		canon, ok := analyzerPrescanRequiredFileCandidate(ctx, rm, candidate)
		if !ok || seen[canon] {
			continue
		}
		seen[canon] = true
		files = append(files, canon)
	}
	return files
}

func analyzerPrescanRequiredFileCandidates(ctx *types.BusContext, rm types.RequestModel, limit int) []string {
	if ctx == nil || ctx.Mutable == nil || limit <= 0 {
		return nil
	}
	seen := make(map[string]bool)
	var listCandidates []string
	var grepCandidates []string
	add := func(dst *[]string, raw string) {
		raw = strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`))
		if raw == "" || strings.HasPrefix(raw, "[") || seen[raw] {
			return
		}
		seen[raw] = true
		*dst = append(*dst, raw)
	}
	for _, result := range ctx.Mutable.DispatchToolResults() {
		if !result.Success {
			continue
		}
		switch types.CanonicalToolName(result.ToolName) {
		case "grep":
			if result.PathDiscovery == nil ||
				result.PathDiscovery.Kind != types.ToolPathDiscoveryKindGrep ||
				!result.PathDiscovery.FilesOnly {
				continue
			}
			for _, path := range result.PathDiscovery.CandidateFiles {
				add(&grepCandidates, path)
			}
		case "list_files":
			if !completionToolResultIsRecursivePathDiscoveryList(result) &&
				!(rm.SourceInventoryProfile != nil && rm.SourceInventoryProfile.Active()) {
				continue
			}
			if result.PathDiscovery == nil {
				continue
			}
			for _, path := range result.PathDiscovery.CandidateFiles {
				add(&listCandidates, path)
			}
		}
	}
	out := make([]string, 0, limit)
	appendLimited := func(in []string) {
		for _, candidate := range in {
			if len(out) >= limit {
				return
			}
			out = append(out, candidate)
		}
	}
	appendLimited(listCandidates)
	appendLimited(grepCandidates)
	return out
}

func analyzerPrescanRequiredFileCandidate(ctx *types.BusContext, rm types.RequestModel, raw string) (string, bool) {
	canon, _, ok := resolveRequiredFileHintPath(ctx, raw)
	if !ok || canon == "" || !types.HasCodeOrConfigPathSuffix(canon) {
		return "", false
	}
	if !sourceInventoryPrescanCandidateCanCarryPrincipalCoverage(rm, canon) {
		return "", false
	}
	profile := emitAnalysisEffectiveInventoryScopeProfileForPrescanProjection(rm)
	if !emitAnalysisPrincipalSourcePathAllowed(ctx, canon, profile) {
		return "", false
	}
	return canon, true
}

func sourceInventoryPrescanCandidateCanCarryPrincipalCoverage(rm types.RequestModel, rel string) bool {
	profile := rm.SourceInventoryProfile
	if profile == nil || !profile.Active() {
		return true
	}
	role := types.ClassifySourcePathRole(rel)
	switch role {
	case types.SourcePathRoleDocumentation, types.SourcePathRolePromptSupport:
		return sourceInventoryPrincipalRolesAllowDocumentationCoverage(profile) ||
			(rm.SourceScopeProfile != nil && rm.SourceScopeProfile.RequestedScope == types.SourceScopeDocumentation)
	default:
		return true
	}
}

func sourceInventoryPrincipalRolesAllowDocumentationCoverage(profile *types.SourceInventoryProfile) bool {
	if profile == nil || !profile.Active() {
		return false
	}
	for _, role := range profile.PrincipalTargetRoles() {
		switch role {
		case types.AnswerCandidateRoleFile,
			types.AnswerCandidateRoleDocumentation,
			types.AnswerCandidateRoleConfigFile:
			return true
		}
	}
	return false
}

func resolveRequiredFileHintPath(ctx *types.BusContext, raw string) (string, bool, bool) {
	repoRoot := ""
	if ctx != nil {
		repoRoot = ctx.RepoRoot
	}
	canon := canonicalRequiredFilePathForRepo(raw, repoRoot)
	if canon == "" {
		return "", false, true
	}
	changed := canon != strings.TrimSpace(raw)
	if types.RuntimeArtifactPathKindInText(canon) != "" || types.RuntimeArtifactPathKindInText(raw) != "" {
		return canon, changed, true
	}
	if ctx == nil {
		return canon, changed, true
	}
	if stripped, ok := stripActiveRepoLabelPrefix(ctx, canon); ok {
		canon = canonicalRequiredFilePathForRepo(stripped, repoRoot)
		changed = true
	}
	qualified, ok := types.QualifyForcedReadSeedPath(ctx, canon)
	if !ok {
		return "", changed, false
	}
	qualified = canonicalRequiredFilePathForRepo(qualified, repoRoot)
	if qualified == "" {
		return "", changed, true
	}
	if qualified != canon {
		changed = true
	}
	rel, ok := requiredFileHintExistingRepoFile(ctx, qualified)
	if !ok {
		return "", changed, false
	}
	if rel != qualified {
		changed = true
	}
	return rel, changed, true
}

func projectRuntimeArtifactPathHintsFromRawRequest(rm *types.RequestModel, raw string) {
	if rm == nil || rm.ExternalObservationPolicy == nil {
		return
	}
	if !rm.ExternalObservationPolicy.ArtifactCitationsExternalOnly() &&
		!rm.ExternalObservationPolicy.ExcludesCurrentSource() {
		return
	}
	seen := make(map[string]bool, len(rm.AnalyzerHints.RequiredFileHints))
	for _, hint := range rm.AnalyzerHints.RequiredFileHints {
		key := strings.ToLower(strings.TrimSpace(hint.Path))
		if key != "" {
			seen[key] = true
		}
	}
	for _, token := range types.RuntimeArtifactPathTokensInText(raw) {
		key := strings.ToLower(strings.TrimSpace(token))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		rm.AnalyzerHints.RequiredFileHints = append(rm.AnalyzerHints.RequiredFileHints, types.RequiredFileHint{
			Path:       token,
			Confidence: 0.8,
			Rationale:  "runtime artifact path preserved from the current request for artifact-lane routing",
		})
	}
}

func requiredFileHintExistingRepoFile(ctx *types.BusContext, path string) (string, bool) {
	if ctx == nil || strings.TrimSpace(ctx.RepoRoot) == "" {
		return path, true
	}
	rel, ok := repoRelativePathWithinRoot(ctx.RepoRoot, path)
	if !ok || rel == "" || rel == "." {
		return "", false
	}
	info, err := os.Stat(filepath.Join(ctx.RepoRoot, filepath.FromSlash(rel)))
	if err != nil || info.IsDir() {
		return "", false
	}
	return rel, true
}

// canonicalRequiredFilePath normalises an LLM-emitted file path to
// the repo-relative POSIX form the explorer's path lookup uses. Same
// rules as canonicalExplorerPath: backslash → forward slash, trim
// "./" prefix, trim whitespace.
func canonicalRequiredFilePath(p string) string {
	return canonicalRequiredFilePathForRepo(p, "")
}

func canonicalRequiredFilePathForRepo(p, repoRoot string) string {
	p = types.CanonicalRequiredFileHintPath(p, repoRoot)
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

// repairEmitAnalysisNonBoundedFactFamilies removes one provably redundant
// cross-field carrier before strict decode. fact_families has consumers only
// under bounded_fact_set; once the same typed object explicitly declares any
// other valid scope, retaining the field cannot change the RequestModel and
// only causes an avoidable retry. Invalid/missing scopes and bounded profiles
// remain untouched so their existing fail-loud validation still owns them.
func repairEmitAnalysisNonBoundedFactFamilies(raw json.RawMessage) (json.RawMessage, []string, bool) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil || len(obj) == 0 {
		return raw, nil, false
	}
	profileRaw, ok := obj["runtime_question_profile"]
	if !ok {
		return raw, nil, false
	}
	var profile map[string]json.RawMessage
	if err := json.Unmarshal(profileRaw, &profile); err != nil || len(profile) == 0 {
		return raw, nil, false
	}
	if _, ok := profile["fact_families"]; !ok {
		return raw, nil, false
	}
	var scopeRaw string
	if err := json.Unmarshal(profile["scope"], &scopeRaw); err != nil {
		return raw, nil, false
	}
	scope := types.RuntimeQuestionScope(strings.TrimSpace(scopeRaw))
	if !scope.IsValid() || scope == types.RuntimeQuestionScopeBoundedFactSet {
		return raw, nil, false
	}
	delete(profile, "fact_families")
	patchedProfile, err := json.Marshal(profile)
	if err != nil {
		return raw, nil, false
	}
	obj["runtime_question_profile"] = patchedProfile
	patched, err := json.Marshal(obj)
	if err != nil || !json.Valid(patched) {
		return raw, nil, false
	}
	warning := fmt.Sprintf(
		"runtime_question_profile.fact_families ignored because typed scope=%s has no fact-family consumer",
		scope,
	)
	return json.RawMessage(patched), []string{warning}, true
}

// repairEmitAnalysisIrrelevantFilePathObjects absorbs a common schema-adjacent
// drift where a model mirrors required_files' object shape inside
// irrelevant_files. The field's semantic contract is still path-only: only an
// explicit `path` string is extracted; confidence/rationale are ignored; objects
// without a path are dropped as malformed optional negative hints.
func repairEmitAnalysisIrrelevantFilePathObjects(raw json.RawMessage) (json.RawMessage, []string, bool) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil || len(obj) == 0 {
		return raw, nil, false
	}
	listRaw, ok := obj["irrelevant_files"]
	if !ok {
		return raw, nil, false
	}
	trimmedList := strings.TrimSpace(string(listRaw))
	if trimmedList == "" || trimmedList == "null" {
		return raw, nil, false
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(listRaw, &entries); err != nil {
		return raw, nil, false
	}
	if len(entries) == 0 {
		return raw, nil, false
	}
	out := make([]json.RawMessage, 0, len(entries))
	extracted := 0
	dropped := 0
	changed := false
	for _, entry := range entries {
		trimmed := strings.TrimSpace(string(entry))
		if trimmed == "" || trimmed == "null" {
			dropped++
			changed = true
			continue
		}
		switch trimmed[0] {
		case '"':
			out = append(out, entry)
		case '{':
			var m map[string]json.RawMessage
			if err := json.Unmarshal(entry, &m); err != nil {
				dropped++
				changed = true
				continue
			}
			pathRaw, ok := m["path"]
			if !ok {
				dropped++
				changed = true
				continue
			}
			var path string
			if err := json.Unmarshal(pathRaw, &path); err != nil || strings.TrimSpace(path) == "" {
				dropped++
				changed = true
				continue
			}
			encoded, err := json.Marshal(strings.TrimSpace(path))
			if err != nil {
				dropped++
				changed = true
				continue
			}
			out = append(out, encoded)
			extracted++
			changed = true
		default:
			dropped++
			changed = true
		}
	}
	if !changed {
		return raw, nil, false
	}
	encodedList, err := json.Marshal(out)
	if err != nil {
		return raw, nil, false
	}
	obj["irrelevant_files"] = encodedList
	patched, err := json.Marshal(obj)
	if err != nil || !json.Valid(patched) {
		return raw, nil, false
	}
	warnings := make([]string, 0, 2)
	if extracted > 0 {
		warnings = append(warnings, fmt.Sprintf("irrelevant_files: repaired %d object entries by extracting path strings", extracted))
	}
	if dropped > 0 {
		warnings = append(warnings, fmt.Sprintf("irrelevant_files: dropped %d malformed optional entries before decode", dropped))
	}
	return json.RawMessage(patched), warnings, true
}

// repairEmitAnalysisRequiredFileStringEntries accepts the common shorthand
// required_files:["path"] and upgrades it to the documented object shape. The
// semantic validator still decides whether the path is usable; this repair only
// removes avoidable JSON-shape retries from the analyzer loop.
func repairEmitAnalysisRequiredFileStringEntries(raw json.RawMessage) (json.RawMessage, []string, bool) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil || len(obj) == 0 {
		return raw, nil, false
	}
	listRaw, ok := obj["required_files"]
	if !ok {
		return raw, nil, false
	}
	trimmedList := strings.TrimSpace(string(listRaw))
	if trimmedList == "" || trimmedList == "null" {
		return raw, nil, false
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(listRaw, &entries); err != nil || len(entries) == 0 {
		return raw, nil, false
	}
	out := make([]json.RawMessage, 0, len(entries))
	repaired := 0
	dropped := 0
	changed := false
	for _, entry := range entries {
		trimmed := strings.TrimSpace(string(entry))
		if trimmed == "" || trimmed == "null" {
			dropped++
			changed = true
			continue
		}
		switch trimmed[0] {
		case '{':
			out = append(out, entry)
		case '"':
			var path string
			if err := json.Unmarshal(entry, &path); err != nil || strings.TrimSpace(path) == "" {
				dropped++
				changed = true
				continue
			}
			objEntry := emitRequiredFileParam{
				Path:       strings.TrimSpace(path),
				Confidence: 0.8,
				Rationale:  "compat repaired from string path",
			}
			encoded, err := json.Marshal(objEntry)
			if err != nil {
				dropped++
				changed = true
				continue
			}
			out = append(out, encoded)
			repaired++
			changed = true
		default:
			dropped++
			changed = true
		}
	}
	if !changed {
		return raw, nil, false
	}
	encodedList, err := json.Marshal(out)
	if err != nil {
		return raw, nil, false
	}
	obj["required_files"] = encodedList
	patched, err := json.Marshal(obj)
	if err != nil || !json.Valid(patched) {
		return raw, nil, false
	}
	warnings := make([]string, 0, 2)
	if repaired > 0 {
		warnings = append(warnings, fmt.Sprintf("required_files: repaired %d string entries to object shape", repaired))
	}
	if dropped > 0 {
		warnings = append(warnings, fmt.Sprintf("required_files: dropped %d malformed optional entries before decode", dropped))
	}
	return json.RawMessage(patched), warnings, true
}

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

func reconcilePrincipalScopeIrrelevantFiles(
	ctx *types.BusContext,
	sourceScopeProfile *types.SourceScopeProfile,
	sourceInventoryProfile *types.SourceInventoryProfile,
	answerExclusionPolicy *types.AnswerExclusionPolicy,
	intent types.Intent,
	predicates types.SemanticPredicates,
	required []types.RequiredFileHint,
	irrelevant []string,
	val *analysisValidationResult,
) ([]types.RequiredFileHint, []string, *types.SourceScopeProfile) {
	if len(irrelevant) == 0 || !emitAnalysisPrincipalSourceInventoryShape(intent, predicates, sourceInventoryProfile) {
		return required, irrelevant, sourceScopeProfile
	}
	requiredSeen := make(map[string]bool, len(required)+len(irrelevant))
	for _, hint := range required {
		if key := strings.TrimSpace(hint.Path); key != "" {
			requiredSeen[key] = true
		}
	}
	filtered := make([]string, 0, len(irrelevant))
	demoted := 0
	promoted := 0
	promotedAuxiliary := false
	effectiveScopeProfile := emitAnalysisEffectiveInventoryScopeProfile(sourceScopeProfile)
	for _, rel := range irrelevant {
		rel = canonicalRequiredFilePath(rel)
		if rel == "" {
			continue
		}
		if !emitAnalysisPrincipalSourcePathAllowed(ctx, rel, effectiveScopeProfile) {
			filtered = append(filtered, rel)
			continue
		}
		role := types.ClassifySourcePathRole(rel)
		if !types.SourcePathRoleIsAuxiliary(role) {
			filtered = append(filtered, rel)
			continue
		}
		if answerExclusionPolicy.ExcludesSourcePathRole(role) {
			filtered = append(filtered, rel)
			continue
		}
		demoted++
		promotedAuxiliary = true
		if len(required) >= requiredFileHintsMax || requiredSeen[rel] {
			continue
		}
		requiredSeen[rel] = true
		required = append(required, types.RequiredFileHint{
			Path:       rel,
			Confidence: 0.80,
			Rationale:  "principal source-scope path preserved from contradictory analyzer irrelevant_files",
		})
		promoted++
	}
	if val != nil && demoted > 0 {
		msg := fmt.Sprintf("irrelevant_files: dropped %d principal source-scope path(s) for source-inventory/enumeration shape", demoted)
		if promoted > 0 {
			msg += fmt.Sprintf("; promoted %d to required_files", promoted)
		}
		val.Warnings = append(val.Warnings, msg)
	}
	if promotedAuxiliary && emitAnalysisShouldSynthesizeAllScopeForAuxiliaryInventory(sourceScopeProfile) {
		sourceScopeProfile = &types.SourceScopeProfile{
			RequestedScope:              types.SourceScopeAll,
			IncludeAuxiliaryAsPrincipal: true,
			Confidence:                  0.70,
			Rationale:                   "source-inventory paths preserved from analyzer negative channel require auxiliary source classes as principal candidates",
		}
		if val != nil {
			val.Warnings = append(val.Warnings, "source_scope_profile: synthesized all-scope because source-inventory irrelevant_files contained principal auxiliary source path(s)")
		}
	}
	if len(filtered) == 0 {
		return required, nil, sourceScopeProfile
	}
	return required, filtered, sourceScopeProfile
}

func emitAnalysisEffectiveInventoryScopeProfile(profile *types.SourceScopeProfile) *types.SourceScopeProfile {
	if profile == nil {
		return nil
	}
	if profile.RequestedScope == types.SourceScopeProduction && len(profile.SourceQuotes) == 0 {
		return nil
	}
	return profile
}

func emitAnalysisEffectiveInventoryScopeProfileForPrescanProjection(rm types.RequestModel) *types.SourceScopeProfile {
	if rm.SourceScopeProfile != nil &&
		rm.SourceScopeProfile.RequestedScope == types.SourceScopeProduction &&
		rm.SourceInventoryProfile != nil &&
		rm.SourceInventoryProfile.Active() &&
		SourceInventoryHasExplicitAuxiliaryExclusion(rm) {
		return rm.SourceScopeProfile
	}
	profile := emitAnalysisEffectiveInventoryScopeProfile(rm.SourceScopeProfile)
	if profile == nil {
		return nil
	}
	if profile.RequestedScope == types.SourceScopeProduction &&
		rm.SourceInventoryProfile != nil &&
		rm.SourceInventoryProfile.Active() &&
		!SourceInventoryHasExplicitAuxiliaryExclusion(rm) {
		return nil
	}
	return profile
}

func emitAnalysisShouldSynthesizeAllScopeForAuxiliaryInventory(profile *types.SourceScopeProfile) bool {
	if profile == nil || profile.RequestedScope == "" || profile.RequestedScope == types.SourceScopeUnknown {
		return true
	}
	return profile.RequestedScope == types.SourceScopeProduction && len(profile.SourceQuotes) == 0
}

func emitAnalysisPrincipalSourceInventoryShape(intent types.Intent, predicates types.SemanticPredicates, profile *types.SourceInventoryProfile) bool {
	if profile != nil && profile.Active() {
		return true
	}
	return intent == types.IntentEnumerate && predicates.IsCategoryEnumeration
}

func emitAnalysisPrincipalSourcePathAllowed(ctx *types.BusContext, rel string, profile *types.SourceScopeProfile) bool {
	rel = canonicalRequiredFilePath(rel)
	if rel == "" || !types.HasCodeOrConfigPathSuffix(rel) {
		return false
	}
	if ctx != nil && strings.TrimSpace(ctx.RepoRoot) != "" {
		if _, ok := requiredFileHintExistingRepoFile(ctx, rel); !ok {
			return false
		}
	}
	scope := types.SourceScopeProduction
	if profile != nil && profile.RequestedScope != "" {
		scope = profile.RequestedScope
	} else {
		scope = types.SourceScopeAll
	}
	return types.SourceScopeAllowsPathRole(scope, types.ClassifySourcePathRole(rel))
}

// normalizeEmitArtifactLineRefs converts the wire shape into the typed
// IR refs through the shared normalizer (source enum bound, positive
// lines, end >= start; invalid entries drop).
func normalizeEmitArtifactLineRefs(in []emitArtifactLineRefParam) []types.ArtifactLineRef {
	if len(in) == 0 {
		return nil
	}
	refs := make([]types.ArtifactLineRef, 0, len(in))
	for _, r := range in {
		refs = append(refs, types.ArtifactLineRef{Source: r.Source, StartLine: r.StartLine, EndLine: r.EndLine})
	}
	return types.NormalizeArtifactLineRefs(refs)
}
