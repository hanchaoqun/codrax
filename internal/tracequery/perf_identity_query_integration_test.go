package tracequery

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPerfEventSearchPatternCannotDisambiguateReusedTIDGeneration(t *testing.T) {
	events := []Event{perfIdentityQuerySample(1, 0.95, 100, 10, "old-name", "Old::work")}
	events = append(events, perfIdentityQueryReuse(2, 1.0, 100, "old-name", "new-name")...)
	events = append(events, perfIdentityQuerySample(4, 1.10, 100, 20, "new-name", "New::work"))
	idx := &Index{Events: events, FirstTs: 0.95, LastTs: 1.1, TimestampOrder: TraceTimestampOrderMonotonic}
	q := Query{
		View: "event_search", PID: 100, Pattern: "New::work",
		EventTypes: []EventType{EventPerfSample}, TimeStart: 0.9, TimeEnd: 1.2, Limit: 8,
	}
	got, caveat := eventSearchIndexed(idx, q)
	if len(got) != 0 {
		t.Fatalf("symbol pattern accidentally selected one reused-TID generation: %+v", got)
	}
	if !strings.Contains(caveat, "perf_thread_generation_fail_closed=true") || !strings.Contains(caveat, "perf_event_search_rows_withheld=true") {
		t.Fatalf("reused-TID event_search withdrawal lacked typed caveat: %q", caveat)
	}
}

func TestPerfEventSearchPatternCannotDisambiguateSameCommAcrossTIDs(t *testing.T) {
	idx := &Index{
		Events: []Event{
			perfIdentityQuerySample(1, 1.0, 7, 70, "worker", "OnlyA"),
			perfIdentityQuerySample(2, 1.1, 8, 80, "worker", "OnlyB"),
		},
		FirstTs: 1, LastTs: 1.1, TimestampOrder: TraceTimestampOrderMonotonic,
	}
	q := Query{
		View: "event_search", Thread: "worker", Pattern: "OnlyA",
		EventTypes: []EventType{EventPerfSample}, TimeStart: 0.9, TimeEnd: 1.2, Limit: 8,
	}
	got, caveat := eventSearchIndexed(idx, q)
	if len(got) != 0 {
		t.Fatalf("symbol pattern accidentally selected one of two same-comm TIDs: %+v", got)
	}
	if !strings.Contains(caveat, "perf_thread_selector_ambiguous=true") || !strings.Contains(caveat, "perf_event_search_rows_withheld=true") {
		t.Fatalf("same-comm event_search withdrawal lacked typed caveat: %q", caveat)
	}
}

func TestPerfEventSearchPreCanceledContextDoesNotBuildSyntheticLedger(t *testing.T) {
	idx := &Index{Events: []Event{perfIdentityQuerySample(1, 1, 7, 70, "worker", "Hot")}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, caveat := eventSearchIndexed(idx, (Query{
		View: "event_search", PID: 7, EventTypes: []EventType{EventPerfSample}, Limit: 8,
	}).WithRunContext(ctx))
	if len(got) != 0 || caveat != "" {
		t.Fatalf("pre-canceled event search published partial output: events=%+v caveat=%q", got, caveat)
	}
	if idx.perfIdentity != nil {
		t.Fatalf("pre-canceled event search built a lazy identity ledger: %+v", idx.perfIdentity)
	}
}

func TestStreamEventSearchMixedIdentityQueryWithholdsOnlyPerfRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mixed-perf-stream.ftrace")
	body := strings.Join([]string{
		"worker-7 (70) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=7 next_prio=20",
		"worker-7 (70) [000] .... 1.010000: perf_sample: cpu=0 cpu_known=true pid=70 tid=7 thread_comm=worker sample_weight=1 event=cpu-cycles symbol=Hot source=fixture sample_kind=on_cpu",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := StreamEventSearch(context.Background(), path, Query{
		View: "event_search", PID: 7, TimeStart: 0.9, TimeEnd: 1.1, Limit: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Events) != 1 || res.Events[0].Type != EventSchedSwitch {
		t.Fatalf("mixed identity discovery either left streaming or raw-matched perf identity: events=%+v caveats=%v", res.Events, res.Caveats)
	}
	if !containsSubstring(res.Caveats, "perf_thread_selector_withheld=true") || !containsSubstring(res.Caveats, "perf_rows_withheld=true") {
		t.Fatalf("stream-local perf withdrawal was not disclosed: %v", res.Caveats)
	}
	if !containsSubstring(res.Caveats, "streamed_event_search=true") {
		t.Fatalf("mixed query unexpectedly materialized a full index: %v", res.Caveats)
	}

	// An explicit perf_sample query opts into the indexed generation authority
	// and may publish the same row only after typed ledger resolution.
	explicit, err := StreamEventSearch(context.Background(), path, Query{
		View: "event_search", PID: 7, EventTypes: []EventType{EventPerfSample},
		TimeStart: 0.9, TimeEnd: 1.1, Limit: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(explicit.Events) != 1 || explicit.Events[0].Type != EventPerfSample || containsSubstring(explicit.Caveats, "streamed_event_search=true") {
		t.Fatalf("explicit perf query did not use typed indexed authority: events=%+v caveats=%v", explicit.Events, explicit.Caveats)
	}
}

func perfIdentityQuerySample(line int, ts float64, tid, tgid int, comm, symbol string) Event {
	known, verified, cpuKnown := true, false, true
	return Event{
		Line: line, Ts: ts, Type: EventPerfSample, PID: tid, TGID: tgid, Comm: comm, CPU: 0,
		PerfFields: &PerfFields{
			TID: tid, PID: tgid, Comm: comm, Symbol: symbol, Period: 10,
			ThreadIdentityKnown: &known, LifecycleUnverified: &verified,
			CPUKnown: &cpuKnown, SampleKind: "on_cpu",
		},
	}
}

func perfIdentityQueryReuse(line int, ts float64, tid int, oldComm, newComm string) []Event {
	return []Event{
		{Line: line, Ts: ts, Type: EventSchedSwitch, CPU: 0, PrevPID: tid, PrevComm: oldComm, PrevState: "X", NextPID: 0, NextComm: "idle"},
		{Line: line + 1, Ts: ts + 0.01, Type: EventSchedWakeup, Name: "sched_wakeup_new", PID: 7, Comm: "creator", WakeePID: tid, WakeeComm: newComm},
	}
}

func TestPerfWindowStatsSurvivesUnrelatedSchedulerIdentityFailure(t *testing.T) {
	events := append(perfIdentityQueryReuse(1, 1.0, 900, "old", "new"), perfIdentityQuerySample(3, 1.1, 100, 10, "target", "Target::work"))
	idx := &Index{Events: events, FirstTs: 1, LastTs: 1.1, TimestampOrder: TraceTimestampOrderMonotonic}
	stats := ComputeWindowStats(idx, Query{TimeStart: 0.9, TimeEnd: 1.2})
	if !containsSubstring(stats.Caveats, "thread_identity_fail_closed=true") {
		t.Fatalf("control: unrelated scheduler reuse must close scheduler PID aggregates: %v", stats.Caveats)
	}
	if stats.PerfSamples == nil || stats.PerfSamples.SampleCount != 1 || len(stats.PerfSamples.TopSymbols) == 0 || stats.PerfSamples.TopSymbols[0].Symbol != "Target::work" {
		t.Fatalf("independent perf inventory was erased by an unrelated scheduler identity failure: %+v", stats.PerfSamples)
	}
}

func TestPerfGlobalAggregatesDiscloseAnonymousIdentityCoverage(t *testing.T) {
	typed := perfIdentityQuerySample(1, 1.0, 7, 70, "worker", "Shared::hot")
	known, unverified, cpuKnown := false, true, true
	anonymous := Event{
		Line: 2, Ts: 1.1, Type: EventPerfSample, CPU: 1,
		PerfFields: &PerfFields{
			Symbol: "Shared::hot", EventName: "cpu-cycles", Period: 10,
			ThreadIdentityKnown: &known, LifecycleUnverified: &unverified, CPUKnown: &cpuKnown,
			Resolution: perfSourceOnlyResolution, Source: "trace_streamer_db", SampleKind: "on_cpu",
		},
	}
	idx := &Index{Events: []Event{typed, anonymous}, FirstTs: 1, LastTs: 1.1, TimestampOrder: TraceTimestampOrderMonotonic}
	q := Query{TimeStart: 0.9, TimeEnd: 1.2, MinDurationMs: 1000}
	ctx := computePerfContext(idx, q, 8)
	if ctx == nil || ctx.SampleCount != 2 || ctx.ThreadIdentityCount != 1 || ctx.ThreadIdentityUnknownSampleCount != 1 {
		t.Fatalf("global context identity coverage ledger drifted: %+v", ctx)
	}
	if ctx.ThreadIdentityCountExact == nil || *ctx.ThreadIdentityCountExact {
		t.Fatalf("mixed typed/anonymous context claimed exact identity coverage: %+v", ctx)
	}
	if ctx.CohortCount != 2 || len(ctx.Cohorts) != 2 || len(ctx.TopSymbols) != 0 {
		t.Fatalf("mixed event cohorts leaked a legacy weighted projection: %+v", ctx)
	}
	var typedHotspot, anonymousHotspot *PerfHotspot
	for i := range ctx.Cohorts {
		cohort := &ctx.Cohorts[i]
		if len(cohort.TopSymbols) != 1 {
			t.Fatalf("cohort hotspot inventory drifted: %+v", cohort)
		}
		switch cohort.Event {
		case "unknown":
			typedHotspot = &cohort.TopSymbols[0]
		case "cpu-cycles":
			anonymousHotspot = &cohort.TopSymbols[0]
		}
	}
	if typedHotspot == nil || typedHotspot.ThreadIdentityCount != 1 || typedHotspot.ThreadIdentityUnknownSampleCount != 0 || typedHotspot.ThreadIdentityCountExact == nil || !*typedHotspot.ThreadIdentityCountExact {
		t.Fatalf("typed cohort identity coverage ledger drifted: %+v", typedHotspot)
	}
	if anonymousHotspot == nil || anonymousHotspot.ThreadIdentityCount != 0 || anonymousHotspot.ThreadIdentityUnknownSampleCount != 1 || anonymousHotspot.ThreadIdentityCountExact == nil || *anonymousHotspot.ThreadIdentityCountExact {
		t.Fatalf("anonymous cohort identity coverage ledger drifted: %+v", anonymousHotspot)
	}
	timeline := BuildPerfTimeline(idx, q)
	if len(timeline.Buckets) != 1 || timeline.Buckets[0].ThreadIdentityCount != 1 || timeline.Buckets[0].ThreadIdentityUnknownSampleCount != 1 {
		t.Fatalf("timeline identity coverage ledger drifted: %+v", timeline.Buckets)
	}
	if timeline.Buckets[0].ThreadIdentityCountExact == nil || *timeline.Buckets[0].ThreadIdentityCountExact {
		t.Fatalf("mixed typed/anonymous timeline claimed exact identity coverage: %+v", timeline.Buckets)
	}
	facts := evidenceFromPerfContext(ctx)
	var disclosedAnonymousCoverage bool
	for _, fact := range facts {
		if strings.Contains(fact.Summary, "thread_identity_unknown_samples=1") && strings.Contains(fact.Summary, "thread_identity_count_exact=false") {
			disclosedAnonymousCoverage = true
			break
		}
	}
	if !disclosedAnonymousCoverage {
		t.Fatalf("model-facing evidence hid anonymous identity coverage: %+v", facts)
	}
}

func TestPerfIdentityCoverageTriStateReachesModelFacingConsumers(t *testing.T) {
	identity := PerfThreadIdentity{TID: 77, Generation: 2, DisplayComm: "worker"}
	for _, tc := range []struct {
		name  string
		exact *bool
	}{
		{name: "legacy_absent", exact: nil},
		{name: "explicit_incomplete", exact: perfThreadIdentityCountExactPtr(false)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hotspot := PerfHotspot{
				Symbol: "Worker::hot", SampleCount: 1, Period: 10,
				ThreadIdentityCount: 1, ThreadIdentityCountExact: tc.exact,
				ThreadIdentities: []PerfThreadIdentity{identity},
			}
			ctx := &PerfContext{
				SampleCount: 1, TotalPeriod: 10,
				ThreadIdentityCount: 1, ThreadIdentityCountExact: tc.exact,
				TopSymbols: []PerfHotspot{hotspot},
			}
			bucket := PerfTimelineBucket{
				StartTs: 1, EndTs: 2, SampleCount: 1, Period: 10,
				TopSymbol: "Worker::hot", ThreadIdentityCount: 1,
				ThreadIdentityCountExact: tc.exact, ThreadIdentities: []PerfThreadIdentity{identity},
			}
			for name, got := range map[string]string{
				"window_stats":  evidenceFromStats(WindowStats{PerfSamples: ctx})[0].Summary,
				"perf_context":  evidenceFromPerfContext(ctx)[0].Summary,
				"perf_timeline": evidenceFromPerfTimeline(PerfTimelineResult{Buckets: []PerfTimelineBucket{bucket}})[0].Summary,
				"root":          rootCausePerfSummary(ctx),
				"root_role":     rootCausePerfRoleSummary([]RootCausePerfRoleContext{{Role: "candidate", PerfContext: ctx}}, 1),
			} {
				if !strings.Contains(got, "thread_identity_count_exact=false") {
					t.Fatalf("%s hid %s identity coverage state: %q", name, tc.name, got)
				}
			}
		})
	}
}

func TestPerfIdentityCoverageWitnessesAreQueryLocal(t *testing.T) {
	first := perfThreadIdentityCountExactPtr(true)
	second := perfThreadIdentityCountExactPtr(true)
	if first == second {
		t.Fatal("exported identity coverage witnesses share mutable storage")
	}
	*first = false
	if !*second {
		t.Fatal("mutating one result corrupted an independent result")
	}
	third := perfThreadIdentityCountExactPtr(true)
	if !*third || third == first || third == second {
		t.Fatal("a mutated historical witness contaminated a future result")
	}
}

func TestPerfRoleContextDoesNotMergeReusedTIDGenerations(t *testing.T) {
	events := []Event{perfIdentityQuerySample(1, 0.95, 100, 10, "old-name", "Old::work")}
	events = append(events, perfIdentityQueryReuse(2, 1.0, 100, "old-name", "new-name")...)
	events = append(events, perfIdentityQuerySample(4, 1.10, 100, 20, "new-name", "New::work"))
	idx := &Index{Events: events, FirstTs: 0.95, LastTs: 1.1, TimestampOrder: TraceTimestampOrderMonotonic}
	q := Query{TimeStart: 0.9, TimeEnd: 1.2}

	global := computePerfContext(idx, q, 8)
	if global == nil || global.SampleCount != 2 || len(global.TopThreads) != 2 {
		t.Fatalf("global inventory must retain both generations as separate typed seats: %+v", global)
	}
	if global.TopThreads[0].Identity == nil || global.TopThreads[1].Identity == nil || global.TopThreads[0].Identity.Generation == global.TopThreads[1].Identity.Generation {
		t.Fatalf("reused TID generations were not separated: %+v", global.TopThreads)
	}
	if got := perfContextForThread(idx, q, ThreadRef{PID: 100}, q.TimeStart, q.TimeEnd, 8); got != nil {
		t.Fatalf("single-thread role crossed a TID generation boundary: %+v", got)
	}
	if got := perfContextForExecutionThread(idx, q, ThreadRef{PID: 100}, q.TimeStart, q.TimeEnd, 8); got != nil {
		t.Fatalf("execution role crossed a TID generation boundary: %+v", got)
	}
}

func TestPerfRoleContextFailsClosedAcrossBoundaryWithOneSampledGeneration(t *testing.T) {
	events := perfIdentityQueryReuse(1, 1.0, 100, "old-name", "new-name")
	events = append(events, perfIdentityQuerySample(3, 1.10, 100, 20, "new-name", "New::work"))
	idx := &Index{Events: events, FirstTs: 1, LastTs: 1.1, TimestampOrder: TraceTimestampOrderMonotonic}
	q := Query{TimeStart: 0.9, TimeEnd: 1.2}

	if control := computePerfContext(idx, q, 8); control == nil || control.SampleCount != 1 {
		t.Fatalf("control: global inventory must retain the sampled new generation: %+v", control)
	}
	if got := perfContextForThread(idx, q, ThreadRef{PID: 100}, q.TimeStart, q.TimeEnd, 8); got != nil {
		t.Fatalf("TID role published across a lifecycle boundary merely because the old generation had no perf sample: %+v", got)
	}
}

func TestPerfRoleContextIgnoresUnrelatedReuseAndPreservesRenameAliases(t *testing.T) {
	events := append(perfIdentityQueryReuse(1, 1.0, 900, "old", "new"),
		perfIdentityQuerySample(3, 1.10, 100, 10, "render-old", "A"),
		perfIdentityQuerySample(4, 1.11, 100, 10, "render-new", "B"),
	)
	idx := &Index{Events: events, FirstTs: 1, LastTs: 1.11, TimestampOrder: TraceTimestampOrderMonotonic}
	q := Query{TimeStart: 0.9, TimeEnd: 1.2}

	ctx := perfContextForThread(idx, q, ThreadRef{PID: 100}, q.TimeStart, q.TimeEnd, 8)
	if ctx == nil || ctx.SampleCount != 2 || len(ctx.TopThreads) != 1 || ctx.TopThreads[0].Identity == nil {
		t.Fatalf("unrelated reuse or same-generation rename erased the target context: %+v", ctx)
	}
	id := ctx.TopThreads[0].Identity
	if id.TID != 100 || id.CommAliasCount != 2 || len(id.CommAliases) != 2 || id.DisplayComm != "render-new" {
		t.Fatalf("rename aliases/display were not derived from the typed generation: %+v", id)
	}
}

func TestPerfSelectorsUsePrivateFullAliasAuthorityBeyondPublicProjection(t *testing.T) {
	events := make([]Event, 0, 10)
	for i := 1; i <= 10; i++ {
		events = append(events, perfIdentityQuerySample(i, 1+float64(i)/1000, 100, 10, fmt.Sprintf("alias%02d", i), "Work"))
	}
	idx := &Index{Events: events, FirstTs: 1.001, LastTs: 1.010, TimestampOrder: TraceTimestampOrderMonotonic}
	q := Query{TimeStart: 1, TimeEnd: 1.02, MinDurationMs: 1000}
	global := computePerfContext(idx, q, 8)
	if global == nil || len(global.TopThreads) != 1 || global.TopThreads[0].Identity == nil {
		t.Fatalf("control: expected one renamed typed cohort: %+v", global)
	}
	for _, alias := range global.TopThreads[0].Identity.CommAliases {
		if alias == "alias09" {
			t.Fatalf("control: alias09 must be outside the public cap=8 projection: %+v", global.TopThreads[0].Identity)
		}
	}
	role := perfContextForThread(idx, q, ThreadRef{Comm: "alias09"}, q.TimeStart, q.TimeEnd, 8)
	if role == nil || role.SampleCount != 10 {
		t.Fatalf("role selector ignored a valid alias omitted from the public projection: %+v", role)
	}
	timeline := BuildPerfTimeline(idx, Query{Thread: "alias09", TimeStart: 1, TimeEnd: 1.02, MinDurationMs: 1000})
	if len(timeline.Buckets) != 1 || timeline.Buckets[0].SampleCount != 10 {
		t.Fatalf("timeline selector ignored a valid alias omitted from the public projection: %+v", timeline)
	}
}

func TestPerfTimelineGlobalKeepsTypedGenerationsButPIDRoleFailsClosed(t *testing.T) {
	events := []Event{perfIdentityQuerySample(1, 0.95, 100, 10, "old-name", "Old::work")}
	events = append(events, perfIdentityQueryReuse(2, 1.0, 100, "old-name", "new-name")...)
	events = append(events, perfIdentityQuerySample(4, 1.10, 100, 20, "new-name", "New::work"))
	idx := &Index{Events: events, FirstTs: 0.95, LastTs: 1.1, TimestampOrder: TraceTimestampOrderMonotonic}
	q := Query{TimeStart: 0.9, TimeEnd: 1.2, MinDurationMs: 1000}

	global := BuildPerfTimeline(idx, q)
	if len(global.Buckets) != 1 || len(global.Buckets[0].ThreadIdentities) != 2 || len(global.Buckets[0].Threads) != 2 {
		t.Fatalf("global timeline must retain both generations in its typed roster: %+v", global)
	}
	pidView := BuildPerfTimeline(idx, Query{PID: 100, TimeStart: 0.9, TimeEnd: 1.2, MinDurationMs: 1000})
	if len(pidView.Buckets) != 0 || (!containsSubstring(pidView.Caveats, "thread_identity_fail_closed=true") && !containsSubstring(pidView.Caveats, "perf_thread_generation_fail_closed=true")) {
		t.Fatalf("PID-addressed timeline crossed the selected TID generation boundary: %+v", pidView)
	}
}

func TestPerfTimelineThreadSelectorCannotMatchSymbolOrDSOText(t *testing.T) {
	ev := perfIdentityQuerySample(1, 1.0, 100, 10, "actual-worker", "render")
	ev.PerfFields.DSO = "render.so"
	idx := &Index{Events: []Event{ev}, FirstTs: 1, LastTs: 1, TimestampOrder: TraceTimestampOrderMonotonic}

	res := BuildPerfTimeline(idx, Query{Thread: "render", TimeStart: 0.9, TimeEnd: 1.1})
	if len(res.Buckets) != 0 {
		t.Fatalf("thread selector was widened by perf symbol/DSO text: %+v", res)
	}
	control := BuildPerfTimeline(idx, Query{Thread: "actual-worker", TimeStart: 0.9, TimeEnd: 1.1})
	if len(control.Buckets) != 1 {
		t.Fatalf("typed display/alias selector must still match the actual perf thread: %+v", control)
	}
}

func TestPerfTypedGenerationRosterReachesEvidenceAndRootSummaries(t *testing.T) {
	events := []Event{
		perfIdentityQuerySample(1, 1.0, 100, 10, "render-old", "Render::draw"),
		perfIdentityQuerySample(2, 1.1, 100, 10, "render-new", "Render::draw"),
	}
	idx := &Index{Events: events, FirstTs: 1, LastTs: 1.1, TimestampOrder: TraceTimestampOrderMonotonic}
	q := Query{TimeStart: 0.9, TimeEnd: 1.2, MinDurationMs: 1000}
	ctx := computePerfContext(idx, q, 8)
	want := "render-new-100@g1"
	if ctx == nil || !strings.Contains(rootCausePerfSummary(ctx), want) {
		t.Fatalf("root perf summary omitted typed generation roster: ctx=%+v summary=%q", ctx, rootCausePerfSummary(ctx))
	}
	roles := rootCausePerfRoleSummary([]RootCausePerfRoleContext{{Role: "candidate_thread", Thread: ThreadRef{PID: 100}, PerfContext: ctx}}, 1)
	if !strings.Contains(roles, want) {
		t.Fatalf("role perf summary omitted typed generation roster: %q", roles)
	}
	assertEvidenceRoster := func(name string, facts []EvidenceFact) {
		t.Helper()
		for _, fact := range facts {
			if strings.Contains(fact.Summary, want) {
				return
			}
		}
		t.Fatalf("%s evidence omitted typed generation roster: %+v", name, facts)
	}
	assertEvidenceRoster("perf_context", evidenceFromPerfContext(ctx))
	stats := ComputeWindowStats(idx, q)
	assertEvidenceRoster("window_stats", evidenceFromStats(stats))
	assertEvidenceRoster("perf_timeline", evidenceFromPerfTimeline(BuildPerfTimeline(idx, q)))
}

func TestPerfTypedGenerationRosterIsBoundedAndSanitized(t *testing.T) {
	identities := make([]PerfThreadIdentity, 0, 6)
	for i := 1; i <= 6; i++ {
		identities = append(identities, PerfThreadIdentity{TID: i, Generation: i, DisplayComm: "bad, name|with=separators"})
	}
	roster := perfThreadIdentityRoster(identities, len(identities), 4)
	if strings.ContainsAny(roster, " |;=") || !strings.Contains(roster, "+2_more") || strings.Count(roster, "@g") != 4 {
		t.Fatalf("typed generation roster is unbounded or delimiter-unsafe: %q", roster)
	}
}

func TestPerfPublishedRostersAreBoundedWithExactTotalsUnderTie(t *testing.T) {
	events := make([]Event, 0, 100)
	for tid := 1; tid <= 100; tid++ {
		events = append(events, perfIdentityQuerySample(tid, 1+float64(tid)/10000, tid, 1000, fmt.Sprintf("worker%03d", tid), "Shared::hot"))
	}
	idx := &Index{Events: events, FirstTs: 1.0001, LastTs: 1.01, TimestampOrder: TraceTimestampOrderMonotonic}
	q := Query{TimeStart: 1, TimeEnd: 1.02, MinDurationMs: 1000}
	ctx := computePerfContext(idx, q, 4)
	if ctx == nil || ctx.ThreadIdentityCount != 100 || len(ctx.TopThreads) != 4 {
		t.Fatalf("context exact identity total/top cap drifted under a 100-way tie: %+v", ctx)
	}
	if ctx.ThreadIdentityCountExact == nil || !*ctx.ThreadIdentityCountExact {
		t.Fatalf("all-typed context did not publish an explicit exact witness: %+v", ctx)
	}
	if len(ctx.TopSymbols) != 1 || ctx.TopSymbols[0].ThreadIdentityCount != 100 || len(ctx.TopSymbols[0].ThreadIdentities) != perfPublishedThreadRosterCap || len(ctx.TopSymbols[0].Threads) != perfPublishedThreadRosterCap {
		t.Fatalf("hotspot typed/legacy rosters are unbounded or total is inexact: %+v", ctx.TopSymbols)
	}
	if roster := perfContextThreadRoster(ctx, 4); !strings.Contains(roster, "+96_more") {
		t.Fatalf("context roster inferred omitted count from truncated TopThreads: %q", roster)
	}
	facts := evidenceFromPerfContext(ctx)
	if len(facts) == 0 || !strings.Contains(facts[0].Summary, "+96_more") {
		t.Fatalf("evidence roster did not consume the exact hotspot total: %+v", facts)
	}
	timeline := BuildPerfTimeline(idx, q)
	if len(timeline.Buckets) != 1 || timeline.Buckets[0].ThreadIdentityCount != 100 || len(timeline.Buckets[0].ThreadIdentities) != perfPublishedThreadRosterCap || len(timeline.Buckets[0].Threads) != perfPublishedThreadRosterCap {
		t.Fatalf("timeline typed/legacy rosters are unbounded or total is inexact: %+v", timeline)
	}
	if timeline.Buckets[0].ThreadIdentityCountExact == nil || !*timeline.Buckets[0].ThreadIdentityCountExact {
		t.Fatalf("all-typed timeline did not publish an explicit exact witness: %+v", timeline.Buckets)
	}
}

func TestPerfTypedIdentitySurvivesRootAndFrameConsumersWithUnrelatedReuse(t *testing.T) {
	target := ThreadRef{PID: 100, Comm: "target"}
	onChain := ThreadRef{PID: 200, Comm: "chain"}
	binderPeer := ThreadRef{PID: 300, Comm: "binder"}
	competitor := ThreadRef{PID: 400, Comm: "competitor"}
	events := append(perfIdentityQueryReuse(1, 1.0, 900, "unrelated-old", "unrelated-new"),
		perfIdentityQuerySample(3, 1.10, target.PID, 10, target.Comm, "Target::run"),
		perfIdentityQuerySample(4, 1.11, onChain.PID, 20, onChain.Comm, "Chain::run"),
		perfIdentityQuerySample(5, 1.12, binderPeer.PID, 30, binderPeer.Comm, "Binder::run"),
		perfIdentityQuerySample(6, 1.13, competitor.PID, 40, competitor.Comm, "Competitor::run"),
	)
	events[2].CPU, events[3].CPU, events[4].CPU, events[5].CPU = 0, 1, 2, 3
	idx := &Index{Events: events, FirstTs: 1, LastTs: 1.13, TimestampOrder: TraceTimestampOrderMonotonic}
	q := Query{PID: target.PID, TimeStart: 1, TimeEnd: 1.2}
	stats := WindowStats{
		PerfSamples: computePerfContext(idx, q, 8),
		RunnableContext: []RunnableContextSummary{{
			Thread: target, RunnableWaitMs: 10, CPU: 3, LineStart: 3, LineEnd: 6,
			SameCPUTopRunning: []ThreadDuration{{Thread: competitor, CPU: 3, DurationMs: 10, StartTs: 1, EndTs: 1.2}},
		}},
	}
	rank := attachPerfContextToRootCauseRank(idx, q, RootCauseRankResult{Target: target, Items: []RootCauseRankItem{{
		Type: "runnable_wait", Thread: target, StartTs: 1, EndTs: 1.2, LineStart: 3, LineEnd: 6, ChainRelevance: "on_chain", runnableCPU: 3, runnableCPUKnown: true,
	}}}, stats)
	if len(rank.Items) != 1 || perfRoleContextByName(rank.Items[0].PerfContexts, "candidate_thread") == nil || perfRoleContextByName(rank.Items[0].PerfContexts, "on_chain_dependency") == nil || perfRoleContextByName(rank.Items[0].PerfContexts, "same_cpu_competitor") == nil {
		t.Fatalf("root rank consumers lost typed roles because an unrelated TID reused: %+v", rank.Items)
	}
	for _, role := range rank.Items[0].PerfContexts {
		if role.PerfContext == nil || role.PerfContext.ThreadIdentityCount == 0 || role.PerfContext.TopThreads[0].Identity == nil {
			t.Fatalf("root role published without typed generation identity: %+v", role)
		}
	}
	chain := &ChainResult{Target: target, Nodes: []ChainNode{{Thread: onChain}}}
	blocking := CriticalBlockingResult{Items: []CriticalBlockingCandidate{{Type: "binder_wait", Thread: target, Peer: binderPeer}}}
	frame := buildFramePerfContexts(idx, q, stats, chain, blocking, target)
	for name, ctx := range map[string]*PerfContext{
		"global": frame.PerfSamples, "target": frame.TargetRunningPerf, "on_chain": frame.OnChainPerf,
		"binder_peer": frame.BinderPeerPerf, "same_cpu": frame.SameCPUCompetitorPerf,
	} {
		if ctx == nil || ctx.SampleCount == 0 || ctx.ThreadIdentityCount == 0 || len(ctx.TopThreads) == 0 || ctx.TopThreads[0].Identity == nil {
			t.Fatalf("frame %s consumer lost typed generation context: %+v", name, ctx)
		}
	}
}

func TestPerfRelevantReuseWithdrawsOnlyAffectedRootAndFrameRole(t *testing.T) {
	target := ThreadRef{PID: 100, Comm: "target-new"}
	onChain := ThreadRef{PID: 200, Comm: "chain"}
	events := perfIdentityQueryReuse(1, 1.0, target.PID, "target-old", target.Comm)
	events = append(events,
		perfIdentityQuerySample(3, 1.10, target.PID, 10, target.Comm, "Target::new"),
		perfIdentityQuerySample(4, 1.11, onChain.PID, 20, onChain.Comm, "Chain::run"),
	)
	idx := &Index{Events: events, FirstTs: 1, LastTs: 1.11, TimestampOrder: TraceTimestampOrderMonotonic}
	q := Query{PID: target.PID, TimeStart: 0.9, TimeEnd: 1.2}
	global := computePerfContext(idx, q, 8)
	if global == nil || global.SampleCount != 2 {
		t.Fatalf("global inventory must survive a relevant lifecycle boundary: %+v", global)
	}
	rank := attachPerfContextToRootCauseRank(idx, q, RootCauseRankResult{Target: target, Items: []RootCauseRankItem{{
		Type: "running", Thread: target, StartTs: 0.9, EndTs: 1.2, ChainRelevance: "on_chain",
	}}}, WindowStats{PerfSamples: global})
	if len(rank.Items) != 1 || perfRoleContextByName(rank.Items[0].PerfContexts, "candidate_thread") != nil || perfRoleContextByName(rank.Items[0].PerfContexts, "target_running") != nil {
		t.Fatalf("root consumer retained an ambiguous target generation role: %+v", rank.Items)
	}
	chain := &ChainResult{Target: target, Nodes: []ChainNode{{Thread: onChain}}}
	frame := buildFramePerfContexts(idx, q, WindowStats{PerfSamples: global}, chain, CriticalBlockingResult{}, target)
	if frame.PerfSamples == nil || frame.TargetRunningPerf != nil || frame.OnChainPerf == nil || frame.OnChainPerf.SampleCount != 1 || frame.OnChainPerf.TopThreads[0].Identity == nil || frame.OnChainPerf.TopThreads[0].Identity.TID != onChain.PID {
		t.Fatalf("frame consumer did not withdraw only the ambiguous target role: %+v", frame)
	}
}

func perfRoleContextByName(contexts []RootCausePerfRoleContext, role string) *PerfContext {
	for _, context := range contexts {
		if context.Role == role {
			return context.PerfContext
		}
	}
	return nil
}

func TestPerfAggregatesDoNotUseDisplayThreadLabelAsHardKey(t *testing.T) {
	source, err := os.ReadFile("query.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"threadSet[threadLabel(thread)]",
		"key := threadLabel(thread)",
		"map[string]*perfThreadAcc",
	} {
		if strings.Contains(string(source), forbidden) {
			t.Fatalf("perf aggregate hard key regressed to display text: %q", forbidden)
		}
	}
}
