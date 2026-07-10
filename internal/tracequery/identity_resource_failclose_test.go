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

func TestDirectResourceAndPluginFamiliesIgnoreUnrelatedReusedTID(t *testing.T) {
	idx := buildTraceIndex(t, "identity_direct_resource_unrelated.systrace", `
          idle-1 (    1) [000] .... 1.001000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=old next_pid=900 next_prio=20
       creator-7 (    7) [001] .... 1.002000: sched_wakeup_new: comm=new pid=900 prio=20 target_cpu=000
         bio-20 (   20) [002] .... 1.003000: bio_latency: op=R path=/data/a.db latency_us=2500 bytes=4096
          fs-21 (   21) [002] .... 1.004000: file_system: syscall=read path=/data/a.db duration_ms=3.5 bytes=1024
       fault-22 (   22) [002] .... 1.005000: page_fault_user: operation=major address=0x1234 duration_us=150 size=4096
     ability-23 (   23) [003] .... 1.006000: ability_monitor: domain=AAFWK event_name=AbilityStart metric=latency_ms value=12.5 category=foreground
      xpower-24 (   24) [004] .... 1.007000: xpower_cpu: component=CPU energy=8.2 usage=73 scene=foreground
       hisys-25 (   25) [005] .... 1.008000: hi_sysevent: domain=POWER eventname=THERMAL_REPORT type=STAT value=hot level=MINOR
       cache-26 (   26) [006] .... 1.009000: mm_filemap_add_to_page_cache: dev 260:84 ino 0x1 page=0 pfn=1 ofs=0
	`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.0, TimeEnd: 1.01, Limit: 20})
	if len(stats.BIOResources) != 1 || len(stats.FilesystemResources) != 1 || len(stats.PageFaultResources) != 1 ||
		len(stats.AbilityEvents) != 1 || len(stats.XPowerEvents) != 1 || len(stats.HiSystemEvents) != 1 {
		t.Fatalf("unrelated tid=900 reuse erased a direct resource/plugin family: %+v caveats=%v", stats, stats.Caveats)
	}
	for _, token := range []string{
		"thread_identity_bio_resource_fail_closed=true",
		"thread_identity_filesystem_resource_fail_closed=true",
		"thread_identity_page_fault_resource_fail_closed=true",
		"thread_identity_ability_monitor_plugin_fail_closed=true",
		"thread_identity_xpower_plugin_fail_closed=true",
		"thread_identity_hi_sysevent_plugin_fail_closed=true",
	} {
		if containsSubstring(stats.Caveats, token) {
			t.Fatalf("unrelated reuse emitted family identity caveat %q: %v", token, stats.Caveats)
		}
	}
	// The page-cache/inode composite still depends on several PID-keyed input
	// maps and deliberately keeps the old global boundary until completeness
	// is propagated across every derived consumer.
	if len(stats.PageCacheByInode) != 0 || stats.TopIOInodes != nil ||
		!containsSubstring(stats.Caveats, "thread_identity_resource_fail_closed=true") {
		t.Fatalf("composite resource lane was widened with the direct summaries: page=%+v top=%+v caveats=%v", stats.PageCacheByInode, stats.TopIOInodes, stats.Caveats)
	}
}

func TestReusedDirectResourceContributorSuppressesOnlyItsFamily(t *testing.T) {
	idx := buildTraceIndex(t, "identity_direct_resource_contributor.systrace", `
          idle-1 (    1) [000] .... 1.001000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=old next_pid=42 next_prio=20
         old-42 (   42) [002] .... 1.002000: bio_latency: op=R path=/data/old.db latency_us=2500 bytes=4096
       creator-7 (    7) [001] .... 1.003000: sched_wakeup_new: comm=new pid=42 prio=20 target_cpu=000
         new-42 (   42) [002] .... 1.004000: bio_latency: op=R path=/data/new.db latency_us=3000 bytes=4096
          fs-21 (   21) [002] .... 1.005000: file_system: syscall=read path=/data/a.db duration_ms=3.5 bytes=1024
       fault-22 (   22) [002] .... 1.006000: page_fault_user: operation=major address=0x1234 duration_us=150 size=4096
     ability-23 (   23) [003] .... 1.007000: ability_monitor: domain=AAFWK event_name=AbilityStart metric=latency_ms value=12.5 category=foreground
      xpower-24 (   24) [004] .... 1.008000: xpower_cpu: component=CPU energy=8.2 usage=73 scene=foreground
       hisys-25 (   25) [005] .... 1.009000: hi_sysevent: domain=POWER eventname=THERMAL_REPORT type=STAT value=hot level=MINOR
	`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.0, TimeEnd: 1.01, Limit: 20})
	if len(stats.BIOResources) != 0 || !containsSubstring(stats.Caveats, "thread_identity_bio_resource_fail_closed=true") {
		t.Fatalf("reused bio contributor must fail the bio family closed: bio=%+v caveats=%v", stats.BIOResources, stats.Caveats)
	}
	if len(stats.FilesystemResources) != 1 || len(stats.PageFaultResources) != 1 ||
		len(stats.AbilityEvents) != 1 || len(stats.XPowerEvents) != 1 || len(stats.HiSystemEvents) != 1 {
		t.Fatalf("bio contributor conflict suppressed independent direct families: %+v caveats=%v", stats, stats.Caveats)
	}
	for _, token := range []string{
		"thread_identity_filesystem_resource_fail_closed=true",
		"thread_identity_page_fault_resource_fail_closed=true",
		"thread_identity_ability_monitor_plugin_fail_closed=true",
		"thread_identity_xpower_plugin_fail_closed=true",
		"thread_identity_hi_sysevent_plugin_fail_closed=true",
	} {
		if containsSubstring(stats.Caveats, token) {
			t.Fatalf("bio-only conflict emitted independent family caveat %q: %v", token, stats.Caveats)
		}
	}
}

func TestReusedPluginContributorSuppressesOnlyItsFamily(t *testing.T) {
	idx := buildTraceIndex(t, "identity_plugin_contributor.systrace", `
          idle-1 (    1) [000] .... 1.001000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=old next_pid=42 next_prio=20
         old-42 (   42) [003] .... 1.002000: ability_monitor: domain=AAFWK event_name=OldAbility metric=latency_ms value=12.5 category=foreground
       creator-7 (    7) [001] .... 1.003000: sched_wakeup_new: comm=new pid=42 prio=20 target_cpu=000
         new-42 (   42) [003] .... 1.004000: ability_monitor: domain=AAFWK event_name=NewAbility metric=latency_ms value=8.0 category=foreground
         bio-20 (   20) [002] .... 1.005000: bio_latency: op=R path=/data/a.db latency_us=2500 bytes=4096
      xpower-24 (   24) [004] .... 1.006000: xpower_cpu: component=CPU energy=8.2 usage=73 scene=foreground
       hisys-25 (   25) [005] .... 1.007000: hi_sysevent: domain=POWER eventname=THERMAL_REPORT type=STAT value=hot level=MINOR
	`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.0, TimeEnd: 1.01, Limit: 20})
	if len(stats.AbilityEvents) != 0 || !containsSubstring(stats.Caveats, "thread_identity_ability_monitor_plugin_fail_closed=true") {
		t.Fatalf("reused ability contributor must fail only ability summaries closed: ability=%+v caveats=%v", stats.AbilityEvents, stats.Caveats)
	}
	if len(stats.BIOResources) != 1 || len(stats.XPowerEvents) != 1 || len(stats.HiSystemEvents) != 1 {
		t.Fatalf("ability contributor conflict suppressed independent families: %+v caveats=%v", stats, stats.Caveats)
	}
	for _, token := range []string{
		"thread_identity_bio_resource_fail_closed=true",
		"thread_identity_xpower_plugin_fail_closed=true",
		"thread_identity_hi_sysevent_plugin_fail_closed=true",
	} {
		if containsSubstring(stats.Caveats, token) {
			t.Fatalf("ability-only conflict emitted independent family caveat %q: %v", token, stats.Caveats)
		}
	}
}

func TestDirectResourceAndPluginContributorSetsFailClosedOnLifecycleAuditCap(t *testing.T) {
	idx := buildTraceIndex(t, "identity_direct_resource_cap.systrace", ebpfResourceTrace+pluginResourceTrace)
	idx.threadIncarnationFailuresCapped = true
	stats := ComputeWindowStats(idx, Query{TimeStart: 8.0, TimeEnd: 9.03, Limit: 20})
	if len(stats.BIOResources) != 0 || len(stats.FilesystemResources) != 0 || len(stats.PageFaultResources) != 0 ||
		len(stats.AbilityEvents) != 0 || len(stats.XPowerEvents) != 0 || len(stats.HiSystemEvents) != 0 {
		t.Fatalf("capped lifecycle audit must fail every non-empty direct family closed: %+v", stats)
	}
	for _, token := range []string{
		"thread_identity_bio_resource_fail_closed=true",
		"thread_identity_filesystem_resource_fail_closed=true",
		"thread_identity_page_fault_resource_fail_closed=true",
		"thread_identity_ability_monitor_plugin_fail_closed=true",
		"thread_identity_xpower_plugin_fail_closed=true",
		"thread_identity_hi_sysevent_plugin_fail_closed=true",
		"lifecycle_audit_truncated",
	} {
		if !containsSubstring(stats.Caveats, token) {
			t.Fatalf("capped lifecycle audit lost caveat %q: %v", token, stats.Caveats)
		}
	}
}

func TestLifecycleAuditCapDoesNotSuppressPIDlessOrInventEmptyDirectFamilyCaveats(t *testing.T) {
	idx := buildTraceIndex(t, "identity_empty_direct_resource_cap.systrace", `
	     bio-0 (    0) [002] .... 1.001000: bio_latency: op=R path=/data/kernel.db latency_us=2500 bytes=4096
	  ability-0 (    0) [003] .... 1.002000: ability_monitor: domain=AAFWK event_name=KernelAbility metric=latency_ms value=12.5 category=system
	`)
	idx.threadIncarnationFailuresCapped = true
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.0, TimeEnd: 1.01, Limit: 20})
	if len(stats.BIOResources) != 1 || len(stats.AbilityEvents) != 1 {
		t.Fatalf("PID-less direct families have no numeric identity dependency and must survive an unrelated audit cap: %+v", stats)
	}
	for _, token := range []string{
		"thread_identity_bio_resource_fail_closed=true",
		"thread_identity_filesystem_resource_fail_closed=true",
		"thread_identity_page_fault_resource_fail_closed=true",
		"thread_identity_ability_monitor_plugin_fail_closed=true",
		"thread_identity_xpower_plugin_fail_closed=true",
		"thread_identity_hi_sysevent_plugin_fail_closed=true",
	} {
		if containsSubstring(stats.Caveats, token) {
			t.Fatalf("PID-less/empty family emitted invented lifecycle caveat %q: %v", token, stats.Caveats)
		}
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
