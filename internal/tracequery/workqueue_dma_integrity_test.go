package tracequery

import (
	"strings"
	"testing"
)

func TestDMAFenceSignalCannotCloseWait(t *testing.T) {
	idx := buildTraceIndex(t, "dma-signal-is-instant.systrace", strings.Join([]string{
		`display-30 (30) [002] .... 1.000000: dma_fence_wait_start: driver=display timeline=present context=7 seqno=9`,
		`display-30 (30) [002] .... 1.100000: dma_fence_signaled: driver=display timeline=present context=7 seqno=9`,
		`display-30 (30) [002] .... 1.200000: dma_fence_wait_end: driver=display timeline=present context=7 seqno=9`,
	}, "\n")+"\n")
	stats := ComputeWindowStats(idx, Query{TimeStart: 0.9, TimeEnd: 1.3})
	var wait *DMAFenceActivity
	for i := range stats.DMAFenceActivity {
		if stats.DMAFenceActivity[i].PairedCount > 0 {
			wait = &stats.DMAFenceActivity[i]
		}
	}
	if wait == nil || !near(wait.WaitMs, 200, 0.001) {
		t.Fatalf("signaled is inventory and must not close wait_start before wait_end: %+v", stats.DMAFenceActivity)
	}
}

func TestDurationEndpointNamesUseCaseSensitiveExactClosedSet(t *testing.T) {
	for _, tc := range []struct {
		name  string
		phase func(string) (string, string)
	}{
		{name: "WORKQUEUE_EXECUTE_START", phase: workqueueBaseAndPhase},
		{name: "DMA_FENCE_WAIT_END", phase: dmaFenceBaseAndPhase},
	} {
		if _, phase := tc.phase(tc.name); phase != "" {
			t.Fatalf("case-drifted inventory event %q entered exact duration endpoint lane as %q", tc.name, phase)
		}
	}
}

func TestWorkqueueAndDMAConcurrentSameIdentitySuppressWholeCohortThenRecover(t *testing.T) {
	idx := buildTraceIndex(t, "duration-cohort.systrace", strings.Join([]string{
		`worker-20 (20) [001] .... 1.000000: workqueue_execute_start: work=0xaa function=flush`,
		`worker-20 (20) [001] .... 1.010000: workqueue_execute_start: work=0xaa function=flush`,
		`worker-20 (20) [001] .... 1.020000: workqueue_execute_end: work=0xaa`,
		`worker-20 (20) [001] .... 1.030000: workqueue_execute_end: work=0xaa`,
		`worker-20 (20) [001] .... 1.040000: workqueue_execute_start: work=0xaa function=flush`,
		`worker-20 (20) [001] .... 1.050000: workqueue_execute_end: work=0xaa`,
		`display-30 (30) [002] .... 1.000000: dma_fence_wait_start: driver=display timeline=present context=7 seqno=9`,
		`display-30 (30) [002] .... 1.010000: dma_fence_wait_start: driver=display timeline=present context=7 seqno=9`,
		`display-30 (30) [002] .... 1.020000: dma_fence_wait_end: driver=display timeline=present context=7 seqno=9`,
		`display-30 (30) [002] .... 1.030000: dma_fence_wait_end: driver=display timeline=present context=7 seqno=9`,
		`display-30 (30) [002] .... 1.040000: dma_fence_wait_start: driver=display timeline=present context=7 seqno=9`,
		`display-30 (30) [002] .... 1.050000: dma_fence_wait_end: driver=display timeline=present context=7 seqno=9`,
	}, "\n")+"\n")
	stats := ComputeWindowStats(idx, Query{TimeStart: 0.9, TimeEnd: 1.1})
	if len(stats.WorkqueueActivity) != 1 {
		t.Fatalf("expected one work identity row: %+v", stats.WorkqueueActivity)
	}
	work := stats.WorkqueueActivity[0]
	if work.PairedCount != 1 || !near(work.DurationMs, 10, 0.001) || work.AmbiguousCohortCount != 1 || work.PairingSuppressedCount != 2 {
		t.Fatalf("workqueue ambiguous cohort must contribute zero, then recover for next pair: %+v", work)
	}
	if len(stats.DMAFenceActivity) != 1 {
		t.Fatalf("expected one DMA identity row: %+v", stats.DMAFenceActivity)
	}
	fence := stats.DMAFenceActivity[0]
	if fence.PairedCount != 1 || !near(fence.WaitMs, 10, 0.001) || fence.AmbiguousCohortCount != 1 || fence.PairingSuppressedCount != 2 {
		t.Fatalf("DMA ambiguous cohort must contribute zero, then recover for next pair: %+v", fence)
	}
	for _, want := range []string{"workqueue_pairing_ambiguous=true", "dma_fence_pairing_ambiguous=true"} {
		if !containsSubstring(stats.Caveats, want) {
			t.Fatalf("missing ambiguity disclosure %q: %v", want, stats.Caveats)
		}
	}
}

func TestWorkqueueAndDMADifferentTypedIdentitiesPairIndependently(t *testing.T) {
	idx := buildTraceIndex(t, "duration-independent-identities.systrace", strings.Join([]string{
		`worker-20 (20) [001] .... 1.000000: workqueue_execute_start: work=0xaa function=first`,
		`worker-20 (20) [001] .... 1.010000: workqueue_execute_start: work=0xbb function=second`,
		`worker-20 (20) [001] .... 1.020000: workqueue_execute_end: work=0xaa`,
		`worker-20 (20) [001] .... 1.030000: workqueue_execute_end: work=0xbb`,
		`display-30 (30) [002] .... 1.000000: dma_fence_wait_start: driver=display timeline=present context=7 seqno=9`,
		`display-30 (30) [002] .... 1.010000: dma_fence_wait_start: driver=display timeline=present context=7 seqno=10`,
		`display-30 (30) [002] .... 1.020000: dma_fence_wait_end: driver=display timeline=present context=7 seqno=9`,
		`display-30 (30) [002] .... 1.030000: dma_fence_wait_end: driver=display timeline=present context=7 seqno=10`,
	}, "\n")+"\n")
	stats := ComputeWindowStats(idx, Query{TimeStart: 0.9, TimeEnd: 1.1})
	if len(stats.WorkqueueActivity) != 2 {
		t.Fatalf("different work identities must retain independent lanes: %+v", stats.WorkqueueActivity)
	}
	for _, work := range stats.WorkqueueActivity {
		if work.PairedCount != 1 || work.AmbiguousCohortCount != 0 || work.PairingSuppressedCount != 0 {
			t.Fatalf("independent work identity was treated as ambiguous: %+v", work)
		}
	}
	if len(stats.DMAFenceActivity) != 2 {
		t.Fatalf("different fence identities must retain independent lanes: %+v", stats.DMAFenceActivity)
	}
	for _, fence := range stats.DMAFenceActivity {
		if fence.PairedCount != 1 || fence.AmbiguousCohortCount != 0 || fence.PairingSuppressedCount != 0 {
			t.Fatalf("independent fence identity was treated as ambiguous: %+v", fence)
		}
	}
}

func TestDMAFenceTupleKeyCannotCollideOnSlashBearingFields(t *testing.T) {
	idx := buildTraceIndex(t, "dma-nul-tuple.systrace", strings.Join([]string{
		`display-30 (30) [002] .... 1.000000: dma_fence_wait_start: driver=a/b timeline=c context=7 seqno=9`,
		`display-30 (30) [002] .... 1.010000: dma_fence_wait_start: driver=a timeline=b/c context=7 seqno=9`,
		`display-30 (30) [002] .... 1.020000: dma_fence_wait_end: driver=a/b timeline=c context=7 seqno=9`,
		`display-30 (30) [002] .... 1.030000: dma_fence_wait_end: driver=a timeline=b/c context=7 seqno=9`,
	}, "\n")+"\n")
	stats := ComputeWindowStats(idx, Query{TimeStart: 0.9, TimeEnd: 1.1})
	if len(stats.DMAFenceActivity) != 2 {
		t.Fatalf("NUL tuple key must keep slash-bearing driver/timeline identities distinct: %+v caveats=%v", stats.DMAFenceActivity, stats.Caveats)
	}
	for _, fence := range stats.DMAFenceActivity {
		if fence.PairedCount != 1 || fence.AmbiguousCohortCount != 0 {
			t.Fatalf("distinct fence tuple collided: %+v", fence)
		}
	}
}

func TestWorkqueuePositionalEndWithoutFunctionStillPairsByWorkIdentity(t *testing.T) {
	idx := buildTraceIndex(t, "workqueue-v414.systrace", strings.Join([]string{
		`worker-20 (20) [001] .... 1.000000: workqueue_execute_start: work struct 0xff: function flush_cookie`,
		`worker-20 (20) [001] .... 1.020000: workqueue_execute_end: work struct 0xff`,
	}, "\n")+"\n")
	stats := ComputeWindowStats(idx, Query{TimeStart: 0.9, TimeEnd: 1.1})
	if len(stats.WorkqueueActivity) != 1 || stats.WorkqueueActivity[0].PairedCount != 1 || !near(stats.WorkqueueActivity[0].DurationMs, 20, 0.001) {
		t.Fatalf("stable work pointer must pair across an end row without function metadata: %+v caveats=%v", stats.WorkqueueActivity, stats.Caveats)
	}
}

func TestWorkqueuePointerReuseWithDifferentFunctionsIsDisclosedAsMultiple(t *testing.T) {
	idx := buildTraceIndex(t, "workqueue-function-variants.systrace", strings.Join([]string{
		`worker-20 (20) [001] .... 1.000000: workqueue_execute_start: work=0xaa function=first`,
		`worker-20 (20) [001] .... 1.010000: workqueue_execute_end: work=0xaa`,
		`worker-20 (20) [001] .... 1.020000: workqueue_execute_start: work=0xaa function=second`,
		`worker-20 (20) [001] .... 1.030000: workqueue_execute_end: work=0xaa`,
	}, "\n")+"\n")
	stats := ComputeWindowStats(idx, Query{TimeStart: 0.9, TimeEnd: 1.1})
	if len(stats.WorkqueueActivity) != 1 || stats.WorkqueueActivity[0].PairedCount != 2 || stats.WorkqueueActivity[0].Function != "multiple" {
		t.Fatalf("reused work pointer must not retain a false first-function label: %+v", stats.WorkqueueActivity)
	}
	if !containsSubstring(stats.Caveats, "workqueue_function_variants=true") {
		t.Fatalf("missing function-variant disclosure: %v", stats.Caveats)
	}
}

func TestMalformedWorkqueueAndDMAEndpointFailOnlyAffectedFamily(t *testing.T) {
	tests := []struct {
		name       string
		broken     string
		familyGone func(WindowStats) bool
		wantField  string
	}{
		{
			name: "work missing work", broken: `worker-20 (20) [001] .... 1.000000: workqueue_execute_start: function=flush`,
			familyGone: func(stats WindowStats) bool { return len(stats.WorkqueueActivity) == 0 }, wantField: "missing_or_invalid=work",
		},
		{
			name: "work invalid pointer", broken: `worker-20 (20) [001] .... 1.000000: workqueue_execute_start: work=bad function=flush`,
			familyGone: func(stats WindowStats) bool { return len(stats.WorkqueueActivity) == 0 }, wantField: "missing_or_invalid=work",
		},
		{
			name: "work alias cannot satisfy hard identity", broken: `worker-20 (20) [001] .... 1.000000: workqueue_execute_start: address=0xaa function=flush`,
			familyGone: func(stats WindowStats) bool { return len(stats.WorkqueueActivity) == 0 }, wantField: "missing_or_invalid=work",
		},
		{
			name: "dma missing context", broken: `display-30 (30) [002] .... 1.000000: dma_fence_wait_start: driver=display timeline=present seqno=9`,
			familyGone: func(stats WindowStats) bool { return len(stats.DMAFenceActivity) == 0 }, wantField: "missing_or_invalid=context",
		},
		{
			name: "dma invalid seqno", broken: `display-30 (30) [002] .... 1.000000: dma_fence_wait_start: driver=display timeline=present context=7 seqno=bad`,
			familyGone: func(stats WindowStats) bool { return len(stats.DMAFenceActivity) == 0 }, wantField: "missing_or_invalid=seqno",
		},
		{
			name: "dma aliases cannot satisfy hard identity", broken: `display-30 (30) [002] .... 1.000000: dma_fence_wait_start: name=display timeline=present context=7 id=9`,
			familyGone: func(stats WindowStats) bool { return len(stats.DMAFenceActivity) == 0 }, wantField: "missing_or_invalid=driver,seqno",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			idx := buildTraceIndex(t, "malformed-endpoint.systrace", strings.Join([]string{
				tc.broken,
				`worker-21 (21) [001] .... 1.010000: workqueue_execute_start: work=0xbb function=ok`,
				`display-31 (31) [002] .... 1.011000: dma_fence_wait_start: driver=display timeline=other context=8 seqno=10`,
				`worker-21 (21) [001] .... 1.020000: workqueue_execute_end: work=0xbb`,
				`display-31 (31) [002] .... 1.021000: dma_fence_wait_end: driver=display timeline=other context=8 seqno=10`,
			}, "\n")+"\n")
			if idx.TimestampOrder != TraceTimestampOrderMonotonic {
				t.Fatalf("fixture must exercise the common monotonic-index path: %v", idx.TimestampOrder)
			}
			stats := ComputeWindowStats(idx, Query{TimeStart: 0.9, TimeEnd: 1.1})
			if !tc.familyGone(stats) || !containsSubstring(stats.Caveats, "duration_pairing_source_fail_closed=true") || !containsSubstring(stats.Caveats, tc.wantField) {
				t.Fatalf("unkeyable endpoint must fail only its physical-source family closed: stats=%+v caveats=%v", stats, stats.Caveats)
			}
			if tc.name == "work missing work" && len(stats.DMAFenceActivity) != 1 {
				t.Fatalf("workqueue identity failure poisoned independent DMA family: %+v", stats.DMAFenceActivity)
			}
			if strings.HasPrefix(tc.name, "dma") && len(stats.WorkqueueActivity) != 1 {
				t.Fatalf("DMA identity failure poisoned independent workqueue family: %+v", stats.WorkqueueActivity)
			}
		})
	}
}

func TestRawRejectedDurationEndpointFailsFamilyClosed(t *testing.T) {
	idx := buildTraceIndex(t, "raw-rejected-endpoint.systrace", strings.Join([]string{
		`worker-20 (20) [99999] .... 1.000000: workqueue_execute_start: work=0xaa function=bad_cpu`,
		`worker-21 (21) [001] .... 1.010000: workqueue_execute_start: work=0xbb function=valid`,
		`display-31 (31) [002] .... 1.011000: dma_fence_wait_start: driver=display timeline=present context=7 seqno=9`,
		`worker-21 (21) [001] .... 1.020000: workqueue_execute_end: work=0xbb`,
		`display-31 (31) [002] .... 1.021000: dma_fence_wait_end: driver=display timeline=present context=7 seqno=9`,
	}, "\n")+"\n")
	stats := ComputeWindowStats(idx, Query{TimeStart: 0.9, TimeEnd: 1.1})
	if len(stats.WorkqueueActivity) != 1 || stats.WorkqueueActivity[0].Work != "0xbb" || !containsSubstring(stats.Caveats, "duration_pairing_exact_lane_quarantined=true") || !containsSubstring(stats.Caveats, "missing_or_invalid=header_cpu") {
		t.Fatalf("parser-rejected non-key metadata must quarantine only its exact lane: work=%+v caveats=%v", stats.WorkqueueActivity, stats.Caveats)
	}
	if len(stats.DMAFenceActivity) != 1 || stats.DMAFenceActivity[0].PairedCount != 1 {
		t.Fatalf("malformed workqueue header poisoned independent DMA family: %+v", stats.DMAFenceActivity)
	}
}

func TestExactDurationEndpointWithMalformedPIDHeaderStillFailsFamilyClosed(t *testing.T) {
	idx := buildTraceIndex(t, "raw-malformed-pid-endpoint.systrace", strings.Join([]string{
		`worker-badpid [001] .... 1.000000: dma_fence_wait_start: driver=display timeline=bad context=7 seqno=9`,
		`worker-21 (21) [001] .... 1.010000: workqueue_execute_start: work=0xbb function=valid`,
		`display-31 (31) [002] .... 1.011000: dma_fence_wait_start: driver=display timeline=present context=8 seqno=10`,
		`worker-21 (21) [001] .... 1.020000: workqueue_execute_end: work=0xbb`,
		`display-31 (31) [002] .... 1.021000: dma_fence_wait_end: driver=display timeline=present context=8 seqno=10`,
	}, "\n")+"\n")
	stats := ComputeWindowStats(idx, Query{TimeStart: 0.9, TimeEnd: 1.1})
	if len(stats.DMAFenceActivity) != 0 || !containsSubstring(stats.Caveats, "family=dma_fence") || !containsSubstring(stats.Caveats, "missing_or_invalid=pid") {
		t.Fatalf("exact event-column token with malformed PID must not disappear into generic unparsed census: dma=%+v caveats=%v", stats.DMAFenceActivity, stats.Caveats)
	}
	if len(stats.WorkqueueActivity) != 1 || stats.WorkqueueActivity[0].PairedCount != 1 {
		t.Fatalf("malformed DMA header poisoned independent workqueue family: %+v", stats.WorkqueueActivity)
	}
}

func TestUnknownTimestampDurationEndpointCannotBeProvenOutsidePositiveWindow(t *testing.T) {
	badTimestampLine := `worker-20 (20) [001] .... ` + strings.Repeat("9", 400) + `.0: workqueue_execute_start: work=0xaa function=bad_time`
	idx := buildTraceIndex(t, "raw-invalid-timestamp.systrace", strings.Join([]string{
		badTimestampLine,
		`worker-21 (21) [001] .... 1.010000: workqueue_execute_start: work=0xbb function=valid`,
		`worker-21 (21) [001] .... 1.020000: workqueue_execute_end: work=0xbb`,
	}, "\n")+"\n")
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.0, TimeEnd: 1.1})
	if len(stats.WorkqueueActivity) != 1 || stats.WorkqueueActivity[0].Work != "0xbb" || !containsSubstring(stats.Caveats, "duration_pairing_exact_lane_quarantined=true") || !containsSubstring(stats.Caveats, "missing_or_invalid=timestamp") {
		t.Fatalf("unknown endpoint timestamp must quarantine its exact lane without deleting a sibling: work=%+v caveats=%v", stats.WorkqueueActivity, stats.Caveats)
	}
}
