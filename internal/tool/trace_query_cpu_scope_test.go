package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestThreadCPULoadPublishesRepresentativeNonExclusiveCPUScope(t *testing.T) {
	item := tracequery.ThreadCPULoadSummary{
		Thread:         tracequery.ThreadRef{PID: 17267, Comm: ".ugc.aweme.lite"},
		RunningMs:      157.248,
		RunnableWaitMs: 5.604,
		CPU:            12,
		CoreClass:      "big",
		Frequency:      2075000,
	}

	var caliber strings.Builder
	writeTraceThreadCPULoadCaliber(&caliber)
	for name, got := range map[string]string{
		"typed summary": traceQueryTypedThreadCPULoadSummary(item),
		"text caliber":  caliber.String(),
	} {
		for _, want := range []string{
			"running_scope=full_window_all_cpu",
			"runnable_scope=full_window_off_cpu_wait",
			"value_scope=running_plus_runnable_state_time_not_cpu_occupancy",
			"cpu_scope=dominant_state_slice_representative_not_exclusive",
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("%s left thread-load caliber %q ambiguous: %s", name, want, got)
			}
		}
	}
	var rowBanner strings.Builder
	writeTraceThreadCPULoad(&rowBanner, item)
	if got := rowBanner.String(); !strings.Contains(got, "cpu=12") || !strings.Contains(got, "cpu_scope=dominant_state_slice_representative_not_exclusive") {
		t.Fatalf("text row left representative CPU semantics ambiguous: %s", got)
	}

	rows := traceQueryTypedWindowStatsObservations(tracequery.WindowStats{
		ThreadCPULoad: []tracequery.ThreadCPULoadSummary{item},
	}, types.ObservationSourceRef{}, "scope", "now")
	if len(rows) != 1 {
		t.Fatalf("expected one typed thread-load row, got %+v", rows)
	}
	row := rows[0]
	if row.Object != tracequery.ThreadCPULoadValueObject || row.Value != "162.852" {
		t.Fatalf("thread state time must not bind to the representative CPU: %+v", row)
	}
	notes := strings.Join(row.RichNotes, " ")
	for _, want := range []string{
		"running_scope=full_window_all_cpu",
		"runnable_scope=full_window_off_cpu_wait",
		"value_scope=running_plus_runnable_state_time_not_cpu_occupancy",
		"cpu=12",
		"cpu_scope=dominant_state_slice_representative_not_exclusive",
	} {
		if !strings.Contains(notes, want) {
			t.Fatalf("typed thread-load row missing %q: %+v", want, row.RichNotes)
		}
	}
}
