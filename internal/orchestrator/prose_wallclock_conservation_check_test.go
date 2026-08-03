package orchestrator

// CR-3 件① P6 wall-clock conservation gate pins (§29.42 P6; §29.50 遗留
// B1/B2, docs/design/real_trace_campaign_20260705.md, 2026-07-12).
//
// Witness fixtures (engine-real shapes):
//   - tieba 三元组 (CR-2 冷读 B1): prose running 20.372 + runnable 46.364
//     against the published full-window partition (26.946/3.636/84.358,
//     window 114.940) — Σ arm + single-dimension arm both catch it.
//   - 案11 form (108.8 + 168 > 233): two prose claims alone bust the window.
//   - CAL-1 F-8: prose 7.830ms same-core queueing vs published runnable
//     total 5.604 — single dimension over the published total.
//
// 全批软纪律 (§29.42.4/§29.47.1): everything here is DISCLOSURE ONLY —
// the provider feeds the system cross-check appendix; it never mints a
// violation and never enters a retry surface.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func p6AccountRecord(subject string, running, runnable, sleep, dstate, sleepIO, total, windowMS float64) types.ObservationRecord {
	rec := psgTraceRecord("r-acct-"+subject, "target_window_states:"+subject, fmt.Sprintf("%.3f", total))
	rec.Subject = subject
	rec.Predicate = "target_window_states"
	rec.Object = "state_partition"
	rec.RichNotes = []string{
		fmt.Sprintf("%s=%.3f", types.TraceNoteKeyRunning, running),
		fmt.Sprintf("%s=%.3f", types.TraceNoteKeyRunnable, runnable),
		fmt.Sprintf("%s=%.3f", types.TraceNoteKeySleep, sleep),
		fmt.Sprintf("%s=%.3f", types.TraceNoteKeyDState, dstate),
		fmt.Sprintf("%s=%.3f", types.TraceNoteKeySleepIOWait, sleepIO),
		fmt.Sprintf("%s=%.3f", types.TraceNoteKeyTotal, total),
		fmt.Sprintf("%s=%.3f", types.TraceNoteKeyWindowMS, windowMS),
		types.TraceNoteKeySelectedWindow + "=34579.472865..34579.587805",
	}
	return rec
}

// tieba full-window partition (CR-2 冷读复算真值): running 26.946 +
// runnable 3.636 + sleep 84.358 + d_state 0 ≈ window 114.940.
func p6TiebaAccount() types.ObservationRecord {
	return p6AccountRecord("com.baidu.tieba-59566", 26.946, 3.636, 84.358, 0, 0, 114.940, 114.940)
}

// donghu full-window partition (CAL-1 真值): running 157.248 + runnable
// 5.604 + sleep 70.338 ≈ window 233.190.
func p6DonghuAccount() types.ObservationRecord {
	return p6AccountRecord(".ugc.aweme.lite-17267", 157.248, 5.604, 70.338, 0, 0, 233.190, 233.190)
}

// TestProseWallClockConservation_TiebaTripleSumForm — 件① witness: the
// CR-2 B1 tieba triple. Prose claims running 20.372 + runnable 46.364 for
// the main thread; with the published sleep truth the partition sums past
// the window → the Σ-form disclosure (「状态时长之和…超过窗长」) is present,
// and the runnable claim also exceeds its published total (single-dim arm).
func TestProseWallClockConservation_TiebaTripleSumForm(t *testing.T) {
	mut := psgTraceMutable(p6TiebaAccount())
	bus := psgBus(mut)
	doc := psgProseDoc("com.baidu.tieba-59566 主线程在窗口内实际运行仅 20.372ms（占 17.7%），runnable 46.364ms，其余时间处于就绪排队或睡眠等待状态。")

	findings := proseWallClockConservationFindings(doc, bus, mut)
	if len(findings) == 0 {
		t.Fatalf("tieba triple must disclose, got none")
	}
	var sumForm, dimForm bool
	for _, f := range findings {
		zh := f.userReadable("zh")
		if strings.Contains(zh, "状态时长之和") && strings.Contains(zh, "超过窗长") &&
			strings.Contains(zh, "口径混用或数值错误") && strings.Contains(zh, "com.baidu.tieba-59566") {
			sumForm = true
			// 附注自证义务 (CR-2 C-1 lesson): the Σ decomposition lists REAL
			// component values with their sources, never invented pairs.
			if !strings.Contains(zh, "46.364") || !strings.Contains(zh, "84.358") {
				t.Fatalf("Σ decomposition must list the real components:\n%s", zh)
			}
		}
		if strings.Contains(zh, "超过该线程该维度已发布总量") && strings.Contains(zh, "46.364") &&
			strings.Contains(zh, "3.636") {
			dimForm = true
		}
	}
	if !sumForm {
		t.Fatalf("Σ-form disclosure missing: %+v", findings)
	}
	if !dimForm {
		t.Fatalf("single-dimension-over-published disclosure missing: %+v", findings)
	}
}

// TestProseWallClockConservation_Case11WindowBust — 案11 form: two prose
// claims for one thread sum past the window on their own (108.8 + 168 >
// 233.190 × (1+ε)).
func TestProseWallClockConservation_Case11WindowBust(t *testing.T) {
	mut := psgTraceMutable(p6DonghuAccount())
	bus := psgBus(mut)
	doc := psgProseDoc("ugc.aweme.lite-17267 在窗口内运行 108.8ms，睡眠 168ms。")

	findings := proseWallClockConservationFindings(doc, bus, mut)
	var sumForm bool
	for _, f := range findings {
		zh := f.userReadable("zh")
		if strings.Contains(zh, "状态时长之和") && strings.Contains(zh, "超过窗长") {
			sumForm = true
		}
	}
	if !sumForm {
		t.Fatalf("案11 window bust must disclose the Σ form, got %+v", findings)
	}
}

// TestProseWallClockConservation_F8SingleDimensionOverPublished — CAL-1
// F-8: prose states 7.830ms of same-core contention for the target
// (designator 目标自身, CPU context in-sentence) while the published
// full-window runnable total is 5.604ms.
func TestProseWallClockConservation_F8SingleDimensionOverPublished(t *testing.T) {
	mut := psgTraceMutable(p6DonghuAccount())
	bus := psgBus(mut)
	doc := psgProseDoc("runnable_wait 12.234 ms，竞争对手包括 udk-irq-12-92（0.418 ms）等，目标自身在 cpu=12 竞争 7.830 ms。")

	findings := proseWallClockConservationFindings(doc, bus, mut)
	var dimForm bool
	for _, f := range findings {
		zh := f.userReadable("zh")
		if strings.Contains(zh, "7.830") && strings.Contains(zh, "超过该线程该维度已发布总量") &&
			strings.Contains(zh, "5.604") {
			dimForm = true
		}
	}
	if !dimForm {
		t.Fatalf("F-8 single-dimension arm must disclose 7.830 vs 5.604, got %+v", findings)
	}
}

// TestProseWallClockConservation_SingleValueOverWindow — 单值>窗长 arm: a
// lone state duration larger than the window discloses even without a
// second claim.
func TestProseWallClockConservation_SingleValueOverWindow(t *testing.T) {
	mut := psgTraceMutable(p6TiebaAccount())
	bus := psgBus(mut)
	doc := psgProseDoc("com.baidu.tieba-59566 在窗口内睡眠 300ms。")

	findings := proseWallClockConservationFindings(doc, bus, mut)
	var windowForm bool
	for _, f := range findings {
		zh := f.userReadable("zh")
		if strings.Contains(zh, "300ms") && strings.Contains(zh, "超过窗长") {
			windowForm = true
		}
	}
	if !windowForm {
		t.Fatalf("single value over window must disclose, got %+v", findings)
	}
}

// TestProseWallClockConservation_TruthfulProseZeroTouch — control: prose
// that restates the published partition verbatim (Σ == window) stays
// silent, as do honest roundings of a small published total.
func TestProseWallClockConservation_TruthfulProseZeroTouch(t *testing.T) {
	mut := psgTraceMutable(p6TiebaAccount())
	bus := psgBus(mut)
	doc := psgProseDoc("com.baidu.tieba-59566 在窗口内运行 26.946ms，runnable 4ms，睡眠 84.358ms。")

	if findings := proseWallClockConservationFindings(doc, bus, mut); len(findings) != 0 {
		t.Fatalf("truthful prose must stay silent, got %+v", findings)
	}
}

func TestProseWallClockConservation_IOWaitUsesExclusiveTypedLane(t *testing.T) {
	rec := p6AccountRecord("io-worker-42", 9.365, 0, 0, 0, 0, 10, 10)
	rec.RichNotes = append(rec.RichNotes, types.TraceNoteKeyIOWait+"=0.635")
	mut := psgTraceMutable(rec)
	bus := psgBus(mut)

	truthful := psgProseDoc("io-worker-42 在窗口内 IO等待 0.635ms。")
	if findings := proseWallClockConservationFindings(truthful, bus, mut); len(findings) != 0 {
		t.Fatalf("typed io_wait value must not be compared with non-IO D-state: %+v", findings)
	}

	excess := psgProseDoc("io-worker-42 在窗口内 IO等待 1.200ms。")
	findings := proseWallClockConservationFindings(excess, bus, mut)
	if len(findings) == 0 || !strings.Contains(findings[0].userReadable("zh"), "0.635") {
		t.Fatalf("an actual io_wait excess must still disclose against 0.635ms: %+v", findings)
	}
}

// TestProseWallClockConservation_UnboundValuesZeroTouch — values with no
// same-sentence thread binding, no state keyword, or a keyword too far
// away never enter the claim set.
func TestProseWallClockConservation_UnboundValuesZeroTouch(t *testing.T) {
	mut := psgTraceMutable(p6TiebaAccount())
	bus := psgBus(mut)
	doc := psgProseDoc("整体开销约 500ms。com.baidu.tieba-59566 处理该任务总计耗时 400ms。所有线程合计运行 900ms。")

	if findings := proseWallClockConservationFindings(doc, bus, mut); len(findings) != 0 {
		t.Fatalf("unbound / keyword-less values must stay silent, got %+v", findings)
	}
}

// TestProseWallClockConservation_CrossThreadAggregatesSilent — a
// cross-thread busy aggregate (no thread token in the sentence) and a
// contention duration WITHOUT CPU context never claim a dimension.
func TestProseWallClockConservation_CrossThreadAggregatesSilent(t *testing.T) {
	mut := psgTraceMutable(p6TiebaAccount())
	bus := psgBus(mut)
	doc := psgProseDoc("十二个核心合计运行 900ms。锁竞争 400ms 发生于 com.baidu.tieba-59566 与其他线程之间。")

	if findings := proseWallClockConservationFindings(doc, bus, mut); len(findings) != 0 {
		t.Fatalf("aggregates / lock contention must stay silent, got %+v", findings)
	}
}

// TestProseWallClockConservation_NonTraceRunsInert — the scope gate: no
// deterministic runtime-query observation → the provider is inert.
func TestProseWallClockConservation_NonTraceRunsInert(t *testing.T) {
	mut := types.NewMutableState("普通源码问题")
	bus := psgBus(mut)
	doc := psgProseDoc("com.baidu.tieba-59566 运行 900ms。")

	if findings := proseWallClockConservationFindings(doc, bus, mut); len(findings) != 0 {
		t.Fatalf("non-trace runs must be inert, got %+v", findings)
	}
}

// TestProseWallClockConservation_RetiredFromProduction — ARITHSUBJ1: the
// free-prose provider remains an offline diagnostic, but no shipping surface
// may publish its inferred subject/value verdict or use it for a retry.
func TestProseWallClockConservation_RetiredFromProduction(t *testing.T) {
	mut := psgTraceMutable(p6TiebaAccount())
	bus := psgBus(mut)
	doc := psgProseDoc("com.baidu.tieba-59566 主线程实际运行仅 20.372ms，runnable 46.364ms。")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)

	o := &Orchestrator{busCtx: bus}
	findings := o.collectSystemCrossCheckFindings()
	for _, f := range findings {
		if strings.Contains(f, "状态时长之和") || strings.Contains(f, "超过该线程该维度已发布总量") {
			t.Fatalf("free-prose conservation verdict leaked into production: %v", findings)
		}
	}
	// The PSG violation lane must not have gained a new raise from P6
	// (the conservation claims are grounded numerals — the PSG membership
	// arm is a separate lane and stays as-is).
	for _, v := range runProseScalarGroundingCheck(doc, bus, mut) {
		if strings.Contains(v.Detail, "状态时长之和") || strings.Contains(v.Detail, "conservation") {
			t.Fatalf("P6 must never mint violations: %+v", v)
		}
	}
}

// TestProseWallClockConservation_NoInternalJargon — appendix wording is
// user-facing: no pipeline vocabulary, no Go type names, no check names.
func TestProseWallClockConservation_NoInternalJargon(t *testing.T) {
	mut := psgTraceMutable(p6TiebaAccount(), p6DonghuAccount())
	bus := psgBus(mut)
	doc := psgProseDoc("com.baidu.tieba-59566 主线程实际运行仅 20.372ms，runnable 46.364ms，睡眠 300ms。目标自身在 cpu=12 竞争 7.830 ms。")

	findings := proseWallClockConservationFindings(doc, bus, mut)
	if len(findings) == 0 {
		t.Fatalf("expected findings for the jargon audit")
	}
	for _, f := range findings {
		for _, lang := range []string{"zh", "en"} {
			text := f.userReadable(lang)
			for _, banned := range []string{"P6", "orchestrator", "MutableState", "BusContext", "ledger", "wallclock", "proseWallClock", "finalize"} {
				if strings.Contains(text, banned) {
					t.Fatalf("appendix wording leaks internal term %q:\n%s", banned, text)
				}
			}
		}
	}
}

// TestProseWallClockConservation_TrailingDesignatorAnaphoraSilent — 首放
// 误报实证 (tieba 复放 2026-07-12, verbatim): the sentence's subject is the
// anaphoric 该线程 (NetworkService, previous sentence); the trailing 主线程
// appears only inside a RELATION clause ("与主线程优先级关系") — a
// designator after the value never binds, so the 20.342 claim stays off
// the main thread and both conservation arms stay silent.
func TestProseWallClockConservation_TrailingDesignatorAnaphoraSilent(t *testing.T) {
	mut := psgTraceMutable(p6TiebaAccount())
	bus := psgBus(mut)
	doc := psgProseDoc("NetworkService-60595 等待 ThreadPoolForeg-60555 完成 IO 后方可执行网络操作。该线程自身 runnable_wait 累计 20.342ms（#2 根因），同时存在 priority_inversion_candidate 效应（14.597ms，#6 根因），lower_priority_dependency 与主线程优先级关系形成优先级反转候选。")

	if findings := proseWallClockConservationFindings(doc, bus, mut); len(findings) != 0 {
		t.Fatalf("trailing-designator anaphora must stay silent, got %+v", findings)
	}
}

// TestProseWallClockConservation_MainThreadDesignatorNeedsProof — 修复轮
// P3①: 主线程 is a process-role word. It binds the sole account subject
// ONLY when the evidence face proves tid==tgid; a non-main-thread target
// (CompThread with tgid 1864) must never absorb 主线程-worded claims, and
// an unproven subject stays unbound too.
func TestProseWallClockConservation_MainThreadDesignatorNeedsProof(t *testing.T) {
	tgidRecord := func(subject, tgid string) types.ObservationRecord {
		rec := psgTraceRecord("r-tgid-"+subject, "root_cause_rank:"+subject, "1.000")
		rec.Subject = subject
		rec.RichNotes = []string{types.TraceNoteKeyTGID + "=" + tgid}
		return rec
	}
	prose := "主线程在窗口内睡眠 300ms。"

	// (a) non-main-thread target: tgid 1864 ≠ tid 2955 → 主线程 unbound.
	acc := p6AccountRecord("CompThread_0-2955", 26.946, 3.636, 84.358, 0, 0, 114.940, 114.940)
	mut := psgTraceMutable(acc, tgidRecord("CompThread_0-2955", "1864"))
	if findings := proseWallClockConservationFindings(psgProseDoc(prose), psgBus(mut), mut); len(findings) != 0 {
		t.Fatalf("主线程 must not bind a non-main-thread target, got %+v", findings)
	}

	// (b) unproven subject (no tgid evidence): stays unbound too.
	mut = psgTraceMutable(p6TiebaAccount())
	if findings := proseWallClockConservationFindings(psgProseDoc(prose), psgBus(mut), mut); len(findings) != 0 {
		t.Fatalf("主线程 must not bind without the tid==tgid proof, got %+v", findings)
	}

	// (c) proven main thread (tgid 59566 == tid 59566): binds, and the
	// impossible 300ms sleep discloses.
	mut = psgTraceMutable(p6TiebaAccount(), tgidRecord("com.baidu.tieba-59566", "59566"))
	findings := proseWallClockConservationFindings(psgProseDoc(prose), psgBus(mut), mut)
	var windowForm bool
	for _, f := range findings {
		if strings.Contains(f.userReadable("zh"), "超过窗长") {
			windowForm = true
		}
	}
	if !windowForm {
		t.Fatalf("a proven main thread binds the 主线程 designator, got %+v", findings)
	}
}

// TestProseWallClockConservation_DesignatorGapBothDirections — 修复轮 P4:
// the 24-rune designator rule is directional AND bounded — a designator
// preceding the value beyond the gap never binds, and one following the
// value never binds however close (only the F-8 preceding-within-gap form
// binds; that positive form is pinned in the F8 test).
func TestProseWallClockConservation_DesignatorGapBothDirections(t *testing.T) {
	mut := psgTraceMutable(p6TiebaAccount())
	bus := psgBus(mut)
	// (a) preceding but beyond 24 runes.
	doc := psgProseDoc("目标线程在完成了非常多的其他准备步骤与全部回调注册工作之后才开始正式运行 400ms。")
	if findings := proseWallClockConservationFindings(doc, bus, mut); len(findings) != 0 {
		t.Fatalf("a designator beyond the gap must not bind, got %+v", findings)
	}
	// (b) following the value, however close.
	doc = psgProseDoc("窗口内睡眠 400ms 的正是目标线程。")
	if findings := proseWallClockConservationFindings(doc, bus, mut); len(findings) != 0 {
		t.Fatalf("a trailing designator must never bind, got %+v", findings)
	}
}

// TestProseWallClockConservation_CPUQueueTotalTrailingNameSilent — 二放
// 误报实证 (tieba 2026-07-12, verbatim): a CPU-queue runnable total
// (572.289ms, CPU-scoped) followed by per-thread contention values must
// not bind to the TRAILING thread name — a subject claims only values it
// precedes.
func TestProseWallClockConservation_CPUQueueTotalTrailingNameSilent(t *testing.T) {
	mut := psgTraceMutable(p6TiebaAccount())
	bus := psgBus(mut)
	doc := psgProseDoc("**CPU 压力** — CPU=0 runnable_wait=572.289ms，核心已被 sysevent_store-47924（28.162ms 竞争）、hilogd.pst-474（24.505ms）等 CFS 线程占满，主线程唤醒后需要等待可运行队列消化。")

	for _, f := range proseWallClockConservationFindings(doc, bus, mut) {
		if strings.Contains(f.userReadable("zh"), "572.289") {
			t.Fatalf("the CPU-queue total must not bind the trailing thread name: %+v", f)
		}
	}
}

// TestProseWallClockConservation_SingleClaimSingleLine — 二放冗余灭: a lone
// over-window claim discloses ONCE (the single-value arm); the Σ line only
// joins when it adds information beyond the already-disclosed components.
func TestProseWallClockConservation_SingleClaimSingleLine(t *testing.T) {
	mut := psgTraceMutable(p6TiebaAccount())
	bus := psgBus(mut)
	doc := psgProseDoc("com.baidu.tieba-59566 在窗口内睡眠 300ms。")

	findings := proseWallClockConservationFindings(doc, bus, mut)
	if len(findings) != 1 {
		t.Fatalf("one impossible claim, one disclosure line — got %d: %+v", len(findings), findings)
	}
	if !strings.Contains(findings[0].userReadable("zh"), "超过窗长") {
		t.Fatalf("the single line is the window form: %+v", findings)
	}
}
