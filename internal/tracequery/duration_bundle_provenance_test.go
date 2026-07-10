package tracequery

import "testing"

const (
	durationSourceA = "/trace/source-a.systrace"
	durationSourceB = "/trace/source-b.systrace"
)

func durationBundleIndex(events []Event) *Index {
	return &Index{
		Path:           "/trace/bundle.tracebundle.json",
		TimestampOrder: TraceTimestampOrderMonotonic,
		TraceArtifacts: []TraceArtifactSource{
			{SourcePath: durationSourceA, LocalLineCount: 50, VirtualLineBase: 0, CausalCompatible: true},
			{SourcePath: durationSourceB, LocalLineCount: 50, VirtualLineBase: 100, CausalCompatible: true},
		},
		Events: events,
	}
}

func TestDurationPairingNeverCrossesPhysicalArtifacts(t *testing.T) {
	events := []Event{
		{Line: 1, Ts: 1.000, Type: EventTraceMark, PID: 10, SpanAction: "B", SpanPID: 10, SpanName: "sync-work", FieldText: "B|10|sync-work"},
		{Line: 101, Ts: 1.001, Type: EventTraceMark, PID: 10, SpanAction: "E", SpanPID: 10, FieldText: "E|10"},
		{Line: 2, Ts: 1.002, Type: EventTraceMark, PID: 10, SpanAction: "S", SpanPID: 10, SpanName: "async-work", SpanValue: "7", FieldText: "S|10|async-work|7"},
		{Line: 102, Ts: 1.003, Type: EventTraceMark, PID: 10, SpanAction: "F", SpanPID: 10, SpanName: "async-work", SpanValue: "7", FieldText: "F|10|async-work|7"},
		{Line: 3, Ts: 1.004, Type: EventIRQ, Name: "irq_handler_entry", PID: 20, CPU: 0, IRQID: 17, IRQName: "timer"},
		{Line: 103, Ts: 1.005, Type: EventIRQ, Name: "irq_handler_exit", PID: 20, CPU: 0, IRQID: 17, IRQName: "timer"},
		{Line: 4, Ts: 1.006, Type: EventSoftIRQ, Name: "softirq_entry", PID: 20, CPU: 1, IRQID: 3, IRQName: "NET_RX"},
		{Line: 104, Ts: 1.007, Type: EventSoftIRQ, Name: "softirq_exit", PID: 20, CPU: 1, IRQID: 3, IRQName: "NET_RX"},
		{Line: 5, Ts: 1.008, Type: EventIPI, Name: "ipi_entry", PID: 20, CPU: 2, IRQName: "rescheduling interrupts"},
		{Line: 105, Ts: 1.009, Type: EventIPI, Name: "ipi_exit", PID: 20, CPU: 2, IRQName: "rescheduling interrupts"},
		{Line: 6, Ts: 1.010, Type: EventWorkqueue, Name: "workqueue_execute_start", PID: 30, FieldText: "work=0xff function=flush_cookie"},
		{Line: 106, Ts: 1.011, Type: EventWorkqueue, Name: "workqueue_execute_end", PID: 30, FieldText: "work=0xff function=flush_cookie"},
		{Line: 7, Ts: 1.012, Type: EventDMAFence, Name: "dma_fence_wait_start", PID: 40, FieldText: "driver=display timeline=present seqno=9"},
		{Line: 107, Ts: 1.013, Type: EventDMAFence, Name: "dma_fence_wait_end", PID: 40, FieldText: "driver=display timeline=present seqno=9"},
	}
	idx := durationBundleIndex(events)
	q := Query{TimeStart: 0.9, TimeEnd: 1.1}
	stats := ComputeWindowStats(idx, q)
	if len(stats.TraceSpans) != 0 {
		t.Fatalf("trace endpoints crossed physical artifacts: %+v", stats.TraceSpans)
	}
	if spans, _ := FindSpanWindows(idx, q, 16); len(spans) != 0 {
		t.Fatalf("span_window endpoints crossed physical artifacts: %+v", spans)
	}
	assertNoInterruptDuration := func(name string, rows []InterruptActivity) {
		t.Helper()
		for _, row := range rows {
			if row.PairedCount != 0 || row.ActiveMs != 0 {
				t.Fatalf("%s endpoints crossed physical artifacts: %+v", name, rows)
			}
		}
	}
	assertNoInterruptDuration("irq", stats.IRQActivity)
	assertNoInterruptDuration("softirq", stats.SoftIRQActivity)
	assertNoInterruptDuration("ipi", stats.IPIActivity)
	for _, row := range stats.WorkqueueActivity {
		if row.PairedCount != 0 || row.DurationMs != 0 {
			t.Fatalf("workqueue endpoints crossed physical artifacts: %+v", stats.WorkqueueActivity)
		}
	}
	for _, row := range stats.DMAFenceActivity {
		if row.PairedCount != 0 || row.WaitMs != 0 {
			t.Fatalf("DMA endpoints crossed physical artifacts: %+v", stats.DMAFenceActivity)
		}
	}
}

func TestSingleArtifactDurationPairsExposePhysicalSource(t *testing.T) {
	block := func() *BlockIOFields {
		return &BlockIOFields{Dev: "8,0", Op: "R", Sector: 123, Len: 8, IdentityParsed: true, IdentityValid: true}
	}
	events := []Event{
		{Line: 1, Ts: 1.000, Type: EventTraceMark, PID: 10, SpanAction: "B", SpanPID: 10, SpanName: "sync-work", FieldText: "B|10|sync-work"},
		{Line: 2, Ts: 1.001, Type: EventTraceMark, PID: 10, SpanAction: "E", SpanPID: 10, FieldText: "E|10"},
		{Line: 3, Ts: 1.002, Type: EventIRQ, Name: "irq_handler_entry", PID: 20, CPU: 0, IRQID: 17, IRQName: "timer"},
		{Line: 4, Ts: 1.003, Type: EventIRQ, Name: "irq_handler_exit", PID: 20, CPU: 0, IRQID: 17, IRQName: "timer"},
		{Line: 5, Ts: 1.004, Type: EventSoftIRQ, Name: "softirq_entry", PID: 20, CPU: 1, IRQID: 3, IRQName: "NET_RX"},
		{Line: 6, Ts: 1.005, Type: EventSoftIRQ, Name: "softirq_exit", PID: 20, CPU: 1, IRQID: 3, IRQName: "NET_RX"},
		{Line: 7, Ts: 1.006, Type: EventIPI, Name: "ipi_entry", PID: 20, CPU: 2, IRQName: "rescheduling interrupts"},
		{Line: 8, Ts: 1.007, Type: EventIPI, Name: "ipi_exit", PID: 20, CPU: 2, IRQName: "rescheduling interrupts"},
		{Line: 9, Ts: 1.008, Type: EventWorkqueue, Name: "workqueue_execute_start", PID: 30, FieldText: "work=0xff function=flush_cookie"},
		{Line: 10, Ts: 1.009, Type: EventWorkqueue, Name: "workqueue_execute_end", PID: 30, FieldText: "work=0xff function=flush_cookie"},
		{Line: 11, Ts: 1.010, Type: EventDMAFence, Name: "dma_fence_wait_start", PID: 40, FieldText: "driver=display timeline=present seqno=9"},
		{Line: 12, Ts: 1.011, Type: EventDMAFence, Name: "dma_fence_wait_end", PID: 40, FieldText: "driver=display timeline=present seqno=9"},
		{Line: 13, Ts: 1.012, Type: EventBlockIssue, Name: "block_rq_issue", PID: 50, BlockIOFields: block()},
		{Line: 14, Ts: 1.013, Type: EventBlockComplete, Name: "block_rq_complete", PID: 50, BlockIOFields: block()},
		{Line: 15, Ts: 1.014, Type: EventStorage, Name: "scsi_dispatch_cmd_start", PID: 60, ResourceFields: &ResourceFields{Op: "read", Bytes: 4096}, FileFields: &FileFields{Dev: "12,80", RW: "read", Len: 4096}},
		{Line: 16, Ts: 1.015, Type: EventStorage, Name: "scsi_dispatch_cmd_done", PID: 60, ResourceFields: &ResourceFields{Op: "read", Bytes: 4096}, FileFields: &FileFields{Dev: "12,80", RW: "read", Len: 4096}},
		{Line: 17, Ts: 1.016, Type: EventTraceMark, PID: 10, SpanAction: "S", SpanPID: 10, SpanName: "async-work", SpanValue: "7", FieldText: "S|10|async-work|7"},
		{Line: 18, Ts: 1.017, Type: EventTraceMark, PID: 10, SpanAction: "F", SpanPID: 10, SpanName: "async-work", SpanValue: "7", FieldText: "F|10|async-work|7"},
	}
	idx := &Index{
		Path:           durationSourceA,
		TimestampOrder: TraceTimestampOrderMonotonic,
		TraceArtifacts: []TraceArtifactSource{{SourcePath: durationSourceA, LocalLineCount: 32, CausalCompatible: true}},
		Events:         events,
	}
	q := Query{TimeStart: 0.9, TimeEnd: 1.1}
	stats := ComputeWindowStats(idx, q)
	if len(stats.TraceSpans) != 2 || stats.TraceSpans[0].SourcePath != durationSourceA || stats.TraceSpans[1].SourcePath != durationSourceA {
		t.Fatalf("trace span source missing: %+v", stats.TraceSpans)
	}
	if spans, _ := FindSpanWindows(idx, q, 16); len(spans) != 2 || spans[0].SourcePath != durationSourceA || spans[1].SourcePath != durationSourceA {
		t.Fatalf("span_window source missing: %+v", spans)
	}
	if len(stats.IOLatencies) != 1 || stats.IOLatencies[0].SourcePath != durationSourceA {
		t.Fatalf("block IO source missing: %+v", stats.IOLatencies)
	}
	for name, rows := range map[string][]InterruptActivity{"irq": stats.IRQActivity, "softirq": stats.SoftIRQActivity, "ipi": stats.IPIActivity} {
		if len(rows) != 1 || rows[0].PairedCount != 1 || rows[0].SourcePath != durationSourceA {
			t.Fatalf("%s source/pair missing: %+v", name, rows)
		}
	}
	if len(stats.WorkqueueActivity) != 1 || stats.WorkqueueActivity[0].PairedCount != 1 || stats.WorkqueueActivity[0].SourcePath != durationSourceA {
		t.Fatalf("workqueue source/pair missing: %+v", stats.WorkqueueActivity)
	}
	if len(stats.DMAFenceActivity) != 1 || stats.DMAFenceActivity[0].PairedCount != 1 || stats.DMAFenceActivity[0].SourcePath != durationSourceA {
		t.Fatalf("DMA source/pair missing: %+v", stats.DMAFenceActivity)
	}
	blockSeen, storageSeen := false, false
	for _, row := range stats.StorageLatencyByLayer {
		if row.SourcePath != durationSourceA {
			t.Fatalf("storage source missing: %+v", row)
		}
		blockSeen = blockSeen || row.Layer == "block" && row.PairedCount == 1
		storageSeen = storageSeen || row.Layer == "scsi" && row.PairedCount == 1
	}
	if !blockSeen || !storageSeen {
		t.Fatalf("block/generic storage positive controls missing: %+v", stats.StorageLatencyByLayer)
	}
}

func TestTraceMarkResetIsScopedToPhysicalSourceAndEmitter(t *testing.T) {
	for _, tc := range []struct {
		name  string
		reset Event
	}{
		{name: "malformed", reset: Event{Line: 2, Ts: 1.002, Type: EventTraceMark, PID: 10, FieldText: "B|bad"}},
		{name: "lifecycle", reset: Event{Line: 2, Ts: 1.002, Type: EventSchedSwitch, PID: 10, PrevPID: 10, PrevState: "X"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			idx := durationBundleIndex([]Event{
				{Line: 101, Ts: 1.000, Type: EventTraceMark, PID: 10, SpanAction: "B", SpanPID: 10, SpanName: "source-b-span", FieldText: "B|10|source-b-span"},
				{Line: 1, Ts: 1.001, Type: EventTraceMark, PID: 10, SpanAction: "B", SpanPID: 10, SpanName: "source-a-span", FieldText: "B|10|source-a-span"},
				tc.reset,
				{Line: 102, Ts: 1.003, Type: EventTraceMark, PID: 10, SpanAction: "E", SpanPID: 10, FieldText: "E|10"},
			})
			q := Query{TimeStart: 0.9, TimeEnd: 1.1}
			spans, _, caveats := computeTraceMarks(idx, q, 16)
			if len(spans) != 1 || spans[0].Name != "source-b-span" || spans[0].SourcePath != durationSourceB {
				t.Fatalf("%s reset leaked across sources: spans=%+v caveats=%+v", tc.name, spans, caveats)
			}
			windows, _ := FindSpanWindows(idx, q, 16)
			if len(windows) != 1 || windows[0].Name != "source-b-span" || windows[0].SourcePath != durationSourceB {
				t.Fatalf("%s span_window reset leaked across sources: %+v", tc.name, windows)
			}
		})
	}
}

func TestUnresolvedDurationSourceFailsClosedWithCaveat(t *testing.T) {
	idx := &Index{
		Path:           "/trace/bundle.tracebundle.json",
		TimestampOrder: TraceTimestampOrderMonotonic,
		TraceArtifacts: []TraceArtifactSource{{SourcePath: durationSourceA, LocalLineCount: 1, CausalCompatible: true}},
		Events: []Event{
			{Line: 99, Ts: 1.000, Type: EventTraceMark, PID: 10, SpanAction: "B", SpanPID: 10, SpanName: "unresolved", FieldText: "B|10|unresolved"},
			{Line: 100, Ts: 1.001, Type: EventTraceMark, PID: 10, SpanAction: "E", SpanPID: 10, FieldText: "E|10"},
			{Line: 101, Ts: 1.002, Type: EventIRQ, Name: "irq_handler_entry", PID: 20, CPU: 0, IRQID: 17, IRQName: "timer"},
			{Line: 102, Ts: 1.003, Type: EventWorkqueue, Name: "workqueue_execute_start", PID: 30, FieldText: "work=0xff function=flush_cookie"},
			{Line: 103, Ts: 1.004, Type: EventDMAFence, Name: "dma_fence_wait_start", PID: 40, FieldText: "driver=display timeline=present seqno=9"},
			{Line: 104, Ts: 1.005, Type: EventBlockIssue, Name: "block_rq_issue", PID: 50, BlockIOFields: &BlockIOFields{Dev: "8,0", Op: "R", Sector: 123, Len: 8, IdentityParsed: true, IdentityValid: true}},
			{Line: 105, Ts: 1.006, Type: EventStorage, Name: "scsi_dispatch_cmd_start", PID: 60, ResourceFields: &ResourceFields{Op: "read", Bytes: 4096}, FileFields: &FileFields{Dev: "12,80", RW: "read", Len: 4096}},
		},
	}
	q := Query{TimeStart: 0.9, TimeEnd: 1.1}
	stats := ComputeWindowStats(idx, q)
	for _, token := range []string{
		"trace_mark_pairing_provenance_unresolved=true",
		"interrupt_pairing_provenance_unresolved=true; family=irq",
		"workqueue_pairing_provenance_unresolved=true",
		"dma_fence_pairing_provenance_unresolved=true",
		"block_io_pairing_provenance_unresolved=true",
		"storage_latency_pairing_provenance_unresolved=true",
	} {
		if !containsSubstring(stats.Caveats, token) {
			t.Fatalf("missing %q caveat: %+v", token, stats.Caveats)
		}
	}
	if windows, caveats := FindSpanWindows(idx, q, 16); len(windows) != 0 || !containsSubstring(caveats, "trace_mark_pairing_provenance_unresolved=true") {
		t.Fatalf("span_window unresolved provenance did not fail closed: windows=%+v caveats=%+v", windows, caveats)
	}
	if len(stats.TraceSpans) != 0 || durationRowsMinted(stats) {
		t.Fatalf("unresolved source minted a duration: %+v", stats)
	}
}

func TestUnresolvedTraceMarkResetFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name  string
		reset Event
	}{
		{name: "malformed", reset: Event{Line: 99, Ts: 1.001, Type: EventTraceMark, PID: 10, FieldText: "B|bad"}},
		{name: "lifecycle", reset: Event{Line: 99, Ts: 1.001, Type: EventSchedSwitch, PID: 10, PrevPID: 10, PrevState: "X"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			idx := &Index{
				Path:           "/trace/bundle.tracebundle.json",
				TimestampOrder: TraceTimestampOrderMonotonic,
				TraceArtifacts: []TraceArtifactSource{{SourcePath: durationSourceA, LocalLineCount: 2, CausalCompatible: true}},
				Events: []Event{
					{Line: 1, Ts: 1.000, Type: EventTraceMark, PID: 10, SpanAction: "B", SpanPID: 10, SpanName: "known-span", FieldText: "B|10|known-span"},
					tc.reset,
					{Line: 2, Ts: 1.002, Type: EventTraceMark, PID: 10, SpanAction: "E", SpanPID: 10, FieldText: "E|10"},
				},
			}
			q := Query{TimeStart: 0.9, TimeEnd: 1.1}
			spans, _, caveats := computeTraceMarks(idx, q, 16)
			if len(spans) != 0 || !containsSubstring(caveats, "trace_mark_pairing_provenance_unresolved=true") {
				t.Fatalf("%s unresolved reset did not fail close trace spans: spans=%+v caveats=%+v", tc.name, spans, caveats)
			}
			windows, windowCaveats := FindSpanWindows(idx, q, 16)
			if len(windows) != 0 || !containsSubstring(windowCaveats, "trace_mark_pairing_provenance_unresolved=true") {
				t.Fatalf("%s unresolved reset did not fail close span_window: spans=%+v caveats=%+v", tc.name, windows, windowCaveats)
			}
		})
	}
}

func durationRowsMinted(stats WindowStats) bool {
	for _, row := range append(append(append([]InterruptActivity{}, stats.IRQActivity...), stats.SoftIRQActivity...), stats.IPIActivity...) {
		if row.ActiveMs != 0 || row.PairedCount != 0 {
			return true
		}
	}
	for _, row := range stats.WorkqueueActivity {
		if row.DurationMs != 0 || row.PairedCount != 0 {
			return true
		}
	}
	for _, row := range stats.DMAFenceActivity {
		if row.WaitMs != 0 || row.PairedCount != 0 {
			return true
		}
	}
	return false
}
