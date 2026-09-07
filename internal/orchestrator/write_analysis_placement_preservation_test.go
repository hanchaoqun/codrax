package orchestrator

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// These pins start at the real emission tool and pass through the actual
// write-analyze quality hook. A partial/unproven local relation remains local
// planning information; dropping placement or changing the operator cannot
// make that proposed relation more accurate.
func TestWriteAnalyzeInvalidPlacementPreservesLocalMeaning(t *testing.T) {
	for _, operator := range []types.WriteBehaviorOperator{types.WriteBehaviorOpContains, types.WriteBehaviorOpNotContains} {
		for _, condition := range []string{"ungrounded_anchor", "missing_anchor", "missing_delimiter", "missing_anchor_with_evidence", "observed", "planning_only", "forbidden", "grounded", "evidence_grounded"} {
			t.Run(string(operator)+"/"+condition, func(t *testing.T) {
				raw := "Adjust the selected label's DEBUG text without changing other labels."
				placement := &types.WriteRenderedTextPlacement{
					Surface: types.WriteRenderedTextSurfaceStdoutLine, Anchor: "private-label",
					Expected: "DEBUG", Relation: types.WriteRenderedTextSameLineContains,
				}
				if operator == types.WriteBehaviorOpNotContains {
					placement.Relation = types.WriteRenderedTextLineLocalNotContains
				}
				contract := types.WriteBehaviorContract{
					ID: "local-label", Kind: types.WriteBehaviorStdout, Subject: "selected label",
					Polarity: types.WriteBehaviorPolarityExpected, Operator: operator,
					Expected: "DEBUG", Placement: placement, Required: true, Source: "write_analyzer",
				}
				switch condition {
				case "missing_anchor_with_evidence":
					placement.Anchor = ""
					placement.EvidenceRef = "tests/display.py:4"
				case "missing_anchor":
					placement.Anchor = ""
				case "missing_delimiter":
					placement.Anchor = "selected label"
					placement.Relation = types.WriteRenderedTextBetweenAnchorAndDelimiter
				case "observed":
					contract.Polarity = types.WriteBehaviorPolarityObserved
				case "forbidden":
					contract.Polarity = types.WriteBehaviorPolarityForbidden
				case "planning_only":
					contract.Source += ";" + types.WriteBehaviorContractSourcePlanningOnlyUngrounded
				case "grounded":
					placement.Anchor = "selected label"
				case "evidence_grounded":
					placement.EvidenceRef = "tests/display.py:4"
				}
				params, err := json.Marshal(map[string]any{
					"task":               map[string]any{"kind": "bugfix", "scope": "micro", "summary": "adjust label rendering"},
					"risk":               map[string]any{"overall": "low"},
					"behavior_contracts": []types.WriteBehaviorContract{contract},
				})
				if err != nil {
					t.Fatal(err)
				}
				var emitted types.WriteBehaviorContract
				var original []byte
				dispatches := 0
				fns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
					types.AgentWriteAnalyzer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
						dispatches++
						res, err := (&tool.EmitWriteAnalysis{}).Execute(&types.BusContext{Mode: types.ModeApply, Mutable: ctx.Mutable}, params)
						if err != nil || !res.Success {
							t.Fatalf("known-enum partial/local carrier should emit successfully: err=%v result=%+v", err, res)
						}
						emitted = ctx.Mutable.WriteAnalysisIR().Request.BehaviorContracts[0]
						original, _ = json.Marshal(emitted)
						return &agent.StageOutput{StageReport: "local rendering proposal"}, nil
					},
				}
				ar, sr, sar := buildRegistries(fns)
				o := New(types.PipelineSettings{}, ar, sr, sar)
				o.busCtx = &types.BusContext{Mode: types.ModeApply, Mutable: types.NewMutableState(raw), AnalysisIR: dagIR(types.AnswerContract{Language: "en"})}
				used, err := o.runWriteAnalyzePhase()
				if err != nil || used != 1 || dispatches != 1 {
					t.Fatalf("authority-only calibration should keep one valid IR: used=%d dispatches=%d err=%v", used, dispatches, err)
				}
				got := o.busCtx.Mutable.WriteAnalysisIR().Request.BehaviorContracts[0]
				t.Logf("emitted=%+v placement=%+v calibrated=%+v placement=%+v", emitted, emitted.Placement, got, got.Placement)
				if got.Operator != emitted.Operator || got.Polarity != emitted.Polarity ||
					got.Expected != emitted.Expected || !reflect.DeepEqual(got.Placement, emitted.Placement) {
					t.Fatalf("quality repair changed local meaning: operator %s->%s, placement %+v->%+v", emitted.Operator, got.Operator, emitted.Placement, got.Placement)
				}
				if condition == "grounded" || condition == "evidence_grounded" {
					if !reflect.DeepEqual(got, emitted) || !types.IsPlacementRequiredWriteBehaviorContract(got) {
						t.Fatalf("grounded local requirement was weakened: %+v", got)
					}
				} else if condition == "observed" {
					if !reflect.DeepEqual(got, emitted) {
						t.Fatalf("observed facts must not be rewritten or stamped supersedable planning proposals: %+v", got)
					}
				} else if got.Required || !types.IsPlanningOnlyWriteBehaviorContract(got) {
					t.Fatalf("unproven/partial placement retained verification authority: %+v", got)
				}
				currentOriginal, _ := json.Marshal(emitted)
				if string(currentOriginal) != string(original) {
					t.Fatal("quality calibration mutated the emitted source contract")
				}
				stored, _ := json.Marshal(o.busCtx.Mutable.WriteAnalysisIR())
				var loaded types.WriteAnalysisIR
				if err := json.Unmarshal(stored, &loaded); err != nil {
					t.Fatal(err)
				}
				loaded.Request.BehaviorContracts = types.NormalizeWriteBehaviorContracts(loaded.Request.BehaviorContracts, nil)
				if !reflect.DeepEqual(loaded.Request.BehaviorContracts[0], got) {
					t.Fatal("serialized/normalized local contract changed meaning or authority")
				}
				again, repairs := repairWriteAnalysisIRQuality(&loaded)
				if len(repairs) != 0 || !reflect.DeepEqual(again, &loaded) {
					t.Fatalf("calibration must be idempotent after persistence: %+v", repairs)
				}
			})
		}
	}
}

// The proposed repair uses an existing persisted authority marker; this test
// separately verifies its current consumers rather than assuming that a
// retained Placement pointer is necessarily a required verifier obligation.
func TestPlanningOnlyPlacementRetainsContextWithoutProofAuthority(t *testing.T) {
	contract := types.WriteBehaviorContract{
		ID: "local-label", Kind: types.WriteBehaviorStdout, Polarity: types.WriteBehaviorPolarityExpected,
		Subject: "row", Operator: types.WriteBehaviorOpNotContains, Expected: "DEBUG",
		Placement: &types.WriteRenderedTextPlacement{
			Surface: types.WriteRenderedTextSurfaceStdoutLine, Anchor: "private-label",
			Expected: "DEBUG", Relation: types.WriteRenderedTextLineLocalNotContains,
		},
		Source: "write_analyzer;" + types.WriteBehaviorContractSourcePlanningOnlyUngrounded,
	}
	ir := &types.WriteAnalysisIR{Request: types.WriteRequestModel{RawRequest: "Adjust DEBUG on the selected label", BehaviorContracts: []types.WriteBehaviorContract{contract}}}
	data, err := json.Marshal(ir)
	if err != nil {
		t.Fatal(err)
	}
	var loaded types.WriteAnalysisIR
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	loaded.Request.BehaviorContracts = types.NormalizeWriteBehaviorContracts(loaded.Request.BehaviorContracts, nil)
	contracts := loaded.Request.BehaviorContracts
	if len(contracts) != 1 || !reflect.DeepEqual(contracts[0], contract) {
		t.Fatalf("persistence/normalization lost local planning information: %+v", contracts)
	}
	if len(types.RequiredWriteBehaviorContractIDs(contracts, true)) != 0 ||
		len(types.HardRequiredWriteBehaviorContractIDs(contracts)) != 0 ||
		len(types.PlacementRequiredWriteBehaviorContractIDs(contracts)) != 0 {
		t.Fatalf("retained local proposal created a proof obligation: %+v", contracts)
	}
	pack := types.WriteContextPackFromWriteAnalysisIR(&loaded)
	view := pack.View(types.WriteConsumerPlanner, 10)
	if len(view.Items) != 1 || view.Items[0].Priority != types.WriteContextP1 {
		t.Fatalf("local proposal should be planner guidance: %+v", view)
	}
	for _, text := range []string{"operator=not_contains", "placement_anchor=private-label", "planning_only=true"} {
		if !strings.Contains(view.Items[0].Text, text) {
			t.Errorf("planner guidance lost %q: %+v", text, view)
		}
	}
	if !strings.Contains(view.Items[0].Text, "placement_relation=line_local_not_contains") && !strings.HasSuffix(view.Items[0].Text, "...") {
		t.Fatalf("bounded context may trim the local relation only with explicit truncation: %+v", view)
	}
	if verifier := pack.View(types.WriteConsumerVerifier, 10); len(verifier.Items) != 0 {
		t.Fatalf("local proposal leaked into verifier context: %+v", verifier)
	}
	// preserving the placement must not create a repair-then-reject loop.
	if rejection := writeAnalysisIRQualityRejection(&loaded); rejection != "" {
		t.Fatalf("planning-only local proposal was re-promoted into a hard quality rejection: %s", rejection)
	}
}
