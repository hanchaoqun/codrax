package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1618TargetWaitOccurrencePublicationRetainsQueryWindow(t *testing.T) {
	for _, tc := range []struct {
		name   string
		window tracequery.TimeWindow
		want   string
	}{
		{"positive", tracequery.TimeWindow{StartTs: 10, EndTs: 11}, "selected_window=10.000000..11.000000"},
		{"explicit_zero", tracequery.TimeWindow{StartTs: 0, EndTs: 11, StartSet: true}, "selected_window=0.000000..11.000000"},
		{"unknown_start", tracequery.TimeWindow{EndTs: 11}, ""},
	} {
		for _, measuredZero := range []bool{false, true} {
			name := tc.name + "/rows"
			if measuredZero {
				name = tc.name + "/measured_zero"
			}
			t.Run(name, func(t *testing.T) {
				account := &tracequery.TargetWindowStateAccount{
					Thread: tracequery.ThreadRef{Comm: "main", PID: 42},
					Window: tc.window, WaitOccurrenceStatus: "complete",
					WindowMs: 1000, TotalMs: 1000, RunningMs: 1000,
				}
				if !measuredZero {
					account.WaitOccurrences = []tracequery.TargetWindowStateOccurrence{
						{Ordinal: 1, State: tracequery.StateDSleep, StartTs: 10.25, EndTs: 10.251, DurationMs: 1, StartLine: 20, EndLine: 25},
						{Ordinal: 2, State: tracequery.StateIOWait, StartTs: 10.5, EndTs: 10.502, DurationMs: 2, StartLine: 40, EndLine: 50, IOWaitKnown: true, IOWait: true},
					}
					account.WaitOccurrenceTotal, account.WaitOccurrenceEmitted = 2, 2
					account.RunningMs, account.DStateMs, account.IOWaitMs = 997, 1, 2
				}
				before, err := json.Marshal(account)
				if err != nil {
					t.Fatal(err)
				}
				records := traceQueryTypedObservations(tracequery.Result{TargetWindowStates: account},
					"trace.ftrace", "", "trace.ftrace", "", time.Unix(1, 0))
				sets, leaves := 0, 0
				for _, record := range records {
					switch record.Predicate {
					case "target_window_wait_occurrences":
						sets++
						if record.ResultCount == nil || *record.ResultCount != account.WaitOccurrenceTotal ||
							record.Span.StartTs != tc.window.StartTs || record.Span.EndTs != tc.window.EndTs {
							t.Fatalf("aggregate query range/count changed: %+v", record)
						}
					case "target_window_wait_occurrence":
						original := account.WaitOccurrences[leaves]
						leaves++
						if record.Span.StartTs != original.StartTs || record.Span.EndTs != original.EndTs ||
							record.Span.LineStart != original.StartLine || record.Span.LineEnd != original.EndLine {
							t.Fatalf("leaf must retain its own interval, independently of the query window: %+v", record)
						}
					default:
						continue
					}
					var selected []string
					for _, note := range record.RichNotes {
						if strings.HasPrefix(note, types.TraceNoteKeySelectedWindow+"=") {
							selected = append(selected, note)
						}
					}
					var want []string
					if tc.want != "" {
						want = []string{tc.want}
					}
					if !reflect.DeepEqual(selected, want) {
						t.Errorf("%s selected query window = %v, want %v; leaf Span must not substitute for account.Window", record.Predicate, selected, want)
					}
				}
				if sets != 1 || leaves != account.WaitOccurrenceTotal {
					t.Fatalf("published set/leaves = %d/%d, want 1/%d", sets, leaves, account.WaitOccurrenceTotal)
				}
				after, err := json.Marshal(account)
				if err != nil || string(after) != string(before) {
					t.Fatalf("publication mutated the engine account: before=%s after=%s err=%v", before, after, err)
				}
			})
		}
	}
}
