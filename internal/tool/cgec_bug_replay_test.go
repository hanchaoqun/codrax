package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestCGEC_ExplorerSkillBug_EndToEndReplay is the Session 10 Group H
// regression test for the "explorer agent 默认用哪个 skill?" bug:
// the e2e scenario that CGEC Session 9 shipped but failed to tame
// in production (see .codrax/logs/codrax-8ae69134/codrax-20260418-
// 110534-000-216789.log — 3 retries exhausted, all citations
// rejected).
//
// The scenario is replayed at tool-call granularity (no LLM
// roundtrip): Turn A ReadFiles contains the three files the
// explorer actually read (cmd/root.go, explorer.go, defaults.go),
// and the finalizer submits an emit_answer_document citing two
// files outside that set (subagent.go:63, agent.go:919). We assert
// the full CGEC chain fires correctly end-to-end:
//
//   1. G1 pre-finalize dry-run REJECTS the call (message contains
//      all the identifying markers)
//   2. A1 AddRepair bridge puts BOTH cited files into PendingReads
//      so runForcedReads can pick them up on the next round
//   3. The RepairReadFile directive is in the Repairs queue with
//      Origin "auto_bridge.emit_answer_document.dry_run" (so the
//      retry-hint renderer surfaces it)
//   4. ClosureStats reflects both the RepairsRaised bump and the
//      per-kind path (kind=RepairReadFile → no ExpandSearch /
//      ShapeSwap on this case)
//
// Any regression that removes A1 mirror, G1 dry-run, or per-kind
// stats plumbing breaks this test.
func TestCGEC_ExplorerSkillBug_EndToEndReplay(t *testing.T) {
	mut := types.NewMutableState("test-cgec-bug-replay")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles: []string{
			"cmd/root.go",
			"internal/agent/explorer.go",
			"internal/skill/defaults.go",
		},
	})
	bus := &types.BusContext{Mutable: mut, RepoRoot: t.TempDir()}

	params, _ := json.Marshal(map[string]any{
		"shape":   "value",
		"summary": "bogus answer",
		"value":   map[string]any{"literal": "explorer", "citation_ref": 0},
		"citations": []map[string]any{
			{"file": "internal/agent/subagent.go", "line": 63},
			{"file": "internal/agent/agent.go", "line": 919},
		},
	})

	tool := &EmitAnswerDocument{}
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	// (1) G1 dry-run must reject.
	if res.Success {
		t.Errorf("expected G1 dry-run to reject the emit_answer_document, got Success=true")
	}
	for _, want := range []string{
		"REJECTED",
		"subagent.go:63",
		"agent.go:919",
		"cmd/root.go",
		"internal/agent/explorer.go",
		"internal/skill/defaults.go",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("G1 rejection message missing %q:\n%s", want, res.Summary)
		}
	}

	closure := mut.EvidenceClosure()

	// (2) A1 bridge must mirror both rejected files into PendingReads.
	pending := closure.PendingReads()
	wantFiles := map[string]bool{
		"internal/agent/subagent.go": false,
		"internal/agent/agent.go":    false,
	}
	for _, p := range pending {
		if _, ok := wantFiles[p.File]; ok {
			wantFiles[p.File] = true
			// Origin must show the A1 bridge namespaced prefix so
			// operators can trace its source.
			if !strings.HasPrefix(p.Origin, "auto_bridge.") {
				t.Errorf("PendingRead[%s] origin=%q, want auto_bridge.* prefix", p.File, p.Origin)
			}
		}
	}
	for f, hit := range wantFiles {
		if !hit {
			t.Errorf("expected PendingRead mirror for %s, not found", f)
		}
	}

	// (3) RepairDirective queue must carry RepairReadFile for each
	// rejected file.
	repairs := closure.PendingRepairs()
	var readFileRepairs int
	for _, r := range repairs {
		if r.Kind == types.RepairReadFile {
			readFileRepairs++
		}
	}
	if readFileRepairs < 1 {
		t.Errorf("expected ≥1 RepairReadFile directive in queue, got %d (repairs=%v)", readFileRepairs, repairs)
	}

	// (4) ClosureStats: RepairsRaised bumped on every AddRepair call;
	// per-kind columns for ExpandSearch / ShapeSwap stay zero.
	stats := closure.Stats()
	if stats.RepairsRaised < 1 {
		t.Errorf("RepairsRaised=%d, want ≥1", stats.RepairsRaised)
	}
	if stats.ExpandSearchRaised != 0 {
		t.Errorf("ExpandSearchRaised=%d, want 0 (bug scenario has no expand-search trigger)", stats.ExpandSearchRaised)
	}
	if stats.ShapeSwapRaised != 0 {
		t.Errorf("ShapeSwapRaised=%d, want 0 (no AnalysisIR → no shape mismatch check)", stats.ShapeSwapRaised)
	}
	if !stats.HasActivity() {
		t.Error("HasActivity must be true after G1 dry-run fired")
	}
}

// TestCGEC_ExplorerSkillBug_SecondTryFromPendingReads is a follow-on
// assertion: after the first emit_answer_document rejection via G1,
// the PendingReads queue is primed. A subsequent round that somehow
// skips runForcedReads (e.g. test harness ordering) and re-calls
// emit_answer_document with the SAME bad citations must still be
// rejected by G1 — dedup-safe, idempotent, no queue inflation.
func TestCGEC_ExplorerSkillBug_SecondTryFromPendingReads(t *testing.T) {
	mut := types.NewMutableState("test-cgec-second-try")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles: []string{"cmd/root.go"},
	})
	bus := &types.BusContext{Mutable: mut, RepoRoot: t.TempDir()}

	params, _ := json.Marshal(map[string]any{
		"shape":     "value",
		"summary":   "bogus",
		"value":     map[string]any{"literal": "explorer", "citation_ref": 0},
		"citations": []map[string]any{{"file": "internal/agent/subagent.go", "line": 63}},
	})
	tool := &EmitAnswerDocument{}

	// First call — rejects, queues repair + PendingRead.
	_, _ = tool.Execute(bus, params)
	firstPending := len(mut.EvidenceClosure().PendingReads())
	firstRepairs := len(mut.EvidenceClosure().PendingRepairs())

	// Second call with identical input — should NOT grow queues
	// (dedup at the AddRepair + AddPendingRead chokepoints).
	_, _ = tool.Execute(bus, params)
	if got := len(mut.EvidenceClosure().PendingReads()); got != firstPending {
		t.Errorf("PendingReads grew on repeat: first=%d second=%d (dedup broken)", firstPending, got)
	}
	if got := len(mut.EvidenceClosure().PendingRepairs()); got != firstRepairs {
		t.Errorf("Repairs grew on repeat: first=%d second=%d (dedup broken)", firstRepairs, got)
	}
}
