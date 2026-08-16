package tool

// answer_document_projection_small3_test.go — SMALL3-1 batch pins (§29.196
// ②③④⑥, 2026-07-21).
//
// Mutation ledger (突变臂, cp 纪律, red logs in the batch workspace):
//   M-件1 re-gluing the badge+[E#] compound reds TestA2BadgeCompoundTokenSpace
//         (answer_document_projection_a2_test.go — the badge-geometry family);
//   M-件2 dropping the ⌗ flat-tail sink reds TestA2FlatLaneCaliberSideSinksToTail
//         (same file — the flat-lane family);
//   M-件3 dropping the ◎ seal-time fold reds TestSmall3ElimFenceWidthCensusZero;
//   M-件4 dropping the aggregate fold-reason lift reds
//         TestWakeupAggregateSupplyFoldCarriesFreqOnlyReason (tracequery) and
//         TestSmall3FoldReasonTwinHealedTiebaWitness below;
//   M-件5 dropping the 未入榜最大 TOP5 sort+cap+tail reds
//         TestSmall3UnrankedMaxTop5.

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mattn/go-runewidth"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// small3UnfoldElim joins ⤷ fold continuations back onto their host line
// (件3 seal-time ◎ fold; no other transformation) so single-line content
// assertions written before §29.196④ keep judging byte-whole row content.
// small3SquashSpaces removes every space — the space-insensitive compare for
// assertions whose wanted phrase spans a fold break space (the emitted line
// drops break spaces, 件①(f), so an unfold cannot restore them byte-exactly).
func small3SquashSpaces(s string) string {
	return strings.ReplaceAll(s, " ", "")
}

func small3UnfoldElim(fence string) string {
	var out []string
	for _, line := range strings.Split(fence, "\n") {
		trimmed := strings.TrimLeft(line, " ")
		if cont, ok := strings.CutPrefix(trimmed, "⤷ "); ok && len(out) > 0 {
			out[len(out)-1] += cont
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// TestSmall3ElimFenceWidthCensusZero — 件3 (§29.196④ settle of the A2R-8
// filing) census-zero arm: EVERY ◎ fence physical line holds the 100-cell row
// budget — the structural single-row families (bar seat rows, ▸ direction
// heads, the ▒ zone head, ◈ rows) that the note governor never routed now
// fold through the shared A2 ⤷ device at seal time (same breakpoint
// whitelist), byte-whole. The fixture forces an over-budget seat row (the
// A2R-8 witness family: customer runnable_2 157-160-cell rows).
func TestSmall3ElimFenceWidthCensusZero(t *testing.T) {
	long := elimChainNode("E-wide", "ThreadPoolForeg-60555-under-a-deliberately-long-comm", "runnable_wait", "runnable", 1, 23.994, 100)
	long.MergedCount = 4
	long.MergedMaxMS = 9.0
	projection := elimBoardProjection()
	projection.OnChainCauses = append(projection.OnChainCauses, long)
	for _, zh := range []bool{true, false} {
		model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		fence := runtimeTraceProjElimOverviewFence(projection, model, zh)
		if fence == "" {
			t.Fatalf("件3 zh=%v: fixture drifted — no ◎ fence rendered", zh)
		}
		sawCont := false
		for _, line := range strings.Split(fence, "\n") {
			if w := runewidth.StringWidth(line); w > runtimeTraceProjTreeRowMaxWidth {
				t.Fatalf("件3 zh=%v census: ◎ line over the row budget (w=%d): %q", zh, w, line)
			}
			if strings.Contains(line, "⤷ ") {
				sawCont = true
			}
		}
		if !sawCont {
			t.Fatalf("件3 zh=%v: fixture drifted — the over-budget seat row no longer folds (no ⤷ continuation):\n%s", zh, fence)
		}
		// Byte-wholeness across the fold: joining the ⤷ continuations back
		// (marker + boundary spaces stripped) preserves the row's value and
		// its [E#] pointer.
		var joined strings.Builder
		for _, line := range strings.Split(fence, "\n") {
			part := strings.TrimSpace(line)
			part = strings.TrimPrefix(part, "⤷ ")
			joined.WriteString(part)
		}
		for _, want := range []string{"23.994ms", "ThreadPoolForeg-60555-under-a-deliberately-long-comm"} {
			if !strings.Contains(joined.String(), want) {
				t.Fatalf("件3 zh=%v: fold lost content %q:\n%s", zh, want, fence)
			}
		}
		// The fold stamps the shared ⤷ mark so the legend/mini key teach it.
		if !model.Marks.has(runtimeTraceProjMarkSeatRowWrapCont) {
			t.Fatalf("件3 zh=%v: the ◎ fold must stamp the shared ⤷ mark", zh)
		}
	}
}

// TestSmall3ElimFenceShortBoardNoFold — 件3 negative arm: a board whose lines
// all fit renders with zero ⤷ continuations (within-budget lines stay
// byte-identical through the seal-time pass).
func TestSmall3ElimFenceShortBoardNoFold(t *testing.T) {
	projection := types.TraceCausalProjection{
		WakeupPath:              []string{"ui-1"},
		RootCauseFamilyObserved: true,
		OnChainCauses: []types.TraceCausalProjectionNode{
			elimChainNode("E-a", "worker-2", "runnable_wait", "runnable", 1, 3.0, 100),
			elimChainNode("E-b", "worker-3", "runnable_wait", "runnable", 2, 9.0, 200),
		},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjElimOverviewFence(projection, model, true)
	if strings.Contains(fence, "⤷") {
		t.Fatalf("件3 负臂: a within-budget board must not fold:\n%s", fence)
	}
}

// TestSmall3FoldReasonTwinHealedTiebaWitness — 件4 (§29.196⑥; §29.195 追审④
// gated/fold reason 孪生分叉残口) witness replay on the tieba flagship board:
// the aggregate-backed inversion node (tieba_flag E19 shape, CookieMonsterCl-
// 59843 running) publishes fold_capability_freq_only_reason beside its gated
// twin with the SAME arm token, and the rendered fold-deficit clause names
// the arm (仅单簇有频点采样) instead of contradicting the gated clause with
// the generic 簇结构不可判 on one node.
func TestSmall3FoldReasonTwinHealedTiebaWitness(t *testing.T) {
	if testing.Short() {
		t.Skip("real-trace battery")
	}
	const trace = "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace"
	if _, err := os.Stat(trace); err != nil {
		t.Skipf("real-trace fixture absent: %v", err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), trace)
	if err != nil {
		t.Fatal(err)
	}
	query := tracequery.Query{PID: 59566, TimeStart: 34579.450627, TimeEnd: 34579.595184,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: tracequery.TraceFlavorHarmonyHitrace, Limit: 12}
	at := time.Unix(1753000000, 0).UTC()
	var obs []types.ObservationRecord
	for _, view := range []string{"wakeup_chain", "root_cause_rank"} {
		q := query
		q.View = view
		result := tracequery.Run(idx, q)
		obs = append(obs, traceQueryTypedObservations(result, "donghu_tieba_frame.systrace", "p-"+view, "r", "", at)...)
	}
	noteValue := func(notes []string, key string) (string, bool) {
		for _, note := range notes {
			if v, ok := strings.CutPrefix(note, key+"="); ok {
				return v, true
			}
		}
		return "", false
	}
	twins := 0
	for _, record := range obs {
		fold, foldOK := noteValue(record.RichNotes, types.TraceNoteKeyFoldCapability)
		if !foldOK || fold != "freq_only" {
			continue
		}
		foldReason, foldReasonOK := noteValue(record.RichNotes, types.TraceNoteKeyFoldCapabilityFreqOnlyReason)
		if !foldReasonOK || strings.TrimSpace(foldReason) == "" {
			t.Fatalf("件4: a freq_only fold record must name its cause arm on the wire (E19 形):\n%v", record.RichNotes)
		}
		if gatedReason, ok := noteValue(record.RichNotes, types.TraceNoteKeyGatedFreqOnlyReason); ok {
			twins++
			if gatedReason != foldReason {
				t.Fatalf("件4: same-node gated/fold reasons fork (%q vs %q):\n%v", gatedReason, foldReason, record.RichNotes)
			}
		}
	}
	if twins == 0 {
		t.Fatalf("件4: fixture drifted — no gated+fold twin record found on the tieba flagship board")
	}
	// Display face: the fold-deficit clause on the twin node names the arm
	// verbatim and never the generic wording (同节点两行同因不再分叉).
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck,
	}, AnswerContract: types.AnswerContract{Language: "zh"}}
	bus.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: obs}}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "witness。"}}}
	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, at)
	if err != nil || !res.Success {
		t.Fatalf("apply: %v %s", err, res.Summary)
	}
	md := render.RenderAnswerDocument(bus.Mutable.AnswerDocumentV2(), "zh")
	if !strings.Contains(md, "单列口径,不计入有效归因,仅单簇有频点采样,按频率比") {
		t.Fatalf("件4: the healed fold-deficit clause must name the single-cluster arm")
	}
	if strings.Contains(md, "单列口径,不计入有效归因,簇结构不可判") {
		t.Fatalf("件4: the generic fold clause must not survive beside a named gated twin (E19 分叉形)")
	}
}

// TestSmall3UnrankedMaxTop5 — 件5 (§29.197② user ruling): the 未入榜最大
// family caps at TOP5 by the row's published lead value (typed AccountMS,
// desc) with the ruled 「另有 N 项见明细」 same-level tail (§29.175.14 文法);
// ≤5 rows render without a tail; the kept rows' identity 括注/word forms are
// byte-untouched. Witness: cust_report_xx.txt:253-273 (×21 per-seat rows in
// the low-priority auxiliary zone, ordered by pre-edge share instead of the
// reading value).
func TestSmall3UnrankedMaxTop5(t *testing.T) {
	disclosure := func(subject string, pre, post float64) types.TraceCausalProjectionGatedCompositeEdgeShareDisclosure {
		return types.TraceCausalProjectionGatedCompositeEdgeShareDisclosure{
			Subject: subject, PreMS: pre, PostMS: post, AccountMS: pre + post,
			AnchorTS: 34579.555890, Via: "direct",
		}
	}
	projection := elimBoardProjection()
	// Arrival order mirrors the witness disease: pre-edge desc, NOT value
	// desc (54.896 arrives first but 45.332 arrives 3rd-from-last).
	projection.GatedCompositeEdgeShareDisclosures = []types.TraceCausalProjectionGatedCompositeEdgeShareDisclosure{
		disclosure("w-a-1", 53.073, 1.823),  // 54.896
		disclosure("w-b-2", 19.467, 14.514), // 33.981
		disclosure("w-c-3", 17.852, 10.040), // 27.892
		disclosure("w-d-4", 15.767, 8.060),  // 23.827
		disclosure("w-e-5", 11.182, 34.150), // 45.332
		disclosure("w-f-6", 2.087, 1.364),   // 3.451
		disclosure("w-g-7", 2.070, 0.367),   // 2.437
	}
	for _, zh := range []bool{true, false} {
		model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		fence := runtimeTraceProjElimOverviewFence(projection, model, zh)
		label, tail := "· 未入榜最大", "另有 2 项见明细"
		if !zh {
			label, tail = "· unranked max", "2 more — see the detail blocks"
		}
		if got := strings.Count(fence, label); got != 6 { // 5 value rows + 1 tail
			t.Fatalf("件5 zh=%v: want 5 value rows + 1 tail (6 label rows), got %d:\n%s", zh, got, fence)
		}
		if !strings.Contains(fence, tail) {
			t.Fatalf("件5 zh=%v: the folded tail must speak the ruled words %q:\n%s", zh, tail, fence)
		}
		// TOP5 by AccountMS desc: 54.896, 45.332, 33.981, 27.892, 23.827 stay;
		// 3.451 / 2.437 fold. The kept rows' word form is byte-untouched.
		kept := []string{"54.896", "45.332", "33.981", "27.892", "23.827"}
		last := -1
		for _, v := range kept {
			at := strings.Index(fence, v+"ms")
			if at < 0 {
				t.Fatalf("件5 zh=%v: TOP5 value %sms missing:\n%s", zh, v, fence)
			}
			if at < last {
				t.Fatalf("件5 zh=%v: TOP5 must render value-desc (%sms out of order):\n%s", zh, v, fence)
			}
			last = at
		}
		for _, cut := range []string{"3.451ms", "2.437ms", "w-f-6", "w-g-7"} {
			if strings.Contains(fence, cut) {
				t.Fatalf("件5 zh=%v: folded row %q must not render:\n%s", zh, cut, fence)
			}
		}
		if zh && !strings.Contains(small3UnfoldElim(fence), "w-a-1 54.896ms(唤醒边前 53.073 + 边后 1.823)· 按口径不拆段入榜") {
			t.Fatalf("件5: the kept row's identity 括注/word form must stay byte-untouched:\n%s", fence)
		}
	}
	// ≤5 negative arm: five rows, no tail.
	projection.GatedCompositeEdgeShareDisclosures = projection.GatedCompositeEdgeShareDisclosures[:5]
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjElimOverviewFence(projection, model, true)
	if strings.Contains(fence, "另有") && strings.Contains(fence, "项见明细") &&
		strings.Contains(small3UnfoldElim(fence), "未入榜最大  另有") {
		t.Fatalf("件5 负臂: ≤5 rows must render without a folded tail:\n%s", fence)
	}
	if got := strings.Count(fence, "· 未入榜最大"); got != 5 {
		t.Fatalf("件5 负臂: five admitted rows must all render (got %d):\n%s", got, fence)
	}
}
