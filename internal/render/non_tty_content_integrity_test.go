package render

import (
	"bytes"
	"strings"
	"testing"
)

// Content rows are occurrences, not notices: equal display text need not mean
// the same record. In particular, a rejected draft must remain auditable even
// when the model itself has duplicated a table row.
func TestRendererNonTTYRejectedDraftPreservesRepeatedContent(t *testing.T) {
	params := `{"blocks":[
		{"id":"text","kind":"summary","text":"Repeated prose\nRepeated prose\n\n\nAfter blanks"},
		{"id":"table","kind":"table","columns":["Owner","Value"],"items":[
			{"id":"occurrence-1","cells":["owner-a","same value"]},
			{"id":"occurrence-2","cells":["owner-a","same value"]},
			{"id":"occurrence-3","cells":["owner-b","other value"]},
			{"id":"occurrence-4","cells":["owner-a","same value"]}]},
		{"id":"diagram","kind":"diagram","diagram":{"language":"mermaid","body":"sequenceDiagram\nparticipant A\nNote over A: repeated observation\nNote over A: repeated observation"}}
	]}`
	for _, trigger := range []string{"tool_rejection", "preview_rejection", "patch_after_rejection"} {
		t.Run(trigger, func(t *testing.T) {
			var buf bytes.Buffer
			r := New(&buf, false)
			r.SetOutput(&buf)
			emit := r.Emitter()
			emit(Event{Kind: EventToolCallEnd, ToolName: "emit_answer_document", ToolOK: trigger != "tool_rejection", ToolParamsJSON: params})
			switch trigger {
			case "preview_rejection":
				emit(Event{Kind: EventLivePreviewClear, PreviewRejected: true})
			case "patch_after_rejection":
				emit(Event{Kind: EventToolCallEnd, ToolName: "emit_answer_document_patch", ToolOK: true})
			}
			out := stripAnsiEscapes(buf.String())
			for text, count := range map[string]int{
				"Repeated prose":                    2,
				"| owner-a | same value |":          3,
				"Note over A: repeated observation": 2,
			} {
				if got := strings.Count(out, text); got != count {
					t.Errorf("draft changed occurrence count for %q: got %d, want %d\n%s", text, got, count, out)
				}
			}
			// Equality checks ordering and layout too, not merely a count of rows.
			want := stripAnsiEscapes(formatScrollbackBody(formatAnswerDocumentDraftPreviewLines(params, "zh"), false))
			if !strings.Contains(out, want) {
				t.Errorf("non-TTY preview changed the complete formatted draft\nwant:\n%s\ngot:\n%s", want, out)
			}
		})
	}
}

func TestRendererNonTTYContentPreservesStatusNoticeDeduplication(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false)
	r.SetOutput(&buf)
	emit := r.Emitter()
	notice := Event{Kind: EventOrchestratorNotice, NoticeKind: NoticeRetry, Reasoning: "same status notice"}
	emit(notice)
	emit(notice)
	if got := strings.Count(buf.String(), "same status notice"); got != 1 {
		t.Fatalf("consecutive status notices should still deduplicate, got %d: %s", got, buf.String())
	}
	emit(Event{Kind: EventToolCallEnd, ToolName: "emit_answer_document", ToolParamsJSON: `{"blocks":[{"id":"summary","kind":"summary","text":"reader content"}]}`})
	emit(notice)
	emit(notice)
	if got := strings.Count(buf.String(), "same status notice"); got != 2 {
		t.Fatalf("content separates notice runs; only consecutive notices should deduplicate, got %d: %s", got, buf.String())
	}
}
