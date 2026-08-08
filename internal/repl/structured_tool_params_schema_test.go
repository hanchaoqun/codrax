package repl

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
)

func TestUnmarshalReplStructuredToolParamsRevalidatesNormalizedNestedEnum(t *testing.T) {
	tool := llm.ToolSchema{
		Name: "emit_plan",
		Parameters: json.RawMessage(`{
		  "type":"object",
		  "properties":{"actions":{"type":"array","items":{"type":"object","properties":{
		    "kind":{"type":"string","enum":["derive_rules"]}
		  },"required":["kind"]}}},
		  "required":["actions"]
		}`),
	}
	var parsed struct {
		Actions []struct {
			Kind string `json:"kind"`
		} `json:"actions"`
	}
	err := unmarshalReplStructuredToolParams(tool,
		[]byte(`{"actions":"[{\"kind\":\"assemble_answer\"}]"}`), &parsed, "rank planner")
	if err == nil || !strings.Contains(err.Error(), "$.actions[0].kind") || !strings.Contains(err.Error(), "same tool schema") {
		t.Fatalf("compatibility normalization must not bypass the nested enum: err=%v parsed=%+v", err, parsed)
	}
}

func TestUnmarshalReplStructuredToolParamsAcceptsSchemaValidNormalizedNestedArray(t *testing.T) {
	tool := llm.ToolSchema{
		Name: "emit_plan",
		Parameters: json.RawMessage(`{
		  "type":"object",
		  "properties":{"actions":{"type":"array","items":{"type":"object","properties":{
		    "kind":{"type":"string","enum":["derive_rules"]}
		  },"required":["kind"]}}},
		  "required":["actions"]
		}`),
	}
	var parsed struct {
		Actions []struct {
			Kind string `json:"kind"`
		} `json:"actions"`
	}
	if err := unmarshalReplStructuredToolParams(tool,
		[]byte(`{"actions":"[{\"kind\":\"derive_rules\"}]"}`), &parsed, "rank planner"); err != nil {
		t.Fatalf("schema-valid compatibility repair should remain lossless: %v", err)
	}
	if len(parsed.Actions) != 1 || parsed.Actions[0].Kind != "derive_rules" {
		t.Fatalf("normalized action lost: %+v", parsed.Actions)
	}
}
