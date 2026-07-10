package tracequery

import "testing"

func TestThreadIncarnationConflictSuppressesPIDKeyedResourceAggregates(t *testing.T) {
	idx := buildTraceIndex(t, "identity_resource_failclose.systrace", `
		  old-42 (   42) [000] .... 1.001000: mm_filemap_add_to_page_cache: dev 260:84 ino 0x1 page=0 pfn=1 ofs=0
          old-42 (   42) [000] .... 1.002000: workqueue_execute_start: work=0xff function=flush_cookie
          old-42 (   42) [000] .... 1.003000: dma_fence_wait_start: driver=display timeline=present context=7 seqno=9
      creator-7 (    7) [001] .... 1.004000: sched_wakeup_new: comm=new pid=42 prio=20 target_cpu=000
          new-42 (   42) [000] .... 1.005000: mm_filemap_delete_from_page_cache: dev 260:84 ino 0x1 page=0 pfn=1 ofs=0
          new-42 (   42) [000] .... 1.006000: workqueue_execute_end: work=0xff function=flush_cookie
          new-42 (   42) [000] .... 1.007000: dma_fence_wait_end: driver=display timeline=present context=7 seqno=9
	`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.0, TimeEnd: 1.01, Limit: 20})
	if len(stats.BIOResources) != 0 || len(stats.FilesystemResources) != 0 || len(stats.PageFaultResources) != 0 ||
		len(stats.FileIOByInode) != 0 || len(stats.PageCacheByInode) != 0 || stats.TopIOInodes != nil ||
		len(stats.StorageLatencyByLayer) != 0 || len(stats.AbilityEvents) != 0 || len(stats.XPowerEvents) != 0 ||
		len(stats.HiSystemEvents) != 0 || len(stats.WorkqueueActivity) != 0 || len(stats.DMAFenceActivity) != 0 {
		t.Fatalf("PID-keyed resources must fail closed across task generations: %+v", stats)
	}
	if !containsSubstring(stats.Caveats, "thread_identity_resource_fail_closed=true") {
		t.Fatalf("resource suppression must be explicit to downstream reasoning: %+v", stats.Caveats)
	}
}

func TestWorkqueueAndDMAIgnoreUnrelatedReusedTID(t *testing.T) {
	idx := buildTraceIndex(t, "identity_resource_unrelated.systrace", `
          idle-1 (    1) [000] .... 1.001000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=old next_pid=900 next_prio=20
       creator-7 (    7) [001] .... 1.002000: sched_wakeup_new: comm=new pid=900 prio=20 target_cpu=000
        worker-100 ( 100) [002] .... 1.003000: workqueue_execute_start: work=0xaa function=flush
        worker-100 ( 100) [002] .... 1.004000: workqueue_execute_end: work=0xaa
       display-101 ( 101) [003] .... 1.005000: dma_fence_wait_start: driver=display timeline=present context=7 seqno=9
       display-101 ( 101) [003] .... 1.006000: dma_fence_wait_end: driver=display timeline=present context=7 seqno=9
	`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.0, TimeEnd: 1.01, Limit: 20})
	if len(stats.WorkqueueActivity) != 1 || stats.WorkqueueActivity[0].PairedCount != 1 {
		t.Fatalf("unrelated tid=900 reuse erased workqueue contributor pid=100: %+v caveats=%v", stats.WorkqueueActivity, stats.Caveats)
	}
	if len(stats.DMAFenceActivity) != 1 || stats.DMAFenceActivity[0].PairedCount != 1 {
		t.Fatalf("unrelated tid=900 reuse erased DMA contributor pid=101: %+v caveats=%v", stats.DMAFenceActivity, stats.Caveats)
	}
	for _, token := range []string{"thread_identity_workqueue_fail_closed=true", "thread_identity_dma_fence_fail_closed=true"} {
		if containsSubstring(stats.Caveats, token) {
			t.Fatalf("unrelated reuse emitted family identity caveat %q: %v", token, stats.Caveats)
		}
	}
}

func TestWorkqueueContributorReuseDoesNotSuppressIndependentDMA(t *testing.T) {
	idx := buildTraceIndex(t, "identity_workqueue_only.systrace", `
           old-42 (   42) [000] .... 1.001000: workqueue_execute_start: work=0xaa function=flush
       creator-7 (    7) [001] .... 1.002000: sched_wakeup_new: comm=new pid=42 prio=20 target_cpu=000
           new-42 (   42) [000] .... 1.003000: workqueue_execute_end: work=0xaa
       display-101 (  101) [003] .... 1.004000: dma_fence_wait_start: driver=display timeline=present context=7 seqno=9
       display-101 (  101) [003] .... 1.005000: dma_fence_wait_end: driver=display timeline=present context=7 seqno=9
	`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.0, TimeEnd: 1.01, Limit: 20})
	if len(stats.WorkqueueActivity) != 0 || !containsSubstring(stats.Caveats, "thread_identity_workqueue_fail_closed=true") {
		t.Fatalf("reused workqueue contributor must fail only workqueue closed: work=%+v caveats=%v", stats.WorkqueueActivity, stats.Caveats)
	}
	if len(stats.DMAFenceActivity) != 1 || stats.DMAFenceActivity[0].PairedCount != 1 || containsSubstring(stats.Caveats, "thread_identity_dma_fence_fail_closed=true") {
		t.Fatalf("workqueue conflict suppressed independent DMA contributor: dma=%+v caveats=%v", stats.DMAFenceActivity, stats.Caveats)
	}
}

func TestDMAContributorReuseDoesNotSuppressIndependentWorkqueue(t *testing.T) {
	idx := buildTraceIndex(t, "identity_dma_only.systrace", `
        worker-100 (  100) [002] .... 1.001000: workqueue_execute_start: work=0xaa function=flush
        worker-100 (  100) [002] .... 1.002000: workqueue_execute_end: work=0xaa
           old-42 (   42) [000] .... 1.003000: dma_fence_wait_start: driver=display timeline=present context=7 seqno=9
       creator-7 (    7) [001] .... 1.004000: sched_wakeup_new: comm=new pid=42 prio=20 target_cpu=000
           new-42 (   42) [000] .... 1.005000: dma_fence_wait_end: driver=display timeline=present context=7 seqno=9
	`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.0, TimeEnd: 1.01, Limit: 20})
	if len(stats.DMAFenceActivity) != 0 || !containsSubstring(stats.Caveats, "thread_identity_dma_fence_fail_closed=true") {
		t.Fatalf("reused DMA contributor must fail only DMA closed: dma=%+v caveats=%v", stats.DMAFenceActivity, stats.Caveats)
	}
	if len(stats.WorkqueueActivity) != 1 || stats.WorkqueueActivity[0].PairedCount != 1 || containsSubstring(stats.Caveats, "thread_identity_workqueue_fail_closed=true") {
		t.Fatalf("DMA conflict suppressed independent workqueue contributor: work=%+v caveats=%v", stats.WorkqueueActivity, stats.Caveats)
	}
}

func TestWorkqueueAndDMAContributorSetsFailClosedOnLifecycleAuditCap(t *testing.T) {
	idx := buildTraceIndex(t, "identity_resource_cap.systrace", `
        worker-100 ( 100) [002] .... 1.001000: workqueue_execute_start: work=0xaa function=flush
        worker-100 ( 100) [002] .... 1.002000: workqueue_execute_end: work=0xaa
       display-101 ( 101) [003] .... 1.003000: dma_fence_wait_start: driver=display timeline=present context=7 seqno=9
       display-101 ( 101) [003] .... 1.004000: dma_fence_wait_end: driver=display timeline=present context=7 seqno=9
	`)
	idx.threadIncarnationFailuresCapped = true
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.0, TimeEnd: 1.01, Limit: 20})
	if len(stats.WorkqueueActivity) != 0 || len(stats.DMAFenceActivity) != 0 ||
		!containsSubstring(stats.Caveats, "thread_identity_workqueue_fail_closed=true") ||
		!containsSubstring(stats.Caveats, "thread_identity_dma_fence_fail_closed=true") ||
		!containsSubstring(stats.Caveats, "lifecycle_audit_truncated") {
		t.Fatalf("capped lifecycle audit must remain fail-closed for both non-empty contributor sets: %+v", stats)
	}
}
