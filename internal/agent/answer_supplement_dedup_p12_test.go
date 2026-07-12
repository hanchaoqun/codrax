package agent

// answer_supplement_dedup_p12_test.go — CR-3 件④ P12 duplicate-block gate
// pins (§29.42 P12, 2026-07-12; 冷读 witness: the trace_query observation
// supplement rendered verbatim twice back-to-back, 行 1185-1235 =
// 1241-1291). The single last-mile append chokepoint suppresses a >K-line
// verbatim duplicate and renders the 同上略 note instead; short segments
// and distinct segments are never touched (model prose is never rewritten
// — this gate governs the system's own deterministic surfaces only).

import (
	"fmt"
	"strings"
	"testing"
)

func p12ObservationBlock() string {
	lines := []string{"---", "", "> **系统补充：trace_query 关键观测核对**", ">"}
	for i := 0; i < 12; i++ {
		lines = append(lines, "> - 观测行 "+strings.Repeat("x", i+1))
	}
	return strings.Join(lines, "\n")
}

// TestAnswerSupplementDedup_SecondVerbatimCopySuppressed — the witness
// shape: the SAME >K-line supplement appended twice renders once plus the
// 同上略 note.
func TestAnswerSupplementDedup_SecondVerbatimCopySuppressed(t *testing.T) {
	block := p12ObservationBlock()
	prose := appendAnswerSupplementDeduped("正文回答。", block, "zh")
	if strings.Count(prose, "trace_query 关键观测核对") != 1 {
		t.Fatalf("first append must render the block:\n%s", prose)
	}
	prose = appendAnswerSupplementDeduped(prose, block, "zh")
	if got := strings.Count(prose, "trace_query 关键观测核对"); got != 1 {
		t.Fatalf("second verbatim copy must be suppressed, block appears %d times:\n%s", got, prose)
	}
	if !strings.Contains(prose, "同上略") {
		t.Fatalf("the suppressed copy must leave the 同上略 note:\n%s", prose)
	}
	// EN face wording.
	proseEN := appendAnswerSupplementDeduped(
		appendAnswerSupplementDeduped("Answer body.", block, "en"), block, "en")
	if !strings.Contains(proseEN, "Same as above, omitted") {
		t.Fatalf("EN note missing:\n%s", proseEN)
	}
}

// TestAnswerSupplementDedup_ShortSegmentsNeverDeduped — a segment at or
// under the K-line floor appends verbatim even when repeated (incidental
// substring matches must not eat legitimate short supplements).
func TestAnswerSupplementDedup_ShortSegmentsNeverDeduped(t *testing.T) {
	short := "---\n\n> **系统补充：阶段绑定核对**\n> - 单行"
	prose := appendAnswerSupplementDeduped("正文。", short, "zh")
	prose = appendAnswerSupplementDeduped(prose, short, "zh")
	if got := strings.Count(prose, "阶段绑定核对"); got != 2 {
		t.Fatalf("short segments stay out of the dedup gate, got %d copies:\n%s", got, prose)
	}
	if strings.Contains(prose, "同上略") {
		t.Fatalf("short segments must not mint the note:\n%s", prose)
	}
}

// TestAnswerSupplementDedup_DistinctBlocksBothRender — distinct >K-line
// segments both render (the gate is verbatim-identity keyed, never
// similarity keyed).
func TestAnswerSupplementDedup_DistinctBlocksBothRender(t *testing.T) {
	a := p12ObservationBlock()
	b := strings.Replace(a, "观测行 x", "观测行 y", 1)
	prose := appendAnswerSupplementDeduped("正文。", a, "zh")
	prose = appendAnswerSupplementDeduped(prose, b, "zh")
	if strings.Contains(prose, "同上略") {
		t.Fatalf("distinct blocks must both render:\n%s", prose)
	}
	if strings.Count(prose, "trace_query 关键观测核对") != 2 {
		t.Fatalf("both distinct blocks must be present:\n%s", prose)
	}
}

// TestAnswerSupplementDedup_EmptyAndFirstUse — empty supplements are
// no-ops; a supplement onto an empty answer becomes the answer.
func TestAnswerSupplementDedup_EmptyAndFirstUse(t *testing.T) {
	if got := appendAnswerSupplementDeduped("正文。", "   ", "zh"); got != "正文。" {
		t.Fatalf("empty supplement must be a no-op, got %q", got)
	}
	block := p12ObservationBlock()
	if got := appendAnswerSupplementDeduped("", block, "zh"); got != strings.TrimSpace(block) {
		t.Fatalf("first supplement onto an empty answer becomes the answer")
	}
}

// TestAnswerSupplementDedup_KLineBoundary — 修复轮 P3②: the K threshold is
// exact — an 8-line (==K) duplicate never dedups, a 9-line (>K) duplicate
// always does.
func TestAnswerSupplementDedup_KLineBoundary(t *testing.T) {
	block := func(lines int) string {
		out := []string{"---", "", "> **系统补充：边界核对**"}
		for i := len(out); i < lines; i++ {
			out = append(out, fmt.Sprintf("> - 行 %d", i))
		}
		return strings.Join(out, "\n")
	}
	eight := block(answerSupplementDuplicateMinLines)
	if got := strings.Count(eight, "\n") + 1; got != 8 {
		t.Fatalf("fixture drifted: want 8 lines, got %d", got)
	}
	prose := appendAnswerSupplementDeduped(appendAnswerSupplementDeduped("正文。", eight, "zh"), eight, "zh")
	if strings.Count(prose, "边界核对") != 2 || strings.Contains(prose, "同上略") {
		t.Fatalf("an ==K-line duplicate stays out of the gate:\n%s", prose)
	}
	nine := block(answerSupplementDuplicateMinLines + 1)
	prose = appendAnswerSupplementDeduped(appendAnswerSupplementDeduped("正文。", nine, "zh"), nine, "zh")
	if strings.Count(prose, "边界核对") != 1 || !strings.Contains(prose, "同上略") {
		t.Fatalf("a >K-line duplicate must dedup:\n%s", prose)
	}
}

// TestAnswerSupplementDedup_NoteItselfDeduped — 修复轮 P4: a third
// identical supplement must not stack a second 同上略 note (the note is a
// system surface too — it obeys its own gate).
func TestAnswerSupplementDedup_NoteItselfDeduped(t *testing.T) {
	block := p12ObservationBlock()
	prose := appendAnswerSupplementDeduped("正文。", block, "zh")
	prose = appendAnswerSupplementDeduped(prose, block, "zh")
	prose = appendAnswerSupplementDeduped(prose, block, "zh")
	if got := strings.Count(prose, "同上略"); got != 1 {
		t.Fatalf("the dedup note must not duplicate itself, got %d notes:\n%s", got, prose)
	}
	if strings.Count(prose, "trace_query 关键观测核对") != 1 {
		t.Fatalf("the block still renders exactly once:\n%s", prose)
	}
}
