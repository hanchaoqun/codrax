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
// typed fields, so long as the roster resolves to ≤1 known query window.
//
// EVOLUTION RECORD (§21.1 CWD-2 ①, huadong_01 revisit E19 witness,
// real_trace_campaign_20260705.md, 2026-07-07 — supersedes the original F-1 ③
// blanket inertness and the "disjoint-cross-window disclosure belongs to the
// q1-B6 batch" deferral): a merged SUM row whose members span MULTIPLE query
// windows is now display-ACTIVE in exactly three adjudicated ways — the
// anchor-window % cell is suppressed (绝不跨窗分子÷单锚窗分母打 %), its
// legend entry renders, and the lossless block grows the 窗来源 roster. The
// SUM value, the ×N(a–b) form token and the 求和口径 wording stay
// byte-identical (disjoint windows are a LEGAL sum; only the %-face was the
// bug). Single-window / windowless rows keep full byte-identity as the
// inertness control.
func TestN2NonUnionRendersByteIdentical(t *testing.T) {
	build := func(windows int) types.TraceCausalProjection {
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
		if windows >= 1 {
			node.QueryWindowStartTs, node.QueryWindowEndTs = 100.000, 101.000
			node.MergedQueryWindows = []types.TraceCausalProjectionQueryWindow{
				{StartTs: 100.000, EndTs: 101.000},
			}
		}
		if windows >= 2 {
			node.MergedQueryWindows = append(node.MergedQueryWindows,
				types.TraceCausalProjectionQueryWindow{StartTs: 101.400, EndTs: 101.800})
		}
		return types.TraceCausalProjection{
			WindowStartTs: 100.000,
			WindowEndTs:   101.000,
			OnChainCauses: []types.TraceCausalProjectionNode{node},
		}
	}
	for _, zh := range []bool{true, false} {
		bare := buildRuntimeTraceProjTreeModel(build(0), newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		single := buildRuntimeTraceProjTreeModel(build(1), newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		lang := "en"
		if zh {
			lang = "zh"
		}
		// Inertness control (F-1 ③ surviving half): a SINGLE-window roster is
		// display-inert on every surface.
		if a, b := runtimeTraceProjTreeFence(bare, zh), runtimeTraceProjTreeFence(single, zh); a != b {
			t.Fatalf("zh=%v: single-window sum fence must be byte-identical:\n%q\nvs\n%q", zh, a, b)
		}
		if a, b := runtimeTraceProjLeadText(build(0), bare, lang, zh), runtimeTraceProjLeadText(build(1), single, lang, zh); a != b {
			t.Fatalf("zh=%v: single-window sum lead must be byte-identical:\n%q\nvs\n%q", zh, a, b)
		}
		if a, b := runtimeTraceProjDetailFullText(bare, zh), runtimeTraceProjDetailFullText(single, zh); a != b {
			t.Fatalf("zh=%v: single-window sum lossless block must be byte-identical:\n%q\nvs\n%q", zh, a, b)
		}
		// §21.1 CWD-2 ① active half: the multi-window roster suppresses the
		// anchor-window % (the bar and the ms cell stay), keeps the plain SUM
		// form + 求和口径 wording, and grows the 窗来源 roster.
		multi := buildRuntimeTraceProjTreeModel(build(2), newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		fence := runtimeTraceProjTreeFence(multi, zh)
		if !strings.Contains(fence, "×3(5.000–20.000ms)") || strings.Contains(fence, ")union") {
			t.Fatalf("zh=%v: multi-window sum row must keep the plain sum form: %s", zh, fence)
		}
		if !strings.Contains(fence, "35.000ms") || strings.Contains(fence, "4%") {
			t.Fatalf("zh=%v: multi-window sum row must keep the ms cell and drop the anchor-window share: %s", zh, fence)
		}
		if !multi.Marks.has(runtimeTraceProjMarkMergedMultiWindowNoShare) {
			t.Fatalf("zh=%v: multi-window sum row must record the no-share mark", zh)
		}
		lossless := runtimeTraceProjDetailFullText(multi, zh)
		if zh && !strings.Contains(lossless, "×3 求和口径,单次 5.000–20.000ms") {
			t.Fatalf("sum row lossless caliber wording drifted:\n%s", lossless)
		}
		if zh && !strings.Contains(lossless, "窗来源") {
			t.Fatalf("§21.1 CWD-2 ①: multi-window sum row must list its 窗来源 roster:\n%s", lossless)
		}
		if !strings.Contains(lossless, "100.000–101.000s") || !strings.Contains(lossless, "101.400–101.800s") {
			t.Fatalf("window roster must name both member windows:\n%s", lossless)
		}
		// The single-window control never records the mark nor grows a roster.
		if single.Marks.has(runtimeTraceProjMarkMergedMultiWindowNoShare) {
			t.Fatalf("zh=%v: single-window sum row must not record the no-share mark", zh)
		}
		if zh && strings.Contains(runtimeTraceProjDetailFullText(single, zh), "窗来源") {
			t.Fatalf("single-window sum row must not grow a 窗来源 line")
		}
	}
}
