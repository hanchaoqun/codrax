package types

// TraceSchedulerWaitPartitionTeaching distinguishes the native scheduler
// state from the engine's exclusive accounting buckets. It complements the
// existing FormatTargetStateAccountCaliber/TraceUninterruptibleWaitMS contract;
// it neither creates a new measurement nor authorizes a causal conclusion.
// Keep query preview, finite-scope and final-recap guidance on this same
// definition rather than inferring physical-state absence from one bucket.
const TraceSchedulerWaitPartitionTeaching = "Scheduler wait buckets are disjoint accounting categories, not mutually exclusive physical meanings: `d_state`/`d_state_occurrences` describe the non-IO D bucket; `io_wait`/`io_wait_occurrences` describe D-opened intervals with a paired kernel IO-wait marker, so both retain D-state provenance. A zero non-IO D bucket alone does not prove absence of D-state waiting. Preserve published buckets and count each interval only once. `sleep_io_wait`/`sleep_iowait_occurrences` remain interruptible S-state waiting, not D-state; this refinement is already inside sleep and is not an extra duration. These definitions apply only to typed scheduler-state accounts, not device/request IO latency or independently completion-closed IO blocking. Missing or incomplete state evidence cannot prove absence."
