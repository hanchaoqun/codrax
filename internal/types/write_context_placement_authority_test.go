package types

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestB1591ContractAuthoritySurvivesBoundedContext(t *testing.T) {
	for _, relation := range []WriteRenderedTextRelation{WriteRenderedTextSameLineContains, WriteRenderedTextLineLocalNotContains} {
		for _, state := range []string{"planning_only", "observed", "observed_required", "hard_required", "soft_required"} {
			for _, longField := range []string{"id", "subject", "expected"} {
				t.Run(string(relation)+"/"+state+"/"+longField, func(t *testing.T) {
					c := WriteBehaviorContract{
						ID: "local", Kind: WriteBehaviorStdout, Polarity: WriteBehaviorPolarityExpected,
						Subject: "label", Operator: WriteBehaviorOpNotContains, Expected: "DEBUG", Required: true,
						Placement: &WriteRenderedTextPlacement{Surface: WriteRenderedTextSurfaceStdoutLine, Anchor: "label", Expected: "DEBUG", Relation: WriteRenderedTextLineLocalNotContains},
					}
					c.Placement.Relation = relation
					if relation == WriteRenderedTextSameLineContains {
						c.Operator = WriteBehaviorOpContains
					}
					want := "hard_required=true"
					priority := WriteContextP0
					switch state {
					case "planning_only":
						c.Required = false
						c.Source = WriteBehaviorContractSourcePlanningOnlyUngrounded
						want, priority = "planning_only=true", WriteContextP1
					case "observed", "observed_required":
						c.Required = state == "observed_required"
						c.Polarity = WriteBehaviorPolarityObserved
						want, priority = "polarity=observed", WriteContextP1
					case "soft_required":
						c.Placement = nil
						c.Operator = WriteBehaviorOpSatisfies
						want, priority = "soft_required=true", WriteContextP1
					}
					long := strings.Repeat("long-value-", 70)
					switch longField {
					case "id":
						c.ID = long
					case "subject":
						c.Subject = long
					case "expected":
						c.Expected = long
					}
					ir := &WriteAnalysisIR{Request: WriteRequestModel{BehaviorContracts: []WriteBehaviorContract{c}}}
					before, _ := json.Marshal(ir)
					view := WriteContextPackFromWriteAnalysisIR(ir).View(WriteConsumerPlanner, 10)
					if len(view.Items) != 1 || view.Items[0].Priority != priority {
						t.Fatalf("unexpected context authority: %+v", view)
					}
					text := view.Items[0].Text
					// Existing display policy keeps 240 runes and adds the three
					// truncation dots; this change must not expand that budget.
					if !strings.HasPrefix(text, want+" ") || len([]rune(text)) != 243 || !strings.HasSuffix(text, "...") {
						t.Fatalf("bounded context must show authority before long values and disclose truncation: %q", text)
					}
					if c.Placement != nil && !strings.Contains(text, "placement_relation="+string(relation)) {
						t.Fatalf("finite local relation must survive long free text: %q", text)
					}
					after, _ := json.Marshal(ir)
					if string(before) != string(after) {
						t.Fatal("display trimming changed the full typed contract")
					}
				})
			}
		}
	}
}
