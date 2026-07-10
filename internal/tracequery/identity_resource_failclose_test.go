package tracequery

import "testing"

func TestThreadIncarnationConflictSuppressesPIDKeyedResourceAggregates(t *testing.T) {
	idx := buildTraceIndex(t, "identity_resource_failclose.systrace", `
		  old-42 (   42) [000] .... 1.001000: mm_filemap_add_to_page_cache: dev 260:84 ino 0x1 page=0 pfn=1 ofs=0
          old-42 (   42) [000] .... 1.002000: workqueue_execute_start: work=0xff function=flush_cookie
          old-42 (   42) [000] .... 1.003000: dma_fence_wait_start: driver=display timeline=present seqno=9
      creator-7 (    7) [001] .... 1.004000: sched_wakeup_new: comm=new pid=42 prio=20 target_cpu=000
          new-42 (   42) [000] .... 1.005000: mm_filemap_delete_from_page_cache: dev 260:84 ino 0x1 page=0 pfn=1 ofs=0
          new-42 (   42) [000] .... 1.006000: workqueue_execute_end: work=0xff function=flush_cookie
          new-42 (   42) [000] .... 1.007000: dma_fence_wait_end: driver=display timeline=present seqno=9
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
