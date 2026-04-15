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

// EmitAnalysis is the analyzer v3 exit tool. It is the single
// structured channel through which the analyzer agent hands its
// classification to the deterministic IR builder.
//
// The tool takes a v3 RequestModel-shaped payload — intent / scenario /
// complexity / writing / hints — and stores it on BusContext.Mutable.
// analyzer.ParseOutput reads this after the ReAct loop exits and runs
// the remaining sub-package pipeline (compiler → risk → hdp →
// counterfactual → gate) to assemble the full AnalysisIR.
//
// Design notes:
//
//   - Uses typed enum names (intent, scenario, answer_shape) directly;
//     no translation layer from LLM-phrased categories.
//   - Does NOT ask the LLM for derived fields that deterministic code
//     can compute. TermGraph, TaskGraph, EvidencePlan, AnswerContract,
//     Hypotheses, RiskMatrix evidence, and the quality gate are all
//     produced post-tool by the normalizer/compiler/risk/hdp/gate
//     sub-packages.
//
// Classified ReadOnly because IsWrite() is the filesystem-write
// boundary; mutating BusContext is not a filesystem write.
type EmitAnalysis struct {
	ReadOnly
	NonEvidenceTool
}

type emitAnalysisParams struct {
	Intent       string   `json:"intent"`
	Scenario     string   `json:"scenario"`
	Complexity   string   `json:"complexity"`
	Keywords     []string `json:"keywords"`
	Entities     []string `json:"entities"`
	QuestionKind string   `json:"question_kind"`
	AnswerShape  string   `json:"answer_shape"`
	Language     string   `json:"language,omitempty"`
}

func (t *EmitAnalysis) Name() string { return "emit_analysis" }

// Description is intentionally short: it names the tool and its
// one-call constraint. The field-level semantics (enum tables, rule
// text) live in the analysis-skill system prompt built from
// internal/skill/analysis_contract.go — the single source of truth.
// The JSON schema returned from Parameters() carries the same enums.
func (t *EmitAnalysis) Description() string {
	return "Analyzer exit channel. Call EXACTLY ONCE at the end of the analyze stage " +
		"to store the classified RequestModel. Required fields and their allowed values are " +
		"defined in the parameter schema."
}

// Parameters returns the emit_analysis JSON schema. Enum arrays are
// pulled from the skill.Analysis*Values() accessors so the schema
// cannot drift from the skill prompt; a consistency test in this
// package verifies the two sides match value-for-value.
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
		Type        string   `json:"type"`
		Enum        []string `json:"enum,omitempty"`
		Description string   `json:"description,omitempty"`
	}
	type arrayProp struct {
		Type        string            `json:"type"`
		Items       map[string]string `json:"items"`
		Description string            `json:"description,omitempty"`
	}

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"intent": stringProp{
				Type:        "string",
				Enum:        skill.AnalysisIntentValues(),
				Description: "Task intent. Pick the closest match to the user's question; see the analysis-skill's Output Format section for each value's meaning.",
			},
			"scenario": stringProp{
				Type:        "string",
				Enum:        skill.AnalysisScenarioValues(),
				Description: "Scenario template picker. See the analysis-skill's Output Format section for each value's meaning.",
			},
			"complexity": stringProp{
				Type:        "string",
				Enum:        skill.AnalysisComplexityValues(),
				Description: "Investigation depth. See the analysis-skill's Output Format section for each value's meaning.",
			},
			"keywords": arrayProp{
				Type:        "array",
				Items:       map[string]string{"type": "string"},
				Description: "Grep search terms. See the analysis-skill's Output Format section for generation rules.",
			},
			"entities": arrayProp{
				Type:        "array",
				Items:       map[string]string{"type": "string"},
				Description: "CamelCase/snake_case symbol names copied VERBATIM from the user's wording. See the analysis-skill's Output Format section for constraints.",
			},
			"question_kind": stringProp{
				Type:        "string",
				Enum:        skill.AnalysisQuestionKindValues(),
				Description: "ERM predicate selector. See the analysis-skill's Output Format section for each value's meaning.",
			},
			"answer_shape": stringProp{
				Type:        "string",
				Enum:        skill.AnalysisAnswerShapeValues(),
				Description: "Expected final-answer structure. See the analysis-skill's Output Format section for each value's meaning.",
			},
			"language": stringProp{
				Type:        "string",
				Enum:        []string{"zh", "en"},
				Description: "Optional. Output language for the final answer. Auto-detected from the raw request if omitted.",
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

	intent := normalizeIntent(p.Intent)
	scenario := normalizeScenario(p.Scenario)
	complexity := normalizeComplexity(p.Complexity)

	// Raw objective — the analyzer gets it from Mutable seeded by
	// the REPL/orchestrator before dispatch. Normalizer builds the
	// TermGraph from this in analyzer.ParseOutput.
	raw := ctx.Mutable.Objective()

	rm := types.RequestModel{
		RawRequest: raw,
		Language:   p.Language,
		Intent:     intent,
		Scenario:   scenario,
		Complexity: complexity,
		AnalyzerHints: types.AnalyzerHints{
			Keywords: trimStringSlice(p.Keywords),
			Entities: trimStringSlice(p.Entities),
			Kind:     normalizeQuestionKind(p.QuestionKind),
			Shape:    normalizeAnswerShape(p.AnswerShape),
		},
	}
	ctx.Mutable.SetRequestModel(rm)

	return types.ToolResult{
		ToolName: t.Name(),
		Success:  true,
		Summary: fmt.Sprintf("analysis emitted: intent=%s scenario=%s complexity=%s kw=%d ent=%d kind=%s shape=%s",
			intent, scenario, complexity, len(rm.AnalyzerHints.Keywords), len(rm.AnalyzerHints.Entities),
			rm.AnalyzerHints.Kind, rm.AnalyzerHints.Shape),
		Timestamp: time.Now(),
	}, nil
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
func normalizeQuestionKind(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "registration", "register":
		return "registration"
	case "mechanism", "process", "flow":
		return "mechanism"
	case "return_value", "return-value", "return", "value_of":
		return "return_value"
	case "conditional", "condition":
		return "conditional"
	case "config_mapping", "config-mapping":
		return "config_mapping"
	case "enumeration", "enumerate", "list", "count":
		return "enumeration"
	case "call_chain", "call-chain", "callchain", "calls":
		return "call_chain"
	}
	return "unknown"
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
