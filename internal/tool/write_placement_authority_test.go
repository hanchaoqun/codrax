package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func b1591PlacementContract(id string) types.WriteBehaviorContract {
	return types.WriteBehaviorContract{
		ID: id, Kind: types.WriteBehaviorStdout, Polarity: types.WriteBehaviorPolarityExpected,
		Operator: types.WriteBehaviorOpNotContains, Expected: "DEBUG", Required: true,
		Placement: &types.WriteRenderedTextPlacement{
			Surface: types.WriteRenderedTextSurfaceStdoutLine, Anchor: "label",
			Expected: "DEBUG", Relation: types.WriteRenderedTextLineLocalNotContains,
			EvidenceRef: "tests/display.py:4",
		}, Source: "write_analyzer",
	}
}

func TestB1591PlacementRefsKeepRequiredGateAndTruthfulRepair(t *testing.T) {
	for _, state := range []string{"required", "planning_only", "observed", "not_required", "without_placement", "unknown", "retired"} {
		t.Run(state, func(t *testing.T) {
			contract := b1591PlacementContract("local")
			ref := contract.ID
			want := ""
			switch state {
			case "planning_only":
				contract.Required = false
				contract.Source += ";" + types.WriteBehaviorContractSourcePlanningOnlyUngrounded
				want = "planning-only"
			case "observed":
				contract.Required = false
				contract.Polarity = types.WriteBehaviorPolarityObserved
				want = "observed"
			case "not_required":
				contract.Required = false
				want = "not a required placement"
			case "without_placement":
				contract.Placement = nil
				want = "without placement{}"
			case "unknown":
				ref = "unknown-id"
				want = "unknown behavior_contract"
			case "retired":
				want = "was superseded by the repair plan"
			}
			plan := &types.ChangePlan{BehaviorContracts: []types.WriteBehaviorContract{contract}, VerificationProbes: []types.VerificationProbe{{ID: "p", PlacementRefs: []string{ref}}}}
			wantReason := verificationProbeContractRefsFailedReason
			if state == "retired" {
				plan.SupersededBehaviorContracts = []types.WriteBehaviorContractTombstone{{ID: contract.ID, Reason: types.WriteBehaviorContractRetiredPlannerSupersession}}
				wantReason = verificationProbeContractRefRetiredReason
			}
			rejection, reason, _ := validatePlanBehaviorContractRefs(plan)
			if state == "required" {
				if rejection != "" || reason != "" {
					t.Fatalf("complete grounded required placement lost its existing admission: %s/%s", rejection, reason)
				}
				return
			}
			if reason != wantReason || !strings.Contains(rejection, want) {
				t.Fatalf("rejection must distinguish authority from a missing local carrier: got %s/%s, want %q", rejection, reason, want)
			}
			if contract.Placement != nil && state != "unknown" && strings.Contains(rejection, "without placement{}") {
				t.Fatalf("present placement was misreported as absent: %s", rejection)
			}
		})
	}
}

func TestB1591PlanningOnlyPlacementRealPlanEmission(t *testing.T) {
	contract := b1591PlacementContract("local")
	contract.Required = false
	contract.Source += ";" + types.WriteBehaviorContractSourcePlanningOnlyUngrounded
	bus := &types.BusContext{RepoRoot: t.TempDir(), Mode: types.ModeApply, Mutable: types.NewMutableState("update output.txt")}
	bus.Mutable.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{
		Task: types.WriteTask{Kind: types.WriteTaskBugfix, Scope: types.ScopePackage}, BehaviorContracts: []types.WriteBehaviorContract{contract},
	}})
	result, err := (&EmitChangePlan{}).Execute(bus, json.RawMessage(`{
		"request":"update output.txt", "summary":"update output.txt",
		"changes":[{"path":"output.txt","kind":"create","new_content":"ready\n"}],
		"verification_probes":[{"id":"p","language":"python","code":"assert True","placement_refs":["local"]}]
	}`))
	if err != nil || result.Success || !strings.Contains(result.Summary, "planning-only") || strings.Contains(result.Summary, "without placement{}") {
		t.Fatalf("real emission must refuse unrequested placement authority without denying its carrier: err=%v result=%+v", err, result)
	}
	if bus.Mutable.ChangePlan() != nil {
		t.Fatal("rejected planning-only placement proof installed a plan")
	}
	// The same real emitter accepts the plan without promoting the optional
	// local context into a required placement proof. Nothing is executed here.
	result, err = (&EmitChangePlan{}).Execute(bus, json.RawMessage(`{
		"request":"update output.txt", "summary":"update output.txt",
		"changes":[{"path":"output.txt","kind":"create","new_content":"ready\n"}]
	}`))
	if err != nil || !result.Success {
		t.Fatalf("planning context must not require a placement proof to emit: err=%v result=%+v", err, result)
	}
	plan := bus.Mutable.ChangePlan()
	if plan == nil || len(plan.BehaviorContracts) != 1 || !reflect.DeepEqual(plan.BehaviorContracts[0], contract) {
		t.Fatalf("accepted plan lost the intact local planning carrier: %+v", plan)
	}
	if len(types.PlacementRequiredWriteBehaviorContractIDs(plan.BehaviorContracts)) != 0 {
		t.Fatal("accepted plan promoted local planning context into a placement obligation")
	}
}

func TestB1591PublishedPlacementRefsSchemaMatchesRequiredDomain(t *testing.T) {
	for _, schema := range []struct {
		name string
		raw  json.RawMessage
		want int
	}{{"emit_change_plan", (&EmitChangePlan{}).Parameters(), 2}, {"run_tests", (&RunTests{}).Parameters(), 1}} {
		t.Run(schema.name, func(t *testing.T) {
			var decoded any
			if err := json.Unmarshal(schema.raw, &decoded); err != nil {
				t.Fatal(err)
			}
			count := 0
			var walk func(any)
			walk = func(value any) {
				switch v := value.(type) {
				case map[string]any:
					for key, child := range v {
						if key == "placement_refs" {
							count++
							description, _ := child.(map[string]any)["description"].(string)
							if description != types.WritePlacementRefsTeaching {
								t.Errorf("published field teaching diverged from the shared authority definition: %s", description)
							}
							for _, word := range []string{"active hard-required", "planning-only", "observed", "line-local"} {
								if !strings.Contains(description, word) {
									t.Errorf("schema placement_refs omits %q authority boundary: %s", word, description)
								}
							}
						}
						walk(child)
					}
				case []any:
					for _, child := range v {
						walk(child)
					}
				}
			}
			walk(decoded)
			if count != schema.want {
				t.Fatalf("placement_refs fields changed: got %d want %d", count, schema.want)
			}
		})
	}
}

func TestB1591WriteAnalysisPlacementSchemaPreservesNegativeLocalMeaning(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal((&EmitWriteAnalysis{}).Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	properties := schema["properties"].(map[string]any)
	contracts := properties["behavior_contracts"].(map[string]any)
	items := contracts["items"].(map[string]any)["properties"].(map[string]any)
	placement := items["placement"].(map[string]any)
	placementProperties := placement["properties"].(map[string]any)
	expectedDescription := placementProperties["expected"].(map[string]any)["description"].(string)
	for _, want := range []string{"presence or absence", "line_local_not_contains", "not global exclusion"} {
		if !strings.Contains(expectedDescription, want) {
			t.Fatalf("placement.expected teaching lost %q: %s", want, expectedDescription)
		}
	}
	for _, description := range []string{placement["description"].(string), contracts["description"].(string)} {
		if !strings.Contains(description, types.WritePlacementRefsTeaching) {
			t.Fatalf("analyzer schema disagrees with proof tool authority: %s", description)
		}
	}
}
