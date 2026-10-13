package types

import (
	"strings"
	"testing"
)

func TestRuntimeEventMeasurementSortCoordinateIsNotObservedZeroHMC236(t *testing.T) {
	for _, typ := range []string{"measure_interval", "process_measure_interval"} {
		for _, tc := range []struct {
			name                   string
			source, canonical      float64
			sourceKnown, wantKnown bool
		}{
			{"null", 0, 0, false, false}, {"invalid_storage", 0, 0, false, false},
			{"negative", -.000000001, 0, true, false}, {"known_zero", 0, 0, true, true},
			{"known_positive", 1.25, 1.25, true, true}, {"different_axis", 1.25, 10, true, false},
		} {
			t.Run(typ+"/"+tc.name, func(t *testing.T) {
				r := runtimeEventRecordHMC236(t)
				for n := range r.EventSearchInventory.Rows {
					row := &r.EventSearchInventory.Rows[n]
					row.EventType, row.JankEvent = typ, nil
					row.SourceTimeKnown, row.SourceTimeSeconds, row.TraceTimeSeconds = tc.sourceKnown, tc.source, tc.canonical
				}
				c := runtimeEventContractHMC236(t, r)
				s := &AnswerRuntimeMeasurementReceipt{ObservationID: r.ID, View: RuntimeMeasurementMembers, EventSelection: &RuntimeEventSelection{FieldKeys: []string{"event.trace_time_seconds", "source.trace_time_seconds"}}}
				if !BindRuntimeMeasurementReceipt(s, c) {
					t.Fatal("measurement event lost")
				}
				got := s.BoundTable.Rows[0]
				if unknown := strings.HasPrefix(got[0], "未知"); unknown == tc.wantKnown {
					t.Fatalf("sort coordinate elevated/lost: %#v", got)
				}
				if unknown := strings.HasPrefix(got[1], "未知"); unknown == tc.sourceKnown {
					t.Fatalf("source coordinate elevated/lost: %#v", got)
				}
				if tc.name == "known_zero" && (got[0] != "0" || got[1] != "0") {
					t.Fatalf("real zero confused: %#v", got)
				}
				if tc.name == "negative" && got[1] != "-0.000000001" {
					t.Fatalf("signed source timestamp lost: %#v", got)
				}
			})
		}
	}
}

func TestRuntimeEventMalformedJankFieldsStayInvalidHMC236(t *testing.T) {
	for _, reason := range []string{"invalid_field_syntax", "duplicate_field", "invalid_nonnegative_int64", "missing_required_field", "end_before_start"} {
		t.Run(reason, func(t *testing.T) {
			r := runtimeEventRecordHMC236(t)
			for n := range r.EventSearchInventory.Rows {
				r.EventSearchInventory.Rows[n].JankEvent = &TraceEventSearchJankEvent{TimeDomainStatus: TraceJankSourceClock, IssueReason: reason}
			}
			c := runtimeEventContractHMC236(t, r)
			s := &AnswerRuntimeMeasurementReceipt{ObservationID: r.ID, View: RuntimeMeasurementMembers, EventSelection: &RuntimeEventSelection{GroupBy: []string{"jank.appid"}}}
			if !BindRuntimeMeasurementReceipt(s, c) || len(s.BoundTable.Rows) != 1 || !strings.HasPrefix(s.BoundTable.Rows[0][0], "无效") || !strings.Contains(s.BoundTable.Rows[0][0], reason) || s.BoundTable.Rows[0][1] != "4" {
				t.Fatalf("malformed field lost invalid status: %+v", s.BoundTable)
			}
		})
	}
}
