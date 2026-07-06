package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// §11-N2 display pins (real_trace_campaign_20260705.md; F-1 复核吸收
// 2026-07-06): a cross-query-window union ×N row must never wear the SUM
// caliber's clothes on ANY surface — its own ×N(a–b)union form token in the
// fence and the (a)-table name cell, its own NEW-7 legend entry, and a
// lossless-block ×N 明细 line that names the union caliber, the raw Σ and
// the member window sources. Non-union renders stay byte-identical.

func n2UnionProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WindowStartTs: 3680.800,
		WindowEndTs:   3681.001,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleCausalHop, EvidenceID: "E10",
			Subject: "binder:8815_1-6581", Object: "runnable", StateKind: "runnable",
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
			ImpactMS: 168.734, CumulativeImpactMS: 168.734,
			StartTs: 3680.6909, EndTs: 3681.0790,
			MergedCount: 4, MergedMinMS: 14.550, MergedMaxMS: 104.127,
			MergedIntervalUnion: true, MergedSumMS: 183.940,
			MergedQueryWindows: []types.TraceCausalProjectionQueryWindow{
				{StartTs: 3680.569, EndTs: 3682.819},
				{StartTs: 3680.800, EndTs: 3681.001},
			},
			MergedEvidenceIDs: []string{"E11", "E12", "E13"},
			Confidence:        0.8,
		}},
	}
}

// TestN2UnionRowWearsUnionFormOnEverySurface pins the q2-E10 render shape:
// the union form token, the union legend entry (and the ABSENCE of the sum
// legend entry — the exact 口径谎言 F-1 closed), the (a)-table flags fork,
// and the lossless block's union caliber + raw Σ + window-source roster.
func TestN2UnionRowWearsUnionFormOnEverySurface(t *testing.T) {
	projection := n2UnionProjection()
	for _, zh := range []bool{true, false} {
		model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		fence := runtimeTraceProjTreeFence(model, zh)
		lang := "en"
		if zh {
			lang = "zh"
		}
		lead := runtimeTraceProjLeadText(projection, model, lang, zh)
		lossless := runtimeTraceProjDetailFullText(model, zh)

		if !strings.Contains(fence, "×4(14.550–104.127ms)union") {
			t.Fatalf("zh=%v: fence must carry the union form token:\n%s", zh, fence)
		}
		unionEntry := "- `×N(a–b)union` = 跨查询窗重叠段不重复计:N 次实例来自不同查询窗且时间重叠,数值为区间并集投影(非求和),a–b 为单次范围;原始和与窗来源见无损块。"
		sumEntry := "- `×N(a–b)` = 同一(线程,原因)的 N 次实例合并,数值为总和,a–b 为单次范围。"
		if !zh {
			unionEntry = "- `×N(a–b)union` = cross-query-window overlap counted once: the N instances come from DIFFERENT query windows and overlap in time; the value is the interval-union projection (never the SUM), a–b the per-instance range; the raw sum and the window sources live in the lossless block."
			sumEntry = "- `×N(a–b)` = N instances of one (thread, cause) merged; the value is the SUM, a–b the per-instance range."
		}
		if !strings.Contains(lead, unionEntry) {
			t.Fatalf("zh=%v: union legend entry missing (NEW-7 verbatim pin):\n%s", zh, lead)
		}
		if strings.Contains(lead, sumEntry) {
			t.Fatalf("zh=%v: the SUM legend entry must NOT render for a union-only tree (口径谎言): %s", zh, lead)
		}
		wantDetail := "×4 union 口径(2 窗重叠段不重复计),原始和 183.940ms 供对照,单次 14.550–104.127ms"
		wantWindows := "3680.569–3682.819s、3680.800–3681.001s"
		if !zh {
			wantDetail = "×4 union caliber (overlap across 2 windows counted once), raw sum 183.940ms for cross-checking, each 14.550–104.127ms"
			wantWindows = "3680.569–3682.819s, 3680.800–3681.001s"
		}
		if !strings.Contains(lossless, wantDetail) {
			t.Fatalf("zh=%v: lossless ×N detail must state the union caliber + raw Σ:\n%s", zh, lossless)
		}
		if !strings.Contains(lossless, wantWindows) {
			t.Fatalf("zh=%v: lossless block must list the member window sources:\n%s", zh, lossless)
		}
		if zh && !strings.Contains(lossless, "窗来源") {
			t.Fatalf("lossless block must carry the 窗来源 lane:\n%s", lossless)
		}
		// (a)-table: the name cell wears the union token, and the gated legend
		// flags fork — a union row must NOT raise mergedSum (the runtime.go
		// gated line "×N(a–b) = 数值为总和" must never gloss a union value).
		_, rows := runtimeTraceProjDetailTable(model, zh)
		tokenSeen := false
		for _, row := range rows {
			if len(row.Cells) > 0 && strings.Contains(row.Cells[0], ")union") {
				tokenSeen = true
			}
		}
		if !tokenSeen {
			t.Fatalf("zh=%v: detail-table name cell must carry the union form token: %+v", zh, rows)
		}
		flags := runtimeTraceProjDetailTableLegendFlagsFor(model, zh)
		if !flags.mergedUnion || flags.mergedSum {
			t.Fatalf("zh=%v: union row must raise mergedUnion and never mergedSum: %+v", zh, flags)
		}
	}
}

// TestN2UnionLegendBidirectional runs the NEW-7 two-way contract on the union
// shape: the ×N(a–b)union legend entry renders exactly when the union mark is
// emitted, and the ")union" fence probe appears exactly when the entry does.
func TestN2UnionLegendBidirectional(t *testing.T) {
	marks := revisit76AssertLegendBidirectional(t, "n2_union_zh", n2UnionProjection(), true)
	if !marks.has(runtimeTraceProjMarkMergedUnion) {
		t.Fatalf("union fixture must emit the union mark")
	}
	if marks.has(runtimeTraceProjMarkMergedSum) {
		t.Fatalf("union fixture must not emit the sum mark (口径谎言 guard)")
	}
	revisit76AssertLegendBidirectional(t, "n2_union_en", n2UnionProjection(), false)
}

// TestN2NonUnionRendersByteIdentical pins F-1 ③: every surface of a plain
// SUM ×N row is byte-identical whether or not the row carries the new §11-N2
// typed fields (window roster + row-level query-window identity) — the
// disjoint-cross-window disclosure belongs to the q1-B6 batch, and until it
// lands the new fields must be display-inert outside the union caliber.
func TestN2NonUnionRendersByteIdentical(t *testing.T) {
	build := func(withFields bool) types.TraceCausalProjection {
		node := types.TraceCausalProjectionNode{
			Role: types.TraceCausalRoleCausalHop, EvidenceID: "E5",
			Subject: "worker-7", Object: "runnable", StateKind: "runnable",
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
			ImpactMS: 35.000, CumulativeImpactMS: 35.000,
			StartTs: 100.100, EndTs: 101.520,
			MergedCount: 3, MergedMinMS: 5.000, MergedMaxMS: 20.000,
			MergedEvidenceIDs: []string{"E6", "E7"},
			Confidence:        0.8,
		}
		if withFields {
			node.QueryWindowStartTs, node.QueryWindowEndTs = 100.000, 101.000
			node.MergedQueryWindows = []types.TraceCausalProjectionQueryWindow{
				{StartTs: 100.000, EndTs: 101.000},
				{StartTs: 101.400, EndTs: 101.800},
			}
		}
		return types.TraceCausalProjection{
			WindowStartTs: 100.000,
			WindowEndTs:   101.000,
			OnChainCauses: []types.TraceCausalProjectionNode{node},
		}
	}
	for _, zh := range []bool{true, false} {
		bare := buildRuntimeTraceProjTreeModel(build(false), newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		full := buildRuntimeTraceProjTreeModel(build(true), newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		lang := "en"
		if zh {
			lang = "zh"
		}
		if a, b := runtimeTraceProjTreeFence(bare, zh), runtimeTraceProjTreeFence(full, zh); a != b {
			t.Fatalf("zh=%v: non-union fence must be byte-identical:\n%q\nvs\n%q", zh, a, b)
		}
		if a, b := runtimeTraceProjLeadText(build(false), bare, lang, zh), runtimeTraceProjLeadText(build(true), full, lang, zh); a != b {
			t.Fatalf("zh=%v: non-union lead must be byte-identical:\n%q\nvs\n%q", zh, a, b)
		}
		if a, b := runtimeTraceProjDetailFullText(bare, zh), runtimeTraceProjDetailFullText(full, zh); a != b {
			t.Fatalf("zh=%v: non-union lossless block must be byte-identical:\n%q\nvs\n%q", zh, a, b)
		}
		// The sum row still wears the sum form + 求和口径 (never union).
		fence := runtimeTraceProjTreeFence(full, zh)
		if !strings.Contains(fence, "×3(5.000–20.000ms)") || strings.Contains(fence, ")union") {
			t.Fatalf("zh=%v: sum row must keep the plain sum form: %s", zh, fence)
		}
		lossless := runtimeTraceProjDetailFullText(full, zh)
		if zh && !strings.Contains(lossless, "×3 求和口径,单次 5.000–20.000ms") {
			t.Fatalf("sum row lossless caliber wording drifted:\n%s", lossless)
		}
		if zh && strings.Contains(lossless, "窗来源") {
			t.Fatalf("non-union row must not grow a 窗来源 line (q1-B6 batch owns that):\n%s", lossless)
		}
	}
}
