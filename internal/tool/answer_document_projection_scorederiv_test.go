package tool

// answer_document_projection_scorederiv_test.go — SCORE-DERIV pins
// (§29.104.22 立案 + §29.104.22.1 user ruling, 2026-07-17; DISPLAY-HYG 件3):
// the 阅读参考 (column-caliber legend) formula entries render ON DEMAND —
// exactly when their value word face is on the render (承诺面双向) — and the
// weight constants stay OFF the generic legend. A typed count-only IO row is
// the narrow exception: its own detail may disclose event_count×5 so a visible
// activity index is explainable without inventing an absolute pressure level.
//
// MUTATION self-checks (cp-copy recovery only):
//   - M-1 render the entries unconditionally → the no-caliber-row negative
//     arm reds (entry without word face);
//   - M-2 drop an entry's flag hook → its positive arm reds;
//   - M-3 spell a weight constant into a generic entry → the zero-digit scan
//     reds.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// scoreDerivClusterText renders the full projection cluster and joins every
// block's text (lead + ◎ + tree, detail-table legend, detail blocks,
// evidence) — the report face the entries and word faces both live on.
func scoreDerivClusterText(t *testing.T, projection types.TraceCausalProjection, lang string) string {
	t.Helper()
	blocks := runtimeTraceCausalProjectionCluster(projection, lang, runtimeTraceProjUserFocus{})
	if len(blocks) == 0 {
		t.Fatalf("projection cluster must render")
	}
	var b strings.Builder
	for _, block := range blocks {
		b.WriteString(block.Text)
		b.WriteString("\n")
	}
	return b.String()
}

// scoreDerivIOPressureNode — an io_pressure aggregate row on the M18 typed
// Unit=composite_score lane (the wire the display's composite caliber reads).
func scoreDerivIOPressureNode() types.TraceCausalProjectionNode {
	node := uxr1BackgroundNode("E-iop", "io_pressure·聚合", "io_pressure", "io_pressure", "", 699.293, 400)
	node.Unit = types.TraceObservationUnitCompositeScore
	node.SubjectKind = types.TraceCausalSubjectKindAggregateMetric
	return node
}

// scoreDerivClampedCountNode — the donghu E46 clamped count-family form
// (published 81.616 < member Σ 119.1; the 超上限截断 word's typed producer).
func scoreDerivClampedCountNode() types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "E-clamp",
		Subject: "app-42", Object: "page_cache_churn", TypeToken: "page_cache_churn",
		ChainRelevance: "self_caliber_side", Tier: types.TraceCausalTierCaliberSide,
		Predicate: "root_cause_context", ImpactMS: 81.616, CumulativeImpactMS: 81.616,
		EffectiveImpactMS: 81.616, Confidence: 0.66, LineStart: 955, LineEnd: 20250,
		FamilyMemberCount: 2, FamilyMemberMaxMS: 84.3, FamilyMemberMinMS: 34.8,
		FamilyMemberSumMS: 119.1, FamilyFoldCaliber: "count_sum",
		FamilyMemberRoster: []string{
			"inode=0x25a01 dev=260:132 计数当量84.300(非墙钟)",
			"inode=0x14088d dev=260:132 计数当量34.800(非墙钟)",
		},
	}
}

const (
	scoreDerivEntryIOPressureZH = "- 综合评分(io_pressure) = 最大单事件块/存储延迟 + iowait 阻塞次数(加权) + D态/iowait 墙钟 + 页缓存事件(加权) + 文件IO事件与字节(加权):跨单位合成分,通用图例不列系数;typed count-only 行可披露精确分解,但不定义绝对压力等级;非墙钟,不参与汇排。"
	scoreDerivEntryBlockIOZH    = "- 综合评分(block_io) = 最大块延迟 + 最大存储延迟 + 文件IO字节(加权):跨单位合成分,加权系数为固定常量(报告不列数值);非墙钟,不参与汇排。"
	scoreDerivEntryCountZH      = "- 计数当量 = 事件数 × 固定当量系数(文件IO热点形另含字节量加权项;系数均不列数值);非墙钟,不参与汇排。"
	scoreDerivEntryClampZH      = "- 超上限截断 = 计数当量按窗长固定比例设上限,超出即按上限发布(原始和随行供对照);非墙钟,不参与汇排。"
)

// TestScoreDerivEntriesRenderOnDemand — the four formula entries render
// exactly with their word faces: the base board (block_io ⌗ + count ⌗) shows
// those two entries only; adding the io_pressure aggregate and the clamped
// count family lights the other two; a caliber-free board shows none.
func TestScoreDerivEntriesRenderOnDemand(t *testing.T) {
	base := scoreDerivClusterText(t, elimBoardProjection(), "zh")
	if !strings.Contains(base, scoreDerivEntryBlockIOZH) || !strings.Contains(base, scoreDerivEntryCountZH) {
		t.Fatalf("block_io/count entries must render verbatim with their ⌗ rows:\n%s", base)
	}
	if strings.Contains(base, "综合评分(io_pressure)") || strings.Contains(base, "- 超上限截断") {
		t.Fatalf("absent word faces must keep their entries off (承诺面双向):\n%s", base)
	}

	full := elimBoardProjection()
	full.BackgroundCauses = append(full.BackgroundCauses, scoreDerivIOPressureNode())
	full.AdjacentCauses = append(full.AdjacentCauses, scoreDerivClampedCountNode())
	text := scoreDerivClusterText(t, full, "zh")
	for _, want := range []string{
		scoreDerivEntryIOPressureZH, scoreDerivEntryBlockIOZH,
		scoreDerivEntryCountZH, scoreDerivEntryClampZH,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("entry must render verbatim:\n%s\nreport:\n%s", want, text)
		}
	}

	// Negative arm: a board with no caliber-side/composite/count rows renders
	// ZERO formula entries.
	bare := elimBoardProjection()
	var adjacent []types.TraceCausalProjectionNode
	for _, node := range bare.AdjacentCauses {
		if node.EvidenceID == "E-blk" || node.EvidenceID == "E-pgc" {
			continue
		}
		adjacent = append(adjacent, node)
	}
	bare.AdjacentCauses = adjacent
	none := scoreDerivClusterText(t, bare, "zh")
	for _, banned := range []string{"综合评分(io_pressure)", "综合评分(block_io)", "- 计数当量 = 事件数", "- 超上限截断"} {
		if strings.Contains(none, banned) {
			t.Fatalf("caliber-free board must render no formula entry (%q):\n%s", banned, none)
		}
	}
}

// TestScoreDerivEntriesENFaces — the EN legend carries the same four entries
// (zh-en 同词 kernel tokens ride along).
func TestScoreDerivEntriesENFaces(t *testing.T) {
	full := elimBoardProjection()
	full.BackgroundCauses = append(full.BackgroundCauses, scoreDerivIOPressureNode())
	full.AdjacentCauses = append(full.AdjacentCauses, scoreDerivClampedCountNode())
	text := scoreDerivClusterText(t, full, "en")
	for _, want := range []string{
		"- composite score (io_pressure) = max single-event block/storage latency + iowait blocked count (weighted) + D-state/iowait wall clock + page-cache events (weighted) + file-IO events and bytes (weighted): a cross-unit blend whose generic legend omits coefficient values; a typed count-only row may disclose its exact breakdown but defines no absolute pressure level; not wall clock, not ranked here.",
		"- composite score (block_io) = max block latency + max storage latency + file-IO bytes (weighted): a cross-unit blend with fixed weight constants (values not listed in the report); not wall clock, not ranked here.",
		"- count equivalent (计数当量) = event count × a fixed equivalence coefficient (the file-IO hotspot form adds a weighted byte-volume term; values not listed); not wall clock, not ranked here.",
		"- over-limit clamp (超上限截断) = the count equivalent is capped at a fixed fraction of the window length and publishes the cap when exceeded (the raw sum rides along for cross-checking); not wall clock, not ranked here.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("EN entry must render verbatim:\n%s\nreport:\n%s", want, text)
		}
	}
}

// TestScoreDerivGenericLegendHidesWeightConstants — the generic entries carry
// no weight digits. The typed count-only detail exception is pinned separately
// in trace_query_io_pressure_caliber_test.go.
func TestScoreDerivGenericLegendHidesWeightConstants(t *testing.T) {
	for _, entry := range []string{
		scoreDerivEntryIOPressureZH, scoreDerivEntryBlockIOZH,
		scoreDerivEntryCountZH, scoreDerivEntryClampZH,
	} {
		for _, r := range entry {
			if r >= '0' && r <= '9' {
				t.Fatalf("formula entry must carry zero digits (weight constants hidden): %q", entry)
			}
		}
	}
	full := elimBoardProjection()
	full.BackgroundCauses = append(full.BackgroundCauses, scoreDerivIOPressureNode())
	full.AdjacentCauses = append(full.AdjacentCauses, scoreDerivClampedCountNode())
	for _, lang := range []string{"zh", "en"} {
		text := scoreDerivClusterText(t, full, lang)
		for _, banned := range []string{"0.35", "×5", "× 5", "×0.2", "×0.3", "×0.1", "0.25"} {
			if strings.Contains(text, banned) {
				t.Fatalf("weight constant %q leaked onto the %s report face:\n%s", banned, lang, text)
			}
		}
	}
}
