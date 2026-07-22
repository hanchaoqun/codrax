package tool

// XLANE-3 跨步熔合门 probe + pins (§29.104.2 定谳③, §29.104.9 XREPRO 形③,
// real_trace_campaign_20260705.md): two query steps over the SAME artifact and
// SAME window but DIFFERENT targets (donghu 13762.791708..13763.024898, step1
// target tid 2955, step2 target tid 9163) fuse into ONE projection
// (projections==1 — same-artifact partition is deliberate) and previously
// rendered indistinguishable rank boards: bare 根因排序#N ×2 collisions, a
// cross-board same-thread same-family seat pair with no mutual reference, and
// a cross-board Σ face. The XLANE-3 board identity is the typed triple
// (query window, board target subject, params fingerprint).

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// xlane3DonghuStep is one step of the §29.104.9 probe recipe: the queried
// target plus the rank-shaping knobs the 修补轮 witnesses fork on.
type xlane3DonghuStep struct {
	tid   int
	minMS float64
	topo  string
}

// xlane3DonghuStepObservations runs the §29.104.9 形③ recipe: N steps over
// ONE window, each stepping wakeup_chain + root_cause_rank (the XREPRO probe
// shape), appended into one ledger slice.
func xlane3DonghuStepObservations(t *testing.T, steps ...xlane3DonghuStep) []types.ObservationRecord {
	t.Helper()
	const trace = "../../eval/fixtures/real_traces/donghu.ftrace"
	if _, err := os.Stat(trace); err != nil {
		t.Skipf("golden fixture not present: %v", err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), trace)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Unix(1751600000, 0).UTC()
	var obs []types.ObservationRecord
	for step, s := range steps {
		minMS := s.minMS
		if minMS <= 0 {
			minMS = 0.5
		}
		for _, view := range []string{"wakeup_chain", "root_cause_rank"} {
			// EVOLUTION RECORD (CHAIN-BUDGET, 2026-07-18): the §29.104.9 witness
			// boards are pinned under the EXPLICIT legacy chain caps
			// (max_branches=8, max_chain_nodes=1 — the degenerate tier,
			// byte-identical to the pre-CHAIN-BUDGET chain), so every
			// board-identity display byte below stays the recorded witness.
			// The board-identity machinery itself is cap-independent.
			q := tracequery.Query{PID: s.tid, TimeStart: 13762.791708, TimeEnd: 13763.024898,
				MaxDepth: 4, MaxBranches: 8, MaxChainNodes: 1, MinDurationMs: minMS, TraceFlavorHint: tracequery.TraceFlavorHarmonyHitrace, Limit: 12}
			q.CoreTopology = s.topo
			q.View = view
			result := tracequery.Run(idx, q)
			obs = append(obs, traceQueryTypedObservations(result, "donghu.ftrace",
				"p"+string(rune('1'+step))+"-"+view, "r", "", at)...)
		}
	}
	return obs
}

// xlane3DonghuTwoStepObservations is the original two-target form (default
// knobs) of the recipe above.
func xlane3DonghuTwoStepObservations(t *testing.T, tids ...int) []types.ObservationRecord {
	t.Helper()
	steps := make([]xlane3DonghuStep, 0, len(tids))
	for _, tid := range tids {
		steps = append(steps, xlane3DonghuStep{tid: tid})
	}
	return xlane3DonghuStepObservations(t, steps...)
}

// xlane3RenderTwoStep renders the fused two-step report through the production
// mutation+render path and returns (markdown, projection set).
func xlane3RenderTwoStep(t *testing.T, obs []types.ObservationRecord) (string, types.TraceCausalProjectionSet) {
	t.Helper()
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck,
	}}
	bus.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: obs}}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "xlane3 two-step witness。"}}}
	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil || !res.Success {
		t.Fatalf("apply: %v %s", err, res.Summary)
	}
	set := types.CompileTraceCausalProjectionSet(types.CompileObservationLedger(types.ObservationLedgerInput{
		ToolResults: []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: obs}},
	}))
	return render.RenderAnswerDocument(bus.Mutable.AnswerDocumentV2(), "zh"), set
}

// TestXLANE3TwoStepFusedBoardsDisambiguate — 件1+件2 pins on the real donghu
// two-step recipe. Pre-fix disease (§29.104.9 形③ verbatim): the fused render
// carried 根因排序#1/#2/#3 各×2 with ZERO disambiguation (six bare collision
// chips), plus the cross-board Σ line 「各根因席位有效归因合计 355.562ms 超过
// 窗长 233.190ms」 summing both boards.
func TestXLANE3TwoStepFusedBoardsDisambiguate(t *testing.T) {
	obs := xlane3DonghuTwoStepObservations(t, 2955, 9163)
	// 件1 producer half: every step's rank observations carry the typed board
	// identity notes (target label + params fingerprint).
	sawStep1, sawStep2 := false, false
	for _, record := range obs {
		if !strings.Contains(record.ID, "#root_cause_rank:") {
			continue
		}
		joined := strings.Join(record.RichNotes, "\n")
		if strings.HasPrefix(record.ID, "trace_query:p1-") &&
			strings.Contains(joined, "rank_board_target=CompThread_0-2955") {
			sawStep1 = true
		}
		if strings.HasPrefix(record.ID, "trace_query:p2-") &&
			strings.Contains(joined, "rank_board_target=logd.writer-9163") {
			sawStep2 = true
		}
		if !strings.Contains(joined, "rank_board_params_fingerprint=") {
			t.Fatalf("rank observation %s must carry the params fingerprint note:\n%s", record.ID, joined)
		}
	}
	if !sawStep1 || !sawStep2 {
		t.Fatalf("both steps' rank observations must carry their typed board target (step1=%v step2=%v)", sawStep1, sawStep2)
	}
	md, set := xlane3RenderTwoStep(t, obs)
	// 件1 board identity design decision (验收④): the same-artifact partition
	// stays ONE projection — boards separate through the typed triple key
	// inside it, never through a second artifact partition.
	if len(set.Projections) != 1 {
		t.Fatalf("same-artifact two-step ledger must stay one projection, got %d", len(set.Projections))
	}
	// 件2: the two boards' #1 seats are distinguishable — each row carries
	// its own board anchor. RULE3-1 件1(c)+件2 (§29.181①②) EVOLUTION: the
	// identical window halves hoist to ONE head declaration, the ➊ badges
	// carry the ordinals, and the anchor halves stay per row.
	if !strings.Contains(md, "- 全席同窗 窗13762.792~13763.025s") {
		t.Fatalf("the fused report must declare the shared window once:\n%s", md)
	}
	if !strings.Contains(md, "·板锚 CompThread_0-2955") ||
		!strings.Contains(md, "·板锚 logd.writer-9163") {
		t.Fatalf("both boards' #1 rows must carry distinct board anchors:\n%s", md)
	}
	// 件2 撞号=0: no seat chip renders a bare ordinal (the ·窗…·板锚 board
	// identity always sits between the ordinal and the confidence tier; the
	// column-glossary literal 「根因排序#N·置信」 spells letter N, not digits).
	if bare := regexp.MustCompile(`根因排序#[0-9]+·置信`); bare.MatchString(md) {
		t.Fatalf("bare ordinal chip collision resurfaced:\n%s", bare.FindString(md))
	}
	if bare := regexp.MustCompile(`邻近影响#[0-9]+·置信`); bare.MatchString(md) {
		t.Fatalf("bare adjacent ordinal chip collision resurfaced:\n%s", bare.FindString(md))
	}
	// The detail seat line keeps the FULL chip bytes (无损明细 — the hoist
	// touches the tree 行2 face only).
	if !strings.Contains(md, "- 根因排序: ➊ #1·窗13762.792~13763.025s·板锚 CompThread_0-2955·置信高") {
		t.Fatalf("the detail seat line must carry the full board identity:\n%s", md)
	}
	// The board-anchor legend entry rides the wearing report.
	if !strings.Contains(md, "`板锚 <线程>`") {
		t.Fatalf("the 板锚 legend entry must render on the multi-board report:\n%s", md)
	}
	// 件3: the cross-board same-thread same-family mutual pointers exist in
	// BOTH directions (the 2.35× witness pair: logd.writer's 9163-board 全额
	// seats vs its 2955-board census/anchored seats). The fence wraps long
	// rows, so the sentence matches on the joined form.
	joinedMD := rspaFenceJoined(md)
	// RULE3-1 件1(a): the ⇄ short markers carry the per-row halves; the
	// invariant sentence lives once in the legend entry.
	if !strings.Contains(joinedMD, "⇄另板") ||
		!strings.Contains(joinedMD, "(板锚 CompThread_0-2955,见图例)") ||
		!strings.Contains(joinedMD, "(板锚 logd.writer-9163,见图例)") {
		t.Fatalf("the cross-board mutual pointers must render both directions:\n%s", md)
	}
	if !strings.Contains(md, "席位值不可跨板相加") {
		t.Fatalf("the ⇄ legend entry must carry the cross-board invariant:\n%s", md)
	}
	// 件3: no cross-board Σ face — the pre-fix line summed both boards
	// (355.562 > 233.190) while NEITHER board exceeds the window alone.
	if strings.Contains(md, "各根因席位有效归因合计") || strings.Contains(md, "同板根因席位有效归因合计") {
		t.Fatalf("the fused report must not sum rank seats across boards:\n%s", md)
	}
	// 修补轮 件A1 (2026-07-16): the coverage residual belongs to the 2955
	// board (CompThread's own account) — the pre-fix note annotated it with
	// 「其中 56.229ms 为周期性信号源期内正常节拍」 whose 56.229 came entirely
	// from the 9163 board's AudioOut chain rows. The identity-less periodic
	// row cannot prove subject-board membership on this multi-board report,
	// so the sentence disappears; the coverage sentence itself is untouched.
	if strings.Contains(md, "周期性信号源期内正常节拍") {
		t.Fatalf("the fused report must not annotate one board's residual with another board's cadence:\n%s", md)
	}
	if !strings.Contains(md, "关注线程等待(sleep/D-state/runnable) 116.963ms 中链上已归因 16.419ms(14%),未归因 100.544ms(86%)。") {
		t.Fatalf("the coverage sentence's own account must stay byte-identical:\n%s", md)
	}
	// 修补轮 件A3: the 未计入 census scopes to the subject board — the
	// pre-fix count (49) included the 9163 board's named rows.
	// EVOLUTION RECORD (ONCHAIN-3c, 2026-07-19): 37→38 — the bare-census-edge
	// state-seat arm lane-changes gpu-token-id4-2931's inversion-typed
	// runnable seat onto the 2955 board's chain tier (fully pre-edge of its
	// direct census edge at 13763.014598, values untouched), adding one
	// subject-board on-chain row the tree does not admit; the MAX stays the
	// 74.915 single-row value.
	if !strings.Contains(md, "另有 38 条链上行未计入上句已归因数值(单项最大 74.915ms") {
		t.Fatalf("the unadmitted census must scope to the subject board (38 rows, max 74.915):\n%s", md)
	}
	// 修补轮 件C (one relation, one sentence): E11↔E44 already cross-refer
	// through the cross-channel pointers — the cross-board sentence must not
	// re-hang the same refs (E11 keeps its OTHER peer E38; E44 keeps its
	// remaining 9163-board peers). The priority-authority closure removes the
	// old unproved inversion seat; board-scoped exact dedupe simultaneously
	// restores p1's valid 0.018ms logd.writer account, so the honest pointer
	// roster is 3 seats rather than the old 4-seat false-inversion shape.
	// EVOLUTION RECORD (ONCHAIN-3c, 2026-07-19): the new gpu-token-id4-2931
	// edge-anchored seat enters the 2955 board as [E36], shifting the former
	// E37/E43 refs to E38/E44 (pure ordinal renumbering — the pointed rows'
	// identities and values are unchanged, verified against the live report).
	if !strings.Contains(joinedMD, "本线程另有邻近席 [E44]") ||
		!strings.Contains(joinedMD, "本线程另有链上席 [E11(+1)]") {
		t.Fatalf("the cross-channel pointers must keep their sentences:\n%s", md)
	}
	// E44's sentence drops E11 (its cross-channel chain pointer) and keeps
	// the remaining 9163-board peers; E11's sentence drops E44 (its
	// cross-channel adjacent pointer) and keeps [E38]. OTHER 9163-board rows
	// without a cross-channel pointer to E44 legitimately keep both refs.
	if strings.Contains(joinedMD, "另板席 [E11(+1)]") {
		t.Fatalf("E44's cross-board sentence must not re-hang its cross-channel-pointed peer E11 (件C):\n%s", md)
	}
	if !strings.Contains(joinedMD, "⇄另板[E38](板锚 CompThread_0-2955,见图例)") {
		t.Fatalf("E11's cross-board sentence must keep exactly its remaining peer [E38] (件C):\n%s", md)
	}
	if !strings.Contains(joinedMD, "⇄另板[E28]、[E29]等3席(板锚 logd.writer-9163") {
		t.Fatalf("E44's cross-board sentence must keep its remaining 9163-board peers (件C; renumbered from E43 by the [E36] insertion):\n%s", md)
	}
	// 修补轮 件F: the micro fold's detail-face ordinal range wears the board
	// chip on this multi-board report (a bare 「#4~#6(折叠合一)」 is the
	// ordinal collision the chip exists for — 撞号 pin 扩折叠形).
	if !strings.Contains(md, "(折叠合一)·窗13762.792~13763.025s·板锚 CompThread_0-2955") {
		t.Fatalf("the fold's ordinal range must wear the board chip:\n%s", md)
	}
	if strings.Contains(md, "(折叠合一)\n") {
		t.Fatalf("a bare fold ordinal range must not render on the multi-board report:\n%s", md)
	}
	// 修补轮 件G: the ◎ overview's single-thread ruler claim is false over
	// two target boards — the head speaks the multi-board ruler and every
	// named member line wears its board anchor.
	if !strings.Contains(md, "尺=各板目标线程 窗内墙钟ms·跨板不可相加") {
		t.Fatalf("the fused ◎ head must speak the multi-board ruler:\n%s", md)
	}
	if strings.Contains(md, "尺=CompThread_0-2955 窗内墙钟ms") {
		t.Fatalf("the single-thread ruler claim must not survive on the fused ◎ head:\n%s", md)
	}
	// OMGCLEAN-1 件7 (§29.175 / §29.133 件G EVOLUTION, 2026-07-20): sections
	// whose members unanimously carry one typed board target hoist the ·板锚
	// chip onto the ▸ head (once per section); mixed sections keep per-row
	// chips. The fused witness sections are unanimous → head-worn anchors.
	if !strings.Contains(md, "最大可消 65.912ms ·板锚 CompThread_0-2955") ||
		!strings.Contains(md, "最大可消 48.519ms ·板锚 logd.writer-9163") {
		t.Fatalf("the fused ◎ section heads must wear the hoisted board anchors:\n%s", md)
	}
	if strings.Contains(md, "·板锚 CompThread_0-2955 [E1]") {
		t.Fatalf("件7: a hoisted section's member rows must drop the per-row anchor:\n%s", md)
	}
	// 件4 (XLANE-1 rider, both directions in the fused form): the elected
	// tree target's own board seats wear 目标自身·墙钟席 legally…
	// RULE3-1 件1(c)+件2: badge ordinal + hoisted window — the anchor half
	// keeps the self seat's board identity inline.
	if !strings.Contains(md, "算力供给候选·目标自身·墙钟席·板锚 CompThread_0-2955") {
		t.Fatalf("the tree target's self seat must keep its legal 自身 word:\n%s", md)
	}
	// …while the OTHER board's legitimate self seats (subject ≠ tree target)
	// wear NO 自身 word anywhere — with the typed self basis provably present
	// on a logd.writer node (the gate suppresses wording, not the basis).
	foreignSelfBasis := false
	for _, node := range set.Projections[0].OnChainCauses {
		if strings.HasPrefix(node.Subject, "logd.writer") &&
			strings.TrimSpace(node.OnChainBasis) == "self_wall_clock_interval" {
			foreignSelfBasis = true
		}
	}
	if !foreignSelfBasis {
		t.Fatalf("fixture drift: the fused ledger must carry logd.writer self-basis rows")
	}
	for _, line := range strings.Split(md, "\n") {
		if strings.Contains(line, "logd.writer") && strings.Contains(line, "自身·") {
			t.Fatalf("a foreign-subject row must never wear the 自身 word:\n%s", line)
		}
	}
}

// TestXLANE3BoardNotesAbsentFailOpen — 件1 negative pin: WITHOUT the typed
// board notes (legacy persisted records), the report stays single-board —
// absence never splits, and the pre-XLANE-3 bare-collision form returns
// verbatim (fail-open, never a fabricated board).
func TestXLANE3BoardNotesAbsentFailOpen(t *testing.T) {
	obs := xlane3DonghuTwoStepObservations(t, 2955, 9163)
	for i := range obs {
		kept := obs[i].RichNotes[:0]
		for _, note := range obs[i].RichNotes {
			if strings.HasPrefix(note, "rank_board_target=") ||
				strings.HasPrefix(note, "rank_board_params_fingerprint=") {
				continue
			}
			kept = append(kept, note)
		}
		obs[i].RichNotes = kept
	}
	md, _ := xlane3RenderTwoStep(t, obs)
	if strings.Contains(md, "板锚") || strings.Contains(md, "参数#") {
		t.Fatalf("board identity must never be guessed from note-less records:\n%s", md)
	}
	// RULE3-1 件2 (§29.181②): the two #1 seats badge (➊ ×2) instead of
	// wording the ordinal — the note-less ledger stays board-token-free.
	if n := strings.Count(md, "➊"); n < 2 {
		t.Fatalf("the note-less ledger keeps the legacy single-board form (➊ ×2), got %d", n)
	}
}

// TestXLANE3SingleStepKeepsLegacyFormAndLegalSelfWord — 泛化红线 (验收⑤ token
// half; the byte-level regression ran operationally pre/post fix): a
// single-step run renders ZERO board tokens, and the 件4 positive direction —
// the step's own target wears 目标自身·墙钟席 legally on its own board.
func TestXLANE3SingleStepKeepsLegacyFormAndLegalSelfWord(t *testing.T) {
	obs := xlane3DonghuTwoStepObservations(t, 9163)
	md, set := xlane3RenderTwoStep(t, obs)
	if len(set.Projections) != 1 {
		t.Fatalf("single-step ledger must stay one projection, got %d", len(set.Projections))
	}
	for _, token := range []string{"板锚", "参数#", "同线程同状态族账另见另板席", "同板根因席位有效归因合计", "·窗13762.792",
		// 修补轮 件G: the single-board ◎ head keeps the single-thread ruler.
		"各板目标线程",
		// 修补轮 件F: the fold's ordinal range stays bare on one board.
		"折叠合一)·"} {
		if strings.Contains(md, token) {
			t.Fatalf("single-board report must carry no board token %q:\n%s", token, md)
		}
	}
	// 修补轮 件A1 legacy half: the single-board report keeps its own periodic
	// cadence note byte-identically (the board scoping bites multi-board
	// fusions only).
	if !strings.Contains(md, "其中 56.229ms 为周期性信号源期内正常节拍") {
		t.Fatalf("the single-board cadence note must stay:\n%s", md)
	}
	if !strings.Contains(md, "尺=logd.writer-9163 窗内墙钟ms") {
		t.Fatalf("the single-board ◎ head keeps the single-thread ruler:\n%s", md)
	}
	// 件4 positive direction: logd.writer IS this board's target — its self
	// seats wear the 自身 word legally (the fused form suppresses exactly the
	// same rows as foreign subjects).
	if !strings.Contains(md, "调度压力候选·目标自身·墙钟席·置信中") {
		t.Fatalf("the single-step target's self seat must wear its legal 自身 word:\n%s", md)
	}
}

// TestXLANE3ParamsForkPerBoardDenominator — 修补轮 件A2 (2026-07-16) on the
// real params-fork recipe: same window, same target 9163, MinDurationMs 0.5
// vs 5.0. Pre-fix disease: the coverage denominator summed BOTH boards'
// SelfRows — 257.635ms > the 233.190ms window (physically impossible for one
// thread; board 2's aggregate runnable seat 17.815 double-counted board 1's
// occurrence rows). Post-fix the denominator is the LARGEST single board's
// account: 239.820ms — hand-verified = 68.697 (identity-less sleep-hop MAX
// admission, shared base) + 171.123 (board 1's own state rows), byte-equal
// to the untouched single-step 9163 denominator. NOTE (verdict deviation,
// reported): 239.820 still exceeds the window — that overshoot is the single
// board's own legacy admission shape (present verbatim in the pinned
// single-step form) and is NOT the cross-board double count this 件 fixes.
func TestXLANE3ParamsForkPerBoardDenominator(t *testing.T) {
	obs := xlane3DonghuStepObservations(t,
		xlane3DonghuStep{tid: 9163, minMS: 0.5}, xlane3DonghuStep{tid: 9163, minMS: 5.0})
	md, _ := xlane3RenderTwoStep(t, obs)
	if strings.Contains(md, "257.635") {
		t.Fatalf("the cross-board denominator sum must not survive:\n%s", md)
	}
	if !strings.Contains(md, "关注线程等待(sleep/D-state/runnable) 239.820ms 中链上已归因") {
		t.Fatalf("the denominator must be the largest single board's own account (239.820):\n%s", md)
	}
	// 件2 params half on the real recipe: both boards' seats wear their
	// fingerprints (same window, same target — the fingerprint is the only
	// discriminating half).
	if !strings.Contains(md, "·参数#") {
		t.Fatalf("the params-fork boards must wear their fingerprint halves:\n%s", md)
	}
	// 件G: same target thread on both boards — the ◎ single-thread ruler
	// claim stays TRUE and the head stays single-board.
	if !strings.Contains(md, "尺=logd.writer-9163 窗内墙钟ms") || strings.Contains(md, "各板目标线程") {
		t.Fatalf("a same-target params fork keeps the single-thread ◎ ruler:\n%s", md)
	}
}

// TestXLANE3MixedLegacyNewFormsUnnamedBoard — 修补轮 件E (2026-07-16): step1's
// board notes stripped (legacy persisted artifact), step2's kept. Pre-fix the
// single-value inheritance handed step2's 9163 target to the identity-less
// seats — the two boards re-fused and the cross-board Σ 病句 (355.562 >
// 233.190) reprinted verbatim. Post-fix the identity-less seats form their
// own UNNAMED board: named seats wear chips, unnamed seats keep the bare
// legacy face, and no per-board Σ exceeds the window.
func TestXLANE3MixedLegacyNewFormsUnnamedBoard(t *testing.T) {
	obs := xlane3DonghuTwoStepObservations(t, 2955, 9163)
	for i := range obs {
		if !strings.HasPrefix(obs[i].ID, "trace_query:p1-") {
			continue
		}
		kept := obs[i].RichNotes[:0]
		for _, note := range obs[i].RichNotes {
			if strings.HasPrefix(note, "rank_board_target=") ||
				strings.HasPrefix(note, "rank_board_params_fingerprint=") {
				continue
			}
			kept = append(kept, note)
		}
		obs[i].RichNotes = kept
	}
	md, _ := xlane3RenderTwoStep(t, obs)
	if strings.Contains(md, "355.562") {
		t.Fatalf("the re-fused cross-board Σ 病句 must not reprint:\n%s", md)
	}
	// The named board's seats wear their board anchor (RULE3-1 件1(c)+件2:
	// the shared window hoists to the tree head and the ➊ badge carries the
	// ordinal; the anchor half stays inline per row).
	if !strings.Contains(md, "- 全席同窗 窗13762.792~13763.025s") {
		t.Fatalf("the mixed form must declare the shared window once:\n%s", md)
	}
	if !strings.Contains(md, "·板锚 logd.writer-9163") {
		t.Fatalf("named seats must wear their board anchors on the mixed form:\n%s", md)
	}
	// …while the identity-less seats keep the bare legacy face (无名板零
	// chip — absence never wears a board claim).
	if !strings.Contains(md, "➊") {
		t.Fatalf("identity-less seats must keep the bare legacy face (badged):\n%s", md)
	}
	if strings.Contains(md, "板锚 CompThread_0-2955") {
		t.Fatalf("no seat may inherit a named target it never carried:\n%s", md)
	}
}

// TestXLANE3TopoForkSplitsFingerprint — 修补轮 件D (2026-07-16): core_topology
// is a rank-shaping knob (witnessed value drift under an unchanged
// fingerprint = silent board collision) — two steps differing only in an
// explicit core_topology mint SPLIT board fingerprints at the producer.
// (Display half: a fingerprint split wears 参数# chips through the same
// machinery the params-fork recipe above and the synthetic shapes pin;
// value-equal twin seats still converge through the R1 same-fact merge —
// deliberate, no differing values are ever mixed.)
func TestXLANE3TopoForkSplitsFingerprint(t *testing.T) {
	obs := xlane3DonghuStepObservations(t,
		xlane3DonghuStep{tid: 9163}, xlane3DonghuStep{tid: 9163, topo: "little=0-3;middle=4-11;big=12-13"})
	fps := map[string]map[string]bool{}
	for _, record := range obs {
		if !strings.Contains(record.ID, "#root_cause_rank:") {
			continue
		}
		step := "p1"
		if strings.HasPrefix(record.ID, "trace_query:p2-") {
			step = "p2"
		}
		for _, note := range record.RichNotes {
			if strings.HasPrefix(note, "rank_board_params_fingerprint=") {
				if fps[step] == nil {
					fps[step] = map[string]bool{}
				}
				fps[step][strings.TrimPrefix(note, "rank_board_params_fingerprint=")] = true
			}
		}
	}
	if len(fps["p1"]) != 1 || len(fps["p2"]) != 1 {
		t.Fatalf("each step mints exactly one fingerprint, got %v", fps)
	}
	for fp := range fps["p1"] {
		if fps["p2"][fp] {
			t.Fatalf("an explicit core_topology must split the board fingerprint (both %s)", fp)
		}
	}
}

// TestXLANE3PerBoardOverWindowSumScopesItsClaim — 件3 Σ face pins on the
// synthetic two-board shape: (a) a cross-board total exceeding the window
// while NEITHER board does stays SILENT (the pre-fix line summed across
// boards); (b) one board exceeding alone mints the 同板-scoped sentence.
func TestXLANE3PerBoardOverWindowSumScopesItsClaim(t *testing.T) {
	projection := xlane3TwoBoardsProjection()
	// Window 10.0..10.2 = 200ms. Cross-board total 150+120=270 > 200 while
	// each board stays within — the silent arm.
	projection.OnChainCauses[0].EffectiveImpactMS = 150
	projection.OnChainCauses[1].EffectiveImpactMS = 120
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if model.RankBoardEffSumMS != 0 {
		t.Fatalf("a cross-board total must never mint the over-window warning (got %.3f)", model.RankBoardEffSumMS)
	}
	// One board over the window alone: the sentence renders 同板-scoped with
	// exactly that board's Σ.
	projection.OnChainCauses[0].EffectiveImpactMS = 250
	model = buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if model.RankBoardEffSumMS != 250 || !model.RankBoardEffSumMultiBoard {
		t.Fatalf("the exceeding board's own Σ must mint the multi-board warning (got %.3f multi=%v)",
			model.RankBoardEffSumMS, model.RankBoardEffSumMultiBoard)
	}
	lead := runtimeTraceProjLeadText(projection, model, "zh", true)
	if !strings.Contains(lead, "同板根因席位有效归因合计最大 250.000ms 超过窗长 200.000ms") ||
		!strings.Contains(lead, "不同板的席位更不可跨板相加") {
		t.Fatalf("the multi-board over-window sentence must scope its claim to 同板:\n%s", lead)
	}
}

// TestXLANE3SyntheticShapesWearBoardIdentity — 件2 synthetic pins: the
// two-board shape wears 板锚 halves (targets differ) and the params-fork
// shape wears 参数# halves (one target, two knob sets); the cross-board
// mutual pointer pairs exactly the same-thread same-family seats.
func TestXLANE3SyntheticShapesWearBoardIdentity(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(xlane3TwoBoardsProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "·板锚 target.a-100") || !strings.Contains(fence, "·板锚 target.b-200") {
		t.Fatalf("the two-board shape must wear both board anchors:\n%s", fence)
	}
	if strings.Contains(fence, "·参数#") {
		t.Fatalf("distinct targets need no params half (each half only where it disambiguates):\n%s", fence)
	}
	if !strings.Contains(fence, "⇄另板") {
		t.Fatalf("the same-thread cross-board pair must render the mutual pointer:\n%s", fence)
	}
	model = buildRuntimeTraceProjTreeModel(xlane3ParamsForkProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence = runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "·参数#cccc3333") || !strings.Contains(fence, "·参数#dddd4444") {
		t.Fatalf("the params-fork shape must wear both fingerprints:\n%s", fence)
	}
	if strings.Contains(fence, "·板锚 ") {
		t.Fatalf("a single-target window carries no board-anchor half:\n%s", fence)
	}
	if strings.Contains(fence, "⇄另板") {
		t.Fatalf("different subjects never pair cross-board:\n%s", fence)
	}
	// EN face parity on the two-board shape.
	enModel := buildRuntimeTraceProjTreeModel(xlane3TwoBoardsProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	enFence := runtimeTraceProjTreeFence(enModel, false)
	// RULE3-1 件1(a): the EN ⇄ short marker + the legend invariant.
	if !strings.Contains(enFence, "board target.a-100") ||
		!strings.Contains(enFence, "⇄ cross-board [") {
		t.Fatalf("the EN face must wear the board anchor and the ⇄ marker:\n%s", enFence)
	}
}

// xlane3BoardSeat builds one synthetic board seat node for the gate pins.
func xlane3BoardSeat(evidence, subject, state, token, target, fp string, rank int, impact float64, lineStart int) types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: evidence,
		Subject: subject, Object: token, TypeToken: token,
		StateKind: state, ChainRelevance: "on_chain", ChainDepth: 1,
		ImpactMS: impact, CumulativeImpactMS: impact, EffectiveImpactMS: impact,
		QueryWindowStartTs: 10.0, QueryWindowEndTs: 10.2,
		RankBoardTarget: target, RankBoardParamsFingerprint: fp,
		Rank: rank, Tier: "tertiary", Confidence: 0.8,
		LineStart: lineStart, LineEnd: lineStart + 10,
	}
}

func xlane3GateProjection(nodes ...types.TraceCausalProjectionNode) types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"waker-1", "target.a-100"},
		WindowStartTs: 10.0,
		WindowEndTs:   10.2,
		OnChainCauses: nodes,
	}
}

// TestXLANE3CrossBoardStamperGates — 修补轮 件B (2026-07-16): per-gate
// mutation-red pins. Each shape differs from the pairing control
// (TestXLANE3SyntheticShapesWearBoardIdentity's two-board pair) in exactly
// the property one gate rejects — deleting that gate pairs the rows and
// turns the test red.
func TestXLANE3CrossBoardStamperGates(t *testing.T) {
	render := func(nodes ...types.TraceCausalProjectionNode) string {
		model := buildRuntimeTraceProjTreeModel(xlane3GateProjection(nodes...), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
		return runtimeTraceProjTreeFence(model, true)
	}
	// G1 (Rank>0 gate): a rank-less row never joins a cross-board family
	// group — even with a full board identity and a positive value.
	seatA := xlane3BoardSeat("g1-a", "workerX-300", "runnable", "runnable_wait", "target.a-100", "aaaa1111", 1, 5.0, 10)
	rankless := xlane3BoardSeat("g1-b", "workerX-300", "runnable", "scheduler_latency", "target.b-200", "bbbb2222", 0, 3.0, 30)
	if fence := render(seatA, rankless); strings.Contains(fence, "⇄另板") {
		t.Fatalf("G1: a rank-less row must never pair cross-board:\n%s", fence)
	}
	// G2 (两把尺 wall-clock gate): a caliber-side token (count equivalent)
	// in the SAME registry family (file_io_hot_inode: IO lane, count
	// additivity) never joins — the family gate alone would admit it.
	ioSeat := xlane3BoardSeat("g2-a", "workerX-300", "d_state", "d_state_or_io_wait", "target.a-100", "aaaa1111", 1, 5.0, 10)
	countSeat := xlane3BoardSeat("g2-b", "workerX-300", "", "file_io_hot_inode", "target.b-200", "bbbb2222", 2, 3.0, 30)
	if fence := render(ioSeat, countSeat); strings.Contains(fence, "⇄另板") {
		t.Fatalf("G2: a caliber-side row must never join a wall-clock cross-board pair:\n%s", fence)
	}
	// G3 (typed state-family gate): rows outside the three duration families
	// (running) never pair even as same-subject ranked seats on two boards.
	runA := xlane3BoardSeat("g3-a", "workerX-300", "running", "running", "target.a-100", "aaaa1111", 1, 5.0, 10)
	runB := xlane3BoardSeat("g3-b", "workerX-300", "running", "running", "target.b-200", "bbbb2222", 2, 3.0, 30)
	if fence := render(runA, runB); strings.Contains(fence, "⇄另板") {
		t.Fatalf("G3: family-less rows must never pair cross-board:\n%s", fence)
	}
	// G4 (complete board identity gate): a windowless seat (target+params
	// present, no typed query window) never pairs and never mints a phantom
	// 0..0 board.
	windowless := xlane3BoardSeat("g4-b", "workerX-300", "runnable", "scheduler_latency", "target.b-200", "bbbb2222", 2, 3.0, 30)
	windowless.QueryWindowStartTs, windowless.QueryWindowEndTs = 0, 0
	if fence := render(seatA, windowless); strings.Contains(fence, "⇄另板") {
		t.Fatalf("G4: a windowless seat must never pair cross-board:\n%s", fence)
	}
	// Control (the gates reject properties, not the shape): the healthy pair
	// on the same helper DOES pair — the mutations above are meaningful.
	seatB := xlane3BoardSeat("ctl-b", "workerX-300", "runnable", "scheduler_latency", "target.b-200", "bbbb2222", 2, 3.0, 30)
	if fence := render(seatA, seatB); !strings.Contains(fence, "⇄另板") {
		t.Fatalf("control pair must pair cross-board:\n%s", fence)
	}
}

// TestXLANE3CrossBoardYieldsToValueMirror — 修补轮 件B G5 reachability verdict
// (可达, arm kept): a cross-board pair that is ALSO a same-span µs-equal
// value-mirror pair (equal displays, diverging cumulative — the engine
// deliberately fails open on ranked seats, so such pairs reach this layer)
// speaks ONE relation: the mirror sentence; the cross-board arm yields
// (one relation, one sentence). Deleting the G5 skip double-tags the pair
// and turns this red.
func TestXLANE3CrossBoardYieldsToValueMirror(t *testing.T) {
	a := xlane3BoardSeat("g5-a", "workerX-300", "runnable", "runnable_wait", "target.a-100", "aaaa1111", 1, 5.0, 10)
	b := xlane3BoardSeat("g5-b", "workerX-300", "runnable", "runnable_wait", "target.b-200", "bbbb2222", 2, 5.0, 10)
	b.CumulativeImpactMS = 8.0 // diverging scope account — the class-(1) mirror carrier
	model := buildRuntimeTraceProjTreeModel(xlane3GateProjection(a, b), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "同段镜像") {
		t.Fatalf("the value-mirror relation must speak on the pair:\n%s", fence)
	}
	if strings.Contains(fence, "⇄另板") {
		t.Fatalf("G5: the cross-board arm must yield to the existing mirror relation:\n%s", fence)
	}
}

// TestXLANE3SeparatorInjectionCannotCollideBoards — 修补轮 件H② : an
// adversarial comm containing the former '|' separator ("t|a" + params "b"
// vs "t" + params "a|b" — byte-identical under pipe joining) can no longer
// collide two boards: the identity key separators are unprintable \x00.
func TestXLANE3SeparatorInjectionCannotCollideBoards(t *testing.T) {
	a := xlane3BoardSeat("inj-a", "workerX-300", "runnable", "runnable_wait", "t|a", "b", 1, 5.0, 10)
	b := xlane3BoardSeat("inj-b", "workerX-300", "runnable", "scheduler_latency", "t", "a|b", 2, 3.0, 30)
	model := buildRuntimeTraceProjTreeModel(xlane3GateProjection(a, b), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "⇄另板") {
		t.Fatalf("the injected pair is TWO boards and must pair (a '|' key would collide them):\n%s", fence)
	}
	if !strings.Contains(fence, "·板锚 t|a") || !strings.Contains(fence, "·板锚 t·") {
		t.Fatalf("both injected boards must wear their verbatim anchors:\n%s", fence)
	}
}

// TestXLANE3ParamsForkAnchorLabelPerLanguage — 修补轮 件H①: the cross-board
// anchor label's params half is per-language — the zh 「·参数#」 must not
// leak into the EN sentence (same word pair as the seat chip's half).
func TestXLANE3ParamsForkAnchorLabelPerLanguage(t *testing.T) {
	a := xlane3BoardSeat("h1-a", "workerX-300", "runnable", "runnable_wait", "target.a-100", "cccc3333", 1, 5.0, 10)
	b := xlane3BoardSeat("h1-b", "workerX-300", "runnable", "scheduler_latency", "target.a-100", "dddd4444", 2, 3.0, 30)
	model := buildRuntimeTraceProjTreeModel(xlane3GateProjection(a, b), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, true))
	if !strings.Contains(fence, "板锚 target.a-100·参数#dddd4444") {
		t.Fatalf("the zh anchor label carries the zh params half:\n%s", fence)
	}
	enModel := buildRuntimeTraceProjTreeModel(xlane3GateProjection(a, b), newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	enFence := rspaFenceJoined(runtimeTraceProjTreeFence(enModel, false))
	if !rspaFenceContains(enFence, "board target.a-100 · params #dddd4444") {
		t.Fatalf("the EN anchor label carries the EN params half:\n%s", enFence)
	}
	if strings.Contains(enFence, "·参数#") {
		t.Fatalf("the zh params half must not leak onto the EN face:\n%s", enFence)
	}
}

// xlane3MicroSeat builds a chain-lane micro anchored CUT seat (⛓ half of a
// bipartition, eff below the fold threshold) on the given board.
func xlane3MicroSeat(evidence, subject, target, fp string, rank, lineStart int, eff float64) types.TraceCausalProjectionNode {
	node := xlane3BoardSeat(evidence, subject, "runnable", "runnable_wait", target, fp, rank, eff, lineStart)
	node.ChainAnchoredMS = eff
	node.ChainAnchorFullMS = eff + 1.0
	return node
}

// TestXLANE3MicroFoldPerBoard — 修补轮 件F: (a) 两板微额席永不混折 — the fold
// key includes the board triple, so each board mints its OWN counted fold
// row; (b) the fold row wears the uniform board chip on its detail ordinal
// range (witness pin lives in the fused test; here the model carries the
// stamped chip); (c) a folded member that the cross-board stamper had
// pointed at is represented by ONE sentence on the fold row, and the peer's
// reverse refs keep resolving through the fold bracket.
func TestXLANE3MicroFoldPerBoard(t *testing.T) {
	m1 := xlane3MicroSeat("m1", "workerX-300", "target.a-100", "aaaa1111", 5, 10, 0.020)
	m2 := xlane3MicroSeat("m2", "workerX-300", "target.a-100", "aaaa1111", 6, 30, 0.030)
	m3 := xlane3MicroSeat("m3", "workerY-400", "target.b-200", "bbbb2222", 7, 50, 0.040)
	m4 := xlane3MicroSeat("m4", "workerY-400", "target.b-200", "bbbb2222", 8, 70, 0.050)
	peer := xlane3BoardSeat("peer", "workerX-300", "runnable", "scheduler_latency", "target.b-200", "bbbb2222", 1, 5.0, 90)
	model := buildRuntimeTraceProjTreeModel(xlane3GateProjection(m1, m2, m3, m4, peer), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if n := strings.Count(fence, "其余 2 项微额锚定席"); n != 2 {
		t.Fatalf("each board folds its own micro seats (want 2 fold rows, got %d):\n%s", n, fence)
	}
	if strings.Contains(fence, "其余 4 项微额锚定席") {
		t.Fatalf("two boards' micro seats must never co-fold:\n%s", fence)
	}
	// (c) the representative cross-board note on board A's fold row, and the
	// reverse direction on the peer.
	joined := rspaFenceJoined(fence)
	if !strings.Contains(joined, "⇄另板互指·折叠成员(板锚 target.b-200,见图例)") {
		t.Fatalf("the fold row must carry the representative cross-board note:\n%s", fence)
	}
	if !strings.Contains(joined, "⇄另板[E1]、[E2](板锚 target.a-100,见图例)") {
		t.Fatalf("the peer's reverse refs must keep pointing at the folded members:\n%s", fence)
	}
	// (b) the fold rows carry the uniform board chip for their detail face.
	foldChips := 0
	for _, row := range model.TreeRows {
		if !row.Node.MicroAnchorFold {
			continue
		}
		if !strings.Contains(row.RankWindowChip, "板锚 target.") {
			t.Fatalf("a fold row must carry its uniform board chip, got %q", row.RankWindowChip)
		}
		foldChips++
	}
	if foldChips != 2 {
		t.Fatalf("want 2 chipped fold rows, got %d", foldChips)
	}
}

// TestXLANE3ElimHeadMultiBoardRuler — 修补轮 件G synthetic pins: the ◎ head's
// multi-board ruler + per-row anchors on a two-target board (zh + EN), and
// the single-board head byte-form staying single-thread.
func TestXLANE3ElimHeadMultiBoardRuler(t *testing.T) {
	projection := xlane3TwoBoardsProjection()
	projection.RootCauseFamilyObserved = true
	for i := range projection.OnChainCauses {
		projection.OnChainCauses[i].Predicate = "root_cause_tertiary"
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjElimOverviewFence(projection, model, true)
	if !strings.Contains(fence, "尺=各板目标线程 窗内墙钟ms·跨板不可相加") {
		t.Fatalf("the two-target ◎ head must speak the multi-board ruler:\n%s", fence)
	}
	if !strings.Contains(fence, "·板锚 target.a-100 [") || !strings.Contains(fence, "·板锚 target.b-200 [") {
		t.Fatalf("every named ◎ member must wear its board anchor:\n%s", fence)
	}
	enModel := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	// SMALL3-1 件3 (§29.196④) deliberate evolution: the EN board-anchor rows
	// exceed the ◎ budget and fold at seal time — unfold before judging.
	enFence := small3SquashSpaces(small3UnfoldElim(runtimeTraceProjElimOverviewFence(projection, enModel, false)))
	if !strings.Contains(enFence, small3SquashSpaces("ruler = each board's target thread, in-window wall-clock ms · never add across boards")) ||
		!strings.Contains(enFence, small3SquashSpaces(" · board target.b-200 [")) {
		t.Fatalf("the EN ◎ face must carry the multi-board ruler and anchors:\n%s", enFence)
	}
	// Single-board control: one board keeps the single-thread ruler verbatim.
	single := xlane3GateProjection(xlane3BoardSeat("s1", "workerX-300", "runnable", "runnable_wait", "target.a-100", "aaaa1111", 1, 5.0, 10))
	single.RootCauseFamilyObserved = true
	single.OnChainCauses[0].Predicate = "root_cause_tertiary"
	singleModel := buildRuntimeTraceProjTreeModel(single, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	singleFence := runtimeTraceProjElimOverviewFence(single, singleModel, true)
	if !strings.Contains(singleFence, "尺=target.a-100 窗内墙钟ms") || strings.Contains(singleFence, "各板目标线程") || strings.Contains(singleFence, "板锚") {
		t.Fatalf("the single-board ◎ head keeps the single-thread ruler:\n%s", singleFence)
	}
}

// xlane3TwoBoardsProjection is the 件1/件2/件3 representative shape: one query
// window hosting TWO rank boards (different targets), with one physical
// thread's scheduling-demand family seated on both boards (the donghu 形③
// witness geometry: 调度延迟 vs runnable cross-board coexistence).
func xlane3TwoBoardsProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"waker-1", "target.a-100"},
		WindowStartTs: 10.0,
		WindowEndTs:   10.2,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "xlane3-board-a-seat",
			Subject: "workerX-300", Object: "runnable_wait", TypeToken: "runnable_wait",
			StateKind: "runnable", ChainRelevance: "on_chain", ChainDepth: 1,
			ImpactMS: 5.0, CumulativeImpactMS: 5.0, EffectiveImpactMS: 5.0,
			QueryWindowStartTs: 10.0, QueryWindowEndTs: 10.2,
			RankBoardTarget: "target.a-100", RankBoardParamsFingerprint: "aaaa1111",
			Rank: 1, Tier: "primary", Confidence: 0.8, LineStart: 10, LineEnd: 20,
		}, {
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "xlane3-board-b-seat",
			Subject: "workerX-300", Object: "scheduler_latency", TypeToken: "scheduler_latency",
			StateKind: "runnable", ChainRelevance: "on_chain", ChainDepth: 1,
			ImpactMS: 3.0, CumulativeImpactMS: 3.0, EffectiveImpactMS: 3.0,
			QueryWindowStartTs: 10.0, QueryWindowEndTs: 10.2,
			RankBoardTarget: "target.b-200", RankBoardParamsFingerprint: "bbbb2222",
			Rank: 2, Tier: "secondary", Confidence: 0.8, LineStart: 30, LineEnd: 40,
		}},
	}
}

// xlane3ParamsForkProjection is the 件2 params-half shape: one window, ONE
// target, two boards whose rank knobs differ (参数# chip half only).
func xlane3ParamsForkProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"waker-1", "target.a-100"},
		WindowStartTs: 10.0,
		WindowEndTs:   10.2,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "xlane3-params-a-seat",
			Subject: "workerY-400", Object: "runnable_wait", TypeToken: "runnable_wait",
			StateKind: "runnable", ChainRelevance: "on_chain", ChainDepth: 1,
			ImpactMS: 5.0, CumulativeImpactMS: 5.0, EffectiveImpactMS: 5.0,
			QueryWindowStartTs: 10.0, QueryWindowEndTs: 10.2,
			RankBoardTarget: "target.a-100", RankBoardParamsFingerprint: "cccc3333",
			Rank: 1, Tier: "primary", Confidence: 0.8, LineStart: 10, LineEnd: 20,
		}, {
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "xlane3-params-b-seat",
			Subject: "workerZ-500", Object: "runnable_wait", TypeToken: "runnable_wait",
			StateKind: "runnable", ChainRelevance: "on_chain", ChainDepth: 1,
			ImpactMS: 3.0, CumulativeImpactMS: 3.0, EffectiveImpactMS: 3.0,
			QueryWindowStartTs: 10.0, QueryWindowEndTs: 10.2,
			RankBoardTarget: "target.a-100", RankBoardParamsFingerprint: "dddd4444",
			Rank: 2, Tier: "secondary", Confidence: 0.8, LineStart: 30, LineEnd: 40,
		}},
	}
}
