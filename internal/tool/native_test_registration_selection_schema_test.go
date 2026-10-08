package tool

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestNativeRegistrationSelectionActualSchemas(t *testing.T) {
	for _, reminder := range []string{emitChangePlanSchemaReminder, emitPlanSkeletonSchemaReminder} {
		if strings.Count(reminder, types.NativeTestRegistrationAssertionSelectionShapeTeaching) != 1 {
			t.Fatal("rejection reminder conflicts with offered reference/pair alternatives")
		}
	}
	ctx, _, _, _, _, choice := nativeRegistrationSelectionFixture(t, false)
	agent := &types.AgentContext{Mutable: ctx.Mutable, RepoRoot: ctx.RepoRoot, Mode: types.ModeApply, Stage: types.StagePlan}
	for _, entry := range []interface {
		Parameters() json.RawMessage
		ParametersFor(*types.AgentContext) json.RawMessage
	}{&EmitChangePlan{}, &EmitPlanSkeleton{}} {
		raw := entry.ParametersFor(agent)
		var schema struct {
			Properties map[string]struct {
				Items struct {
					Properties map[string]struct {
						Enum        []string `json:"enum"`
						Description string   `json:"description"`
					} `json:"properties"`
					Required []string `json:"required"`
					AnyOf    []any    `json:"anyOf"`
				} `json:"items"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatal(err)
		}
		item := schema.Properties["project_test_observations"].Items
		ref := item.Properties["assertion_ref"]
		if len(ref.Enum) != 1 || ref.Enum[0] != choice.Ref || ref.Description != types.NativeTestRegistrationAssertionSelectionTeaching || len(item.AnyOf) != 2 || len(item.Required) != 3 {
			t.Fatalf("reference/pair alternatives or typed teaching unavailable: %s", raw)
		}
		if !bytes.Equal(raw, nativeTestRegistrationSelectionSchemaForBus(entry.Parameters(), ctx)) {
			t.Fatal("emit recovery used a different schema than model dispatch")
		}
		for _, condition := range []string{"nil", "read", "verify", "foreign", "revoked"} {
			copy := types.AgentContext{Mutable: agent.Mutable, RepoRoot: agent.RepoRoot, Mode: agent.Mode, Stage: agent.Stage}
			switch condition {
			case "nil":
				copy.Mutable = nil
			case "read":
				copy.Mode = types.ModeRead
			case "verify":
				copy.Stage = types.StageVerify
			case "foreign":
				copy.RepoRoot = t.TempDir()
			case "revoked":
				copy.Mutable = types.NewMutableState("history only")
			}
			if got := entry.ParametersFor(&copy); !bytes.Equal(got, entry.Parameters()) || strings.Contains(string(got), choice.Ref) {
				t.Fatalf("inactive dispatch exposed selector (%s)", condition)
			}
		}
	}
}
