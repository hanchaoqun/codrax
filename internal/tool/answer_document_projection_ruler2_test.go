package tool

// answer_document_projection_ruler2_test.go — RULER2-1 (§29.150② user ruling
// / R-19-b, 2026-07-19) display pins: the self runnable account two-ruler
// cross-row sentence — 行2 sub-line on the LEAD seat row (fixture home for
// the representative-shapes bidirectional harness entry
// ruler2_two_ruler_accounting).
//
// Spec (ruler2_spec.md 定形): typed admission = same-thread self runnable
// seats spanning BOTH closed rulers; single-ruler boards silent; carriers
// absent silent; word-face single point; NO cross-ruler sum value anywhere
// (inverse string-ban pin — Σ6.797 is a mixed-ruler number, M3 禁混尺);
// same-ruler subtotal µs identity (3.956+1.193=5.149 on the witness);
// idempotent re-render; legend zh/EN bidirectional; donghu17267 production
// witness verbatim.

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// ruler2TwoRulerProjection — representative shape: the target's own runnable
// account split across both rulers (two self-wall-clock seats #2/#5 + one
// edge-anchored seat #4) with the matching side-channel record (subtotals =
// exact same-ruler sums).
func ruler2TwoRulerProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:              []string{"dep-7", "app-100"},
		WindowStartTs:           6.000,
		WindowEndTs:             6.010,
		RootCauseFamilyObserved: true,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleRootCauseContext, Subject: "dep-7",
				Object: "d_state_or_io_wait", StateKind: "d_sleep", ChainRelevance: "on_chain",
				Causality: "on_wakeup_chain", ImpactMS: 5.0, EffectiveImpactMS: 5.0,
				Rank: 1, Confidence: 0.8, StartTs: 6.001, EndTs: 6.002},
			{Role: types.TraceCausalRoleRootCauseContext, Subject: "app-100",
				Object: "runnable_wait", StateKind: "runnable", ChainRelevance: "on_chain",
				Causality: "self_wall_clock", OnChainBasis: "self_wall_clock_interval",
				ImpactMS: 3.0, EffectiveImpactMS: 3.0, CumulativeImpactMS: 3.0,
				Rank: 2, Confidence: 0.7, StartTs: 6.002, EndTs: 6.004},
			{Role: types.TraceCausalRoleRootCauseContext, Subject: "app-100",
				Object: "runnable_wait", StateKind: "runnable", ChainRelevance: "on_chain",
				Causality: "on_wakeup_chain",
				ImpactMS:  2.0, EffectiveImpactMS: 2.0, CumulativeImpactMS: 2.0,
				Rank: 4, Confidence: 0.7, StartTs: 6.005, EndTs: 6.006},
			{Role: types.TraceCausalRoleRootCauseContext, Subject: "app-100",
				Object: "runnable_wait", StateKind: "runnable", ChainRelevance: "on_chain",
				Causality: "self_wall_clock",
				ImpactMS:  1.0, EffectiveImpactMS: 1.0, CumulativeImpactMS: 1.0,
				Rank: 5, Confidence: 0.7, StartTs: 6.007, EndTs: 6.008},
		},
		SelfRunnableTwoRulerAccountings: []types.TraceCausalProjectionSelfRunnableTwoRuler{{
			Subject:    "app-100",
			WallEffsMS: []float64{3.0, 1.0}, WallRanks: []int{2, 5}, WallSubtotalMS: 4.0,
			EdgeEffsMS: []float64{2.0}, EdgeRanks: []int{4}, EdgeSubtotalMS: 2.0,
		}},
	}
}

const ruler2SyntheticSentenceZH = "自身runnable账按两把尺记账:自身墙钟尺 2 席(3.000+1.000=4.000ms,同尺内可加)·唤醒边锚尺 1 席(2.000ms,另一把尺);跨尺不相加,无合计数"
const ruler2SyntheticSentenceEN = "self runnable account split across two rulers: self wall-clock ruler 2 seats (3.000+1.000=4.000ms, additive within one ruler) · wakeup-edge-anchored ruler 1 seat (2.000ms, the other ruler); never additive across rulers, no combined total"

// TestRuler2SyntheticSentenceZHAndEN — the 行2 sentence renders VERBATIM on
// the LEAD seat row (#2) in both languages, exactly once, and the family
// mark records.
func TestRuler2SyntheticSentenceZHAndEN(t *testing.T) {
	for _, zh := range []bool{true, false} {
		proj := ruler2TwoRulerProjection()
		model := buildRuntimeTraceProjTreeModel(proj, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		fence := partsplitSquash(runtimeTraceProjTreeFence(model, zh))
		want := ruler2SyntheticSentenceZH
		if !zh {
			want = ruler2SyntheticSentenceEN
		}
		if got := strings.Count(fence, partsplitSquash(want)); got != 1 {
			t.Fatalf("zh=%v: the two-ruler sentence must render VERBATIM exactly once, got %d:\n%s", zh, got, fence)
		}
		if !model.Marks.has(runtimeTraceProjMarkSelfRunnableTwoRuler) {
			t.Fatalf("zh=%v: the family mark must record at the emission site", zh)
		}
	}
}

// TestRuler2DisplayAdmissionAndStampNegatives — the render-time typed gates:
// a single-ruler record, a drifted same-ruler subtotal, a missing host row
// and an ambiguous host all stay SILENT (宁漏勿假指 / 缺载体静默).
func TestRuler2DisplayAdmissionAndStampNegatives(t *testing.T) {
	render := func(proj types.TraceCausalProjection) (string, *runtimeTraceProjMarkSet) {
		model := buildRuntimeTraceProjTreeModel(proj, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
		return runtimeTraceProjTreeFence(model, true), model.Marks
	}
	head := runtimeTraceProjSelfTwoRulerHead(true)

	// Single-ruler record (edge side empty) — the admission gate refuses.
	proj := ruler2TwoRulerProjection()
	proj.SelfRunnableTwoRulerAccountings[0].EdgeEffsMS = nil
	proj.SelfRunnableTwoRulerAccountings[0].EdgeRanks = nil
	proj.SelfRunnableTwoRulerAccountings[0].EdgeSubtotalMS = 0
	if fence, marks := render(proj); strings.Contains(fence, head) || marks.has(runtimeTraceProjMarkSelfRunnableTwoRuler) {
		t.Fatalf("单尺记录必须静默:\n%s", fence)
	}

	// Drifted same-ruler subtotal — the µs identity re-check refuses.
	proj = ruler2TwoRulerProjection()
	proj.SelfRunnableTwoRulerAccountings[0].WallSubtotalMS = 4.01
	if fence, marks := render(proj); strings.Contains(fence, head) || marks.has(runtimeTraceProjMarkSelfRunnableTwoRuler) {
		t.Fatalf("同尺小计恒等破缺必须静默:\n%s", fence)
	}

	// No host row (lead ordinal absent from the board) — the stamp refuses.
	proj = ruler2TwoRulerProjection()
	proj.OnChainCauses = append(proj.OnChainCauses[:1], proj.OnChainCauses[2:]...)
	if fence, marks := render(proj); strings.Contains(fence, head) || marks.has(runtimeTraceProjMarkSelfRunnableTwoRuler) {
		t.Fatalf("缺载体必须静默:\n%s", fence)
	}

	// Value-diverged lead row (record value ≠ row value) — the µs host
	// match refuses (an inherited or drifted record never lies).
	proj = ruler2TwoRulerProjection()
	proj.SelfRunnableTwoRulerAccountings[0].WallEffsMS = []float64{3.2, 1.0}
	proj.SelfRunnableTwoRulerAccountings[0].WallSubtotalMS = 4.2
	if fence, marks := render(proj); strings.Contains(fence, head) || marks.has(runtimeTraceProjMarkSelfRunnableTwoRuler) {
		t.Fatalf("值失合记录必须静默:\n%s", fence)
	}
}

// --- Production witness (donghu17267 flagship board, verbatim capture) -------

const ruler2WitnessSentenceZH = "自身runnable账按两把尺记账:自身墙钟尺 2 席(3.956+1.193=5.149ms,同尺内可加)·唤醒边锚尺 1 席(1.648ms,另一把尺);跨尺不相加,无合计数"

// ruler2DonghuWitnessMarkdown renders the donghu17267 flagship board through
// the FULL production chain (BuildIndex → typed observations → projection
// compile → zh render), applying the answer mutation `applies` times on ONE
// bus (the idempotence arm re-applies — the fence is recomposed from the
// typed model, so a replay must not stack the sentence).
func ruler2DonghuWitnessMarkdown(t *testing.T, applies int) string {
	t.Helper()
	idx, err := tracequery.BuildIndex(context.Background(), elimSemanticDonghuTrace)
	if err != nil {
		t.Fatal(err)
	}
	query := tracequery.Query{PID: 17267, TimeStart: 13762.791708, TimeEnd: 13763.024898,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: tracequery.TraceFlavorHarmonyHitrace, Limit: 12}
	at := time.Unix(1751600000, 0).UTC()
	var obs []types.ObservationRecord
	for _, view := range []string{"wakeup_chain", "root_cause_rank"} {
		q := query
		q.View = view
		result := tracequery.Run(idx, q)
		obs = append(obs, traceQueryTypedObservations(result, "fixture", "p-"+view, "r", "", at)...)
	}
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck,
	}}
	bus.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: obs}}
	for i := 0; i < applies; i++ {
		doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "RULER2-1 witness。"}}}
		res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
		if err != nil || !res.Success {
			t.Fatalf("apply %d: %v %s", i+1, err, res.Summary)
		}
	}
	return render.RenderAnswerDocument(bus.Mutable.AnswerDocumentV2(), "zh")
}

// TestRuler2Donghu17267WitnessVerbatim — the production acceptance pin: the
// three-seat split's cross-row sentence renders VERBATIM exactly once under
// the lead seat row, the legend entry renders, the same-ruler subtotal face
// is arithmetic-consistent, and NO cross-ruler sum appears anywhere on the
// report (inverse ban: 6.797 is the mixed-ruler number — M3).
func TestRuler2Donghu17267WitnessVerbatim(t *testing.T) {
	if testing.Short() {
		t.Skip("real-trace witness")
	}
	if _, err := os.Stat(elimSemanticDonghuTrace); err != nil {
		t.Skipf("golden fixture not present: %v", err)
	}
	md := ruler2DonghuWitnessMarkdown(t, 1)
	squashed := partsplitSquash(md)
	if got := strings.Count(squashed, partsplitSquash(ruler2WitnessSentenceZH)); got != 1 {
		t.Fatalf("witness sentence must render VERBATIM exactly once, got %d:\n%s", got, md)
	}
	// The sentence hangs under the LEAD seat row (三席中首席行下披露行): the
	// #5 seat row precedes it, and the sentence sits inside the self stanza.
	leadAt := strings.Index(squashed, partsplitSquash("❺ ⧖ 自身·runnable 合计3.956ms [E6]"))
	sentenceAt := strings.Index(squashed, partsplitSquash(ruler2WitnessSentenceZH))
	if leadAt < 0 || sentenceAt < leadAt {
		t.Fatalf("the sentence must render under the lead seat row (lead@%d sentence@%d):\n%s", leadAt, sentenceAt, md)
	}
	// 图例词条 (zh; the EN face is pinned by the bidirectional sweep +
	// synthetic EN verbatim pin).
	if !strings.Contains(squashed, partsplitSquash("`自身runnable账按两把尺记账` = 目标线程自身的 runnable 席分属两把已闭合的尺")) {
		t.Fatalf("the legend entry must render with the sentence:\n%s", md)
	}
	// M3 inverse ban: the cross-ruler sum value NEVER appears on any face of
	// the report — 6.797 (=3.956+1.193+1.648) is a mixed-ruler number.
	if strings.Contains(md, "6.797") {
		t.Fatalf("M3 禁混尺: the cross-ruler sum 6.797 must never appear:\n%s", md)
	}
	// The witness board still publishes the honest per-seat values and the
	// full-window account (值通道零动).
	for _, want := range []string{"3.956", "1.193", "1.648", "5.604"} {
		if !strings.Contains(md, want) {
			t.Fatalf("published value %q must stay untouched:\n%s", want, md)
		}
	}
}

// TestRuler2IdempotentReRender — 咽喉幂等纪律: re-applying the SAME answer
// mutation must not stack the sentence (double-render red arm).
func TestRuler2IdempotentReRender(t *testing.T) {
	if testing.Short() {
		t.Skip("real-trace witness")
	}
	if _, err := os.Stat(elimSemanticDonghuTrace); err != nil {
		t.Skipf("golden fixture not present: %v", err)
	}
	md := ruler2DonghuWitnessMarkdown(t, 2)
	squashed := partsplitSquash(md)
	if got := strings.Count(squashed, partsplitSquash(ruler2WitnessSentenceZH)); got != 1 {
		t.Fatalf("re-render must not stack the sentence, got %d occurrence(s):\n%s", got, md)
	}
	if got := strings.Count(squashed, partsplitSquash("`自身runnable账按两把尺记账`")); got != 1 {
		t.Fatalf("re-render must not stack the legend entry, got %d occurrence(s):\n%s", got, md)
	}
}
