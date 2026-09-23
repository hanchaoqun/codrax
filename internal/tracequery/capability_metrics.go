package tracequery

import "strings"

// Published measurement families, not a promise that every input carries them.
// Paths refer to the existing Result wire shape and are checked against it.
func capabilityMetricDescriptors() []MetricCapability {
	o := func(section, fields, unit, caliber string) CapabilityOutput {
		return CapabilityOutput{section, strings.Fields(fields), unit, caliber}
	}
	r := func(all string, alternatives [][]string, optional, conditions string) CapabilityRequirements {
		return CapabilityRequirements{strings.Fields(all), alternatives, strings.Fields(optional), []string{conditions}}
	}
	m := func(id, summary string, outputs []CapabilityOutput, requirements CapabilityRequirements, limit string) MetricCapability {
		return MetricCapability{ID: id, Summary: summary, Outputs: outputs, Requirements: requirements, Limitations: []string{limit}}
	}
	metrics := []MetricCapability{
		m("event_inventory", "Matched events, original coordinates and finite-search coverage.", []CapabilityOutput{
			o("event_search_coverage", "matched_total emitted", "count", "matched population versus bounded display"),
			o("window_stats", "event_counts", "count", "recognized event-family counts in the selected window"),
			o("events", "ts", "seconds", "artifact-local event timestamp"),
		}, r("", nil, "", "A recognized timestamped event or explicit raw-visibility carrier; inspect scan scope and cancellation."), "Raw visibility is inventory-only. No matches is not proof of absent event instrumentation or a missing measurement equal to zero."),
		m("reported_jank", "Source-reported jank marker fields and exact integer filtering.", []CapabilityOutput{
			o("events.jank_event.values", "start_ts_ns end_ts_ns reported_duration_ns", "ns", "reported endpoints/difference on the same source trace axis, not a new independent frame measurement"),
			o("events.jank_event.values", "jank_frames", "count", "source-reported frame count, not recomputed missed deadlines"),
			o("events.jank_event.values", "appid", "identifier", "producer application identifier, not inferred scheduler TID"),
		}, r("trace_mark", nil, "", "Exact B|owner|jank_event_sync: grammar with valid nonnegative int64 start_ts/end_ts/jank_frames/appid; malformed fields remain unavailable. Filter predicates are ANDed against typed fields without floating-point conversion."), "Reported marker time may differ from both payload endpoints. Nanosecond endpoints already share the trace clock; converting to seconds does not prove scheduler identity, a causal edge, refresh rate or an independently measured jank verdict."),
		m("native_protocol_fields", "Exact native frame, async-sample and eBPF payload measures.", []CapabilityOutput{
			o("events.frame_gpu", "timestamp_ns duration_ns", "ns", "source GPU-row timestamp and duration"),
			o("events.ebpf_interval", "timestamp_ns end_timestamp_ns duration_ns", "ns", "source-declared eBPF interval; independent identity status"),
			o("events.perf_napi_async", "event_count", "count", "source async-sample event count"),
		}, r("", [][]string{{"frame_gpu"}, {"frame_map"}, {"frame_callstack"}, {"ebpf_interval"}, {"perf_napi_async"}}, "", "Validated native conversion payload and its source row/clock/identity metadata."), "GPU duration is not CPU running. Native row/callchain IDs do not establish resolved source calls or scheduler dependencies; unsupported payloads remain raw inventory."),
		m("event_density", "Streaming bucket inventory.", []CapabilityOutput{
			o("window_sweep.coverage", "sched_switches sched_wakeups d_state_entries irq_entries trace_marks", "count", "events in this coverage row's declared start_ts/end_ts and buckets; folded rows span multiple grid buckets"),
		}, r("", [][]string{{"sched_switch"}, {"sched_wakeup"}, {"sched_waking"}, {"irq"}, {"trace_mark"}}, "", "Timestamped scan coverage and a bounded grid; target counts additionally need an exact TID."), "Arrival/event density is not time occupancy, queue depth, IO concurrency or a causal rank."),
		m("scheduler_states", "Scheduler intervals and the target's window-clamped state account.", []CapabilityOutput{
			o("target_window_states", "running_ms runnable_ms sleep_ms d_state_ms io_wait_ms total_ms", "ms", "measured per-thread wall-clock state partition"),
			o("target_window_states", "sleep_io_wait_ms", "ms", "IO-marked subset already inside sleep_ms; not an addend"),
		}, r("sched_switch", nil, "sched_wakeup sched_waking sched_blocked_reason", "Unique thread incarnation, ordered state transitions, explicit or derived window; head and CPU continuity must be inspected."), "d_state_ms is the non-IO D accounting lane, not the physical D total. IO markers do not identify a device. Raw S/D states and classified IO accounts are distinct; uncovered time stays unknown."),
		m("cpu_occupancy", "Per-CPU busy/idle and thread/process occupancy.", []CapabilityOutput{
			o("window_stats.cpu", "busy_ms idle_ms", "ms", "per-CPU measured wall-clock occupancy"),
			o("window_stats.top_running", "duration_ms", "ms", "thread running duration within its published source/window"),
		}, r("sched_switch", nil, "sched_wakeup", "CPU identity and governing scheduler transitions; read busy_idle_status and coverage."), "Cross-thread/cross-CPU sums are CPU time, not one target's elapsed latency. A frequency-only CPU row has unavailable busy/idle, not measured zero."),
		m("cpu_pressure", "Per-CPU runnable backlog and priority-scoped competition context.", []CapabilityOutput{
			o("window_stats.cpu_pressure", "runnable_wait_ms", "CPU·ms", "cross-thread runnable-wait sum on this CPU, not one thread's wall latency"),
			o("window_stats.cpu_pressure", "runnable_wait_density", "ratio", "runnable wait sum / explicit window wall time; average backlog, not peak"),
			o("window_stats.cpu_pressure", "high_priority_running_ms high_priority_running_overlap_ms", "ms", "separate background running and displacement-overlap time on this CPU"),
		}, r("sched_switch", nil, "sched_wakeup sched_waking", "CPU-consistent runnable segments, bounded denominator and event-time priority identity for priority-specific conclusions."), "Harmony flavor uses larger values as higher priority (1–40 CFS, 41–159 RT); >159 remains raw system/kernel. Android/generic retain raw semantics. Unknown or inferred priority is not inversion proof; this is not an exact peak runnable-depth series."),
		m("cpu_frequency", "CPU frequency samples, residency and explicit limits.", []CapabilityOutput{
			o("window_stats.cpu", "frequency", "kHz", "CPU frequency sample, not generic clock activity"),
			o("window_stats.cpu.frequency_residency", "frequency", "kHz", "governing CPU frequency tier"),
			o("window_stats.cpu.frequency_residency", "duration_ms", "ms", "frequency tier residency"),
		}, r("cpu_frequency", nil, "cpu_frequency_limits cpu_idle", "CPU identity; temporal predecessor or disclosed fallback; any shared-cluster reuse needs its reported membership source."), "clock_set_rate alone is not a CPU frequency measurement. Missing frequency is unknown; kHz must not be relabeled MHz/Hz. Complete idle-state × frequency analysis is not provided here."),
		m("cpu_constraints", "Thread affinity/cpuset epochs and proven runnable overlap.", []CapabilityOutput{
			o("window_stats.cpu_constraints", "runnable_wait_ms", "ms", "runnable interval overlap with the published constraint account"),
		}, r("", [][]string{{"cpu_constraint"}, {"sched_switch"}}, "sched_wakeup", "Exact thread/epoch and a supported constraint payload or kernel next_info snapshot; causal overlap needs its separate proof."), "An affinity mask alone is not restriction or priority-inversion proof. Harmony priority ranges require Harmony flavor; Android/generic priorities retain their raw semantics."),
		m("compute_supply", "Frequency-weighted compute and separately measured wall-clock mismatch.", []CapabilityOutput{
			o("window_stats.compute_supply_balance", "nominal_capacity_ms delivered_compute_ms low_frequency_loss_ms core_limited_ms", "CPU·ms", "cross-CPU additive compute; core_limited_ms is an approximate remainder"),
			o("window_stats.compute_supply_balance", "window_ms idle_mismatch_ms", "ms", "wall-clock window and idle-with-runnable overlap"),
			o("window_stats.compute_supply_balance", "supply_ratio", "ratio", "delivered / nominal capacity"),
		}, r("sched_switch", nil, "sched_wakeup cpu_frequency cpu_frequency_limits", "Bounded window and observed CPU set; inspect frequency_known, topology and approximation disclosures."), "Unknown frequency may use a disclosed neutral weight; it is not proof of full supply. The computed deficit is not automatically removable latency."),
		m("blocked_reasons", "Kernel blocked-reason occurrences and separately supplied delay.", []CapabilityOutput{
			o("window_stats.blocked_reason_census", "count", "count", "records for the declared target TID"),
			o("window_stats.blocked_reason_census.callers", "delay_total_ms", "ms", "vendor delay sum only when all caller rows supply it"),
		}, r("sched_blocked_reason", nil, "sched_switch sched_wakeup", "Caller and iowait-known state are independent; Harmony vendor delay is converted from microseconds where supplied."), "Record count/delay is not a scheduler interval count/duration. A recorded caller is not a waited object, owner or device."),
		m("sched_accounting", "Kernel scheduler accounting corroboration.", []CapabilityOutput{
			o("window_stats.sched_stat_accounting", "total_delay_ms total_runtime_ms total_vruntime_ms", "ms", "normalized raw accounting counters, separately labeled"),
		}, r("sched_stat", nil, "sched_switch", "A recognized wait/sleep/iowait/blocked/runtime accounting format and exact target TID."), "Accounting deltas do not replace sched_switch wall-clock intervals; virtual runtime is not elapsed runtime."),
		m("io_request_latency", "Complete-pair latency distributions within each existing storage identity group.", []CapabilityOutput{
			o("window_stats.storage_latency_by_layer.request_latency_distribution", "sample_count", "count", IORequestLatencySamplePolicy),
			o("window_stats.storage_latency_by_layer.request_latency_distribution", "min_ms max_ms mean_ms p50_ms p90_ms p95_ms p99_ms", "ms", IORequestLatencyCaliber+"; "+IORequestLatencyQuantileMethod),
			o("window_stats.io_latencies", "duration_ms", "ms", BlockIOWaitCaliberIssueToComplete+" or "+BlockIOWaitCaliberBIOQueueToComplete+" as declared by the row"),
		}, r("", [][]string{{"block_rq_issue", "block_rq_complete"}, {"block_bio_queue", "block_bio_complete"}, {"storage"}, {"filesystem"}}, "sched_switch sched_wakeup sched_blocked_reason", "storage/filesystem name parsed event families, not arbitrary raw prefixes: only start/done profiles admitted by the shared endpoint decoder (including F2FS direct-IO, sync-file, write-begin/end; MMC, SCSI, Android/ext4 profiles) with strict payloads and unique source/layer/request identity can contribute. An inventory event alone is insufficient; ambiguous or incomplete pairs never enter quantiles."), "All samples in each group precede detail truncation. No cross-group/all-layer rollup, P99 averaging or exact in-flight depth is provided. Request residence is not issuer-blocked time and not causal proof; absence differs from measured zero."),
		m("file_io", "Inode IO counts, bytes and explicitly scoped latency summaries.", []CapabilityOutput{
			o("window_stats.file_io_by_inode", "count completion_count", "count", "recognized records by source/dev/inode/thread/operation"),
			o("window_stats.file_io_by_inode", "bytes", "bytes", "published record accounting, not wall-clock bandwidth"),
			o("window_stats.file_io_by_inode", "total_latency_ms max_latency_ms", "ms", "same-group latency total and maximum"),
		}, r("", [][]string{{"filesystem"}, {"storage"}}, "", "Recognized file IO payload with dev/inode and operation; exact pair requirements apply to paired latency."), "Inode or entry_name does not prove a full path. Cross-thread inode latency is not additive wall time. Summary bytes must not be assumed to be unique throughput bytes; sizes do not prove random/sequential access."),
		m("page_cache", "Observed inode page-cache add/delete inventory.", []CapabilityOutput{
			o("window_stats.page_cache_by_inode", "adds deletes churn", "count", "observed cache events"),
			o("window_stats.page_cache_by_inode", "bytes", "bytes", "published size accounting where known"),
		}, r("memory", nil, "", "Recognized page-cache operation and physical source/dev/inode identity."), "Adds minus deletes is not a complete cache occupancy/lifecycle proof; page size, missing endpoints and prior state must not be guessed."),
		m("io_pressure", "IO event/request pressure context.", []CapabilityOutput{
			o("window_stats", "block_issue_count block_complete_count block_remap_count", "count", "observed rows, not in-flight concurrency"),
		}, r("", [][]string{{"block_rq_issue"}, {"block_rq_complete"}, {"storage"}, {"filesystem"}}, "sched_blocked_reason", "Same source and selected query scope; inspect each pressure summary's own population and coverage."), "Pressure or burst counts do not prove a target dependency, exact IO depth, IOPS, wall-clock bandwidth or device bottleneck."),
		m("interrupt_activity", "IRQ/softirq/IPI inventory and qualified paired active time.", []CapabilityOutput{
			o("window_stats.irq_activity", "count paired_count", "count", "entry/exit inventory and accepted pairs"),
			o("window_stats.irq_activity", "active_ms max_active_ms window_overlap_ms", "ms", "paired active intervals and selected-window overlap"),
			o("window_stats.irq_bursts", "span_ms", "ms", "first-to-last inventory envelope, not active duration"),
		}, r("", [][]string{{"irq"}, {"softirq"}, {"ipi"}}, "sched_switch", "Recognized matching entry/exit family on the same CPU/source for active duration."), "An ipi_raise is an instant, not an active interval. Burst envelopes and cross-CPU sums must not masquerade as target elapsed time."),
		m("workqueue", "Exact workqueue execution-pair activity.", []CapabilityOutput{
			o("window_stats.workqueue_activity", "duration_ms max_latency_ms", "ms", "qualified execute_start to execute_end pairs"),
		}, r("workqueue", nil, "", "Supported execute endpoints and exact source/work identity; unique pairing."), "Unpaired and ambiguous cohorts remain disclosed and do not acquire execution duration or a causal wakeup edge."),
		m("dma_fence", "Exact fence-wait-pair activity.", []CapabilityOutput{
			o("window_stats.dma_fence_activity", "wait_ms max_wait_ms", "ms", "qualified wait_start to wait_end pairs"),
		}, r("dma_fence", nil, "", "Supported endpoints with matching source/driver/timeline/context/seqno identity."), "Fence wait is not GPU execution duration or proof that GPU completion caused the target wakeup."),
		m("trace_spans", "Closed synchronous B/E and asynchronous S/F intervals.", []CapabilityOutput{
			o("span_windows", "start_ts end_ts", "seconds", "original span endpoints"),
			o("span_windows", "duration_ms", "ms", "closed native span duration"),
		}, r("", [][]string{{"trace_mark"}, {"trace_async_interval"}}, "", "Valid B/E stack, unique S/F owner/name/cookie, or an admitted explicit complete trace_async_interval; endpoint completeness and scope disclosed."), "No duration from an unmatched B/S. A business label alone does not prove mechanism, execution ownership or causal relation."),
		m("track_spans", "G/H logical-track intervals.", []CapabilityOutput{
			o("window_stats.trace_track_spans", "duration_ms actual_duration_ms", "ms", "selected projection versus original complete logical-track duration"),
		}, r("trace_mark", nil, "", "Valid G/H pairing with exact logical owner/track/name/cookie identity."), "Emitter threads do not become execution owners of the logical track; these spans do not mint root-rank authority."),
		m("trace_instants", "I/N point-marker inventory.", []CapabilityOutput{
			o("window_stats.trace_instants", "ts", "seconds", "artifact-local instant"),
		}, r("trace_mark", nil, "", "Valid I or N point marker with its payload owner and track distinction."), "Point markers have no duration; nearby points do not form an elapsed interval by implication."),
		m("trace_counters", "C marker values preserved with declared owner and unknown unit.", []CapabilityOutput{
			o("window_stats.trace_counters", "value", "unknown", "literal counter value; the wire carries no unit"),
			o("window_stats.trace_counters", "count", "count", "observed records"),
		}, r("trace_mark", nil, "", "Valid C counter and exact physical source/payload owner/name; emitter and owner are distinct."), "Counter names cannot establish bytes, frequency or another unit."),
		m("counter_deltas", "Finite numeric C-series differences within the query window.", []CapabilityOutput{
			o("window_stats.counter_deltas", "first last min max delta", "unknown", "in_window_first_sample baseline; unit_status unknown"),
		}, r("trace_mark", nil, "", "Exact counter series identity and all admitted values finite."), "No pre-window baseline or zero is invented; invalid series are disclosed rather than silently repaired."),
		m("runtime_resources", "Source-explicit resource inventory, sizes and latency where carried.", []CapabilityOutput{
			o("events", "resource_bytes", "bytes", "explicit recognized resource size"),
			o("events", "resource_latency_ms", "ms", "explicit recognized resource latency"),
			o("window_stats.filesystem_resources", "count", "count", "resource record occurrences"),
			o("window_stats.filesystem_resources", "bytes", "bytes", "explicit size field, not arbitrary resource count"),
			o("window_stats.filesystem_resources", "total_latency_ms max_latency_ms", "ms", "explicit recognized latency field"),
		}, r("", [][]string{{"filesystem"}, {"storage"}, {"memory"}, {"trace_mark"}}, "", "A supported resource payload; inspect kind/operation, nullable size/end time and source units."), "Native-hook I counts, C cumulative values, byte sizes and nullable lifetimes are different measures. A callchain key is not a resolved source call or leak proof."),
		m("plugin_inventory", "Ability, power and system-event payload inventory.", []CapabilityOutput{
			o("window_stats.ability_events", "count", "count", "recognized plugin records"),
			o("window_stats.xpower_events", "value", "source_declared_or_unknown", "verbatim plugin metric value"),
		}, r("", [][]string{{"ability_monitor"}, {"xpower"}, {"hi_sysevent"}, {"power"}}, "", "A supported plugin payload and its source-provided metric semantics."), "Plugin presence/value does not prove measured energy, device state, or a causal explanation without that specific protocol."),
		m("perf_samples", "Counts and independent event/weight cohorts with symbolization quality.", []CapabilityOutput{
			o("perf_stats", "sample_count", "count", "observed samples"),
			o("window_stats.perf_samples", "sample_count", "count", "same-window observed samples"),
			o("perf_timeline.buckets", "sample_count", "count", "observed samples in each bucket"),
			o("perf_stats.top_symbols", "period", "cohort.weight_unit", "sample weights in the same event/unit cohort"),
			o("perf_stats.top_symbols", "percent", "percent", "share of that cohort's eligible sample weight"),
		}, r("perf_sample", nil, "sched_switch", "Source/event/weight-unit cohort; inspect thread generation, clock alignment, integer aggregation and symbolization quality."), "cycles, instructions, nanoseconds and counts cannot share a denominator. Legacy top_symbols/period mirrors are available only for a single exact cohort; multiple cohorts retain their own weights in cohorts[]. Sampling weight is not elapsed execution duration; unresolved symbols/callchains and unknown CPU remain unknown."),
		m("perf_timeline", "Bucketed sample counts and unit-preserving weights.", []CapabilityOutput{
			o("perf_timeline", "bucket_ms", "ms", "timeline bucket width"),
			o("perf_timeline.buckets", "sample_count", "count", "samples in the bucket"),
			o("perf_timeline.buckets", "period", "cohort.weight_unit", "only exact compatible cohort aggregates"),
		}, r("perf_sample", nil, "", "Valid sample timestamps and independent event/unit/source cohorts."), "For multiple cohorts read each cohorts[] weight separately; a missing legacy period mirror does not mean missing samples. A sample bucket is neither CPU occupancy nor a proof that an absent sample means zero execution."),
		m("state_churn", "Measured state-switch activity and accumulated state intervals.", []CapabilityOutput{
			o("window_stats.state_churn", "running_ms runnable_ms sleep_ms", "ms", "published per-thread state totals"),
		}, r("sched_switch", nil, "sched_wakeup sched_waking", "Continuous thread identity and measured state intervals."), "Frequent transitions are not measured context-switch overhead or recoverable compute cost."),
		m("vsync_inventory", "Generator event/wakeup census and source-supplied pacing period.", []CapabilityOutput{
			o("events", "ts", "seconds", "generator occurrence timestamps; census retains its own scope"),
		}, r("", [][]string{{"trace_mark"}, {"sched_wakeup"}}, "sched_switch", "A recognized generator identity; any authoritative period must be supplied by its own supported payload."), "Event count is not frame count, FPS or dropped frames. Native jank start_ts_ns/end_ts_ns use the same trace axis after ns-to-seconds conversion, not an automatic clock-domain shift."),
		m("frame_spans", "Frame-like complete span candidates.", []CapabilityOutput{
			o("span_windows", "duration_ms", "ms", "complete frame-like span duration"),
		}, r("", [][]string{{"trace_mark"}, {"trace_async_interval"}}, "frame_map frame_callstack frame_gpu", "Complete native span and qualified thread/process ownership; official frame metadata needs its own identity match."), "A name match does not supply refresh rate, deadline, dropped-frame verdict or complete display pipeline coverage."),
		m("render_phases", "Measured render phase intervals.", []CapabilityOutput{
			o("frame_pipeline.items", "duration_ms", "ms", "complete phase-span duration"),
		}, r("", [][]string{{"trace_mark"}, {"trace_async_interval"}}, "frame_map frame_callstack frame_gpu", "Recognized frame/render spans with source-local identity and complete endpoints."), "Phase classification is not a measured cross-process call relationship."),
		m("frame_timeline", "Frame phase intervals and original coordinates.", []CapabilityOutput{
			o("frame_timeline.items", "start_ts end_ts", "seconds", "trace-axis phase endpoints"),
			o("frame_timeline.items", "duration_ms", "ms", "complete phase duration"),
		}, r("", [][]string{{"trace_mark"}, {"trace_async_interval"}}, "frame_map frame_callstack frame_gpu", "Admitted phase span and typed role/source identity; inspect role authority and process membership."), "Native GPU payload nanoseconds are not CPU running time. Unknown roles/CPU ownership are not inferred from proximity."),
		m("frame_flow", "Published frame relation kind and temporal separation.", []CapabilityOutput{
			o("frame_timeline.flows", "latency_ms", "ms", "reported phase separation under its relation_kind"),
		}, r("", [][]string{{"trace_mark"}, {"trace_async_interval"}}, "", "Distinct complete phase intervals in the selected source scope."), "Default sorted adjacency is temporal_sequence with causality_unproven; it cannot become a causal pipeline edge."),
		m("scheduler_latency", "Runnable-wait distribution and interval examples.", []CapabilityOutput{
			o("scheduler_latency_stats", "count", "count", "measured runnable intervals"),
			o("scheduler_latency_stats", "mean_ms p50_ms p95_ms p99_ms max_ms", "ms", "runnable scheduling delay"),
		}, r("sched_switch", nil, "sched_wakeup sched_waking cpu_frequency", "Runnable entry from wakeup or preemption and subsequent run; unique incarnation and continuity."), "Runnable wait excludes preceding sleep. Unknown CPU continuity does not remove CPU-independent runnable wall time or grant CPU-specific attribution."),
		m("binder_relations", "Binder send/receive relations and explicit call semantics.", []CapabilityOutput{
			o("ipc_graph.edges", "latency_ms", "ms", "qualified send/receive latency, when available"),
		}, r("binder_transaction binder_transaction_received", nil, "binder_reply binder_transaction_alloc_buf", "Exact transaction/source identity and admitted endpoints; inspect flags, destination and receiver-source known state."), "A oneway or unresolved call is not synchronous blocking; no missing receiver/reply is invented."),
		m("wakeup_relations", "Directed native wakeup edges and their state context.", []CapabilityOutput{
			o("wakeup_chain.edges", "wakeup_ts", "seconds", "native wakeup time on the artifact clock"),
			o("wakeup_chain.causal_impacts", "projected_impact_ms actual_impact_ms", "ms", "selected-window projection versus complete native state interval, not ranked effective attribution"),
		}, r("sched_switch", [][]string{{"sched_wakeup"}, {"sched_waking"}}, "binder_transaction sched_blocked_reason", "Unique target incarnation and time-consistent directed wakeup chain; inspect path completeness and expansion limits."), "Waker identity proves a recorded edge, not its lock/device/business mechanism or all missing upstream causes."),
		m("root_attribution", "Ranked effective attribution alongside raw/projected/cumulative quantities.", []CapabilityOutput{
			o("root_cause_rank.items", "projected_impact_ms cumulative_impact_ms effective_impact_ms", "ms", "distinct row-declared projection, accumulation and attribution calibers"),
		}, r("sched_switch", nil, "sched_wakeup sched_waking trace_mark sched_blocked_reason cpu_frequency", "Target/window/rank-board identity plus the existing category-specific admission and causal proofs."), "Attribution is not guaranteed removable latency. Context/background, target-self symptoms, semantic intervals and CAP deficits keep their separate authority and cannot be inferred merely from this catalog."),
		m("blocking_candidates", "Typed blocking candidates and counterpart state where available.", []CapabilityOutput{
			o("critical_blocking_calls.items", "duration_ms", "ms", "candidate's measured or explicitly supplied interval"),
		}, r("", [][]string{{"trace_mark"}, {"trace_async_interval"}, {"sched_switch"}, {"block_rq_issue", "block_rq_complete"}, {"block_bio_queue", "block_bio_complete"}, {"binder_transaction", "binder_transaction_received"}, {"memory"}}, "sched_wakeup sched_blocked_reason", "Each candidate kind has separate requirements: scheduler D/IO intervals, qualified request pairs with issuer-state closure, span/contention payloads, binder joins or resource inventory; counterpart identity and causal classification need their own proof."), "Inventory alone has no measured duration or dependency. A callsite or lock-like name does not prove ownership, dependency or device cause."),
		m("interactions", "Directional peer wakeup/binder event counts.", []CapabilityOutput{
			o("interaction_stats.items", "wakeups_to_target wakeups_from_target binder_to_target binder_from_target total_interactions", "count", "source/window/target/direction-scoped interactions"),
		}, r("", [][]string{{"sched_wakeup"}, {"sched_waking"}, {"binder_transaction"}}, "binder_transaction_received", "Exact target and resolved peer identity with the requested direction."), "More events do not prove greater elapsed impact or causal rank."),
		m("evidence_composition", "Reuse already qualified measurement and relation components.", []CapabilityOutput{
			o("evidence_pack", "summary", "not_applicable", "line-backed component finding, no new numeric ruler"),
		}, r("", nil, "", "The selected recipe/pack components retain their own event, identity, window and coverage requirements."), "Composition neither creates missing measurements nor permits cross-source/cross-window evidence substitution."),
	}
	for i := range metrics {
		if metrics[i].ID == "reported_jank" {
			metrics[i].Filter = &CapabilityFilter{View: "event_search", Parameter: "event_field_filters", Fields: EventFieldFilterFields(), Operators: EventFieldFilterOps(), MaxPredicates: EventFieldFilterLimit}
		}
	}
	return metrics
}
