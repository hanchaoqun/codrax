package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestEmitPlanSkeleton_HappyPath_InstallsPartialPlan covers the
// canonical shape: a 3-file skeleton with valid metadata installs
// PartialChangePlan with placeholder bodies and zero side-effects on
// the final ChangePlan slot. Subsequent emit_plan_change calls take
// it from there.
func TestEmitPlanSkeleton_HappyPath_InstallsPartialPlan(t *testing.T) {
	tool := &EmitPlanSkeleton{}
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	params := json.RawMessage(`{
		"request": "Add SSH MCP server",
		"summary": "Plan creates a new SSH MCP server in internal/mcp/, registers it via cmd/root.go, and adds the crypto/ssh dependency to go.mod. Three files are touched: one create, two modifies.",
		"changes": [
			{"path": "internal/mcp/ssh.go",  "kind": "create", "rationale": "new SSH MCP server impl"},
			{"path": "cmd/root.go",          "kind": "modify", "rationale": "register the new MCP server", "depends_on": ["internal/mcp/ssh.go"]},
			{"path": "go.mod",               "kind": "modify", "rationale": "declare crypto/ssh require"}
		],
		"acceptance_tests": ["ssh_download tool is callable from a registered MCP server"]
	}`)

	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("execute err: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got rejection: %s", res.Summary)
	}
	pp := bus.Mutable.PartialChangePlan()
	if pp == nil {
		t.Fatalf("PartialChangePlan should be installed")
	}
	if cp := bus.Mutable.ChangePlan(); cp != nil {
		t.Fatalf("ChangePlan must remain nil until finalize, got %s", cp.ID)
	}
	if len(pp.Changes) != 3 {
		t.Fatalf("Changes len = %d, want 3", len(pp.Changes))
	}
	for _, c := range pp.Changes {
		if c.NewContent != "" || c.Patch != "" {
			t.Errorf("change %q has non-empty body in skeleton: NewContent=%q Patch=%q", c.Path, c.NewContent, c.Patch)
		}
	}
	if !strings.Contains(res.Summary, "pending_bodies=3") {
		t.Errorf("summary should report pending body count, got: %s", res.Summary)
	}
}

func TestEmitPlanSkeleton_StructuredPayloadCompatRepairsStringArrays(t *testing.T) {
	tool := &EmitPlanSkeleton{}
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	params := json.RawMessage(`{
		"request": "Add SSH MCP server",
		"summary": "Plan creates a new SSH MCP server in internal/mcp/, registers it via cmd/root.go, and adds the crypto/ssh dependency to go.mod.",
		"changes": "[{\"path\":\"internal/mcp/ssh.go\",\"kind\":\"create\",\"rationale\":\"new SSH MCP server impl\"},{\"path\":\"cmd/root.go\",\"kind\":\"modify\",\"rationale\":\"register the new MCP server\",\"depends_on\":\"[\\\"internal/mcp/ssh.go\\\"]\"}]",
		"acceptance_tests": "[\"ssh_download tool is callable\"]"
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("execute err: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected schema-compatible payload to be accepted, got: %s", res.Summary)
	}
	pp := bus.Mutable.PartialChangePlan()
	if pp == nil || len(pp.Changes) != 2 {
		t.Fatalf("PartialChangePlan should be installed from repaired payload: %+v", pp)
	}
	if len(pp.Changes[1].DependsOn) != 1 || pp.Changes[1].DependsOn[0] != "internal/mcp/ssh.go" {
		t.Fatalf("nested depends_on was not repaired: %+v", pp.Changes[1].DependsOn)
	}
	if len(pp.AcceptanceTests) != 1 || pp.AcceptanceTests[0] != "ssh_download tool is callable" {
		t.Fatalf("acceptance tests were not repaired: %+v", pp.AcceptanceTests)
	}
}

// TestEmitPlanSkeleton_RejectsTruncatedPayload pins the Layer-3
// re-prime: a too-small payload returns the schema reminder so a
// streaming-truncation retry has the structural shape.
func TestEmitPlanSkeleton_RejectsTruncatedPayload(t *testing.T) {
	tool := &EmitPlanSkeleton{}
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	cases := []string{`{}`, `null`, ``, `{"changes": `}
	for _, s := range cases {
		t.Run(s, func(t *testing.T) {
			res, _ := tool.Execute(bus, json.RawMessage(s))
			if res.Success {
				t.Fatalf("expected rejection for payload %q", s)
			}
			if !strings.Contains(res.Summary, "REQUIRED schema") {
				t.Errorf("rejection should re-prime schema, got: %s", res.Summary)
			}
		})
	}
}

// TestEmitPlanSkeleton_RejectsContentInPayload — the schema's
// additionalProperties:false catches new_content / patch fields in
// changes[]. An LLM that mistakenly puts a body in the skeleton is
// reminded what each layer is for.
func TestEmitPlanSkeleton_RejectsContentInPayload(t *testing.T) {
	tool := &EmitPlanSkeleton{}
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	params := json.RawMessage(`{
		"request": "x",
		"summary": "this skeleton is wrong because it carries the body inline; the schema rejects unknown fields",
		"changes": [
			{"path": "a.go", "kind": "create", "rationale": "x", "new_content": "package a"}
		]
	}`)
	res, _ := tool.Execute(bus, params)
	if res.Success {
		t.Fatalf("expected rejection for new_content in skeleton, got success")
	}
	if !strings.Contains(strings.ToLower(res.Summary), "new_content") &&
		!strings.Contains(strings.ToLower(res.Summary), "unknown field") {
		t.Errorf("rejection should mention the offending field, got: %s", res.Summary)
	}
}

// TestEmitPlanSkeleton_RejectsDuplicatePath delegates to the shared
// validatePlanGraphIntegrity helper — covered here to assert the
// helper actually runs from this tool.
func TestEmitPlanSkeleton_RejectsDuplicatePath(t *testing.T) {
	tool := &EmitPlanSkeleton{}
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	params := json.RawMessage(`{
		"request": "x",
		"summary": "duplicate-path rejection should fire from the skeleton stage too — every shared validator is wired in",
		"changes": [
			{"path": "a.go", "kind": "create", "rationale": "x"},
			{"path": "a.go", "kind": "modify", "rationale": "y"}
		]
	}`)
	res, _ := tool.Execute(bus, params)
	if res.Success {
		t.Fatalf("expected rejection for duplicate path")
	}
	if !strings.Contains(res.Summary, "duplicate change") {
		t.Errorf("rejection should mention duplicate, got: %s", res.Summary)
	}
}
