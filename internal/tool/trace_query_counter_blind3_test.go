package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestTraceQuerySummaryPublishesTypedCounterIdentityAndQuality(t *testing.T) {
	result := tracequery.Result{
		View: "window_stats",
		WindowStats: &tracequery.WindowStats{
			CounterDeltas: []tracequery.TraceCounterDeltaSummary{{
				Thread:   tracequery.ThreadRef{Comm: "rs", PID: 1621, TGID: 1252},
				OwnerPID: 1252, OwnerScope: "payload_process", Name: "H:VSync-rs", TrailingTag: "I38",
				SourcePath: "/tmp/capture.systrace", Baseline: "in_window_first_sample", UnitStatus: "unknown",
				Samples: 2, First: 0, Last: 1, Min: 0, Max: 1, Delta: 1,
				FirstLine: 10, LastLine: 20, FirstLocalLine: 10, LastLocalLine: 20,
			}},
			CounterQuality: &tracequery.TraceCounterQualitySummary{
				Rows: 3, ValidIdentityRows: 3, NumericRows: 2, NonNumericRows: 1,
				TotalSeries: 2, PublishedSeries: 1, SuppressedSeries: 1,
				BaselinePolicy: "in_window_first_sample", UnitPolicy: "wire_schema_has_no_unit",
				Issues: []tracequery.TraceCounterIssueSummary{{
					Reason: "non_decimal_or_non_finite_value", Count: 1,
					Samples: []tracequery.TraceCounterIssueSample{{SourcePath: "/tmp/capture.systrace", LocalLine: 30, OwnerRaw: "1252", Name: "Heap", Value: "NaN"}},
				}},
			},
		},
	}
	summary := traceQuerySummary(result, traceQueryParams{View: "window_stats"}, "path", "/tmp/payload.json")
	for _, want := range []string{
		"counter_delta owner_scope=payload_process owner_pid=1252",
		"trailing_tag=I38 source=capture.systrace baseline=in_window_first_sample unit=unknown",
		"emitter=rs-1621",
		"counter_quality rows=3 valid_identity=3 numeric=2 invalid=0 non_numeric=1 derived_invalid_series=0",
		"counter_issue reason=non_decimal_or_non_finite_value count=1",
		`capture.systrace:30(owner=1252,name="Heap",value="NaN"`,
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, summary)
		}
	}
}
