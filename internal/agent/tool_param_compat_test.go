package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	toolpkg "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestNormalizeToolCallParams_RepairUsesPerAgentConfig(t *testing.T) {
	base := &BaseAgent{
		name: types.AgentExplorer,
		deps: &Dependencies{
			ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{
				types.AgentExplorer: {Mode: types.ToolParamCompatRepair},
			},
		},
	}
	calls := []llm.ToolCall{{
		ID:     "call_1",
		Name:   "read_file",
		Params: json.RawMessage(`{"path":"internal/types/enums.go","offset":"146","limit":"25"}`),
	}}
	schemas := []llm.ToolSchema{{
		Name:       "read_file",
		Parameters: readFileCompatTestSchema(),
	}}

	got := base.normalizeToolCallParams(calls, schemas)
	if string(got[0].Params) == string(calls[0].Params) {
		t.Fatalf("expected repaired params, got unchanged %s", got[0].Params)
	}
	if string(calls[0].Params) != `{"path":"internal/types/enums.go","offset":"146","limit":"25"}` {
		t.Fatalf("normalizeToolCallParams mutated caller slice: %s", calls[0].Params)
	}

	var decoded struct {
		Offset int `json:"offset"`
		Limit  int `json:"limit"`
	}
	if err := json.Unmarshal(got[0].Params, &decoded); err != nil {
		t.Fatalf("repaired params are invalid JSON: %v", err)
	}
	if decoded.Offset != 146 || decoded.Limit != 25 {
		t.Fatalf("offset/limit = %d/%d, want 146/25; raw=%s", decoded.Offset, decoded.Limit, got[0].Params)
	}
}

func TestNormalizeToolCallParams_RepairsLineOffsetString(t *testing.T) {
	base := &BaseAgent{
		name: types.AgentExplorer,
		deps: &Dependencies{
			ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{
				types.AgentExplorer: {Mode: types.ToolParamCompatRepair},
			},
		},
	}
	calls := []llm.ToolCall{{
		ID:     "call_1",
		Name:   "read_file",
		Params: json.RawMessage(`{"path":"internal/types/enums.go","line_offset":"146","limit":"25"}`),
	}}
	schemas := []llm.ToolSchema{{
		Name:       "read_file",
		Parameters: readFileCompatTestSchema(),
	}}

	got := base.normalizeToolCallParams(calls, schemas)
	var decoded struct {
		LineOffset int `json:"line_offset"`
		Limit      int `json:"limit"`
	}
	if err := json.Unmarshal(got[0].Params, &decoded); err != nil {
		t.Fatalf("repaired params are invalid JSON: %v", err)
	}
	if decoded.LineOffset != 146 || decoded.Limit != 25 {
		t.Fatalf("line_offset/limit = %d/%d, want 146/25; raw=%s", decoded.LineOffset, decoded.Limit, got[0].Params)
	}
}

func TestNormalizeToolCallParams_RepairsTraceQueryRecentScalarsFromRealSchema(t *testing.T) {
	base := &BaseAgent{
		name: types.AgentExplorer,
		deps: &Dependencies{
			ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{
				types.AgentExplorer: {Mode: types.ToolParamCompatRepair},
			},
		},
	}
	calls := []llm.ToolCall{{
		ID:   "call_1",
		Name: "trace_query",
		Params: json.RawMessage(`{
			"source":"path",
			"path":"sample.systrace",
			"view":"wakeup_chain",
			"thread":"com.tencent.mm-36379",
			"pid":"36379",
			"time_start":"2942.124416s",
			"time_end":"2942.260210s",
			"line_start":"1102710",
			"line_end":"1139190",
			"max_depth":"6",
			"max_branches":"8",
			"min_duration_ms":"1",
			"include_window_stats":"true",
			"limit":"40",
			"trace_flavor":"harmony_hitrace",
			"platform":"donghu",
			"event_types":"cpu_frequency_limits,softirq,storage",
			"pattern":"jank_frames=7",
			"span_name":"Choreographer#doFrame",
			"interaction_direction":"both",
			"recipe_name":"sleep_root_cause"
		}`),
	}}
	schemas := []llm.ToolSchema{{
		Name:       "trace_query",
		Parameters: (&toolpkg.TraceQuery{}).Parameters(),
	}}

	got := base.normalizeToolCallParams(calls, schemas)
	var decoded struct {
		PID                int      `json:"pid"`
		TimeStart          string   `json:"time_start"`
		TimeEnd            string   `json:"time_end"`
		LineStart          int      `json:"line_start"`
		LineEnd            int      `json:"line_end"`
		MaxDepth           int      `json:"max_depth"`
		MaxBranches        int      `json:"max_branches"`
		MinDurationMS      float64  `json:"min_duration_ms"`
		IncludeWindowStats bool     `json:"include_window_stats"`
		Limit              int      `json:"limit"`
		TraceFlavor        string   `json:"trace_flavor"`
		Platform           string   `json:"platform"`
		EventTypes         []string `json:"event_types"`
		Pattern            string   `json:"pattern"`
		SpanName           string   `json:"span_name"`
		Interaction        string   `json:"interaction_direction"`
		RecipeName         string   `json:"recipe_name"`
	}
	if err := json.Unmarshal(got[0].Params, &decoded); err != nil {
		t.Fatalf("repaired trace_query params are invalid JSON: %v\n%s", err, got[0].Params)
	}
	if decoded.PID != 36379 || decoded.LineStart != 1102710 || decoded.LineEnd != 1139190 ||
		decoded.MaxDepth != 6 || decoded.MaxBranches != 8 || decoded.MinDurationMS != 1 ||
		!decoded.IncludeWindowStats || decoded.Limit != 40 {
		t.Fatalf("trace_query numeric/bool scalar repair failed: %+v\nraw=%s", decoded, got[0].Params)
	}
	if decoded.TimeStart != "2942.124416s" || decoded.TimeEnd != "2942.260210s" ||
		decoded.TraceFlavor != "harmony_hitrace" || decoded.Platform != "donghu" ||
		decoded.Pattern != "jank_frames=7" || decoded.SpanName != "Choreographer#doFrame" || decoded.Interaction != "both" ||
		decoded.RecipeName != "sleep_root_cause" {
		t.Fatalf("trace_query string fields should survive repair unchanged: %+v\nraw=%s", decoded, got[0].Params)
	}
	if len(decoded.EventTypes) != 3 || decoded.EventTypes[0] != "cpu_frequency_limits" ||
		decoded.EventTypes[1] != "softirq" || decoded.EventTypes[2] != "storage" {
		t.Fatalf("trace_query event_types string should repair to []string: %+v\nraw=%s", decoded.EventTypes, got[0].Params)
	}
}

func TestNormalizeToolCallParams_AuditDoesNotMutate(t *testing.T) {
	base := &BaseAgent{
		name: types.AgentExplorer,
		deps: &Dependencies{
			ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{
				types.AgentExplorer: {Mode: types.ToolParamCompatAudit},
			},
		},
	}
	calls := []llm.ToolCall{{
		ID:     "call_1",
		Name:   "read_file",
		Params: json.RawMessage(`{"path":"x.go","offset":"7"}`),
	}}
	schemas := []llm.ToolSchema{{Name: "read_file", Parameters: readFileCompatTestSchema()}}

	got := base.normalizeToolCallParams(calls, schemas)
	if string(got[0].Params) != string(calls[0].Params) {
		t.Fatalf("audit mode mutated params: got %s want %s", got[0].Params, calls[0].Params)
	}
}

func TestNormalizeToolCallParams_RepairsBooleanStringsWhenSchemaAvailable(t *testing.T) {
	base := &BaseAgent{
		name: types.AgentExplorer,
		deps: &Dependencies{
			ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{
				types.AgentExplorer: {Mode: types.ToolParamCompatRepair},
			},
		},
	}
	calls := []llm.ToolCall{{
		ID:     "call_1",
		Name:   "grep",
		Params: json.RawMessage(`{"pattern":"func buildAnalysisIR","files_only":"True"}`),
	}}
	schemas := []llm.ToolSchema{{Name: "grep", Parameters: grepCompatTestSchema()}}

	got := base.normalizeToolCallParams(calls, schemas)
	var decoded struct {
		FilesOnly bool `json:"files_only"`
	}
	if err := json.Unmarshal(got[0].Params, &decoded); err != nil {
		t.Fatalf("repaired params are invalid JSON: %v\n%s", err, got[0].Params)
	}
	if !decoded.FilesOnly {
		t.Fatalf("files_only should be normalized true: %s", got[0].Params)
	}
}

func TestNormalizeToolCallParams_RepairsGrepRuntimeSearchScalarsFromRealSchema(t *testing.T) {
	base := &BaseAgent{
		name: types.AgentExplorer,
		deps: &Dependencies{
			ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{
				types.AgentExplorer: {Mode: types.ToolParamCompatRepair},
			},
		},
	}
	calls := []llm.ToolCall{{
		ID:   "call_1",
		Name: "grep",
		Params: json.RawMessage(`{
			"pattern":"[GT]Thread#1",
			"fixedString":"true",
			"lineStart":"10",
			"lineEnd":"12",
			"contextLines":"2"
		}`),
	}}
	schemas := []llm.ToolSchema{{
		Name:       "grep",
		Parameters: (&toolpkg.GrepTool{}).Parameters(),
	}}

	got := base.normalizeToolCallParams(calls, schemas)
	if string(got[0].Params) == string(calls[0].Params) {
		t.Fatalf("expected real grep schema to repair runtime search fields")
	}
	var decoded struct {
		FixedString  bool `json:"fixed_string"`
		LineStart    int  `json:"line_start"`
		LineEnd      int  `json:"line_end"`
		ContextLines int  `json:"context_lines"`
	}
	if err := json.Unmarshal(got[0].Params, &decoded); err != nil {
		t.Fatalf("repaired params are invalid JSON: %v\n%s", err, got[0].Params)
	}
	if !decoded.FixedString || decoded.LineStart != 10 || decoded.LineEnd != 12 || decoded.ContextLines != 2 {
		t.Fatalf("unexpected normalized grep params: %+v raw=%s", decoded, got[0].Params)
	}
}

func TestNormalizeToolCallParams_RepairStringWrappedArrayWithBareQuotes(t *testing.T) {
	base := &BaseAgent{
		name: types.AgentExplorer,
		deps: &Dependencies{
			ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{
				types.AgentExplorer: {Mode: types.ToolParamCompatRepair},
			},
		},
	}
	calls := []llm.ToolCall{{
		ID:     "call_1",
		Name:   "emit_evidence",
		Params: json.RawMessage(`{"items":"[{\"summary\":\"EvidenceNodeSpec appends \"_tN\" and mentions a \"key\": \"value\" pair\",\"line_start\":\"82\"}]"}`),
	}}
	schemas := []llm.ToolSchema{{
		Name:       "emit_evidence",
		Parameters: evidenceItemsCompatTestSchema(),
	}}

	got := base.normalizeToolCallParams(calls, schemas)
	if string(got[0].Params) == string(calls[0].Params) {
		t.Fatalf("expected repaired emit_evidence params, got unchanged %s", got[0].Params)
	}
	var decoded struct {
		Items []struct {
			Summary   string `json:"summary"`
			LineStart int    `json:"line_start"`
		} `json:"items"`
	}
	if err := json.Unmarshal(got[0].Params, &decoded); err != nil {
		t.Fatalf("repaired params are invalid JSON: %v\n%s", err, got[0].Params)
	}
	if len(decoded.Items) != 1 || decoded.Items[0].LineStart != 82 {
		t.Fatalf("unexpected repaired items: %+v", decoded.Items)
	}
	if summary := decoded.Items[0].Summary; !strings.Contains(summary, `"_tN"`) || !strings.Contains(summary, `"key": "value"`) {
		t.Fatalf("summary lost quoted text: %q", summary)
	}
}

func TestNormalizeToolCallParams_RepairFunctionArgumentEnvelope(t *testing.T) {
	base := &BaseAgent{
		name: types.AgentAnalyzer,
		deps: &Dependencies{
			ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{
				types.AgentAnalyzer: {Mode: types.ToolParamCompatRepair},
			},
		},
	}
	calls := []llm.ToolCall{{
		ID:     "call_1",
		Name:   "emit_analysis",
		Params: json.RawMessage(`{"arguments":"{\"intent\":\"explain\",\"keywords\":[\"agent\"],\"limit\":\"25\"}"}`),
	}}
	schemas := []llm.ToolSchema{{
		Name:       "emit_analysis",
		Parameters: analysisEnvelopeCompatTestSchema(),
	}}

	got := base.normalizeToolCallParams(calls, schemas)
	if string(got[0].Params) == string(calls[0].Params) {
		t.Fatalf("expected repaired function argument envelope, got unchanged %s", got[0].Params)
	}
	var decoded struct {
		Intent   string   `json:"intent"`
		Keywords []string `json:"keywords"`
		Limit    int      `json:"limit"`
	}
	if err := json.Unmarshal(got[0].Params, &decoded); err != nil {
		t.Fatalf("repaired params are invalid JSON: %v\n%s", err, got[0].Params)
	}
	if decoded.Intent != "explain" || strings.Join(decoded.Keywords, "|") != "agent" || decoded.Limit != 25 {
		t.Fatalf("unexpected repaired params: %+v", decoded)
	}
}

func TestNormalizeToolCallParams_AddsConservativeEmitAnalysisProfiles(t *testing.T) {
	base := &BaseAgent{
		name: types.AgentAnalyzer,
		deps: &Dependencies{
			ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{
				types.AgentAnalyzer: {Mode: types.ToolParamCompatRepair},
			},
		},
	}
	calls := []llm.ToolCall{{
		ID:   "call_1",
		Name: "emit_analysis",
		Params: json.RawMessage(`{
		  "intent":"explain",
		  "scenario":"architecture_explain",
		  "complexity":"moderate",
		  "keywords":["agent"],
		  "entities":["Agent"],
		  "question_kind":"mechanism",
		  "intent_confidence":0.7,
		  "complexity_confidence":0.7,
		  "kind_confidence":0.7,
		  "predicates":{"is_scalar_answer":false},
		  "diagnostic_profile":{"is_diagnostic":false,"current_risk":false,"historical_regression":false,"current_version_check":false,"confidence":0.7}
		}`),
	}}
	schemas := []llm.ToolSchema{{
		Name:       "emit_analysis",
		Parameters: analysisEnvelopeCompatTestSchema(),
	}}

	got := base.normalizeToolCallParams(calls, schemas)
	if string(got[0].Params) == string(calls[0].Params) {
		t.Fatalf("expected emit_analysis profile default repair, got unchanged")
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(got[0].Params, &decoded); err != nil {
		t.Fatalf("repaired params are invalid JSON: %v\n%s", err, got[0].Params)
	}
	for _, field := range []string{"answer_role_profile", "error_granularity_profile"} {
		if _, ok := decoded[field]; !ok {
			t.Fatalf("expected %s to be filled in repaired params: %s", field, got[0].Params)
		}
	}
	if _, ok := decoded["diagnostic_profile"]; !ok {
		t.Fatalf("existing diagnostic_profile should be preserved: %s", got[0].Params)
	}
}

func TestNormalizeToolCallParams_UnwrapsEmitAnalysisStringEnumArtifacts(t *testing.T) {
	base := &BaseAgent{
		name: types.AgentAnalyzer,
		deps: &Dependencies{
			ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{
				types.AgentAnalyzer: {Mode: types.ToolParamCompatRepair},
			},
		},
	}
	calls := []llm.ToolCall{{
		ID:   "call_1",
		Name: "emit_analysis",
		Params: json.RawMessage(`{
		  "intent":"\"explain\"",
		  "scenario":"\"architecture_explain\"",
		  "complexity":"\"complex\"",
		  "keywords":["codrax","opencode"],
		  "entities":["codrax","opencode"],
		  "question_kind":"\"mechanism\"",
		  "language":"\"zh\"",
		  "predicate_axis":"\"\"",
		  "intent_confidence":0.9,
		  "complexity_confidence":0.9,
		  "kind_confidence":0.9,
		  "predicates":{"is_scalar_answer":false},
		  "diagnostic_profile":{"is_diagnostic":false,"current_risk":false,"historical_regression":false,"current_version_check":false,"confidence":0.7}
		}`),
	}}
	schemas := []llm.ToolSchema{{
		Name:       "emit_analysis",
		Parameters: analysisEnumCompatTestSchema(),
	}}

	got := base.normalizeToolCallParams(calls, schemas)
	if string(got[0].Params) == string(calls[0].Params) {
		t.Fatalf("expected emit_analysis enum artifact repair, got unchanged")
	}
	var decoded struct {
		Intent        string `json:"intent"`
		Scenario      string `json:"scenario"`
		Complexity    string `json:"complexity"`
		QuestionKind  string `json:"question_kind"`
		Language      string `json:"language"`
		PredicateAxis string `json:"predicate_axis"`
	}
	if err := json.Unmarshal(got[0].Params, &decoded); err != nil {
		t.Fatalf("repaired params are invalid JSON: %v\n%s", err, got[0].Params)
	}
	if decoded.Intent != "explain" || decoded.Scenario != "architecture_explain" ||
		decoded.Complexity != "complex" || decoded.QuestionKind != "mechanism" ||
		decoded.Language != "zh" || decoded.PredicateAxis != "" {
		t.Fatalf("unexpected repaired analysis enums: %+v\nraw=%s", decoded, got[0].Params)
	}
}

func TestNormalizeToolCallParams_NormalizesEmitAnalysisDiagramHintShapes(t *testing.T) {
	base := &BaseAgent{
		name: types.AgentAnalyzer,
		deps: &Dependencies{
			ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{
				types.AgentAnalyzer: {Mode: types.ToolParamCompatRepair},
			},
		},
	}
	calls := []llm.ToolCall{
		{
			ID:   "call_1",
			Name: "emit_analysis",
			Params: json.RawMessage(`{
			  "intent":"trace",
			  "scenario":"architecture_explain",
			  "complexity":"moderate",
			  "keywords":["buildAnalysisIR"],
			  "entities":["buildAnalysisIR"],
			  "question_kind":"call_chain",
			  "intent_confidence":0.9,
			  "complexity_confidence":0.8,
			  "kind_confidence":0.8,
			  "predicates":{"is_scalar_answer":false},
			  "diagram_hint":["call_dag"]
			}`),
		},
		{
			ID:   "call_2",
			Name: "emit_analysis",
			Params: json.RawMessage(`{
			  "intent":"trace",
			  "scenario":"architecture_explain",
			  "complexity":"moderate",
			  "keywords":["buildAnalysisIR"],
			  "entities":["buildAnalysisIR"],
			  "question_kind":"call_chain",
			  "intent_confidence":0.9,
			  "complexity_confidence":0.8,
			  "kind_confidence":0.8,
			  "predicates":{"is_scalar_answer":false},
			  "diagram_hint":"<parameter=kind>\nsequence"
			}`),
		},
		{
			ID:   "call_3",
			Name: "emit_analysis",
			Params: json.RawMessage(`{
			  "intent":"trace",
			  "scenario":"architecture_explain",
			  "complexity":"moderate",
			  "keywords":["buildAnalysisIR"],
			  "entities":["buildAnalysisIR"],
			  "question_kind":"call_chain",
			  "intent_confidence":0.9,
			  "complexity_confidence":0.8,
			  "kind_confidence":0.8,
			  "predicates":{"is_scalar_answer":false},
			  "diagram_hint":"kind: call-dag"
			}`),
		},
	}
	schemas := []llm.ToolSchema{{Name: "emit_analysis", Parameters: analysisEnumCompatTestSchema()}}

	got := base.normalizeToolCallParams(calls, schemas)
	for i, call := range got {
		var decoded struct {
			DiagramHint struct {
				Kind string `json:"kind"`
			} `json:"diagram_hint"`
		}
		if err := json.Unmarshal(call.Params, &decoded); err != nil {
			t.Fatalf("call %d repaired params invalid JSON: %v\n%s", i, err, call.Params)
		}
		want := []string{"call_dag", "sequence", "call_dag"}[i]
		if decoded.DiagramHint.Kind != want {
			t.Fatalf("call %d diagram_hint.kind=%q want %q\nraw=%s", i, decoded.DiagramHint.Kind, want, call.Params)
		}
	}
}

func TestDiagramKindFromStringRejectsConflictingKinds(t *testing.T) {
	if _, ok := diagramKindFromString("sequence or call_dag"); ok {
		t.Fatal("conflicting diagram kinds must not be normalized by taking the first match")
	}
}

func TestNormalizeAnalyzerPrescanGrepCompat_RepairModeSetsFilesOnly(t *testing.T) {
	base := &BaseAgent{
		name: types.AgentAnalyzer,
		deps: &Dependencies{
			ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{
				types.AgentAnalyzer: {Mode: types.ToolParamCompatRepair},
			},
		},
	}
	ctx := &types.AgentContext{Stage: types.StageAnalyze}
	tc := llm.ToolCall{Name: "grep", Params: json.RawMessage(`{"pattern":"StageAnalyze","files_only":false}`)}

	got, ok := base.normalizeAnalyzerPrescanGrepCompat(ctx, tc)
	if !ok {
		t.Fatal("expected analyze grep compatibility repair")
	}
	var decoded struct {
		FilesOnly bool `json:"files_only"`
	}
	if err := json.Unmarshal(got.Params, &decoded); err != nil {
		t.Fatalf("repaired grep params are invalid JSON: %v\n%s", err, got.Params)
	}
	if !decoded.FilesOnly {
		t.Fatalf("files_only should be normalized true: %s", got.Params)
	}
}

func TestNormalizeToolCallParams_NoInjectedPolicyIsNoOp(t *testing.T) {
	base := &BaseAgent{
		name: types.AgentExplorer,
		deps: &Dependencies{},
	}
	calls := []llm.ToolCall{{
		ID:     "call_1",
		Name:   "read_file",
		Params: json.RawMessage(`{"path":"x.go","offset":"7"}`),
	}}
	schemas := []llm.ToolSchema{{Name: "read_file", Parameters: readFileCompatTestSchema()}}

	got := base.normalizeToolCallParams(calls, schemas)
	if string(got[0].Params) != string(calls[0].Params) {
		t.Fatalf("default-off mode mutated params: got %s want %s", got[0].Params, calls[0].Params)
	}
}

func readFileCompatTestSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string"},
			"line_offset": {"type": "integer"},
			"offset": {"type": "integer"},
			"limit": {"type": "integer"}
		},
		"required": ["path"]
	}`)
}

func grepCompatTestSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {"type": "string"},
			"files_only": {"type": "boolean"},
			"file_type": {"type": "string"}
		},
		"required": ["pattern"]
	}`)
}

func evidenceItemsCompatTestSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"items": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"summary": {"type": "string"},
						"line_start": {"type": "integer"}
					}
				}
			}
		},
		"required": ["items"]
	}`)
}

func analysisEnvelopeCompatTestSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"intent": {"type": "string"},
			"keywords": {"type": "array", "items": {"type": "string"}},
			"limit": {"type": "integer"}
		}
	}`)
}

func analysisEnumCompatTestSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"intent": {"type": "string", "enum": ["explain", "root_cause", "trace"]},
			"scenario": {"type": "string", "enum": ["generic", "architecture_explain", "root_cause"]},
			"complexity": {"type": "string", "enum": ["simple", "moderate", "complex"]},
			"question_kind": {"type": "string", "enum": ["mechanism", "enumeration", "comparison"]},
			"language": {"type": "string", "enum": ["zh", "en"]},
			"predicate_axis": {"type": "string", "enum": ["call", "register", ""]},
			"keywords": {"type": "array", "items": {"type": "string"}},
			"entities": {"type": "array", "items": {"type": "string"}},
			"intent_confidence": {"type": "number"},
			"complexity_confidence": {"type": "number"},
			"kind_confidence": {"type": "number"},
			"predicates": {"type": "object"},
			"diagnostic_profile": {"type": "object"},
			"answer_role_profile": {"type": "object"},
			"error_granularity_profile": {"type": "object"}
		}
	}`)
}
