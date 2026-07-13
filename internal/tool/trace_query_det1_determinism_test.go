package tool

// trace_query_det1_determinism_test.go — DET-1 determinism pin (v5 P1 批
// 追加件, 2026-07-13): identical input must produce identical typed
// observations across passes. The witness (donghu golden trace, aweme main
// thread window): the block_io_by_inode 0x14088d/0x25a01 storage_max
// attribution, the caliber_side member election (2.405↔2.694,
// member_fold_caliber=max_overlap_fallback), the io_burst_episode top-8
// census and an evidence_fact:root_cause_tertiary subject all flipped
// run-to-run — root cause: nearestBlockInodeForThread elected over a Go map
// in iteration order, so distance ties landed on a random inode (噪音从源头
// 消除; 帽/选举前确定性次序). This pin re-runs the SAME query set over one
// index several times in one process (each pass re-iterates the maps) and
// requires the sorted record stream byte-identical — it guards every future
// same-family regression, not just the witness.
//
// MUTATION self-check: restoring the map-order election (dropping the
// sorted-key iteration in nearestBlockInodeForThread) reds this pin with
// high probability per pass pair.

import (
	"context"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func det1RecordStream(t *testing.T, idx *tracequery.Index) string {
	t.Helper()
	var lines []string
	at := time.Unix(1751600000, 0).UTC()
	for _, view := range []string{"window_stats", "root_cause_rank", "critical_blocking_calls"} {
		result := tracequery.Run(idx, tracequery.Query{View: view, Thread: ".ugc.aweme.lite-17267", TimeStart: 13762.791708, TimeEnd: 13763.024898})
		for _, r := range traceQueryTypedObservations(result, "donghu.ftrace", "p-"+view, "r", "", at) {
			lines = append(lines, r.ClaimKey+"|"+r.Subject+"|"+r.Value+"|"+strings.Join(r.RichNotes, ";"))
		}
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func TestDET1TypedObservationsDeterministic(t *testing.T) {
	const trace = "../../eval/fixtures/real_traces/donghu.ftrace"
	if _, err := os.Stat(trace); err != nil {
		t.Skipf("golden fixture not present: %v", err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), trace)
	if err != nil {
		t.Fatal(err)
	}
	base := det1RecordStream(t, idx)
	for pass := 1; pass <= 3; pass++ {
		got := det1RecordStream(t, idx)
		if got == base {
			continue
		}
		baseLines := strings.Split(base, "\n")
		gotLines := strings.Split(got, "\n")
		for i := 0; i < len(baseLines) && i < len(gotLines); i++ {
			if baseLines[i] != gotLines[i] {
				t.Fatalf("pass %d diverged at sorted line %d:\n  base: %s\n  got:  %s", pass, i, baseLines[i], gotLines[i])
			}
		}
		t.Fatalf("pass %d diverged in line count: %d vs %d", pass, len(baseLines), len(gotLines))
	}
}
