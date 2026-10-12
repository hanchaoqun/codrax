package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestBusinessDiagramTransportDoesNotConsumePromptNotes(t *testing.T) {
	r := presentationLogRecord("trace-source", 4)
	r.Producer = "trace_query"
	r.RichNotes = []string{TraceNoteKeyBusinessDiagramAnnotation + "={malformed transport}", "business_diagram_annotation_v1_detail=source field", "text mentions business_diagram_annotation_v1={literal}"}
	before, _ := json.Marshal(r)
	got := nativePresentationNoteRecord(r)
	if !reflect.DeepEqual(got.RichNotes, r.RichNotes[1:]) {
		t.Fatal("private transport leaked or literal source note changed")
	}
	after, _ := json.Marshal(r)
	if string(before) != string(after) {
		t.Fatal("projection mutated raw ledger")
	}
	r.Producer = "log_triage"
	if !reflect.DeepEqual(nativePresentationNoteRecord(r).RichNotes, r.RichNotes) {
		t.Fatal("unrelated producer source text stripped")
	}
}
