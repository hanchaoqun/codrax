package render

import (
	"strings"
	"testing"
)

// TestAnswerDocumentAcceptedSummary_OptionalCarrierLineKeepsCounts (V2-3,
// §40.19): the executor appends the optional-carrier disclosure on its own
// line after the accepted counts line (and after the citation-ledger delta
// tokens). The accepted-summary renderer must keep binding blocks/citations
// from line one — the disclosure line never re-binds the greedy count regex
// even when its reason quotes a candidate id or an evidence badge.
func TestAnswerDocumentAcceptedSummary_OptionalCarrierLineKeepsCounts(t *testing.T) {
	summary := "emit_answer_document accepted: replace_all blocks=3 citations=2 citations_submitted=3 citations_pruned_unused=1\n" +
		"[optional_carrier_ignored: carrier=trace_root_causes reason=trace_root_causes.root_causes[0].candidate_id \"invented-candidate\" is outside the selectable typed on-chain roster; resend the complete ordered selection as replace_trace_root_causes in the next emit_answer_document_patch; selectable candidate_id: candidate-sched]"
	zh := stripAnsiEscapes(formatAnswerDocumentAcceptedSummary(summary, true))
	if !strings.Contains(zh, "答案草稿已写入：3 个区块 · 2 条引用") {
		t.Fatalf("registered counts must still bind from line one: %q", zh)
	}
	if !strings.Contains(zh, "3 条提交 → 2 条入册") {
		t.Fatalf("citation delta note must survive the extra line: %q", zh)
	}
	en := stripAnsiEscapes(formatAnswerDocumentAcceptedSummary(summary, false))
	if !strings.Contains(en, "Answer draft written: 3 block(s) · 2 citation(s)") {
		t.Fatalf("EN counts must still bind: %q", en)
	}
}
