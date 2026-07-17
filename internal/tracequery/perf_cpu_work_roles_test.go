package tracequery

import "testing"

func TestPerfCPUWorkThreadRolesRequireExecutionSamples(t *testing.T) {
	intern := newStringInterner()
	lines := []string{
		`target-10 (10) [000] .... 3.001000: perf_sample: cpu=0 cpu_known=true pid=10 tid=10 thread_comm=target sample_weight=10 event=cpu-cycles symbol=TargetOn source=fixture sample_kind=on_cpu`,
		`target-10 (10) [000] .... 3.002000: perf_sample: cpu=0 cpu_known=true pid=10 tid=10 thread_comm=target sample_weight=100 event=cpu-clock symbol=TargetOff source=fixture sample_kind=off_cpu`,
		`target-10 (10) [000] .... 3.003000: perf_sample: cpu=0 cpu_known=true pid=10 tid=10 thread_comm=target sample_weight=90 event=cpu-cycles symbol=TargetUnknown source=fixture sample_kind=unknown`,
		`target-10 (10) [000] .... 3.004000: perf_sample: cpu=0 cpu_known=false pid=10 tid=10 thread_comm=target sample_weight=80 event=cpu-cycles symbol=TargetCPUUnknown source=fixture sample_kind=on_cpu`,
		`competitor-20 (20) [001] .... 3.011000: perf_sample: cpu=1 cpu_known=true pid=20 tid=20 thread_comm=competitor sample_weight=11 event=cpu-cycles symbol=CompetitorOn source=fixture sample_kind=on_cpu`,
		`competitor-20 (20) [001] .... 3.012000: perf_sample: cpu=1 cpu_known=true pid=20 tid=20 thread_comm=competitor sample_weight=110 event=cpu-clock symbol=CompetitorOff source=fixture sample_kind=off_cpu`,
		`competitor-20 (20) [001] .... 3.013000: perf_sample: cpu=1 cpu_known=true pid=20 tid=20 thread_comm=competitor sample_weight=99 event=cpu-cycles symbol=CompetitorUnknown source=fixture sample_kind=unknown`,
		`competitor-20 (20) [001] .... 3.014000: perf_sample: cpu=1 cpu_known=false pid=20 tid=20 thread_comm=competitor sample_weight=88 event=cpu-cycles symbol=CompetitorCPUUnknown source=fixture sample_kind=on_cpu`,
		`dependency-30 (30) [002] .... 3.020000: perf_sample: cpu=2 cpu_known=true pid=30 tid=30 thread_comm=dependency sample_weight=120 event=cpu-clock symbol=DependencyOff source=fixture sample_kind=off_cpu`,
	}
	events := make([]Event, 0, len(lines))
	for i, line := range lines {
		ev, ok := ParseLine(i+1, line, intern)
		if !ok || ev.PerfFields == nil {
			t.Fatalf("fixture line %d did not parse", i+1)
		}
		events = append(events, ev)
	}
	idx := &Index{Events: events}
	q := Query{TimeStart: 3, TimeEnd: 3.1}
	target := ThreadRef{Comm: "target", PID: 10, TGID: 10}
	competitor := ThreadRef{Comm: "competitor", PID: 20, TGID: 20}
	dependency := ThreadRef{Comm: "dependency", PID: 30, TGID: 30}

	assertSinglePerfSymbol(t, perfContextForExecutionThread(idx, q, target, 3, 3.1, 8), "TargetOn")
	assertSinglePerfSymbol(t, perfContextForExecutionThread(idx, q, competitor, 3, 3.1, 8), "CompetitorOn")
	if generic := perfContextForThread(idx, q, target, 3, 3.1, 8); generic == nil || generic.SampleCount != 4 || generic.TopSymbols[0].Symbol != "TargetOff" {
		t.Fatalf("generic candidate/dependency inventory was incorrectly narrowed: %+v", generic)
	}
	if generic := perfContextForThreads(idx, q, map[int]ThreadRef{30: dependency}, 8); generic == nil || generic.SampleCount != 1 || generic.TopSymbols[0].Symbol != "DependencyOff" {
		t.Fatalf("generic on-chain/binder inventory lost off-CPU support: %+v", generic)
	}
	mixedThreads := map[int]ThreadRef{10: target, 20: competitor}
	if execution := perfContextForExecutionThreads(idx, q, mixedThreads, 8); execution == nil || execution.SampleCount != 2 || perfContextHasSymbol(execution, "TargetOff") || perfContextHasSymbol(execution, "CompetitorOff") || !perfContextHasSymbol(execution, "TargetOn") || !perfContextHasSymbol(execution, "CompetitorOn") {
		t.Fatalf("multi-thread execution context admitted non-execution samples: %+v", execution)
	}
	if generic := perfContextForThreads(idx, q, mixedThreads, 8); generic == nil || generic.SampleCount != 8 || !perfContextHasSymbol(generic, "TargetOff") || !perfContextHasSymbol(generic, "CompetitorUnknown") {
		t.Fatalf("multi-thread generic inventory was incorrectly narrowed: %+v", generic)
	}

	window := TimeWindow{StartTs: 3, EndTs: 3.1}
	targetRoles := appendRootCauseStatsPerfContexts(idx, q, WindowStats{}, RootCauseRankItem{Type: "running", Thread: target}, window, nil)
	assertPerfRoleSingleSymbol(t, targetRoles, "target_running", "TargetOn")

	competitorDuration := []ThreadDuration{{Thread: competitor, CPU: 1, StartTs: 3, EndTs: 3.1, DurationMs: 100}}
	for _, role := range []string{"same_cpu_competitor", "cpu_pressure_top_running"} {
		roles := appendRootCauseRunnableCompetitorPerfContexts(idx, q, competitorDuration, ThreadRef{}, window, role, "test CPU-work role", nil)
		assertPerfRoleSingleSymbol(t, roles, role, "CompetitorOn")
	}

	// Force the frame competitor's CPU-first lookup to miss (CPU9), so this
	// exercises the thread fallback specifically. Both target and competitor
	// fallbacks must still stay execution-only.
	stats := WindowStats{RunnableContext: []RunnableContextSummary{{
		Thread: target, RunnableWaitMs: 1, CPU: 9,
		SameCPUTopRunning: competitorDuration,
	}}}
	stats.PerfSamples = computePerfContext(idx, q, 8)
	frame := buildFramePerfContexts(idx, q, stats, nil, CriticalBlockingResult{}, target)
	assertSinglePerfSymbol(t, frame.TargetRunningPerf, "TargetOn")
	assertSinglePerfSymbol(t, frame.SameCPUCompetitorPerf, "CompetitorOn")
}

func assertSinglePerfSymbol(t *testing.T, ctx *PerfContext, want string) {
	t.Helper()
	if ctx == nil || ctx.SampleCount != 1 || len(ctx.TopSymbols) != 1 || ctx.TopSymbols[0].Symbol != want {
		t.Fatalf("execution context = %+v, want only %s", ctx, want)
	}
}

func assertPerfRoleSingleSymbol(t *testing.T, contexts []RootCausePerfRoleContext, role, want string) {
	t.Helper()
	for _, ctx := range contexts {
		if ctx.Role == role {
			assertSinglePerfSymbol(t, ctx.PerfContext, want)
			return
		}
	}
	t.Fatalf("role %q missing from %+v", role, contexts)
}

func perfContextHasSymbol(ctx *PerfContext, want string) bool {
	if ctx == nil {
		return false
	}
	for _, item := range ctx.TopSymbols {
		if item.Symbol == want {
			return true
		}
	}
	return false
}
