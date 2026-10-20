package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC270WakeInventoryActualFinalizerKeepsReceiverPopulation(t *testing.T) {
	var trace strings.Builder
	for n := 0; n < 8; n++ {
		waker := 1021
		if n >= 5 {
			waker = 20975
		}
		fmt.Fprintf(&trace, "same-%d (908) [000] .... %.6f: sched_wakeup: comm=target pid=42 prio=120 target_cpu=1\n", waker, 1.001+float64(n)*.001)
	}
	trace.WriteString("target-42 (1182) [001] .... 1.009000: sched_wakeup: comm=other pid=99 prio=120 target_cpu=1\n")
	trace.WriteString("same-20975 (908) [000] .... 1.010000: sched_wakeup: comm=target pid=42 prio=120 target_cpu=1\n")
	result := traceInventoryScopePublicQuery(t, trace.String(), 1, 1.01, nil)
	ctx := traceInventoryScopeContext(result, 1, 1.01)
	ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 42, Source: "user_explicit"}}
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.FactFamilies = []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactDirectWaker}
	before := traceInventoryScopeAuthority(t, ctx)
	prompt := traceEventInventoryActualFinalizerPrompt(t, ctx)
	views := traceEventInventoryPromptViews(t, prompt)
	if len(views) != 1 || len(views[0].Inventory.Rows) != 8 || len(views[0].SelectorRowIDs) != 8 {
		t.Fatalf("finalizer inventory/selector lost receiver population: %+v", views)
	}
	if views[0].Inventory.Coverage.MatchedTotal != 10 {
		t.Fatal("display silently rewrote the original participation query census")
	}
	for n, row := range views[0].Inventory.Rows {
		tid := 1021
		if n >= 5 {
			tid = 20975
		}
		if row.EmitterTID != tid || row.EmitterTGID != 908 || row.EmitterTGIDKnown == nil || !*row.EmitterTGIDKnown {
			t.Fatalf("row %d lost observed thread/process identity: %+v", n, row)
		}
	}
	for _, token := range []string{"recorded_wakeup_receiver_tid", "同一来源中的不同 TID 不代表不同进程", "event_selection"} {
		if !strings.Contains(prompt, token) {
			t.Fatalf("public finalizer handoff omitted %q", token)
		}
	}
	if before != traceInventoryScopeAuthority(t, ctx) {
		t.Fatal("receiver writing projection mutated ledger/causal authority")
	}
}
