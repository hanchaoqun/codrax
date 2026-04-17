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
	Intent       string           `json:"intent"`
	Scenario     string           `json:"scenario"`
	Complexity   string           `json:"complexity"`
	Keywords     []string         `json:"keywords"`
	Entities     []string         `json:"entities"`
	QuestionKind string           `json:"question_kind"`
	AnswerShape  string           `json:"answer_shape"`
	Language     string           `json:"language,omitempty"`
	SubTopics    []types.SubTopic `json:"sub_topics,omitempty"`
}

func (t *EmitAnalysis) Name() string { return "emit_analysis" }

// Description is a single sentence: what the tool does and its
// one-call-per-dispatch constraint. Strategy guidance — how to pick
// an enum value, how many keywords to emit, what not to put in
// entities — lives in the analysis-skill system prompt, not here.
func (t *EmitAnalysis) Description() string {
	return "Stores the classified RequestModel on MutableState so the " +
		"deterministic analyzer pipeline can assemble the full AnalysisIR. " +
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
		},
		"required": []string{
			"intent", "scenario", "complexity", "keywords", "entities", "question_kind", "answer_shape",
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
	}
	ctx.Mutable.SetRequestModel(rm)

	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   buildEmitAnalysisSummary(p, rm, val),
		Timestamp: time.Now(),
	}, nil
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
//   1. types.StripConversationPrefix — strips a full REPL prefix
//      when the LLM pasted the whole effective-request wrapper.
//   2. Pollution-marker check on what remains. When any marker is
//      still present, the summary is considered unusable prose and
//      is replaced by a comma-joined entity list if available, else
//      emptied. An empty summary causes compiler.expandEvidenceNodes
//      and the renderer to fall back to the node ID — ugly, but less
//      misleading than rendering "## Prior conversation ..." as a
//      sub-topic title.
//   3. Whitespace + length cap at subTopicSummaryMaxChars so a
//      pasted paragraph cannot dominate the live task-row width.
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
