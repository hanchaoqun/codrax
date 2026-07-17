package tracequery

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRelationScopeIgnoresPerfIdentityBeforeGenerationLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relation-perf-authority.ftrace")
	body := strings.Join([]string{
		"app-100 (100) [001] .... 4.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120",
		"app-200 (999) [002] .... 4.005000: perf_sample: cpu=2 cpu_known=true pid=999 tid=200 thread_comm=app sample_weight=1 event=cpu-cycles symbol=Hot source=fixture sample_kind=on_cpu",
		"noise-77 (77) [003] .... 4.010000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=noise next_pid=77 next_prio=120",
		"app-100 (100) [001] .... 4.020000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		TimeStart: 3.9, TimeEnd: 4.1, TimeStartSet: true, TimeEndSet: true,
		AllowWindowedParse: true, RelationScoped: true, ScopeThread: "app",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !idx.RelationScoped {
		t.Fatalf("a scheduler-proved unique selector was not relation scoped: caveats=%v events=%+v", idx.Caveats, idx.Events)
	}
	if containsSubstring(idx.Caveats, "relation_scope_thread_ambiguous") {
		t.Fatalf("perf TGID/TID/comm became a second relation seed authority: %v", idx.Caveats)
	}
	for _, ev := range idx.Events {
		if ev.Type == EventPerfSample || ev.PrevPID == 77 || ev.NextPID == 77 || ev.PID == 77 {
			t.Fatalf("relation scope retained rows selected only by perf/noise identity: %+v", ev)
		}
	}
}

func TestRelationScopePerfOnlyNameDegradesToUnprunedWithCaveat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relation-perf-only.ftrace")
	body := strings.Join([]string{
		"app-200 (999) [002] .... 4.005000: perf_sample: cpu=2 cpu_known=true pid=999 tid=200 thread_comm=app sample_weight=1 event=cpu-cycles symbol=Hot source=fixture sample_kind=on_cpu",
		"noise-77 (77) [003] .... 4.010000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=noise next_pid=77 next_prio=120",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		TimeStart: 3.9, TimeEnd: 4.1, TimeStartSet: true, TimeEndSet: true,
		AllowWindowedParse: true, RelationScoped: true, ScopeThread: "app",
	})
	if err != nil {
		t.Fatal(err)
	}
	if idx.RelationScoped || len(idx.Events) != 2 {
		t.Fatalf("perf-only display name hard-pruned the causal universe: relation_scoped=%t events=%+v caveats=%v", idx.RelationScoped, idx.Events, idx.Caveats)
	}
	if !containsSubstring(idx.Caveats, "relation_scope_thread_unresolved") {
		t.Fatalf("perf-only selector degradation was not disclosed: %v", idx.Caveats)
	}
}

func TestCollectRelationScopeThreadCandidatesRejectsPerfEnvelopeAndBodyCoordinates(t *testing.T) {
	sel := parseThreadSelector("app")
	out := map[int]struct{}{}
	collectRelationScopeThreadCandidates(sel, Event{
		Type: EventPerfSample, PID: 200, TGID: 999, Comm: "app",
		PerfFields: &PerfFields{PID: 999, TID: 200, Comm: "app"},
	}, out)
	if len(out) != 0 {
		t.Fatalf("perf envelope/body coordinates entered pre-ledger relation candidates: %v", out)
	}
}
