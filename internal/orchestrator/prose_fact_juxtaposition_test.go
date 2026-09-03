package orchestrator

// prose_fact_juxtaposition_test.go — CR-4 修复轮方向改造 pins (用户裁定
// 2026-07-12): fact-juxtaposition witness coverage + FP-vector sentences.
// Every original CR-4 witness now pins a FACT LINE IN PLACE (typed truth the
// reader juxtaposes) instead of an accusatory verdict; the review's false-
// positive vector sentences pin zero verdicts (fact lines are allowed —
// under correct prose they are still true, hence harmless).

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// cr4FactRecords builds the donghu/tieba-shaped typed comparators.
func cr4FactRecords() []types.ObservationRecord {
	comp := psgTraceRecord("trace_query:t#root_cause_rank:1", "root_cause_primary", "36.757",
		"rank=1", "tier=primary", "chain_relevance=on_chain",
		types.TraceNoteKeyMemberCount+"=4",
		types.TraceNoteKeyBlockedReasonCaller+"=dma_fence_default_w",
		types.TraceNoteKeyTGID+"=1864")
	comp.Subject = "CompThread_0-2955"
	comp.Object = "d_state_or_io_wait"

	target := p6AccountRecord(".ugc.aweme.lite-17267", 157.248, 5.604, 70.338, 0, 0, 233.190, 233.190)
	targetIO := psgTraceRecord("trace_query:t#root_cause_rank:2", "root_cause_target_self_state", "1.354",
		"tier=target_self_state",
		types.TraceNoteKeyBlockedReasonWindowCount+"=6",
		types.TraceNoteKeyBlockedReasonWindowCaller+"=fscache_page_get_an/hmfs_read",
		types.TraceNoteKeyTGID+"=17267")
	targetIO.Subject = ".ugc.aweme.lite-17267"
	targetIO.Object = "io_wait"

	lock := psgTraceRecord("trace_query:t#critical_blocking:1", "critical_blocking:lock_contention", "",
		types.TraceNoteKeyBlockingKind+"=lock_contention",
		types.TraceNoteKeyOwnerTidRaw+"=62020",
		types.TraceNoteKeyWaiters+"=18")
	lock.Subject = "T7@ZeusThreadPo-61841"
	lock.Predicate = "critical_blocking"
	lock.Object = "lock_contention"

	load1 := psgTraceRecord("trace_query:t#thread_cpu_load:1", "thread_cpu_load:a", "12.4",
		"cpu=3", "freq=2189000kHz", "running=10.1ms")
	load1.Subject = "CookieMonsterCl-59843"
	load1.Predicate = "thread_cpu_load"
	load2 := psgTraceRecord("trace_query:t#thread_cpu_load:2", "thread_cpu_load:b", "8.0",
		"cpu=3", "freq=807000kHz")
	load2.Subject = "NetworkService-60595"
	load2.Predicate = "thread_cpu_load"

	return []types.ObservationRecord{comp, target, targetIO, lock, load1, load2}
}

// TestCR4Fact_WaitingObjectLine — F-CR3-3 witness (verbatim prose): the
// prose mentions CompThread + fscache_page_get_an; the fact line carries the
// thread's typed waiting-object account for the reader to juxtapose.
func TestCR4Fact_WaitingObjectLine(t *testing.T) {
	mut := psgTraceMutable(cr4FactRecords()...)
	bus := psgBus(mut)
	doc := psgProseDoc("CompThread_0-2955 的 D 态阻塞归因于 fscache_page_get_an 操作，建议优先优化文件页缓存。")

	facts := proseFactJuxtapositionFindings(doc, bus, mut)
	var hit string
	for _, f := range facts {
		zh := f.userReadable("zh")
		if strings.Contains(zh, "CompThread_0-2955") && strings.Contains(zh, "内核等待调用点=dma_fence_default_w") {
			hit = zh
		}
	}
	if hit == "" {
		t.Fatalf("the waiting-object fact line must render, got %+v", facts)
	}
	if !strings.Contains(hit, "根因排序=#1") || !strings.Contains(hit, "成员共4") {
		t.Fatalf("the seat + member-count facts ride the same line: %s", hit)
	}
	cr4BannedWordingCheck(t, []string{hit})
}

// TestProseFactThreadLineUsesReaderVocabulary pins the bilingual display
// mapping at its single renderer. Internal typed-protocol names remain in
// the ledger, but the user-facing appendix describes their meaning.
func TestProseFactThreadLineUsesReaderVocabulary(t *testing.T) {
	ranked := &proseFactThreadFacts{
		subject: "worker-200",
		seats: []proseFactSeat{{
			rank: 1, effectiveMS: 8.25, hasEff: true, memberCount: 2,
		}},
		callers:     []string{"fscache_page_wait_on_page_bit"},
		callerCount: 3,
		lockWaiter:  true,
		ownerTIDs:   []string{"300"},
	}
	zh, en := proseFactThreadLine(ranked)
	for _, want := range []string{"事实对照：worker-200", "根因排序=#1", "内核等待调用点=", "锁等待角色=等待侧"} {
		if !strings.Contains(zh, want) {
			t.Fatalf("zh reader vocabulary missing %q: %s", want, zh)
		}
	}
	for _, want := range []string{"Evidence reference: worker-200", "root-cause rank(s)=#1", "kernel wait call-site=", "lock-wait role=waiter side"} {
		if !strings.Contains(en, want) {
			t.Fatalf("en reader vocabulary missing %q: %s", want, en)
		}
	}

	confidenceZH, confidenceEN := proseFactThreadLine(&proseFactThreadFacts{
		subject:     "worker-201",
		confidences: []float64{0.91},
	})
	if !strings.Contains(confidenceZH, "证据置信度=0.91") || !strings.Contains(confidenceEN, "evidence confidence=0.91") {
		t.Fatalf("confidence must use reader vocabulary: zh=%s en=%s", confidenceZH, confidenceEN)
	}
	for _, surface := range []string{zh, en, confidenceZH, confidenceEN} {
		for _, banned := range []string{"typed 事实", "typed fact", "typed 席位", "typed seat", "typed 内核", "typed kernel", "typed 锁", "typed lock", "typed confidence"} {
			if strings.Contains(surface, banned) {
				t.Fatalf("reader surface leaks internal vocabulary %q: %s", banned, surface)
			}
		}
	}
}

// TestCR4Fact_LockRoleLine — F-CR3-1 witness (verbatim prose incl. the
// name-tid chimera): the fact line carries the thread's typed lock role and
// the owner tid.
func TestCR4Fact_LockRoleLine(t *testing.T) {
	mut := psgTraceMutable(cr4FactRecords()...)
	bus := psgBus(mut)
	doc := psgProseDoc("T7@ZeusThreadPo-61841（tid:62020）持有锁并产生锁争用，是优先级反转的持有侧。")

	facts := proseFactJuxtapositionFindings(doc, bus, mut)
	var hit string
	for _, f := range facts {
		zh := f.userReadable("zh")
		if strings.Contains(zh, "T7@ZeusThreadPo-61841") && strings.Contains(zh, "锁等待角色=等待侧") {
			hit = zh
		}
	}
	if hit == "" {
		t.Fatalf("the lock-role fact line must render, got %+v", facts)
	}
	if !strings.Contains(hit, "锁主 tid=62020") {
		t.Fatalf("the owner tid fact must ride the line: %s", hit)
	}
}

// TestCR4Fact_CPUFrequencyLines — F-CR3-2 witness + C-3 k=v word form: the
// prose names CPU0 with a frequency token; the fact line states CPU0 has no
// in-window typed frequency observation, and a real-point CPU renders its
// typed points.
func TestCR4Fact_CPUFrequencyLines(t *testing.T) {
	mut := psgTraceMutable(cr4FactRecords()...)
	bus := psgBus(mut)
	doc := psgProseDoc("CPU0 运行频率固定在 1090MHz（省电模式），限制了线程执行速度。CPU3 有实测频点。")

	facts := proseFactJuxtapositionFindings(doc, bus, mut)
	var cpu0, cpu3 string
	for _, f := range facts {
		zh := f.userReadable("zh")
		if strings.Contains(zh, "CPU0") {
			cpu0 = zh
		}
		if strings.Contains(zh, "CPU3") {
			cpu3 = zh
		}
	}
	if !strings.Contains(cpu0, "无窗内频率观测记录") {
		t.Fatalf("CPU0's absent-observation fact must render, got %+v", facts)
	}
	if !strings.Contains(cpu3, "807MHz") || !strings.Contains(cpu3, "2189MHz") {
		t.Fatalf("CPU3's typed points must render: %q (%+v)", cpu3, facts)
	}

	// C-3 word form: the bare k=v spelling (「CPU0(freq=1090000 kHz)」) also
	// counts as a frequency token.
	mut2 := psgTraceMutable(cr4FactRecords()...)
	doc2 := psgProseDoc("主线程在 CPU0(freq=1090000 kHz) 上等待调度。")
	found := false
	for _, f := range proseFactJuxtapositionFindings(doc2, psgBus(mut2), mut2) {
		if strings.Contains(f.userReadable("zh"), "CPU0") && strings.Contains(f.userReadable("zh"), "无窗内频率观测记录") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the k=v frequency form must trigger the CPU fact line")
	}
}

// TestCR4Fact_FourStateAccountLine — SMR-1 P0 witness: the target thread's
// name in prose triggers its full four-state account fact line.
func TestCR4Fact_FourStateAccountLine(t *testing.T) {
	mut := psgTraceMutable(cr4FactRecords()...)
	bus := psgBus(mut)
	doc := psgProseDoc("ugc.aweme.lite-17267 在窗口（233.19ms）内全程处于 s_sleep 等待,未发生 runnable/running 片段。")

	facts := proseFactJuxtapositionFindings(doc, bus, mut)
	var hit string
	for _, f := range facts {
		zh := f.userReadable("zh")
		if strings.Contains(zh, "17267") && strings.Contains(zh, "窗内五态") {
			hit = zh
		}
	}
	if hit == "" {
		t.Fatalf("the four-state fact line must render, got %+v", facts)
	}
	if !strings.Contains(hit, "running 157.248") || !strings.Contains(hit, "sleep 70.338") {
		t.Fatalf("the account values must ride the line: %s", hit)
	}
	if !strings.Contains(hit, tool.TraceStateNonIODStateWord(true)) {
		t.Fatalf("exclusive five-state D lane must disclose non-IO semantics via the single-source word: %s", hit)
	}
}

func TestCR4Fact_BoundedRelationFamiliesDoNotTriggerStateJuxtaposition(t *testing.T) {
	mut := psgTraceMutable(cr4FactRecords()...)
	bus := psgBus(mut)
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:        types.IntentExplain,
		Scenario:      types.ScenarioGeneric,
		AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
		RuntimeTargets: []types.RuntimeTarget{{
			Kind: types.RuntimeTargetKindThread, PID: 17267, Thread: ".ugc.aweme.lite-17267",
		}},
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{
			Scope: types.RuntimeQuestionScopeBoundedFactSet,
			FactFamilies: []types.RuntimeQuestionFactFamily{
				types.RuntimeQuestionFactRelationPeer,
				types.RuntimeQuestionFactTransactionID,
				types.RuntimeQuestionFactDirectWaker,
			},
		},
	}}
	doc := psgProseDoc("ugc.aweme.lite-17267 的 peer、transaction 和直接唤醒者见正文；窗口内 sleep 70.338ms、running 157.248ms。")
	for _, finding := range proseFactJuxtapositionFindings(doc, bus, mut) {
		if strings.Contains(finding.userReadable("zh"), "窗内五态") {
			t.Fatalf("bounded relation facts must not trigger an unrequested state appendix: %+v", finding)
		}
	}

	bus.AnalysisIR.RequestModel.RuntimeQuestionProfile.FactFamilies = append(
		bus.AnalysisIR.RequestModel.RuntimeQuestionProfile.FactFamilies,
		types.RuntimeQuestionFactTargetSchedulerState,
	)
	found := false
	for _, finding := range proseFactJuxtapositionFindings(doc, bus, mut) {
		if strings.Contains(finding.userReadable("zh"), "窗内五态") {
			found = true
		}
	}
	if !found {
		t.Fatal("an explicitly requested target-state family must retain its typed juxtaposition")
	}
}

// TestCR4Fact_EquationVerdict — C-2 假等式臂 (witness verbatim:
// 15.758+5.395=20.816, actual 21.153): the ONLY verdict wording, pure
// arithmetic; a correct equation and an approx form stay silent.
func TestCR4Fact_EquationVerdict(t *testing.T) {
	mut := psgTraceMutable(cr4FactRecords()...)
	bus := psgBus(mut)
	doc := psgProseDoc("链路1 合计 15.758ms + 5.395ms = 20.816ms,其余链路不变。")

	facts := proseFactJuxtapositionFindings(doc, bus, mut)
	var hit string
	for _, f := range facts {
		zh := f.userReadable("zh")
		if strings.Contains(zh, "文中等式") {
			hit = zh
		}
	}
	if hit == "" {
		t.Fatalf("the false equation must draw the arithmetic verdict, got %+v", facts)
	}
	if !strings.Contains(hit, "15.758+5.395=20.816") || !strings.Contains(hit, "实际和为 21.153") {
		t.Fatalf("the verdict must restate the equation and the actual sum: %s", hit)
	}

	// Correct arithmetic → silent; approx (≈) forms → silent.
	mut2 := psgTraceMutable(cr4FactRecords()...)
	doc2 := psgProseDoc("合计 5.251ms + 15.565ms = 20.816ms;另有 15.758 + 5.395 ≈ 21.2。")
	for _, f := range proseFactJuxtapositionFindings(doc2, psgBus(mut2), mut2) {
		if strings.Contains(f.userReadable("zh"), "文中等式") {
			t.Fatalf("correct/approx equations must stay silent: %s", f.userReadable("zh"))
		}
	}
}

// TestCR4Fact_WakeupTargetCPUDegradation — 引擎 typed 退化事实: the banner
// marker + a prose CPU token render the degradation fact line.
func TestCR4Fact_WakeupTargetCPUDegradation(t *testing.T) {
	degraded := psgTraceRecord("trace_query:t#wakeup_target_cpu_integrity", "wakeup_target_cpu_integrity:1.000000..2.000000", "1697",
		types.TraceNoteKeyWakeupTargetCPUIntegrityStatus+"=suspected_degraded_all_zero",
		types.TraceNoteKeyWakeupTargetCPUObservedCount+"=1697",
		types.TraceNoteKeyWakeupTargetCPUZeroCount+"=1697",
		types.TraceNoteKeyWakeupTargetCPUEmitterCPUCount+"=6")
	degraded.Subject = "sched_wakeup.target_cpu"
	degraded.Predicate = "wakeup_target_cpu_integrity"
	degraded.Object = "suspected_degraded_all_zero"
	degraded.Span = types.ObservationSpan{StartTs: 1, EndTs: 2}
	edge := psgTraceRecord("trace_query:t#wakeup_chain_edge:1", "wakeup_chain_edge:worker-1:app-2", "",
		types.TraceNoteKeyWakeupTs+"=1.500000",
		types.TraceNoteKeyWakeupWakerCPU+"=3",
		types.TraceNoteKeyWakeupWakeeTargetCPU+"=0",
		types.TraceNoteKeyWakeupCPURelation+"=cross_cpu")
	edge.Subject = "worker-1"
	edge.Predicate = "wakeup_chain_edge"
	edge.Object = "app-2"
	edge.Span = types.ObservationSpan{StartTs: 1.5, EndTs: 1.5}
	records := append(cr4FactRecords(), degraded, edge)
	mut := psgTraceMutable(records...)
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		Observations: records,
		Summary:      "- caveat=wakeup_target_cpu_degraded=true total=1697 — in-window sched_wakeup target_cpu all zero",
	}}})
	bus := psgBus(mut)
	doc := psgProseDoc("多线程在 CPU0 上竞争调度,频率受限。")

	facts := proseFactJuxtapositionFindings(doc, bus, mut)
	var hit string
	for _, f := range facts {
		zh := f.userReadable("zh")
		if strings.Contains(zh, "target_cpu 字段疑退化") {
			hit = zh
		}
	}
	if hit == "" {
		t.Fatalf("the degradation fact must render, got %+v", facts)
	}
	if !strings.Contains(hit, "1697 条全 0") {
		t.Fatalf("the degradation fact must carry the typed count: %s", hit)
	}
	for _, fact := range facts {
		if strings.Contains(fact.userReadable("zh"), "唤醒拓扑 worker-1") {
			t.Fatalf("a degraded target_cpu row must not simultaneously publish exact cross-CPU authority: %+v", facts)
		}
	}
}

// TestCR4Fact_FPVectorSentencesDrawNoVerdicts — the review's false-positive
// vector sentences (否定纠错/被动/同进程否定/祈使/假设/hedge 后缀/范围句/
// 正确名-tid 对/正确等式/正确归因): under the fact design NONE of them can
// draw a verdict — the appendix carries only typed fact lines (true under
// correct prose, hence harmless) and zero accusatory wording.
func TestCR4Fact_FPVectorSentencesDrawNoVerdicts(t *testing.T) {
	vectors := []string{
		"CompThread_0-2955 的 D 态阻塞并非归因于 fscache_page_get_an,而是 dma_fence_default_w。",
		"T7@ZeusThreadPo-61841 由锁持有者 tid 62020 阻塞,并未持有锁。",
		"JankManager-9655 与目标线程分属不同进程组。",
		"建议评估 CompThread_0-2955 的内核调用点 fscache_page_get_an。",
		"如果 ugc.aweme.lite-17267 全程处于 s_sleep,则应检查唤醒链。",
		"ugc.aweme.lite-17267 基本上全程处于繁忙状态。",
		"CPU 3-5 运行于 1880MHz 的说法需按各簇分别核实。",
		"T7@ZeusThreadPo-61841（tid:61841）在等待锁,锁主 tid 为 62020。",
		"合计 5.251ms + 15.565ms = 20.816ms。",
		"ugc.aweme.lite-17267 的 IO 等待归因于 fscache_page_get_an 等文件页缓存操作。",
	}
	for i, sentence := range vectors {
		mut := psgTraceMutable(cr4FactRecords()...)
		bus := psgBus(mut)
		doc := psgProseDoc(sentence)
		lines := cr4AllAppendixLines(t, doc, bus, mut)
		cr4BannedWordingCheck(t, lines)
		for _, line := range lines {
			if strings.Contains(line, "文中等式") {
				t.Fatalf("vector %d must not draw an arithmetic verdict: %s", i+1, line)
			}
		}
		// Every fact-lane line is a typed fact line (fact prefix), never a
		// judgment sentence.
		for _, f := range proseFactJuxtapositionFindings(doc, bus, mut) {
			zh := f.userReadable("zh")
			if !strings.HasPrefix(zh, "事实对照：") && !strings.HasPrefix(zh, "文中等式") {
				t.Fatalf("vector %d: fact lane emitted a non-fact line: %s", i+1, zh)
			}
		}
	}
}

// TestCR4Fact_NonTraceRunsInert — scope gate: no deterministic runtime
// observation → the fact lane stays inert.
func TestCR4Fact_NonTraceRunsInert(t *testing.T) {
	mut := psgTraceMutable()
	bus := psgBus(mut)
	doc := psgProseDoc("CompThread_0-2955 在 CPU0 运行于 1090MHz。")
	if facts := proseFactJuxtapositionFindings(doc, bus, mut); len(facts) != 0 {
		t.Fatalf("non-trace runs must stay inert, got %+v", facts)
	}
}

// TestCR4Fact_SeatRepublicationCollapses — 复放实证 (tieba 34072): the same
// board row can reach the ledger through several result channels; identical
// (rank, effective, members) republications collapse to one seat entry,
// while distinct-value twins (two query windows' boards) both stay.
func TestCR4Fact_SeatRepublicationCollapses(t *testing.T) {
	seat := func(id string, eff string) types.ObservationRecord {
		rec := psgTraceRecord(id, "root_cause_rank_4", eff, "rank=4", "effective_impact_ms="+eff)
		rec.Subject = "ThreadPoolForeg-60555"
		rec.Object = "io_wait"
		return rec
	}
	mut := psgTraceMutable(
		seat("trace_query:t#root_cause_rank:1", "17.819"),
		seat("trace_query:t#root_cause_rank:2", "17.819"), // identical republication
		seat("trace_query:t#root_cause_rank:3", "18.221"), // distinct-window twin
	)
	bus := psgBus(mut)
	doc := psgProseDoc("ThreadPoolForeg-60555 的 IO 等待是链源。")
	facts := proseFactJuxtapositionFindings(doc, bus, mut)
	var line string
	for _, f := range facts {
		zh := f.userReadable("zh")
		if strings.Contains(zh, "ThreadPoolForeg-60555") {
			line = zh
		}
	}
	if line == "" {
		t.Fatalf("seat fact line must render, got %+v", facts)
	}
	if strings.Count(line, "#4(有效归因 17.819ms)") != 1 {
		t.Fatalf("identical seat republications must collapse to one entry: %s", line)
	}
	if !strings.Contains(line, "#4(有效归因 18.221ms)") {
		t.Fatalf("the distinct-window twin seat must stay: %s", line)
	}
}

// cr4SubtractionRecords — the §29.78 tieba shape: the cause-unproven
// remainder seat (10.433) plus the caller-named proven shares, published
// both as a caller seat value (7.386) and as census Σ magnitudes (0.145 /
// 0.171 — the hmfs shares the witness prose nested inside the remainder).
func cr4SubtractionRecords() []types.ObservationRecord {
	recs := cr4FactRecords()
	remainder := psgTraceRecord("trace_query:t#root_cause_rank:3", "root_cause_tertiary", "10.433",
		"rank=3", "effective_impact_ms=10.433",
		types.TraceNoteKeyMemberCount+"=3",
		types.TraceNoteKeyDStateCauseUnprovenRemainder+"=true")
	remainder.Subject = "ThreadPoolForeg-60555"
	remainder.Object = "io_wait"
	proven := psgTraceRecord("trace_query:t#root_cause_rank:4", "root_cause_secondary", "7.386",
		"rank=4", "effective_impact_ms=7.386",
		types.TraceNoteKeyBlockedReasonCaller+"=fscache_page_wait_o")
	proven.Subject = "ThreadPoolForeg-60555"
	proven.Object = "io_wait"
	census := psgTraceRecord("trace_query:t#blocked_reason_census:1", "blocked_reason_census", "19",
		types.TraceNoteKeyBlockedReasonCensus+"=fscache_page_wait_o×17(Σ7.386ms)/hmfs_read×1(Σ0.145ms)/hmfs_get_dnode×1(Σ0.171ms)")
	census.Subject = "ThreadPoolForeg-60555"
	census.Predicate = "blocked_reason_census"
	census.Object = "blocked_reason"
	return append(recs, remainder, proven, census)
}

// TestCR4Fact_ImplicitNestedResubtraction — PROSE-RC-4 臂② (§29.78): the
// witness carried NO "=" equation — the remainder seat was framed as a
// TOTAL containing the hmfs caller shares and 10.117 = 10.433−0.145−0.171
// was minted as the "real" unproven amount (with a wrong self-summed 0.296
// alongside). The three-value co-occurrence with the pure arithmetic
// identity and the unpublished residual draws the fact-only line.
func TestCR4Fact_ImplicitNestedResubtraction(t *testing.T) {
	mut := psgTraceMutable(cr4SubtractionRecords()...)
	bus := psgBus(mut)
	doc := psgProseDoc("D 状态总量 10.433ms 中,已获内核 blocked_reason 记录证实的部分仅 0.296ms(hmfs_read 的 0.145ms + hmfs_get_dnode 的 0.171ms),剩余约 10.117ms 的 D 状态段无对应内核等待原因记录。")

	facts := proseFactJuxtapositionFindings(doc, bus, mut)
	var hit string
	for _, f := range facts {
		if zh := f.userReadable("zh"); strings.Contains(zh, "系统事实对照") {
			hit = zh
		}
	}
	if hit == "" {
		t.Fatalf("the implicit nested-re-subtraction form must draw the fact line, got %+v", facts)
	}
	for _, want := range []string{
		// The net-of account property, the arithmetic identity over the
		// prose's own verbatim tokens, and the negative set-membership fact.
		"10.433 为已扣除全部已证原因份额后的净值",
		"各已证份额均在其外",
		"文中共现数值满足 10.433-0.145-0.171≈10.117",
		"10.117 非本次运行的任何 typed 发布值",
	} {
		if !strings.Contains(hit, want) {
			t.Fatalf("implicit-subtraction fact line missing %q: %s", want, hit)
		}
	}
	// The wrong self-summed 0.296 must never be picked as the residual (it
	// satisfies no subtraction identity), and no accusatory wording appears.
	if strings.Contains(hit, "0.296") {
		t.Fatalf("the non-identity residual must not be picked: %s", hit)
	}
	cr4BannedWordingCheck(t, []string{hit})
}

// TestCR4Fact_ImplicitSubtractionNegatives — the precise-gate boundary
// (§29.78 边界谨慎: the false-positive surface is legitimate multi-value
// citation): published values, rounded re-quotes of published values, and
// co-occurrence WITHOUT the arithmetic identity all stay silent; runs
// without a remainder or without proven shares are structurally inert.
func TestCR4Fact_ImplicitSubtractionNegatives(t *testing.T) {
	silent := func(t *testing.T, prose string, records []types.ObservationRecord) {
		t.Helper()
		mut := psgTraceMutable(records...)
		bus := psgBus(mut)
		doc := psgProseDoc(prose)
		for _, f := range proseFactJuxtapositionFindings(doc, bus, mut) {
			if strings.Contains(f.userReadable("zh"), "系统事实对照") {
				t.Fatalf("legitimate citation must stay silent: %s", f.userReadable("zh"))
			}
		}
	}
	// Legitimate multi-value citation: every number restates a published
	// value (10.43 is an honest rounded re-quote of 10.433 — half-ulp gate).
	silent(t, "原因未证余数约 10.43ms;其外的已证份额包括 fscache_page_wait_o 7.386ms、hmfs_read 0.145ms 与 hmfs_get_dnode 0.171ms。", cr4SubtractionRecords())
	// Co-occurrence without the arithmetic identity: 9.850 matches no
	// remainder-minus-shares subset — token presence alone never fires.
	silent(t, "余数 10.433ms 中已证 0.145ms 与 0.171ms 之外,还有约 9.850ms 待确认。", cr4SubtractionRecords())
	// A percent form never enters the operand lane — even one that would
	// satisfy the identity as a bare number.
	silent(t, "余数 10.433ms;已证 0.145ms 与 0.171ms;某处占比记为 10.117%。", cr4SubtractionRecords())
	// The residual gate reads the published-value set: once 10.117 itself is
	// published on the evidence face, the witness sentence stays silent.
	published := psgTraceRecord("trace_query:t#state_drilldown:1", "state_drilldown:x", "10.117")
	published.Subject = "ThreadPoolForeg-60555"
	silent(t, "D 状态总量 10.433ms 中,已证部分为 0.145ms 与 0.171ms,剩余约 10.117ms。", append(cr4SubtractionRecords(), published))
	// No cause-unproven remainder on the evidence face → structurally inert
	// even with an unpublished residual in the prose.
	silent(t, "总量 10.433ms 减去 0.145ms 与 0.171ms 后余 10.117ms。", cr4FactRecords())
}
