package types

import "testing"

// answer_document_rewrite_keeps_sidecar_test.go — §40.29.1 ★19: a system-side
// rewrite of the accepted answer keeps its sibling typed artifacts; a model
// emit commit still owns (and therefore resets) them.
func TestRewriteAcceptedAnswerDocumentKeepsSidecarButModelCommitResetsIt(t *testing.T) {
	m := NewMutableState("trace")
	doc := &AnswerDocumentV2{DocumentModel: "v2", Blocks: []AnswerBlock{{ID: "s", Kind: BlockSummary, Text: "accepted"}}}
	m.SetAnswerDocumentV2WithMutation(MutationReplaceAll, doc)
	report := &TraceRootCauseReportV2{SchemaVersion: TraceRootCauseReportSchemaVersion,
		RootCauses: []*TraceRootCauseItemV2{{CandidateID: "c1", Category: TraceRootCauseIOBlocking, ThreadName: "worker",
			ImpactCaliber: TraceImpactCaliberEffectiveAttribution, CausalQualifier: TraceCausalQualifierProven}}}
	m.SetTraceRootCauseReport(report)
	repaired := &AnswerDocumentV2{DocumentModel: "v2", Blocks: []AnswerBlock{{ID: "s", Kind: BlockSummary, Text: "repaired"}}}
	m.RewriteAcceptedAnswerDocumentV2(repaired)
	if got := m.AnswerDocumentV2(); got == nil || got.Blocks[0].Text != "repaired" {
		t.Fatalf("rewrite must commit the repaired document: %+v", got)
	}
	if got := m.TraceRootCauseReport(); got == nil || len(got.RootCauses) != 1 || got.RootCauses[0].CandidateID != "c1" {
		t.Fatalf("system-side rewrite must keep the accepted sidecar selection: %+v", got)
	}
	m.SetAnswerDocumentV2WithMutation(MutationReplaceAll, repaired)
	if got := m.TraceRootCauseReport(); got != nil {
		t.Fatalf("a model emit commit owns the selector and resets the stored sidecar: %+v", got)
	}
}
