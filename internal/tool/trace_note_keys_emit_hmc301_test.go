package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Use the normal public query against a real trace fixture. The selected
// window clips the first sleep but not the second, so both legitimate
// actual-vs-selected start forms ride the registry census unchanged.
func traceNoteKeysEmitMeasuredWaitFixtureHMC301(t *testing.T) []types.ObservationRecord {
	t.Helper()
	_, result, _ := runtimeWakeFixture(t)
	want := map[float64]struct {
		start, actual float64
		line          int
	}{
		10.004: {10.001, 10, 3},
		10.006: {10.005, 10.005, 7},
	}
	seen := make(map[float64]bool)
	for _, r := range result.Observations {
		wait, ok := types.DecodeTraceWakeupMeasuredWait(r)
		if !ok {
			continue
		}
		event, valid := types.DecodeRuntimeWakeupEvent(r)
		expected, exists := want[event.Timestamp]
		if !valid || !exists || seen[event.Timestamp] || wait.StartTs != expected.start || wait.EndTs != event.Timestamp ||
			wait.SegmentEndTs != event.Timestamp || wait.SegmentState != "s_sleep" || wait.StartLine != expected.line ||
			wait.ActualStartTs == nil || *wait.ActualStartTs != expected.actual {
			t.Fatalf("public measured-wait fixture drifted: event=%+v wait=%+v", event, wait)
		}
		if r.SourceRef.QueryScopeID == "" || r.SourceRef.PayloadRef == "" ||
			r.SourceRef.QueryWindowStartTs != 10.001 || r.SourceRef.QueryWindowEndTs != 10.012 ||
			r.SourceRef.Path != event.IndexPath || r.Span.StartTs != event.Timestamp || r.Span.EndTs != event.Timestamp {
			t.Fatalf("wait fixture lost its own query/source or changed the wake point: %+v", r)
		}
		seen[event.Timestamp] = true
	}
	if len(seen) != len(want) {
		t.Fatalf("public producer must supply both exact wait carriers, got %d want %d", len(seen), len(want))
	}
	return result.Observations
}

func TestHMC301MeasuredWaitEmissionFixtureBindings(t *testing.T) {
	records := traceNoteKeysEmitMeasuredWaitFixtureHMC301(t)
	var original types.ObservationRecord
	for _, r := range records {
		if _, ok := types.DecodeTraceWakeupMeasuredWait(r); ok {
			original = r
			break
		}
	}
	before, ok := types.DecodeRuntimeWakeupEvent(original)
	if !ok {
		t.Fatal("fixture must retain a separate valid physical wake event")
	}
	for _, name := range []string{"nil_optional", "unknown_actual_start", "unknown_state", "other_source", "other_query", "other_payload", "other_window"} {
		t.Run(name, func(t *testing.T) {
			r := original
			r.RichNotes = nil
			for _, note := range original.RichNotes {
				if value, found := strings.CutPrefix(note, "wakeup_measured_wait="); found {
					if name == "nil_optional" {
						continue
					}
					var p types.TraceWakeupMeasuredWaitPublication
					if err := json.Unmarshal([]byte(value), &p); err != nil {
						t.Fatal(err)
					}
					switch name {
					case "unknown_actual_start":
						p.Wait.ActualStartTs = nil
					case "unknown_state":
						p.Wait.SegmentState = "unknown"
					case "other_source":
						p.Path += ".other"
					case "other_query":
						p.QueryScopeID += ".other"
					case "other_payload":
						p.PayloadRef += ".other"
					case "other_window":
						p.QueryStartTs -= 1
					}
					wire, err := json.Marshal(p)
					if err != nil {
						t.Fatal(err)
					}
					note = "wakeup_measured_wait=" + string(wire)
				}
				r.RichNotes = append(r.RichNotes, note)
			}
			wait, valid := types.DecodeTraceWakeupMeasuredWait(r)
			if name == "unknown_actual_start" {
				if !valid || wait.ActualStartTs != nil || !strings.Contains(types.TraceWakeupMeasuredWaitDisplay(wait), "actual_segment_start=unknown start_clipped=unknown") {
					t.Fatalf("unknown actual start became zero or suppressed the selected measured interval: %+v", wait)
				}
			} else if valid {
				t.Fatal("missing/invalid/foreign optional measurement became qualified")
			}
			after, validEvent := types.DecodeRuntimeWakeupEvent(r)
			if !validEvent || !reflect.DeepEqual(before, after) || !reflect.DeepEqual(original.Span, r.Span) {
				t.Fatal("optional display change altered the physical wake event or point span")
			}
		})
	}
}

func TestHMC301IOEligibilityNoteEmissionAndClassification(t *testing.T) {
	row, ok := types.TraceNoteKeyLookup("root_cause_eligibility")
	if !ok || row.Family != "io" || row.Carrier != types.TraceNoteCarrierDisplayOnly || types.TraceNoteKeyIORootCauseEligibility != "root_cause_eligibility" {
		t.Fatalf("unparsed IO episode label must remain display-only: %+v", row)
	}
	waitRow, ok := types.TraceNoteKeyLookup("wakeup_measured_wait")
	if !ok || waitRow.Carrier != types.TraceNoteCarrierSoftConsumer {
		t.Fatalf("parsed optional wait must retain its soft-consumer contract: %+v", waitRow)
	}
	seen := 0
	for _, r := range traceNoteKeysEmitMeasuredWaitFixtureHMC301(t) {
		for _, note := range r.RichNotes {
			if note == "root_cause_eligibility=context_only_derived_projection" {
				if r.Predicate != "io_burst_episode" || r.ProvenanceLane != types.ObservationProvenanceArtifactSpan {
					t.Fatalf("display label changed the source observation lane: %+v", r)
				}
				seen++
			}
		}
	}
	if seen != 1 {
		t.Fatalf("real producer must exercise exactly one IO display label, got %d", seen)
	}
}
