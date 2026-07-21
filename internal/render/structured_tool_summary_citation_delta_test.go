package render

import (
	"strings"
	"testing"
)

// TestAnswerDocumentAcceptedSummary_CitationDeltaDisclosure pins the
// §29.174 RUN2AUDIT-1 F6 disclosure face against the runnable_2.txt
// witness: the tool call advertised "citations=5", the acceptance line
// said "0 条引用", and nothing explained the drop (runtime-artifact
// references are deliberately rerouted through the E# evidence index).
// With the executor's typed delta tokens present, the acceptance line
// must word the submitted→registered delta and the true reroute reason.
func TestAnswerDocumentAcceptedSummary_CitationDeltaDisclosure(t *testing.T) {
	zh := stripAnsiEscapes(formatAnswerDocumentAcceptedSummary(
		"emit_answer_document accepted: replace_all blocks=3 citations=0 citations_submitted=5 citations_redirected_runtime=5", true))
	if !strings.Contains(zh, "答案草稿已写入：3 个区块 · 0 条引用") {
		t.Fatalf("registered counts must stay on the line (regex must not re-bind to the delta tokens): %q", zh)
	}
	if !strings.Contains(zh, "5 条提交 → 0 条入册") {
		t.Fatalf("submitted→registered delta missing: %q", zh)
	}
	if !strings.Contains(zh, "运行时工件引用改走 E# 证据索引") {
		t.Fatalf("runtime-artifact reroute reason missing: %q", zh)
	}

	en := stripAnsiEscapes(formatAnswerDocumentAcceptedSummary(
		"emit_answer_document accepted: replace_all blocks=3 citations=0 citations_submitted=5 citations_redirected_runtime=5", false))
	if !strings.Contains(en, "5 submitted → 0 registered") ||
		!strings.Contains(en, "rerouted via the E# evidence index") {
		t.Fatalf("EN delta disclosure missing: %q", en)
	}

	// Mixed reasons surface individually, honest per drop point.
	mixed := stripAnsiEscapes(formatAnswerDocumentAcceptedSummary(
		"emit_answer_document accepted: replace_all blocks=3 citations=1 citations_submitted=5 citations_rejected_form=2 citations_pruned_unused=2", true))
	if !strings.Contains(mixed, "非源码行号形引用被拒") || !strings.Contains(mixed, "未被条目引用的池项已清理") {
		t.Fatalf("per-reason wording missing: %q", mixed)
	}

	// Delta present but reasons unattributed (persist-chain residue):
	// generic logs pointer, never a guessed reason.
	generic := stripAnsiEscapes(formatAnswerDocumentAcceptedSummary(
		"emit_answer_document accepted: replace_all blocks=3 citations=2 citations_submitted=5", true))
	if !strings.Contains(generic, "处置明细见日志") {
		t.Fatalf("unattributed delta must fall back to the logs pointer: %q", generic)
	}
	if strings.Contains(generic, "证据索引") {
		t.Fatalf("unattributed delta must not guess a reroute reason: %q", generic)
	}
}

// TestAnswerDocumentAcceptedSummary_NoDeltaTokenByteStable is the
// negative arm: without the executor's delta token (delta == 0 never
// emits one), the acceptance line stays byte-identical to the
// pre-§29.174 form — no note, no noise.
func TestAnswerDocumentAcceptedSummary_NoDeltaTokenByteStable(t *testing.T) {
	line := stripAnsiEscapes(formatAnswerDocumentAcceptedSummary(
		"emit_answer_document accepted: replace_all blocks=4 citations=7", true))
	if !strings.Contains(line, "答案草稿已写入：4 个区块 · 7 条引用") {
		t.Fatalf("baseline acceptance line broken: %q", line)
	}
	for _, forbidden := range []string{"提交", "入册", "（"} {
		if strings.Contains(line, forbidden) {
			t.Fatalf("token-less summary must not grow a delta note (%q leaked): %q", forbidden, line)
		}
	}
	// Token present but equal counts (defensive): still no note.
	equal := stripAnsiEscapes(formatAnswerDocumentAcceptedSummary(
		"emit_answer_document accepted: replace_all blocks=4 citations=7 citations_submitted=7", true))
	if strings.Contains(equal, "入册") {
		t.Fatalf("equal submitted/registered must not annotate: %q", equal)
	}
}
