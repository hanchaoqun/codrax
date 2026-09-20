package tool

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryIODetailCoverageCountsNativeSelectionNotProjectedRows(t *testing.T) {
	for _, beyond := range []int{0, 18} {
		t.Run(fmt.Sprint(beyond), func(t *testing.T) {
			stats := tracequery.WindowStats{
				Window: tracequery.TimeWindow{StartTs: 0, EndTs: 20},
				IOLatencies: []tracequery.IOLatencySummary{
					{IssueThread: tracequery.ThreadRef{Comm: "issuer", PID: 41}, IssueTs: 10, CompleteTs: 10.001, DurationMs: 1},
					// A valid native zero-duration pair stays in the selected
					// detail list, but the existing observation loop omits it.
					{IssueThread: tracequery.ThreadRef{Comm: "issuer", PID: 41}, IssueTs: 11, CompleteTs: 11},
				},
				IOLatencyOverflowCount: beyond,
			}
			ref := types.ObservationSourceRef{Path: "/captures/io.ftrace", QueryScopeID: "query", QueryWindowKnown: true, QueryWindowEndTs: 20}
			rows := traceQueryTypedIOLatencyObservations(stats, ref, "test", "now")
			pairs := 0
			var coverage types.ObservationRecord
			for _, row := range rows {
				if row.Predicate == "io_latency" {
					pairs++
				} else if row.Predicate == "io_latency_coverage" {
					coverage = row
				}
			}
			if pairs != 1 || coverage.ResultCount == nil || *coverage.ResultCount != 2+beyond {
				t.Fatalf("test needs unchanged native population and a smaller observation rowset: %d %+v", pairs, coverage)
			}
			if coverage.SourceRef != ref || coverage.Unit != "requests" || coverage.Value != fmt.Sprint(2+beyond) {
				t.Fatalf("publication changed exact counts or query source: %+v", coverage)
			}
			notes := strings.Join(coverage.RichNotes, "\n")
			for _, want := range []string{"io_latency_emitted=2", "total=" + fmt.Sprint(2+beyond), "io_latency_overflow_pairs=" + fmt.Sprint(beyond)} {
				if !strings.Contains(notes, want) {
					t.Errorf("native count lost, including explicit zero: %q: %s", want, notes)
				}
			}
			meaning := types.TraceIODetailCoverageFromObservation(coverage).PromptMeaning()
			if !strings.HasPrefix(coverage.Summary, meaning) {
				t.Errorf("producer and consumer diverged: %s != %s", coverage.Summary, meaning)
			}
			banner := traceQuerySummary(tracequery.Result{View: "window_stats", WindowStats: &stats}, traceQueryParams{}, "trace", "payload")
			for name, got := range map[string]string{"summary": coverage.Summary, "tool banner": banner} {
				for _, want := range []string{"query-selected details=2", "not current prompt row counts", "not capture or scan completeness", "not failed or missing-endpoint pairs"} {
					if !strings.Contains(got, want) {
						t.Errorf("%s lost native/projection boundary %q: %s", name, want, got)
					}
				}
			}
		})
	}
}
