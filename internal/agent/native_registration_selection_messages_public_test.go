package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Use real native producer identities and capture the actual planner adapter
// request. The controller authorization is an explicit protocol fixture seam;
// this test neither registers a test nor claims a new verification execution.
func TestNativeRegistrationSelectionActualPlannerRequest(t *testing.T) {
	sourceBus, _ := nativeIdentityPublicRun(t, "test_other", false)
	report := sourceBus.Mutable.ChangeReport()
	root, err := filepath.EvalSymlinks(sourceBus.RepoRoot)
	if err != nil {
		t.Fatal(err)
	}
	sha := strings.Repeat("a", 40)
	delivery := types.VerificationDeliverySnapshot{SourcePlanID: report.PlanID, AppliedCommitSHA: sha,
		PatchEffect: &types.PatchEffectRecord{PlanID: report.PlanID, RecordID: "dispatch-fixture", Source: "applied_commit", HeadRef: sha, DiffFingerprint: strings.Repeat("b", 64)}}
	for _, mode := range []string{"authorized", "ordinary", "revoked", "foreign_root", "restored_history"} {
		t.Run(mode, func(t *testing.T) {
			mu := currentBatchPromptFixture()
			if err := mu.AuthorizeNativeTestRegistration(root, delivery, []types.WriteBehaviorContract{{ID: "criterion-current"}}, []string{"packages/widget/widget.py"}); err != nil {
				t.Fatal(err)
			}
			mu.InstallNativeTestRegistrationIdentity(mu.NativeTestRegistrationAuthorization().ID, report)
			choices := mu.NativeTestRegistrationIdentityChoices(root)
			if len(choices) != 1 || choices[0].AssertionID != "python/unittest@packages/widget::test_increment" {
				t.Fatalf("real producer identity not offered: %+v", choices)
			}
			bus := &types.BusContext{Mutable: mu, RepoRoot: root, WorkDir: t.TempDir(), Mode: types.ModeApply}
			switch mode {
			case "ordinary":
				bus.Mutable = types.NewMutableState("ordinary source change")
			case "revoked":
				mu.RevokeNativeTestRegistrationAuthorization()
			case "foreign_root":
				bus.RepoRoot = t.TempDir()
			case "restored_history":
				body := b1122JSON(t, mu.WriteWorkflowRun())
				var restored types.WriteWorkflowRun
				if err := json.Unmarshal(body, &restored); err != nil {
					t.Fatal(err)
				}
				bus.Mutable = types.NewMutableState("restored history")
				bus.Mutable.SetWriteWorkflowRun(&restored)
				bus.Mutable.SetChangePlan(&types.ChangePlan{ID: report.PlanID})
				bus.Mutable.SetChangeReport(report)
			}
			before := b1122JSON(t, []any{report, bus.Mutable.ChangePlan(), bus.Mutable.WriteWorkflowRun()})
			registry := tool.NewRegistry()
			tool.RegisterDefaults(registry)
			capture := &traceTeachingCaptureLLM{stop: errors.New("captured actual native registration choices")}
			planner := NewPlannerAgent(&Dependencies{LLM: capture, Tools: registry, MaxIterations: 1})
			_, err := planner.Execute(ctxbuilder.BuildAgentContext(bus, types.AgentPlanner, types.StagePlan), traceTeachingSkill(t, "change-plan-skill"))
			if !errors.Is(err, capture.stop) || capture.calls != 1 {
				t.Fatalf("missing actual planner request: calls=%d err=%v", capture.calls, err)
			}
			found := 0
			for _, offered := range capture.tools {
				if offered.Name != "emit_change_plan" && offered.Name != "emit_plan_skeleton" {
					continue
				}
				found++
				if mode == "authorized" {
					if !bytes.Contains(offered.Parameters, []byte(choices[0].Ref)) || !bytes.Contains(offered.Parameters, []byte(`"anyOf"`)) {
						t.Fatalf("actual %s schema omitted current exact selector: %s", offered.Name, offered.Parameters)
					}
				} else {
					original, _ := registry.Get(offered.Name)
					canonical := append(json.RawMessage(nil), original.Parameters()...)
					expected := llm.CompactToolSchemas([]llm.ToolSchema{{Name: offered.Name, Parameters: canonical}})[0]
					if !bytes.Equal(offered.Parameters, expected.Parameters) {
						t.Fatalf("inactive %s wire differs from exact canonical transport projection", offered.Name)
					}
					expanded := nativeRegistrationExpandWireSchema(t, offered.Parameters)
					if !reflect.DeepEqual(expanded, nativeRegistrationDecodeSchema(t, canonical)) {
						t.Fatalf("inactive %s wire changed canonical constraints or identities", offered.Name)
					}
					expandedJSON := b1122JSON(t, expanded)
					for _, schema := range [][]byte{offered.Parameters, expandedJSON} {
						if bytes.Contains(schema, []byte(`"assertion_ref"`)) || bytes.Contains(schema, []byte(choices[0].Ref)) {
							t.Fatalf("inactive request acquired selection schema: %s", offered.Name)
						}
					}
					if !bytes.Equal(canonical, original.Parameters()) {
						t.Fatalf("wire projection mutated canonical %s schema", offered.Name)
					}
				}
			}
			if found != 2 {
				t.Fatalf("actual planner did not expose both plan entry points: %d", found)
			}
			var message strings.Builder
			for _, row := range capture.messages {
				message.WriteString(row.Content)
			}
			if strings.Contains(message.String(), choices[0].Ref) != (mode == "authorized") {
				t.Fatal("visible identity list and actual schema authorization disagree")
			}
			if !bytes.Equal(before, b1122JSON(t, []any{report, bus.Mutable.ChangePlan(), bus.Mutable.WriteWorkflowRun()})) {
				t.Fatal("schema projection changed source report, plan, or workflow")
			}
		})
	}
}

func nativeRegistrationDecodeSchema(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var schema map[string]any
	if err := dec.Decode(&schema); err != nil {
		t.Fatal(err)
	}
	return schema
}

// Independently expand only generated schema references. Instance-valued
// const/default/enum/example data is not a schema and must remain unchanged.
func nativeRegistrationExpandWireSchema(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	root := nativeRegistrationDecodeSchema(t, raw)
	defs, _ := root["$defs"].(map[string]any)
	delete(root, "$defs")
	active := map[string]bool{}
	var expand func(map[string]any) map[string]any
	expand = func(node map[string]any) map[string]any {
		if value, exists := node["$ref"]; exists {
			ref, ok := value.(string)
			if !ok || len(node) != 1 || !strings.HasPrefix(ref, "#/$defs/shared_schema_") {
				t.Fatalf("unexpected schema reference: %v", node)
			}
			definition, ok := defs[strings.TrimPrefix(ref, "#/$defs/")].(map[string]any)
			if !ok || active[ref] {
				t.Fatalf("dangling or cyclic schema reference: %s", ref)
			}
			active[ref] = true
			expanded := expand(definition)
			delete(active, ref)
			return expanded
		}
		for _, key := range []string{"properties", "patternProperties", "dependencies", "dependentSchemas"} {
			children, _ := node[key].(map[string]any)
			for name, child := range children {
				if schema, ok := child.(map[string]any); ok {
					children[name] = expand(schema)
				}
			}
		}
		for _, key := range []string{"items", "additionalItems", "contains", "contentSchema", "additionalProperties", "unevaluatedProperties", "unevaluatedItems", "propertyNames", "not", "if", "then", "else", "allOf", "anyOf", "oneOf", "prefixItems"} {
			switch child := node[key].(type) {
			case map[string]any:
				node[key] = expand(child)
			case []any:
				for i, item := range child {
					if schema, ok := item.(map[string]any); ok {
						child[i] = expand(schema)
					}
				}
			}
		}
		return node
	}
	return expand(root)
}
