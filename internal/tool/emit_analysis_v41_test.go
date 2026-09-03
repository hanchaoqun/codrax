package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// emit_analysis_v41_test.go — V4-1 (colleague_merge_audit §40.9 → §40.34): the
// required-diagram "missing participant" HARD arm and its normalizer siblings
// judge identity membership through ONE whole-surface authority
// (entityNamedInQuote), never an unbounded substring (`Reader` ⊂ `FileReader`
// used to hard-reject a valid slate — the T3-2 class, fifth instance).

func TestEntityNamedInQuote(t *testing.T) {
	for _, tc := range []struct {
		quote, entity string
		want          bool
	}{
		{"FileReader 到 Parser 的数据流", "Reader", false},
		{"JSONParser 与 Cache", "Parser", false},
		{"CacheStore 与 Parser", "Cache", false},
		{"pkg.Reader 与 Parser", "Reader", false},
		{"emit_answer_document_patch", "emit_answer_document", false},
		{"Reader 与 Parser 的数据流", "Reader", true},
		{"Reader与Parser的数据流", "Reader", true},
		{"Reader（Parser）", "Reader", true},
		{"Mutable/BusContext 之间", "Mutable", true},
		{"Mutable/BusContext 之间", "BusContext", true},
		{"EmitAnswerDocument与Parser", "emit_answer_document", true},
		{"analyzer、explorer", "Explorer", true},
	} {
		if got := entityNamedInQuote(tc.quote, tc.entity); got != tc.want {
			t.Fatalf("entityNamedInQuote(%q, %q)=%v want %v", tc.quote, tc.entity, got, tc.want)
		}
	}
}

func TestReconcileDiagramParticipantsClosedScopeUsesWholeWordAuthority(t *testing.T) {
	hint := &types.DiagramHint{Required: true, RelationScopeQuote: "FileReader 的数据流",
		Participants: []types.DiagramParticipantHint{
			{Identity: "FileReader", Role: "incident_required", SourceQuote: "FileReader"},
			{Identity: "Parser", Role: "incident_required", SourceQuote: "Parser"},
		}}
	got, warning := reconcileDiagramParticipantsWithClosedRelationScope(hint, []string{"Reader", "FileReader", "Parser"})
	// Only ONE scope-named entity (FileReader) — the closed-surface trim must
	// not fire from the substring hit `Reader` ⊂ `FileReader`.
	if got == nil || len(got.Participants) != 2 || warning != "" {
		t.Fatalf("substring inflation of the closed scope must not drop a participant: %+v warning=%q", got, warning)
	}
}

func v41FlowPayload(scope string, participants string) string {
	return `{
		"intent":"explain",
		"scenario":"architecture_explain",
		"complexity":"moderate",
		"keywords":["FileReader","Parser","数据流"],
		"entities":["Reader","FileReader","Parser"],
		"question_kind":"mechanism",
		"predicate_axis":"flow",
		"diagram_hint":{"kind":"flow","required":true,"relation_scope_quote":"` + scope + `","participants":[` + participants + `]}
	}`
}

// The omitted entity `Reader` is only a substring of the scope-named
// `FileReader`: a whole-surface authority admits the slate.
func TestEmitAnalysis_Execute_OmittedScopeEntityUsesWholeWordAuthority(t *testing.T) {
	raw := "解释 FileReader 到 Parser 的数据流，并画出流程图"
	mu := types.NewMutableState(raw)
	payload := v41FlowPayload("FileReader 到 Parser 的数据流",
		`{"identity":"FileReader","role":"incident_required","source_quote":"FileReader"},{"identity":"Parser","role":"incident_required","source_quote":"Parser"}`)
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu, PresentationDirective: "Mermaid flow diagram", PresentationDiagramRequired: true}, json.RawMessage(withV4Required(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success || strings.Contains(res.Summary, "no matching participant row remains") {
		t.Fatalf("`Reader` is not named by the scope quote as a whole surface; the slate must be accepted: %+v", res)
	}
	if mu.RequestModel() == nil {
		t.Fatal("accepted analysis must publish the RequestModel")
	}
}

// A standalone scope entity still needs its participant row (fidelity of the
// hard arm is unchanged).
func TestEmitAnalysis_Execute_StandaloneScopeEntityStillRequiresRow(t *testing.T) {
	raw := "解释 Reader 与 Parser 的数据流，并画出流程图"
	mu := types.NewMutableState(raw)
	payload := v41FlowPayload("Reader 与 Parser 的数据流",
		`{"identity":"Parser","role":"incident_required","source_quote":"Parser"}`)
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu, PresentationDirective: "Mermaid flow diagram", PresentationDiagramRequired: true}, json.RawMessage(withV4Required(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Success || !strings.Contains(res.Summary, "identity/entities [Reader] but no matching participant row remains") {
		t.Fatalf("a whole-surface scope entity without a row must still be rejected: %+v", res)
	}
	if mu.RequestModel() != nil {
		t.Fatal("a rejected slate must not publish a RequestModel")
	}
}
