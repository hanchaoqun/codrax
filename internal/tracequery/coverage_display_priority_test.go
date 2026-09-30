package tracequery

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestCoverageDisplayKeepsObservedQualityAheadOfOptionalInventory(t *testing.T) {
	// Unknown family/table names exercise the generic policy, not a HiSys seat.
	rows := make([]traceBundleCoverage, 0, 100)
	for i := 0; i < 90; i++ {
		rows = append(rows, traceBundleCoverage{Family: "optional", Table: fmt.Sprint("missing_", i), Skipped: "table_absent"})
	}
	rows = append(rows,
		traceBundleCoverage{Family: "capture_completeness", Table: "stat", Role: "capture_completeness"},
		traceBundleCoverage{Family: "custom", Table: "successful", Found: true, RowsRead: 12, RowsEmitted: 12},
		traceBundleCoverage{Family: "custom", Table: "affected", Found: true, RowsRead: 10, RowsEmitted: 8, Skipped: "invalid_timestamp=2, invalid_identity=5"})
	before := append([]traceBundleCoverage(nil), rows...)
	got := traceBundleCoverageCaveats("tracebundle_trace_db_coverage", rows)
	if !reflect.DeepEqual(rows, before) {
		t.Fatal("display ordering mutated source receipts")
	}
	if !strings.Contains(got[0], "table=affected") || !strings.Contains(got[1], "table=successful") {
		t.Fatalf("source quality evicted by optional tables: %v", got)
	}
	for _, want := range []string{"rows_read=10", "rows_emitted=8", "invalid_timestamp=2", "invalid_identity=5", "counts_scope=source_table_before_query_filters", "diagnostic_counts_may_overlap", "untimed_rows_have_no_query_window_assignment"} {
		if !strings.Contains(got[0], want) {
			t.Errorf("missing %q: %s", want, got[0])
		}
	}
	if len(got) > traceBundleCoverageCaveatLimit+traceBundleCoveragePriorityCaveatLimit+1 || !strings.Contains(strings.Join(got, "\n"), "capture_state=unknown") {
		t.Fatalf("budget/protocol regression: %v", got)
	}
	if !strings.Contains(got[len(got)-1], "total=93 emitted=24") {
		t.Fatal("omitted inventory was not disclosed")
	}
}
