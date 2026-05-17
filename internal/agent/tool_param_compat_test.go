package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
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
			"offset": {"type": "integer"},
			"limit": {"type": "integer"}
		},
		"required": ["path"]
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
