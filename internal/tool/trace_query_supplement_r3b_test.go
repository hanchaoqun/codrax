package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// R3B-DEEP (§13.8, 2026-07-25) — the customer-shape END-TO-END pin: label-form
// user target (comm=unknown, name hits another tid), an unrelated in-window
// wakeup_new conflict, ledger pressure (600 padded observations with the
// supplement appended LAST), supplement rank+critical on the user window.
// On HEAD every leg holds: rank mints causal rows (incl. Rank=0 self rows),
// the supplement observations survive ledger compile, the projection carries
// the user window, and the coverage face knows it. The fifth replay's
// zero-row/all-unknown outcome therefore does NOT reproduce from this shape
// alone — the pin guards the proven legs while the residual divergence
// (composite tracebundle index lane / in-view cancellation partials) stays
// under investigation.
func TestR3BSupplementCustomerShapeReachesLedgerAndProjection(t *testing.T) {
	dir := t.TempDir()
	trace := strings.Join([]string{
		`unknown-32788 (32788) [004] .... 2.990000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=unknown next_pid=32788 next_prio=53`,
		`unknown-32788 (32788) [004] .... 3.001000: sched_switch: prev_comm=unknown prev_pid=32788 prev_prio=53 prev_state=D ==> next_comm=idle/4 next_pid=0 next_prio=120`,
		`unknown-32788 (32788) [004] .... 3.001200: sched_blocked_reason: pid=32788 iowait=0 caller=timerfd_read+0x70/0x25c`,
		`old-50173 (50173) [001] .... 3.005000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=old next_pid=50173 next_prio=20`,
		`ss.hm.ugc.aweme-33410 (33410) [005] .... 3.010000: sched_switch: prev_comm=idle/5 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=ss.hm.ugc.aweme next_pid=33410 next_prio=40`,
		`ss.hm.ugc.aweme-33410 (33410) [005] .... 3.012000: sched_switch: prev_comm=ss.hm.ugc.aweme prev_pid=33410 prev_prio=40 prev_state=S ==> next_comm=idle/5 next_pid=0 next_prio=120`,
		`old-50173 (50173) [001] .... 3.014000: sched_switch: prev_comm=old prev_pid=50173 prev_prio=20 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120`,
		`creator-7 (   7) [001] .... 3.075000: sched_wakeup_new: comm=new pid=50173 prio=20 target_cpu=001`,
		`sysmgr-99 (  99) [004] .... 3.150000: sched_wakeup: comm=unknown pid=32788 prio=53 target_cpu=004`,
		`unknown-32788 (32788) [004] .... 3.150500: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=unknown next_pid=32788 next_prio=53`,
		`unknown-32788 (32788) [004] .... 3.151000: sched_switch: prev_comm=unknown prev_pid=32788 prev_prio=53 prev_state=S ==> next_comm=idle/4 next_pid=0 next_prio=120`,
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, types.AttachedTraceBlobBasename), []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("分析卡顿")}
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		RuntimeTargets: []types.RuntimeTarget{{
			Kind: types.RuntimeTargetKindThread, Thread: "ss.hm.ugc.aweme [32788]",
			Source: "user_explicit", Confidence: 1,
		}},
	}}
	res, err := (&TraceQuery{}).Execute(ctx, json.RawMessage(`{"view":"event_search","pattern":"aweme","time_start":3.0,"time_end":3.2}`))
	if err != nil || !res.Success {
		t.Fatalf("model call failed: %+v %v", res, err)
	}
	ctx.ToolResults = append(ctx.ToolResults, res)
	for pad := 0; pad < 15; pad++ {
		junk := types.ToolResult{ToolName: "trace_query", Success: true}
		for j := 0; j < 40; j++ {
			junk.Observations = append(junk.Observations, types.ObservationRecord{
				ID:              fmt.Sprintf("trace_query:pad%d#io:%d", pad, j),
				Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
				Producer:        "trace_query",
				GroundingPolicy: types.ClaimGroundingHard,
				Predicate:       "io_activity",
				ClaimKey:        fmt.Sprintf("io_activity:pad%d-%d", pad, j),
				Subject:         fmt.Sprintf("padthread-%d-%d", pad, j),
				Object:          "io",
				Value:           "1.000",
				Unit:            "ms",
				RichNotes:       []string{"selected_window=3.000000..3.200000"},
				Confidence:      0.5,
			})
		}
		ctx.ToolResults = append(ctx.ToolResults, junk)
	}
	out := RunTraceQuerySystemSupplement(ctx)
	if len(out.Executed) != 2 || out.Executed[0] != "root_cause_rank" {
		t.Fatalf("supplement must run rank+critical on the user window: %+v", out)
	}
	supplementObs := 0
	for _, r := range ctx.Mutable.SystemTraceSupplementResults() {
		supplementObs += len(r.Observations)
	}
	if supplementObs == 0 {
		t.Fatal("supplement results lost their observations")
	}
	input := types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit)
	ledger := types.CompileObservationLedger(input)
	rootRows, stateRows := 0, 0
	for _, rec := range ledger.Records {
		if strings.HasPrefix(rec.Predicate, "root_cause") {
			rootRows++
		}
		if rec.Predicate == "target_window_states" {
			stateRows++
		}
	}
	if rootRows == 0 || stateRows == 0 {
		t.Fatalf("supplement causal/state rows must survive ledger pressure: root=%d state=%d records=%d", rootRows, stateRows, len(ledger.Records))
	}
	set := types.CompileTraceCausalProjectionSet(ledger)
	if len(set.Projections) != 1 || set.Projections[0].WindowStartTs != 3.0 || set.Projections[0].WindowEndTs != 3.2 {
		t.Fatalf("projection must carry the user window: %+v", set.Projections)
	}
	authority := runtimeTraceCoverageAuthority(input)
	if !authority.analysisWindowKnown || len(authority.targetStates) != 1 {
		t.Fatalf("coverage face must know the window and the state account: known=%v states=%d", authority.analysisWindowKnown, len(authority.targetStates))
	}
}
