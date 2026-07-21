package tool

// answer_document_projection_calside_test.go — CALSIDE-1 pins (用户显示裁定
// 2026-07-19; witness .codrax/output/20260719-161405.439-17874.md:164-165 /
// :316/:319; F7 filed in §29.147, docs/design/real_trace_campaign_20260705.md).
//
// 件1 — ◎ ⌗ footnote rows carry their seat-row SEMANTIC CLASS WORD between
//   subject and value (single source runtimeTraceProjElimClassWord — the ◎
//   board composer; zero second word table); a class-less seat keeps the
//   word-less form (absence stays absent, never synthesized).
// 件2 — F7 假单位修: a row whose published value renders in a non-wall-clock
//   form (计数当量X(非墙钟) / X(综合评分,非墙钟)) publishes NO bare wall-clock
//   ms in the tree value column, NO window-share % and NO wall-clock bar
//   (cross-unit pools); wall-clock rows stay byte-identical.
// 件3 — the ⌗口径旁栏 legend entry teaches both promises (词条-图例双向).
//
// MUTATION self-checks (cp-copy recovery only, never git):
//   - M-1 drop the count value arm in runtimeTraceProjRowMetricParts (fall
//     back to " %9.3fms") → TestCalside1CountRowValueColumnDropsMsSuit red;
//   - M-2 drop !nonWallClockValue from the % gate →
//     TestCalside1CaliberRowsNoWindowShareNoBar red (share renders);
//   - M-3 drop the nonWallClockValue bar-blank arm →
//     TestCalside1CaliberRowsNoWindowShareNoBar red (bar glyphs render);
//   - M-4 drop the class-word insertion in the ⌗ footnote →
//     TestElimCaliberFootnotePerSeatLines red (word gone);
//   - M-5 synthesize a class word for class-less seats →
//     TestCalside1FootnoteClassAbsenceStaysAbsent red (byte pin).

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// calsideFenceRowLine returns the ONE fence line containing marker.
func calsideFenceRowLine(t *testing.T, fence, marker string) string {
	t.Helper()
	for _, line := range strings.Split(fence, "\n") {
		if strings.Contains(line, marker) {
			return line
		}
	}
	t.Fatalf("no fence line carries %q:\n%s", marker, fence)
	return ""
}

// TestCalside1CountRowValueColumnDropsMsSuit — 件2 (F7 witness 17874:316
// 「7.200ms 6%」 on a ⌗ 计数当量 row): a non-family count-class row's tree
// value column adopts the ONE suffix-free 计数当量X(非墙钟) form — the same
// single-source arm the self 行1 / detail table / ◎ footnote read — and the
// composite row keeps its established form; the wall-clock sibling keeps its
// bare ms cell byte-identically (negative arm).
func TestCalside1CountRowValueColumnDropsMsSuit(t *testing.T) {
	projection := types.TraceCausalProjectionFromObservationRecords(v2p0CaliberRecords())
	for _, zh := range []bool{true, false} {
		model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		fence := runtimeTraceProjTreeFence(model, zh)
		wantCount := runtimeTraceProjCountEquivalentValueText(0.600, zh)
		if !strings.Contains(fence, wantCount) {
			t.Fatalf("zh=%v: the count ⌗ row's value column must speak %q:\n%s", zh, wantCount, fence)
		}
		if strings.Contains(fence, "0.600ms") {
			t.Fatalf("zh=%v: the count ⌗ row must never wear the wall-clock ms suit:\n%s", zh, fence)
		}
		wantComposite := runtimeTraceProjCompositeScoreValueText(0.198, zh)
		if !strings.Contains(fence, wantComposite) || strings.Contains(fence, "0.198ms") {
			t.Fatalf("zh=%v: the composite ⌗ row keeps its suffix-free form %q:\n%s", zh, wantComposite, fence)
		}
		// Negative arm: the wall-clock sibling keeps its bare ms value cell.
		if !strings.Contains(fence, "0.099ms") {
			t.Fatalf("zh=%v: the wall-clock row must keep its ms cell:\n%s", zh, fence)
		}
	}
}

// TestCalside1CaliberRowsNoWindowShareNoBar — 件2 (F7): under a real analysis
// window both ⌗-family rows publish NO window-share % and draw NO wall-clock
// bar (a count-equivalent / composite-score numerator over a wall-clock
// denominator is a cross-unit fake — the CMP-3 cross-thread precedent); the
// wall-clock adjacent sibling keeps bar + % byte-identically. The clamped
// count-sum FAMILY seat (the 计数当量 stem's other mint) rides the same
// predicate: no bar, no %.
func TestCalside1CaliberRowsNoWindowShareNoBar(t *testing.T) {
	projection := elimBoardProjection() // windowed: 2942.100..2942.300 (200ms)
	// Production shape (witness E32/E33 audit line: tier=caliber_side): the ⌗
	// rows ride the caliber-side tier; the io_latency sibling stays a
	// wall-clock rank row.
	for i := range projection.AdjacentCauses {
		switch projection.AdjacentCauses[i].EvidenceID {
		case "E-blk", "E-pgc":
			projection.AdjacentCauses[i].Tier = types.TraceCausalTierCaliberSide
			projection.AdjacentCauses[i].Predicate = "root_cause_caliber_side"
			projection.AdjacentCauses[i].Rank = 0
		}
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	countLine := calsideFenceRowLine(t, fence, "计数当量 0.600(非墙钟)")
	compositeLine := calsideFenceRowLine(t, fence, "0.198(综合评分,非墙钟)")
	for _, line := range []string{countLine, compositeLine} {
		if strings.Contains(line, "%") {
			t.Fatalf("件2: a non-wall-clock value row must not publish a window share: %q", line)
		}
		if strings.Contains(line, "█") || strings.Contains(line, "░") {
			t.Fatalf("件2: a non-wall-clock value row must not draw the wall-clock bar: %q", line)
		}
	}
	// Negative arm: the wall-clock ◇ sibling keeps bar + window share.
	wallLine := calsideFenceRowLine(t, fence, "0.710ms")
	if !strings.Contains(wallLine, "░") || !strings.Contains(wallLine, "%") {
		t.Fatalf("件2 负臂: the wall-clock row keeps its bar and %% cell: %q", wallLine)
	}
	// Clamped count-sum family seat: same predicate, no bar / no %.
	clamped := types.CompileTraceCausalProjection(disp2CountFamilyLedger())
	clamped.WindowStartTs = 100.0
	clamped.WindowEndTs = 100.2
	cModel := buildRuntimeTraceProjTreeModel(clamped, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	cFence := runtimeTraceProjTreeFence(cModel, true)
	cLine := calsideFenceRowLine(t, cFence, "计数当量 41.671(非墙钟)")
	if strings.Contains(cLine, "%") || strings.Contains(cLine, "█") || strings.Contains(cLine, "░") {
		t.Fatalf("件2: the clamped count family seat must not wear bar/%%: %q", cLine)
	}
}

// TestCalside1FootnoteClassAbsenceStaysAbsent — 件1 负臂: a caliber-side seat
// whose single word source resolves NO class word keeps the word-less legacy
// footnote form byte-identically (absence stays absent — the class slot is
// never synthesized); the worded sibling seats on the same footnote carry
// theirs.
func TestCalside1FootnoteClassAbsenceStaysAbsent(t *testing.T) {
	projection := elimBoardProjection()
	wordless := uxr1AdjacentNode("E-nlc", "", "", 4, 0.310, 140)
	wordless.Tier = types.TraceCausalTierCaliberSide
	wordless.Predicate = "root_cause_caliber_side"
	wordless.Rank = 0
	projection.AdjacentCauses = append(projection.AdjacentCauses, wordless)
	_, fence := elimRenderOverview(t, projection, true)
	found := ""
	for _, line := range strings.Split(fence, "\n") {
		if strings.Contains(line, "0.310") {
			found = line
			break
		}
	}
	if found == "" {
		t.Fatalf("the class-less caliber seat must still ride the ⌗ footnote:\n%s", fence)
	}
	// OMGCLEAN-1 件9 (§29.175.8/.13, 2026-07-20). EVOLUTION RECORD: the seat
	// rides a 口径旁栏 aux row (⌗ glyph and boilerplate off this face); the
	// absence discipline itself is unchanged — no class word is synthesized.
	if !strings.HasPrefix(found, "· 口径旁栏") || !strings.Contains(found, "[GT]ColdPool#6-36644 0.310 [") {
		t.Fatalf("件1 负臂: the class-less seat keeps the word-less form on its aux row, got %q\n%s", found, fence)
	}
	// The worded sibling on the SAME account still carries its class word.
	if !strings.Contains(fence, "· 口径旁栏  [GT]ColdPool#6-36644 · 页缓存抖动 · 计数当量 0.600(非墙钟) [") {
		t.Fatalf("件1: the worded sibling seat must keep its class word:\n%s", fence)
	}
}

// TestCalside1LegendCaliberSideEntryTeachesBothPromises — 件3 (词条-图例双向):
// a render that fields ⌗ rows teaches the evolved entry — the class word's
// single-source promise AND the no-bar/no-% caliber statement — on both faces.
func TestCalside1LegendCaliberSideEntryTeachesBothPromises(t *testing.T) {
	projection := types.TraceCausalProjectionFromObservationRecords(v2p0CaliberRecords())
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	runtimeTraceProjTreeFence(model, true)
	lead := runtimeTraceProjLeadText(projection, model, "zh", true)
	if !strings.Contains(lead, "行内类别词与其席行同源") {
		t.Fatalf("件3: the ⌗ entry must teach the class-word single-source promise:\n%s", lead)
	}
	if !strings.Contains(lead, "非墙钟数值不画时长条、不标占窗%(不与墙钟同池比较)") {
		t.Fatalf("件3: the ⌗ entry must teach the no-bar/no-%% caliber statement:\n%s", lead)
	}
	enModel := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	runtimeTraceProjTreeFence(enModel, false)
	enLead := runtimeTraceProjLeadText(projection, enModel, "en", false)
	if !strings.Contains(enLead, "its in-row class word shares its seat row's word source") ||
		!strings.Contains(enLead, "draws no duration bar and no window %") {
		t.Fatalf("件3 EN: the ⌗ entry must teach both promises:\n%s", enLead)
	}
}
