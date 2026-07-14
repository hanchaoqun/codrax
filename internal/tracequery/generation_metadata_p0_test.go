package tracequery

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

const reusedTIDPriorityFixture = `          <idle>-0 [000] d..1 1.000000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=42 next_prio=40
       worker-42 [000] d..1 1.100000: sched_switch: prev_comm=worker prev_pid=42 prev_prio=40 prev_state=X ==> next_comm=swapper/0 next_pid=0 next_prio=120
          <idle>-0 [002] d..1 1.200000: sched_switch: prev_comm=swapper/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=43 next_prio=30
       worker-43 [002] d..1 1.300000: sched_switch: prev_comm=worker prev_pid=43 prev_prio=30 prev_state=X ==> next_comm=swapper/2 next_pid=0 next_prio=120
       worker-42 [001] d..1 2.000000: sched_wakeup: comm=peer pid=99 prio=80 target_cpu=001
       worker-43 [002] d..1 2.010000: sched_wakeup: comm=peer pid=98 prio=80 target_cpu=002
          <idle>-0 [003] d..1 2.200000: sched_switch: prev_comm=swapper/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=42 next_prio=90
`

func TestReusedTIDPriorityAndCPUMetadataStayInsideNewGeneration(t *testing.T) {
	idx := buildTraceIndex(t, "generation-priority.htrace", reusedTIDPriorityFixture)
	if boundaries, capped := threadGenerationBoundaries(idx, 42); capped || len(boundaries) == 0 {
		t.Fatal("fixture must carry an exact X/Z→reappearance lifecycle boundary")
	}
	unknownThread := ThreadRef{PID: 43, Comm: "worker"}

	// The new occupant has reappeared but has not yet published its own
	// priority or switch-in sample. The old occupant's prio=40 / cpu=0 must
	// not be used as a convenient nearest fallback.
	if prio, class := threadPriorityNear(idx, TraceFlavorHarmonyHitrace, unknownThread, 2.020000); prio != 0 || class != "" {
		t.Fatalf("old generation priority leaked into new occupant: prio=%d class=%q", prio, class)
	}
	cache := newChainQueryCache(idx, nil)
	if prio, class := cache.priorityNear(TraceFlavorHarmonyHitrace, unknownThread, 2.020000); prio != 0 || class != "" {
		t.Fatalf("cached old generation priority leaked into new occupant: prio=%d class=%q", prio, class)
	}
	if cpu, ok := cache.threadCPUNear(unknownThread, 2.020000); ok {
		t.Fatalf("old generation switch-in leaked into new occupant: cpu=%d", cpu)
	}

	// Existing nearest-sample behavior remains inside the same generation:
	// priority may use the nearest later sample, while CPU-near still requires
	// an actual switch-in at or before the lookup point.
	thread := ThreadRef{PID: 42, Comm: "worker"}
	if prio, _ := threadPriorityNear(idx, TraceFlavorHarmonyHitrace, thread, 2.100000); prio != 90 {
		t.Fatalf("same-generation nearest priority = %d, want 90", prio)
	}
	if prio, _ := cache.priorityNear(TraceFlavorHarmonyHitrace, thread, 2.100000); prio != 90 {
		t.Fatalf("cached same-generation nearest priority = %d, want 90", prio)
	}
	if _, ok := cache.threadCPUNear(thread, 2.100000); ok {
		t.Fatal("future same-generation switch-in must not be treated as CPU-at-point")
	}
	if cpu, ok := cache.threadCPUNear(thread, 2.210000); !ok || cpu != 3 {
		t.Fatalf("same-generation CPU-near = (%d,%v), want (3,true)", cpu, ok)
	}
}

const reusedTIDTGIDFixture = `          <idle>-0 [000] d..1 1.000000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=42 next_prio=40
       worker-42 [000] d..1 1.010000: tracing_mark_write: B|500|oldVoteOne
       worker-42 [000] d..1 1.020000: tracing_mark_write: E|500|
       worker-42 [000] d..1 1.030000: tracing_mark_write: B|500|oldVoteTwo
       worker-42 [000] d..1 1.040000: tracing_mark_write: E|500|
       worker-42 [000] d..1 1.100000: sched_switch: prev_comm=worker prev_pid=42 prev_prio=40 prev_state=X ==> next_comm=swapper/0 next_pid=0 next_prio=120
       creator-7 [001] d..1 2.000000: sched_wakeup_new: comm=worker pid=42 prio=90 target_cpu=001
          <idle>-0 [001] d..1 2.010000: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=42 next_prio=90
       worker-42 [001] d..1 2.020000: tracing_mark_write: B|600|newVote
       worker-42 [001] d..1 2.030000: tracing_mark_write: E|600|
       worker-42 [001] d..1 2.050000: sched_switch: prev_comm=worker prev_pid=42 prev_prio=90 prev_state=S ==> next_comm=swapper/1 next_pid=0 next_prio=120
`

func assertNewGenerationTGID(t *testing.T, idx *Index) {
	t.Helper()
	q := Query{TimeStart: 2.0, TimeEnd: 2.1, TraceFlavorHint: TraceFlavorHarmonyHitrace}
	stats := ComputeWindowStats(idx, q)
	row := b3ProcessRow(t, stats.ProcessCPULoad, 600)
	if row.ThreadCount != 1 || row.Process.PID != 600 {
		t.Fatalf("new generation process grouping = %+v, want pid=600/thread_count=1", row)
	}
	for _, candidate := range stats.ProcessCPULoad {
		if candidate.Process.PID == 500 && candidate.ThreadCount > 0 {
			t.Fatalf("old generation TGID vote leaked into new occupant: %+v", candidate)
		}
	}
}

func TestReusedTIDDerivedTGIDIsGenerationScopedColdAndWarm(t *testing.T) {
	t.Run("cold full index", func(t *testing.T) {
		idx := buildTraceIndex(t, "generation-tgid-cold.htrace", reusedTIDTGIDFixture)
		if got := idx.derivedTidTgid().tgidFor(42); got != 500 {
			t.Fatalf("fixture must prove the unsafe full-index vote would choose old pid=500, got %d", got)
		}
		assertNewGenerationTGID(t, idx)
	})

	t.Run("warm derived window with old rows retained as padding", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "generation-tgid-warm.htrace")
		if err := os.WriteFile(path, []byte(reusedTIDTGIDFixture), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := BuildIndex(context.Background(), path); err != nil {
			t.Fatal(err)
		}
		idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
			AllowWindowedParse: true,
			TimeStart:          2.0,
			TimeStartSet:       true,
			TimeEnd:            2.1,
			TimeEndSet:         true,
			TimePaddingBefore:  2.0,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !idx.Windowed || len(idx.threadIncarnationFailures) == 0 {
			t.Fatalf("expected warm padded window to retain lifecycle proof: windowed=%v failures=%+v", idx.Windowed, idx.threadIncarnationFailures)
		}
		assertNewGenerationTGID(t, idx)
	})
}
