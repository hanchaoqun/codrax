package tool

// trace_query_det1_determinism_test.go — DET-1 determinism pin (v5 P1 批
// 追加件, 2026-07-13): identical input must produce identical typed
// observations across passes. The original witness was a nondeterministic
// nearest-thread/time inode↔block election; that unproven join is now removed
// entirely. This pin still re-runs the SAME query set over one
// index several times in one process (each pass re-iterates the maps) and
// requires the sorted record stream byte-identical — it guards every future
// same-family regression, not just the witness.
//
// MUTATION self-check: any future map-order leak in the remaining typed
// carriers still diverges this byte stream across passes.

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
