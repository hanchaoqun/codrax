package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func perMemberTableBus(hasPerMemberTable bool) *types.BusContext {
	return &types.BusContext{
		Mutable: types.NewMutableState("q"),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Predicates: types.SemanticPredicates{HasPerMemberTable: hasPerMemberTable},
			},
		},
	}
}

// The declared per-member-table shape makes a resolved completion
// without a member_set handoff a typed reject naming both exits.
func TestEmitInvestigationComplete_PerMemberTableRequiresMemberSet(t *testing.T) {
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"pipeline stages traced end to end",
		"confidence":"high",
		"result_kind":"resolved"
	}`)
	res, err := tool.Execute(perMemberTableBus(true), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatalf("resolved completion without member_set must bounce")
	}
	if !strings.Contains(res.Summary, "member_set") || !strings.Contains(res.Summary, "absence_justification") {
		t.Fatalf("reject must name both exits: %s", res.Summary)
	}
}

// A member_set handoff satisfies the obligation.
func TestEmitInvestigationComplete_PerMemberTableMemberSetAccepted(t *testing.T) {
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"all four stages verified",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[
			{
				"kind":"member_set",
				"label":"pipeline stages",
				"members":["StageAnalyze","StageExplore","StageExtract","StageFinalize"]
			}
		]
	}`)
	res, err := tool.Execute(perMemberTableBus(true), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("member_set completion should be accepted: %s", res.Summary)
	}
}

// absence_justification is the typed escape lane.
func TestEmitInvestigationComplete_PerMemberTableAbsenceEscape(t *testing.T) {
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"the requested member set does not exist in this repository",
		"confidence":"high",
		"result_kind":"absence",
		"absence_justification":"no matching members exist under the asked range"
	}`)
	res, err := tool.Execute(perMemberTableBus(true), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("absence escape must pass the gate: %s", res.Summary)
	}
}

// Without the declared shape the gate is inert — existing behavior
// stays byte-identical.
func TestEmitInvestigationComplete_PerMemberTableInertWhenUndeclared(t *testing.T) {
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"mechanism explained",
		"confidence":"high",
		"result_kind":"resolved"
	}`)
	res, err := tool.Execute(perMemberTableBus(false), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("undeclared shape must not gate: %s", res.Summary)
	}
}
