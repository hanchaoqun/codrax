package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Build defaults before altering a profile: missing/null test inputs must reach
// Execute unchanged, not be restored by a fixture's default-field adapter.
func b1793TargetRosterPayload(t *testing.T, artifact bool) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(withV4Required(`{"intent":"explain","scenario":"generic","complexity":"moderate","question_kind":"mechanism","keywords":["runtime","target"],"entities":[]}`)), &payload); err != nil {
		t.Fatal(err)
	}
	scope, declaration := "not_applicable", "not_applicable"
	if artifact {
		scope, declaration = "full_artifact", "no_named_target"
	}
	payload["runtime_artifact_scope_profile"] = map[string]any{"requested_scope": scope, "confidence": 0.9}
	payload["runtime_target_profile"] = map[string]any{"declaration": declaration, "confidence": 0.9}
	payload["runtime_targets"] = []any{}
	return payload
}

func b1793ExecuteTargetRoster(t *testing.T, artifact bool, payload map[string]any) (types.ToolResult, *types.RequestModel) {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	before := string(data)
	ctx := &types.BusContext{Mutable: types.NewMutableState("explain worker-200")}
	if artifact {
		ctx.Mutable.SetPerfTrace(&types.PerfBundle{Meta: types.PerfMeta{Source: "attached.systrace", Signals: []string{"sched_switch"}}})
	}
	result, err := (&EmitAnalysis{}).Execute(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != before {
		t.Fatal("Execute changed the submitted JSON")
	}
	return result, ctx.Mutable.RequestModel()
}

func TestB1793RuntimeTargetRosterProfileValidationCompatibility(t *testing.T) {
	cases := []struct {
		name    string
		profile any
		omit    bool
		want    string
	}{
		{"missing", nil, true, "runtime_target_profile object missing"},
		{"null", nil, false, "runtime_target_profile object missing"},
		{"empty", map[string]any{}, false, "runtime_target_profile missing required field(s): declaration, confidence"},
		{"invalid_declaration", map[string]any{"declaration": "invented", "confidence": 0.9}, false, `runtime_target_profile.declaration "invented" is invalid`},
		{"missing_declaration", map[string]any{"confidence": 0.9}, false, "runtime_target_profile missing required field(s): declaration"},
		{"missing_confidence", map[string]any{"declaration": "no_named_target"}, false, "runtime_target_profile missing required field(s): confidence"},
		{"negative_confidence", map[string]any{"declaration": "no_named_target", "confidence": -0.1}, false, "runtime_target_profile.confidence -0.10 out of [0,1]"},
		{"excess_confidence", map[string]any{"declaration": "no_named_target", "confidence": 1.1}, false, "runtime_target_profile.confidence 1.10 out of [0,1]"},
		{"attached_not_applicable", map[string]any{"declaration": "not_applicable", "confidence": 0.9}, false, "not_applicable conflicts with the attached/referenced runtime request"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := b1793TargetRosterPayload(t, true)
			payload["runtime_target_profile"] = tc.profile
			if tc.omit {
				delete(payload, "runtime_target_profile")
			}
			result, rm := b1793ExecuteTargetRoster(t, true, payload)
			if result.Success || rm != nil || !strings.Contains(result.Summary, tc.want) {
				t.Fatalf("invalid profile must not acquire empty-roster authority: success=%t rm=%+v summary=%s", result.Success, rm, result.Summary)
			}
			// This repairs only the declaration. The same otherwise-valid public
			// payload must succeed without manufacturing any target identity.
			payload["runtime_target_profile"] = map[string]any{"declaration": "no_named_target", "confidence": 0.9}
			result, rm = b1793ExecuteTargetRoster(t, true, payload)
			if !result.Success || rm == nil || rm.RuntimeTargetProfile == nil || rm.RuntimeTargetProfile.Declaration != types.RuntimeTargetDeclarationNoNamedTarget || len(rm.RuntimeTargets) != 0 {
				t.Fatalf("profile-only repair should retain the empty roster: success=%t rm=%+v summary=%s", result.Success, rm, result.Summary)
			}
		})
	}
}

func TestB1793RuntimeTargetRosterOutsideRuntimeCompatibility(t *testing.T) {
	for _, shape := range []string{"missing", "null", "not_applicable", "unspecified", "no_named_target"} {
		t.Run(shape, func(t *testing.T) {
			payload := b1793TargetRosterPayload(t, false)
			switch shape {
			case "missing":
				delete(payload, "runtime_target_profile")
			case "null":
				payload["runtime_target_profile"] = nil
			default:
				payload["runtime_target_profile"] = map[string]any{"declaration": shape, "confidence": 0.9}
			}
			result, rm := b1793ExecuteTargetRoster(t, false, payload)
			if !result.Success || rm == nil || rm.RuntimeTargetProfile == nil || rm.RuntimeTargetProfile.Declaration != types.RuntimeTargetDeclarationNotApplicable || len(rm.RuntimeTargets) != 0 {
				t.Fatalf("non-runtime empty-roster compatibility changed: success=%t rm=%+v summary=%s", result.Success, rm, result.Summary)
			}
		})
	}
	t.Run("named_before_attachment", func(t *testing.T) {
		payload := b1793TargetRosterPayload(t, false)
		payload["runtime_target_profile"] = map[string]any{"declaration": "named_target", "source_quote": "worker-200", "confidence": 0.9}
		payload["runtime_targets"] = []any{map[string]any{"kind": "thread", "pid": 200, "source": "user_explicit", "confidence": 0.9}}
		result, rm := b1793ExecuteTargetRoster(t, false, payload)
		if !result.Success || rm == nil || rm.RuntimeTargetProfile == nil || !rm.RuntimeTargetProfile.NamedTarget() || len(rm.RuntimeTargets) != 1 || rm.RuntimeTargets[0].PID != 200 || rm.RuntimeTargets[0].Source != "user_explicit" {
			t.Fatalf("typed named identity must remain usable before attachment preflight: success=%t rm=%+v summary=%s", result.Success, rm, result.Summary)
		}
	})
}

func TestB1793RuntimeTargetRosterMalformedSiblingAndIndependentCensus(t *testing.T) {
	for _, independentErrors := range []bool{false, true} {
		name := "whole_roster"
		if independentErrors {
			name = "independent_census"
		}
		t.Run(name, func(t *testing.T) {
			payload := b1793TargetRosterPayload(t, true)
			payload["runtime_target_profile"] = map[string]any{"declaration": "named_target", "source_quote": "worker-200", "confidence": 0.9}
			payload["runtime_targets"] = []any{
				map[string]any{"kind": "thread", "pid": 200, "source": "user_explicit", "confidence": 0.9},
				map[string]any{"kind": "process", "source": "user_explicit", "confidence": 0.9},
			}
			if independentErrors {
				payload["runtime_artifact_scope_profile"] = map[string]any{}
				payload["runtime_question_profile"] = map[string]any{}
			}
			result, rm := b1793ExecuteTargetRoster(t, true, payload)
			rosterError := "runtime_targets[1] is structurally invalid: pid or thread is required"
			if result.Success || rm != nil || !strings.Contains(result.Summary, rosterError) {
				t.Fatalf("valid sibling must not survive a malformed roster: success=%t rm=%+v summary=%s", result.Success, rm, result.Summary)
			}
			if strings.Contains(result.Summary, "named_target requires at least one structurally valid runtime_targets entry") {
				t.Fatalf("malformed roster generated a dependent missing-target error: %s", result.Summary)
			}
			if independentErrors {
				last := -1
				for _, want := range []string{
					"runtime_artifact_scope_profile missing required field(s): requested_scope, confidence",
					rosterError,
					"runtime_question_profile missing required field(s): scope, runtime_work_relation_requested, frame_causality_requested, confidence",
				} {
					at := strings.Index(result.Summary, want)
					if at < 0 || at <= last {
						t.Fatalf("independent errors must retain schema order, missing/misordered %q: %s", want, result.Summary)
					}
					last = at
				}
			}
		})
	}
}
