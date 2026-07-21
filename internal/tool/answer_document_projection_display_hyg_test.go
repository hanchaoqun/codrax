package tool

// answer_document_projection_display_hyg_test.go — DISPLAY-HYG 第一轮 件1+件2
// pins (§29.104.18.2 + §29.114 P2, 2026-07-17).
//
// 件1 — ◎ caliber footnote per-seat lines (witness 20260717-092738: one
// footnote line carried two ⌗ seats, each repeating the whole
// ·⌗口径旁栏·XX(非墙钟,不占序数) boilerplate tail):
//   - ≥2 seats: boilerplate hoists into the head line ONCE, one seat per
//     indented line (subject + single-source value form + short ⌗ word + [E#]);
//   - single seat: the legacy one-line form stays BYTE-IDENTICAL (负臂);
//   - overflow: the counted tail renders as its own continuation line.
//
// 件2 — ◎ region width governance: footnote/note lines over the 100-cell cap
// break at token boundaries through runtimeTraceProjWrapDisplay (the §29.114
// engine — CJK word runs, registered compounds, E# references and the `·`
// no-dangle rule all inherit); bar/member/head lines are structural and are
// never wrapped; short fences stay byte-identical.
//
// MUTATION self-checks (cp-copy recovery only):
//   - M-1 drop the multi-seat fork (always join on one line) →
//     TestElimCaliberFootnotePerSeatLines red (head + per-seat lines gone);
//   - M-2 hoist the boilerplate on the single-seat form too →
//     TestElimCaliberFootnoteSingleSeatKeepsLegacyForm red (byte pin);
//   - M-3 wrap bar rows (route member lines through the governor) →
//     TestElimNoteWrapNeverTouchesBarRows red (member line count changes);
//   - M-4 rune-hard-split the footnote (bypass the token engine) →
//     TestElimNoteWrapTokenAware red (E# reference / word-run bisected).

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"

	"github.com/hanchaoqun/codrax/internal/types"
)

// elimFenceLines returns the fence body lines (opener/closer stripped).
func elimFenceLines(fence string) []string {
	lines := strings.Split(fence, "\n")
	var out []string
	for _, line := range lines {
		if strings.HasPrefix(line, "```") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// TestElimCaliberFootnotePerSeatLines — 件1 正臂. EVOLUTION RECORD
// (OMGCLEAN-1 件9, §29.175.8/.13/.14, 2026-07-20): the §29.104.18.2 hoisted
// head + indented seats form retires into same-level 口径旁栏 aux rows — one
// seat per `· ` row, the ⌗ glyph and the (非墙钟,不占序数) boilerplate off
// this face (tree 行2 + legend keep them), the CALSIDE-1 class word kept
// between subject and value (single word source: runtimeTraceProjElimClassWord).
func TestElimCaliberFootnotePerSeatLines(t *testing.T) {
	_, fence := elimRenderOverview(t, elimBoardProjection(), true)
	for _, want := range []string{
		"· 口径旁栏  [GT]ColdPool#6-36644 · 块设备IO(inode) · 0.198(综合评分,非墙钟) [E4]",
		"· 口径旁栏  [GT]ColdPool#6-36644 · 页缓存抖动 · 计数当量0.600(非墙钟) [E6]",
	} {
		if !strings.Contains(fence, want+"\n") && !strings.HasSuffix(fence, want) {
			t.Fatalf("件9: each ⌗ seat must render its own 口径旁栏 aux row:\n%s", fence)
		}
	}
	// §29.175.13: the boilerplate and the ⌗ glyph live on the legend now.
	if strings.Contains(fence, "不占序数") || strings.Contains(fence, "⌗") {
		t.Fatalf("件9: boilerplate/⌗ must stay off the ◎ aux rows (legend home):\n%s", fence)
	}
	// The legacy footnote forms are gone.
	if strings.Contains(fence, "不参与汇排") {
		t.Fatalf("件9: the legacy 不参与汇排 footnote family must retire:\n%s", fence)
	}

	_, en := elimRenderOverview(t, elimBoardProjection(), false)
	squash := strings.ReplaceAll(en, "\n", " ")
	for strings.Contains(squash, "  ") {
		squash = strings.ReplaceAll(squash, "  ", " ")
	}
	for _, want := range []string{
		"· caliber sidebar [GT]ColdPool#6-36644 · block_io_by_inode · 0.198 (composite score, not wall clock) · 综合评分 [E4]",
		"· caliber sidebar [GT]ColdPool#6-36644 · page_cache_churn · count-equivalent 0.600 (not wall clock) · 计数当量 [E6]",
	} {
		if !strings.Contains(squash, want) {
			t.Fatalf("件9 EN: per-seat aux row %q missing:\n%s", want, en)
		}
	}
	if strings.Contains(en, "not ranked here") {
		t.Fatalf("件9 EN: the legacy footnote family must retire:\n%s", en)
	}
}

// TestElimCaliberFootnoteSingleSeatKeepsLegacyForm — 件1 负臂: exactly one ⌗
// seat keeps the UNHOISTED one-line form (full caliber word, boilerplate
// parenthetical in place — never the multi-seat hoisted head).
//
// EVOLUTION RECORD (CALSIDE-1 件1, 2026-07-19): the single-seat line carries
// the seat-row class word too (` · 块设备IO(inode) · `), which pushes it over
// the 100-cell cap — the 件2 width governor breaks it at a token boundary
// (deterministic two-line pin below). The strict byte-identical negative arm
// migrated to the class-ABSENT seat (absence stays absent:
// TestCalside1FootnoteClassAbsenceStaysAbsent).
func TestElimCaliberFootnoteSingleSeatKeepsLegacyForm(t *testing.T) {
	projection := elimBoardProjection()
	// Drop the count seat: the composite ⌗ row is the lone caliber-side seat.
	adjacent := projection.AdjacentCauses[:0]
	for _, node := range projection.AdjacentCauses {
		if node.EvidenceID == "E-pgc" {
			continue
		}
		if node.EvidenceID == "E-blk" {
			node.Subject = "Pool-36644"
		}
		adjacent = append(adjacent, node)
	}
	projection.AdjacentCauses = adjacent
	_, fence := elimRenderOverview(t, projection, true)
	// OMGCLEAN-1 件9 (§29.175.8, 2026-07-20). EVOLUTION RECORD: the 件1
	// single-seat/multi-seat dual form is retired — one seat renders exactly
	// one 口径旁栏 aux row on the same grammar as the multi-seat case (the
	// two-form fork itself was the noise the ruling names).
	want := "· 口径旁栏  Pool-36644 · 块设备IO(inode) · 0.198(综合评分,非墙钟) [E4]"
	if !strings.Contains(fence, want+"\n") && !strings.HasSuffix(fence, want+"\n```") {
		t.Fatalf("件9: the single seat must render one 口径旁栏 aux row, want\n%s\ngot:\n%s", want, fence)
	}
	if strings.Contains(fence, "不参与汇排") || strings.Contains(fence, "⌗") {
		t.Fatalf("件9: the legacy footnote forms must retire:\n%s", fence)
	}
}

// TestElimCaliberFootnoteSingleSeatOverWideWraps — §29.114 P2 witness parity
// (the ◎ 脚注 105-cell cold-read finding): an over-cap single-seat footnote
// KEEPS the legacy word form but the width governor breaks it at a token
// boundary — the [E#] reference moves down whole, never bisected.
func TestElimCaliberFootnoteSingleSeatOverWideWraps(t *testing.T) {
	projection := elimBoardProjection()
	adjacent := projection.AdjacentCauses[:0]
	for _, node := range projection.AdjacentCauses {
		if node.EvidenceID == "E-pgc" {
			continue
		}
		adjacent = append(adjacent, node)
	}
	projection.AdjacentCauses = adjacent
	_, fence := elimRenderOverview(t, projection, true)
	// OMGCLEAN-1 件9: the aux row is far shorter than the legacy worded
	// footnote (⌗ word + boilerplate hoisted to the legend), so the same
	// fixture now fits the 100-cell cap on ONE line — the width governor
	// still guards the family (over-cap aux rows break at token boundaries,
	// TestElimNoteWrapTokenAware), and the E# stays whole.
	want := "· 口径旁栏  [GT]ColdPool#6-36644 · 块设备IO(inode) · 0.198(综合评分,非墙钟) [E4]"
	if !strings.Contains(fence, want+"\n") {
		t.Fatalf("件9: the single-seat aux row must render whole with its E#:\n%s", fence)
	}
}

// TestElimCaliberFootnoteOverflowTailLine — 件1: with more seats than the cap
// the counted overflow renders as its own continuation line under the head.
func TestElimCaliberFootnoteOverflowTailLine(t *testing.T) {
	projection := elimBoardProjection()
	projection.AdjacentCauses = append(projection.AdjacentCauses,
		uxr1AdjacentNode("E-pgc2", "page_cache_churn", "page_cache_churn", 4, 0.500, 140),
		uxr1AdjacentNode("E-pgc3", "page_cache_churn", "page_cache_churn", 5, 0.400, 150),
	)
	_, fence := elimRenderOverview(t, projection, true)
	// OMGCLEAN-1 件9 (§29.175.14 同级行纪律): the counted overflow renders as
	// its own same-level 口径旁栏 row — never a continuation line.
	if !strings.Contains(fence, "· 口径旁栏  …等共4行,其余见明细") {
		t.Fatalf("件9: the counted overflow must render as its own 口径旁栏 row:\n%s", fence)
	}
}

// elimHygWideProjection — 件2 fixture: a caliber seat whose subject is wide
// enough that the per-seat footnote line exceeds the 100-cell cap, plus a
// chain member with the same wide subject so a BAR row is over-wide too.
func elimHygWideProjection() (types.TraceCausalProjection, string) {
	wide := "VeryLongThreadNameForWidthGovernanceWitnessAndMore-1234567890"
	projection := elimBoardProjection()
	for i := range projection.AdjacentCauses {
		if projection.AdjacentCauses[i].EvidenceID == "E-pgc" {
			projection.AdjacentCauses[i].Subject = wide
		}
		if projection.AdjacentCauses[i].EvidenceID == "E-blk" {
			projection.AdjacentCauses[i].Subject = wide
		}
	}
	for i := range projection.OnChainCauses {
		if projection.OnChainCauses[i].EvidenceID == "E-run" {
			projection.OnChainCauses[i].Subject = wide
		}
	}
	return projection, wide
}

// TestElimNoteWrapTokenAware — 件2 正臂: an over-wide footnote line breaks at
// token boundaries into indented continuation lines; every note line holds
// the 100-cell cap; no E# reference is bisected and no `·` dangles at EOL;
// the squashed join preserves the content losslessly.
func TestElimNoteWrapTokenAware(t *testing.T) {
	projection, wide := elimHygWideProjection()
	_, fence := elimRenderOverview(t, projection, true)
	lines := elimFenceLines(fence)
	sawContinuation := false
	for _, line := range lines {
		if strings.Contains(line, "█") || strings.Contains(line, "░") {
			continue // bar rows are structural (covered below)
		}
		if w := runewidth.StringWidth(line); w > runtimeTraceProjTreeRowMaxWidth {
			t.Fatalf("件2: note line exceeds the cap (%d cells): %q\n%s", w, line, fence)
		}
		trimmed := strings.TrimRight(line, " ")
		if strings.HasSuffix(trimmed, "·") {
			t.Fatalf("件2: `·` must never dangle at EOL: %q\n%s", line, fence)
		}
		if i := strings.LastIndex(trimmed, "[E"); i >= 0 && !strings.Contains(trimmed[i:], "]") {
			t.Fatalf("件2: E# reference bisected across lines: %q\n%s", line, fence)
		}
		// OMGCLEAN-1 件9: aux rows lead with `· ` and continue with two
		// spaces; tail-note rows (另有/…等共) are two-space by design — a
		// continuation is any two-space line that is not a tail-note row.
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "  另有") &&
			!strings.HasPrefix(line, "  …") && strings.TrimSpace(line) != "" {
			sawContinuation = true
		}
	}
	if !sawContinuation {
		t.Fatalf("件2: the wide seat line must produce an indented continuation:\n%s", fence)
	}
	// Lossless: the squashed fence must still carry the seat's subject, value
	// and evidence reference (wrap relocates bytes, never drops them).
	squash := strings.ReplaceAll(strings.ReplaceAll(fence, "\n", " "), "  ", " ")
	for strings.Contains(squash, "  ") {
		squash = strings.ReplaceAll(squash, "  ", " ")
	}
	for _, want := range []string{wide, "计数当量0.600(非墙钟)", "[E6]"} {
		if !strings.Contains(squash, want) {
			t.Fatalf("件2: wrapped footnote lost %q:\n%s", want, fence)
		}
	}
}

// TestElimNoteWrapNeverTouchesBarRows — 件2 负臂: an over-wide BAR row stays
// ONE physical line (structural, never wrapped), and the standard short-form
// board renders zero continuation lines (byte-stable geometry).
func TestElimNoteWrapNeverTouchesBarRows(t *testing.T) {
	projection, wide := elimHygWideProjection()
	_, fence := elimRenderOverview(t, projection, true)
	barLines := elimOverviewMemberLines(fence)
	sawWideBar := false
	for _, line := range barLines {
		if strings.Contains(line, wide) {
			sawWideBar = true
			if !strings.Contains(line, "ms") || !strings.Contains(line, "[E") {
				t.Fatalf("件2 负臂: bar row must keep value and E# on its ONE line: %q", line)
			}
		}
	}
	if !sawWideBar {
		t.Fatalf("件2 负臂: fixture must field an over-wide bar row:\n%s", fence)
	}
	// Short-form geometry stays untouched: no wrapped continuation lines at
	// all (bar rows lead with their right-aligned value cell spaces and are
	// excluded — they are structural, not notes).
	_, short := elimRenderOverview(t, elimBoardProjection(), true)
	for _, line := range elimFenceLines(short) {
		if strings.Contains(line, "█") || strings.Contains(line, "░") {
			continue
		}
		if strings.HasPrefix(line, "    ") {
			t.Fatalf("件2 负臂: the short board must render zero continuation lines: %q\n%s", line, short)
		}
	}
}
