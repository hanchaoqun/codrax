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
