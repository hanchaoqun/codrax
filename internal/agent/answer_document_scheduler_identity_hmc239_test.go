package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestSchedulerIdentityActualFinalizer239(t *testing.T) {
	result := traceInventoryScopePublicQuery(t, "record-11 (10) [000] .... 1.001000: sched_wakeup: comm=target pid=42 prio=120 target_cpu=0\n", 1, 1.01, nil)
	ctx := traceInventoryScopeContext(result, 1, 1.01)
	ctx.AnalysisIR.RequestModel.RuntimeTargets = nil
	before := traceInventoryScopeAuthority(t, ctx)
	prompt := traceEventInventoryActualFinalizerPrompt(t, ctx)
	for _, want := range []string{`"key":"scheduler.wakee.tid"`, `"value":"42"`, "唤醒目标线程编号（TID）", "唤醒目标线程所属进程编号（TGID）", types.TraceSchedulerIdentityTeaching} {
		if !strings.Contains(prompt, want) {
			t.Errorf("actual finalizer lost %q", want)
		}
	}
	if before != traceInventoryScopeAuthority(t, ctx) {
		t.Fatal("display changed ledger or causal authority")
	}
}

func TestSchedulerIdentityFiniteWakeeScopeRemainsIndependent239(t *testing.T) {
	result := traceInventoryScopePublicQuery(t, "record-11 (10) [000] .... 1.001000: sched_wakeup: comm=target pid=42 prio=120 target_cpu=0\n", 1, 1.01, map[string]any{"pid": 42})
	ctx := traceInventoryScopeContext(result, 1, 1.01)
	ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 42, Thread: "target-42", Source: "user_explicit"}}
	original := traceInventoryScopeReceipt(t, ctx)
	if len(original.EventSearchInventory.Rows) != 1 {
		t.Fatal("query missed exact wakee")
	}
	before := traceInventoryScopeAuthority(t, ctx)
	prompt := traceEventInventoryActualFinalizerPrompt(t, ctx)
	views := traceEventInventoryPromptViews(t, prompt)
	// Existing member scope matches the emitter, not scheduler endpoint roles.
	// HMC-01.3/16.4 remains open: new display fields must not silently acquire
	// owner authority to work around that separate contract. The RED experiment
	// showing the omitted row is preserved outside the repository.
	if len(views) != 1 || len(views[0].Inventory.Rows) != 0 || views[0].PromptRowsOmitted != 1 {
		t.Fatalf("display semantics changed existing owner qualification: %+v", views)
	}
	if before != traceInventoryScopeAuthority(t, ctx) {
		t.Fatal("display altered full ledger")
	}
	retainedWakeup := false
	for _, record := range answerDocObservationLedger(ctx).Records {
		if record.Predicate == "scheduler_wakeup_event" && record.Subject == "record-11" && record.Object == "target-42" {
			retainedWakeup = true
		}
	}
	if !retainedWakeup {
		t.Fatal("display removed the existing scheduler wakeup observation")
	}
}
