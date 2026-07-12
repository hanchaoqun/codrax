package orchestrator

// prose_lexicon_board_check_test.go — CR-1 件② (P1 约-extension + P2
// vocabulary) + 件⑤ (P3a/P3b) pins (§29.42/.3/.4 user rulings,
// docs/design/real_trace_campaign_20260705.md, 2026-07-12). All witnesses
// are the 41006 cold-read shapes: 案12 「约45ms」自造聚合数, 案22
// same_priority_dependency 词表外 token, 案17 双第一根因 + 静默榜序偏离.
// Every pin also asserts the §29.42.4 verbatim duty: ONE soft round, second
// draft ships, never a hard reject, never a caveat, advisory-log fallback.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func lexiconBoardViolations(vs []types.Violation) []types.Violation {
	var out []types.Violation
	for _, v := range vs {
		if v.Kind == types.ViolProseLexiconBoardInconsistent {
			out = append(out, v)
		}
	}
	return out
}

func lexiconBoardRankRecord(pos int, subject, typ string, notes ...string) types.ObservationRecord {
	rec := psgTraceRecord("trace_query:t#root_cause_rank:"+string(rune('0'+pos)), "root_cause_primary", "36.757", notes...)
	rec.ID = "trace_query:t#root_cause_rank:" + string(rune('0'+pos))
	rec.Subject = subject
	rec.Object = typ
	return rec
}

// TestProseApproxAggregateFlagged — CR-1 P1 extension, 案12 witness: an
// approximation-marked ms value that no published value reproduces (直接成员
// 与相对容差均不中) is flagged even when a coincidental PAIR SUM reproduces
// it — 约-marked wall clock never takes the cross-row sum exemption.
func TestProseApproxAggregateFlagged(t *testing.T) {
	// 15.758 + 5.395 = 21.153 — within ulp(21)=1 of the prose "约21ms".
	mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "15.758", "impact_ms=5.395"))
	bus := psgBus(mut)
	doc := psgProseDoc("binder 同步等待累计约21ms。")
	got := psgViolations(runProseScalarGroundingCheck(doc, bus, mut))
	if len(got) != 1 || !strings.Contains(got[0].Detail, "21ms") {
		t.Fatalf("approx-marked cross-row aggregate must be flagged, got %+v", got)
	}
}

// TestProseApproxRelativeToleranceGrounds — the loosening half: 约113ms
// against the published 111.916 token (0.96%% off, inside the 5%% relative
// band) is grounded; the same distance UNMARKED stays flagged (the relative
// band is exclusive to explicit approximation marks).
func TestProseApproxRelativeToleranceGrounds(t *testing.T) {
	mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "111.916"))
	bus := psgBus(mut)
	if got := psgViolations(runProseScalarGroundingCheck(psgProseDoc("peer 睡眠累计约113ms。"), bus, mut)); len(got) != 0 {
		t.Fatalf("约-marked value within the relative band must ground, got %+v", got)
	}
	mut2 := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "111.916"))
	bus2 := psgBus(mut2)
	if got := psgViolations(runProseScalarGroundingCheck(psgProseDoc("peer 睡眠累计 113ms。"), bus2, mut2)); len(got) != 1 {
		t.Fatalf("the relative band must stay exclusive to approx-marked values, got %+v", got)
	}
}

// TestProseUnmarkedSumExemptionPreserved — control: an UNMARKED honest total
// of two published figures keeps the pairwise-sum exemption byte-identically.
func TestProseUnmarkedSumExemptionPreserved(t *testing.T) {
	mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "15.758", "impact_ms=5.395"))
	bus := psgBus(mut)
	if got := psgViolations(runProseScalarGroundingCheck(psgProseDoc("两段合计 21.153ms。"), bus, mut)); len(got) != 0 {
		t.Fatalf("the unmarked pairwise-sum exemption must survive, got %+v", got)
	}
}

// TestProseLexiconUnpublishedTokenOneRoundLatch — P2 vocabulary arm, 案22
// witness: an engine-styled token no evidence surface published raises ONE
// soft violation; after the hint is dispatched the round is spent and the
// SAME draft ships (zero violations, latch set); a later retry with other
// kinds stays silent (sticky). No combination ever hard-rejects.
func TestProseLexiconUnpublishedTokenOneRoundLatch(t *testing.T) {
	mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "1.805", "type=binder_wait"))
	bus := psgBus(mut)
	doc := psgProseDoc("传导延迟 1.805ms,类型为 same_priority_dependency。")

	first := lexiconBoardViolations(runProseLexiconBoardCheck(doc, bus, mut))
	if len(first) != 1 {
		t.Fatalf("unpublished token must raise exactly one violation, got %+v", first)
	}
	v := first[0]
	if !strings.Contains(v.Detail, "same_priority_dependency") {
		t.Fatalf("Detail must name the token verbatim:\n%s", v.Detail)
	}
	if !strings.Contains(v.Repair, "ONE BY ONE") || !strings.Contains(v.Repair, "never reorder silently") ||
		!strings.Contains(v.Repair, "this reminder is delivered once") {
		t.Fatalf("Repair must carry the one-shot targeted directive:\n%s", v.Repair)
	}
	// S3' ① (§29.47.1, 2026-07-12): the raise is INFORMATION only — never
	// strict for the bus, never a repair round; the findings surface on the
	// system cross-check appendix instead.
	if isStrictViolationForBus(v, bus) {
		t.Fatalf("S3' ①: the lexicon/board raise must never be strict for the bus")
	}
	if got := FilterFinalizerRetryRootViolationsForBus([]types.Violation{v}, bus); len(got) != 0 {
		t.Fatalf("S3' ①: the raise must never become a retry root, got %+v", got)
	}
	// Same-attempt recheck stays consistent (latch only sets on dispatch).
	if got := lexiconBoardViolations(runProseLexiconBoardCheck(doc, bus, mut)); len(got) != 1 {
		t.Fatalf("same-attempt recheck must re-raise consistently, got %+v", got)
	}
	// The retry decision publishes the hint; the second draft MAINTAINS the
	// token — round spent, ships as written (§29.42.4: 第二稿维持即照发).
	mut.SetRetryState(&types.RetryState{Attempt: 1, ActiveViolations: []types.ScoredViolation{{
		Kind: types.ViolProseLexiconBoardInconsistent, Severity: types.SeverityMedium, Layer: "answer_oracle",
	}}})
	if got := lexiconBoardViolations(runProseLexiconBoardCheck(doc, bus, mut)); len(got) != 0 {
		t.Fatalf("the hint round is final for this lane, got %+v", got)
	}
	if !mut.ProseLexiconBoardHintDelivered() {
		t.Fatalf("observing the dispatched hint must set the sticky run latch")
	}
	// Sticky across later retries built for OTHER kinds — and even for NEW
	// findings of this lane (any combination: no second raise, no error).
	mut.SetRetryState(&types.RetryState{Attempt: 2, ActiveViolations: []types.ScoredViolation{{
		Kind: types.ViolBlockCoverageMissing, Severity: types.SeverityHigh, Layer: "v2_oracle",
	}}})
	worse := psgProseDoc("类型为 same_priority_dependency 与 sync_buffer_read;主根因是 keva-1-17437,主根因是 CompThread_0-2955。")
	if got := lexiconBoardViolations(runProseLexiconBoardCheck(worse, bus, mut)); len(got) != 0 {
		t.Fatalf("the run-level latch caps the lane at one round total, got %+v", got)
	}
}

// TestProseLexiconPublishedTokensZeroTouch — control: tokens the run
// published (registry universe, observation notes, tool output, the user's
// own request) never trigger.
func TestProseLexiconPublishedTokensZeroTouch(t *testing.T) {
	mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "1.805",
		"type=binder_wait", "chain_relevance=on_chain", "supply_fold_deficit_ms=2.286"))
	bus := psgBus(mut)
	doc := psgProseDoc("binder_wait 车道 chain_relevance=on_chain;供给折算 supply_fold_deficit_ms 已披露;d_state_or_io_wait 家族正常。")
	if got := lexiconBoardViolations(runProseLexiconBoardCheck(doc, bus, mut)); len(got) != 0 {
		t.Fatalf("published tokens must stay silent, got %+v", got)
	}
}

// TestProseBoardDualPrimaryContradiction — P3a (案17 双第一根因): two
// different entities claimed as THE primary root cause in one document draws
// the single soft hint.
func TestProseBoardDualPrimaryContradiction(t *testing.T) {
	mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "36.757"))
	bus := psgBus(mut)
	doc := psgProseDoc("主根因是 binder:496_9-10961 的同步阻塞。……综上,主根因为 CompThread_0-2955 的 D-state 等待。")
	got := lexiconBoardViolations(runProseLexiconBoardCheck(doc, bus, mut))
	if len(got) != 1 || !strings.Contains(got[0].Detail, "different entities as THE primary root cause") {
		t.Fatalf("dual-primary contradiction must raise one soft violation, got %+v", got)
	}
}

// TestProseBoardSilentDeviationVsDisclosed — P3b (§29.42.3 conscious-flip):
// a prose primary differing from the typed board's #1 seat WITHOUT
// disclosure wording draws the hint; the SAME deviation WITH disclosure
// wording ships silently.
func TestProseBoardSilentDeviationVsDisclosed(t *testing.T) {
	board := lexiconBoardRankRecord(1, "CompThread_0-2955", "d_state_or_io_wait",
		"rank=1", "tier=primary", "chain_relevance=on_chain", "effective_impact_ms=36.757")
	mut := psgTraceMutable(board)
	bus := psgBus(mut)
	silent := psgProseDoc("主根因是 binder:496_9-10961 的同步等待,合计 36.757ms。")
	got := lexiconBoardViolations(runProseLexiconBoardCheck(silent, bus, mut))
	if len(got) != 1 || !strings.Contains(got[0].Detail, "CompThread_0-2955") {
		t.Fatalf("silent board deviation must raise one soft violation naming the #1 seat, got %+v", got)
	}
	mut2 := psgTraceMutable(board)
	bus2 := psgBus(mut2)
	disclosed := psgProseDoc("主根因是 binder:496_9-10961 的同步等待(此为综合判断,与测量榜序不同:榜首为 CompThread_0-2955)。")
	if got := lexiconBoardViolations(runProseLexiconBoardCheck(disclosed, bus2, mut2)); len(got) != 0 {
		t.Fatalf("a DISCLOSED deviation must ship without a hint, got %+v", got)
	}
	mut3 := psgTraceMutable(board)
	bus3 := psgBus(mut3)
	aligned := psgProseDoc("主根因是 CompThread_0-2955 的 D-state 等待,合计 36.757ms。")
	if got := lexiconBoardViolations(runProseLexiconBoardCheck(aligned, bus3, mut3)); len(got) != 0 {
		t.Fatalf("a board-aligned primary must stay silent, got %+v", got)
	}
}

// TestProseBoardListFormPrimaryClaim — FEED-1② (复核, 2026-07-12), the 84618
// replay misfire shape: the model's own board opened 「根因排序:① app-9511
// …」 with NO superlative wording, so P3b never saw the claim. The board
// head's first-listed entity now binds as the primary claim: zero overlap
// with the typed #1 seat and no disclosure → exactly one soft hint; the
// disclosed variant ships silently.
func TestProseBoardListFormPrimaryClaim(t *testing.T) {
	board := lexiconBoardRankRecord(1, "CompThread_0-2955", "d_state_or_io_wait",
		"rank=1", "tier=primary", "chain_relevance=on_chain", "effective_impact_ms=36.757")
	mut := psgTraceMutable(board)
	bus := psgBus(mut)
	silent := psgProseDoc("根因排序:① app-9511 两次 sleep 聚合 20.816ms(TimerDispatch → app → ugc 链路)。② DetectViewRect-17679 14.302ms。")
	got := lexiconBoardViolations(runProseLexiconBoardCheck(silent, bus, mut))
	if len(got) != 1 || !strings.Contains(got[0].Detail, "CompThread_0-2955") {
		t.Fatalf("the prose board-list form must bind its first entry as the primary claim and flag the silent deviation, got %+v", got)
	}
	mut2 := psgTraceMutable(board)
	bus2 := psgBus(mut2)
	disclosed := psgProseDoc("根因排序(综合判断,与测量榜序不同——榜首为 CompThread_0-2955):① app-9511 两次 sleep 聚合 20.816ms。")
	if got := lexiconBoardViolations(runProseLexiconBoardCheck(disclosed, bus2, mut2)); len(got) != 0 {
		t.Fatalf("a DISCLOSED board-list deviation must ship without a hint, got %+v", got)
	}
}

// TestProseDeclaredSortKeyNonMonotonic — P3-1 (§29.42.3 P3a 第二子臂, 案17
// 键自违): a declared 「按…排序」 whose own listed ms values increase down
// the list is an internal contradiction — one soft finding, same latch.
func TestProseDeclaredSortKeyNonMonotonic(t *testing.T) {
	mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "15.758", "impact_ms=4.795"))
	bus := psgBus(mut)
	doc := psgProseDoc("按有效归因降序排序:① keva-1-17437 4.795ms ② binder:496_9-10961 15.758ms。")
	got := lexiconBoardViolations(runProseLexiconBoardCheck(doc, bus, mut))
	if len(got) != 1 || !strings.Contains(got[0].Detail, "do not follow it") {
		t.Fatalf("a self-violated declared sort key must raise one soft finding, got %+v", got)
	}
	mut2 := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "15.758", "impact_ms=4.795"))
	bus2 := psgBus(mut2)
	ordered := psgProseDoc("按有效归因降序排序:① binder:496_9-10961 15.758ms ② keva-1-17437 4.795ms。")
	if got := lexiconBoardViolations(runProseLexiconBoardCheck(ordered, bus2, mut2)); len(got) != 0 {
		t.Fatalf("a key-consistent list must stay silent, got %+v", got)
	}
}

// TestProseLexiconToolOutputTokensArePublished — P4-3 (复核, 2026-07-12):
// snake_case tokens printed by the CURRENT run's deterministic tool outputs
// are engine publications (冷读实锤: ohos_rt-class tokens come from tool
// output) — never a false soft hint.
func TestProseLexiconToolOutputTokensArePublished(t *testing.T) {
	mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "1.805"))
	bus := psgBus(mut)
	bus.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true,
		Summary: "thread load running=1.805ms priority_class=ohos_rt policy=next_info"}}
	doc := psgProseDoc("该线程调度类为 ohos_rt,policy 取 next_info。")
	if got := lexiconBoardViolations(runProseLexiconBoardCheck(doc, bus, mut)); len(got) != 0 {
		t.Fatalf("tool-output tokens are published vocabulary, got %+v", got)
	}
}

// TestProseLexiconStandardEventNamesAndAttachedCorpus — S1 (STAB-1,
// 2026-07-12), the 2779 witness: real kernel event names
// (block_rq_issue / irq_handler_entry / irq_handler_exit) were flagged as
// fabricated tokens and the resulting rewrite round DAMAGED the answer.
// Source b (the closed-set standard event dictionary) clears them without
// any attachment; source a (the attached trace's own token corpus, cached
// one-pass extraction) clears vendor tokens greppable in the attachment;
// a token published NOWHERE still flags (control).
func TestProseLexiconStandardEventNamesAndAttachedCorpus(t *testing.T) {
	// Source b: dictionary alone (no attachment).
	mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "1.805"))
	bus := psgBus(mut)
	doc := psgProseDoc("窗口内 block_rq_issue 与 irq_handler_entry/irq_handler_exit 事件密集。")
	if got := lexiconBoardViolations(runProseLexiconBoardCheck(doc, bus, mut)); len(got) != 0 {
		t.Fatalf("standard kernel event names must never flag (2779 shape), got %+v", got)
	}
	// Source a: a vendor token that exists ONLY in the attached trace text.
	mut2 := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "1.805"))
	bus2 := psgBus(mut2)
	bus2.AttachedHitrace = "  tpp-0 (2) [000] .... 1.0: tppmgr_sched_hint: cpu=0 state=2\n"
	vendor := psgProseDoc("厂商事件 tppmgr_sched_hint 表明调度提示介入。")
	if got := lexiconBoardViolations(runProseLexiconBoardCheck(vendor, bus2, mut2)); len(got) != 0 {
		t.Fatalf("attached-trace tokens must never flag, got %+v", got)
	}
	if mut2.AttachedArtifactLexicon() == nil {
		t.Fatalf("the attachment corpus must be cached after the first scan (one-pass discipline)")
	}
	// Control: a token published nowhere still flags.
	mut3 := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "1.805"))
	bus3 := psgBus(mut3)
	fabricated := psgProseDoc("类型为 same_priority_dependency。")
	if got := lexiconBoardViolations(runProseLexiconBoardCheck(fabricated, bus3, mut3)); len(got) != 1 {
		t.Fatalf("an unpublished token must still flag, got %+v", got)
	}
}

// TestProseApproxMarkerBoundaries — P3-2 (复核, 2026-07-12): the 约-marker
// must not fire through compound words (节约/预约…) or range notation
// (「108~113ms」), while a full-width-space-separated marker still counts.
func TestProseApproxMarkerBoundaries(t *testing.T) {
	run := func(prose string) int {
		mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "111.916"))
		bus := psgBus(mut)
		return len(psgViolations(runProseScalarGroundingCheck(psgProseDoc(prose), bus, mut)))
	}
	// Compound word: 节约 is prose, not an approximation mark — 113 must not
	// borrow the 5% relative band (111.916 is beyond the last-digit band).
	if got := run("本方案节约 113ms 的等待。"); got != 1 {
		t.Fatalf("a compound-word 约 must not open the relative band, got %d violations", got)
	}
	// Range notation: the tilde between two numerals is a range separator.
	if got := run("耗时区间 108~113ms。"); got != 1 {
		t.Fatalf("a range tilde must not open the relative band, got %d violations", got)
	}
	// Full-width space separation still marks the approximation.
	if got := run("累计约　113ms。"); got != 0 {
		t.Fatalf("a full-width-space-separated 约 must still mark the approximation, got %d violations", got)
	}
}

// TestProseLexiconBoardInertWithoutTraceEvidence — scope gate: non-trace
// runs never enter the lane.
func TestProseLexiconBoardInertWithoutTraceEvidence(t *testing.T) {
	mut := types.NewMutableState("普通代码问题")
	bus := psgBus(mut)
	doc := psgProseDoc("类型为 made_up_snake_token,主根因是 foo-1 又是 bar-2。")
	if got := lexiconBoardViolations(runProseLexiconBoardCheck(doc, bus, mut)); len(got) != 0 {
		t.Fatalf("non-trace runs must stay inert, got %+v", got)
	}
}

// TestProseLexiconBoardNeverHardRejects — §29.42.4 无硬拒路径 pin: whatever
// finding combination the scan produces, the lane emits at most ONE
// SoftByDefault violation and, once latched, ZERO — there is no error/hard
// termination path in this lane by construction.
func TestProseLexiconBoardNeverHardRejects(t *testing.T) {
	spec, ok := types.ViolKindSpecFor(types.ViolProseLexiconBoardInconsistent)
	if !ok || !spec.SoftByDefault {
		t.Fatalf("the lane's kind must be SoftByDefault at the registry: %+v", spec)
	}
	board := lexiconBoardRankRecord(1, "CompThread_0-2955", "d_state_or_io_wait",
		"rank=1", "tier=primary", "chain_relevance=on_chain", "effective_impact_ms=36.757")
	mut := psgTraceMutable(board)
	bus := psgBus(mut)
	// Every arm at once: unpublished tokens + dual primary + board deviation.
	doc := psgProseDoc("类型为 same_priority_dependency 与 sync_buffer_read;主根因是 binder:496_9-10961;另一处又称主根因为 keva-1-17437。")
	got := lexiconBoardViolations(runProseLexiconBoardCheck(doc, bus, mut))
	if len(got) != 1 {
		t.Fatalf("all finding combinations merge into exactly ONE soft violation, got %d: %+v", len(got), got)
	}
	mut.SetRetryState(&types.RetryState{Attempt: 1, ActiveViolations: []types.ScoredViolation{{
		Kind: types.ViolProseLexiconBoardInconsistent, Severity: types.SeverityMedium, Layer: "answer_oracle",
	}}})
	if got := lexiconBoardViolations(runProseLexiconBoardCheck(doc, bus, mut)); len(got) != 0 {
		t.Fatalf("after the single round the lane must be silent forever, got %+v", got)
	}
}

// TestSoftRepairHintsCarryMinimalDiffObligation — S2 (STAB-1, 2026-07-12,
// 2779 witness: the model answered a one-token soft hint with a whole-answer
// rewrite that came out WORSE than the first draft): both soft lanes' repair
// hints carry the mechanical smallest-edit directive — named blocks only,
// every other block byte-for-byte, block-level patch over full rewrite.
func TestSoftRepairHintsCarryMinimalDiffObligation(t *testing.T) {
	mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "1.805"))
	bus := psgBus(mut)
	lex := runProseLexiconBoardCheck(psgProseDoc("类型为 same_priority_dependency。"), bus, mut)
	if len(lex) != 1 {
		t.Fatalf("expected one lexicon violation, got %+v", lex)
	}
	mut2 := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "38.996"))
	bus2 := psgBus(mut2)
	psg := runProseScalarGroundingCheck(psgProseDoc("聚合影响 46.821ms。"), bus2, mut2)
	if len(psg) != 1 {
		t.Fatalf("expected one scalar violation, got %+v", psg)
	}
	for _, v := range []types.Violation{lex[0], psg[0]} {
		for _, want := range []string{
			"SMALLEST possible edit",
			"change ONLY the blocks named in the finding (by their block id)",
			"byte-for-byte identical",
			"prefer emit_answer_document_patch",
		} {
			if !strings.Contains(v.Repair, want) {
				t.Fatalf("%s repair hint missing %q:\n%s", v.Kind, want, v.Repair)
			}
		}
	}
}
