package agent

import (
	"encoding/json"
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

func TestNormalizeToolCallParams_DefaultOff(t *testing.T) {
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
