package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

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
// Unlike todo_write (the pre-v3 carrier) this tool:
//
//   - Uses v3 typed enum names (intent, scenario, answer_shape) directly;
//     no legacy question_kind → Intent translation layer.
//   - Does NOT write a TaskList. The analyzer synthesises a single-item
//     TaskList from the raw objective in ParseOutput; v3 does not ask
//     the LLM to decompose into sibling tasks at analyze time.
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
	Writing      bool     `json:"writing"`
	HighRisk     bool     `json:"high_risk"`
	Keywords     []string `json:"keywords"`
	Entities     []string `json:"entities"`
	QuestionKind string   `json:"question_kind"`
	AnswerShape  string   `json:"answer_shape"`
	Language     string   `json:"language,omitempty"`
}

func (t *EmitAnalysis) Name() string { return "emit_analysis" }

func (t *EmitAnalysis) Description() string {
	return "Analyzer v3 exit channel. Call EXACTLY ONCE at the end of the analyze stage " +
		"with the classified request. Required fields: intent, scenario, complexity, writing, " +
		"keywords, entities, question_kind, answer_shape. The system synthesises the TermGraph, " +
		"TaskGraph, RiskMatrix, EvidencePlan, AnswerContract, and Hypotheses deterministically " +
		"from this input — you do not need to provide them."
}

func (t *EmitAnalysis) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "intent": {
      "type": "string",
      "enum": ["explain","root_cause","trace","enumerate","config_query","return_value","refactor","bugfix","security_audit","unknown"],
      "description": "v3 Intent enum. Read questions use explain/trace/enumerate/return_value/config_query; writing tasks use refactor/bugfix/security_audit. Use 'unknown' only if genuinely ambiguous."
    },
    "scenario": {
      "type": "string",
      "enum": ["architecture_explain","root_cause","security_audit","refactor_design","config_trace","performance_bottleneck","generic"],
      "description": "Scenario template picker. architecture_explain for explaining code/mechanism, root_cause for debugging, config_trace for config→behaviour questions, generic if none fit."
    },
    "complexity": {
      "type": "string",
      "enum": ["simple","moderate","complex"],
      "description": "Investigation depth: simple (single lookup), moderate (3-5 files), complex (cross-component flow)."
    },
    "writing": {
      "type": "boolean",
      "description": "True only if the task may mutate files. Read/explain/audit requests are always false."
    },
    "high_risk": {
      "type": "boolean",
      "description": "True only when writing=true AND the change needs design/code review (security-sensitive, schema changes, irreversible ops). Always false for read-only work."
    },
    "keywords": {
      "type": "array",
      "items": {"type": "string"},
      "description": "≥8 search terms for grep. MUST include every CamelCase and snake_case identifier the user mentioned, plus conceptual synonyms. For Chinese questions include BOTH Chinese and English forms."
    },
    "entities": {
      "type": "array",
      "items": {"type": "string"},
      "description": "CamelCase/snake_case symbol names copied VERBATIM from the user's wording. Do NOT translate, re-case, pluralise, or paraphrase. Generic nouns (count, function, agent, handler, module) MUST NOT appear. Leave empty only when the question has no identifier-looking tokens."
    },
    "question_kind": {
      "type": "string",
      "enum": ["registration","mechanism","return_value","conditional","config_mapping","enumeration","call_chain","unknown"],
      "description": "Evidence-requirement shape for ERM's predicate whitelist. registration='which/how many X register Y', mechanism='how does X work', return_value='what does X return', conditional='when/under what condition', config_mapping='what does config key K do', enumeration='list all X', call_chain='which X calls Y'."
    },
    "answer_shape": {
      "type": "string",
      "enum": ["list_of_symbols","step_list","value","boolean","config_value","explanation","none"],
      "description": "Expected final-answer structure. list_of_symbols for sets of names, step_list for mechanism steps, value for a single literal, boolean for yes/no, config_value for a resolved key:value, explanation for long prose."
    },
    "language": {
      "type": "string",
      "enum": ["zh","en"],
      "description": "Optional. Output language for the final answer. Auto-detected from the raw request if omitted."
    }
  },
  "required": ["intent","scenario","complexity","writing","keywords","entities","question_kind","answer_shape"]
}`)
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
	complexity := normalizeComplexityV3(p.Complexity)

	// Raw objective — the analyzer gets it from the TaskList seeded by
	// the REPL/orchestrator before dispatch. Normalizer builds the
	// TermGraph from this in analyzer.ParseOutput.
	raw := ""
	if tl := ctx.Mutable.TaskList(); tl.Objective != "" {
		raw = tl.Objective
	}

	rm := types.RequestModel{
		RawRequest: raw,
		Language:   p.Language,
		Intent:     intent,
		Scenario:   scenario,
		Complexity: complexity,
		AnalyzerHints: types.AnalyzerHints{
			Keywords: trimStringSliceV3(p.Keywords),
			Entities: trimStringSliceV3(p.Entities),
			Kind:     normalizeQuestionKindV3(p.QuestionKind),
			Shape:    normalizeAnswerShapeV3(p.AnswerShape),
		},
	}
	// Writing / HighRisk are consumed by risk.DerivePolicy through a
	// lightweight side channel — we stash them on the RequestModel via
	// the zero-cost Ambiguities slice so the downstream pipeline does
	// not need a second carrier. The analyzer reads them back by
	// looking for the well-known sentinel clauses.
	//
	// (No: storing on Ambiguities is semantic abuse. Instead we extend
	// the RequestModel with a `Writing` field.)
	rm.Writing = p.Writing
	rm.HighRisk = p.HighRisk

	ctx.Mutable.SetRequestModel(rm)

	return types.ToolResult{
		ToolName: t.Name(),
		Success:  true,
		Summary: fmt.Sprintf("analysis emitted: intent=%s scenario=%s complexity=%s writing=%t kw=%d ent=%d kind=%s shape=%s",
			intent, scenario, complexity, p.Writing, len(rm.AnalyzerHints.Keywords), len(rm.AnalyzerHints.Entities),
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
	case "refactor":
		return types.IntentRefactor
	case "bugfix", "bug_fix", "bug-fix":
		return types.IntentBugfix
	case "security_audit", "security-audit", "security":
		return types.IntentSecurityAudit
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
	case "security_audit", "security":
		return types.ScenarioSecurityAudit
	case "refactor_design", "refactor":
		return types.ScenarioRefactorDesign
	case "config_trace", "config":
		return types.ScenarioConfigTrace
	case "performance_bottleneck", "performance":
		return types.ScenarioPerformanceBottleneck
	}
	return types.ScenarioGeneric
}

// normalizeComplexityV3 maps LLM-emitted complexity strings to the
// typed v3 Complexity enum. Unknown values default to moderate.
func normalizeComplexityV3(s string) types.Complexity {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "simple":
		return types.ComplexitySimple
	case "complex":
		return types.ComplexityComplex
	}
	return types.ComplexityModerate
}

// normalizeQuestionKindV3 keeps the legacy ERM enum strings stable
// across the v3 migration so downstream ERM predicate whitelisting
// does not need to change. Unknown values fall back to "unknown".
func normalizeQuestionKindV3(s string) string {
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

// normalizeAnswerShapeV3 maps LLM-emitted answer_shape strings to the
// canonical set consumed by the finalizer. Empty or unknown falls
// back to "none".
func normalizeAnswerShapeV3(s string) string {
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

// trimStringSliceV3 drops empty/whitespace-only entries and trims
// each remaining element. Returns nil for an empty input so the
// field stays omitempty-friendly. Named *V3 to avoid collision with
// the legacy todo_write helper during the migration.
func trimStringSliceV3(in []string) []string {
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
