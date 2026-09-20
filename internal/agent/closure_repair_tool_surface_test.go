package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestClosureRepairHintUsesExactToolSurfaceWithoutWaivingDebt(t *testing.T) {
	for _, tc := range []struct {
		name      string
		kind      types.RepairKind
		tools     []string
		known     bool
		available map[string]bool
		limited   bool
	}{
		{"runtime query remains available", types.RepairEmitEvidence, nil, true, map[string]bool{"trace_query": true, "emit_investigation_complete": true}, true},
		{"source tool available", types.RepairEmitEvidence, nil, true, map[string]bool{"emit_evidence": true, "emit_investigation_complete": true}, false},
		{"unknown tool surface", types.RepairEmitEvidence, nil, false, nil, false},
		{"source read unavailable", types.RepairReadFile, nil, true, map[string]bool{"trace_query": true, "emit_investigation_complete": true}, true},
		{"explicit producer tools unavailable", types.RepairStructuredHandoff, []string{"repo_map"}, true, map[string]bool{"emit_investigation_complete": true}, true},
		{"structured handoff available", types.RepairStructuredHandoff, []string{"emit_investigation_complete"}, true, map[string]bool{"trace_query": true, "emit_investigation_complete": true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mut := types.NewMutableState("mixed source contract")
			mut.EvidenceClosure().AddRepair(types.RepairDirective{
				Kind: tc.kind, Tools: tc.tools, Files: []string{"src/worker.go"},
				Subject: "principal member_set", Rationale: "producer-owned repair instruction",
				Origin: "emit_investigation_complete.relation_member_set",
			})
			before, _ := json.Marshal(mut.EvidenceClosure().ActiveRepairs())
			debtClass := types.ClassifyRepairDirective(mut.EvidenceClosure().ActiveRepairs()[0])
			e := &explorerEvaluator{phase: 1, mutable: mut}
			result := types.ToolResult{ToolName: "emit_investigation_complete", Success: true}
			obs := LoopObservation{Phase: PhaseMidLoop, Iteration: 6, LastToolResult: &result, AllToolResults: []types.ToolResult{result}, ToolSurfaceKnown: tc.known, AvailableToolNames: tc.available}
			sig := e.observeMidLoopWithContext(&types.AgentContext{Stage: types.StageExplore, Mutable: mut}, obs)
			if !sig.HintRequested || sig.StopRequested || mut.IsInvestigationComplete() {
				t.Fatalf("capability hints must retain the incomplete state: %+v", sig)
			}
			if strings.Contains(sig.Hint, "Repair Capability Limit") != tc.limited {
				t.Fatalf("hint does not match exact tool surface: %s", sig.Hint)
			}
			if tc.limited {
				for _, forbidden := range []string{"Re-emit grounded evidence", "re-emit grounded evidence if needed", "Read these blocking source", "producer-owned repair instruction", "After one structured handoff repair succeeds"} {
					if strings.Contains(sig.Hint, forbidden) {
						t.Fatalf("unavailable action leaked through %q: %s", forbidden, sig.Hint)
					}
				}
				wantDebt := []string{"src/worker.go", "remain unresolved", "does not waive"}
				if subject := mut.EvidenceClosure().ActiveRepairs()[0].Subject; subject != "" {
					wantDebt = append(wantDebt, subject)
				}
				for _, want := range wantDebt {
					if !strings.Contains(sig.Hint, want) {
						t.Fatalf("capability disclosure lost debt %q: %s", want, sig.Hint)
					}
				}
			} else if !strings.Contains(sig.Hint, "producer-owned repair instruction") && tc.kind == types.RepairEmitEvidence {
				t.Fatalf("available/unknown surface must preserve source instructions: %s", sig.Hint)
			}
			after, _ := json.Marshal(mut.EvidenceClosure().ActiveRepairs())
			if string(before) != string(after) || types.ClassifyRepairDirective(mut.EvidenceClosure().ActiveRepairs()[0]) != debtClass {
				t.Fatal("capability guidance mutated or weakened the stored repair")
			}
		})
	}
}

func TestClosureRepairHintMergedToolNeedsAndFooter(t *testing.T) {
	repairs := []types.RepairDirective{
		{Kind: types.RepairEmitEvidence, Tools: []string{"repo_map"}, Subject: "same debt", Rationale: "first action"},
		{Kind: types.RepairEmitEvidence, Subject: "same debt", Rationale: "second action"},
	}
	before, _ := json.Marshal(repairs)
	obs := LoopObservation{ToolSurfaceKnown: true, AvailableToolNames: map[string]bool{"repo_map": true, "emit_investigation_complete": true}}
	hint := renderClosureRepairHintWithToolSurface(repairs, false, obs)
	if !strings.Contains(hint, "Repair Capability Limit") || !strings.Contains(hint, "emit_evidence") || strings.Contains(hint, "first action") {
		t.Fatalf("merging hints lost the second repair's default required tool: %s", hint)
	}
	after, _ := json.Marshal(repairs)
	if string(before) != string(after) {
		t.Fatal("display merge mutated stored tool needs")
	}

	read := []types.RepairDirective{{Kind: types.RepairReadFile, Files: []string{"worker.go"}}}
	obs.AvailableToolNames = map[string]bool{"read_file": true, "emit_investigation_complete": true}
	hint = renderClosureRepairHintWithToolSurface(read, false, obs)
	if !strings.Contains(hint, "Read these blocking source") || strings.Contains(hint, "re-emit grounded evidence if needed") || !strings.Contains(hint, "requirements remain unchanged") {
		t.Fatalf("available read must not promise unavailable evidence materialization: %s", hint)
	}
	obs.AvailableToolNames = map[string]bool{"read_file": true}
	hint = renderClosureRepairHintWithToolSurface(read, false, obs)
	if strings.Contains(hint, "retry `emit_investigation_complete") || !strings.Contains(hint, "Preserve unresolved requirements") {
		t.Fatalf("unavailable completion tool must not be requested: %s", hint)
	}
}

func TestClosureRepairHintDisplayedActionsRespectCapabilities(t *testing.T) {
	for _, repair := range []types.RepairDirective{
		{Kind: types.RepairEmitEvidence, Advisory: true},
		{Kind: types.RepairEmitEvidence, Tools: []string{"repo_map"}},
	} {
		hint := renderClosureRepairHintWithToolSurface([]types.RepairDirective{repair}, false, LoopObservation{
			ToolSurfaceKnown: true, AvailableToolNames: map[string]bool{"repo_map": true, "emit_investigation_complete": true},
		})
		if !strings.Contains(hint, "Repair Capability Limit") || strings.Contains(hint, "Re-emit grounded evidence") {
			t.Fatalf("rendered action must be available even when scheduling metadata differs: %s", hint)
		}
		if repair.Advisory && !strings.Contains(hint, "does not create a completion blocker") {
			t.Fatalf("unavailable advisory action must not be promoted to blocking debt: %s", hint)
		}
	}
	hint := renderClosureRepairHintWithToolSurface([]types.RepairDirective{{Kind: types.RepairStructuredHandoff, Advisory: true, Tools: []string{"repo_map"}, Rationale: "producer instruction"}}, false, LoopObservation{
		ToolSurfaceKnown: true, AvailableToolNames: map[string]bool{"emit_investigation_complete": true},
	})
	if !strings.Contains(hint, "Repair Capability Limit") || !strings.Contains(hint, "does not create a completion blocker") || strings.Contains(hint, "producer instruction") {
		t.Fatalf("advisory explicit tools are still display capabilities, not blocking debt: %s", hint)
	}
	repairs := []types.RepairDirective{
		{Kind: types.RepairStructuredHandoff, Advisory: true, Tools: []string{"emit_investigation_complete"}, Subject: "same advice"},
		{Kind: types.RepairStructuredHandoff, Advisory: true, Tools: []string{"repo_map"}, Subject: "same advice"},
	}
	before, _ := json.Marshal(repairs)
	hint = renderClosureRepairHintWithToolSurface(repairs, false, LoopObservation{ToolSurfaceKnown: true, AvailableToolNames: map[string]bool{"emit_investigation_complete": true}})
	after, _ := json.Marshal(repairs)
	if !strings.Contains(hint, "Repair Capability Limit") || !strings.Contains(hint, "does not create a completion blocker") || string(before) != string(after) {
		t.Fatalf("merged advisory explicit tools lost capabilities or mutated scheduling metadata: %s", hint)
	}
}

func TestClosureOnlyRepairCannotReintroduceUnavailableAction(t *testing.T) {
	mut := types.NewMutableState("source proof remains required")
	mut.EvidenceClosure().AddRepair(types.RepairDirective{Kind: types.RepairEmitEvidence, Files: []string{"src/worker.go"}, Origin: "pre_complete.relation_member_set"})
	before, _ := json.Marshal(mut.EvidenceClosure().ActiveRepairs())
	e := &explorerEvaluator{phase: 1, mutable: mut, midLoopClosureRepairSent: true, midLoopClosureRepairResultsLen: 1, midLoopLastResultsLen: 1}
	results := []types.ToolResult{{ToolName: "emit_investigation_complete", Success: true}, {ToolName: "read_file", Success: true}}
	sig := e.observeMidLoop(LoopObservation{
		Phase: PhaseMidLoop, Iteration: 7, LastToolResult: &results[1], AllToolResults: results,
		ToolSurfaceKnown: true, AvailableToolNames: map[string]bool{"read_file": true, "emit_investigation_complete": true},
	})
	if !sig.HintRequested || !strings.Contains(sig.Hint, "Repair Capability Limit") || strings.Contains(sig.Hint, "emit a corrected grounded evidence batch") {
		t.Fatalf("later closure-only hint reintroduced an unavailable action: %+v", sig)
	}
	after, _ := json.Marshal(mut.EvidenceClosure().ActiveRepairs())
	if string(before) != string(after) || mut.IsInvestigationComplete() || sig.StopRequested {
		t.Fatal("later capability guidance changed repair authority")
	}
}
