package types

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestNativeSamplingPromptNotesPreserveLedgerAndReaderNotes(t *testing.T) {
	record := presentationLogRecord("trace-source", 4)
	record.Producer = "trace_query:run2"
	record.Predicate, record.ClaimKey = "perf_sampling_tree_observation", "perf_sampling_tree_observation"
	record.RichNotes = []string{
		TraceNoteKeyPerfSamplingTree + `={"status":"available","cohorts":[]}`,
		TraceNoteKeyPerfSamplingTree + "=" + strings.Repeat("malformed private transport ", 20),
		"perf_sampling_tree_detail=reader-owned detail",
		"diagnostic mentions perf_sampling_tree={ordinary source text}",
	}
	before, _ := json.Marshal(record)
	for _, opts := range []ObservationPromptProjectionOptions{DefaultObservationPromptProjectionOptions(1), SemanticReviewObservationPromptProjectionOptions(1)} {
		opts.NoteLimit, opts.PrincipalNoteLimit, opts.OriginSpecificSupportingNoteLimit = 2, 2, 2
		got := ProjectObservationPromptRecords([]ObservationRecord{record}, nil, nil, opts)
		if len(got) != 1 || !reflect.DeepEqual(got[0].Notes, record.RichNotes[2:]) {
			t.Fatalf("sampling transport consumed note budget or source prose changed: %+v", got)
		}
	}
	after, _ := json.Marshal(record)
	if string(before) != string(after) {
		t.Fatal("sampling projection mutated source ledger")
	}
}

func TestNativeSamplingPromptNotesOnlyRemoveProducerOwnedCarrier(t *testing.T) {
	for name, alter := range map[string]func(*ObservationRecord){
		"other_producer": func(r *ObservationRecord) { r.Producer = "log_triage" },
		"other_origin":   func(r *ObservationRecord) { r.Origin = AnswerEvidenceOriginExternalDocument },
		"other_source":   func(r *ObservationRecord) { r.SourceRef.Kind = ObservationSourceCurrentSource },
	} {
		t.Run(name, func(t *testing.T) {
			record := presentationLogRecord("trace-source", 4)
			record.Producer = "trace_query"
			record.RichNotes = []string{TraceNoteKeyPerfSamplingTree + "=ordinary source field"}
			alter(&record)
			got := ProjectObservationPromptRecords([]ObservationRecord{record}, nil, nil, DefaultObservationPromptProjectionOptions(1))
			if len(got) != 1 || !reflect.DeepEqual(got[0].Notes, record.RichNotes) {
				t.Fatalf("non-native source note stripped: %+v", got)
			}
		})
	}
}
