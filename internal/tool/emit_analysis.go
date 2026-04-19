package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/skill"
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
	Intent        string                  `json:"intent"`
	Scenario      string                  `json:"scenario"`
	Complexity    string                  `json:"complexity"`
	Keywords      []string                `json:"keywords"`
	Entities      []string                `json:"entities"`
	QuestionKind  string                  `json:"question_kind"`
	AnswerShape   string                  `json:"answer_shape"`
	Language      string                  `json:"language,omitempty"`
	SubTopics     []types.SubTopic        `json:"sub_topics,omitempty"`
	AnswerSubject *emitAnswerSubjectParam `json:"answer_subject,omitempty"`

	// Schema v4 — confidence + alternatives + LLM semantic predicates +
	// LLM-emitted predicate axis. These replace the historical prose-cue
	// reconciliation tables (countVerbPrefixes / crossComponentCues /
	// enumerationCuePrefixes / relationalVerbCues / categoryEnumerationCues
	// / predicateVerbMap) with cross-language LLM judgement. All
	// predicate fields are required to be explicit (true OR false) — a
	// missing field is fail-loud, not a silent default.
	IntentConfidence     float64                 `json:"intent_confidence"`
	ComplexityConfidence float64                 `json:"complexity_confidence"`
	KindConfidence       float64                 `json:"kind_confidence"`
	ShapeConfidence      float64                 `json:"shape_confidence"`
	IntentAlternatives   []string                `json:"intent_alternatives,omitempty"`
	KindAlternatives     []string                `json:"kind_alternatives,omitempty"`
	ShapeAlternatives    []string                `json:"shape_alternatives,omitempty"`
	Predicates           *emitPredicatesParam    `json:"predicates"`
	PredicateAxis        string                  `json:"predicate_axis,omitempty"`
}

// emitPredicatesParam is the wire shape of the required `predicates`
// object. Pointer-typed so missing object can be detected via nil
// (validation rejects nil with a fail-loud error). Each bool field
// must be explicitly emitted by the LLM — JSON omission would parse
// to false silently which would mask a classification miss.
type emitPredicatesParam struct {
	IsScalarAnswer        *bool `json:"is_scalar_answer"`
	IsCountQuestion       *bool `json:"is_count_question"`
	IsCrossComponent      *bool `json:"is_cross_component"`
	IsRelationalLookup    *bool `json:"is_relational_lookup"`
	IsCategoryEnumeration *bool `json:"is_category_enumeration"`
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
			"intent_alternatives": map[string]any{
				"type":        "array",
				"description": "Runner-up intent value(s) when you hesitated; same enum as intent. Empty when confident. Up to 2 entries.",
				"items":       map[string]any{"type": "string", "enum": skill.AnalysisIntentValues()},
			},
			"kind_alternatives": map[string]any{
				"type":        "array",
				"description": "Runner-up question_kind value(s) when you hesitated. Empty when confident. Up to 2 entries.",
				"items":       map[string]any{"type": "string", "enum": skill.AnalysisQuestionKindValues()},
			},
			"shape_alternatives": map[string]any{
				"type":        "array",
				"description": "Runner-up answer_shape value(s) when you hesitated. Empty when confident. Up to 2 entries.",
				"items":       map[string]any{"type": "string", "enum": skill.AnalysisAnswerShapeValues()},
			},
			"predicates": map[string]any{
				"type":        "object",
				"description": "Cross-language semantic self-assessment of the question. Every field is required; emit true OR false explicitly. A missing field is fail-loud rejection, not a silent default.",
				"properties": map[string]any{
					"is_scalar_answer":        map[string]any{"type": "boolean", "description": "True if the answer is a single scalar (a number, a literal, a path) rather than a set or sequence."},
					"is_count_question":       map[string]any{"type": "boolean", "description": "True if the user is asking 'how many X' / a count question, in any language. Implies is_scalar_answer."},
					"is_cross_component":      map[string]any{"type": "boolean", "description": "True if the question compares or relates two distinct subsystems / components / types."},
					"is_relational_lookup":    map[string]any{"type": "boolean", "description": "True if filtering set X by a relationship to Y ('functions that return Z', 'agents that use skill Y')."},
					"is_category_enumeration": map[string]any{"type": "boolean", "description": "True if asking 'what kinds / types / categories of X exist'."},
				},
				"required": []string{"is_scalar_answer", "is_count_question", "is_cross_component", "is_relational_lookup", "is_category_enumeration"},
			},
			"predicate_axis": map[string]any{
				"type":        "string",
				"enum":        skill.AnalysisPredicateAxisValues(),
				"description": "Action-direction axis of the question (call / register / define / return / configure / condition / implement). Empty when no clear verb cue. Used by the evidence ranker to bias items whose anchor matches the axis.",
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
			Keywords: keywords,
			Entities: entities,
			Kind:     kind,
			Shape:    shape,
		},
		AnswerSubject:        parseAnswerSubject(p.AnswerSubject),
		IntentConfidence:     p.IntentConfidence,
		ComplexityConfidence: p.ComplexityConfidence,
		KindConfidence:       p.KindConfidence,
		ShapeConfidence:      p.ShapeConfidence,
		IntentAlternatives:   trimStringSlice(p.IntentAlternatives),
		KindAlternatives:     trimStringSlice(p.KindAlternatives),
		ShapeAlternatives:    trimStringSlice(p.ShapeAlternatives),
		Predicates:           predicates,
		PredicateAxis:        axis,
	}
	ctx.Mutable.SetRequestModel(rm)

	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   buildEmitAnalysisSummary(p, rm, val),
		Timestamp: time.Now(),
	}, nil
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
			"predicates object missing — emit `predicates` with is_scalar_answer / is_count_question / is_cross_component / is_relational_lookup / is_category_enumeration each set to true or false"
	}
	missing := []string{}
	if p.IsScalarAnswer == nil {
		missing = append(missing, "is_scalar_answer")
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
	if len(missing) > 0 {
		return types.SemanticPredicates{}, fmt.Sprintf(
			"predicates missing required field(s): %s — every field must be set explicitly to true or false (no silent default)",
			strings.Join(missing, ", "),
		)
	}
	return types.SemanticPredicates{
		IsScalarAnswer:        *p.IsScalarAnswer,
		IsCountQuestion:       *p.IsCountQuestion,
		IsCrossComponent:      *p.IsCrossComponent,
		IsRelationalLookup:    *p.IsRelationalLookup,
		IsCategoryEnumeration: *p.IsCategoryEnumeration,
	}, ""
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
