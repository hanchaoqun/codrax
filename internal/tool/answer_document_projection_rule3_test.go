package tool

// answer_document_projection_rule3_test.go — RULE3-1 batch pins (§29.181 /
// §29.182 / §29.183 / §29.185 user rulings, 2026-07-20/21):
//
//	件1  树面套话上收 — (a) cross-board ⇄ short marker + legend invariant;
//	     (b) edge-anchor short marker (mechanism lives in the legend);
//	     (c) same-value seat-window hoist (head declaration + inline drop;
//	     异值窗负臂; detail face keeps full chips).
//	件2  序数单载 — badge-wearing 行2 drops 根因排序#N; un-badged ordinal
//	     rows keep the word form (fold-twin 兜底).
//	件4  ε-overlap 披露道 — chainSupportEpsilonOverlapDiscloseRatio (1%,
//	     independent constant), live DHMINE specimen (donghu 2179 board,
//	     0.099%), ≥floor negative arm, value zero-motion arm.
//	件5  matches 披露注 — see emit_investigation_complete pins below.
//	件6  白话扩案 — 降道/车道 banned on every render face + legend catalog.
//	件7  非IO已证 D 席判词 — see TestOmgcleanVerdictWordTable (evolved).
//	件9  ◎ 席行凭证级 chip — identity-inheritance / envelope-credential
//	     chips, per-segment-credential negative arm.
//	件10 守恒图例种群句.
//	件11 ◈ TOP5 — see business_span_mention_test.go (tracequery).
//
// MUTATION self-checks (cp-copy recovery only):
//	M-件1a re-inlining the invariant clause reds the ⇄ short-form pin;
//	M-件1c dropping the hoist reds the head-declaration arm; hoisting a
//	       distinct-window report reds the 异值窗 negative arm;
//	M-件2  re-appending the chip word on badged rows reds the 双载 arm;
//	M-件4  borrowing the INTERFLOOR 5% floor reds the constant pin;
//	M-件9  wearing the chip on per-segment-credential seats reds the
//	       negative arm.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// rule3TwoBoardSeat builds a minimal cross-board pair member.
func rule3TwoBoardSeat(evidence, subject, token, target string, rank int, eff float64, line int) types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: evidence,
		Subject: subject, Object: token, TypeToken: token,
		StateKind: "runnable", ChainRelevance: "on_chain", ChainDepth: 1,
		ImpactMS: eff, CumulativeImpactMS: eff, EffectiveImpactMS: eff,
		QueryWindowStartTs: 10.0, QueryWindowEndTs: 10.2,
		RankBoardTarget: target, Rank: rank, Tier: "primary", Confidence: 0.72,
		LineStart: line, LineEnd: line + 10,
	}
}

func rule3TwoBoardProjection() types.TraceCausalProjection {
	a := rule3TwoBoardSeat("r3p-a", "workerX-300", "runnable_wait", "target.a-100", 1, 5.0, 10)
	b := rule3TwoBoardSeat("r3p-b", "workerX-300", "runnable_wait", "target.b-200", 2, 3.0, 30)
	return types.TraceCausalProjection{
		WakeupPath:    []string{"waker-1", "target.a-100"},
		WindowStartTs: 10.0, WindowEndTs: 10.2,
		OnChainCauses: []types.TraceCausalProjectionNode{a, b},
	}
}

// TestRule3CrossBoardShortMarkerAndLegendInvariant — 件1(a) 正臂: the wearing
// row carries the ⇄ short marker with its per-row halves ONLY, the invariant
// clause lives in the legend entry once, and the retired long sentence never
// renders on a row.
func TestRule3CrossBoardShortMarkerAndLegendInvariant(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(rule3TwoBoardProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, true))
	if !strings.Contains(fence, "⇄另板[") || !strings.Contains(fence, "(板锚 target.") {
		t.Fatalf("件1(a): the ⇄ short marker with refs + 板锚 must render:\n%s", fence)
	}
	for _, banned := range []string{"同线程同状态族账另见另板席", "各板独立成账、口径各异,不可跨板相加)"} {
		if strings.Contains(fence, banned) {
			t.Fatalf("件1(a) 套话复活: the retired long sentence %q must not ride a row:\n%s", banned, fence)
		}
	}
	legend := strings.Join(runtimeTraceProjLegendGroupLines(model.Marks, true), "\n")
	if !strings.Contains(legend, "`⇄另板[E#](板锚 X)`") ||
		!strings.Contains(legend, "席位值不可跨板相加") {
		t.Fatalf("件1(a) 词条-图例双向: the legend must carry the invariant once:\n%s", legend)
	}
	// EN mirror.
	modelEN := buildRuntimeTraceProjTreeModel(rule3TwoBoardProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	fenceEN := rspaFenceJoined(runtimeTraceProjTreeFence(modelEN, false))
	if !strings.Contains(fenceEN, "⇄ cross-board [") || strings.Contains(fenceEN, "never add across boards)") {
		t.Fatalf("件1(a) EN: short marker on the row, invariant in the legend only:\n%s", fenceEN)
	}
}

// TestRule3EdgeAnchorRowNeverRepeatsMechanism — 件1(b) 负臂: the mechanism
// sentence renders in the LEGEND only; no row line repeats it.
func TestRule3EdgeAnchorRowNeverRepeatsMechanism(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(r3HostEdgeAnchoredProjection(),
		newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, true))
	if strings.Contains(fence, "本席凭宿主线程自身对目标的窗内 typed 唤醒边入链上") {
		t.Fatalf("件1(b) 套话复活: the mechanism sentence must not ride a row:\n%s", fence)
	}
	legend := strings.Join(runtimeTraceProjLegendGroupLines(model.Marks, true), "\n")
	if !strings.Contains(legend, "以宿主自身对目标的窗内 typed 唤醒边为链上凭证") ||
		!strings.Contains(legend, "状态席值=状态段清单边前份合计") {
		t.Fatalf("件1(b): the legend keeps the full mechanism + both value clauses:\n%s", legend)
	}
}

// TestRule3SeatWindowHoistPositiveAndNegative — 件1(c): the same-value
// two-board report declares the window once (head line + legend entry, inline
// halves keep 板锚 only); a distinct-window report keeps inline chips and
// mints NO declaration (异值窗负臂).
func TestRule3SeatWindowHoistPositiveAndNegative(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(rule3TwoBoardProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "- 全席同窗 窗10.000~10.200s") {
		t.Fatalf("件1(c) 正臂: the head must declare the shared window once:\n%s", fence)
	}
	if strings.Contains(fence, "·窗10.000~10.200s") {
		t.Fatalf("件1(c) 正臂: the hoisted window must not restate inline:\n%s", fence)
	}
	// The detail face keeps the FULL chip bytes (无损明细).
	detail := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(detail, "窗10.000~10.200s·板锚 target.a-100") {
		t.Fatalf("件1(c): the detail face keeps the full chip:\n%s", detail)
	}
	// 异值窗负臂.
	distinct := rule3TwoBoardProjection()
	distinct.OnChainCauses[1].QueryWindowStartTs = 10.0
	distinct.OnChainCauses[1].QueryWindowEndTs = 10.3
	distinctModel := buildRuntimeTraceProjTreeModel(distinct, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	distinctFence := runtimeTraceProjTreeFence(distinctModel, true)
	if strings.Contains(distinctFence, "全席同窗") {
		t.Fatalf("件1(c) 负臂: distinct windows must not hoist:\n%s", distinctFence)
	}
	if !strings.Contains(distinctFence, "窗10.000~10.200s") || !strings.Contains(distinctFence, "窗10.000~10.300s") {
		t.Fatalf("件1(c) 负臂: distinct-window rows keep their inline chips:\n%s", distinctFence)
	}
}

// TestRule3OrdinalSingleCarrier — 件2: a badged seat's 行2 carries NO
// 根因排序#N word (徽章即序数); an un-badged ordinal row (rank #6, past the
// TOP5 badge range) keeps the word form as fallback carrier.
func TestRule3OrdinalSingleCarrier(t *testing.T) {
	projection := rule3TwoBoardProjection()
	// Six seats on one board → #6 exists un-badged.
	projection.OnChainCauses = nil
	for i := 1; i <= 6; i++ {
		projection.OnChainCauses = append(projection.OnChainCauses,
			rule3TwoBoardSeat(fmt.Sprintf("r3o-%d", i), fmt.Sprintf("worker%d-30%d", i, i),
				"runnable_wait", "", i, float64(10-i), 10*i))
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "❶") || !strings.Contains(fence, "❺") {
		t.Fatalf("件2: the TOP5 seats must wear badges:\n%s", fence)
	}
	// 佩章行无词臂: no badged ordinal word (only #6 may word).
	for i := 1; i <= 5; i++ {
		if strings.Contains(fence, fmt.Sprintf("根因排序#%d", i)) {
			t.Fatalf("件2 双载复活: badged seat #%d must not word its ordinal:\n%s", i, fence)
		}
	}
	// 未佩章行有词臂 (fold-twin 兜底词形).
	if !strings.Contains(fence, "根因排序#6") {
		t.Fatalf("件2 兜底臂: the un-badged #6 seat keeps the word form:\n%s", fence)
	}
	// 图例句: per-board issue + single-carrier + crown-caliber sentence.
	legend := strings.Join(runtimeTraceProjLegendGroupLines(model.Marks, true), "\n")
	for _, want := range []string{"按板各发", "徽章即序数", "标题主根因=选举权威", "❶=板内值序"} {
		if !strings.Contains(legend, want) {
			t.Fatalf("件2+件3 图例句 missing %q:\n%s", want, legend)
		}
	}
}

// TestRule3EpsilonOverlapDisclosure — 件4: the ε-overlap disclosure lane.
// Synthetic specimen mirrors the DHMINE live values (anchored 0.031 of full
// 31.191 = 0.099% < 1% floor): the row wears the short marker, the detail
// face discloses the absolute overlap, and every value channel is untouched.
// ≥floor negative arm: 10% anchored share wears nothing.
func TestRule3EpsilonOverlapDisclosure(t *testing.T) {
	seat := rule3TwoBoardSeat("r3e-1", "JankManager-9655", "runnable_wait", "", 1, 31.160, 10)
	seat.ChainAnchoredMS = 0.031
	seat.ChainAnchorFullMS = 31.191
	seat.ChainAnchorRemainderSeat = true
	seat.ChainRelevance = "adjacent"
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"waker-1", "VSyncGenerator-2179"},
		WindowStartTs: 13762.791708, WindowEndTs: 13763.024898,
		AdjacentCauses: []types.TraceCausalProjectionNode{seat},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, true))
	if !strings.Contains(fence, "ε 重叠 0.10%(见图例)") {
		t.Fatalf("件4 正臂: the ε marker must render on the specimen shape:\n%s", fence)
	}
	detail := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(detail, "0.10%(重叠绝对值 0.031ms,全账 31.191ms") {
		t.Fatalf("件4: the detail face must disclose the absolute overlap:\n%s", detail)
	}
	// 值零动臂: the published value is untouched beside the marker.
	if !strings.Contains(fence, "31.160ms") {
		t.Fatalf("件4 值零动臂: the seat value must render unchanged:\n%s", fence)
	}
	legend := strings.Join(runtimeTraceProjLegendGroupLines(model.Marks, true), "\n")
	if !strings.Contains(legend, "`ε 重叠 X%`") || !strings.Contains(legend, "低于披露地板 1%") {
		t.Fatalf("件4 图例: the ε entry must teach the 1%% floor:\n%s", legend)
	}
	// ≥地板负臂.
	big := seat
	big.ChainAnchoredMS = 3.2 // 10.3% of 31.191
	bigProjection := projection
	bigProjection.AdjacentCauses = []types.TraceCausalProjectionNode{big}
	bigModel := buildRuntimeTraceProjTreeModel(bigProjection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	bigFence := rspaFenceJoined(runtimeTraceProjTreeFence(bigModel, true))
	if strings.Contains(bigFence, "ε 重叠") {
		t.Fatalf("件4 负臂: an ≥floor anchored share must not wear the marker:\n%s", bigFence)
	}
	// 独立地板常数臂: 禁借 INTERFLOOR 5% / jitter 0.5ms.
	if chainSupportEpsilonOverlapDiscloseRatio != 0.01 {
		t.Fatalf("件4: the disclosure floor is the ruled independent 1%% constant, got %v",
			chainSupportEpsilonOverlapDiscloseRatio)
	}
	if chainSupportEpsilonOverlapDiscloseRatio == tracequery.RootCauseCrossDirectionOverlapDeMinimisRatio {
		t.Fatalf("件4 禁借臂: the ε floor must not alias the INTERFLOOR ratio")
	}
}

// TestRule3EpsilonOverlapLiveSpecimen — 件4 活体标本正臂 (DHMINE 0.099%,
// evalcase_dhm_watch_pin_test.go DHM-EPS twin): the donghu VSyncGenerator-2179
// board's JankManager-9655 remainder seat reaches the rendered tree wearing
// the ε marker with the live 0.10% ratio.
func TestRule3EpsilonOverlapLiveSpecimen(t *testing.T) {
	if testing.Short() {
		t.Skip("real-trace battery")
	}
	trace := "../../eval/fixtures/real_traces/donghu.ftrace"
	if _, err := os.Stat(trace); err != nil {
		t.Skipf("real-trace fixture absent: %v", err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), trace)
	if err != nil {
		t.Fatal(err)
	}
	var obs []types.ObservationRecord
	for _, view := range []string{"wakeup_chain", "root_cause_rank"} {
		q := tracequery.Query{PID: 2179, TimeStart: 13762.791708, TimeEnd: 13763.024898,
			TraceFlavorHint: tracequery.TraceFlavorHarmonyHitrace, View: view}
		result := tracequery.Run(idx, q)
		obs = append(obs, traceQueryTypedObservations(result, "fixture", "p-"+view, "r", "", time.Unix(1753000000, 0).UTC())...)
	}
	md := p3mRenderUserFace(t, obs, "zh")
	if !strings.Contains(md, "ε 重叠 0.10%") {
		t.Fatalf("件4 活体标本: the 2179 board's 0.099%% split must wear the ε marker:\n%s", md)
	}
	// 值零动臂: the remainder account renders its exact live value.
	if !strings.Contains(md, "31.160ms") {
		t.Fatalf("件4 活体标本值零动: the remainder value must stay exact:\n%s", md)
	}
}

// TestRule3InternalTermSweepExtension — 件6: 降道/车道 join the banned render
// literals — representative render surface AND the full legend catalog (the
// OMGCLEAN sweep surface does not include every legend line, so the catalog
// census is the load-bearing arm for legend-resident words).
func TestRule3InternalTermSweepExtension(t *testing.T) {
	for _, zh := range []bool{true, false} {
		projection := elimBoardProjection()
		model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		tree := runtimeTraceProjTreeFence(model, zh)
		elim := runtimeTraceProjElimOverviewFence(projection, model, zh)
		legend := strings.Join(runtimeTraceProjLegendGroupLines(model.Marks, zh), "\n")
		surface := tree + "\n" + elim + "\n" + legend
		for _, banned := range []string{"降道", "车道", "whole-seat demotion"} {
			if strings.Contains(surface, banned) {
				at := strings.Index(surface, banned)
				lo := at - 40
				if lo < 0 {
					lo = 0
				}
				t.Fatalf("件6 (zh=%v): banned reader-facing term %q near ...%s...", zh, banned, surface[lo:at+len(banned)])
			}
		}
	}
	// Full legend catalog census (both language columns).
	for _, entry := range runtimeTraceProjLegendCatalog() {
		for _, banned := range []string{"降道", "车道", "whole-seat demotion"} {
			if strings.Contains(entry.ZH, banned) || strings.Contains(entry.EN, banned) {
				t.Fatalf("件6 catalog census: banned term %q in legend entry for mark %d", banned, entry.Mark)
			}
		}
	}
}

// TestRule3WholeSeatOffBoardWording — 件6 定稿词形: the live demoted-seat row
// speaks 整席不入链上榜 (zh) / whole seat off the on-chain board (EN).
func TestRule3WholeSeatOffBoardWording(t *testing.T) {
	seat := rule3TwoBoardSeat("r3w-1", "logd.writer-9163", "runnable_wait", "", 1, 47.678, 10)
	seat.ChainRelevance = "adjacent"
	seat.ChainCredentialLaneDemoted = true
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"waker-1", "app-100"},
		WindowStartTs: 10.0, WindowEndTs: 10.2,
		AdjacentCauses: []types.TraceCausalProjectionNode{seat},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, true))
	if !strings.Contains(fence, "无链上凭证(整席不入链上榜,见图例)") {
		t.Fatalf("件6: the demoted row must speak the ruled reader words:\n%s", fence)
	}
	modelEN := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	fenceEN := rspaFenceJoined(runtimeTraceProjTreeFence(modelEN, false))
	if !strings.Contains(fenceEN, "no chain credential (whole seat off the on-chain board; see legend)") {
		t.Fatalf("件6 EN: the demoted row must speak the reader words:\n%s", fenceEN)
	}
}

// TestRule3ElimCredentialTierChips — 件9: the ◎ ⛓ seat rows wear the
// credential-tier chips (·身份继承 / ·包络凭证) on exactly the typed shapes
// the tree-face words fire on; a per-segment-credential seat wears none.
func TestRule3ElimCredentialTierChips(t *testing.T) {
	inherit := rule3TwoBoardSeat("r3c-1", "workerA-301", "runnable_wait", "", 1, 8.0, 10)
	inherit.ChainIdentityInheritance = true
	envelope := rule3TwoBoardSeat("r3c-2", "workerB-302", "d_state_or_io_wait", "", 2, 6.0, 30)
	envelope.StateKind = "d_sleep"
	envelope.ChainCredentialEnvelopeLevel = true
	segments := rule3TwoBoardSeat("r3c-3", "workerC-303", "runnable_wait", "", 3, 4.0, 50)
	segments.ChainCredentialSegments = [][2]float64{{10.01, 10.02}}
	projection := types.TraceCausalProjection{
		RootCauseFamilyObserved: true,
		WakeupPath:              []string{"waker-1", "app-100"},
		WindowStartTs:           10.0, WindowEndTs: 10.2,
		OnChainCauses: []types.TraceCausalProjectionNode{inherit, envelope, segments},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	overview := runtimeTraceProjElimOverviewFence(projection, model, true)
	lineOf := func(subject string) string {
		for _, line := range strings.Split(overview, "\n") {
			if strings.Contains(line, subject) {
				return line
			}
		}
		return ""
	}
	if line := lineOf("workerA-301"); !strings.Contains(line, "·身份继承") {
		t.Fatalf("件9 正臂: the identity-inheritance seat must wear its chip, got %q in:\n%s", line, overview)
	}
	if line := lineOf("workerB-302"); !strings.Contains(line, "·包络凭证") {
		t.Fatalf("件9 正臂: the envelope-credential seat must wear its chip, got %q in:\n%s", line, overview)
	}
	if line := lineOf("workerC-303"); strings.Contains(line, "身份继承") || strings.Contains(line, "包络凭证") {
		t.Fatalf("件9 负臂: the per-segment-credential seat wears no tier chip, got %q", line)
	}
	// 基石 B 零动: the 「已证可消除量」 legend sentence stays verbatim.
	legend := strings.Join(runtimeTraceProjLegendGroupLines(model.Marks, true), "\n")
	if !strings.Contains(legend, "链上席=已证可消除量") {
		t.Fatalf("件9 基石B: the GREENLIT sentence must stay:\n%s", legend)
	}
}

// TestRule3ConservationPopulationSentence — 件10: the 守恒 legend entry names
// its population (§29.183 G9); the §29.175.8 ✓ row form is untouched.
func TestRule3ConservationPopulationSentence(t *testing.T) {
	for _, entry := range runtimeTraceProjLegendCatalog() {
		if entry.Mark != runtimeTraceProjMarkElimConservation {
			continue
		}
		if !strings.Contains(entry.ZH, "种群=严格链上全额持值席,◇ 邻近、包络级凭证、计数当量与自身症状席不入") {
			t.Fatalf("件10: the zh conservation entry must declare its population:\n%s", entry.ZH)
		}
		if !strings.Contains(entry.EN, "population = strict on-chain full-value seats only") {
			t.Fatalf("件10 EN: the conservation entry must declare its population:\n%s", entry.EN)
		}
		return
	}
	t.Fatalf("conservation legend entry missing")
}
