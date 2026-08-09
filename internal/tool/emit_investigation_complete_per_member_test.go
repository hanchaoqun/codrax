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
// without a member_set handoff a typed downgrade naming both exits.
func TestEmitInvestigationComplete_PerMemberTableRequiresMemberSet(t *testing.T) {
	tool := &EmitInvestigationComplete{}
	bus := perMemberTableBus(true)
	params := json.RawMessage(`{
		"reason":"pipeline stages traced end to end",
		"confidence":"high",
		"result_kind":"resolved"
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("resolved completion without member_set should downgrade without tool failure: %s", res.Summary)
	}
	if bus.Mutable.IsInvestigationComplete() {
		t.Fatalf("downgrade must not mark investigation complete before convergence")
	}
	if !strings.Contains(res.Summary, "member_set") || !strings.Contains(res.Summary, "absence_justification") {
		t.Fatalf("downgrade must name both exits: %s", res.Summary)
	}
	repairs := bus.Mutable.EvidenceClosure().ActiveRepairs()
	if len(repairs) != 1 ||
		repairs[0].Kind != types.RepairStructuredHandoff ||
		repairs[0].DowngradeLane != types.DowngradeLanePrincipalMemberSetHandoff {
		t.Fatalf("expected typed principal member-set repair, got %+v", repairs)
	}
	if res.Repair == nil || res.Repair.Code != "principal_member_set_handoff" {
		t.Fatalf("expected structured tool repair metadata, got %+v", res.Repair)
	}
}

func TestEmitInvestigationComplete_PerMemberTableForceCompletesWithCaveatAfterNoProgress(t *testing.T) {
	SetDowngradeConvergenceHardThreshold(3)
	t.Cleanup(func() { SetDowngradeConvergenceHardThreshold(0) })
	tool := &EmitInvestigationComplete{}
	bus := perMemberTableBus(true)
	params := json.RawMessage(`{
		"reason":"pipeline stages traced end to end",
		"confidence":"high",
		"result_kind":"resolved"
	}`)
	for i := 1; i <= 2; i++ {
		res, err := tool.Execute(bus, params)
		if err != nil {
			t.Fatalf("attempt %d unexpected error: %v", i, err)
		}
		if !res.Success {
			t.Fatalf("attempt %d should downgrade without tool failure: %s", i, res.Summary)
		}
		if bus.Mutable.IsInvestigationComplete() {
			t.Fatalf("attempt %d should not complete before convergence threshold", i)
		}
	}
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("threshold attempt unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("threshold attempt should complete with caveat: %s", res.Summary)
	}
	if !bus.Mutable.IsInvestigationComplete() {
		t.Fatalf("threshold attempt should force-complete with typed caveat")
	}
	caveats := bus.Mutable.EvidenceClosure().CompletionCaveats()
	if len(caveats) != 1 || caveats[0].Lane != types.DowngradeLanePrincipalMemberSetHandoff {
		t.Fatalf("completion caveats = %+v, want principal_member_set_handoff", caveats)
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

func TestEmitInvestigationComplete_PerMemberTableCurrentSourceRowsRequireMemberAlignedEvidence(t *testing.T) {
	repo := t.TempDir()
	bus := perMemberTableBus(true)
	bus.RepoRoot = repo
	evidence := []types.EvidenceItem{
		{
			ID: "stage-analyze", Kind: types.EvidenceDirect, Scope: types.ScopeLine,
			AnchorKind: types.AnchorDefinition, AnchorSymbol: "StageAnalyze", Subject: "StageAnalyze",
			Source: "internal/types/stage_binding.go", LineStart: 10, GroundingStatus: types.GroundingGrounded,
		},
		{
			ID: "stage-extract", Kind: types.EvidenceDirect, Scope: types.ScopeLine,
			AnchorKind: types.AnchorDefinition, AnchorSymbol: "StageExtract", Subject: "StageExtract",
			Source: "internal/types/stage_binding.go", LineStart: 12, GroundingStatus: types.GroundingGrounded,
		},
	}
	bus.Mutable.AppendEvidence(evidence)
	tool := &EmitInvestigationComplete{}
	bad := json.RawMessage(`{
		"reason":"stage rows complete",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"member_set",
			"role":"principal_answer",
			"label":"pipeline stages",
			"members":["StageAnalyze","StagePlan"],
			"support_refs":["stage-analyze","stage-extract"]
		}]
	}`)
	res, err := tool.Execute(bus, bad)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success || !strings.Contains(res.Summary, "DOWNGRADED") || bus.Mutable.IsInvestigationComplete() {
		t.Fatalf("wrong member-to-evidence binding must remain in exploration: %+v complete=%t", res, bus.Mutable.IsInvestigationComplete())
	}
	for _, want := range []string{"StagePlan", "stage-extract", "same member"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("member-alignment downgrade missing %q: %s", want, res.Summary)
		}
	}

	bus.Mutable = types.NewMutableState("q")
	bus.Mutable.AppendEvidence(evidence)
	good := json.RawMessage(`{
		"reason":"stage rows complete",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"member_set",
			"role":"principal_answer",
			"label":"pipeline stages",
			"members":["StageAnalyze","StageExtract"],
			"support_refs":["stage-analyze","stage-extract"]
		}]
	}`)
	res, err = tool.Execute(bus, good)
	if err != nil {
		t.Fatalf("unexpected valid error: %v", err)
	}
	if !res.Success || strings.Contains(res.Summary, "DOWNGRADED") || !bus.Mutable.IsInvestigationComplete() {
		t.Fatalf("member-aligned current-source rows should complete: %+v complete=%t", res, bus.Mutable.IsInvestigationComplete())
	}
}

func TestAggregatePerMemberTableCurrentSourceSetRequiresAlignedSupportIsTypedAndScoped(t *testing.T) {
	fact := types.AnswerAggregateFact{
		Kind: types.AnswerAggregateMemberSet, Role: types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"StageAnalyze"},
	}
	evidence := []types.EvidenceItem{{
		ID: "stage-analyze", Kind: types.EvidenceDirect, Scope: types.ScopeLine,
		AnchorKind: types.AnchorDefinition, AnchorSymbol: "StageAnalyze", Subject: "StageAnalyze",
		Source: "internal/types/stage_binding.go", LineStart: 10, GroundingStatus: types.GroundingGrounded,
	}}
	bus := perMemberTableBus(true)
	bus.RepoRoot = t.TempDir()
	if !aggregatePerMemberTableCurrentSourceSetRequiresAlignedSupport(bus, fact, evidence) {
		t.Fatal("typed current-source per-member table should require aligned support")
	}
	bus.AnalysisIR.RequestModel.Predicates.HasPerMemberTable = false
	if aggregatePerMemberTableCurrentSourceSetRequiresAlignedSupport(bus, fact, evidence) {
		t.Fatal("undeclared per-member table must not activate the row gate")
	}
	bus.AnalysisIR.RequestModel.Predicates.HasPerMemberTable = true
	bus.RepoRoot = ""
	if aggregatePerMemberTableCurrentSourceSetRequiresAlignedSupport(bus, fact, evidence) {
		t.Fatal("a repository-less conceptual roster must not enter current-source row alignment")
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
