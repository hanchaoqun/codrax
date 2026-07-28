package tracequery

import "testing"

func TestExactContextInventoryTypesQ2b(t *testing.T) {
	cases := []struct {
		name   string
		fields string
		want   EventType
	}{
		{name: "rss_stat", fields: "mm_id=7 curr=3 member=2 size=4096", want: EventRSSStat},
		{name: "phase_task_delta", fields: "comm=worker tid=42 delta_exec=900 deltas={1,2}", want: EventPhaseTaskDelta},
	}

	events := make([]Event, 0, len(cases))
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyEventType("worker", tc.name, tc.fields); got != tc.want {
				t.Fatalf("exact classification = %s, want %s", got, tc.want)
			}
			line := "          worker-42    (   42) [001] .... 1.00000" + string(rune('1'+i)) + ": " + tc.name + ": " + tc.fields
			ev, ok := ParseLine(i+1, line, newStringInterner())
			if !ok || ev.Type != tc.want || ev.Name != tc.name {
				t.Fatalf("ParseLine = %+v ok=%v, want exact context inventory type %s", ev, ok, tc.want)
			}
			assertContextInventoryEventHasNoAuthorityQ2b(t, ev)
			events = append(events, ev)
		})
	}

	idx := &Index{Events: events, TimestampOrder: TraceTimestampOrderMonotonic}
	q := Query{PID: 42, TimeStart: 1, TimeEnd: 1.1, Limit: 16}
	stats := ComputeWindowStats(idx, q)
	if stats.EventCounts[EventRSSStat] != 1 || stats.EventCounts[EventPhaseTaskDelta] != 1 {
		t.Fatalf("context inventory counts drifted: %+v", stats.EventCounts)
	}
	if len(stats.TraceSpans) != 0 || len(stats.TopRunning) != 0 || len(stats.RunnableTop) != 0 {
		t.Fatalf("context inventory acquired span/scheduler projection: spans=%+v running=%+v runnable=%+v", stats.TraceSpans, stats.TopRunning, stats.RunnableTop)
	}
	if rank := BuildRootCauseRank(idx, q); len(rank.AbsorbedItems) != 0 {
		t.Fatalf("context inventory acquired absorbed root-rank authority: %+v", rank)
	} else {
		for _, item := range rank.Items {
			// The target has no scheduler rows, so the ranker honestly emits its
			// independent no_sched_data disclosure. Neither context event may
			// create any non-gap contender.
			if item.Type != "trace_gap" {
				t.Fatalf("context inventory acquired root-rank authority: %+v", rank)
			}
		}
	}
}

func TestContextInventoryTypesRequireByteExactNamesQ2b(t *testing.T) {
	for _, tc := range []struct {
		name string
		near string
	}{
		{name: "rss_stat", near: "rss_stat_vendor"},
		{name: "rss_stat", near: "RSS_STAT"},
		{name: "phase_task_delta", near: "phase_task_delta_end"},
		{name: "phase_task_delta", near: "Phase_task_delta"},
	} {
		t.Run(tc.near, func(t *testing.T) {
			if got := classifyEventType("worker", tc.near, "tid=42"); got != EventUnknown {
				t.Fatalf("near-name %q for %q classified as %s, want unknown", tc.near, tc.name, got)
			}
		})
	}
	if got := classifyEventType("app", "print", "hello world"); got != EventUnknown {
		t.Fatalf("plain print B-4 contract drifted: got %s, want unknown", got)
	}
}

func TestContextInventoryParserVersionSeparatesCacheGenerationQ2b(t *testing.T) {
	if ParserVersion != "tracequery-v38" {
		t.Fatalf("context inventory type change requires parser cache invalidation, got %q", ParserVersion)
	}
	cache := newTraceIndexCache(1 << 20)
	oldKey := parseCacheKey{path: "context.trace", size: 1, modUnix: 1, version: "tracequery-v31"}
	currentKey := oldKey
	currentKey.version = ParserVersion
	cache.Store(oldKey, &Index{Path: oldKey.path, Events: []Event{{Type: EventUnknown}}})
	if _, ok := cache.Load(currentKey); ok {
		t.Fatal("the v32 context inventory parser reused a v31 cached index")
	}
}

func TestPhaseTaskDeltaIsStandardTraceEventNameQ2b(t *testing.T) {
	seen := map[string]bool{}
	for _, name := range StandardTraceEventNameUniverse() {
		seen[name] = true
	}
	if !seen["rss_stat"] || !seen["phase_task_delta"] {
		t.Fatalf("standard trace event universe missing exact context inventory names: rss_stat=%v phase_task_delta=%v", seen["rss_stat"], seen["phase_task_delta"])
	}
}

func assertContextInventoryEventHasNoAuthorityQ2b(t *testing.T, ev Event) {
	t.Helper()
	if ev.ConstraintFields != nil || ev.SchedStatFields != nil || ev.BinderFields != nil || ev.BlockIOFields != nil ||
		ev.ResourceFields != nil || ev.FileFields != nil || ev.PluginFields != nil || ev.PerfFields != nil {
		t.Fatalf("context inventory event acquired a typed side table: %+v", ev)
	}
	if ev.SubsystemKind != "" || ev.MemoryKind != "" || ev.SpanAction != "" || ev.SpanPID != 0 || ev.SpanName != "" || ev.SpanValue != "" ||
		ev.WakeePID != 0 || ev.PrevPID != 0 || ev.NextPID != 0 || ev.Frequency != 0 || ev.State != 0 || ev.IOWait != 0 {
		t.Fatalf("context inventory event acquired semantic projection fields: %+v", ev)
	}
}
