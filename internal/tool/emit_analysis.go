package tool

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/analysis/prescan"
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
	Intent            string                  `json:"intent"`
	Scenario          string                  `json:"scenario"`
	Complexity        string                  `json:"complexity"`
	Keywords          []string                `json:"keywords"`
	Entities          []string                `json:"entities"`
	QuestionKind      string                  `json:"question_kind"`
	AnswerShape       string                  `json:"answer_shape"`
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
	IntentConfidence     float64                       `json:"intent_confidence"`
	ComplexityConfidence float64                       `json:"complexity_confidence"`
	KindConfidence       float64                       `json:"kind_confidence"`
	ShapeConfidence      float64                       `json:"shape_confidence"`
	Predicates           *emitPredicatesParam          `json:"predicates"`
	PredicateAxis        string                        `json:"predicate_axis,omitempty"`
	DiagramHint          *emitDiagramHintParam         `json:"diagram_hint,omitempty"`
	EnumerationBoundary  *emitEnumerationBoundaryParam `json:"enumeration_boundary,omitempty"`
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
			"answer_shape":  stringProp{Type: "string", Enum: skill.AnalysisAnswerShapeValues()},
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
				"description": "Optional exact user-asked targets, copied verbatim from the current request text. Use when the request mentions neighboring context items but asks about one specific key/path/symbol/literal. Every item must be explicitly present in the current request text before downstream stages use it.",
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
				"description": "Optional. Classifies what kind of source-code literal the answer should be (skill_name, agent_name, config_key, ...). The chain ranker uses this to demote chains whose terminal token is the wrong kind. Leave unset when the answer kind is ambiguous; the system's deterministic fallback infers from question_kind.",
				"properties": map[string]any{
					"kind":        stringProp{Type: "string", Enum: skill.AnalysisAnswerSubjectValues()},
					"entity_axes": arrayProp{Type: "array", Items: map[string]string{"type": "string"}},
					"confidence":  map[string]string{"type": "number"},
				},
			},
			"intent_confidence":     map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0, "description": "Your certainty about the intent classification, in [0, 1]. 0.9+ = unambiguous; 0.5-0.7 = plausible alternative exists; < 0.5 = guessing."},
			"complexity_confidence": map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0, "description": "Your certainty about the complexity classification, in [0, 1]."},
			"kind_confidence":       map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0, "description": "Your certainty about the question_kind classification, in [0, 1]."},
			"shape_confidence":      map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0, "description": "Your certainty about the answer_shape classification, in [0, 1]."},
			"predicates": map[string]any{
				"type":        "object",
				"description": "Cross-language semantic self-assessment of the question. Every field is required; emit true OR false explicitly. A missing field is fail-loud rejection, not a silent default.",
				"properties": map[string]any{
					"is_scalar_answer":        map[string]any{"type": "boolean", "description": "True if the answer is a single scalar (a number, a literal, a path) rather than a set or sequence."},
					"is_role_locate_lookup":   map[string]any{"type": "boolean", "description": "True when the request names a clue / output / context entity, but the answer is a DIFFERENT single literal that plays a role relative to that clue (entry function, defining file, config key, route, owner symbol, etc.). Implies is_scalar_answer=true and requires answer_subject.kind."},
					"is_count_question":       map[string]any{"type": "boolean", "description": "True when the answer is a single number that must be computed by aggregating values across multiple source units — e.g. counting items, summing lines of code, summing file sizes, totalling bytes across a directory tree. Implies is_scalar_answer. Set false when the answer is a number that already exists as a single source-code literal (a const declaration, a default value, an enum ordinal) — that case is is_scalar_answer=true without is_count_question."},
					"is_cross_component":      map[string]any{"type": "boolean", "description": "True if the question genuinely spans multiple distinct components / subsystems / independently-answerable code regions. Leave false for a single named target that merely needs nearby context, precedence layers, or override stages, and also leave false for one ordered source-to-sink call/flow trace even when that chain crosses files or packages."},
					"is_relational_lookup":    map[string]any{"type": "boolean", "description": "True if filtering set X by a relationship to Y ('functions that return Z', 'agents that use skill Y')."},
					"is_category_enumeration": map[string]any{"type": "boolean", "description": "True if asking 'what kinds / types / categories of X exist'."},
					"is_history_lookup":       map[string]any{"type": "boolean", "description": "True when the literal answer should come from repository history / authorship metadata (git log / blame / commit history), not from a repo file:line."},
				},
				"required": []string{"is_scalar_answer", "is_role_locate_lookup", "is_count_question", "is_cross_component", "is_relational_lookup", "is_category_enumeration", "is_history_lookup"},
			},
			"predicate_axis": map[string]any{
				"type":        "string",
				"enum":        skill.AnalysisPredicateAxisValues(),
				"description": "Action-direction axis of the question (call / register / define / return / configure / condition / implement). Empty when no clear verb cue. Used by the evidence ranker to bias items whose anchor matches the axis.",
			},
			"diagram_hint": map[string]any{
				"type":        "object",
				"description": "Optional. Suggests the diagram family that would best explain the answer. The deterministic analyzer compiler derives the final diagram contract from stronger structural signals, so omit when unsure.",
				"properties": map[string]any{
					"kind": stringProp{Type: "string", Enum: skill.AnalysisDiagramKindValues()},
				},
				"required": []string{"kind"},
			},
			"enumeration_boundary": map[string]any{
				"type":        "object",
				"description": "Optional. Use only when the user explicitly declares a bounded principal set such as 'the 7 checks', 'the first 3 handlers', or 'top 5 stages'. Copy the evidence-bearing phrase verbatim from the current request into source_quote and set declared_count to that same user-declared number.",
				"properties": map[string]any{
					"declared_count": map[string]any{"type": "integer", "minimum": 1},
					"source_quote":   map[string]string{"type": "string"},
				},
				"required": []string{"declared_count", "source_quote"},
			},
		},
		"required": []string{
			"intent", "scenario", "complexity", "keywords", "entities", "question_kind", "answer_shape",
			"intent_confidence", "complexity_confidence", "kind_confidence", "shape_confidence",
			"predicates",
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
			Summary:   "emit_analysis requires BusContext.Mutable; the caller did not provide one (sub-agents are not supported)",
			Timestamp: time.Now(),
		}, nil
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
	shape := normalizeAnswerShape(p.AnswerShape)

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
	if val.RejectReason != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: " + val.RejectReason,
			Timestamp: time.Now(),
		}, nil
	}
	entities = val.FilteredEntities
	if reason := rejectDegenerateClassification(intent, kind, shape, keywords, entities); reason != "" {
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
	if reason := validateConfidenceRange(p.IntentConfidence, p.ComplexityConfidence, p.KindConfidence, p.ShapeConfidence); reason != "" {
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
	enumerationBoundary, enumerationBoundaryErr := parseEnumerationBoundary(raw, p.EnumerationBoundary)
	if enumerationBoundaryErr != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: " + enumerationBoundaryErr,
			Timestamp: time.Now(),
		}, nil
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
	// Self-consistency: when the LLM's chosen intent / shape contradicts
	// its own predicates, or when a define-axis single-target question
	// failed to disambiguate whether it is a role-locate scalar lookup
	// versus an explanation of the named entity itself, reject here so
	// the retry hint forces the LLM to reconcile its own classification
	// instead of a Go reconcile rule papering over the inconsistency.
	if reason := validateSelfConsistency(intent, kind, shape, predicates, axis, entities, p.SubTopics, answerSubject); reason != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_analysis rejected: " + reason,
			Timestamp: time.Now(),
		}, nil
	}
	exactContextTerms, exactContextWarn := sanitizeExactContextTerms(exactTargets, mentionedEntities, p.ExactContextTerms)
	exactContextRoles, exactContextRoleWarn := sanitizeExactContextRoles(
		exactTargets,
		answerSubject.Kind,
		scenario,
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
			Kind:              kind,
			Shape:             shape,
		},
		AnswerSubject:        answerSubject,
		IntentConfidence:     p.IntentConfidence,
		ComplexityConfidence: p.ComplexityConfidence,
		KindConfidence:       p.KindConfidence,
		ShapeConfidence:      p.ShapeConfidence,
		Predicates:           predicates,
		PredicateAxis:        axis,
		DiagramHint:          diagramHint,
		EnumerationBoundary:  enumerationBoundary,
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
	shape string,
	keywords []string,
	entities []string,
) string {
	if intent != types.IntentUnknown {
		return ""
	}
	if !strings.EqualFold(strings.TrimSpace(kind), "unknown") {
		return ""
	}
	if !strings.EqualFold(strings.TrimSpace(shape), "none") {
		return ""
	}
	if len(keywords) > 0 || len(entities) > 0 {
		return ""
	}
	return "degenerate classification (intent=unknown, question_kind=unknown, answer_shape=none, keywords=0, entities=0). Re-read the User Request section only and emit at least one real keyword/entity or choose a concrete question_kind/answer_shape."
}

// validateSelfConsistency cross-checks the LLM's intent / kind /
// shape against its own predicates and rejects internally-contradictory
// classifications. The downstream pipeline trusts the LLM's predicates
// as the cross-language signal that replaces the deleted prose-cue
// tables; if the LLM emits "is_count_question=true" but also picks
// intent=enumerate + shape=list_of_symbols, two things are wrong at
// once and the retry hint should force the LLM to reconcile its own
// answer rather than have a Go rule silently override one of them
// (which is what session 6 reconcileIntent used to do via the
// prose-cue table that's about to be deleted).
//
// Each check fires only on a clear contradiction. We deliberately do
// NOT enforce the full Cartesian product (e.g. is_cross_component
// implies complexity=complex) because a few of those would trip on
// legitimate edge cases; callers should add a check here only when a
// real failure is observed.
func validateSelfConsistency(
	intent types.Intent,
	kind string,
	shape string,
	preds types.SemanticPredicates,
	axis types.PredicateAxis,
	entities []string,
	subTopics []types.SubTopic,
	answerSubject types.AnswerSubject,
) string {
	if preds.IsRoleLocateLookup {
		if !preds.IsScalarAnswer {
			return "is_role_locate_lookup=true requires is_scalar_answer=true — a role-locate question still resolves to one literal answer"
		}
		shapeLower := strings.ToLower(strings.TrimSpace(shape))
		if shapeLower != string(types.ShapeValue) {
			return "is_role_locate_lookup=true requires answer_shape=value — the answer is one located literal, not a prose/list carrier"
		}
		if !roleLocateSubjectKindAllowed(answerSubject.Kind) {
			return "is_role_locate_lookup=true requires answer_subject.kind to name the located literal kind (function_name / type_name / file_path / handler_route / config_key / interface_name / struct_field / enum_value)"
		}
	}
	// Count question must resolve to a scalar answer, not a list. If
	// the LLM marked is_count_question but picked an enumerate intent
	// or a list_of_symbols shape, it has answered the question wrong
	// in two places at once.
	if preds.IsCountQuestion {
		if intent == types.IntentEnumerate {
			return "is_count_question=true is inconsistent with intent=enumerate — a count question returns a single scalar; pick intent=return_value (and ensure answer_shape=value, not list_of_symbols)"
		}
		shapeLower := strings.ToLower(strings.TrimSpace(shape))
		if shapeLower == "list_of_symbols" {
			return "is_count_question=true is inconsistent with answer_shape=list_of_symbols — a count question returns a single scalar; set answer_shape=value"
		}
	}
	// is_count_question implies is_scalar_answer (the prompt says so).
	// LLM can still get this wrong; reject so it has to fix one or
	// the other.
	if preds.IsCountQuestion && !preds.IsScalarAnswer {
		return "is_count_question=true requires is_scalar_answer=true — a count question always yields a single scalar"
	}
	// Category enumeration ("what kinds of X exist") implies a list
	// shape, not a single scalar. If both predicates are set the
	// question is contradictory.
	if preds.IsCategoryEnumeration && preds.IsScalarAnswer {
		return "is_category_enumeration=true and is_scalar_answer=true are mutually exclusive — a 'what kinds of X' question yields a list, not a scalar"
	}
	if preds.IsHistoryLookup {
		if intent == types.IntentEnumerate {
			return "is_history_lookup=true is inconsistent with intent=enumerate — repository-history questions yield a single scalar / literal, not a list"
		}
		shapeLower := strings.ToLower(strings.TrimSpace(shape))
		if shapeLower == "list_of_symbols" {
			return "is_history_lookup=true is inconsistent with answer_shape=list_of_symbols — repository-history questions yield a single scalar / literal; set answer_shape=value"
		}
		if !preds.IsScalarAnswer {
			return "is_history_lookup=true requires is_scalar_answer=true — history / authorship lookups yield a single scalar / literal"
		}
	}
	if needsRoleLocateDisambiguation(axis, intent, shape, preds, entities, subTopics) &&
		answerSubject.Kind == types.SubjectUnknown {
		return "single-target define-axis lookup is under-specified: set answer_subject.kind explicitly so the system can tell whether this is a role-locate scalar lookup (function / type / file / route / config key) or an explanation of the named entity itself; also set predicates.is_role_locate_lookup to true or false explicitly"
	}
	return ""
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
	shape string,
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
	switch strings.ToLower(strings.TrimSpace(shape)) {
	case "", string(types.ShapeNone), string(types.ShapeBoolean), string(types.ShapeListOfSymbols):
		return false
	}
	return true
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
			"predicates object missing — emit `predicates` with is_scalar_answer / is_role_locate_lookup / is_count_question / is_cross_component / is_relational_lookup / is_category_enumeration / is_history_lookup each set to true or false"
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
	}, ""
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

func sanitizeExactContextRoles(exactTargets []string, subjectKind types.AnswerSubjectKind, scenario types.Scenario, in []string) ([]types.EvidenceDiagramRole, string) {
	if len(in) == 0 {
		return nil, ""
	}
	if len(exactTargets) == 0 {
		return nil, "dropped exact_context_roles because exact_targets were absent or ambiguous"
	}
	if scenario != types.ScenarioConfigTrace {
		return nil, "dropped exact_context_roles because this request is not a config-trace exact-target question"
	}
	if subjectKind != types.SubjectUnknown && subjectKind != types.SubjectConfigKey {
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
func validateConfidenceRange(intentConf, compConf, kindConf, shapeConf float64) string {
	for _, c := range [...]struct {
		name string
		val  float64
	}{
		{"intent_confidence", intentConf},
		{"complexity_confidence", compConf},
		{"kind_confidence", kindConf},
		{"shape_confidence", shapeConf},
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

func parseEnumerationBoundary(raw string, p *emitEnumerationBoundaryParam) (*types.RequestedEnumerationBoundary, string) {
	if p == nil {
		return nil, ""
	}
	if p.DeclaredCount <= 0 {
		return nil, "enumeration_boundary.declared_count must be >= 1"
	}
	if strings.TrimSpace(p.SourceQuote) == "" {
		return nil, "enumeration_boundary.source_quote must be copied verbatim from the current request"
	}
	boundary := types.NormalizeRequestedEnumerationBoundary(raw, &types.RequestedEnumerationBoundary{
		DeclaredCount: p.DeclaredCount,
		SourceQuote:   p.SourceQuote,
	})
	if boundary == nil {
		return nil, "enumeration_boundary.source_quote must appear in the current request text (whitespace-insensitive match allowed)"
	}
	return boundary, ""
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

	fmt.Fprintf(&b, "analysis emitted: intent=%s scenario=%s complexity=%s kw=%d ent=%d kind=%s shape=%s",
		rm.Intent, rm.Scenario, rm.Complexity,
		len(h.Keywords), len(h.Entities),
		h.Kind, h.Shape)
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
	check("answer_shape", raw.AnswerShape, rm.AnalyzerHints.Shape)
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

// normalizeAnswerShape maps LLM-emitted answer_shape strings to the
// canonical set consumed by the finalizer. Empty or unknown falls
// back to "none".
func normalizeAnswerShape(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "list_of_symbols", "symbol_list", "list-of-symbols":
		return "list_of_symbols"
	case "step_list", "steps", "step-list":
		return "step_list"
	case "value", "literal":
		return "value"
	case "boolean", "yes_no", "yes/no":
		return "boolean"
	case "config_value", "config-value":
		return "config_value"
	case "explanation", "prose":
		return "explanation"
	}
	return "none"
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
