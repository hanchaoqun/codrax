package types

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestNativeMeasurementPromptNotesKeepReaderFieldsAndLedger(t *testing.T) {
	for _, predicate := range []string{"measurements_observation", "thread_timeline_observation", "cpu_frequency_timeline_observation", "future_native_fact"} {
		t.Run(predicate, func(t *testing.T) {
			record := presentationLogRecord("trace-source", 4)
			record.Producer, record.Predicate, record.ClaimKey = "trace_query:run2", predicate, predicate
			record.RichNotes = []string{
				TraceNoteKeyRuntimeMeasurement + `={"version":1,"tables":[]}`,
				TraceNoteKeyRuntimeMeasurement + "=" + strings.Repeat("private publication ", 30),
				"business_stage=texture upload",
				"new_provider_detail=unknown remains unknown",
			}
			before, _ := json.Marshal(record)
			for _, opts := range []ObservationPromptProjectionOptions{DefaultObservationPromptProjectionOptions(1), SemanticReviewObservationPromptProjectionOptions(1)} {
				opts.NoteLimit, opts.PrincipalNoteLimit, opts.OriginSpecificSupportingNoteLimit = 2, 2, 2
				got := ProjectObservationPromptRecords([]ObservationRecord{record}, nil, nil, opts)
				if len(got) != 1 || !reflect.DeepEqual(got[0].Notes, record.RichNotes[2:]) {
					t.Fatalf("private transport consumed note budget or reader notes changed: %+v", got)
				}
			}
			after, _ := json.Marshal(record)
			if string(before) != string(after) {
				t.Fatal("prompt projection removed the original audit receipt")
			}
		})
	}
}

func TestNativeMeasurementPromptNotesOnlyRemoveReservedCarrier(t *testing.T) {
	for _, note := range []string{
		"runtime_measurement_detail=business-owned detail",
		"diagnostic says runtime_measurement={unparsed source text}",
		"unknown_provider_key=retain this exact note",
		"texture upload completion remains unconfirmed",
	} {
		record := presentationLogRecord("trace-source", 4)
		record.Producer, record.RichNotes = "trace_query", []string{note}
		got := ProjectObservationPromptRecords([]ObservationRecord{record}, nil, nil, DefaultObservationPromptProjectionOptions(1))
		if len(got) != 1 || !reflect.DeepEqual(got[0].Notes, []string{note}) {
			t.Fatalf("reserved carrier filter became a prose/unknown-key filter: %q / %+v", note, got)
		}
	}
	for name, alter := range map[string]func(*ObservationRecord){
		"other_producer": func(r *ObservationRecord) { r.Producer = "log_triage" },
		"other_origin":   func(r *ObservationRecord) { r.Origin = AnswerEvidenceOriginExternalDocument },
		"other_source":   func(r *ObservationRecord) { r.SourceRef.Kind = ObservationSourceCurrentSource },
	} {
		t.Run(name, func(t *testing.T) {
			record := presentationLogRecord("trace-source", 4)
			record.Producer = "trace_query"
			record.RichNotes = []string{TraceNoteKeyRuntimeMeasurement + "=ordinary source field"}
			alter(&record)
			got := ProjectObservationPromptRecords([]ObservationRecord{record}, nil, nil, DefaultObservationPromptProjectionOptions(1))
			if len(got) != 1 || !reflect.DeepEqual(got[0].Notes, record.RichNotes) {
				t.Fatalf("non-native source note was silently stripped: %+v", got)
			}
		})
	}
}
