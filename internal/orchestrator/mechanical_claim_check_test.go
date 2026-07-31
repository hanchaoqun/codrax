package orchestrator

// mechanical_claim_check_test.go — EVALFIX-2C 类3 pins (2026-07-30,
// docs/design/evalfix2_class_designs_20260730.md CLASS 3 §7). Covers:
//   - the F7 witness (zh + en) raises exactly ONE merged soft violation
//     whose detail carries both numbers and whose repair hint explains
//     the direction fix (red-first: written against the empty scanner);
//   - direction-CORRECT claims raise nothing (判决性正例 — kills a
//     flipped comparison operator);
//   - the conservative skip set, one table row per predicate;
//   - unit normalization across us/ms/s;
//   - the one-round latch (PSG shape) and the decisive-tier ship-time
//     caveat (decisive fires, ordinary tier stays hint-only);
//   - the registry extension shape (next family = one table row);
//   - softness pins (registry default soft + promotable; never strict
//     for the gate or the bus).

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
)

func mccDoc(prose string) *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal, Text: prose,
	}}}
}

func mccMutable() *types.MutableState {
	return types.NewMutableState("分析滑动卡顿")
}

func mccViolations(vs []types.Violation) []types.Violation {
	var out []types.Violation
	for _, v := range vs {
		if v.Kind == types.ViolMechanicalClaimContradiction {
			out = append(out, v)
		}
	}
	return out
}

const mccF7Sentence = "Choreographer#doFrame 本身的执行时长（80ms）未超过 60Hz 帧预算（16.67ms），但其内部子阶段分布不均。"

// TestMechanicalClaim_F7WitnessRaisesOnce — the eval specimen: the
// sentence claims 80ms ≤ 16.67ms while 80 > 16.67 (≈4.8×). Exactly one
// merged violation; the detail names both numbers and the block; the
// repair hint explains the direction fix without internal vocabulary.
// Red-first: this test was written against the empty scanner.
func TestMechanicalClaim_F7WitnessRaisesOnce(t *testing.T) {
	mut := mccMutable()
	got := runMechanicalClaimCheck(mccDoc(mccF7Sentence), mut, "zh")
	if len(got) != 1 {
		t.Fatalf("F7 witness must raise exactly one merged violation, got %d: %+v", len(got), got)
	}
	v := got[0]
	if v.Kind != types.ViolMechanicalClaimContradiction {
		t.Fatalf("kind = %q", v.Kind)
	}
	if !strings.Contains(v.Detail, "80ms") || !strings.Contains(v.Detail, "16.67ms") ||
		!strings.Contains(v.Detail, `block "summary"`) {
		t.Fatalf("Detail must carry both numbers and the block id:\n%s", v.Detail)
	}
	// The arithmetic direction must be spelled out (80ms > 16.67ms).
	if !strings.Contains(v.Detail, "80ms > 16.67ms") {
		t.Fatalf("Detail must state the arithmetic direction verbatim:\n%s", v.Detail)
	}
	// Repair hint pin (LLM-facing): direction-word fix + mis-stated-number
	// fix + reword escape + minimal edit, zero internal plumbing names.
	for _, want := range []string{
		"ONE BY ONE",
		"fix the comparison word",
		"correct that number to the value this report's evidence publishes",
		"rewrite the sentence so the comparison it states agrees with its own numbers",
		"SMALLEST possible edit",
		"emit_answer_document_patch",
	} {
		if !strings.Contains(v.Repair, want) {
			t.Fatalf("Repair must contain %q:\n%s", want, v.Repair)
		}
	}
	for _, forbidden := range []string{"mechanicalClaim", "ViolMechanical", "orchestrator", "contract_check", "MutableState"} {
		if strings.Contains(v.Repair, forbidden) {
			t.Fatalf("Repair leaks internal vocabulary %q:\n%s", forbidden, v.Repair)
		}
	}
}

// TestMechanicalClaim_F7WitnessEnglishForm — the en spelling of the same
// contradiction ("does not exceed") triggers identically.
func TestMechanicalClaim_F7WitnessEnglishForm(t *testing.T) {
	mut := mccMutable()
	doc := mccDoc("The 80ms doFrame duration does not exceed the 16.67ms frame budget at 60Hz.")
	got := runMechanicalClaimCheck(doc, mut, "en")
	if len(got) != 1 {
		t.Fatalf("en witness must raise exactly one violation, got %+v", got)
	}
	if !strings.Contains(got[0].Detail, "80ms > 16.67ms") {
		t.Fatalf("Detail must state the arithmetic direction:\n%s", got[0].Detail)
	}
}

// TestMechanicalClaim_CorrectDirectionsSilent — 判决性正例: claims whose
// direction agrees with the numbers raise NOTHING; a flipped comparison
// operator in the validator turns this red.
func TestMechanicalClaim_CorrectDirectionsSilent(t *testing.T) {
	for _, prose := range []string{
		"执行时长 80ms 超过了 16.67ms 的帧预算。",
		"16.67ms 未超过 80ms。",
		"帧间隔 16.67ms 低于实测 80ms。",
		"The 80ms duration exceeds the 16.67ms frame budget.",
		"A 16.67ms frame is well within the 80ms window.",
		"实测 12ms 在 16.67ms 预算之内。",
	} {
		mut := mccMutable()
		if got := runMechanicalClaimCheck(mccDoc(prose), mut, "zh"); len(got) != 0 {
			t.Fatalf("direction-correct claim %q must stay silent, got %+v", prose, got)
		}
	}
}

// TestMechanicalClaim_SkipSet — one row per conservative skip predicate
// (precision over recall: every row must trigger NOTHING even though
// most carry an arithmetically reversed pair).
func TestMechanicalClaim_SkipSet(t *testing.T) {
	cases := []struct {
		name  string
		prose string
	}{
		{"modal_zh_normative", "帧预算要求执行时长不应超过 16.67ms，实测为 80ms。"},
		{"modal_zh_must", "每帧必须在 16.67ms 之内完成，而实测为 80ms。"},
		{"conditional_zh", "如果 80ms 超过 16.67ms 的预算，就会丢帧。"},
		{"conditional_zh_ruo", "若执行时长（80ms）未超过 16.67ms，则不会丢帧。"},
		{"modal_en_should", "The 80ms duration should not exceed the 16.67ms budget."},
		{"conditional_en_if", "If 80ms exceeds the 16.67ms budget, the frame is dropped."},
		{"question_zh", "执行时长（80ms）未超过 16.67ms 的帧预算？"},
		{"question_en", "Does the 80ms duration stay within the 16.67ms budget?"},
		{"range_endpoint", "各帧耗时 10～80ms 未超过 16.67ms 预算。"},
		{"near_value", "实测 17ms 未超过 16.67ms 预算。"},
		{"approx_narrow", "约 20ms 未超过 16.67ms 预算。"},
		{"missing_left_quantity", "实际执行时长超过了 16.67ms。"},
		{"unitless_side", "该函数的调用次数超过 3 次。"},
		{"cross_dimension", "占比 80% 未超过 16.67ms 的预算。"},
		{"zero_value", "实测 80ms 未超过 0ms。"},
		{"contested_binding", "80ms 未超过 16.67ms 高于 5ms。"},
		{"beyond_leash", "执行时长 80ms 在整个渲染管线的多个连续阶段中依次展开并逐级累积传递到最终的合成输出上，因此未超过 16.67ms。"},
		// R6-3 (round-6 sweep, eval/sweep_round6_findings_20260730.md):
		// reproduced false positives from negation over-match. 不仅/不过 are
		// non-negating compounds (closed set) — the comparator's own
		// direction is judged, and these sentences are CORRECT as written;
		// the en parenthetical / correlative "not" is not comparator
		// negation. All four rows raised a contradiction pre-fix.
		{"non_negating_compound_zh_bujin", "耗时 80ms 不仅超过了 16.67ms 的帧预算，还阻塞了后续输入。"},
		{"non_negating_compound_en_not_only", "The 80ms frame time not only exceeds the 16.67ms budget but also blocks input."},
		{"non_negating_compound_zh_buguo", "该帧总耗时 20ms，不过超过 16.67ms 预算的部分主要来自 GC。"},
		{"parenthetical_negation_en", "The 80ms cost (not counting GC) exceeds the 16.67ms budget."},
		// R6-3 precision rule: a negation-like token that is neither a
		// known non-negating compound nor immediately adjacent to the
		// comparator is AMBIGUOUS — never flip on a guess, skip the claim.
		{"ambiguous_negation_zh", "实测 80ms 不再超过 16.67ms 预算。"},
		{"ambiguous_negation_en", "The 80ms cost not counting GC exceeds the 16.67ms budget."},
		// R6-4 (round-6 sweep): operand binding stops at clause boundaries
		// (，。；、,;.) and at clause-opening 在…内 adverbial shapes — a
		// window/trace duration from the previous clause never binds as the
		// left operand of an attributive comparator phrase; with no
		// in-clause left operand the claim is SKIPPED. Both rows raised a
		// decisive 10000ms-vs-16.67ms contradiction pre-fix.
		{"cross_clause_left_operand_zh", "在 10s 的窗口内，低于 16.67ms 的帧占比 95%。"},
		{"cross_clause_left_operand_en", "During the 10s trace, frames below the 16.67ms budget account for 95% of all frames."},
		{"adverbial_scope_left_operand_zh", "在 10s 的窗口内低于 16.67ms 的帧占比 95%。"},
		// R7-0 (round-7 sweep): the 在…内 adverbial arm must be SIDE-FATAL,
		// not per-candidate. R6-4's predicate rejected only the candidate
		// sitting INSIDE the adverbial (在 before the token), so the farther
		// 80ms BEFORE the 在-opener bound across the closer and manufactured
		// a false contradiction (80ms 低于 16.67ms) on a CORRECT sentence
		// that was silent pre-R6 (10ms bound, true claim). The closer is now
		// a true barrier: any candidate whose gap crosses it dies, so the
		// whole left side is empty and the comparator skips.
		{"adverbial_barrier_side_fatal_zh", "总耗时 80ms 在 10ms 的采样间隔内低于 16.67ms 的样本占比达 95%。"},
		// R8-0 (round-8 sweep): R7-0's closer recognition re-anchored the
		// paired-在 search to the CLOSER position but kept the 30-rune bind
		// leash as its bound, so a descriptor long enough to push 在 beyond
		// the leash (38 runes back here) left the closer unrecognized while
		// the in-adverbial quantity still bound through its own ≤30-rune
		// gap — 3680.569s bound as the left operand of 低于 16.67ms and a
		// CORRECT sentence raised a decisive false contradiction. The
		// 在-pairing is a clause property, not a leash property: the search
		// is now bounded by the previous clause boundary (numeric-internal
		// '.'/',' transparent — 3680.569 must not stop the walk), never by
		// the leash.
		{"adverbial_long_descriptor_zh", "在 3680.569s 的完整采样窗口（覆盖三个连续 vsync 刷新周期）内低于 16.67ms 的帧占比达 95%。"},
		// R9-0/R9-2 (round-9 sweep): the clause-bounded paired-在 walk
		// severed on LEGITIMATE descriptor content — an enumeration 、, a
		// non-digit-flanked '.' (v2.x), or a comma before an elided second
		// 在 — leaving the closer unrecognized so the in-adverbial quantity
		// bound through and a CORRECT sentence raised a decisive false
		// contradiction (all three live-reproduced). Round 9 SIMPLIFIED the
		// arm (fourth consecutive round on this machinery): a 在 with ANY
		// later 内/中/里 before the comparator now declares the left
		// binding AMBIGUOUS and the claim skips — no pairing precision, no
		// clause walking, recall-only cost.
		{"adverbial_enumeration_descriptor_zh", "在渲染、合成两阶段的 10s 采样窗口内低于 16.67ms 的帧占比达 95%。"},
		{"adverbial_version_dot_descriptor_zh", "在 v2.x 版本共 3680s 的采样窗口内低于 16.67ms 的帧占比达 95%。"},
		{"adverbial_elided_zai_zh", "在压测过程中，10s 的采样窗口内低于 16.67ms 的帧占比达 95%。"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mut := mccMutable()
			if got := runMechanicalClaimCheck(mccDoc(tc.prose), mut, "zh"); len(got) != 0 {
				t.Fatalf("skip case %q must trigger NOTHING, got %+v", tc.prose, got)
			}
		})
	}
}

// TestMechanicalClaim_NonNegatingCompoundStillJudges — R6-3
// discriminator: the closed non-negating compound set must not silence
// the lane. With 不仅 / "not only" the base comparator keeps its own
// direction, so a genuinely reversed pair still fires (kills a fix that
// skips every sentence containing 不/not).
func TestMechanicalClaim_NonNegatingCompoundStillJudges(t *testing.T) {
	for _, prose := range []string{
		"耗时 10ms 不仅超过了 16.67ms 的帧预算，还阻塞了后续输入。",
		"The 10ms frame time not only exceeds the 16.67ms budget but also blocks input.",
	} {
		mut := mccMutable()
		got := runMechanicalClaimCheck(mccDoc(prose), mut, "zh")
		if len(got) != 1 {
			t.Fatalf("non-negating compound must keep the base direction judged for %q, got %+v", prose, got)
		}
		if !strings.Contains(got[0].Detail, "10ms") || !strings.Contains(got[0].Detail, "16.67ms") {
			t.Fatalf("Detail must carry the reversed pair:\n%s", got[0].Detail)
		}
	}
}

// TestMechanicalClaim_AdverbialCloserNeedsPairedZai — R7-0 (kept live
// under the R9 simplification) discriminating positive: the adverbial
// ambiguity skip requires a 在 in the sentence prefix. A word-internal
// closer with NO 在 anywhere before the comparator (内部 in a
// parenthetical aside) is trivially decidable — no adverbial can be
// open — so the genuinely reversed pair still binds and fires. Kills an
// over-broad fix that skips on ANY closer rune.
func TestMechanicalClaim_AdverbialCloserNeedsPairedZai(t *testing.T) {
	mut := mccMutable()
	got := runMechanicalClaimCheck(mccDoc("实测耗时 80ms（内部含 GC 停顿）未超过 16.67ms 帧预算。"), mut, "zh")
	if len(got) != 1 {
		t.Fatalf("closer rune without a paired 在 must not barrier the binding, got %+v", got)
	}
	if !strings.Contains(got[0].Detail, "80ms > 16.67ms") {
		t.Fatalf("Detail must carry the reversed pair:\n%s", got[0].Detail)
	}
}

// TestMechanicalClaim_SentenceRuneCapSkips — R9-1 (round-9 sweep):
// sentences beyond mechanicalClaimSentenceRuneCap runes are degenerate
// generations, not legitimate claim carriers — the whole
// numeric_direction pass skips them wholesale. Red-first: pre-cap the
// embedded reversed pair fired a decisive contradiction. The under-cap
// discriminating positive keeps the cap from silencing real prose.
func TestMechanicalClaim_SentenceRuneCapSkips(t *testing.T) {
	prose := "实测 80ms 未超过 16.67ms 预算" + strings.Repeat("而后续阶段依次逐级展开", 40) + "。"
	if n := len([]rune(prose)); n <= mechanicalClaimSentenceRuneCap {
		t.Fatalf("probe must exceed the sentence cap, got %d runes", n)
	}
	mut := mccMutable()
	if got := runMechanicalClaimCheck(mccDoc(prose), mut, "zh"); len(got) != 0 {
		t.Fatalf("an over-cap sentence must be skipped wholesale, got %+v", got)
	}
	mut2 := mccMutable()
	if got := runMechanicalClaimCheck(mccDoc("实测 80ms 未超过 16.67ms 预算。"), mut2, "zh"); len(got) != 1 {
		t.Fatalf("the same claim under the cap must still fire, got %+v", got)
	}
}

// TestMechanicalClaim_DegenerateSentencePerf — R9-1 perf pin: a ~27KB
// terminator-free run-on sentence (dense quantities, comparators and
// 在…内 closer runes — the round-9 degenerate probe shape) must complete
// the full lane scan well under budget. Pre-fix the per-closer per-gap
// adverbial walk was O(n²): ~32s at finalize and AGAIN at the ship-time
// rescan; the sentence-rune cap turns it into a wholesale skip. The
// scan used here (scanMechanicalClaimFindings) is the ONE detector both
// consumers share, so the pin covers both.
func TestMechanicalClaim_DegenerateSentencePerf(t *testing.T) {
	// Probe shape matters: barrier-free and 在-free, so pre-fix every
	// closer rune in every token-gap walked all the way back to the
	// sentence start (the unbounded-walk arm of R9-1) instead of
	// short-circuiting on a clause barrier or a paired 在.
	head := "总耗时 80ms 与 10ms 的采样间隔内低于 16ms 的样本占比达 95% "
	seg := "渲染合成两阶段的采样窗口内持续低于阈值且流水线依次展开 "
	var b strings.Builder
	b.WriteString(head)
	for b.Len() < 27*1024 {
		b.WriteString(seg)
	}
	doc := mccDoc(b.String())
	start := time.Now()
	findings := scanMechanicalClaimFindings(doc)
	elapsed := time.Since(start)
	if len(findings) != 0 {
		t.Fatalf("a degenerate over-cap sentence is not a claim carrier, got %+v", findings)
	}
	if elapsed > 200*time.Millisecond {
		t.Fatalf("degenerate 27KB probe took %v, budget 200ms", elapsed)
	}
}

// TestMechanicalClaim_ExemptBlocksNotScanned — the shared prose-unit
// collection exempts caveat / diagram / next-step blocks (reused from
// the prose_* family; zero new code) — the F7 sentence inside them
// never fires.
func TestMechanicalClaim_ExemptBlocksNotScanned(t *testing.T) {
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "caveats", Kind: types.BlockCaveat, Text: mccF7Sentence},
		{ID: "flow", Kind: types.BlockDiagram, Text: mccF7Sentence},
		{ID: "next_steps", Kind: types.BlockSection, Text: mccF7Sentence},
	}}
	mut := mccMutable()
	if got := runMechanicalClaimCheck(doc, mut, "zh"); len(got) != 0 {
		t.Fatalf("exempt blocks must not be scanned, got %+v", got)
	}
}

// TestMechanicalClaim_UnitNormalization — same-dimension units compare
// after normalization: 0.08s IS 80ms (fires against 16.67ms), 80µs is
// 0.08ms (the LE claim is true — silent).
func TestMechanicalClaim_UnitNormalization(t *testing.T) {
	mut := mccMutable()
	got := runMechanicalClaimCheck(mccDoc("执行时长 0.08s 未超过 16.67ms 的帧预算。"), mut, "zh")
	if len(got) != 1 {
		t.Fatalf("0.08s vs 16.67ms must fire after normalization, got %+v", got)
	}
	if !strings.Contains(got[0].Detail, "80ms > 16.67ms") {
		t.Fatalf("Detail must show normalized ms values:\n%s", got[0].Detail)
	}
	mut2 := mccMutable()
	if got := runMechanicalClaimCheck(mccDoc("执行时长 80µs 未超过 16.67ms 的帧预算。"), mut2, "zh"); len(got) != 0 {
		t.Fatalf("80µs ≤ 16.67ms is TRUE as written — must stay silent, got %+v", got)
	}
}

// TestMechanicalClaim_IndependentBindingsInOneSentence — multiple
// comparators bind their own nearest tokens independently; only the
// contradicted one reports (and non-contested pairs are unaffected by
// the ambiguity predicate).
func TestMechanicalClaim_IndependentBindingsInOneSentence(t *testing.T) {
	mut := mccMutable()
	got := runMechanicalClaimCheck(mccDoc("阶段一 80ms 超过 16.67ms 且阶段二 90ms 未超过 20ms。"), mut, "zh")
	if len(got) != 1 {
		t.Fatalf("exactly the second comparator contradicts, got %+v", got)
	}
	if !strings.Contains(got[0].Detail, "90ms > 20ms") {
		t.Fatalf("Detail must carry the contradicted pair only:\n%s", got[0].Detail)
	}
	if strings.Contains(got[0].Detail, "80ms > 16.67ms") {
		t.Fatalf("the direction-correct pair must not be reported:\n%s", got[0].Detail)
	}
}

// TestMechanicalClaim_OneRoundLatch — PSG-shaped anti-livelock: within
// one attempt the verdict repeats; once a dispatched retry surface
// carried the kind the lane never raises again this run, even after a
// later retry rebuilds the surface without the kind.
func TestMechanicalClaim_OneRoundLatch(t *testing.T) {
	mut := mccMutable()
	doc := mccDoc(mccF7Sentence)
	if got := runMechanicalClaimCheck(doc, mut, "zh"); len(got) != 1 {
		t.Fatalf("first pass must raise, got %+v", got)
	}
	// Same-attempt recheck stays consistent (latch never set at raise time).
	if got := runMechanicalClaimCheck(doc, mut, "zh"); len(got) != 1 {
		t.Fatalf("same-attempt recheck must re-raise, got %+v", got)
	}
	mut.SetRetryState(&types.RetryState{Attempt: 1, ActiveViolations: []types.ScoredViolation{{
		Kind: types.ViolMechanicalClaimContradiction, Severity: types.SeveritySoft, Layer: "answer_oracle",
	}}})
	if got := runMechanicalClaimCheck(doc, mut, "zh"); len(got) != 0 {
		t.Fatalf("the hint round is final for this lane, got %+v", got)
	}
	if !mut.MechanicalClaimHintDelivered() {
		t.Fatalf("observing the dispatched hint must set the sticky run latch")
	}
	mut.SetRetryState(&types.RetryState{Attempt: 2, ActiveViolations: []types.ScoredViolation{{
		Kind: types.ViolBlockCoverageMissing, Severity: types.SeverityHigh, Layer: "v2_oracle",
	}}})
	if got := runMechanicalClaimCheck(doc, mut, "zh"); len(got) != 0 {
		t.Fatalf("the run-level latch caps the lane at one round total, got %+v", got)
	}
}

func mccDeliverHint(t *testing.T, mut *types.MutableState, doc *types.AnswerDocumentV2) {
	t.Helper()
	mut.SetRetryState(&types.RetryState{Attempt: 1, ActiveViolations: []types.ScoredViolation{{
		Kind: types.ViolMechanicalClaimContradiction, Severity: types.SeveritySoft, Layer: "answer_oracle",
	}}})
	if got := runMechanicalClaimCheck(doc, mut, "zh"); len(got) != 0 {
		t.Fatalf("hint delivery pass must stay silent, got %+v", got)
	}
	if !mut.MechanicalClaimHintDelivered() {
		t.Fatalf("hint delivery must set the sticky run latch")
	}
}

// TestMechanicalClaim_ResidualCaveatDecisiveOnly — ship-time disclosure
// two-tier pin: a decisive (≥2×) residual on the shipped doc appends
// the caveat exactly once (CAVSTR dedup); an ordinary-tier residual
// (~1.3×) got the hint but ships with NO caveat.
func TestMechanicalClaim_ResidualCaveatDecisiveOnly(t *testing.T) {
	// Decisive arm: 80 vs 16.67 is 4.8×.
	mut := mccMutable()
	doc := mccDoc(mccF7Sentence)
	if got := runMechanicalClaimCheck(doc, mut, "zh"); len(got) != 1 {
		t.Fatalf("first pass must raise, got %+v", got)
	}
	mccDeliverHint(t, mut, doc)
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	o := &Orchestrator{busCtx: &types.BusContext{Ctx: context.Background(), Mutable: mut, Language: "zh"}}
	answer := o.appendMechanicalClaimResidualCaveatToAnswer("正文结论。")
	if !strings.HasPrefix(answer, "正文结论。") {
		t.Fatalf("the body must never be rewritten:\n%s", answer)
	}
	if !strings.Contains(answer, "以下断言中两个数值的比较方向与其数值本身不一致，请谨慎采信：") ||
		!strings.Contains(answer, "80ms > 16.67ms") {
		t.Fatalf("decisive residual must append the direction caveat:\n%s", answer)
	}
	// CAVSTR dedup: appending again is a no-op.
	if again := o.appendMechanicalClaimResidualCaveatToAnswer(answer); again != answer {
		t.Fatalf("the caveat must append at most once, got:\n%s", again)
	}

	// Ordinary-tier arm: 22 vs 16.67 is 1.32× (rel diff ≈24% > margin,
	// under the decisive ratio) — hint yes, caveat no.
	mut2 := mccMutable()
	doc2 := mccDoc("实测 22ms 未超过 16.67ms 预算。")
	if got := runMechanicalClaimCheck(doc2, mut2, "zh"); len(got) != 1 {
		t.Fatalf("ordinary-tier contradiction must still raise the hint, got %+v", got)
	}
	mccDeliverHint(t, mut2, doc2)
	mut2.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc2)
	o2 := &Orchestrator{busCtx: &types.BusContext{Ctx: context.Background(), Mutable: mut2, Language: "zh"}}
	if answer := o2.appendMechanicalClaimResidualCaveatToAnswer("正文结论。"); answer != "正文结论。" {
		t.Fatalf("ordinary-tier residual must ship with NO caveat, got:\n%s", answer)
	}
}

// TestMechanicalClaim_RepairedDocAndColdRunNoCaveat — the caveat reads
// the SHIPPED document (repaired doc → nothing even with the latch
// set), and a run where the lane never fired ships byte-identical.
func TestMechanicalClaim_RepairedDocAndColdRunNoCaveat(t *testing.T) {
	mut := mccMutable()
	bad := mccDoc(mccF7Sentence)
	if got := runMechanicalClaimCheck(bad, mut, "zh"); len(got) != 1 {
		t.Fatalf("first pass must raise, got %+v", got)
	}
	mccDeliverHint(t, mut, bad)
	repaired := mccDoc("Choreographer#doFrame 本身的执行时长（80ms）超过了 60Hz 帧预算（16.67ms）。")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, repaired)
	o := &Orchestrator{busCtx: &types.BusContext{Ctx: context.Background(), Mutable: mut, Language: "zh"}}
	if answer := o.appendMechanicalClaimResidualCaveatToAnswer("正文结论。"); answer != "正文结论。" {
		t.Fatalf("a repaired final doc must ship with no caveat, got:\n%s", answer)
	}
	// Cold run: latch never set.
	mut2 := mccMutable()
	mut2.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, mccDoc(mccF7Sentence))
	o2 := &Orchestrator{busCtx: &types.BusContext{Ctx: context.Background(), Mutable: mut2, Language: "zh"}}
	if answer := o2.appendMechanicalClaimResidualCaveatToAnswer("正文结论。"); answer != "正文结论。" {
		t.Fatalf("a run whose lane never fired must ship byte-identical, got:\n%s", answer)
	}
}

// TestMechanicalClaim_RegistryExtensionShape — the "next family = one
// registry row" promise is TESTED, not commented: injecting a fake
// checker row makes the shared shell consume it with zero other
// changes; all families merge into ONE violation; the finding cap
// bounds the merge; both language faces render through the caveat
// message. Also pins the first row's identity.
func TestMechanicalClaim_RegistryExtensionShape(t *testing.T) {
	if len(mechanicalClaimCheckers) < 1 || mechanicalClaimCheckers[0].Name != "numeric_direction" {
		t.Fatalf("registry must lead with the numeric_direction family, got %+v", mechanicalClaimCheckers)
	}
	saved := mechanicalClaimCheckers
	t.Cleanup(func() { mechanicalClaimCheckers = saved })
	fake := mechanicalClaimChecker{Name: "fake_family", Scan: func(unit proseTextUnit, spans [][2]int) []mechanicalClaimFinding {
		var out []mechanicalClaimFinding
		for i := 0; i < 20; i++ {
			out = append(out, mechanicalClaimFinding{
				entry:    "fake finding en",
				entryZH:  "fake finding zh",
				blockID:  unit.blockID,
				decisive: true,
			})
		}
		return out
	}}
	mechanicalClaimCheckers = append(append([]mechanicalClaimChecker{}, saved...), fake)

	mut := mccMutable()
	got := runMechanicalClaimCheck(mccDoc(mccF7Sentence), mut, "zh")
	if len(got) != 1 {
		t.Fatalf("all families must merge into ONE violation, got %d: %+v", len(got), got)
	}
	if !strings.Contains(got[0].Detail, "80ms > 16.67ms") || !strings.Contains(got[0].Detail, "fake finding en") {
		t.Fatalf("the merged detail must carry findings from BOTH families:\n%s", got[0].Detail)
	}
	// Cap: 1 real + 20 fake findings collapse to the named cap.
	findings := scanMechanicalClaimFindings(mccDoc(mccF7Sentence))
	if len(findings) != mechanicalClaimFindingCap {
		t.Fatalf("finding cap must bound the merge at %d, got %d", mechanicalClaimFindingCap, len(findings))
	}
	// Dual-language rendering through the shared caveat face.
	zh := mechanicalClaimResidualCaveatMessage("zh", findings[:1])
	en := mechanicalClaimResidualCaveatMessage("en", findings[:1])
	if !strings.Contains(zh, "请谨慎采信") || !strings.Contains(zh, findings[0].entryZH) {
		t.Fatalf("zh caveat face wrong: %q", zh)
	}
	if !strings.Contains(en, "treat them with caution") || !strings.Contains(en, findings[0].entry) {
		t.Fatalf("en caveat face wrong: %q", en)
	}
}

// TestMechanicalClaim_SoftnessPins — the lane is soft everywhere by
// default: registry default-soft + promotable; the contract gate never
// flips on this kind alone; the bus never treats it as strict.
func TestMechanicalClaim_SoftnessPins(t *testing.T) {
	spec, ok := types.ViolKindSpecFor(types.ViolMechanicalClaimContradiction)
	if !ok {
		t.Fatalf("kind must have a registry spec")
	}
	if !spec.SoftByDefault || spec.DefaultSeverity != types.SeveritySoft || !spec.Promotable {
		t.Fatalf("registry must classify the kind soft-by-default + promotable, got %+v", spec)
	}
	if spec.CaveatFamilyID != "" {
		t.Fatalf("the lane owns its own deterministic caveat — no caveat family, got %q", spec.CaveatFamilyID)
	}
	vs := []types.Violation{{Kind: types.ViolMechanicalClaimContradiction}}
	if hasAnyStrictViolation(vs) {
		t.Fatalf("a pure mechanical-claim violation set must never be strict for the gate")
	}
	bus := &types.BusContext{Ctx: context.Background()}
	if isStrictViolationForBus(vs[0], bus) {
		t.Fatalf("the kind must never be strict for the bus by default")
	}
}

// TestMechanicalClaim_ContractCheckEndToEnd — the wiring pin (唯一行为
// 变更的发布接线必配正向 pin): the F7 witness raises through
// runContractCheck itself, the soft-only round still PASSES the
// contract, and the one-round cap holds end to end.
func TestMechanicalClaim_ContractCheckEndToEnd(t *testing.T) {
	mut := mccMutable()
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, mccDoc(mccF7Sentence))
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.busCtx = &types.BusContext{Ctx: context.Background(), Mutable: mut, Language: "zh"}
	out := &agent.StageOutput{FinalAnswer: "答案正文"}

	res := runContractCheck(out, types.AnswerContract{}, mut, o, contractCheckSkipLLMReview())
	raised := mccViolations(res.Violations)
	if len(raised) != 1 {
		t.Fatalf("the wired check must raise exactly once through runContractCheck, got %+v", res.Violations)
	}
	if !res.Passed {
		t.Fatalf("a soft-only mechanical-claim round must PASS the contract (zero answer cost), violations: %+v", res.Violations)
	}
	// The retry decision publishes the hint; the lane goes silent.
	populateRetryState(mut, res, 0)
	res2 := runContractCheck(out, types.AnswerContract{}, mut, o, contractCheckSkipLLMReview())
	if got := mccViolations(res2.Violations); len(got) != 0 {
		t.Fatalf("the one-round cap must hold through runContractCheck, got %+v", got)
	}
}
