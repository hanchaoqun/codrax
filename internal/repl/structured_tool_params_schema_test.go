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

func TestUnmarshalReplStructuredToolParamsValidatesOptedInNativeProperty(t *testing.T) {
	tool := llm.ToolSchema{
		Name: "emit_plan",
		Parameters: json.RawMessage(`{
		  "type":"object",
		  "x-codrax-native-validation-properties":["actions"],
		  "properties":{"actions":{"type":"array","items":{"type":"object","properties":{
		    "kind":{"type":"string","enum":["derive_rules"]}
		  },"required":["kind"]}}}
		}`),
	}
	var parsed struct {
		Actions []struct {
			Kind string `json:"kind"`
		} `json:"actions"`
	}
	err := unmarshalReplStructuredToolParams(tool,
		[]byte(`{"actions":[{"kind":"assemble_answer"}]}`), &parsed, "rank planner")
	if err == nil || !strings.Contains(err.Error(), "$.actions[0].kind") ||
		!strings.Contains(err.Error(), "exact tool schema") {
		t.Fatalf("native enum violation must be rejected at the owning schema: err=%v parsed=%+v", err, parsed)
	}
}

func TestUnmarshalReplStructuredToolParamsDoesNotHardenUnmarkedNativeProperty(t *testing.T) {
	tool := llm.ToolSchema{
		Name: "legacy_plan",
		Parameters: json.RawMessage(`{
		  "type":"object",
		  "properties":{"actions":{"type":"array","items":{"type":"object","properties":{
		    "kind":{"type":"string","enum":["derive_rules"]}
		  },"required":["kind"]}}}
		}`),
	}
	var parsed struct {
		Actions []struct {
			Kind string `json:"kind"`
		} `json:"actions"`
	}
	if err := unmarshalReplStructuredToolParams(tool,
		[]byte(`{"actions":[{"kind":"assemble_answer"}]}`), &parsed, "legacy planner"); err != nil {
		t.Fatalf("unmarked legacy schema must keep owning-decoder behavior: %v", err)
	}
}

func TestUnmarshalReplStructuredToolParamsRequiresOnlyOptedInLoadBearingField(t *testing.T) {
	tool := llm.ToolSchema{
		Name: "emit_policy",
		Parameters: json.RawMessage(`{
		  "type":"object",
		  "x-codrax-native-validation-required":["requires_diagram"],
		  "properties":{
		    "requires_diagram":{"type":"boolean"},
		    "legacy_default":{"type":"string"}
		  },
		  "required":["requires_diagram","legacy_default"]
		}`),
	}
	var parsed struct {
		RequiresDiagram bool `json:"requires_diagram"`
	}
	err := unmarshalReplStructuredToolParams(tool, []byte(`{}`), &parsed, "policy")
	if err == nil || !strings.Contains(err.Error(), "$.requires_diagram") ||
		!strings.Contains(err.Error(), "required field is missing") {
		t.Fatalf("opted-in missing authority must fail exactly: %v", err)
	}
	if err := unmarshalReplStructuredToolParams(tool,
		[]byte(`{"requires_diagram":false}`), &parsed, "policy"); err != nil {
		t.Fatalf("explicit false must remain valid without hardening unrelated legacy fields: %v", err)
	}
}
