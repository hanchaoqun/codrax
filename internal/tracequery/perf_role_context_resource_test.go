package tracequery

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestPerfRoleContextRootAndFrameShareOneFullScanAndFreshPublications(t *testing.T) {
	events := perfRoleResourceEvents(t,
		`target-10 (10) [000] .... 3.001000: perf_sample: cpu=0 cpu_known=true pid=10 tid=10 thread_comm=target sample_weight=10 event=cpu-cycles symbol=TargetOn source=fixture sample_kind=on_cpu`,
		`target-10 (10) [000] .... 3.002000: perf_sample: cpu=0 cpu_known=true pid=10 tid=10 thread_comm=target sample_weight=20 event=cpu-clock symbol=TargetOff source=fixture sample_kind=off_cpu`,
	)
	idx := &Index{Events: events}
	probe := &perfRoleContextScanProbe{}
	q := Query{TimeStart: 3, TimeEnd: 3.1, Limit: 1 << 20, perfRoleContextScanProbe: probe}
	stats := WindowStats{PerfSamples: &PerfContext{SampleCount: len(events)}}
	target := ThreadRef{Comm: "target", PID: 10, TGID: 10}
	rank := RootCauseRankResult{Items: make([]RootCauseRankItem, ViewCapacityFor("root_cause_rank").MaxLimit)}
	for i := range rank.Items {
		rank.Items[i] = RootCauseRankItem{Type: "running", Thread: target, StartTs: 3, EndTs: 3.1}
	}
	roleIndex := newPerfRoleContextIndex(idx, q)
	rank = attachPerfContextToRootCauseRankWithIndex(idx, q, rank, stats, roleIndex)
	frame := buildFramePerfContextsWithIndex(idx, q, stats, nil, CriticalBlockingResult{}, target, roleIndex)
	if probe.FullEventScans != 1 || probe.FullEventsVisited != len(events) {
		t.Fatalf("root(12 seats)+frame role index scans = %d/%d, want one full pass over %d events", probe.FullEventScans, probe.FullEventsVisited, len(events))
	}
	if probe.RoleLookups > ViewCapacityFor("root_cause_rank").MaxLimit*2+1 {
		t.Fatalf("default 12-seat board role lookups = %d, want mechanical <= %d", probe.RoleLookups, ViewCapacityFor("root_cause_rank").MaxLimit*2+1)
	}
	if len(rank.Items) < 2 || rank.Items[0].PerfContext == nil || rank.Items[1].PerfContext == nil || frame.TargetRunningPerf == nil {
		t.Fatalf("indexed role publications missing: rank=%+v frame=%+v", rank.Items, frame)
	}
	if rank.Items[0].PerfContext == rank.Items[1].PerfContext || rank.Items[0].PerfContext == frame.TargetRunningPerf {
		t.Fatal("distinct root/frame publications shared a mutable *PerfContext")
	}
	if rank.Items[0].PerfContext.ThreadIdentityCountExact == rank.Items[1].PerfContext.ThreadIdentityCountExact {
		t.Fatal("distinct role publications shared the mutable identity-exact witness pointer")
	}
}

func TestPerfRoleContextNoPerfSkipsFullScan(t *testing.T) {
	probe := &perfRoleContextScanProbe{}
	q := Query{perfRoleContextScanProbe: probe}
	rank := RootCauseRankResult{Items: []RootCauseRankItem{{Type: "running", Thread: ThreadRef{PID: 10}}}}
	got := attachPerfContextToRootCauseRank(&Index{Events: make([]Event, 1000)}, q, rank, WindowStats{})
	if len(got.Items) != 1 || probe.FullEventScans != 0 || probe.FullEventsVisited != 0 {
		t.Fatalf("no-perf rank should skip role index: rank=%+v probe=%+v", got, probe)
	}
	frame := buildFramePerfContexts(&Index{Events: make([]Event, 1000)}, q, WindowStats{}, nil, CriticalBlockingResult{}, ThreadRef{PID: 10})
	if frame.PerfSamples != nil || probe.FullEventScans != 0 || probe.FullEventsVisited != 0 {
		t.Fatalf("no-perf frame should skip role index: frame=%+v probe=%+v", frame, probe)
	}
}

func TestPerfRoleLifecycleReplayTracksOnlyPerfCandidateTIDs(t *testing.T) {
	events := perfRoleResourceEvents(t,
		`target-10 (10) [000] .... 3.001000: perf_sample: cpu=0 cpu_known=true pid=10 tid=10 thread_comm=target sample_weight=10 event=cpu-cycles symbol=TargetOn source=fixture sample_kind=on_cpu`,
	)
	for i := 0; i < 10000; i++ {
		pid := 100000 + i
		events = append(events, Event{Type: EventSchedSwitch, PID: pid, PrevPID: pid, PrevState: "X", NextPID: pid + 1, Line: i + 2, Ts: 4 + float64(i)/1000000})
	}
	probe := &perfRoleContextScanProbe{}
	q := Query{TimeStart: 3, TimeEnd: 20, perfRoleContextScanProbe: probe}
	roleIndex := newPerfRoleContextIndex(&Index{Events: events}, q)
	if !roleIndex.complete || probe.LifecycleCandidateTIDs != 1 || len(roleIndex.conflicts) != 0 {
		t.Fatalf("unrelated scheduler roster entered perf lifecycle state: candidates=%d conflicts=%d complete=%t", probe.LifecycleCandidateTIDs, len(roleIndex.conflicts), roleIndex.complete)
	}
}

func TestPerfRoleIndexCommonAliasBuildIsLinearShape(t *testing.T) {
	const count = 32000
	ledger := &perfIdentityLedger{
		records:  make([]perfThreadIdentityRecord, count),
		bindings: make([]perfIdentityOrdinalBinding, count),
	}
	events := make([]Event, count)
	for i := 0; i < count; i++ {
		tid := i + 1
		key := perfThreadKey{TID: tid, Generation: 1}
		ledger.records[i] = perfThreadIdentityRecord{key: key, identity: PerfThreadIdentity{TID: tid, TGID: tid, Generation: 1, DisplayComm: "worker"}, selectorAliases: []string{"worker"}}
		ledger.bindings[i] = perfIdentityOrdinalBinding{ordinal: i, record: i}
		events[i] = Event{Type: EventPerfSample, PID: tid, Line: i + 1, Ts: 1 + float64(i)/1000000, PerfFields: &PerfFields{TID: tid, PID: tid, Comm: "worker"}}
	}
	idx := &Index{Events: events}
	idx.perfIdentityOnce.Do(func() { idx.perfIdentity = ledger })
	roleIndex := newPerfRoleContextIndex(idx, Query{})
	if !roleIndex.complete || len(roleIndex.keysByAlias["worker"]) != count {
		t.Fatalf("common-alias index build lost keys or failed to complete: complete=%t keys=%d want=%d", roleIndex.complete, len(roleIndex.keysByAlias["worker"]), count)
	}
}

func TestPerfRoleIndexPreCanceledDoesNotBuildLazyLedgerOrScan(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	probe := &perfRoleContextScanProbe{}
	q := Query{perfRoleContextScanProbe: probe}.WithRunContext(ctx)
	idx := &Index{Events: perfRoleResourceEvents(t,
		`target-10 (10) [000] .... 3.001000: perf_sample: cpu=0 cpu_known=true pid=10 tid=10 thread_comm=target sample_weight=10 event=cpu-cycles symbol=TargetOn source=fixture sample_kind=on_cpu`,
	)}
	roleIndex := newPerfRoleContextIndex(idx, q)
	if roleIndex.complete || idx.perfIdentity != nil || probe.FullEventScans != 0 {
		t.Fatalf("pre-canceled role index did work: complete=%t ledger=%p probe=%+v", roleIndex.complete, idx.perfIdentity, probe)
	}
}

func TestIndexedRoleAndCPUCaveatsDoNotRepublishGlobalLedgerCaveats(t *testing.T) {
	events := perfRoleResourceEvents(t,
		`target-10 (10) [000] .... 3.001000: perf_sample: cpu=0 cpu_known=true pid=10 tid=10 thread_comm=target sample_weight=10 event=cpu-cycles symbol=TargetOn source=fixture sample_kind=on_cpu`,
	)
	idx := &Index{Events: events}
	ledger := ensurePerfIdentityLedger(idx)
	ledger.caveatV1 = append(ledger.caveatV1, "global-ledger-caveat")
	roleIndex := newPerfRoleContextIndex(idx, Query{TimeStart: 3, TimeEnd: 3.1})
	for name, ctx := range map[string]*PerfContext{
		"role": roleIndex.contextForThread(ThreadRef{PID: 10}, 3, 3.1, 4, false),
		"cpu":  roleIndex.contextForCPU(0, 3, 3.1, 4),
	} {
		if ctx == nil || strings.Contains(strings.Join(ctx.Caveats, "\n"), "global-ledger-caveat") {
			t.Fatalf("%s indexed projection copied global ledger caveats: %+v", name, ctx)
		}
	}
}

func perfRoleResourceEvents(t *testing.T, lines ...string) []Event {
	t.Helper()
	intern := newStringInterner()
	events := make([]Event, 0, len(lines))
	for i, line := range lines {
		ev, ok := ParseLine(i+1, line, intern)
		if !ok || ev.PerfFields == nil {
			t.Fatalf("fixture %d did not parse as perf sample: %s", i, line)
		}
		events = append(events, ev)
	}
	return events
}

func BenchmarkPerfRoleIndexCommonAlias(b *testing.B) {
	for _, count := range []int{1000, 32000} {
		b.Run(fmt.Sprintf("records_%d", count), func(b *testing.B) {
			ledger := &perfIdentityLedger{records: make([]perfThreadIdentityRecord, count), bindings: make([]perfIdentityOrdinalBinding, count)}
			events := make([]Event, count)
			for i := 0; i < count; i++ {
				tid := i + 1
				key := perfThreadKey{TID: tid, Generation: 1}
				ledger.records[i] = perfThreadIdentityRecord{key: key, identity: PerfThreadIdentity{TID: tid, Generation: 1}, selectorAliases: []string{"worker"}}
				ledger.bindings[i] = perfIdentityOrdinalBinding{ordinal: i, record: i}
				events[i] = Event{Type: EventPerfSample, PID: tid, PerfFields: &PerfFields{TID: tid}}
			}
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				idx := &Index{Events: events}
				idx.perfIdentityOnce.Do(func() { idx.perfIdentity = ledger })
				_ = newPerfRoleContextIndex(idx, Query{})
			}
		})
	}
}
