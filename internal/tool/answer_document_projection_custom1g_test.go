package tool

// answer_document_projection_custom1g_test.go — V-series pins (customer trace
// revisit 2026-07-03, custom_1g.txt, docs/design/customer_dead_session_audit_
// 20260703.md §6):
//   V1 — the conclusion line consumes the engine's typed rank first, falls back
//        to single-instance effective attribution, and NEVER lets a ×N merged
//        SUM participate in selection or publish as the headline ms;
//   V2 — the coverage line's denominator is the target's own symptom duration
//        when self-state rows exist (whole window only as fallback);
//   V3 — the R3 background fold displays the member max (never a cross-thread
//        sum) and whole-window background rows carry the idle annotation;
//   V5 — the detail table's fold-row cell keeps the member roster the tree
//        fold line already shows.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// custom1gPrimaries mirrors the real customer window 6793222.7–6793222.8
// (101ms): the engine's rank=1 is a running row of 4.115ms; rank=2 is a ×7
// merged io_latency aggregate whose window-projection SUM (13.324ms) dwarfs it.
func custom1gProjection(mergedRank, runningRank int) types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WindowStartTs: 6793222.700,
		WindowEndTs:   6793222.801,
		PrimaryRootCauses: []types.TraceCausalProjectionNode{
			{
				EvidenceID: "E3", Subject: "hmfs_discard-26-562", Object: "io_latency",
				Rank: mergedRank, ImpactMS: 13.324, CumulativeImpactMS: 13.324,
				MergedCount: 7, MergedMinMS: 0.352, MergedMaxMS: 1.940,
				ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
			},
			{
				EvidenceID: "E4", Subject: "RSUniRenderThre-1963", Object: "running",
				Rank: runningRank, ImpactMS: 4.115, CumulativeImpactMS: 4.115,
				EffectiveImpactMS: 4.115, StateKind: "running",
				ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
			},
		},
	}
}

// --- V1: conclusion line selection ----------------------------------------------

func TestRuntimeTraceProjConclusionPrefersEngineRankOverMergedSum(t *testing.T) {
	projection := custom1gProjection(2, 1)
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	line := runtimeTraceProjConclusionLine(projection, model, true)
	// The customer render named the rank=2 ×7 sum (13.324ms) as 主根因 and
	// contradicted its own VSync/GPU/binder narrative — the engine's rank=1
	// running row must win.
	if !strings.Contains(line, "RSUniRenderThre-1963") || !strings.Contains(line, "4.115ms") {
		t.Fatalf("conclusion must consume the engine's rank=1 row:\n%s", line)
	}
	for _, banned := range []string{"hmfs_discard", "13.324"} {
		if strings.Contains(line, banned) {
			t.Fatalf("the rank=2 merged sum must not be the conclusion (%q):\n%s", banned, line)
		}
	}
	en := runtimeTraceProjConclusionLine(projection, buildRuntimeTraceProjTreeModel(projection, nil, false), false)
	if !strings.Contains(en, "RSUniRenderThre-1963") || strings.Contains(en, "13.324") {
		t.Fatalf("EN conclusion must fork the same way:\n%s", en)
	}
}

func TestRuntimeTraceProjConclusionMergedLeadShowsSingleMaxNotSum(t *testing.T) {
	// When the merged family really leads the post-aggregation board, the
	// headline ms is the per-instance max with the count — the ×N SUM never
	// publishes as the hard fact (S1 同族裁定), and the window share follows
	// the same value.
	//
	// COV+LEAD 批 (§24.11 C-1, 2026-07-08). EVOLUTION RECORD: the lead election
	// now consumes the shared post-aggregation rank board (the ❶ badge lane's
	// population and key — EffectiveImpactMS), so a window-local rank ordinal
	// no longer overrides the board (rank chips collide across query windows:
	// huadong_78 carried two #1). The fixture therefore gives the merged
	// family the board-leading attribution (its 单次最大 caliber, §24.2) so
	// the merged-lead FORMATTING lane stays pinned; the rank-tie shape this
	// test previously encoded is covered by the board-population pins in
	// answer_document_projection_covlead_test.go.
	projection := custom1gProjection(1, 2)
	projection.PrimaryRootCauses[0].EffectiveImpactMS = 1.940
	projection.PrimaryRootCauses[1].EffectiveImpactMS = 0.415
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	line := runtimeTraceProjConclusionLine(projection, model, true)
	if !strings.Contains(line, "单次最大 1.940ms(共7次)(占窗2%)") {
		t.Fatalf("merged lead must show the single-instance max with count:\n%s", line)
	}
	if strings.Contains(line, "13.324") {
		t.Fatalf("the ×7 sum must not publish on the conclusion line:\n%s", line)
	}
	en := runtimeTraceProjConclusionLine(projection, buildRuntimeTraceProjTreeModel(projection, nil, false), false)
	if !strings.Contains(en, "single max 1.940ms (of 7) (2% of window)") || strings.Contains(en, "13.324") {
		t.Fatalf("EN merged lead must show the single-instance form:\n%s", en)
	}
}

func TestRuntimeTraceProjConclusionRanklessFallbackIgnoresMergedSum(t *testing.T) {
	// No candidate carries a rank: selection falls back to the single-instance
	// effective attribution — the merged row competes with its per-instance max
	// (1.940), never its SUM (13.324), so the 4.115ms running row wins.
	projection := custom1gProjection(0, 0)
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	line := runtimeTraceProjConclusionLine(projection, model, true)
	if !strings.Contains(line, "RSUniRenderThre-1963") {
		t.Fatalf("rankless fallback must order by single-instance attribution:\n%s", line)
	}
	if strings.Contains(line, "hmfs_discard") {
		t.Fatalf("the merged SUM must not win the rankless fallback:\n%s", line)
	}
}

// --- V2: coverage denominator = target symptom duration --------------------------

func custom1gWindowModel(withSelfRows bool) runtimeTraceProjTreeModel {
	model := runtimeTraceProjTreeModel{
		Target:   "render_service-1963",
		WindowMS: 101.0,
		TreeRows: []runtimeTraceProjTreeRow{
			{Kind: runtimeTraceProjTreeRowChain, Depth: 1, HasData: true,
				Node: types.TraceCausalProjectionNode{Subject: "waker-1", CumulativeImpactMS: 3.391}},
		},
	}
	if withSelfRows {
		// Customer shape: the 🎯 target slept/blocked 11.716ms of the 101ms
		// window (5.925 sleep + 5.791 D-state own-state segments — both in the
		// symptom family; the running-excluded / runnable-included split is
		// pinned in the F-round tests, RN-6 §7.9).
		model.SelfRows = []runtimeTraceProjTreeRow{
			{Kind: runtimeTraceProjTreeRowSelf, HasData: true,
				Node: types.TraceCausalProjectionNode{Subject: "render_service-1963", ImpactMS: 5.925, StateKind: "s_sleep"}},
			{Kind: runtimeTraceProjTreeRowSelf, HasData: true,
				Node: types.TraceCausalProjectionNode{Subject: "render_service-1963", ImpactMS: 5.791, StateKind: "d_state"}},
		}
	}
	return model
}

func TestRuntimeTraceProjWindowLineTargetSymptomDenominator(t *testing.T) {
	projection := types.TraceCausalProjection{WindowStartTs: 6793222.700, WindowEndTs: 6793222.801}
	line := runtimeTraceProjWindowLine(projection, custom1gWindowModel(true), true)
	// (Wording pin updated for RN-6 §7.9: the denominator family now includes
	// runnable, so the label reads the wait-family parenthetical.)
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: 目标等待 → 关注线程等待
	// (其他族); on-chain 已归因 → 链上已归因 (归因族).
	if !strings.Contains(line, "关注线程等待(sleep/D-state/runnable) 11.716ms 中链上已归因 3.391ms(29%),未归因 8.325ms(71%)") {
		t.Fatalf("coverage must use the target symptom duration as denominator:\n%s", line)
	}
	// The misleading whole-window residual ("残差 97%") must be gone.
	for _, banned := range []string{"97%", "残差"} {
		if strings.Contains(line, banned) {
			t.Fatalf("whole-window residual wording must not render with self rows (%q):\n%s", banned, line)
		}
	}
	en := runtimeTraceProjWindowLine(projection, custom1gWindowModel(true), false)
	if !strings.Contains(en, "Of the focused thread's 11.716ms wait time (sleep/D-state/runnable), on-chain attributed 3.391ms (29%), unattributed 8.325ms (71%)") {
		t.Fatalf("EN coverage must use the symptom denominator:\n%s", en)
	}
}

func TestRuntimeTraceProjWindowLineFallsBackToWholeWindowWithoutSelfRows(t *testing.T) {
	projection := types.TraceCausalProjection{WindowStartTs: 6793222.700, WindowEndTs: 6793222.801}
	// No self-state rows → the whole-window fallback arm renders byte-stably.
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: on-chain 已归因 →
	// 链上已归因 (归因族); 未归因残差 → 未归因 (归因族); 目标等待 → 关注线程等待
	// (其他族); 归因口径合计 → 各链上口径合计 (归因族).
	// COV 批 (§24.11 C-3 后半场, 2026-07-08). EVOLUTION RECORD: 各链上口径合计 →
	// 链上单项最大 — the numerator is a single-row MAX, never a sum (墙钟红线);
	// "合计" 冒名退役 (huadong_78: "各链上口径合计 4.431ms" = 单行 E15).
	line := runtimeTraceProjWindowLine(projection, custom1gWindowModel(false), true)
	if !strings.Contains(line, "链上已归因 3.391ms(3%),未归因 97.609ms(97%)") {
		t.Fatalf("fallback branch wording must stay unchanged:\n%s", line)
	}
	// §15.D gap③ (P0-A1, overturns the V2 pin that stood here): attribution
	// exceeding the symptom duration KEEPS the symptom denominator — the old
	// whole-window demotion fabricated residual (q6: 112.223ms lock span
	// 0.048ms past a 112.175ms sleep → "94% + 6.838ms 未归因残差"). A 0.391ms
	// excess is boundary jitter → full coverage + verbatim caliber disclosure;
	// the regime pins live in answer_document_projection_p0a1_coverage_test.go.
	model := custom1gWindowModel(true)
	model.SelfRows[0].Node.ImpactMS = 1.0
	model.SelfRows[1].Node.ImpactMS = 2.0
	line = runtimeTraceProjWindowLine(projection, model, true)
	if !strings.Contains(line, "关注线程等待(sleep/D-state/runnable) 3.000ms 中链上已归因 3.000ms(100%)") ||
		!strings.Contains(line, "链上单项最大 3.391ms,略超关注线程等待 0.391ms") {
		t.Fatalf("attribution above the symptom must keep the symptom denominator (§15.D gap③):\n%s", line)
	}
	// Old-form guard (未归因残差 retired) + new-form whole-window residual figure.
	for _, banned := range []string{"未归因残差", "未归因 97.609ms"} {
		if strings.Contains(line, banned) {
			t.Fatalf("the whole-window residual claim must not render with a symptom denominator (%q):\n%s", banned, line)
		}
	}
}

// --- V3: whole-window background annotation + fold-row max form ------------------

func TestRuntimeTraceProjBackgroundWholeWindowIdleAnnotation(t *testing.T) {
	mkRow := func(impact float64) runtimeTraceProjTreeRow {
		return runtimeTraceProjTreeRow{
			Kind: runtimeTraceProjTreeRowBackground, HasData: true,
			Node: types.TraceCausalProjectionNode{
				Subject: "audio_server-77", Object: "unknown-thread",
				ImpactMS: impact, ChainRelevance: "background",
				// F3: the idle annotation is gated on the typed wait-family
				// StateKind (running/stateless whole-window rows are pinned as
				// UNTAGGED in the F-round tests).
				StateKind: "s_sleep",
			},
		}
	}
	// A background thread waiting out the whole 101ms window reads as idle.
	// NEW-10 (§7.6): under the 100-cell row cap the annotation may display-
	// truncate to its protected marker phrase on the fence row; the full text
	// stays pinned on the detail-table mirror below.
	line := runtimeTraceProjStanzaRowLine(mkRow(101.0), runtimeTraceProjTreeLabelWidth, 101.0, true, true)
	if !strings.Contains(line, "整窗等待") {
		t.Fatalf("whole-window background row must carry the idle annotation:\n%s", line)
	}
	en := runtimeTraceProjStanzaRowLine(mkRow(101.0), runtimeTraceProjTreeLabelWidth, 101.0, true, false)
	if !strings.Contains(en, "whole-window wait") {
		t.Fatalf("EN idle annotation missing:\n%s", en)
	}
	// Over-window cumulative rows are the H8 multi-CPU shape — an ACTIVE burst,
	// never annotated idle. PTV4 T4: the H8 explanation lives in the legend's
	// 占窗>100% entry (typed OverWindowShare mark); the row keeps the bare %.
	overRow := mkRow(204.382)
	overMarks := &runtimeTraceProjMarkSet{}
	overRow.marks = overMarks
	over := runtimeTraceProjStanzaRowLine(overRow, runtimeTraceProjTreeLabelWidth, 101.0, true, true)
	if strings.Contains(over, "疑似空闲") || strings.Contains(over, "整窗等待") {
		t.Fatalf("over-window row must not be annotated idle:\n%s", over)
	}
	if !overMarks.has(runtimeTraceProjMarkOverWindowShare) {
		t.Fatalf("over-window row must record the H8 legend mark:\n%s", over)
	}
	// Mid-window rows carry neither.
	mid := runtimeTraceProjStanzaRowLine(mkRow(50.0), runtimeTraceProjTreeLabelWidth, 101.0, true, true)
	if strings.Contains(mid, "整窗等待") {
		t.Fatalf("a 50%% row must not be annotated idle:\n%s", mid)
	}
	// The lossless surface mirrors the FULL annotation on the (b) blocks'
	// impact-shape line (PTV4 T10; the fence keeps the bare 整窗等待 token).
	model := runtimeTraceProjTreeModel{WindowMS: 101.0, Background: []runtimeTraceProjTreeRow{mkRow(101.0)}}
	if full := runtimeTraceProjDetailFullText(model, true); !strings.Contains(full, "整窗等待(疑似空闲)") {
		t.Fatalf("detail blocks must mirror the idle annotation:\n%s", full)
	}
}

func custom1gFoldNode() types.TraceCausalProjectionNode {
	// The R3 subjectless fold of six whole-window background threads — the
	// customer render summed the members to 606.000ms/600%.
	return types.TraceCausalProjectionNode{
		EvidenceID: "E16", Object: "unknown-thread", ChainRelevance: "background",
		MergedCount: 6, MergedMinMS: 99.500, MergedMaxMS: 101.000,
		ImpactMS: 101.000, CumulativeImpactMS: 101.000, // member max since V3
		MergedSubjects: []string{"a-1", "b-2", "c-3", "d-4"},
		// Unanimous member state propagated by the R3 fold since F3 — the idle
		// annotation is gated on the typed wait family, so a stateless fold
		// would (correctly) render untagged.
		StateKind: "s_sleep",
	}
}

func TestRuntimeTraceProjBackgroundFoldRowShowsMaxFormNotSumForm(t *testing.T) {
	row := runtimeTraceProjTreeRow{
		Kind: runtimeTraceProjTreeRowBackground, HasData: true, Node: custom1gFoldNode(),
	}
	_, tags := runtimeTraceProjRowMetricParts(row, 101.0, true, true)
	var joined []string
	for _, tag := range tags {
		joined = append(joined, tag.Text)
	}
	flat := strings.Join(joined, " · ")
	// PTV4 T4 (×N 三式): the max form is the N线程取最大(单项a~b) data token — the
	// 不求和 semantics live in the legend's 口径组 entry.
	if !strings.Contains(flat, "6线程取最大(单项99.500~101.000ms)") {
		t.Fatalf("fold row must declare the max form:\n%s", flat)
	}
	if strings.Contains(flat, "单次") {
		t.Fatalf("fold row must not reuse the SUM-aggregate tag form:\n%s", flat)
	}
	// A whole-window fold is the idle shape too — both annotations coexist.
	if !strings.Contains(flat, "整窗等待") {
		t.Fatalf("whole-window fold must carry the idle annotation:\n%s", flat)
	}
	// Same-thread R2 aggregates keep the sum form untouched.
	sum := custom1gFoldNode()
	sum.Subject = "worker-9"
	sum.MergedSubjects = nil
	sumRow := runtimeTraceProjTreeRow{Kind: runtimeTraceProjTreeRowAdjacent, HasData: true, Node: sum}
	_, sumTags := runtimeTraceProjRowMetricParts(sumRow, 101.0, true, true)
	var sumFlat []string
	for _, tag := range sumTags {
		sumFlat = append(sumFlat, tag.Text)
	}
	if !strings.Contains(strings.Join(sumFlat, " · "), "6次(99.500~101.000ms)") {
		t.Fatalf("subject-bearing aggregate must keep the sum form: %v", sumFlat)
	}
	if strings.Contains(strings.Join(sumFlat, " · "), "取最大") {
		t.Fatalf("subject-bearing aggregate must not claim the max form: %v", sumFlat)
	}
}

// --- V5: detail-table fold-row roster ---------------------------------------------

func TestRuntimeTraceProjDetailTableFoldRowKeepsRoster(t *testing.T) {
	model := runtimeTraceProjTreeModel{
		WindowMS: 101.0,
		Background: []runtimeTraceProjTreeRow{{
			Kind: runtimeTraceProjTreeRowBackground, HasData: true, Node: custom1gFoldNode(),
		}},
	}
	_, rows := runtimeTraceProjDetailTable(model, true)
	if len(rows) != 1 {
		t.Fatalf("expected the fold row: %+v", rows)
	}
	// PTV4 T10 (a): the node cell carries the max-form data token; the FULL
	// member roster lives in the (b) blocks' 合并明细 line (更无损: the roster
	// is no longer subject to the name-cell cap).
	if cell := rows[0].Cells[0]; !strings.Contains(cell, "6线程取最大(单项99.500~101.000ms)") {
		t.Fatalf("fold cell must carry the max-form count: %q", cell)
	}
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: ×6 取最大口径(…,不求和)
	// → ×6 跨线程折叠取最大(墙钟跨线程不可加和) (合并明细族); 成员: → 成员(共6,列4):
	// (截断 roster 自报账).
	full := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(full, "6线程取最大(墙钟跨线程不可加和),各 99.500~101.000ms;成员(共6,列4): a-1、b-2、c-3、d-4 等") {
		t.Fatalf("(b) blocks must carry the full member roster:\n%s", full)
	}
	if enFull := runtimeTraceProjDetailFullText(model, false); !strings.Contains(enFull, "members (6 total, 4 listed): a-1, b-2, c-3, d-4, …") {
		t.Fatalf("EN (b) blocks must carry the roster:\n%s", enFull)
	}
}

// --- V3+V4 end-to-end: compile → model → fence -----------------------------------

// TestRuntimeTraceProjCustom1gEndToEndFoldAndDedup runs the real record
// compile: three duplicate irq publications dedup BEFORE R2 (no ×3 sum), and
// six whole-window unknown-object background threads fold to max with the idle
// annotation (no 606ms/600%).
func TestRuntimeTraceProjCustom1gEndToEndFoldAndDedup(t *testing.T) {
	mkBG := func(id, subject string) types.ObservationRecord {
		return projV3Obs(id, "root_cause_background", "root_cause_background:"+id,
			subject, "unknown-thread", "101.000", 101.0, 1000, 2000,
			"chain_relevance=background", "causality=background",
			// The producer's Q2 whole-window-sleeper signal: since F3 the idle
			// annotation requires this typed dominant state (the R3 fold row
			// inherits it by strict member unanimity).
			"dominant_state=s_sleep")
	}
	mkIRQ := func(id string, lineStart, lineEnd int) types.ObservationRecord {
		return projV3Obs(id, "root_cause_context", "root_cause_context:"+id,
			"irq_handler-100", "irq_activity", "35.350", 35.350, lineStart, lineEnd,
			"chain_relevance=adjacent", "causality=adjacent_to_chain")
	}
	records := []types.ObservationRecord{
		{
			ID: "anchor", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard, Predicate: "frame_target_resolution", ClaimKey: "frame_target_resolution:f",
			Subject: "render_service-1963", Object: "frame",
			Span:      types.ObservationSpan{StartTs: 6793222.700, EndTs: 6793222.801},
			RichNotes: []string{"window_source=query_window"},
		},
		mkIRQ("E13a", 793201, 830007),
		mkIRQ("E13b", 793204, 830012),
		mkIRQ("E13c", 793199, 830001),
		mkBG("E16", "bg-1"), mkBG("E17", "bg-2"), mkBG("E18", "bg-3"),
		mkBG("E19", "bg-4"), mkBG("E20", "bg-5"), mkBG("E21", "bg-6"),
	}
	projection := types.TraceCausalProjectionFromObservationRecords(records)
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	fence := runtimeTraceProjTreeFence(model, true)
	// V4: one 35.350ms row carrying the typed publication count; never a sum.
	// (The fence may width-elide the Extra-class ×3 tag under B4 — the detail
	// table below is the lossless surface for it, same policy as before V4.)
	if len(model.Adjacent) != 1 || model.Adjacent[0].Node.DuplicatePublications != 3 ||
		model.Adjacent[0].Node.ImpactMS != 35.350 {
		t.Fatalf("duplicate publications must fold to one ±0 row: %+v", model.Adjacent)
	}
	_, detailRows := runtimeTraceProjDetailTable(model, true)
	detailFlat := ""
	for _, item := range detailRows {
		detailFlat += strings.Join(item.Cells, " | ") + "\n"
	}
	if !strings.Contains(detailFlat, "3次同值") {
		t.Fatalf("detail table must carry the duplicate-publication label:\n%s", detailFlat)
	}
	for _, banned := range []string{"106.05", "3次(35.350"} {
		if strings.Contains(fence, banned) || strings.Contains(detailFlat, banned) {
			t.Fatalf("duplicate publications must not SUM (%q):\n%s\n%s", banned, fence, detailFlat)
		}
	}
	// V3: the background fold (keep-2 + fold-4) publishes the member max with
	// the idle annotation — six 101ms threads never render as 404/606ms.
	// NEW-10: the fence row keeps at least the protected 整窗等待 marker; the
	// detail table stays the lossless surface for the full annotation.
	if !strings.Contains(fence, "其余 4 项合并") || !strings.Contains(fence, "整窗等待") {
		t.Fatalf("background fold must render with roster + idle annotation:\n%s", fence)
	}
	for _, banned := range []string{"404.000", "606.000"} {
		if strings.Contains(fence, banned) {
			t.Fatalf("cross-thread sums must never render (%q):\n%s", banned, fence)
		}
	}
}
