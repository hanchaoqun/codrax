package types_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC232SleepInstanceActualBusinessFixture(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var results []types.ToolResult
	for _, pid := range []int{100, 200} {
		for _, view := range []string{"wakeup_chain", "root_cause_rank"} {
			params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": view, "pid": pid, "time_start": .999, "time_end": 1.051})
			r, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
			if err != nil || !r.Success {
				t.Fatalf("query %s pid=%d: %v %s", view, pid, err, r.Summary)
			}
			results = append(results, r)
		}
	}
	mut := types.NewMutableState("Inspect recorded sleep instances")
	for _, r := range results {
		mut.AppendDispatchToolResult(r)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: mut, ToolResults: results}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
	p := types.CompileTraceCausalProjection(ledger)
	var closed, tail int
	for _, bucket := range [][]types.TraceCausalProjectionNode{p.PrimaryRootCauses, p.OnChainCauses, p.AdjacentCauses, p.BackgroundCauses, p.SupportingHops} {
		for _, n := range bucket {
			if n.Subject != "document-worker-200" || !n.IsSleepState() {
				continue
			}
			t.Logf("sleep id=%s window=%.9f..%.9f lines=%d..%d branch=%d wake=%t %.9f", n.EvidenceID, n.StartTs, n.EndTs, n.LineStart, n.LineEnd, n.ChainBranch, n.DrilldownWakeupPointKnown, n.DrilldownWakeupTs)
			if n.StartTs == 1.0455 {
				tail++
				if n.DrilldownWakeupPointKnown || n.DrilldownWakeupTs != 0 || n.DrilldownWakeupLine != 0 {
					t.Errorf("open tail sleep borrowed earlier wake: %+v", n)
				}
			}
			if n.EndTs == 1.040010 {
				closed++
				if !n.DrilldownWakeupPointKnown || n.DrilldownWakeupTs != 1.040010 || n.DrilldownWakeupLine != 12 {
					t.Errorf("closed native wait lost exact wake: %+v", n)
				}
			}
		}
	}
	if tail == 0 || closed == 0 {
		t.Fatalf("fixture did not exercise both wait instances: closed=%d tail=%d", closed, tail)
	}
}

func TestHMC232SleepInstancePublicSubMicrosecondGap(t *testing.T) {
	const trace = `# tracer: nop
app-100 (100) [001] .... 1.000000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=120
worker-200 (100) [002] .... 1.000000000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=120
app-100 (100) [001] .... 1.001000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
worker-200 (100) [002] .... 1.001000000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=120 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
irq-80 (2) [002] .... 1.002000000: sched_wakeup: comm=worker pid=200 prio=120 target_cpu=002
worker-200 (100) [002] .... 1.002000100: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=120
worker-200 (100) [002] .... 1.002000200: sched_switch: prev_comm=worker prev_pid=200 prev_prio=120 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
irq-80 (2) [002] .... 1.003000000: sched_wakeup: comm=worker pid=200 prio=120 target_cpu=002
worker-200 (100) [002] .... 1.003000000: sched_wakeup: comm=app pid=100 prio=120 target_cpu=001
app-100 (100) [001] .... 1.004000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=120
app-100 (100) [001] .... 1.005000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
`
	dir := t.TempDir()
	path := filepath.Join(dir, "micro-gap.systrace")
	if err := os.WriteFile(path, []byte(trace), 0600); err != nil {
		t.Fatal(err)
	}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "wakeup_chain", "pid": 100, "time_start": 1.0, "time_end": 1.005, "min_duration_ms": .00001})
	r, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
	if err != nil || !r.Success {
		t.Fatalf("native gap query: %v %s", err, r.Summary)
	}
	mut := types.NewMutableState("inspect physical waits")
	mut.AppendDispatchToolResult(r)
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(&types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: mut, ToolResults: []types.ToolResult{r}}, types.ObservationExtractLedgerEvidenceLimit))
	p := types.CompileTraceCausalProjection(ledger)
	found, events := false, 0
	for _, record := range ledger.Records {
		if _, ok := types.DecodeRuntimeWakeupEvent(record); ok {
			events++
		}
	}
	for _, bucket := range [][]types.TraceCausalProjectionNode{p.OnChainCauses, p.SupportingHops, p.BackgroundCauses, p.PrimaryRootCauses, p.AdjacentCauses} {
		for _, n := range bucket {
			if n.Subject != "worker-200" || n.StartTs != 1.001 || n.EndTs != 1.003 || !n.IsSleepState() {
				continue
			}
			found = true
			if n.RunningMS != 0 || n.RunnableMS != 0 || n.StateOccupancyPresence&3 == 0 {
				t.Fatalf("fixture must have positive non-sleep states rounded to 0.000ms: running=%g runnable=%g presence=%d", n.RunningMS, n.RunnableMS, n.StateOccupancyPresence)
			}
			if n.DrilldownWakeupPointKnown || n.DrilldownWakeupTs != 0 || n.DrilldownWakeupLine != 0 {
				t.Fatal("two sleeps separated by 200ns borrowed the last member's closure")
			}
			if n.DrilldownTarget != "irq-80" {
				t.Fatal("unproved aggregate closure erased native upstream navigation")
			}
		}
	}
	if !found || events < 2 {
		t.Fatalf("fixture lost native mixed wait/event evidence: mixed=%t events=%d", found, events)
	}
}
