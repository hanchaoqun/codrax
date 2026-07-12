package tracequery

// standard_event_names.go — S1b (STAB-1 软修复轮稳定化, 2026-07-12): the
// CLOSED SET of standard Linux kernel / ftrace / Android / OpenHarmony trace
// event names. SINGLE SOURCE: prose-vocabulary consumers (the orchestrator's
// lexicon gate) read this constant list so a report that names a standard
// kernel event (the 2779 witness: block_rq_issue / irq_handler_entry /
// irq_handler_exit — real event names visible in any kernel documentation
// and in the attached trace itself) is never flagged as a fabricated token.
//
// P3-3 勘正 (2026-07-12): binder_transaction_reply (an engine parse ALIAS —
// kernel replies are binder_transaction rows with reply=1, no such
// tracepoint exists) and cpu_capacity (vendor-class evidence rows, not a
// power/sched tracepoint name) were removed — no standard-source evidence.
//
// Closed-set discipline: explicit names only, never prefixes or patterns —
// adding a family means listing its members here (精确信号纪律: the
// vocabulary check is set membership, and a prefix rule would admit
// arbitrary fabricated suffixes).
var standardTraceEventNames = []string{
	// Scheduler family.
	"sched_switch", "sched_wakeup", "sched_waking", "sched_wakeup_new",
	"sched_blocked_reason", "sched_migrate_task", "sched_process_exit",
	"sched_process_fork", "sched_process_exec", "sched_process_free",
	"sched_process_wait", "sched_stat_runtime", "sched_stat_sleep",
	"sched_stat_wait", "sched_stat_blocked", "sched_stat_iowait",
	"sched_pi_setprio",
	// Binder family (Android / OpenHarmony).
	"binder_transaction", "binder_transaction_received",
	"binder_transaction_alloc_buf", "binder_lock", "binder_locked",
	"binder_unlock", "binder_set_priority", "binder_command",
	"binder_return",
	// Block IO family.
	"block_rq_issue", "block_rq_complete", "block_rq_insert",
	"block_rq_requeue", "block_bio_queue", "block_bio_complete",
	"block_bio_remap", "block_bio_backmerge", "block_bio_frontmerge",
	"block_getrq", "block_plug", "block_unplug", "block_rq_remap",
	// IRQ / softirq / IPI family.
	"irq_handler_entry", "irq_handler_exit", "softirq_entry",
	"softirq_exit", "softirq_raise", "ipi_entry", "ipi_exit", "ipi_raise",
	// Workqueue family.
	"workqueue_execute_start", "workqueue_execute_end",
	"workqueue_activate_work", "workqueue_queue_work",
	// CPU / power / clock family.
	"cpu_idle", "cpu_frequency", "cpu_frequency_limits", "clock_set_rate",
	"clk_set_rate", "clk_enable", "clk_disable",
	// Task lifecycle.
	"task_rename", "task_newtask",
	// Memory / page-cache family.
	"mm_filemap_add_to_page_cache", "mm_filemap_delete_from_page_cache",
	"mm_page_alloc", "mm_page_free", "mm_vmscan_direct_reclaim_begin",
	"mm_vmscan_direct_reclaim_end", "rss_stat", "ion_heap_grow",
	"ion_heap_shrink", "oom_score_adj_update",
	// DMA fence / GPU sync family.
	"dma_fence_init", "dma_fence_emit", "dma_fence_signaled",
	"dma_fence_wait_start", "dma_fence_wait_end", "dma_fence_enable_signal",
	"dma_fence_destroy",
	// Filesystem families commonly present in mobile traces.
	"ext4_da_write_begin", "ext4_da_write_end", "ext4_sync_file_enter",
	"ext4_sync_file_exit", "f2fs_sync_file_enter", "f2fs_sync_file_exit",
	"f2fs_write_begin", "f2fs_write_end", "f2fs_readpage", "f2fs_readpages",
	"f2fs_dataread_start", "f2fs_dataread_end",
	// ftrace bookkeeping rows.
	"tracing_mark_write", "print",
}

// StandardTraceEventNameUniverse returns the closed set above (copy — the
// single source stays immutable).
func StandardTraceEventNameUniverse() []string {
	return append([]string(nil), standardTraceEventNames...)
}
