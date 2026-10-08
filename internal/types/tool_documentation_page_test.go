package types

import (
	"strings"
	"testing"
)

func TestToolDocumentationPageIdentityBoundedAndReadBound(t *testing.T) {
	m, rm := documentationCompletionState()
	first := documentationCompletionResult(t, "", 0)
	second := documentationCompletionResult(t, "", 0)
	second.Handoff.Documentation.Selection.Cursor = "v1.catalog.12"
	if toolDocumentationReadKey(first) == toolDocumentationReadKey(second) {
		t.Fatal("different pages collide in read credential identity")
	}
	if got := NormalizeToolHandoffCarriers([]ToolHandoffCarrier{*first.Handoff, *second.Handoff}); len(got) != 2 {
		t.Fatal("page identity lost when content happens to be identical")
	}
	if ToolHandoffCarrierBytes(*second.Handoff)-ToolHandoffCarrierBytes(*first.Handoff) != len(second.Handoff.Documentation.Selection.Cursor) {
		t.Fatal("page cursor bypassed byte accounting")
	}
	stamped := m.StampToolDocumentationResult(first)
	stamped.Handoff.Documentation.Selection.Cursor = second.Handoff.Documentation.Selection.Cursor
	m.AppendDispatchToolResult(stamped)
	if m.ToolDocumentationReady() {
		t.Fatal("mutated page reused an earlier read credential")
	}
	m.AppendDispatchToolResult(m.StampToolDocumentationResult(first))
	m.AppendDispatchToolResult(m.StampToolDocumentationResult(second))
	sealDocumentationCompletion(t, m, &rm)
	if len(m.AcceptedToolDocumentationCarriers()) != 2 {
		t.Fatal("genuine separate page reads collided")
	}
	if chunk := ToolDocumentationPromptChunk(*second.Handoff); !strings.Contains(chunk, `page cursor: "v1.catalog.12"`) {
		t.Fatal("prompt omitted the selected page identity")
	}
	doc := *second.Handoff.Documentation
	doc.Selection.Cursor = strings.Repeat("x", 257)
	if _, ok := NormalizeToolDocumentation(doc); ok {
		t.Fatal("unbounded cursor admitted")
	}
}
