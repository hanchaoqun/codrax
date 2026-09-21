package agent

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	toolpkg "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the planner's actual tool-schema projection, with no execution
// report available. Splitting one plan across calls must not change the
// authority of the same optional prose checklist or the planner's tool rights.
func TestPlannerAcceptanceChecklistSchemaAgreesAcrossEmissionModes(t *testing.T) {
	reg := toolpkg.NewRegistry()
	toolpkg.RegisterDefaults(reg)
	base := NewBaseAgent(types.AgentPlanner, &Dependencies{Tools: reg}, nil)
	ctx := &types.AgentContext{
		Stage: types.StagePlan, Mode: types.ModeApply,
		Mutable: types.NewMutableState("repair the implementation without changing existing tests"),
	}
	schemas := base.buildToolSchemas(&skill.Config{ToolSuggestions: []string{
		"emit_change_plan", "emit_plan_skeleton", "emit_plan_change", "run_tests", "exec_command", "apply_patch",
	}}, ctx)
	checklists := map[string]map[string]any{}
	for _, schema := range schemas {
		if schema.Name == "exec_command" || schema.Name == "apply_patch" {
			t.Fatalf("checklist teaching must not expand planner permissions: %s", schema.Name)
		}
		if schema.Name != "emit_change_plan" && schema.Name != "emit_plan_skeleton" {
			continue
		}
		var decoded struct {
			Properties map[string]map[string]any `json:"properties"`
			Required   []string                  `json:"required"`
		}
		if err := json.Unmarshal(schema.Parameters, &decoded); err != nil {
			t.Fatal(err)
		}
		for _, field := range decoded.Required {
			if field == "acceptance_tests" {
				t.Fatalf("%s made the planning checklist mandatory", schema.Name)
			}
		}
		field := decoded.Properties["acceptance_tests"]
		if field["type"] != "array" || !reflect.DeepEqual(field["items"], map[string]any{"type": "string"}) {
			t.Fatalf("%s changed the native JSON string-array carrier: %#v", schema.Name, field)
		}
		description, _ := field["description"].(string)
		for _, want := range []string{"planning checklist", "does not itself create proof or a hard completion obligation", "exact project_test_observations"} {
			if !strings.Contains(description, want) {
				t.Errorf("%s checklist teaching misses %q: %s", schema.Name, want, description)
			}
		}
		if strings.Contains(description, "must cover") {
			t.Errorf("%s promotes prose to a mandatory execution contract: %s", schema.Name, description)
		}
		checklists[schema.Name] = field
	}
	if len(checklists) != 2 || !reflect.DeepEqual(checklists["emit_change_plan"], checklists["emit_plan_skeleton"]) {
		t.Fatalf("full and split plans must publish the same checklist contract: %#v", checklists)
	}
}
