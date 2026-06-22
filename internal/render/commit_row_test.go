package render

import (
	"strings"
	"testing"
)

// TestFormatCommitRow_NoticeUsesSoftRecoverable pins the post-fix
// contract for commitRowNotice (e.g. "草稿被丢弃, 正在重写"): the
// glyph is ↻ (recoverable) NOT ✗ (fatal), and the color is the
// muted yellow `statusRecoverable` — matching the soft "we are
// continuing, the previous draft just needed redoing" semantics.
//
// Pre-fix the kind shared a switch case with commitRowFailure /
// commitRowCancelled, so the glyph rendered red ✗ while the body
// (set by the caller in statusRecoverable yellow) read in yellow —
// the user saw a red error glyph next to yellow text, conflating
// "system continues" with "system failed".
func TestFormatCommitRow_NoticeUsesSoftRecoverable(t *testing.T) {
	body := "上一份草稿被规则丢弃，正在重写"
	line := formatCommitRow(commitRow{
		kind: commitRowNotice,
		body: statusRecoverable.Sprint(body),
	})
	plain := stripAnsiEscapes(line)
	if !strings.ContainsRune(plain, glyphRecoverable) {
		t.Errorf("notice row must render with ↻ glyph; got plain %q (raw %q)", plain, line)
	}
	if strings.ContainsRune(plain, glyphFatal) {
		t.Errorf("notice row must NOT render with ✗ glyph (recoverable, not failure); got plain %q", plain)
	}
	// Check the styled glyph segment carries the yellow palette.
	yellowGlyph := statusRecoverable.Sprint(string(glyphRecoverable))
	if !strings.Contains(line, yellowGlyph) {
		t.Errorf("notice row glyph must be styled with statusRecoverable; raw line %q does not contain %q", line, yellowGlyph)
	}
	if !strings.Contains(plain, body) {
		t.Errorf("notice row must include body text; got plain %q", plain)
	}
}

// TestFormatCommitRow_CancelledUsesMutedGlyph pins that user-initiated
// cancellation renders with ⊘ + muted gray — distinct from the red ✗
// reserved for SYSTEM failures. A user pressing Ctrl+C is a deliberate
// stop, not an error condition; the dock summary line should reflect
// that visually.
//
// Pre-fix commitRowCancelled shared a case with commitRowFailure (red
// ✗), and the caller wrapped the body with statusFatal (red text). The
// only signal distinguishing "you cancelled" from "we crashed" was the
// 已取消 / cancelled word — a label the eye reads after color, not
// before. After the fix the row reads as quiet gray informational.
func TestFormatCommitRow_CancelledUsesMutedGlyph(t *testing.T) {
	body := "已取消 · 总耗时 5s"
	line := formatCommitRow(commitRow{
		kind: commitRowCancelled,
		body: statusMeta.Sprint(body),
	})
	plain := stripAnsiEscapes(line)
	if !strings.ContainsRune(plain, glyphCancelled) {
		t.Errorf("cancelled row must render with ⊘ glyph; got plain %q (raw %q)", plain, line)
	}
	if strings.ContainsRune(plain, glyphFatal) {
		t.Errorf("cancelled row must NOT render with ✗ glyph (user stop, not failure); got plain %q", plain)
	}
	// Glyph segment must be styled with statusMeta (not statusFatal).
	grayGlyph := statusMeta.Sprint(string(glyphCancelled))
	if !strings.Contains(line, grayGlyph) {
		t.Errorf("cancelled row glyph must be styled with statusMeta; raw line %q does not contain %q", line, grayGlyph)
	}
	if !strings.Contains(plain, body) {
		t.Errorf("cancelled row must include body text; got plain %q", plain)
	}
}

// TestFormatCommitRow_FailureKeepsFatalRed locks that genuine system
// failures continue to render in the loud ✗ + red palette. The split
// must NOT relax the failure path — it only carves notice / cancelled
// out of the original three-way alias.
func TestFormatCommitRow_FailureKeepsFatalRed(t *testing.T) {
	body := "失败 · 总耗时 9s"
	line := formatCommitRow(commitRow{
		kind: commitRowFailure,
		body: statusFatal.Sprint(body),
	})
	plain := stripAnsiEscapes(line)
	if !strings.ContainsRune(plain, glyphFatal) {
		t.Errorf("failure row must render with ✗ glyph; got plain %q", plain)
	}
	redGlyph := statusFatal.Sprint(string(glyphFatal))
	if !strings.Contains(line, redGlyph) {
		t.Errorf("failure row glyph must be styled with statusFatal; raw line %q does not contain %q", line, redGlyph)
	}
}

// TestFormatCommitRow_CancelledAndFailureRenderDifferently locks the
// distinction at the BYTE level: the same body string fed through the
// two kinds must produce different output, otherwise the split is a
// no-op cosmetic refactor. Pinning the inequality catches a future
// refactor that accidentally re-merges them.
func TestFormatCommitRow_CancelledAndFailureRenderDifferently(t *testing.T) {
	body := "stop · 5s"
	cancelled := formatCommitRow(commitRow{kind: commitRowCancelled, body: body})
	failure := formatCommitRow(commitRow{kind: commitRowFailure, body: body})
	if cancelled == failure {
		t.Errorf("cancelled and failure rows must render distinctly; both produced %q", cancelled)
	}
}
