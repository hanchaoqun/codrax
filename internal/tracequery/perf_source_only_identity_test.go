package tracequery

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestPerfSourceOnlyNewWireScrubsIdentityButKeepsAuditInventory(t *testing.T) {
	intern := newStringInterner()
	ev, ok := ParseLine(1, `perf-unverified-777 (666) [003] .... 10.000000: perf_sample: cpu=3 cpu_known=true pid=666 tid=777 thread_comm=worker sample_weight=9 event=cpu-cycles symbol=Hot::work dso=libhot.so source=trace_streamer_db sample_kind=on_cpu thread_identity_known=false resolution=perf_source_only lifecycle_unverified=true perf_source_pid=666 perf_source_tid=777 perf_source_comm=worker`, intern)
	if !ok || ev.PerfFields == nil {
		t.Fatal("source-only perf inventory row should parse")
	}
	pf := ev.PerfFields
	if ev.PID != 0 || ev.TGID != 0 || ev.Comm != "" || pf.PID != 0 || pf.TID != 0 || pf.Comm != "" {
		t.Fatalf("source-only transport/header identity survived normalization: %+v", ev)
	}
	if pf.ThreadIdentityKnown == nil || *pf.ThreadIdentityKnown ||
		pf.LifecycleUnverified == nil || !*pf.LifecycleUnverified ||
		pf.Resolution != perfSourceOnlyResolution ||
		pf.SourcePID != 666 || pf.SourceTID != 777 || pf.SourceComm != "worker" {
		t.Fatalf("typed source-only provenance was not retained: %+v", pf)
	}
	if !perfSampleHasKnownCPU(ev) || !perfSampleIsOnCPU(ev) || ev.CPU != 3 {
		t.Fatalf("independently proved CPU0+ sample identity was lost: %+v", ev)
	}
	if perfSampleHasTypedThreadIdentity(ev) || eventMentionsPID(ev, 777) || eventMentionsThread(ev, "worker") || perfSampleMatchesThread(ev, ThreadRef{PID: 777, Comm: "worker"}) {
		t.Fatal("source-only audit coordinates were resurrected as typed thread identity")
	}
	if got := perfSampleThread(ev); got != (ThreadRef{}) {
		t.Fatalf("source-only sample contributed an incarnation identity: %+v", got)
	}
	if !eventMatchesPattern(ev, "Hot::work") || !eventMatchesPattern(ev, "libhot.so") {
		t.Fatal("safe symbol/DSO inventory became unsearchable")
	}
	for _, unsafePattern := range []string{
		"worker",
		"777",
		"perf-unverified-777",
		"thread_comm=worker",
		"perf_source_tid=777",
		"cpu=3 cpu_known=true",
	} {
		if eventMatchesPattern(ev, unsafePattern) {
			t.Fatalf("source-only audit/header/raw-field pattern %q revived anonymous identity", unsafePattern)
		}
		if got := EventSearch(&Index{Events: []Event{ev}}, Query{Pattern: unsafePattern, EventTypes: []EventType{EventPerfSample}, Limit: 8}); len(got) != 0 {
			t.Fatalf("event_search revived source-only identity with pattern %q: %+v", unsafePattern, got)
		}
	}
	for _, inventoryPattern := range []string{
		"Hot::work",
		"libhot.so",
		"cpu-cycles",
		"trace_streamer_db",
		"perf_source_only",
		"on_cpu",
	} {
		if got := EventSearch(&Index{Events: []Event{ev}}, Query{Pattern: inventoryPattern, EventTypes: []EventType{EventPerfSample}, Limit: 8}); len(got) != 1 {
			t.Fatalf("safe source-only inventory pattern %q became unsearchable: %+v", inventoryPattern, got)
		}
	}

	idx := &Index{Events: []Event{ev}}
	global := computePerfContext(idx, Query{TimeStart: 9.9, TimeEnd: 10.1}, 8)
	if global == nil || global.SampleCount != 1 || len(global.TopSymbols) != 1 || len(global.TopDSO) != 1 {
		t.Fatalf("global anonymous inventory was lost: %+v", global)
	}
	if len(global.TopThreads) != 0 || len(global.TopSymbols[0].Threads) != 0 {
		t.Fatalf("anonymous inventory minted a thread roster: %+v", global)
	}
	if !reflect.DeepEqual(global.TopSymbols[0].CPUs, []int{3}) {
		t.Fatalf("independently proved on-CPU coordinate was lost: %+v", global.TopSymbols[0])
	}
	if scoped := perfContextForThread(idx, Query{}, ThreadRef{PID: 777, Comm: "worker"}, 9.9, 10.1, 8); scoped != nil {
		t.Fatalf("source-only sample attached to a thread: %+v", scoped)
	}
	if byPID := BuildPerfTimeline(idx, Query{PID: 777, TimeStart: 9.9, TimeEnd: 10.1}); len(byPID.Buckets) != 0 {
		t.Fatalf("PID selector revived source-only sample: %+v", byPID)
	}
	globalTimeline := BuildPerfTimeline(idx, Query{TimeStart: 9.9, TimeEnd: 10.1})
	if len(globalTimeline.Buckets) != 1 || len(globalTimeline.Buckets[0].Threads) != 0 || !reflect.DeepEqual(globalTimeline.Buckets[0].CPUs, []int{3}) {
		t.Fatalf("global timeline source-only dimensions are dishonest: %+v", globalTimeline)
	}
	_, _, count, contributors, withheld := perfTimelineWindow(idx, Query{TimeStart: 9.9, TimeEnd: 10.1}, ensurePerfIdentityLedger(idx))
	if withheld {
		t.Fatal("source-only anonymous sample must not masquerade as a withheld identity selector")
	}
	if count != 1 || len(contributors) != 0 {
		t.Fatalf("source-only sample entered incarnation contributor set: count=%d contributors=%v", count, contributors)
	}

	encoded, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	var surface map[string]any
	if err := json.Unmarshal(encoded, &surface); err != nil {
		t.Fatal(err)
	}
	for _, unsafeKey := range []string{"comm", "pid", "tgid", "perf_pid", "perf_tid", "perf_comm"} {
		if _, exists := surface[unsafeKey]; exists {
			t.Fatalf("scrubbed identity key %q reappeared in JSON: %s", unsafeKey, encoded)
		}
	}
	for _, auditKey := range []string{"perf_thread_identity_known", "perf_resolution", "perf_lifecycle_unverified", "perf_source_pid", "perf_source_tid", "perf_source_comm", "perf_symbol", "perf_dso"} {
		if _, exists := surface[auditKey]; !exists {
			t.Fatalf("audit/inventory key %q missing from JSON: %s", auditKey, encoded)
		}
	}
	var round Event
	if err := json.Unmarshal(encoded, &round); err != nil {
		t.Fatal(err)
	}
	if !perfSampleIsSourceOnlyIdentity(round) || perfSampleHasTypedThreadIdentity(round) || round.PerfFields.SourceTID != 777 {
		t.Fatalf("JSON roundtrip changed source-only semantics: %+v", round)
	}
}

func TestPerfPatternHardNegativeDoesNotChangeResolvedOrNonPerfSearch(t *testing.T) {
	known, verified := true, false
	resolved := Event{
		Line: 91, Type: EventPerfSample, Comm: "resolved-worker", PID: 777, TGID: 666,
		FieldText: "raw-resolved-coordinate",
		PerfFields: &PerfFields{
			PID: 666, TID: 777, Comm: "resolved-worker",
			ThreadIdentityKnown: &known, LifecycleUnverified: &verified, Resolution: "resolved",
			Symbol: "Resolved::hot", DSO: "libresolved.so",
		},
	}
	for _, pattern := range []string{"resolved-worker", "777", "raw-resolved-coordinate"} {
		if !eventMatchesPattern(resolved, pattern) {
			t.Fatalf("resolved perf pattern %q regressed", pattern)
		}
	}

	nonPerf := Event{Line: 92, Type: EventSchedWakeup, Comm: "ordinary-worker", PID: 888, FieldText: "raw-non-perf-coordinate"}
	for _, pattern := range []string{"ordinary-worker", "888", "raw-non-perf-coordinate"} {
		if !eventMatchesPattern(nonPerf, pattern) {
			t.Fatalf("non-perf pattern %q regressed", pattern)
		}
	}
}

func TestPerfSourceOnlyLegacyWireAndDirectEventsAreHardNegative(t *testing.T) {
	intern := newStringInterner()
	legacy, ok := ParseLine(1, `legacy-worker-777 (666) [004] .... 10.000000: perf_sample: cpu=4 cpu_known=true pid=666 tid=777 thread_comm=legacy-worker sample_weight=7 event=cpu-cycles symbol=Legacy::hot dso=liblegacy.so source=trace_streamer_db sample_kind=on_cpu resolution=perf_source_only`, intern)
	if !ok || legacy.PerfFields == nil {
		t.Fatal("legacy source-only row should parse as inventory")
	}
	if legacy.PID != 0 || legacy.TGID != 0 || legacy.Comm != "" || legacy.PerfFields.PID != 0 || legacy.PerfFields.TID != 0 || legacy.PerfFields.Comm != "" {
		t.Fatalf("legacy source-only identity survived: %+v", legacy)
	}
	if !perfSampleIsSourceOnlyIdentity(legacy) || eventMentionsPID(legacy, 777) || eventMentionsThread(legacy, "legacy-worker") {
		t.Fatalf("legacy source-only token was not a hard negative: %+v", legacy)
	}

	known := true
	direct := Event{
		Type: EventPerfSample,
		Comm: "forged-header", PID: 777, TGID: 666, CPU: 4,
		FieldText: "raw-direct-coordinate",
		PerfFields: &PerfFields{
			PID: 666, TID: 777, Comm: "forged-body", Source: "trace_streamer_db",
			Resolution: perfSourceOnlyResolution, CPUKnown: &known, SampleKind: "on_cpu",
			SourcePID: 666, SourceTID: 777, SourceComm: "audit-worker",
			Symbol: "Direct::hot", DSO: "libdirect.so",
		},
	}
	if perfSampleHasTypedThreadIdentity(direct) || eventMentionsPID(direct, 777) || eventMentionsThread(direct, "forged-header") || perfSampleThread(direct) != (ThreadRef{}) {
		t.Fatal("a direct Event bypass resurrected legacy source-only identity")
	}
	ctx := computePerfContext(&Index{Events: []Event{direct}}, Query{}, 8)
	if ctx == nil || len(ctx.TopThreads) != 0 || len(ctx.TopSymbols) != 1 || len(ctx.TopSymbols[0].Threads) != 0 {
		t.Fatalf("direct source-only Event minted aggregate thread identity: %+v", ctx)
	}
	for _, pattern := range []string{"forged-header", "forged-body", "audit-worker", "777", "raw-direct-coordinate"} {
		if eventMatchesPattern(direct, pattern) {
			t.Fatalf("direct source-only Event exposed identity pattern %q", pattern)
		}
	}
	if !eventMatchesPattern(direct, "Direct::hot") || !eventMatchesPattern(direct, "libdirect.so") {
		t.Fatal("direct source-only Event lost safe symbol/DSO inventory")
	}
}

func TestPerfSourceOnlyHardGateUsesClosedBooleanToken(t *testing.T) {
	for _, raw := range []string{"true", "TRUE", "false", "FALSE"} {
		if _, ok := perfWireBool(raw); !ok {
			t.Fatalf("closed boolean %q rejected", raw)
		}
	}
	for _, raw := range []string{"", "0", "1", "unknown", "available", "no"} {
		if _, ok := perfWireBool(raw); ok {
			t.Fatalf("free-form token %q became a hard identity boolean", raw)
		}
	}

	intern := newStringInterner()
	ev, ok := ParseLine(1, `thirdparty-77 (66) [002] .... 1.000000: perf_sample: cpu=2 cpu_known=true pid=66 tid=77 thread_comm=thirdparty sample_weight=1 event=cpu-cycles symbol=work source=thirdparty sample_kind=on_cpu resolution=perf_source_only thread_identity_known=unknown`, intern)
	if !ok || ev.PerfFields == nil {
		t.Fatalf("malformed hard-gate inventory row was lost: %+v", ev)
	}
	if !perfSampleIsSourceOnlyIdentity(ev) || perfSampleHasTypedThreadIdentity(ev) ||
		ev.PID != 0 || ev.TGID != 0 || ev.Comm != "" ||
		ev.PerfFields.PID != 0 || ev.PerfFields.TID != 0 || ev.PerfFields.Comm != "" {
		t.Fatalf("malformed hard-gate scalar retained typed thread authority: %+v", ev)
	}
	if !eventMatchesPattern(ev, "work") {
		t.Fatal("hard identity withdrawal also erased safe symbol inventory")
	}
}

func TestPerfIndependentNegativeIdentityClaimsCannotBeRescued(t *testing.T) {
	intern := newStringInterner()
	rows := []struct {
		name string
		wire string
	}{
		{
			name: "identity false contradicts resolved",
			wire: "thread_identity_known=false lifecycle_unverified=false resolution=resolved",
		},
		{
			name: "lifecycle unverified contradicts identity known",
			wire: "thread_identity_known=true lifecycle_unverified=true resolution=resolved",
		},
		{
			name: "identity false with other claims missing",
			wire: "thread_identity_known=false",
		},
		{
			name: "lifecycle unverified with other claims missing",
			wire: "lifecycle_unverified=true",
		},
	}
	for i, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			line := `thirdparty-77 (66) [002] .... 1.000000: perf_sample: cpu=2 cpu_known=true pid=66 tid=77 thread_comm=thirdparty sample_weight=1 event=cpu-cycles symbol=work source=thirdparty sample_kind=on_cpu ` + tc.wire
			ev, ok := ParseLine(i+1, line, intern)
			if !ok || ev.PerfFields == nil {
				t.Fatal("negative-claim inventory row should parse")
			}
			if !perfSampleIsSourceOnlyIdentity(ev) || perfSampleHasTypedThreadIdentity(ev) || ev.PID != 0 || ev.TGID != 0 || ev.Comm != "" || ev.PerfFields.PID != 0 || ev.PerfFields.TID != 0 || ev.PerfFields.Comm != "" {
				t.Fatalf("independent negative claim was rescued: %+v", ev)
			}
			if !eventMatchesPattern(ev, "work") {
				t.Fatal("safe inventory was lost while scrubbing identity")
			}
		})
	}

	known, unknown := true, false
	verified, unverified := false, true
	direct := []Event{
		{
			Type: EventPerfSample, Comm: "direct", PID: 77, TGID: 66,
			PerfFields: &PerfFields{TID: 77, PID: 66, Comm: "direct", ThreadIdentityKnown: &unknown, LifecycleUnverified: &verified, Resolution: "resolved"},
		},
		{
			Type: EventPerfSample, Comm: "direct", PID: 77, TGID: 66,
			PerfFields: &PerfFields{TID: 77, PID: 66, Comm: "direct", ThreadIdentityKnown: &known, LifecycleUnverified: &unverified, Resolution: "resolved"},
		},
	}
	for i, ev := range direct {
		if !perfSampleIsSourceOnlyIdentity(ev) || perfSampleHasTypedThreadIdentity(ev) || eventMentionsPID(ev, 77) || eventMentionsThread(ev, "direct") || perfSampleThread(ev) != (ThreadRef{}) {
			t.Fatalf("direct contradictory Event %d resurrected identity: %+v", i, ev)
		}
	}
}

func TestPerfCPUClaimsRequireKnownAndOnCPUForConcreteAttribution(t *testing.T) {
	intern := newStringInterner()
	lines := []string{
		`on-10 (10) [000] .... 2.000000: perf_sample: cpu=0 cpu_known=true pid=10 tid=10 thread_comm=on sample_weight=4 event=cpu-cycles symbol=OnCPU source=fixture sample_kind=on_cpu sample_kind_source=scheduler_running`,
		`off-11 (11) [004] .... 2.000100: perf_sample: cpu=4 cpu_known=true pid=11 tid=11 thread_comm=off sample_weight=3 event=cpu-clock symbol=OffCPU source=fixture sample_kind=off_cpu`,
		`unknown-12 (12) [005] .... 2.000200: perf_sample: cpu=5 cpu_known=true pid=12 tid=12 thread_comm=unknown sample_weight=2 event=cpu-cycles symbol=UnknownCPU source=fixture sample_kind=unknown`,
		`false-13 (13) [007] .... 2.000300: perf_sample: cpu=7 cpu_known=false pid=13 tid=13 thread_comm=false sample_weight=1 event=cpu-cycles symbol=FalseCPU source=fixture sample_kind=on_cpu`,
		`upper-14 (14) [006] .... 2.000400: perf_sample: cpu=6 cpu_known=true pid=14 tid=14 thread_comm=upper sample_weight=1 event=cpu-cycles symbol=UpperCPU source=fixture sample_kind=ON_CPU`,
	}
	events := make([]Event, 0, len(lines))
	for i, line := range lines {
		ev, ok := ParseLine(i+1, line, intern)
		if !ok || ev.PerfFields == nil {
			t.Fatalf("line %d did not parse", i+1)
		}
		events = append(events, ev)
	}
	if events[3].CPU != -1 || perfSampleHasKnownCPU(events[3]) {
		t.Fatalf("cpu_known=false retained transport CPU7: %+v", events[3])
	}
	if !perfSampleHasKnownCPU(events[1]) || perfSampleIsOnCPU(events[1]) || !perfSampleHasKnownCPU(events[2]) || perfSampleIsOnCPU(events[2]) {
		t.Fatal("CPU identity and execution semantics were conflated")
	}
	if events[0].PerfFields.SampleKindSource != "scheduler_running" || !strings.Contains(perfSampleExample(events[0]), "sample_kind_source=scheduler_running") {
		t.Fatalf("sample-kind provenance was not preserved: %+v", events[0].PerfFields)
	}

	idx := &Index{Events: events}
	ctx := computePerfContext(idx, Query{TimeStart: 1.9, TimeEnd: 2.1}, 8)
	if ctx == nil || ctx.SampleCount != 5 || ctx.Quality == nil || ctx.Quality.CPUKnownCount != 4 || ctx.Quality.CPUUnknownCount != 1 {
		t.Fatalf("global perf inventory/quality was lost: %+v", ctx)
	}
	for _, symbol := range []string{"OffCPU", "UnknownCPU", "FalseCPU", "UpperCPU"} {
		hot := perfHotspotBySymbolForTest(t, ctx.TopSymbols, symbol)
		if len(hot.CPUs) != 0 {
			t.Fatalf("%s gained concrete CPU execution attribution: %+v", symbol, hot)
		}
	}
	if hot := perfHotspotBySymbolForTest(t, ctx.TopSymbols, "OnCPU"); !reflect.DeepEqual(hot.CPUs, []int{0}) {
		t.Fatalf("proved on-CPU sample lost legal CPU0: %+v", hot)
	}
	if got := perfContextForCPUs(idx, Query{}, map[int]bool{0: true}, 8); got == nil || got.SampleCount != 1 || got.TopSymbols[0].Symbol != "OnCPU" {
		t.Fatalf("CPU0 scoped join lost proved on-CPU sample: %+v", got)
	}
	if got := perfContextForCPU(idx, Query{}, 0, 1.9, 2.1, 8); got == nil || got.SampleCount != 1 || got.TopSymbols[0].Symbol != "OnCPU" {
		t.Fatalf("root-cause CPU0 drilldown lost proved on-CPU sample: %+v", got)
	}
	for _, cpu := range []int{4, 5, 6, 7} {
		if got := perfContextForCPUs(idx, Query{}, map[int]bool{cpu: true}, 8); got != nil {
			t.Fatalf("CPU%d scoped join admitted off/unknown/unproved sample: %+v", cpu, got)
		}
		if got := perfContextForCPU(idx, Query{}, cpu, 1.9, 2.1, 8); got != nil {
			t.Fatalf("root-cause CPU%d drilldown admitted off/unknown/unproved sample: %+v", cpu, got)
		}
	}
	timeline := BuildPerfTimeline(idx, Query{TimeStart: 1.9, TimeEnd: 2.1, MinDurationMs: 10})
	if len(timeline.Buckets) != 1 || !reflect.DeepEqual(timeline.Buckets[0].CPUs, []int{0}) {
		t.Fatalf("timeline CPU dimension treated off/unknown as execution: %+v", timeline)
	}
	joined := strings.Join(ctx.Quality.Caveats, "\n")
	if !strings.Contains(joined, "off_cpu") || !strings.Contains(joined, "unknown sample_kind") || !strings.Contains(joined, "CPU-unknown") {
		t.Fatalf("global quality disclosure lost semantic caveats: %s", joined)
	}
}

func perfHotspotBySymbolForTest(t *testing.T, items []PerfHotspot, symbol string) PerfHotspot {
	t.Helper()
	for _, item := range items {
		if item.Symbol == symbol {
			return item
		}
	}
	t.Fatalf("symbol %q missing from %+v", symbol, items)
	return PerfHotspot{}
}
