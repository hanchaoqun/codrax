package types

import (
	"encoding/json"
	"testing"
)

func TestAnswerRenderedSurfacesPrivateCopyAndReset(t *testing.T) {
	m := NewMutableState("audit")
	s := AnswerRenderedSurfaces{Answer: "full", Primary: "primary", Principal: "principal"}
	m.SetAnswerRenderedSurfaces(s)
	s.Answer = "changed"
	got := m.AnswerRenderedSurfaces()
	got.Primary = "changed"
	if got := m.AnswerRenderedSurfaces(); got.Answer != "full" || got.Primary != "primary" {
		t.Fatal("shared snapshot")
	}
	if raw, _ := json.Marshal(m.AnswerRenderedSurfaces()); string(raw) != "{}" {
		t.Fatalf("private receipt entered model JSON: %s", raw)
	}
	m.ResetActiveAnswerDocumentV2ForFinalizeDispatch()
	if m.AnswerRenderedSurfaces() != nil {
		t.Fatal("new dispatch retained previous receipt")
	}
	m.SetAnswerRenderedSurfaces(s)
	m.ResetAnswerDocumentV2()
	if m.AnswerRenderedSurfaces() != nil {
		t.Fatal("new task retained previous receipt")
	}
}
