package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

const targetRosterEmptyInstruction = "For no_named_target or unspecified, emit runtime_targets=[] or omit the field; do not emit placeholder objects."

func TestRuntimeTargetRosterTeachingEmptyDeclarationRepairConverges(t *testing.T) {
	for _, declaration := range []string{"no_named_target", "unspecified"} {
		for _, repairedShape := range []string{"empty", "omitted", "null"} {
			t.Run(declaration+"/"+repairedShape, func(t *testing.T) {
				raw, payload := mixedRuntimePayload(t, "causal_attribution", true, true)
				payload["runtime_target_profile"] = map[string]any{"declaration": declaration, "confidence": 0.95}
				payload["runtime_targets"] = []any{map[string]any{"kind": "thread", "source": "user_explicit", "confidence": 0.95}}
				before, _ := json.Marshal(payload)
				ok, summary, rm := executeMixedRuntime(t, raw, payload)
				if ok || rm != nil || !strings.Contains(summary, "runtime_targets[0] is structurally invalid") {
					t.Fatalf("empty identity must fail without publishing authority: ok=%t rm=%+v %s", ok, rm, summary)
				}
				if !strings.Contains(summary, targetRosterEmptyInstruction) || strings.Contains(summary, "correct the typed target identity instead of omitting it") {
					t.Errorf("recovery instruction contradicts the declared absence of a user target: %s", summary)
				}
				after, _ := json.Marshal(payload)
				if string(before) != string(after) {
					t.Fatal("rejection must not rewrite the model payload")
				}
				switch repairedShape {
				case "empty":
					payload["runtime_targets"] = []any{}
				case "omitted":
					delete(payload, "runtime_targets")
				case "null":
					payload["runtime_targets"] = nil
				}
				ok, summary, rm = executeMixedRuntime(t, raw, payload)
				if !ok || rm == nil || len(rm.RuntimeTargets) != 0 || string(rm.RuntimeTargetProfile.Declaration) != declaration {
					t.Fatalf("model-owned empty roster repair did not converge: ok=%t rm=%+v %s", ok, rm, summary)
				}
				if !rm.RuntimeArtifactScopeProfile.HasExplicitTimeWindows() || !rm.RuntimeQuestionProfile.FrameCausalityRequested || !rm.RuntimeQuestionProfile.RuntimeWorkRelationRequested || len(rm.RequestedAnswerDimensions.Dimensions) != 3 {
					t.Fatal("identity-only repair lost independent time, frame, work or causal requirements")
				}
				if decided, allowed := types.RuntimeTraceReportShapeAuthority(rm); !decided || !allowed {
					t.Fatal("empty user-target roster must not remove independently declared causal projection authority")
				}
			})
		}
	}
}

func TestRuntimeTargetRosterTeachingNamedIdentityStillRequired(t *testing.T) {
	for _, kind := range []string{"process", "thread"} {
		t.Run(kind, func(t *testing.T) {
			raw, payload := mixedRuntimePayload(t, "causal_attribution", false, true)
			raw += "; worker-200"
			payload["runtime_target_profile"] = map[string]any{"declaration": "named_target", "source_quote": "worker-200", "confidence": 0.95}
			row := map[string]any{"kind": kind, "source": "user_explicit", "confidence": 0.95}
			payload["runtime_targets"] = []any{row}
			ok, summary, _ := executeMixedRuntime(t, raw, payload)
			if ok || !strings.Contains(summary, "For named_target,") || !strings.Contains(summary, "source=user_explicit") {
				t.Errorf("named malformed identity must get provenance-preserving recovery: ok=%t %s", ok, summary)
			}
			payload["runtime_targets"] = []any{}
			ok, summary, rm := executeMixedRuntime(t, raw, payload)
			if ok || rm != nil || !strings.Contains(summary, "requires at least one structurally valid") {
				t.Fatalf("new teaching must not permit omission of an explicitly named identity: ok=%t %s", ok, summary)
			}
			row["pid"] = 200
			payload["runtime_targets"] = []any{row}
			ok, summary, rm = executeMixedRuntime(t, raw, payload)
			if !ok || rm == nil || len(rm.RuntimeTargets) != 1 || rm.RuntimeTargets[0].PID != 200 || !rm.RuntimeTargetProfile.NamedTarget() {
				t.Fatalf("model-owned complete named repair should converge: ok=%t rm=%+v %s", ok, rm, summary)
			}
			for _, source := range []string{"artifact_metadata", "tool_handoff"} {
				row["source"] = source
				ok, summary, rm = executeMixedRuntime(t, raw, payload)
				if ok || rm != nil || !strings.Contains(summary, "source must be user_explicit") {
					t.Fatalf("%s cannot become named user authority: ok=%t %s", source, ok, summary)
				}
			}
			row["source"] = "user_explicit"
			payload["runtime_target_profile"].(map[string]any)["source_quote"] = "artifact-only-thread-300"
			ok, summary, rm = executeMixedRuntime(t, raw, payload)
			if ok || rm != nil || !strings.Contains(summary, "copied verbatim from the current request") {
				t.Fatalf("recovery must still reject a fabricated quote: ok=%t %s", ok, summary)
			}
		})
	}
}

func TestRuntimeTargetRosterTeachingUsesOneRuleAcrossPromptSchemaAndReject(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal((&EmitAnalysis{}).Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	props := schema["properties"].(map[string]any)
	before, _ := json.Marshal(schema)
	faces := map[string]string{"analysis": skill.BuildAnalysisSkill().OutputFormat}
	for _, key := range []string{"runtime_target_profile", "runtime_targets"} {
		faces[key] = props[key].(map[string]any)["description"].(string)
	}
	_, _, faces["rejection"] = parseRuntimeTargets([]emitRuntimeTargetParam{{Kind: "thread", Confidence: testFloatPtr(0.9)}})
	for name, face := range faces {
		t.Run(name, func(t *testing.T) {
			if strings.Count(face, targetRosterEmptyInstruction) != 1 {
				t.Errorf("expected one explicit empty-roster rule at %s: %s", name, face)
			}
			if strings.Count(face, skill.AnalysisRuntimeTargetRosterTeaching) != 1 {
				t.Errorf("%s does not reuse the complete single-source target roster teaching", name)
			}
		})
	}
	required, _ := json.Marshal(props["runtime_targets"].(map[string]any)["items"].(map[string]any)["required"])
	if string(required) != `["kind","confidence"]` {
		t.Fatal("teaching must not expand the schema gate")
	}
	if err := json.Unmarshal((&EmitAnalysis{}).Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(schema)
	if string(before) != string(after) {
		t.Fatal(fmt.Sprintf("schema changed during diagnostic generation: %d vs %d", len(before), len(after)))
	}
}
