package types

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

const waitPrevStateTestNote = "target_wait_occurrence_prev_state_raw"

func waitPrevStateTestRows(scope, raw string) []ObservationRecord {
	rows := b1618p2aWaitRows(scope, "/capture/raw-state.ftrace", 10, 10.02, .3)
	if raw == "" {
		return rows
	}
	for i, note := range rows[0].RichNotes {
		if strings.HasPrefix(note, TraceNoteKeyTargetWaitOccurrence+"=") {
			rows[0].RichNotes[i] += " prev_state_raw=" + raw
		}
	}
	rows[1].RichNotes = append(rows[1].RichNotes, waitPrevStateTestNote+"="+raw)
	return rows
}

func waitPrevStateTestField(t *testing.T, row TargetWaitOccurrenceAuthorityRow) string {
	t.Helper()
	field := reflect.ValueOf(row).FieldByName("PrevStateRaw")
	if !field.IsValid() || field.Kind() != reflect.String {
		t.Error("authority row drops the optional original scheduler state")
		return ""
	}
	return field.String()
}

func TestTargetWaitPrevStatePreservesIdentityAndLegacy(t *testing.T) {
	for _, raw := range []string{"", "D", "D|K", "S"} {
		t.Run(fmt.Sprintf("raw_%q", raw), func(t *testing.T) {
			rows := waitPrevStateTestRows("single", raw)
			before := fmt.Sprintf("%#v", rows)
			full := BuildTraceTargetWaitSummaryAuthorities(ObservationLedger{Records: rows}, b1618p2aWaitRequest())
			preview := BuildTargetWaitOccurrenceAuthorities(ObservationLedger{Records: rows}, b1618p2aWaitRequest())
			if len(full) != 1 || len(preview) != 1 || full[0].Count != 1 || full[0].DStateOccurrences != 1 || full[0].WallClockMS != .3 || preview[0].SumMS != .3 || len(full[0].SourceRecordIDs) != 2 || len(preview[0].SourceRecordIDs) != 2 {
				t.Fatalf("optional metadata changed the original authority: full=%+v preview=%+v", full, preview)
			}
			for _, row := range []TargetWaitOccurrenceAuthorityRow{full[0].Occurrences[0], preview[0].Rows[0]} {
				if got := waitPrevStateTestField(t, row); got != raw {
					t.Errorf("original state=%q want %q", got, raw)
				}
				want := "#1 state=d_sleep 10.001000..10.001300 duration=0.300ms iowait=0 caller=wait_call"
				if row.CanonicalLine() != want {
					t.Errorf("metadata changed canonical identity: %q", row.CanonicalLine())
				}
				display, ok := any(row).(interface{ DisplayLine() string })
				if !ok {
					t.Error("missing separate occurrence DisplayLine method")
					continue
				}
				if raw != "" {
					want += " prev_state_raw=" + raw
				}
				if display.DisplayLine() != want {
					t.Errorf("display=%q want %q", display.DisplayLine(), want)
				}
			}
			if before != fmt.Sprintf("%#v", rows) {
				t.Fatal("compilation mutated observations")
			}
		})
	}
}

func TestTargetWaitPrevStateScopeMergeIsOrderIndependent(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  []string
		want string
	}{
		{"missing_is_not_conflict", []string{"", "D"}, "D"},
		{"equal", []string{"D", "D"}, "D"},
		{"conflict", []string{"D", "S"}, ""},
		{"sticky_conflict", []string{"D", "S", "D"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var records []ObservationRecord
			for i, raw := range tc.raw {
				records = append(records, waitPrevStateTestRows(fmt.Sprintf("scope%d", i), raw)...)
			}
			for _, reverse := range []bool{false, true} {
				if reverse {
					for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
						records[i], records[j] = records[j], records[i]
					}
				}
				full := BuildTraceTargetWaitSummaryAuthorities(ObservationLedger{Records: records}, b1618p2aWaitRequest())
				preview := BuildTargetWaitOccurrenceAuthorities(ObservationLedger{Records: records}, b1618p2aWaitRequest())
				if len(full) != 1 || len(preview) != 1 || full[0].Count != 1 || full[0].WallClockMS != .3 || full[0].DStateOccurrences != 1 || preview[0].SumMS != .3 || len(full[0].SourceRecordIDs) != len(records) || len(preview[0].SourceRecordIDs) != len(records) {
					t.Fatalf("metadata changed scope admission or receipts: full=%+v preview=%+v", full, preview)
				}
				for _, row := range []TargetWaitOccurrenceAuthorityRow{full[0].Occurrences[0], preview[0].Rows[0]} {
					if got := waitPrevStateTestField(t, row); got != tc.want {
						t.Errorf("reverse=%t raw=%q want=%q", reverse, got, tc.want)
					}
				}
			}
		})
	}
}

func TestTargetWaitPrevStateSameIDConflictKeepsMeasurementNotShadowPermission(t *testing.T) {
	a, b := waitPrevStateTestRows("same-id", "D"), waitPrevStateTestRows("same-id", "S")
	for _, records := range [][]ObservationRecord{append(a, b...), append(b, a...)} {
		full := BuildTraceTargetWaitSummaryAuthorities(ObservationLedger{Records: records}, b1618p2aWaitRequest())
		preview := BuildTargetWaitOccurrenceAuthorities(ObservationLedger{Records: records}, b1618p2aWaitRequest())
		if len(full) != 1 || len(preview) != 1 || full[0].Count != 1 || full[0].WallClockMS != .3 || preview[0].SumMS != .3 {
			t.Fatalf("optional conflict suppressed measurement: full=%+v preview=%+v", full, preview)
		}
		if len(full[0].SourceRecordIDs) != 0 || len(preview[0].SourceRecordIDs) != 0 {
			t.Fatal("non-identical same-ID metadata gained reader-shadow authority")
		}
		for _, row := range []TargetWaitOccurrenceAuthorityRow{full[0].Occurrences[0], preview[0].Rows[0]} {
			if got := waitPrevStateTestField(t, row); got != "" {
				t.Errorf("conflict chose first-seen physical state %q", got)
			}
		}
	}
}

func TestTargetWaitPrevStateWithinRecordAndLeafConflicts(t *testing.T) {
	rows := waitPrevStateTestRows("one", "D")
	rows[1].RichNotes = append(rows[1].RichNotes, waitPrevStateTestNote+"=S")
	for i, note := range rows[0].RichNotes {
		if strings.HasPrefix(note, TraceNoteKeyTargetWaitOccurrence+"=") {
			rows[0].RichNotes[i] += " prev_state_raw=S"
		}
	}
	full := BuildTraceTargetWaitSummaryAuthorities(ObservationLedger{Records: rows}, b1618p2aWaitRequest())
	preview := BuildTargetWaitOccurrenceAuthorities(ObservationLedger{Records: rows}, b1618p2aWaitRequest())
	if len(full) != 1 || len(preview) != 1 || full[0].WallClockMS != .3 || preview[0].SumMS != .3 {
		t.Fatalf("optional repeated field invalidated base roster: full=%+v preview=%+v", full, preview)
	}
	for _, row := range []TargetWaitOccurrenceAuthorityRow{full[0].Occurrences[0], preview[0].Rows[0]} {
		if got := waitPrevStateTestField(t, row); got != "" {
			t.Errorf("conflicting optional tokens chose %q", got)
		}
	}
}

func TestTargetWaitPrevStatePreviewMustBeQualifiedAndWholeRosterMatched(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func([]ObservationRecord)
		want   string
	}{
		{"qualified_conflict", func([]ObservationRecord) {}, ""},
		{"invalid_status", func(r []ObservationRecord) {
			r[0].RichNotes[1] = strings.ReplaceAll(r[0].RichNotes[1], "status=complete", "status=invalid")
		}, "D"},
		{"incomplete", func(r []ObservationRecord) {
			r[0].RichNotes[1] = strings.ReplaceAll(r[0].RichNotes[1], "status=complete", "status=incomplete")
		}, "D"},
		{"wrong_total", func(r []ObservationRecord) {
			r[0].RichNotes[1] = strings.ReplaceAll(r[0].RichNotes[1], "total=1", "total=2")
		}, "D"},
		{"wrong_sum", func(r []ObservationRecord) {
			r[0].RichNotes[len(r[0].RichNotes)-1] = TraceNoteKeyTargetWaitOccurrencePromptSum + "=0.400"
		}, "D"},
		{"wrong_interval", func(r []ObservationRecord) {
			r[0].RichNotes[2] = strings.ReplaceAll(r[0].RichNotes[2], "10.001000", "10.001001")
		}, "D"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := waitPrevStateTestRows("one", "D")
			rows[0].RichNotes[2] = strings.ReplaceAll(rows[0].RichNotes[2], "prev_state_raw=D", "prev_state_raw=S")
			tc.mutate(rows)
			full := BuildTraceTargetWaitSummaryAuthorities(ObservationLedger{Records: rows}, b1618p2aWaitRequest())
			if len(full) != 1 || full[0].Count != 1 || full[0].WallClockMS != .3 {
				t.Fatalf("preview metadata changed full admission: %+v", full)
			}
			if got := waitPrevStateTestField(t, full[0].Occurrences[0]); got != tc.want {
				t.Errorf("full raw=%q want=%q", got, tc.want)
			}
			if tc.name == "qualified_conflict" {
				preview := BuildTargetWaitOccurrenceAuthorities(ObservationLedger{Records: rows}, b1618p2aWaitRequest())
				if len(preview) != 1 || waitPrevStateTestField(t, preview[0].Rows[0]) != "" || len(preview[0].SourceRecordIDs) != 2 {
					t.Fatalf("cross-path conflict must disappear on both displays without changing receipts: %+v", preview)
				}
			}
		})
	}
}

func TestTargetWaitPrevStateMetadataConflictPropagatesWithoutUnsafeReceipt(t *testing.T) {
	for _, secondRaw := range []string{"", "D", "S"} {
		a, b := waitPrevStateTestRows("same", "D"), waitPrevStateTestRows("same", secondRaw)
		// The compact D conflicts with its own full S. The duplicate set has
		// the same measurement but a non-identical ID, so its receipt is unsafe.
		a[1].RichNotes[1] = waitPrevStateTestNote + "=S"
		b[0].Summary = "different optional summary"
		for _, records := range [][]ObservationRecord{append(a, b...), append(b, a...)} {
			full := BuildTraceTargetWaitSummaryAuthorities(ObservationLedger{Records: records}, b1618p2aWaitRequest())
			preview := BuildTargetWaitOccurrenceAuthorities(ObservationLedger{Records: records}, b1618p2aWaitRequest())
			if len(full) != 1 || len(preview) != 1 || full[0].WallClockMS != .3 || preview[0].SumMS != .3 {
				t.Fatalf("optional metadata changed measurement: full=%+v preview=%+v", full, preview)
			}
			for _, row := range []TargetWaitOccurrenceAuthorityRow{full[0].Occurrences[0], preview[0].Rows[0]} {
				if got := waitPrevStateTestField(t, row); got != "" {
					t.Errorf("unsafe ID blocked conflict propagation, displaying %q", got)
				}
			}
			for _, ids := range [][]string{full[0].SourceRecordIDs, preview[0].SourceRecordIDs} {
				for _, id := range ids {
					if id == a[0].ID {
						t.Fatal("metadata transport authorized unsafe aggregate hiding")
					}
				}
			}
		}
	}
}

func TestTargetWaitPrevStateElevenLeavesSurviveIncompletePreview(t *testing.T) {
	rows := b1618p2aWaitRows("eleven", "/capture/raw-state.ftrace", 10, 10.02, .3, .3, .3, .3, .3, .3, .3, .3, .3, .3, .3)
	rows[0].RichNotes[1] = TraceNoteKeyTargetWaitOccurrencePrompt + "=status=incomplete,emitted=8,total=11"
	for i := 1; i < len(rows); i++ {
		rows[i].RichNotes = append(rows[i].RichNotes, waitPrevStateTestNote+"=D")
	}
	for i := 2; i < 10; i++ {
		rows[0].RichNotes[i] += " prev_state_raw=S"
	}
	rows[0].RichNotes = append(rows[0].RichNotes[:10], TraceNoteKeyTargetWaitOccurrencePromptSum+"=2.400")
	full := BuildTraceTargetWaitSummaryAuthorities(ObservationLedger{Records: rows}, b1618p2aWaitRequest())
	if len(full) != 1 || len(full[0].Occurrences) != 11 || full[0].Count != 11 || full[0].DStateOccurrences != 11 || fmt.Sprintf("%.3f", full[0].WallClockMS) != "3.300" {
		t.Fatalf("full roster capped or changed: %+v", full)
	}
	for _, row := range full[0].Occurrences {
		if got := waitPrevStateTestField(t, row); got != "D" {
			t.Errorf("incomplete preview altered full native state on row %d: %q", row.Ordinal, got)
		}
	}
	if preview := BuildTargetWaitOccurrenceAuthorities(ObservationLedger{Records: rows}, b1618p2aWaitRequest()); len(preview) != 0 {
		t.Fatal("metadata upgraded an incomplete eight-row preview")
	}
}

func TestTargetWaitPrevStateKnownAndLegacySameIDDoNotRestoreShadowPermission(t *testing.T) {
	a, b := waitPrevStateTestRows("same", ""), waitPrevStateTestRows("same", "D")
	for _, records := range [][]ObservationRecord{append(a, b...), append(b, a...)} {
		full := BuildTraceTargetWaitSummaryAuthorities(ObservationLedger{Records: records}, b1618p2aWaitRequest())
		preview := BuildTargetWaitOccurrenceAuthorities(ObservationLedger{Records: records}, b1618p2aWaitRequest())
		if len(full) != 1 || len(preview) != 1 || full[0].Count != 1 || full[0].WallClockMS != .3 || preview[0].SumMS != .3 {
			t.Fatalf("legacy metadata changed measurement: full=%+v preview=%+v", full, preview)
		}
		if len(full[0].SourceRecordIDs) != 0 || len(preview[0].SourceRecordIDs) != 0 {
			t.Fatal("known metadata restored unsafe same-ID shadow permission")
		}
		for _, row := range []TargetWaitOccurrenceAuthorityRow{full[0].Occurrences[0], preview[0].Rows[0]} {
			if got := waitPrevStateTestField(t, row); got != "D" {
				t.Errorf("missing+known state=%q want D", got)
			}
		}
	}
}

func TestTargetWaitPrevStateReceiptNeverBorrowsNeighborOrOtherResult(t *testing.T) {
	base := waitPrevStateTestRows("source", "D")[0]
	receipt := newTargetWaitOccurrenceRawReceipt(base)
	for _, tc := range []struct {
		name   string
		mutate func(*ObservationRecord)
		want   bool
	}{
		{"exact", func(*ObservationRecord) {}, true},
		{"adjacent_microsecond", func(r *ObservationRecord) { r.RichNotes = []string{"selected_window=10.000001..10.020001"} }, false},
		{"other_capture", func(r *ObservationRecord) { r.SourceRef.Path = "/other/raw-state.ftrace" }, false},
		{"other_result", func(r *ObservationRecord) { r.SourceRef.PayloadRef = "/payload/other.json" }, false},
		{"other_execution", func(r *ObservationRecord) { r.ObservedAt = "2026-09-08T10:00:01Z" }, false},
		{"other_target", func(r *ObservationRecord) { r.Subject = "other-200" }, false},
		{"other_set", func(r *ObservationRecord) { r.ID += "-other" }, false},
		{"unknown_query", func(r *ObservationRecord) { r.RichNotes = nil }, false},
		{"unknown_result", func(r *ObservationRecord) { r.SourceRef.PayloadRef = ""; r.SourceRef.RawRef = "" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			other := base
			tc.mutate(&other)
			if got := receipt.matches(other); got != tc.want {
				t.Errorf("metadata-only receipt match=%t want=%t", got, tc.want)
			}
		})
	}
}
