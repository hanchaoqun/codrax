package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Use the public parser/engine without a JSON roundtrip: non-timeline views
// deliberately keep their native target timeline off the Result JSON surface.
func hmc313LiveTimeline(t *testing.T, view string) (tracequery.Result, tracequery.Query, types.ObservationSourceRef) {
	t.Helper()
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	bus, path := businessRefTestContext(t, string(body))
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	q := tracequery.Query{View: view, PID: 100, TimeStart: .999, TimeEnd: 1.051}
	native := tracequery.Run(idx, q)
	if native.NativeTargetTimeline() == nil || native.NativeTargetTimeline().MeasurementDomain == nil {
		t.Fatal("live native partition/domain missing")
	}
	if (native.Timeline != nil) != (view == "thread_timeline") {
		t.Fatal("fixture must exercise the private fallback for non-explicit views")
	}
	result := businessRefTestQuery(t, bus, map[string]any{"path": path, "view": view, "pid": 100, "time_start": .999, "time_end": 1.051})
	if !result.Success {
		t.Fatal(result.Summary)
	}
	var ref types.ObservationSourceRef
	var hasSources bool
	for _, row := range result.Observations {
		if row.Predicate == TraceThreadTimelinePredicate {
			ref = row.SourceRef
		}
		if row.Predicate == "target_window_states" && row.MeasurementSources != nil && len(row.MeasurementSources.Domains) > 0 {
			hasSources = true
		}
	}
	if ref.PayloadRef == "" || !hasSources {
		t.Fatal("public query lost timeline or separate measurement provenance")
	}
	raw, err := os.ReadFile(ref.PayloadRef)
	if err != nil {
		t.Fatal(err)
	}
	var saved tracequery.Result
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	account := saved.TargetWindowStates
	if saved.FrameRootCauseBundle != nil {
		account = saved.FrameRootCauseBundle.TargetWindowStates
	}
	if account == nil || account.MeasurementDomain == nil || account.MeasurementDomain.PartitionID == "" {
		t.Fatal("saved original JSON lost measurement provenance")
	}
	return native, q, ref
}

func hmc313TimelineObservation(t *testing.T, native tracequery.Result, q tracequery.Query, ref types.ObservationSourceRef) types.ObservationRecord {
	t.Helper()
	rows := traceQueryTypedObservations(native, ref.Path, ref.PayloadRef, ref.RawRef, "", time.Unix(1, 0), q)
	var found []types.ObservationRecord
	for _, row := range rows {
		if row.Predicate == TraceThreadTimelinePredicate {
			found = append(found, row)
		}
	}
	if len(found) != 1 {
		t.Fatalf("want exactly one full native timeline publication, got %d", len(found))
	}
	return found[0]
}

func TestHMC313LiveTimelineProvenanceDoesNotChangeDisplayFacts(t *testing.T) {
	for _, view := range []string{"thread_timeline", "window_stats", "wakeup_chain", "root_cause_rank", "critical_blocking_calls"} {
		t.Run(view, func(t *testing.T) {
			native, q, ref := hmc313LiveTimeline(t, view)
			timeline := native.NativeTargetTimeline()
			before, err := json.Marshal(timeline)
			if err != nil {
				t.Fatal(err)
			}
			original := timeline.MeasurementDomain
			base := hmc313TimelineObservation(t, native, q, ref)
			after, _ := json.Marshal(timeline)
			if !bytes.Equal(before, after) || timeline.MeasurementDomain != original {
				t.Fatal("display producer mutated the live original")
			}
			for _, mode := range []string{"absent", "other_partition", "other_provenance"} {
				t.Run(mode, func(t *testing.T) {
					domain := types.CloneTraceSchedulerMeasurementDomain(original)
					switch mode {
					case "absent":
						domain = nil
					case "other_partition":
						domain.PartitionID += "-independent-provenance"
					case "other_provenance":
						domain.Version, domain.Status, domain.Method = 2, "unknown", "other-measurement"
						domain.TargetTID, domain.QueryLineStart, domain.QueryLineEnd = 200, 3, 4
						domain.WindowStartTs, domain.WindowEndTs = 2, 3
					}
					timeline.MeasurementDomain = domain
					defer func() { timeline.MeasurementDomain = original }()
					got := hmc313TimelineObservation(t, native, q, ref)
					if !reflect.DeepEqual(base, got) {
						t.Fatal("provenance-only field changed full observation/display identity")
					}
					if timeline.MeasurementDomain != domain {
						t.Fatal("producer removed or replaced original provenance")
					}
				})
			}
		})
	}
}

func TestHMC313LiveTimelineUndisplayedNativeFactsRemainDistinct(t *testing.T) {
	for _, view := range []string{"thread_timeline", "window_stats"} {
		t.Run(view, func(t *testing.T) {
			native, q, ref := hmc313LiveTimeline(t, view)
			timeline := native.NativeTargetTimeline()
			base := hmc313TimelineObservation(t, native, q, ref)
			baseline, _ := json.Marshal(timeline)
			visible, _ := json.Marshal(base)
			for _, mode := range []string{"duration_precision", "head_witness", "interval_summary"} {
				t.Run(mode, func(t *testing.T) {
					var changed tracequery.TimelineResult
					if err := json.Unmarshal(baseline, &changed); err != nil {
						t.Fatal(err)
					}
					switch mode {
					case "duration_precision":
						changed.Intervals[0].DurationMs += 1e-10
					case "head_witness":
						changed.HeadState = &tracequery.TimelineHeadState{Status: "observed_in_index", BoundaryTs: changed.Window.StartTs,
							State: changed.Intervals[0].State, Reason: "independent native head witness"}
					case "interval_summary":
						changed.Intervals[0].Summary += " independent native interval fact"
					}
					original := *timeline
					*timeline = changed
					defer func() { *timeline = original }()
					got := hmc313TimelineObservation(t, native, q, ref)
					gotVisible, _ := json.Marshal(got)
					if !bytes.Equal(visible, gotVisible) {
						t.Fatal("negative control must retain identical displayed rows/columns/notes")
					}
					if reflect.DeepEqual(base, got) {
						t.Fatal("full native fact change was discarded behind equal visible strings")
					}
				})
			}
		})
	}
}
