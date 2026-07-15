package tracequery

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

func schedulerWakeLine(rawType, fields string) string {
	return "waker-200 (200) [002] .... 5.001000: " + rawType + ": " + fields
}

func TestStrictWakeupProfilesPreserveCanonicalProducerRows(t *testing.T) {
	interner := newStringInterner()
	for _, tc := range []struct {
		rawType string
		typ     EventType
	}{
		{rawType: "sched_wakeup", typ: EventSchedWakeup},
		{rawType: "sched_wakeup_new", typ: EventSchedWakeup},
		{rawType: "sched_waking", typ: EventSchedWaking},
	} {
		t.Run(tc.rawType, func(t *testing.T) {
			ev, ok := ParseLine(1, schedulerWakeLine(tc.rawType,
				"comm=effect thread foo_pid=77 xpid=88 pid:99 pid=100 prio=159 target_cpu=4095"), interner)
			if !ok {
				t.Fatal("canonical wake row was rejected")
			}
			if ev.Type != tc.typ || ev.Name != tc.rawType || ev.WakeeComm != "effect thread foo_pid=77 xpid=88 pid:99" || ev.WakeePID != 100 {
				t.Fatalf("strict wake profile changed identity/display fields: %+v", ev)
			}
			if eventWakeePriorityForHardUse(ev) != 159 || ev.WakeePrioritySource() != "" {
				t.Fatalf("exact Harmony/Donghu RT priority lost authority: %+v", ev)
			}
			if cpu, valid := eventTargetCPU(ev); !valid || cpu != 4095 || ev.CPUInputInvalid {
				t.Fatalf("exact target CPU boundary was degraded: %+v", ev)
			}
		})
	}

	zeroCPU, ok := ParseLine(2, schedulerWakeLine("sched_wakeup",
		"comm=app pid=100 prio=140 target_cpu=000"), interner)
	if !ok || eventWakeePriorityForHardUse(zeroCPU) != 140 {
		t.Fatalf("priority 140 must remain exact Harmony RT: ok=%v event=%+v", ok, zeroCPU)
	}
	if cpu, valid := eventTargetCPU(zeroCPU); !valid || cpu != 0 {
		t.Fatalf("canonical zero target CPU lost authority: %+v", zeroCPU)
	}
}

func TestStrictWakeupPIDIsUniquePositiveCanonicalIdentity(t *testing.T) {
	interner := newStringInterner()
	for _, fields := range []string{
		"comm=app pid=100 pid=100 prio=20 target_cpu=001",
		"comm=app pid=999 pid=100 prio=20 target_cpu=001",
		"comm=app pid =100 prio=20 target_cpu=001",
		"comm=app pid=0 prio=20 target_cpu=001",
		"comm=app pid=-1 prio=20 target_cpu=001",
		"comm=app pid=0100 prio=20 target_cpu=001",
		"comm=app pid=2147483648 prio=20 target_cpu=001",
		// The unquoted producer wire cannot distinguish this declaration in
		// comm from a duplicate core identity. It must fail closed, not elect
		// the last occurrence.
		"comm=label pid=777 worker pid=100 prio=20 target_cpu=001",
	} {
		line := schedulerWakeLine("sched_wakeup", fields)
		if ev, ok := ParseLine(1, line, interner); ok {
			t.Fatalf("ambiguous/invalid wake PID minted an edge: fields=%q event=%+v", fields, ev)
		}
		failure := schedulerRowValidationFailure(1, line)
		if failure == nil || !containsSubstring(failure.Fields, "pid") ||
			(!failure.AffectsAllPIDs && len(failure.PIDs) == 0) {
			t.Fatalf("bad wake PID needs a typed global-or-candidate witness: fields=%q failure=%+v", fields, failure)
		}
	}
}

func TestDuplicateSchedulerPIDPoisonUsesBoundedCandidateUnion(t *testing.T) {
	fieldsFor := func(count int) string {
		parts := []string{"comm=app"}
		for pid := 1; pid <= count; pid++ {
			parts = append(parts, fmt.Sprintf("pid=%d", pid))
		}
		return strings.Join(append(parts, "prio=20", "target_cpu=001"), " ")
	}

	scoped := schedulerRowValidationFailure(1, schedulerWakeLine("sched_wakeup", fieldsFor(32)))
	if scoped == nil || scoped.AffectsAllPIDs || len(scoped.PIDs) != 32 || scoped.PIDs[0] != 1 || scoped.PIDs[31] != 32 {
		t.Fatalf("32 exact duplicate candidates did not retain sorted union scope: %+v", scoped)
	}
	if !schedulerRowIntegrityFailureRelevantToQuery(scoped, Query{}, 17) ||
		schedulerRowIntegrityFailureRelevantToQuery(scoped, Query{}, 999) {
		t.Fatalf("candidate union contaminated unrelated TID: %+v", scoped)
	}

	global := schedulerRowValidationFailure(1, schedulerWakeLine("sched_wakeup", fieldsFor(33)))
	if global == nil || !global.AffectsAllPIDs || len(global.PIDs) != 0 {
		t.Fatalf("candidate cap did not fail closed globally: %+v", global)
	}
	for _, fields := range []string{
		"comm=app pid=100 pid=bad prio=20 target_cpu=001",
		"comm=app pid=100 pid =999 prio=20 target_cpu=001",
	} {
		failure := schedulerRowValidationFailure(1, schedulerWakeLine("sched_wakeup", fields))
		if failure == nil || !failure.AffectsAllPIDs || len(failure.PIDs) != 0 {
			t.Fatalf("non-exact duplicate candidate was unsafely scoped: fields=%q failure=%+v", fields, failure)
		}
	}
	same := schedulerRowValidationFailure(1,
		schedulerWakeLine("sched_wakeup", "comm=app pid=100 pid=100 prio=20 target_cpu=001"))
	if same == nil || same.AffectsAllPIDs || len(same.PIDs) != 1 || same.PIDs[0] != 100 {
		t.Fatalf("same-value duplicate lost precise candidate scope: %+v", same)
	}
}

func TestScopedDuplicatePIDPreservesUnrelatedWindowHeadParity(t *testing.T) {
	lines := []string{
		`idle-0 (0) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=other next_pid=999 next_prio=20`,
		`other-999 (999) [000] .... 1.100000: sched_switch: prev_comm=other prev_pid=999 prev_prio=20 prev_state=R ==> next_comm=idle/0 next_pid=0 next_prio=120`,
		`waker-7 (7) [001] .... 1.200000: sched_wakeup: comm=bad pid=100 pid=101 prio=20 target_cpu=001`,
		`idle-0 (0) [000] .... 2.050000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=other next_pid=999 next_prio=20`,
	}
	fullPath := writeSchedulerIntegrityTrace(t, "scoped-head-full.systrace", lines...)
	windowPath := writeSchedulerIntegrityTrace(t, "scoped-head-window.systrace", lines...)
	full, err := BuildIndex(context.Background(), fullPath)
	if err != nil {
		t.Fatal(err)
	}
	windowed, err := BuildIndexWithOptions(context.Background(), windowPath, BuildOptions{
		AllowWindowedParse: true,
		TimeStart:          2,
		TimeStartSet:       true,
		TimeEnd:            2.1,
		TimeEndSet:         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	query := Query{PID: 999, TimeStart: 2, TimeEnd: 2.1}
	for name, idx := range map[string]*Index{"full": full, "windowed": windowed} {
		head := schedulerHeadForQuery(idx, query)
		if head == nil || !head.Complete || head.Reason != "" {
			t.Fatalf("%s unrelated head was globally poisoned: %+v", name, head)
		}
		state, ok := head.Threads[999]
		if !ok || state.State != StateRunnable || state.StartTs != 1.1 {
			t.Fatalf("%s lost unrelated runnable carry-in: %+v", name, state)
		}
		if failure := schedulerStateIntegrityFailureForQuery(idx, query, 999); failure != nil {
			t.Fatalf("%s scoped poison contaminated pid 999: %+v", name, failure)
		}
		if failure := schedulerStateIntegrityFailureForQuery(idx, Query{PID: 100, TimeStart: 2, TimeEnd: 2.1}, 100); failure == nil {
			t.Fatalf("%s candidate pid 100 escaped scoped fail-close", name)
		}
	}
}

func TestWarmAnchorSeekTransportsScopedSchedulerPoison(t *testing.T) {
	resetAnchorCaches()
	defer resetAnchorCaches()
	lineCount := 3 * traceAnchorLineInterval
	lines := make([]string, 0, lineCount)
	lines = append(lines,
		`idle-0 (0) [000] .... 100.000100: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=other next_pid=999 next_prio=20`,
		`other-999 (999) [000] .... 100.000200: sched_switch: prev_comm=other prev_pid=999 prev_prio=20 prev_state=R ==> next_comm=idle/0 next_pid=0 next_prio=120`,
		`waker-7 (7) [001] .... 100.000300: sched_wakeup: comm=bad pid=100 pid=101 prio=20 target_cpu=001`,
		`logger-8 (8) [001] .... `+strings.Repeat("9", 306)+`.000000: tracing_mark_write: note=sched_wakeup: comm=fake pid=999`,
	)
	for lineNo := 5; lineNo <= lineCount; lineNo++ {
		ts := 100 + float64(lineNo)*0.0001
		lines = append(lines, fmt.Sprintf(
			`freq-1 (1) [000] .... %.6f: cpu_frequency: state=1000000 cpu_id=0`, ts))
	}
	path := writeSchedulerIntegrityTrace(t, "scoped-anchor-seek.systrace", lines...)
	start := 100 + float64(2*traceAnchorLineInterval+100)*0.0001
	opts := BuildOptions{
		AllowWindowedParse: true,
		TimeStart:          start,
		TimeStartSet:       true,
		TimeEnd:            start + 0.01,
		TimeEndSet:         true,
	}
	cold, err := BuildIndexWithOptions(context.Background(), path, opts)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	set := anchorCache.load(traceAnchorKeyForInfo(canonicalTraceIndexPath(path), info))
	if set == nil || set.TimestampOrder != TraceTimestampOrderMonotonic || len(set.Anchors) < 2 {
		t.Fatalf("cold scan did not establish warm-seek precondition: %+v", set)
	}
	indexCache = newTraceIndexCache(traceIndexCacheBudgetBytes)
	warm, err := BuildIndexWithOptions(context.Background(), path, opts)
	if err != nil {
		t.Fatal(err)
	}
	if warm.ScannedLineCount >= cold.ScannedLineCount {
		t.Fatalf("fixture did not exercise anchor seek: cold=%d warm=%d", cold.ScannedLineCount, warm.ScannedLineCount)
	}
	query := Query{PID: 999, TimeStart: start, TimeEnd: start + 0.01}
	head := schedulerHeadForQuery(warm, query)
	if head == nil || !head.Complete || head.Threads[999].State != StateRunnable {
		t.Fatalf("warm seek lost unrelated runnable carry: %+v", head)
	}
	if failure := schedulerStateIntegrityFailureForQuery(warm, query, 999); failure != nil {
		t.Fatalf("warm transported poison contaminated unrelated pid: %+v", failure)
	}
	failure := schedulerRowIntegrityFailureForQuery(warm,
		Query{PID: 100, TimeStart: start, TimeEnd: start + 0.01}, 100)
	if failure == nil || failure.AffectsAllPIDs || len(failure.PIDs) != 2 ||
		failure.PIDs[0] != 100 || failure.PIDs[1] != 101 || failure.LocalLine != 3 ||
		failure.SourcePath != canonicalTraceIndexPath(path) {
		t.Fatalf("anchor seek dropped or corrupted scoped prefix witness: %+v", failure)
	}

	globalLines := append([]string(nil), lines...)
	globalLines[2] = `waker-7 (7) [001] .... ` + strings.Repeat("9", 306) +
		`.000000: sched_wakeup: comm=bad pid=100 prio=20 target_cpu=001`
	globalPath := writeSchedulerIntegrityTrace(t, "global-anchor-seek.systrace", globalLines...)
	globalCold, err := BuildIndexWithOptions(context.Background(), globalPath, opts)
	if err != nil {
		t.Fatal(err)
	}
	indexCache = newTraceIndexCache(traceIndexCacheBudgetBytes)
	globalWarm, err := BuildIndexWithOptions(context.Background(), globalPath, opts)
	if err != nil {
		t.Fatal(err)
	}
	if globalWarm.ScannedLineCount >= globalCold.ScannedLineCount {
		t.Fatalf("global warm fixture did not consume cached monotonic/anchor state: cold=%d warm=%d", globalCold.ScannedLineCount, globalWarm.ScannedLineCount)
	}
	globalFailure := schedulerRowIntegrityFailureForQuery(globalWarm, query, 999)
	if globalFailure == nil || !globalFailure.AffectsAllPIDs ||
		!containsSubstring(globalFailure.Fields, "parser_rejected_row") {
		t.Fatalf("anchor seek dropped global rejected-row witness: %+v", globalFailure)
	}
	if globalHead := schedulerHeadForQuery(globalWarm, query); globalHead == nil || globalHead.Complete ||
		!strings.Contains(globalHead.Reason, "scheduler_row_parse_incomplete") {
		t.Fatalf("global rejected prefix did not fail head closed: %+v", globalHead)
	}
}

func TestStrictWakeupPriorityDegradesWithoutSuppressingEdge(t *testing.T) {
	interner := newStringInterner()
	for _, fields := range []string{
		"comm=app pid=100 target_cpu=001",
		"comm=app pid=100 prio=bad target_cpu=001",
		"comm=app pid=100 prio=0 target_cpu=001",
		"comm=app pid=100 prio=-1 target_cpu=001",
		"comm=app pid=100 prio=020 target_cpu=001",
		"comm=app pid=100 prio=2147483648 target_cpu=001",
		"comm=app pid=100 prio=20 prio=20 target_cpu=001",
		"comm=app pid=100 prio=20 prio=159 target_cpu=001",
	} {
		ev, ok := ParseLine(1, schedulerWakeLine("sched_wakeup", fields), interner)
		if !ok || ev.WakeePID != 100 {
			t.Fatalf("field-level priority failure suppressed exact edge: fields=%q ok=%v event=%+v", fields, ok, ev)
		}
		if eventWakeePriorityForHardUse(ev) != 0 || ev.WakeePrioritySource() != WakeePrioritySourceUnknown {
			t.Fatalf("bad priority retained hard authority: fields=%q event=%+v", fields, ev)
		}
		if cpu, valid := eventTargetCPU(ev); !valid || cpu != 1 {
			t.Fatalf("bad priority poisoned valid target CPU sibling: fields=%q event=%+v", fields, ev)
		}
	}

	for _, fields := range []string{
		"comm=app pid=100 prio=159 target_cpu=001 codrax_prio_source=unknown codrax_prio_source=unknown",
		"comm=app pid=100 prio=159 target_cpu=001 codrax_prio_source=unknown codrax_prio_source=inferred_next_sched_slice",
		"comm=app pid=100 prio=159 target_cpu=001 codrax_prio_source=future_source",
	} {
		ev, ok := ParseLine(1, schedulerWakeLine("sched_wakeup", fields), interner)
		if !ok || eventWakeePriorityForHardUse(ev) != 0 || ev.WakeePrioritySource() != "untrusted" {
			t.Fatalf("ambiguous/unknown provenance regained exact authority: fields=%q ok=%v event=%+v", fields, ok, ev)
		}
	}
}

func TestStrictWakeupTargetCPUFailureOnlyRemovesCPUDimension(t *testing.T) {
	interner := newStringInterner()
	for _, tc := range []struct {
		fields string
		reason string
	}{
		{fields: "comm=app pid=100 prio=159", reason: "missing"},
		{fields: "comm=app pid=100 prio=159 target_cpu=bad", reason: "not_unsigned_decimal"},
		{fields: "comm=app pid=100 prio=159 target_cpu=-1", reason: "not_unsigned_decimal"},
		{fields: "comm=app pid=100 prio=159 target_cpu=4096", reason: "cpu_above_limit"},
		{fields: "comm=app pid=100 prio=159 target_cpu=99999999999999999999", reason: "integer_overflow"},
		{fields: "comm=app pid=100 prio=159 target_cpu=001 target_cpu=001", reason: "duplicate"},
		{fields: "comm=app pid=100 prio=159 target_cpu=001 target_cpu=002", reason: "duplicate"},
		// Producer comm is an opaque, variable-length prefix. A key-shaped token
		// before the unique PID is display text, so the real suffix dimension is
		// missing rather than a second/misordered authority.
		{fields: "comm=app target_cpu=001 pid=100 prio=159", reason: "missing"},
	} {
		t.Run(strings.ReplaceAll(tc.fields, " ", "_"), func(t *testing.T) {
			line := schedulerWakeLine("sched_wakeup", tc.fields)
			ev, ok := ParseLine(1, line, interner)
			if !ok || ev.WakeePID != 100 || eventWakeePriorityForHardUse(ev) != 159 {
				t.Fatalf("CPU-only failure suppressed valid edge/priority: ok=%v event=%+v", ok, ev)
			}
			if _, valid := eventTargetCPU(ev); valid || !ev.CPUInputInvalid {
				t.Fatalf("bad target CPU retained CPU authority: %+v", ev)
			}
			failures := cpuInputValidationFailures(1, line)
			matched := false
			for _, failure := range failures {
				if failure.Field == "target_cpu" && failure.ReasonCode == tc.reason {
					matched = true
				}
			}
			if !matched {
				t.Fatalf("CPU degradation lacks exact typed witness reason=%q failures=%+v", tc.reason, failures)
			}
		})
	}
}

func TestMalformedWakeupPriorityCannotBorrowNearbySliceForInversion(t *testing.T) {
	const prefix = `
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=159 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 5.000100: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
`
	const suffix = `
     worker-200 (200) [002] .... 5.005100: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.006000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=159
        app-100 (100) [001] .... 5.007000: sched_switch: prev_comm=app prev_pid=100 prev_prio=159 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
`
	for _, priority := range []string{"", " prio=bad", " prio=0", " prio=20 prio=159"} {
		idx := buildTraceIndex(t, "strict-wake-priority.systrace", prefix+
			"     worker-200 (200) [002] .... 5.005000: sched_wakeup: comm=app pid=100"+priority+" target_cpu=001"+suffix)
		chain := BuildWakeupChain(idx, Query{
			PID: 100, TimeStart: 5, TimeEnd: 5.007, MaxDepth: 4,
			MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace,
		})
		found := false
		for _, edge := range chain.Edges {
			if edge.Waker.PID != 200 || edge.Wakee.PID != 100 {
				continue
			}
			found = true
			if edge.WakeePriority != 0 || edge.WakeePrioritySource != WakeePrioritySourceUnknown || edge.PriorityRelation != "" || edge.PriorityInversionCandidate {
				t.Fatalf("nearby sched slice restored non-exact wake priority %q: %+v", priority, edge)
			}
		}
		if !found {
			t.Fatalf("priority degradation removed the physical wake edge %q: %+v", priority, chain)
		}
	}
}

func TestStrictSchedMigrateRequiresUniqueCompleteTuple(t *testing.T) {
	interner := newStringInterner()
	valid := "migrator-7 (7) [001] .... 5.001000: sched_migrate_task: comm=effect thread pid=100 prio=-1 orig_cpu=0 dest_cpu=4095"
	ev, ok := ParseLine(1, valid, interner)
	if !ok || ev.Type != EventCPUConstraint || ev.ConstraintFields == nil || ev.ConstraintFields.PID != 100 ||
		!ev.ConstraintFields.OrigCPUSet || ev.ConstraintFields.OrigCPU != 0 || !ev.ConstraintFields.DestCPUSet || ev.ConstraintFields.DestCPU != 4095 {
		t.Fatalf("canonical migrate tuple lost authority: ok=%v event=%+v", ok, ev)
	}

	for _, fields := range []string{
		"comm=app pid=100 pid=100 prio=20 orig_cpu=0 dest_cpu=1",
		"comm=app pid=100 prio=20 orig_cpu=0 orig_cpu=0 dest_cpu=1",
		"comm=app pid=100 prio=20 orig_cpu=0 dest_cpu=1 dest_cpu=1",
		"comm=app pid=100 prio=20 orig_cpu=4096 dest_cpu=1",
		"comm=app pid=100 prio=20 orig_cpu=0 dest_cpu=-1",
		"comm=app target_pid=100 prio=20 orig_cpu=0 dest_cpu=1",
	} {
		line := "migrator-7 (7) [001] .... 5.001000: sched_migrate_task: " + fields
		if rejected, accepted := ParseLine(1, line, interner); accepted {
			t.Fatalf("incomplete/ambiguous migrate tuple minted a migration: fields=%q event=%+v", fields, rejected)
		}
		if failure := schedulerRowValidationFailure(1, line); failure == nil {
			t.Fatalf("rejected migrate tuple lacks typed integrity witness: %q", fields)
		}
	}

	for _, fields := range []string{
		"comm=app pid=100 prio=20 prio=20 orig_cpu=0 dest_cpu=1",
		"comm=app pid=100 prio=bad orig_cpu=0 dest_cpu=1",
		"comm=app pid=100 orig_cpu=0 dest_cpu=1",
	} {
		line := "migrator-7 (7) [001] .... 5.001000: sched_migrate_task: " + fields
		ev, ok := ParseLine(1, line, interner)
		if !ok || ev.ConstraintFields == nil || ev.ConstraintFields.PID != 100 ||
			!ev.ConstraintFields.OrigCPUSet || ev.ConstraintFields.OrigCPU != 0 ||
			!ev.ConstraintFields.DestCPUSet || ev.ConstraintFields.DestCPU != 1 {
			t.Fatalf("non-authoritative migrate priority poisoned exact CPU tuple: fields=%q ok=%v event=%+v", fields, ok, ev)
		}
	}
}

func TestAmbiguousWakeupNewCannotResetGenerationFullOrWindowed(t *testing.T) {
	path := writeSchedulerIntegrityTrace(t, "ambiguous-wakeup-new.systrace",
		`idle-0 (0) [000] .... 1.900000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=old next_pid=42 next_prio=20`,
		`old-42 (42) [000] .... 1.950000: sched_switch: prev_comm=old prev_pid=42 prev_prio=20 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120`,
		`creator-7 (7) [001] .... 2.000000: sched_wakeup_new: comm=new pid=42 pid=42 prio=30 target_cpu=001`,
	)
	builds := []struct {
		name string
		load func() (*Index, error)
	}{
		{name: "full", load: func() (*Index, error) { return BuildIndex(context.Background(), path) }},
		{name: "windowed", load: func() (*Index, error) {
			return BuildIndexWithOptions(context.Background(), path, BuildOptions{
				AllowWindowedParse: true,
				TimeStart:          1.9,
				TimeStartSet:       true,
				TimeEnd:            2.1,
				TimeEndSet:         true,
			})
		}},
	}
	for _, build := range builds {
		t.Run(build.name, func(t *testing.T) {
			idx, err := build.load()
			if err != nil {
				t.Fatal(err)
			}
			for _, ev := range idx.Events {
				if ev.Name == "sched_wakeup_new" {
					t.Fatalf("ambiguous creation edge entered generation state: %+v", ev)
				}
			}
			if boundaries, _ := threadGenerationBoundaries(idx, 42); len(boundaries) != 0 {
				t.Fatalf("rejected wakeup_new reset numeric TID generation: %+v", boundaries)
			}
			if len(idx.schedulerRowIntegrityFailures) != 1 ||
				idx.schedulerRowIntegrityFailures[0].EventName != "sched_wakeup_new" ||
				!containsSubstring(idx.schedulerRowIntegrityFailures[0].Fields, "pid_duplicate") {
				t.Fatalf("full/windowed lanes lost the same typed PID poison: %+v", idx.schedulerRowIntegrityFailures)
			}
		})
	}
}

func TestStrictSchedulerScalarFailuresDoNotPoisonValidSiblings(t *testing.T) {
	for _, tc := range []struct {
		name       string
		line       string
		wantFields []string
	}{
		{
			name:       "wake_bad_pid",
			line:       schedulerWakeLine("sched_wakeup", "comm=app pid=-1 prio=159 target_cpu=001"),
			wantFields: []string{"pid", "pid_invalid"},
		},
		{
			name:       "migrate_bad_pid",
			line:       "migrator-7 (7) [001] .... 5.001000: sched_migrate_task: comm=app pid=-1 prio=20 orig_cpu=0 dest_cpu=1",
			wantFields: []string{"pid", "pid_invalid"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failure := schedulerRowValidationFailure(1, tc.line)
			if failure == nil || strings.Join(failure.Fields, ",") != strings.Join(tc.wantFields, ",") {
				t.Fatalf("scalar failure cascaded into valid siblings: got=%+v want=%v", failure, tc.wantFields)
			}
			if cpuFailures := cpuInputValidationFailures(1, tc.line); len(cpuFailures) != 0 {
				t.Fatalf("non-CPU scalar failure fabricated CPU integrity poison: %+v", cpuFailures)
			}
		})
	}

	badPriority := "migrator-7 (7) [001] .... 5.001000: sched_migrate_task: comm=app pid=100 prio=bad orig_cpu=0 dest_cpu=1"
	if ev, ok := ParseLine(1, badPriority, newStringInterner()); !ok || ev.ConstraintFields == nil || !ev.ConstraintFields.DestCPUSet {
		t.Fatalf("migrate priority is not a hard CPU-tuple dimension: ok=%v event=%+v", ok, ev)
	}
	if failure := schedulerRowValidationFailure(1, badPriority); failure != nil {
		t.Fatalf("bad advisory migrate priority poisoned scheduler state: %+v", failure)
	}
	if cpuFailures := cpuInputValidationFailures(1, badPriority); len(cpuFailures) != 0 {
		t.Fatalf("bad advisory migrate priority fabricated CPU poison: %+v", cpuFailures)
	}
}

func TestStrictSchedulerCoreRejectsBareTextBetweenProducerFields(t *testing.T) {
	interner := newStringInterner()
	for _, line := range []string{
		schedulerWakeLine("sched_wakeup", "comm=label pid=777 worker prio=159 target_cpu=001"),
		schedulerWakeLine("sched_wakeup", "comm=label pid=100 prio=159 worker target_cpu=001"),
		"migrator-7 (7) [001] .... 5.001000: sched_migrate_task: comm=label pid=100 worker prio=20 orig_cpu=0 dest_cpu=1",
	} {
		if ev, ok := ParseLine(1, line, interner); ok {
			t.Fatalf("bare text inside scheduler core minted a typed edge: %+v", ev)
		}
		failure := schedulerRowValidationFailure(1, line)
		if failure == nil || !containsSubstring(failure.Fields, "scheduler_core_noncanonical") {
			t.Fatalf("core grammar rejection lacks typed witness: %+v", failure)
		}
	}

	for _, line := range []string{
		schedulerWakeLine("sched_wakeup", "comm=app pid=100 prio=159 target_cpu=001 vendor_meta=ok"),
		schedulerWakeLine("sched_wakeup", "comm=app pid=100 prio=159 success=1 target_cpu=001 vendor_meta=ok"),
		"migrator-7 (7) [001] .... 5.001000: sched_migrate_task: comm=app pid=100 prio=20 orig_cpu=0 dest_cpu=1 vendor_meta=ok",
	} {
		if _, ok := ParseLine(1, line, interner); !ok {
			t.Fatalf("structured trailing metadata/closed success profile was rejected: %q", line)
		}
	}
}

func TestLegacyWakeSuccessProfileCannotMintCausality(t *testing.T) {
	interner := newStringInterner()
	for _, fields := range []string{
		"comm=app pid=100 prio=159 target_cpu=001",
		"comm=app pid=100 prio=159 success=1 target_cpu=001",
	} {
		ev, ok := ParseLine(1, schedulerWakeLine("sched_wakeup", fields), interner)
		if !ok || ev.Type != EventSchedWakeup || ev.WakeePID != 100 {
			t.Fatalf("modern/legacy-success wake profile lost causal edge: fields=%q ok=%v event=%+v", fields, ok, ev)
		}
	}

	noTransition := schedulerWakeLine("sched_wakeup", "comm=app pid=100 prio=159 success=0 target_cpu=001")
	noOp, ok := ParseLine(1, noTransition, interner)
	if !ok || noOp.Type != EventUnknown || noOp.Name != "sched_wakeup" || noOp.WakeePID != 0 {
		t.Fatalf("legacy success=0 minted a causal wake or lost physical observation: ok=%v event=%+v", ok, noOp)
	}
	if failure := schedulerRowValidationFailure(1, noTransition); failure != nil {
		t.Fatalf("valid no-transition observation was mislabeled malformed: %+v", failure)
	}

	for _, success := range []string{"-7", "2", "01", "+1"} {
		line := schedulerWakeLine("sched_wakeup", "comm=app pid=100 prio=159 success="+success+" target_cpu=001")
		if ev, accepted := ParseLine(1, line, interner); accepted {
			t.Fatalf("noncanonical legacy success minted an edge: success=%q event=%+v", success, ev)
		}
		failure := schedulerRowValidationFailure(1, line)
		if failure == nil || failure.AffectsAllPIDs || len(failure.PIDs) != 1 || failure.PIDs[0] != 100 ||
			!containsSubstring(failure.Fields, "success") {
			t.Fatalf("bad success lacks PID-scoped typed witness: success=%q failure=%+v", success, failure)
		}
	}
	duplicate := schedulerWakeLine("sched_wakeup", "comm=app pid=100 prio=159 success=1 success=1 target_cpu=001")
	if ev, accepted := ParseLine(1, duplicate, interner); accepted {
		t.Fatalf("duplicate success minted an edge: %+v", ev)
	}
	if failure := schedulerRowValidationFailure(1, duplicate); failure == nil ||
		!containsSubstring(failure.Fields, "success_duplicate") {
		t.Fatalf("duplicate success lacks exact typed witness: %+v", failure)
	}
}

func TestWakeupNewSuccessZeroFullWindowParity(t *testing.T) {
	for _, tc := range []struct {
		name         string
		success      string
		wantBoundary bool
		badTargetCPU bool
	}{
		{name: "no_transition", success: "0", wantBoundary: false, badTargetCPU: true},
		{name: "transition", success: "1", wantBoundary: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := "001"
			if tc.badTargetCPU {
				target = "bad"
			}
			lines := []string{
				`idle-0 (0) [000] .... 1.900000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=old next_pid=42 next_prio=20`,
				`old-42 (42) [000] .... 1.950000: sched_switch: prev_comm=old prev_pid=42 prev_prio=20 prev_state=X ==> next_comm=idle/0 next_pid=0 next_prio=120`,
				`creator-7 (7) [001] .... 2.000000: sched_wakeup_new: comm=new pid=42 prio=30 success=` + tc.success + ` target_cpu=` + target,
			}
			// Distinct paths force both cold parsers; otherwise the global cache
			// may derive the windowed view from the full index and mask parity bugs.
			fullPath := writeSchedulerIntegrityTrace(t, "success-full-"+tc.success+".systrace", lines...)
			windowPath := writeSchedulerIntegrityTrace(t, "success-window-"+tc.success+".systrace", lines...)
			builds := []struct {
				name string
				load func() (*Index, error)
			}{
				{name: "full", load: func() (*Index, error) { return BuildIndex(context.Background(), fullPath) }},
				{name: "windowed", load: func() (*Index, error) {
					return BuildIndexWithOptions(context.Background(), windowPath, BuildOptions{
						AllowWindowedParse: true,
						TimeStart:          1.9,
						TimeStartSet:       true,
						TimeEnd:            2.1,
						TimeEndSet:         true,
					})
				}},
			}
			for _, build := range builds {
				t.Run(build.name, func(t *testing.T) {
					idx, err := build.load()
					if err != nil {
						t.Fatal(err)
					}
					boundaries, _ := threadGenerationBoundaries(idx, 42)
					if (len(boundaries) != 0) != tc.wantBoundary {
						t.Fatalf("success=%s generation boundary=%+v want=%v", tc.success, boundaries, tc.wantBoundary)
					}
					if len(idx.schedulerRowIntegrityFailures) != 0 {
						t.Fatalf("valid success profile produced scheduler poison: %+v", idx.schedulerRowIntegrityFailures)
					}
					wantCPUFailures := 0
					if tc.badTargetCPU {
						wantCPUFailures = 1
					}
					if len(idx.cpuInputIntegrityFailures) != wantCPUFailures {
						t.Fatalf("full/window CPU audit drift: got=%+v want_count=%d", idx.cpuInputIntegrityFailures, wantCPUFailures)
					}
					if tc.success == "0" {
						chain := BuildWakeupChain(idx, Query{PID: 42, TimeStart: 1.9, TimeEnd: 2.1, MaxDepth: 2})
						for _, edge := range chain.Edges {
							if edge.Waker.PID == 7 && edge.Wakee.PID == 42 {
								t.Fatalf("success=0 raw observation leaked into wake consumer: %+v", edge)
							}
						}
					}
				})
			}
		})
	}
}

func TestFtraceHeaderThreadIdentityUsesSignedInt32Domain(t *testing.T) {
	interner := newStringInterner()
	maximum := `waker-2147483647 (2147483647) [002] .... 5.001000: sched_wakeup: comm=app pid=100 prio=159 target_cpu=001`
	ev, ok := ParseLine(1, maximum, interner)
	if !ok || ev.PID != 2147483647 || ev.TGID != 2147483647 || ev.WakeePID != 100 {
		t.Fatalf("signed-int32 header boundary lost authority: ok=%v event=%+v", ok, ev)
	}

	overflowPID := `waker-2147483648 (1) [002] .... 5.001000: sched_wakeup: comm=app pid=100 prio=159 target_cpu=001`
	if forged, accepted := ParseLine(1, overflowPID, interner); accepted {
		t.Fatalf("native-int-width header TID minted a wake edge: %+v", forged)
	}
	for _, tgid := range []string{"2147483648", "-7", "-----"} {
		line := `waker-200 (` + tgid + `) [002] .... 5.001000: sched_wakeup: comm=app pid=100 prio=159 target_cpu=001`
		got, accepted := ParseLine(1, line, interner)
		if !accepted || got.PID != 200 || got.TGID != 0 || got.WakeePID != 100 {
			t.Fatalf("invalid optional TGID did not degrade platform-stably: tgid=%q ok=%v event=%+v", tgid, accepted, got)
		}
	}

	path := writeSchedulerIntegrityTrace(t, "overflow-waker-pid.systrace", overflowPID)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	for _, indexed := range idx.Events {
		if indexed.Name == "sched_wakeup" {
			t.Fatalf("index admitted impossible outer waker identity: %+v", indexed)
		}
	}
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 5, TimeEnd: 5.01, MaxDepth: 2})
	if len(chain.Edges) != 0 {
		t.Fatalf("rejected outer waker identity reached relation engine: %+v", chain.Edges)
	}
}

func TestStrictSchedulerDuplicatePredecessorDoesNotFabricateCPUPoison(t *testing.T) {
	for _, line := range []string{
		schedulerWakeLine("sched_wakeup", "comm=app pid=100 pid=100 prio=159 target_cpu=001"),
		"migrator-7 (7) [001] .... 5.001000: sched_migrate_task: comm=app pid=100 pid=100 prio=20 orig_cpu=0 dest_cpu=1",
		"migrator-7 (7) [001] .... 5.001000: sched_migrate_task: comm=app pid=100 prio=20 prio=20 orig_cpu=0 dest_cpu=1",
	} {
		if cpuFailures := cpuInputValidationFailures(1, line); len(cpuFailures) != 0 {
			t.Fatalf("duplicate non-CPU predecessor cascaded into CPU poison: line=%q failures=%+v", line, cpuFailures)
		}
	}
}

func TestSoftWakeFieldsReorderOrRepeatDegradeOnlyTheirDimension(t *testing.T) {
	interner := newStringInterner()
	for _, fields := range []string{
		"comm=app pid=100 target_cpu=001 prio=159",
		"comm=app pid=100 prio=159 target_cpu=001 prio=159",
		"comm=app pid=100 prio= 20 target_cpu=001",
	} {
		ev, ok := ParseLine(1, schedulerWakeLine("sched_wakeup", fields), interner)
		if !ok || ev.WakeePID != 100 || eventWakeePriorityForHardUse(ev) != 0 || ev.WakeePrioritySource() != WakeePrioritySourceUnknown {
			t.Fatalf("priority disorder/duplicate suppressed edge or retained priority: fields=%q ok=%v event=%+v", fields, ok, ev)
		}
		if cpu, valid := eventTargetCPU(ev); !valid || cpu != 1 || ev.CPUInputInvalid {
			t.Fatalf("priority disorder/duplicate poisoned exact CPU sibling: fields=%q event=%+v", fields, ev)
		}
	}

	duplicateTarget := schedulerWakeLine("sched_wakeup",
		"comm=app pid=100 prio=159 target_cpu=001 codrax_prio_source=unknown target_cpu=001")
	ev, ok := ParseLine(1, duplicateTarget, interner)
	if !ok || ev.WakeePID != 100 || eventWakeePriorityForHardUse(ev) != 0 {
		t.Fatalf("duplicate target CPU suppressed exact edge: ok=%v event=%+v", ok, ev)
	}
	if _, valid := eventTargetCPU(ev); valid || !ev.CPUInputInvalid {
		t.Fatalf("duplicate target CPU retained CPU authority: %+v", ev)
	}

	spacedTarget := schedulerWakeLine("sched_wakeup", "comm=app pid=100 prio=159 target_cpu= 001")
	ev, ok = ParseLine(1, spacedTarget, interner)
	if !ok || ev.WakeePID != 100 || eventWakeePriorityForHardUse(ev) != 159 {
		t.Fatalf("bad target spacing suppressed valid identity/priority: ok=%v event=%+v", ok, ev)
	}
	if _, valid := eventTargetCPU(ev); valid || !ev.CPUInputInvalid {
		t.Fatalf("bad target spacing retained CPU authority: %+v", ev)
	}

	reorderedMigrate := "migrator-7 (7) [001] .... 5.001000: sched_migrate_task: comm=app pid=100 orig_cpu=0 prio=20 dest_cpu=1"
	migration, ok := ParseLine(1, reorderedMigrate, interner)
	if !ok || migration.ConstraintFields == nil || !migration.ConstraintFields.OrigCPUSet || !migration.ConstraintFields.DestCPUSet {
		t.Fatalf("advisory migrate priority order poisoned exact CPU tuple: ok=%v event=%+v", ok, migration)
	}
}

func TestQuotedOpaqueMetadataCannotMintSchedulerAuthority(t *testing.T) {
	interner := newStringInterner()
	wake := schedulerWakeLine("sched_wakeup", `comm=app pid=100 vendor="x prio=159 target_cpu=001 y"`)
	ev, ok := ParseLine(1, wake, interner)
	if !ok || ev.WakeePID != 100 || eventWakeePriorityForHardUse(ev) != 0 || ev.WakeePrioritySource() != WakeePrioritySourceUnknown {
		t.Fatalf("quoted opaque metadata minted wake priority or removed edge: ok=%v event=%+v", ok, ev)
	}
	if _, valid := eventTargetCPU(ev); valid || !ev.CPUInputInvalid {
		t.Fatalf("quoted opaque metadata minted target CPU: %+v", ev)
	}

	migrate := `migrator-7 (7) [001] .... 5.001000: sched_migrate_task: comm=app pid=100 vendor="x prio=20 orig_cpu=0 dest_cpu=1 y"`
	if forged, accepted := ParseLine(1, migrate, interner); accepted {
		t.Fatalf("quoted opaque metadata minted migration tuple: %+v", forged)
	}
	failure := schedulerRowValidationFailure(1, migrate)
	if failure == nil || !containsSubstring(failure.Fields, "orig_cpu_missing") || !containsSubstring(failure.Fields, "dest_cpu_missing") {
		t.Fatalf("quoted migrate pseudo fields lack precise typed rejection: %+v", failure)
	}

	for _, metadata := range []string{
		`vendor="x prio=20 target_cpu=007 y"`,
		`vendor= "x prio=20 target_cpu=007 y"`,
		`vendor = "x prio=20 target_cpu=007 y"`,
	} {
		exactWake := schedulerWakeLine("sched_wakeup",
			"comm=app pid=100 prio=159 target_cpu=001 "+metadata)
		exact, ok := ParseLine(1, exactWake, interner)
		if !ok || exact.WakeePID != 100 || eventWakeePriorityForHardUse(exact) != 159 {
			t.Fatalf("quoted soft lookalikes poisoned real wake authority: metadata=%q ok=%v event=%+v", metadata, ok, exact)
		}
		if cpu, valid := eventTargetCPU(exact); !valid || cpu != 1 {
			t.Fatalf("quoted soft lookalike replaced real target CPU: metadata=%q event=%+v", metadata, exact)
		}
	}

	for _, fields := range []string{
		`comm=app pid=100 prio=159 target_cpu=001 vendor="x pid=999 prio=20 target_cpu=007 y"`,
		`comm=label foo=" x pid=100 prio=159 target_cpu=001 " pid=999 prio=20 target_cpu=002`,
		`comm=label=" pid=100 " pid=999 prio=20 target_cpu=001`,
	} {
		line := schedulerWakeLine("sched_wakeup", fields)
		if forged, accepted := ParseLine(1, line, interner); accepted {
			t.Fatalf("quoted comm/metadata hid or replaced PID authority: fields=%q event=%+v", fields, forged)
		}
		failure := schedulerRowValidationFailure(1, line)
		if failure == nil || !containsSubstring(failure.Fields, "pid_duplicate") {
			t.Fatalf("quote-blind PID rejection lacks precise witness: fields=%q failure=%+v", fields, failure)
		}
	}

	badQuote := schedulerWakeLine("sched_wakeup",
		`comm=app pid=100 vendor=x" prio=159 target_cpu=001 y"`)
	if forged, accepted := ParseLine(1, badQuote, interner); accepted {
		t.Fatalf("mid-value quote swallowed producer core: %+v", forged)
	}
	if failure := schedulerRowValidationFailure(1, badQuote); failure == nil ||
		!containsSubstring(failure.Fields, "scheduler_core_noncanonical") {
		t.Fatalf("mid-value quote rejection lacks typed witness: %+v", failure)
	}
}
