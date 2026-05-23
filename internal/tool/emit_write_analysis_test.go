package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func newTestBusForWriteAnalysis() *types.BusContext {
	return &types.BusContext{
		Mutable: types.NewMutableState("test objective"),
		Mode:    types.ModePlan,
	}
}

// TestEmitWriteAnalysis_HappyPath verifies a fully-populated emit
// stores a normalised IR with all enums canonicalised.
func TestEmitWriteAnalysis_HappyPath(t *testing.T) {
	tool := &EmitWriteAnalysis{}
	bus := newTestBusForWriteAnalysis()
	params := json.RawMessage(`{
		"raw_request": "add a --quiet flag to suppress non-error output",
		"task": {
			"kind": "feature",
			"scope": "micro",
			"summary": "wire a --quiet flag through cmd/root.go"
		},
		"risk": {
			"affects_public_api": true,
			"changes_persistence": false,
			"changes_build_system": false,
			"overall": "low"
		},
		"scope_anchors": ["cmd/root.go", "internal/render"],
		"expected_outcomes": [
			"the --quiet flag suppresses INFO-level log output",
			"existing tests in cmd/ still pass"
		]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("Execute success=false: %s", res.Summary)
	}
	ir := bus.Mutable.WriteAnalysisIR()
	if ir == nil {
		t.Fatal("WriteAnalysisIR not stored")
	}
	if ir.Request.Task.Kind != types.WriteTaskFeature {
		t.Errorf("kind = %q; want feature", ir.Request.Task.Kind)
	}
	if ir.Request.Task.Scope != types.ScopeMicro {
		t.Errorf("scope = %q; want micro", ir.Request.Task.Scope)
	}
	if ir.Request.Risk.Overall != types.RiskBandLow {
		t.Errorf("overall = %q; want low", ir.Request.Risk.Overall)
	}
	if !ir.Request.Risk.AffectsPublicAPI {
		t.Error("AffectsPublicAPI lost in round-trip")
	}
	if len(ir.Request.ScopeAnchors) != 2 {
		t.Errorf("ScopeAnchors len = %d; want 2", len(ir.Request.ScopeAnchors))
	}
	if len(ir.Request.ExpectedOutcomes) != 2 {
		t.Errorf("ExpectedOutcomes len = %d; want 2", len(ir.Request.ExpectedOutcomes))
	}
}

func TestEmitWriteAnalysis_StructuredPayloadCompatRepairsStringArrays(t *testing.T) {
	tool := &EmitWriteAnalysis{}
	bus := newTestBusForWriteAnalysis()
	params := json.RawMessage(`{
		"raw_request": "add a --quiet flag",
		"task": {
			"kind": "feature",
			"scope": "micro",
			"summary": "wire a --quiet flag through cmd/root.go"
		},
		"risk": {
			"affects_public_api": true,
			"changes_persistence": false,
			"changes_build_system": false,
			"overall": "low"
		},
		"scope_anchors": "[\"cmd/root.go\", \"internal/render\"]",
		"expected_outcomes": "[\"quiet flag suppresses info output\", \"existing tests pass\"]",
		"phase_proposal": {
			"split": "sequential",
			"phases": "[{\"goal\":\"wire flag\",\"rough_target_paths\":\"[\\\"cmd/root.go\\\"]\"},{\"goal\":\"update render tests\",\"rough_target_paths\":\"[\\\"internal/render\\\"]\"}]"
		}
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected schema-compatible payload to be accepted, got: %s", res.Summary)
	}
	ir := bus.Mutable.WriteAnalysisIR()
	if ir == nil {
		t.Fatal("WriteAnalysisIR not stored")
	}
	if len(ir.Request.ScopeAnchors) != 2 || ir.Request.ScopeAnchors[0] != "cmd/root.go" {
		t.Fatalf("scope anchors were not repaired: %+v", ir.Request.ScopeAnchors)
	}
	if len(ir.Request.ExpectedOutcomes) != 2 {
		t.Fatalf("expected outcomes were not repaired: %+v", ir.Request.ExpectedOutcomes)
	}
	if ir.PhaseProposal.Split != "sequential" || len(ir.PhaseProposal.Phases) != 2 ||
		len(ir.PhaseProposal.Phases[0].RoughTargetPaths) != 1 {
		t.Fatalf("nested phase proposal arrays were not repaired: %+v", ir.PhaseProposal)
	}
}

// TestEmitWriteAnalysis_EnumFallback verifies unknown enum values
// fall back to the safest option rather than rejecting the emit.
func TestEmitWriteAnalysis_EnumFallback(t *testing.T) {
	tool := &EmitWriteAnalysis{}
	bus := newTestBusForWriteAnalysis()
	params := json.RawMessage(`{
		"raw_request": "do something",
		"task": {"kind": "creative_invention", "scope": "global", "summary": "do something"},
		"risk": {"affects_public_api": false, "changes_persistence": false, "changes_build_system": false, "overall": "catastrophic"}
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil || !res.Success {
		t.Fatalf("Execute failed: err=%v summary=%s", err, res.Summary)
	}
	ir := bus.Mutable.WriteAnalysisIR()
	if ir.Request.Task.Kind != types.WriteTaskMisc {
		t.Errorf("unknown kind should fall back to misc; got %q", ir.Request.Task.Kind)
	}
	if ir.Request.Task.Scope != types.ScopeMicro {
		t.Errorf("unknown scope should fall back to micro; got %q", ir.Request.Task.Scope)
	}
	if ir.Request.Risk.Overall != types.RiskBandLow {
		t.Errorf("unknown overall should fall back to low; got %q", ir.Request.Risk.Overall)
	}
}

// TestEmitWriteAnalysis_RejectsEmptyRequiredFields verifies the
// minimum-information guard.
func TestEmitWriteAnalysis_RejectsEmptyRequiredFields(t *testing.T) {
	tool := &EmitWriteAnalysis{}
	bus := newTestBusForWriteAnalysis()
	cases := []struct {
		name   string
		params string
		want   string
	}{
		{
			name: "empty raw_request",
			params: `{"raw_request": "", "task": {"kind": "feature", "scope": "micro", "summary": "x"},
				"risk": {"affects_public_api": false, "changes_persistence": false, "changes_build_system": false, "overall": "low"}}`,
			want: "raw_request is empty",
		},
		{
			name: "empty task.summary",
			params: `{"raw_request": "x", "task": {"kind": "feature", "scope": "micro", "summary": ""},
				"risk": {"affects_public_api": false, "changes_persistence": false, "changes_build_system": false, "overall": "low"}}`,
			want: "task.summary is empty",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := tool.Execute(bus, json.RawMessage(c.params))
			if err != nil {
				t.Fatalf("Execute returned err: %v", err)
			}
			if res.Success {
				t.Fatalf("expected reject; got success summary=%s", res.Summary)
			}
			if !strings.Contains(res.Summary, c.want) {
				t.Errorf("rejection summary = %q; want substring %q", res.Summary, c.want)
			}
		})
	}
}

// TestEmitWriteAnalysis_PhaseProposalOverCapTruncates pins the
// stage-II MaxPhasesPerGroup cap (commit 18): an emit with more
// than 5 phases is truncated to 5 rather than rejected, so a
// near-cap LLM emit doesn't fail the entire dispatch. WARN is
// logged at truncation time.
func TestEmitWriteAnalysis_PhaseProposalOverCapTruncates(t *testing.T) {
	tool := &EmitWriteAnalysis{}
	bus := newTestBusForWriteAnalysis()
	// Build a sequential proposal with 8 phases (exceeds the cap of 5).
	params := json.RawMessage(`{
		"raw_request": "do many things",
		"task": {"kind": "feature", "scope": "project", "summary": "many phases"},
		"risk": {"affects_public_api": false, "changes_persistence": false, "changes_build_system": false, "overall": "low"},
		"phase_proposal": {
			"split": "sequential",
			"phases": [
				{"goal": "phase1"}, {"goal": "phase2"}, {"goal": "phase3"},
				{"goal": "phase4"}, {"goal": "phase5"}, {"goal": "phase6"},
				{"goal": "phase7"}, {"goal": "phase8"}
			]
		}
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil || !res.Success {
		t.Fatalf("Execute failed: err=%v summary=%s", err, res.Summary)
	}
	ir := bus.Mutable.WriteAnalysisIR()
	if ir.PhaseProposal.Split != "sequential" {
		t.Errorf("Split should remain sequential after cap; got %q", ir.PhaseProposal.Split)
	}
	if len(ir.PhaseProposal.Phases) != types.MaxPhasesPerGroup {
		t.Errorf("Expected truncation to %d phases; got %d", types.MaxPhasesPerGroup, len(ir.PhaseProposal.Phases))
	}
	// First N phases should survive verbatim (truncation drops the tail).
	if ir.PhaseProposal.Phases[0].Goal != "phase1" {
		t.Errorf("First phase goal drift; got %q", ir.PhaseProposal.Phases[0].Goal)
	}
}

// TestEmitWriteAnalysis_PhaseProposalCollapse verifies a sequential
// proposal with fewer than 2 phases collapses to single — the
// downstream scheduler treats single as the no-op (zero-regression)
// path so the contradiction has to be resolved here.
func TestEmitWriteAnalysis_PhaseProposalCollapse(t *testing.T) {
	tool := &EmitWriteAnalysis{}
	bus := newTestBusForWriteAnalysis()
	params := json.RawMessage(`{
		"raw_request": "x",
		"task": {"kind": "feature", "scope": "micro", "summary": "y"},
		"risk": {"affects_public_api": false, "changes_persistence": false, "changes_build_system": false, "overall": "low"},
		"phase_proposal": {
			"split": "sequential",
			"phases": [{"goal": "only one"}]
		}
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil || !res.Success {
		t.Fatalf("Execute failed: %v %s", err, res.Summary)
	}
	ir := bus.Mutable.WriteAnalysisIR()
	if ir.PhaseProposal.Split != "single" {
		t.Errorf("single-phase sequential should collapse to single; got %q", ir.PhaseProposal.Split)
	}
	if len(ir.PhaseProposal.Phases) != 0 {
		t.Errorf("expected phases dropped on collapse; got %d", len(ir.PhaseProposal.Phases))
	}
}

// TestEmitWriteAnalysis_RequiresMutable verifies the structural
// no-op safeguard against a misconfigured BusContext.
func TestEmitWriteAnalysis_RequiresMutable(t *testing.T) {
	tool := &EmitWriteAnalysis{}
	res, err := tool.Execute(&types.BusContext{}, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute returned err: %v", err)
	}
	if res.Success {
		t.Error("expected failure when Mutable is nil")
	}
	if !strings.Contains(res.Summary, "writable context") {
		t.Errorf("rejection summary = %q; expected 'writable context' mention", res.Summary)
	}
}
