package tracequery

import (
	"math"
	"sync"
	"time"
)

const ParserVersion = "tracequery-v12"

type EventType string

const (
	EventUnknown            EventType = "unknown"
	EventSchedSwitch        EventType = "sched_switch"
	EventSchedWakeup        EventType = "sched_wakeup"
	EventSchedWaking        EventType = "sched_waking"
	EventSchedBlockedReason EventType = "sched_blocked_reason"
	EventSchedStat          EventType = "sched_stat"
	EventCPUIdle            EventType = "cpu_idle"
	EventCPUFrequency       EventType = "cpu_frequency"
	EventCPUFrequencyLimit  EventType = "cpu_frequency_limits"
	EventCPUConstraint      EventType = "cpu_constraint"
	EventClockSetRate       EventType = "clock_set_rate"
	EventBlockIssue         EventType = "block_rq_issue"
	EventBlockRemap         EventType = "block_bio_remap"
	EventBlockComplete      EventType = "block_rq_complete"
	EventBinderTransaction  EventType = "binder_transaction"
	EventBinderReceived     EventType = "binder_transaction_received"
	EventBinderAllocBuf     EventType = "binder_transaction_alloc_buf"
	EventBinderLock         EventType = "binder_lock"
	EventBinderLocked       EventType = "binder_locked"
	EventBinderUnlock       EventType = "binder_unlock"
	EventBinderReply        EventType = "binder_reply"
	EventIRQ                EventType = "irq"
	EventSoftIRQ            EventType = "softirq"
	EventIPI                EventType = "ipi"
	EventTraceMark          EventType = "trace_mark"
	EventMemory             EventType = "memory"
	EventStorage            EventType = "storage"
	EventFilesystem         EventType = "filesystem"
	EventPower              EventType = "power"
	EventAbilityMonitor     EventType = "ability_monitor"
	EventXPower             EventType = "xpower"
	EventHiSystemEvent      EventType = "hi_sysevent"
	EventWorkqueue          EventType = "workqueue"
	EventDMAFence           EventType = "dma_fence"
	EventPerfSample         EventType = "perf_sample"
)

type TracePlatform string

const (
	TracePlatformAuto    TracePlatform = "auto"
	TracePlatformHarmony TracePlatform = "harmony"
	TracePlatformAndroid TracePlatform = "android"
	TracePlatformDonghu  TracePlatform = "donghu"
	TracePlatformGeneric TracePlatform = "generic"
)

type Event struct {
	Line int       `json:"line"`
	Ts   float64   `json:"ts"`
	CPU  int       `json:"cpu,omitempty"`
	Type EventType `json:"type"`
	Name string    `json:"name,omitempty"`

	Comm string `json:"comm,omitempty"`
	PID  int    `json:"pid,omitempty"`
	TGID int    `json:"tgid,omitempty"`

	PrevComm            string `json:"prev_comm,omitempty"`
	PrevPID             int    `json:"prev_pid,omitempty"`
	PrevPrio            int    `json:"prev_prio,omitempty"`
	PrevPrioClass       string `json:"prev_prio_class,omitempty"`
	PrevState           string `json:"prev_state,omitempty"`
	NextComm            string `json:"next_comm,omitempty"`
	NextPID             int    `json:"next_pid,omitempty"`
	NextPrio            int    `json:"next_prio,omitempty"`
	NextPrioClass       string `json:"next_prio_class,omitempty"`
	NextInfo            string `json:"next_info,omitempty"`
	NextInfoAffinity    string `json:"next_info_affinity,omitempty"`
	NextInfoAllowedCPUs []int  `json:"next_info_allowed_cpus,omitempty"`
	NextInfoLoad        int    `json:"next_info_load,omitempty"`
	NextInfoGroup       int    `json:"next_info_group,omitempty"`
	NextInfoRestricted  bool   `json:"next_info_restricted,omitempty"`
	NextInfoExpel       int    `json:"next_info_expel,omitempty"`
	NextInfoCGID        int    `json:"next_info_cgid,omitempty"`
	CGroup              string `json:"cgroup,omitempty"`

	WakeeComm      string `json:"wakee_comm,omitempty"`
	WakeePID       int    `json:"wakee_pid,omitempty"`
	WakeePrio      int    `json:"wakee_prio,omitempty"`
	WakeePrioClass string `json:"wakee_prio_class,omitempty"`
	TargetCPU      int    `json:"target_cpu,omitempty"`

	ConstraintComm       string `json:"constraint_comm,omitempty"`
	ConstraintPID        int    `json:"constraint_pid,omitempty"`
	ConstraintKind       string `json:"constraint_kind,omitempty"`
	ConstraintPolicy     string `json:"constraint_policy,omitempty"`
	ConstraintCPU        int    `json:"constraint_cpu,omitempty"`
	ConstraintCPUValid   bool   `json:"-"`
	ConstraintOrigCPU    int    `json:"constraint_orig_cpu,omitempty"`
	ConstraintOrigCPUSet bool   `json:"-"`
	ConstraintDestCPU    int    `json:"constraint_dest_cpu,omitempty"`
	ConstraintDestCPUSet bool   `json:"-"`
	AllowedCPUsText      string `json:"allowed_cpus_text,omitempty"`
	AllowedCPUs          []int  `json:"allowed_cpus,omitempty"`
	CPUSet               string `json:"cpuset,omitempty"`

	State            int    `json:"state,omitempty"`
	Frequency        int    `json:"frequency,omitempty"`
	FrequencyMin     int    `json:"frequency_min,omitempty"`
	FrequencyMax     int    `json:"frequency_max,omitempty"`
	CPUForField      int    `json:"cpu_for_field,omitempty"`
	CPUForFieldValid bool   `json:"cpu_for_field_valid,omitempty"`
	ClockName        string `json:"clock_name,omitempty"`
	Reason           string `json:"reason,omitempty"`
	IOWait           int    `json:"io_wait,omitempty"`
	SchedStatKind    string `json:"sched_stat_kind,omitempty"`
	SchedStatComm    string `json:"sched_stat_comm,omitempty"`
	SchedStatPID     int    `json:"sched_stat_pid,omitempty"`
	SchedStatDelayNs int64  `json:"sched_stat_delay_ns,omitempty"`
	SchedStatRunNs   int64  `json:"sched_stat_runtime_ns,omitempty"`
	SchedStatVRunNs  int64  `json:"sched_stat_vruntime_ns,omitempty"`
	SpanAction       string `json:"span_action,omitempty"`
	SpanPID          int    `json:"span_pid,omitempty"`
	SpanName         string `json:"span_name,omitempty"`
	SpanValue        string `json:"span_value,omitempty"`

	BinderTransactionID int    `json:"binder_transaction_id,omitempty"`
	BinderDestProc      int    `json:"binder_dest_proc,omitempty"`
	BinderDestThread    int    `json:"binder_dest_thread,omitempty"`
	BinderReply         int    `json:"binder_reply,omitempty"`
	BinderFlags         string `json:"binder_flags,omitempty"`
	BinderCode          string `json:"binder_code,omitempty"`
	BinderDebugID       int    `json:"binder_debug_id,omitempty"`
	BinderDataSize      int64  `json:"binder_data_size,omitempty"`
	BinderOffsetsSize   int64  `json:"binder_offsets_size,omitempty"`
	BinderExtraSize     int64  `json:"binder_extra_size,omitempty"`
	BinderLockTag       string `json:"binder_lock_tag,omitempty"`

	BlockDev       string `json:"block_dev,omitempty"`
	BlockOp        string `json:"block_op,omitempty"`
	BlockSector    int64  `json:"block_sector,omitempty"`
	BlockLen       int64  `json:"block_len,omitempty"`
	BlockError     string `json:"block_error,omitempty"`
	BlockSrcDev    string `json:"block_src_dev,omitempty"`
	BlockSrcSector int64  `json:"block_src_sector,omitempty"`

	IRQName string `json:"irq_name,omitempty"`
	IRQID   int    `json:"irq_id,omitempty"`

	IPITargetMask string `json:"ipi_target_mask,omitempty"`
	IPITargetCPUs []int  `json:"ipi_target_cpus,omitempty"`

	MemoryKind    string `json:"memory_kind,omitempty"`
	SubsystemKind string `json:"subsystem_kind,omitempty"`

	ResourcePath      string  `json:"resource_path,omitempty"`
	ResourceOp        string  `json:"resource_op,omitempty"`
	ResourceLatencyMs float64 `json:"resource_latency_ms,omitempty"`
	ResourceBytes     int64   `json:"resource_bytes,omitempty"`
	ResourceAddress   string  `json:"resource_address,omitempty"`
	ResourceCallstack string  `json:"resource_callstack,omitempty"`

	FSDev       string `json:"fs_dev,omitempty"`
	Inode       string `json:"inode,omitempty"`
	ParentInode string `json:"parent_inode,omitempty"`
	EntryName   string `json:"entry_name,omitempty"`
	FileOffset  int64  `json:"file_offset,omitempty"`
	FileLen     int64  `json:"file_len,omitempty"`
	FileRW      string `json:"file_rw,omitempty"`
	FileRet     int64  `json:"file_ret,omitempty"`
	FileSize    int64  `json:"file_size,omitempty"`

	PluginDomain    string `json:"plugin_domain,omitempty"`
	PluginEventName string `json:"plugin_event_name,omitempty"`
	PluginMetric    string `json:"plugin_metric,omitempty"`
	PluginValue     string `json:"plugin_value,omitempty"`
	PluginCategory  string `json:"plugin_category,omitempty"`

	PerfPID                 int    `json:"perf_pid,omitempty"`
	PerfTID                 int    `json:"perf_tid,omitempty"`
	PerfComm                string `json:"perf_comm,omitempty"`
	PerfPeriod              int64  `json:"perf_period,omitempty"`
	PerfEvent               string `json:"perf_event,omitempty"`
	PerfSymbol              string `json:"perf_symbol,omitempty"`
	PerfDSO                 string `json:"perf_dso,omitempty"`
	PerfIP                  string `json:"perf_ip,omitempty"`
	PerfAddr                string `json:"perf_addr,omitempty"`
	PerfSampleID            string `json:"perf_sample_id,omitempty"`
	PerfStreamID            string `json:"perf_stream_id,omitempty"`
	PerfRawWeight           int64  `json:"perf_raw_weight,omitempty"`
	PerfDataSrc             string `json:"perf_data_src,omitempty"`
	PerfTransaction         string `json:"perf_transaction,omitempty"`
	PerfPhysAddr            string `json:"perf_phys_addr,omitempty"`
	PerfCGroupID            string `json:"perf_cgroup_id,omitempty"`
	PerfDataPageSize        int64  `json:"perf_data_page_size,omitempty"`
	PerfCodePageSize        int64  `json:"perf_code_page_size,omitempty"`
	PerfRawSize             int64  `json:"perf_raw_size,omitempty"`
	PerfBranchCount         int64  `json:"perf_branch_count,omitempty"`
	PerfUserRegsABI         string `json:"perf_user_regs_abi,omitempty"`
	PerfUserRegsCount       int64  `json:"perf_user_regs_count,omitempty"`
	PerfUserStackSize       int64  `json:"perf_user_stack_size,omitempty"`
	PerfAuxSize             int64  `json:"perf_aux_size,omitempty"`
	PerfCallchain           string `json:"perf_callchain,omitempty"`
	PerfSource              string `json:"perf_source,omitempty"`
	PerfSampleKind          string `json:"perf_sample_kind,omitempty"`
	PerfSymbolizationStatus string `json:"perf_symbolization_status,omitempty"`
	PerfClock               string `json:"perf_clock,omitempty"`
	PerfCPUKnown            *bool  `json:"perf_cpu_known,omitempty"`
	PerfClockConfidence     string `json:"perf_clock_confidence,omitempty"`
	PerfCallchainStatus     string `json:"perf_callchain_status,omitempty"`

	FieldText string `json:"field_text,omitempty"`
}

type Index struct {
	Path             string
	Size             int64
	ModTime          time.Time
	LineCount        int
	ScannedLineCount int
	Windowed         bool
	IndexTimeStart   float64
	IndexTimeEnd     float64
	IndexLineStart   int
	IndexLineEnd     int
	Events           []Event
	FirstTs          float64
	LastTs           float64
	ParsedKnown      int
	// ParseLinePanics counts lines whose parse panicked (malformed
	// artifact input is untrusted; one bad line must not kill the
	// query). ClockRegressions counts events whose timestamp moved
	// backwards relative to the previous parsed event — typed input
	// for the query layer's caveat, never a hard gate.
	ParseLinePanics int
	// RetainedStringBytes accumulates the ACTUAL bytes of strings
	// retained by this index (interner contents plus per-event unique
	// strings). eventSizeBytes (unsafe.Sizeof) counts only string
	// headers, so cache accounting without this underestimates real
	// memory by up to 2x on payload-heavy traces. Telemetry + cache
	// cost input only — never a gate signal.
	RetainedStringBytes int64
	ClockRegressions    int
	// UnparsedLines counts non-empty scanned lines that matched no known
	// trace line format (ParseLine returned no event, without panicking)
	// — typed input for the query layer's coverage caveat, never a hard
	// gate.
	UnparsedLines    int
	TraceFlavor      TraceFlavor
	FlavorConfidence float64
	FlavorSignals    []string
	Caveats          []string
	// PaddingTruncated marks a windowed build whose MaxEvents budget was hit
	// only inside the safety padding tail: the parse observed zero clock
	// regressions and the budget-tripping event's ts lies STRICTLY beyond the
	// requested TimeEnd, so monotonicity proves the core [TimeStart,TimeEnd]
	// window lost zero events. Typed input for the query layer's
	// compaction/caveat note — never a hard gate, and the build succeeds
	// instead of returning IndexEventLimitError.
	PaddingTruncated bool
	// PaddingTruncatedNote is the verbatim display note for PaddingTruncated
	// (indexPaddingTruncatedNoteFmt rendered with the real parse boundary);
	// display surfaces fold it as-is.
	PaddingTruncatedNote string
	// PaddingTruncatedLastTs is the parse boundary at the degrade point —
	// idx.LastTs when the budget tripped, i.e. the timestamp parsing actually
	// reached before the padding tail was cut. Typed input for query-layer
	// caveats (> TimeEnd by construction); zero when PaddingTruncated is
	// false.
	PaddingTruncatedLastTs float64
	// tidTgidVoteOnce/tidTgidVote back the B-3 (§7.11) per-index tid→tgid
	// soft derivation (trace_mark span-pid majority vote for TGID-column-less
	// hmtrace shapes). Lazily built once by derivedTidTgid(); non-exported
	// and never serialized — display-layer grouping enrichment only, never a
	// filter or gate input. Index is pointer-only throughout the package, so
	// the sync.Once is copy-safe.
	tidTgidVoteOnce sync.Once
	tidTgidVote     *tidTgidDerivation
}

type Query struct {
	View                   string
	Thread                 string
	ThreadInput            string
	ThreadPIDInferred      bool
	PID                    int
	TimeStart              float64
	TimeEnd                float64
	TimeStartSet           bool
	TimeEndSet             bool
	LineStart              int
	LineEnd                int
	EventTypes             []EventType
	Pattern                string
	SpanName               string
	FrameWindowAutoDerived bool
	InteractionDirection   string
	RecipeName             string
	MaxDepth               int
	MaxBranches            int
	MinDurationMs          float64
	// ViaThread is the RN-14a (§7.9) wakeup_chain via selector: same forms as
	// the thread selector (bare pid, pid=N, "comm-pid", or a full thread
	// name). Matching is canonical-exact only (parsed pid integer equality,
	// otherwise verbatim comm equality) — never substring/fuzzy. Branches of
	// the target's chain tree whose wakeup subtree contains the via thread
	// are immune to the MaxBranches top-N cap, and ChainResult.ViaThread
	// reports whether the via thread is ON a wakeup path to the target
	// (per-hop latency) or only a scheduling-contention neighbor.
	ViaThread          string
	IncludeWindowStats bool
	Limit              int
	// BucketMs is the view=window_sweep coverage bucket width in
	// milliseconds; StreamWindowSweep clamps it via ClampWindowSweepBucketMs
	// (default 100, allowed 50..500). Ignored by every other view.
	BucketMs              float64
	CoreTopology          string
	TraceFlavor           TraceFlavor
	TraceFlavorHint       TraceFlavor
	TraceFlavorHintSource string
	TracePlatform         TracePlatform
	TracePlatformHint     TracePlatform
	TracePlatformSource   string
}

type Result struct {
	View                        string                  `json:"view"`
	SourcePath                  string                  `json:"source_path"`
	TraceFlavor                 string                  `json:"trace_flavor,omitempty"`
	Platform                    string                  `json:"platform,omitempty"`
	PlatformCandidate           string                  `json:"platform_candidate,omitempty"`
	PlatformCandidateConfidence float64                 `json:"platform_candidate_confidence,omitempty"`
	PlatformCandidateSignals    []string                `json:"platform_candidate_signals,omitempty"`
	FlavorConfidence            float64                 `json:"trace_flavor_confidence,omitempty"`
	FlavorSignals               []string                `json:"trace_flavor_signals,omitempty"`
	FrameworkMode               string                  `json:"framework_mode,omitempty"`
	FrameworkSurfaces           []FrameworkSurface      `json:"framework_surfaces,omitempty"`
	TimeUnit                    string                  `json:"time_unit,omitempty"`
	PrioritySemantics           string                  `json:"priority_semantics,omitempty"`
	LineCount                   int                     `json:"line_count,omitempty"`
	ScannedLineCount            int                     `json:"scanned_line_count,omitempty"`
	IndexWindowed               bool                    `json:"index_windowed,omitempty"`
	IndexTimeStart              float64                 `json:"index_time_start,omitempty"`
	IndexTimeEnd                float64                 `json:"index_time_end,omitempty"`
	IndexLineStart              int                     `json:"index_line_start,omitempty"`
	IndexLineEnd                int                     `json:"index_line_end,omitempty"`
	EventCount                  int                     `json:"event_count,omitempty"`
	UnparsedLineCount           int                     `json:"unparsed_line_count,omitempty"`
	ParseLinePanics             int                     `json:"parse_line_panics,omitempty"`
	ClockRegressions            int                     `json:"clock_regressions,omitempty"`
	TimeStart                   float64                 `json:"time_start,omitempty"`
	TimeEnd                     float64                 `json:"time_end,omitempty"`
	Events                      []EventView             `json:"events,omitempty"`
	Timeline                    *TimelineResult         `json:"timeline,omitempty"`
	WindowStats                 *WindowStats            `json:"window_stats,omitempty"`
	SchedulerLatency            *SchedulerLatencyResult `json:"scheduler_latency_stats,omitempty"`
	IPCGraph                    *IPCGraphResult         `json:"ipc_graph,omitempty"`
	WakeupChain                 *ChainResult            `json:"wakeup_chain,omitempty"`
	SpanWindows                 []TraceSpanSummary      `json:"span_windows,omitempty"`
	FramePipeline               *FramePipelineResult    `json:"frame_pipeline,omitempty"`
	FrameTimeline               *FrameTimelineResult    `json:"frame_timeline,omitempty"`
	CriticalBlocking            *CriticalBlockingResult `json:"critical_blocking_calls,omitempty"`
	RootCauseRank               *RootCauseRankResult    `json:"root_cause_rank,omitempty"`
	FrameRootCauseBundle        *FrameRootCauseBundle   `json:"frame_root_cause_bundle,omitempty"`
	InteractionStats            *InteractionStatsResult `json:"interaction_stats,omitempty"`
	PerfStats                   *PerfContext            `json:"perf_stats,omitempty"`
	PerfTimeline                *PerfTimelineResult     `json:"perf_timeline,omitempty"`
	WindowSweep                 *WindowSweepResult      `json:"window_sweep,omitempty"`
	Recipe                      *RecipeResult           `json:"recipe,omitempty"`
	EvidencePack                []EvidenceFact          `json:"evidence_pack,omitempty"`
	Caveats                     []string                `json:"caveats,omitempty"`
	// Compactions are the typed truncation records for this result (E4).
	// They ride ALONGSIDE the prose compaction caveats (which stay verbatim);
	// the tool refinement layer reads these first and keeps caveat-substring
	// matching only as a fallback for paths not yet publishing typed records.
	Compactions []ViewCompaction `json:"compactions,omitempty"`
}

type FrameworkSurface struct {
	Surface        string      `json:"surface"`
	ProcessCount   int         `json:"process_count,omitempty"`
	ExampleThreads []ThreadRef `json:"example_threads,omitempty"`
	Signals        []string    `json:"signals,omitempty"`
}

type EventView struct {
	Event
	Raw string `json:"raw,omitempty"`
}

type ThreadRef struct {
	Comm string `json:"comm,omitempty"`
	PID  int    `json:"pid,omitempty"`
	TGID int    `json:"tgid,omitempty"`
}

type ThreadState string

const (
	StateRunning  ThreadState = "running"
	StateRunnable ThreadState = "runnable"
	StateSSleep   ThreadState = "s_sleep"
	StateDSleep   ThreadState = "d_sleep"
	StateIOWait   ThreadState = "io_wait"
	// StateStopped / StateDead (§7.11 B-1, customer_dead_session_audit_
	// 20260703.md): T/t (SIGSTOP / ptrace-stopped) and X/Z (exit-dead /
	// zombie) prev_state codes. Typed non-Unknown classes so the segments
	// enter interval-level faces honestly; the per-state lane accumulators
	// (running/runnable/sleep/d_state/io_wait) intentionally skip them —
	// stopped/dead time is neither scheduling demand nor compute delivery
	// pressure, and booking it into any existing lane would be a semantic
	// lie. Raw codes stay drillable via Interval.PrevStateRaw.
	StateStopped ThreadState = "stopped"
	StateDead    ThreadState = "dead"
	StateUnknown ThreadState = "unknown"
)

type Interval struct {
	Thread     ThreadRef   `json:"thread"`
	State      ThreadState `json:"state"`
	StartTs    float64     `json:"start_ts"`
	EndTs      float64     `json:"end_ts"`
	DurationMs float64     `json:"duration_ms"`
	// CPU/CPUKnown record which CPU a RUNNING interval executed on (from the
	// sched_switch-in event). Only set by builders that see the switch event;
	// consumers must treat CPUKnown=false as "unknown", never as CPU 0. Feeds
	// the R5d weak-core gate for priority-inversion impact (§7.30.1).
	CPU              int     `json:"cpu,omitempty"`
	CPUKnown         bool    `json:"cpu_known,omitempty"`
	ActualStartTs    float64 `json:"actual_start_ts,omitempty"`
	ActualEndTs      float64 `json:"actual_end_ts,omitempty"`
	ActualDurationMs float64 `json:"actual_duration_ms,omitempty"`
	StartLine        int     `json:"start_line,omitempty"`
	EndLine          int     `json:"end_line,omitempty"`
	WakeupLine       int     `json:"wakeup_line,omitempty"`
	PrevStateRaw     string  `json:"prev_state_raw,omitempty"`
	Summary          string  `json:"summary,omitempty"`
}

type TimelineResult struct {
	Thread    ThreadRef  `json:"thread"`
	Window    TimeWindow `json:"window"`
	Intervals []Interval `json:"intervals"`
	Caveats   []string   `json:"caveats,omitempty"`
}

type TimeWindow struct {
	StartTs float64 `json:"start_ts,omitempty"`
	EndTs   float64 `json:"end_ts,omitempty"`
}

type WindowStats struct {
	Window       TimeWindow        `json:"window"`
	EventCounts  map[EventType]int `json:"event_counts,omitempty"`
	CPU          []CPUStats        `json:"cpu,omitempty"`
	CoreTopology []CoreClassStats  `json:"core_topology,omitempty"`
	// ClusterFrequencyCeilings is the CFC (§7.10 VS-2c 设计) single-point
	// per-cluster fmax snapshot for this window (VS-2b ladder per cluster:
	// window-governing limits Max > highest governed observed sample),
	// computed once so the scattered fmax consumers share one source.
	// INTERNAL computation structure, not an observation face: json:"-"
	// keeps it off every JSON surface and out of the causal token registry
	// (CFC ruling: no new token). Sole display consumer: the soft
	// window_stats stanza line (writeTraceClusterFrequencyCeilings,
	// internal/tool/trace_query.go).
	ClusterFrequencyCeilings []ClusterFrequencyCeiling  `json:"-"`
	TopRunning               []ThreadDuration           `json:"top_running,omitempty"`
	RunnableTop              []ThreadDuration           `json:"runnable_top,omitempty"`
	SleepTop                 []ThreadDuration           `json:"sleep_top,omitempty"`
	DStateTop                []ThreadDuration           `json:"d_state_top,omitempty"`
	IOWaitTop                []ThreadDuration           `json:"io_wait_top,omitempty"`
	CPUPressure              []CPUPressureStats         `json:"cpu_pressure,omitempty"`
	CPUConstraints           []CPUConstraintSummary     `json:"cpu_constraints,omitempty"`
	ThreadCPULoad            []ThreadCPULoadSummary     `json:"thread_cpu_load,omitempty"`
	ProcessCPULoad           []ProcessCPULoadSummary    `json:"process_cpu_load,omitempty"`
	RunnableContext          []RunnableContextSummary   `json:"runnable_context,omitempty"`
	IOLatencies              []IOLatencySummary         `json:"io_latencies,omitempty"`
	CPUFrequencyLimits       []CPUFrequencyLimit        `json:"cpu_frequency_limits,omitempty"`
	SubsystemEvents          []SubsystemEventSummary    `json:"subsystem_events,omitempty"`
	BlockIssueCount          int                        `json:"block_issue_count,omitempty"`
	BlockRemapCount          int                        `json:"block_remap_count,omitempty"`
	BlockCompleteCount       int                        `json:"block_complete_count,omitempty"`
	BinderCount              int                        `json:"binder_count,omitempty"`
	BinderReceivedCount      int                        `json:"binder_received_count,omitempty"`
	BinderAuxCount           int                        `json:"binder_aux_count,omitempty"`
	IRQCount                 int                        `json:"irq_count,omitempty"`
	SoftIRQCount             int                        `json:"softirq_count,omitempty"`
	MemoryEventCount         int                        `json:"memory_event_count,omitempty"`
	StorageEventCount        int                        `json:"storage_event_count,omitempty"`
	FilesystemEventCount     int                        `json:"filesystem_event_count,omitempty"`
	PowerEventCount          int                        `json:"power_event_count,omitempty"`
	AbilityEventCount        int                        `json:"ability_event_count,omitempty"`
	XPowerEventCount         int                        `json:"xpower_event_count,omitempty"`
	HiSystemEventCount       int                        `json:"hi_sysevent_event_count,omitempty"`
	WorkqueueEventCount      int                        `json:"workqueue_event_count,omitempty"`
	DMAFenceEventCount       int                        `json:"dma_fence_event_count,omitempty"`
	BlockedReasonCount       int                        `json:"blocked_reason_count,omitempty"`
	SchedStatCount           int                        `json:"sched_stat_count,omitempty"`
	IPICount                 int                        `json:"ipi_count,omitempty"`
	IOWaitBlockedCount       int                        `json:"io_wait_blocked_count,omitempty"`
	BlockedReasons           []BlockedReasonSummary     `json:"blocked_reasons,omitempty"`
	TraceSpans               []TraceSpanSummary         `json:"trace_spans,omitempty"`
	TraceCounters            []TraceCounterSummary      `json:"trace_counters,omitempty"`
	CounterDeltas            []TraceCounterDeltaSummary `json:"counter_deltas,omitempty"`
	IRQBursts                []IRQBurstSummary          `json:"irq_bursts,omitempty"`
	MemoryKinds              []MemoryKindSummary        `json:"memory_kinds,omitempty"`
	BIOResources             []RuntimeResourceSummary   `json:"bio_resources,omitempty"`
	FilesystemResources      []RuntimeResourceSummary   `json:"filesystem_resources,omitempty"`
	PageFaultResources       []RuntimeResourceSummary   `json:"page_fault_resources,omitempty"`
	FileIOByInode            []FileIOSummary            `json:"file_io_by_inode,omitempty"`
	PageCacheByInode         []PageCacheSummary         `json:"page_cache_by_inode,omitempty"`
	StorageLatencyByLayer    []StorageLatencySummary    `json:"storage_latency_by_layer,omitempty"`
	IOPressureSummary        *IOPressureSummary         `json:"io_pressure_summary,omitempty"`
	IOBurstEpisodes          []IOBurstEpisodeSummary    `json:"io_burst_episodes,omitempty"`
	BlockIOByInode           []BlockIOByInodeSummary    `json:"block_io_by_inode,omitempty"`
	IRQActivity              []InterruptActivity        `json:"irq_activity,omitempty"`
	SoftIRQActivity          []InterruptActivity        `json:"softirq_activity,omitempty"`
	IPIActivity              []InterruptActivity        `json:"ipi_activity,omitempty"`
	WorkqueueActivity        []WorkqueueActivity        `json:"workqueue_activity,omitempty"`
	DMAFenceActivity         []DMAFenceActivity         `json:"dma_fence_activity,omitempty"`
	SchedStatAccounting      []SchedStatSummary         `json:"sched_stat_accounting,omitempty"`
	SupplyPressureSummary    *SupplyPressureSummary     `json:"supply_pressure_summary,omitempty"`
	TraceMarkCategories      []TraceMarkCategory        `json:"trace_mark_categories,omitempty"`
	AsyncFileWork            []AsyncFileWorkSummary     `json:"async_file_work,omitempty"`
	AbilityEvents            []TracePluginSummary       `json:"ability_events,omitempty"`
	XPowerEvents             []TracePluginSummary       `json:"xpower_events,omitempty"`
	HiSystemEvents           []TracePluginSummary       `json:"hi_sysevent_events,omitempty"`
	ThreadDrifts             []ThreadDriftSummary       `json:"thread_drifts,omitempty"`
	ComputeSupply            []ComputeSupplySummary     `json:"compute_supply,omitempty"`
	// CPUOccupancy is the CMP-8 (§7.1) occupancy-side decomposition of the
	// selected window: who actually consumed the CPUs (top running threads,
	// per-process running rollup, per-CPU top occupiers, priority-band
	// running split). Built from the SAME sched_switch running segments that
	// feed TopRunning/CPUPressure — never a second timing pass. Nil when the
	// window exposes no running segments.
	CPUOccupancy *CPUOccupancyStats `json:"cpu_occupancy,omitempty"`
	// ComputeSupplyBalance is the CMP-10 (§7.4) true supply-side metric:
	// frequency-weighted delivered compute Σ(running×f/fmax) against the
	// nominal capacity (window×cpus), with the supply-gap three-way
	// decomposition (low-frequency loss / idle-vs-runnable mismatch /
	// core-limited remainder). Nil when the query window is unbounded — the
	// nominal denominator would be an estimate (CMP-3: no window, no
	// estimate).
	ComputeSupplyBalance *ComputeSupplyBalance     `json:"compute_supply_balance,omitempty"`
	StateChurn           []ThreadStateChurnSummary `json:"state_churn,omitempty"`
	StateDrilldownPlan   []StateDrilldownStep      `json:"state_drilldown_plan,omitempty"`
	// IdleWholeWindowSleepers summarizes the top_sleep drilldown candidates
	// folded out of StateDrilldownPlan because their cumulative sleep covered
	// (>=99% of) the entire selected window — idle service threads, not
	// root-cause evidence. Typed input for the display-side one-line fold;
	// never a gate. Nil when nothing was folded.
	IdleWholeWindowSleepers *IdleWholeWindowSleeperFold `json:"idle_whole_window_sleepers,omitempty"`
	PerfSamples             *PerfContext                `json:"perf_samples,omitempty"`
	Caveats                 []string                    `json:"caveats,omitempty"`
}

// IdleWholeWindowSleeperFold is the typed aggregate of whole-window sleeper
// threads dropped from the state-drilldown plan (berlin.systrace 2026-07-03:
// 15+ AudioOut/DNS/FFRT rows at impact=101.000ms in a 101ms window drowned
// the real candidates). Count is exact; Threads carries at most the first 8
// thread labels in SleepTop order so the fold line stays one line.
type IdleWholeWindowSleeperFold struct {
	Count   int      `json:"count"`
	Threads []string `json:"threads,omitempty"`
}

type PerfContext struct {
	SampleCount   int                 `json:"sample_count,omitempty"`
	TotalPeriod   int64               `json:"total_period,omitempty"`
	Quality       *PerfQualitySummary `json:"quality,omitempty"`
	TopSymbols    []PerfHotspot       `json:"top_symbols,omitempty"`
	TopDSO        []PerfHotspot       `json:"top_dso,omitempty"`
	TopCallchains []PerfHotspot       `json:"top_callchains,omitempty"`
	TopThreads    []PerfThreadSummary `json:"top_threads,omitempty"`
	TopEvents     []PerfHotspot       `json:"top_events,omitempty"`
}

type PerfHotspot struct {
	Symbol              string      `json:"symbol,omitempty"`
	DSO                 string      `json:"dso,omitempty"`
	Callchain           string      `json:"callchain,omitempty"`
	Event               string      `json:"event,omitempty"`
	WeightUnit          string      `json:"weight_unit,omitempty"`
	Source              string      `json:"source,omitempty"`
	SymbolizationStatus string      `json:"symbolization_status,omitempty"`
	SampleCount         int         `json:"sample_count,omitempty"`
	Period              int64       `json:"period,omitempty"`
	Percent             float64     `json:"percent,omitempty"`
	Threads             []ThreadRef `json:"threads,omitempty"`
	CPUs                []int       `json:"cpus,omitempty"`
	LineStart           int         `json:"line_start,omitempty"`
	LineEnd             int         `json:"line_end,omitempty"`
	Example             string      `json:"example,omitempty"`
}

type PerfQualitySummary struct {
	Sources               []PerfValueCount `json:"sources,omitempty"`
	SymbolizationStatuses []PerfValueCount `json:"symbolization_statuses,omitempty"`
	SampleKinds           []PerfValueCount `json:"sample_kinds,omitempty"`
	WeightUnits           []PerfValueCount `json:"weight_units,omitempty"`
	Clocks                []PerfValueCount `json:"clocks,omitempty"`
	ClockConfidences      []PerfValueCount `json:"clock_confidences,omitempty"`
	CallchainStatuses     []PerfValueCount `json:"callchain_statuses,omitempty"`
	CPUKnownCount         int              `json:"cpu_known_count,omitempty"`
	CPUUnknownCount       int              `json:"cpu_unknown_count,omitempty"`
	CallchainKnownCount   int              `json:"callchain_known_count,omitempty"`
	CallchainUnknownCount int              `json:"callchain_unknown_count,omitempty"`
	Caveats               []string         `json:"caveats,omitempty"`
}

type PerfValueCount struct {
	Value       string  `json:"value,omitempty"`
	SampleCount int     `json:"sample_count,omitempty"`
	Period      int64   `json:"period,omitempty"`
	Percent     float64 `json:"percent,omitempty"`
}

type PerfThreadSummary struct {
	Thread      ThreadRef `json:"thread,omitempty"`
	SampleCount int       `json:"sample_count,omitempty"`
	Period      int64     `json:"period,omitempty"`
	Percent     float64   `json:"percent,omitempty"`
	CPUs        []int     `json:"cpus,omitempty"`
	LineStart   int       `json:"line_start,omitempty"`
	LineEnd     int       `json:"line_end,omitempty"`
	Example     string    `json:"example,omitempty"`
}

type PerfTimelineResult struct {
	Window   TimeWindow           `json:"window"`
	BucketMs float64              `json:"bucket_ms,omitempty"`
	Buckets  []PerfTimelineBucket `json:"buckets,omitempty"`
	Caveats  []string             `json:"caveats,omitempty"`
}

type PerfTimelineBucket struct {
	StartTs     float64     `json:"start_ts,omitempty"`
	EndTs       float64     `json:"end_ts,omitempty"`
	SampleCount int         `json:"sample_count,omitempty"`
	Period      int64       `json:"period,omitempty"`
	TopSymbol   string      `json:"top_symbol,omitempty"`
	TopDSO      string      `json:"top_dso,omitempty"`
	TopEvent    string      `json:"top_event,omitempty"`
	Threads     []ThreadRef `json:"threads,omitempty"`
	CPUs        []int       `json:"cpus,omitempty"`
	LineStart   int         `json:"line_start,omitempty"`
	LineEnd     int         `json:"line_end,omitempty"`
	Example     string      `json:"example,omitempty"`
}

type SchedulerLatencyResult struct {
	Target      ThreadRef              `json:"target,omitempty"`
	Window      TimeWindow             `json:"window"`
	Count       int                    `json:"count,omitempty"`
	MeanMs      float64                `json:"mean_ms,omitempty"`
	P50Ms       float64                `json:"p50_ms,omitempty"`
	P95Ms       float64                `json:"p95_ms,omitempty"`
	P99Ms       float64                `json:"p99_ms,omitempty"`
	MaxMs       float64                `json:"max_ms,omitempty"`
	Items       []SchedulerLatencyItem `json:"items,omitempty"`
	Caveats     []string               `json:"caveats,omitempty"`
	Compactions []ViewCompaction       `json:"compactions,omitempty"`
}

type SchedulerLatencyItem struct {
	Thread     ThreadRef `json:"thread"`
	StartTs    float64   `json:"start_ts,omitempty"`
	EndTs      float64   `json:"end_ts,omitempty"`
	DurationMs float64   `json:"duration_ms,omitempty"`
	CPU        int       `json:"cpu"`
	CoreClass  string    `json:"core_class,omitempty"`
	// Frequency is the legacy single cpu_frequency sample at the wait start,
	// kept for context only. Low-frequency judgements use WeightedFrequency /
	// ObservedMaxFrequency (methodology audit §7.30.2 R5e).
	Frequency     int    `json:"frequency,omitempty"`
	Priority      int    `json:"priority,omitempty"`
	PriorityClass string `json:"priority_class,omitempty"`
	StartLine     int    `json:"start_line,omitempty"`
	EndLine       int    `json:"end_line,omitempty"`
	// WeightedFrequency is the duration-weighted CPU frequency (kHz) across
	// this wait interval, integrated over cpu_frequency change points
	// (§7.30.2 R5e). Zero when the CPU has no frequency samples at all.
	WeightedFrequency int `json:"weighted_frequency,omitempty"`
	// ObservedMaxFrequency is the max cpu_frequency sample observed inside or
	// nearest to this interval — the low-frequency benchmark (§7.30.2 R5e:
	// never the window-wide residency max).
	ObservedMaxFrequency int `json:"observed_max_frequency,omitempty"`
	// FrequencySample is FrequencySampleNearestFallback when no cpu_frequency
	// sample fell inside the interval, i.e. WeightedFrequency rests on the
	// nearest sample(s) (preceding preferred, following as last resort).
	FrequencySample string  `json:"frequency_sample,omitempty"`
	SameCPUBusyMs   float64 `json:"same_cpu_busy_ms,omitempty"`
	SameCPUIdleMs   float64 `json:"same_cpu_idle_ms,omitempty"`
	OtherCPUIdleMs  float64 `json:"other_cpu_idle_ms,omitempty"`
	// HighPriorityRunningMs is the full-window high-priority running total on
	// this CPU — background pressure with no overlap check (§7.30.2 R5g).
	// Competition claims must use HighPriorityRunningOverlapMs.
	HighPriorityRunningMs float64 `json:"high_priority_running_ms,omitempty"`
	// HighPriorityRunningOverlapMs counts only high-priority running time from
	// other threads that overlapped THIS runnable wait interval — the
	// displacement-evidenced share (§7.30.2 R5g).
	HighPriorityRunningOverlapMs float64 `json:"high_priority_running_overlap_ms,omitempty"`
	// SameCPUTopRunning lists only threads whose running time overlapped this
	// wait interval; DurationMs is the overlapped portion, not the window
	// running total. Serial hand-offs (zero overlap) are excluded (§7.30.2
	// R5g).
	SameCPUTopRunning []ThreadDuration `json:"same_cpu_top_running,omitempty"`
	Summary           string           `json:"summary,omitempty"`
}

// FrequencySampleNearestFallback marks a frequency judgement whose weighted
// value comes entirely from the nearest cpu_frequency sample(s) outside the
// judged segment(s) — preceding preferred, following as last resort — because
// no sample fell inside them (methodology audit §7.30.2 R5e).
const FrequencySampleNearestFallback = "nearest_fallback"

type ComputeSupplySummary struct {
	Thread     ThreadRef `json:"thread,omitempty"`
	State      string    `json:"state,omitempty"`
	CPU        int       `json:"cpu"`
	CoreClass  string    `json:"core_class,omitempty"`
	DurationMs float64   `json:"duration_ms,omitempty"`
	// Frequency is the legacy single sample taken at the last judged segment
	// start, kept for context only. The verdict uses WeightedFrequency /
	// ObservedMaxFrequency (methodology audit §7.30.2 R5e).
	Frequency int `json:"frequency,omitempty"`
	// WeightedFrequency is the duration-weighted CPU frequency (kHz) across
	// the judged running/runnable segments, integrated over cpu_frequency
	// change points (§7.30.2 R5e).
	WeightedFrequency int `json:"weighted_frequency,omitempty"`
	// ObservedMaxFrequency is the max cpu_frequency sample observed inside or
	// nearest to the judged segments — the 0.65× low-frequency benchmark
	// (§7.30.2 R5e: never the window-wide residency max).
	ObservedMaxFrequency int `json:"observed_max_frequency,omitempty"`
	// FrequencySample is FrequencySampleNearestFallback when no cpu_frequency
	// sample fell inside any judged segment.
	FrequencySample string  `json:"frequency_sample,omitempty"`
	CPUBusyMs       float64 `json:"cpu_busy_ms,omitempty"`
	CPUIdleMs       float64 `json:"cpu_idle_ms,omitempty"`
	RunnableWaitMs  float64 `json:"runnable_wait_ms,omitempty"`
	// HighPriorityRunningMs is the full-window background figure for this CPU
	// (§7.30.2 R5g); the verdict pressure term uses
	// HighPriorityRunningOverlapMs instead.
	HighPriorityRunningMs float64 `json:"high_priority_running_ms,omitempty"`
	// HighPriorityRunningOverlapMs counts only high-priority running time from
	// other threads that overlapped THIS thread's runnable waits on the same
	// CPU (displacement evidence, §7.30.2 R5g).
	HighPriorityRunningOverlapMs float64 `json:"high_priority_running_overlap_ms,omitempty"`
	Verdict                      string  `json:"verdict,omitempty"`
	Confidence                   float64 `json:"confidence,omitempty"`
	LineStart                    int     `json:"line_start,omitempty"`
	LineEnd                      int     `json:"line_end,omitempty"`
	Summary                      string  `json:"summary,omitempty"`
}

// CPUOccupancyStats is the CMP-8 (§7.1) occupancy-side answer to "who ate the
// CPU" for one selected window. All RunningMs figures are cpu-time (cpu·ms)
// clipped to the wall-clock window; cross-CPU sums may exceed WindowMs and
// must never be narrated as wall-clock elapsed time. Reuses the window's
// existing sched_switch running segmentation — no second timing pass.
type CPUOccupancyStats struct {
	// WindowMs is the wall-clock length of the selected window (0 when the
	// query window is unbounded; per-row cpu·ms stay valid, ratios do not).
	WindowMs float64 `json:"window_ms,omitempty"`
	// TopThreads ranks threads by running cpu·ms aggregated across CPUs.
	TopThreads []CPUOccupancyThread `json:"top_threads,omitempty"`
	// TopProcesses ranks tgid-level processes by running cpu·ms
	// (ProcessCPULoadSummary reused; RunnableWaitMs stays 0 on this surface —
	// occupancy is strictly the running side).
	TopProcesses []ProcessCPULoadSummary `json:"top_processes,omitempty"`
	// PerCPUTop lists, per CPU, the top occupier threads (at most 2).
	PerCPUTop []CPUOccupancyPerCPU `json:"per_cpu_top,omitempty"`
	// PriorityBands splits running cpu·ms by the platform priority_rule
	// classes (classifyTracePriority) plus the isHighPriorityForPressure
	// verdict — typed platform semantics only, never a heuristic threshold.
	PriorityBands []CPUOccupancyPriorityBand `json:"priority_bands,omitempty"`
	Caveats       []string                   `json:"caveats,omitempty"`
}

type CPUOccupancyThread struct {
	Thread ThreadRef `json:"thread"`
	// RunningMs is cpu-time (cpu·ms) summed across this thread's CPUs.
	RunningMs     float64  `json:"running_ms"`
	CPUs          []int    `json:"cpus,omitempty"`
	CoreClasses   []string `json:"core_classes,omitempty"`
	Priority      int      `json:"priority,omitempty"`
	PriorityClass string   `json:"priority_class,omitempty"`
	LineStart     int      `json:"line_start,omitempty"`
	LineEnd       int      `json:"line_end,omitempty"`
}

type CPUOccupancyPerCPU struct {
	CPU       int     `json:"cpu"`
	CoreClass string  `json:"core_class,omitempty"`
	BusyMs    float64 `json:"busy_ms,omitempty"`
	IdleMs    float64 `json:"idle_ms,omitempty"`
	// Top holds this CPU's top running occupiers (at most 2, cpu·ms).
	Top []ThreadDuration `json:"top,omitempty"`
}

type CPUOccupancyPriorityBand struct {
	// Band is the typed priority class from classifyTracePriority
	// ("ohos_rt", "ohos_cfs", "system_or_kernel",
	// "android_raw_scheduler_prio", "raw_scheduler_prio") or "unclassified"
	// when the segment carried no positive priority.
	Band string `json:"band"`
	// HighPriority mirrors isHighPriorityForPressure for this band's class —
	// the platform priority_rule verdict, not a numeric threshold invented
	// here. Always false on flavors whose raw priority semantics are unknown.
	HighPriority bool    `json:"high_priority"`
	RunningMs    float64 `json:"running_ms"`
	ThreadCount  int     `json:"thread_count,omitempty"`
}

// ComputeSupplyBalance is the CMP-10 (§7.4) window-level supply-side ledger.
// Units are heterogeneous BY DESIGN and every consumer must keep the type
// annotation: NominalCapacityMs / DeliveredComputeMs / LowFrequencyLossMs /
// CoreLimitedMs are cpu·ms (cpu-time, cross-CPU additive), IdleMismatchMs is
// wall-clock ms (a duration during which ∃CPU idle ∧ the global runnable
// queue was non-empty). CoreLimitedMs is an explicit remainder approximation.
type ComputeSupplyBalance struct {
	WindowMs float64 `json:"window_ms,omitempty"`
	// CPUCount counts CPUs observed via sched_switch in the window — the
	// nominal-capacity denominator (unobserved cores cannot be counted).
	CPUCount int `json:"cpu_count,omitempty"`
	// NominalCapacityMs = WindowMs × CPUCount (cpu·ms).
	NominalCapacityMs float64 `json:"nominal_capacity_ms,omitempty"`
	// DeliveredComputeMs = Σ over running segments of dur×f/fmax (cpu·ms),
	// fmax = the max over the frequency samples that GOVERN this window —
	// the head-governing sample (nearest preceding the window start) plus
	// in-window samples, i.e. the window residency timeline — never raw
	// pre-window history. CPUs without any governing sample weigh 1.0 and
	// are flagged via PerCPU[].FrequencyKnown=false + a caveat.
	DeliveredComputeMs float64 `json:"delivered_compute_ms,omitempty"`
	// SupplyRatio = DeliveredComputeMs / NominalCapacityMs.
	SupplyRatio float64 `json:"supply_ratio,omitempty"`
	// LowFrequencyLossMs = Σ running×(1−f/fmax) (cpu·ms).
	LowFrequencyLossMs float64 `json:"low_frequency_loss_ms,omitempty"`
	// IdleMismatchMs is WALL-CLOCK ms with ∃CPU idle ∧ runnable backlog>0 —
	// scheduling/affinity mismatch, not missing capacity. Single O(events)
	// pass over sched_switch/sched_wakeup maintaining per-CPU idle state and
	// a global runnable set.
	IdleMismatchMs float64 `json:"idle_mismatch_ms,omitempty"`
	// CoreLimitedMs ≈ NominalCapacityMs − DeliveredComputeMs −
	// LowFrequencyLossMs − IdleMismatchMs, clamped at 0 — the residual
	// core-count-limited share. Approximation by design (§7.4:
	// 其余=核数受限近似).
	CoreLimitedMs float64                   `json:"core_limited_ms,omitempty"`
	PerCPU        []ComputeSupplyCPUBalance `json:"per_cpu,omitempty"`
	Caveats       []string                  `json:"caveats,omitempty"`
	Summary       string                    `json:"summary,omitempty"`
}

type ComputeSupplyCPUBalance struct {
	CPU       int    `json:"cpu"`
	CoreClass string `json:"core_class,omitempty"`
	// RunningMs is this CPU's busy cpu·ms in the window.
	RunningMs          float64 `json:"running_ms,omitempty"`
	DeliveredComputeMs float64 `json:"delivered_compute_ms,omitempty"`
	LowFrequencyLossMs float64 `json:"low_frequency_loss_ms,omitempty"`
	// MaxFrequencyKHz is the fmax benchmark: the max over the cpu_frequency
	// samples that govern this window (the nearest preceding sample that
	// governs the window head + in-window samples — the same set as the
	// window frequency-residency timeline). Pre-window history samples that
	// govern nothing inside the window never participate. 0 when no
	// governing sample exists.
	MaxFrequencyKHz int `json:"max_frequency_khz,omitempty"`
	// FrequencyKnown is false when the CPU had no frequency sample at all —
	// its running time was weighted 1.0 (无频点数据).
	FrequencyKnown bool `json:"frequency_known"`
}

type BlockedReasonSummary struct {
	Thread ThreadRef `json:"thread"`
	IOWait int       `json:"io_wait,omitempty"`
	Reason string    `json:"reason,omitempty"`
	Count  int       `json:"count,omitempty"`
	Line   int       `json:"line,omitempty"`
	Ts     float64   `json:"ts,omitempty"`
}

type CPUStats struct {
	CPU                int                     `json:"cpu"`
	CoreClass          string                  `json:"core_class,omitempty"`
	BusyMs             float64                 `json:"busy_ms,omitempty"`
	IdleMs             float64                 `json:"idle_ms,omitempty"`
	Frequency          int                     `json:"frequency,omitempty"`
	FrequencyResidency []CPUFrequencyResidency `json:"frequency_residency,omitempty"`
}

type CPUConstraintSummary struct {
	Thread             ThreadRef `json:"thread"`
	Kind               string    `json:"kind,omitempty"`
	Policy             string    `json:"policy,omitempty"`
	CPUSet             string    `json:"cpuset,omitempty"`
	CGroup             string    `json:"cgroup,omitempty"`
	AllowedCPUs        []int     `json:"allowed_cpus,omitempty"`
	AllowedCoreClasses []string  `json:"allowed_core_classes,omitempty"`
	ObservedCPU        int       `json:"observed_cpu,omitempty"`
	ObservedCPUKnown   bool      `json:"-"`
	ObservedCoreClass  string    `json:"observed_core_class,omitempty"`
	MigrationCount     int       `json:"migration_count,omitempty"`
	ConstraintCount    int       `json:"constraint_count,omitempty"`
	RunnableWaitMs     float64   `json:"runnable_wait_ms,omitempty"`
	OtherCPUIdleMs     float64   `json:"other_cpu_idle_ms,omitempty"`
	StartTs            float64   `json:"start_ts,omitempty"`
	EndTs              float64   `json:"end_ts,omitempty"`
	LineStart          int       `json:"line_start,omitempty"`
	LineEnd            int       `json:"line_end,omitempty"`
	Summary            string    `json:"summary,omitempty"`
}

type ThreadCPULoadSummary struct {
	Thread                ThreadRef `json:"thread"`
	RunningMs             float64   `json:"running_ms,omitempty"`
	RunnableWaitMs        float64   `json:"runnable_wait_ms,omitempty"`
	HighPriorityRunningMs float64   `json:"high_priority_running_ms,omitempty"`
	CPU                   int       `json:"cpu"`
	CoreClass             string    `json:"core_class,omitempty"`
	Frequency             int       `json:"frequency,omitempty"`
	Priority              int       `json:"priority,omitempty"`
	PriorityClass         string    `json:"priority_class,omitempty"`
	LineStart             int       `json:"line_start,omitempty"`
	LineEnd               int       `json:"line_end,omitempty"`
	Summary               string    `json:"summary,omitempty"`
}

type ProcessCPULoadSummary struct {
	Process               ThreadRef `json:"process"`
	ThreadCount           int       `json:"thread_count,omitempty"`
	RunningMs             float64   `json:"running_ms,omitempty"`
	RunnableWaitMs        float64   `json:"runnable_wait_ms,omitempty"`
	HighPriorityRunningMs float64   `json:"high_priority_running_ms,omitempty"`
	TopThread             ThreadRef `json:"top_thread,omitempty"`
	TopThreadMs           float64   `json:"top_thread_ms,omitempty"`
	CPUs                  []int     `json:"cpus,omitempty"`
	CoreClasses           []string  `json:"core_classes,omitempty"`
	LineStart             int       `json:"line_start,omitempty"`
	LineEnd               int       `json:"line_end,omitempty"`
	Summary               string    `json:"summary,omitempty"`
}

type RunnableContextSummary struct {
	Thread         ThreadRef `json:"thread"`
	RunnableWaitMs float64   `json:"runnable_wait_ms,omitempty"`
	CPU            int       `json:"cpu"`
	CoreClass      string    `json:"core_class,omitempty"`
	Frequency      int       `json:"frequency,omitempty"`
	Priority       int       `json:"priority,omitempty"`
	PriorityClass  string    `json:"priority_class,omitempty"`
	SameCPUBusyMs  float64   `json:"same_cpu_busy_ms,omitempty"`
	SameCPUIdleMs  float64   `json:"same_cpu_idle_ms,omitempty"`
	OtherCPUIdleMs float64   `json:"other_cpu_idle_ms,omitempty"`
	// HighPriorityRunningMs is the full-window background figure (§7.30.2
	// R5g); the cpu_pressure verdict reads HighPriorityRunningOverlapMs.
	HighPriorityRunningMs float64 `json:"high_priority_running_ms,omitempty"`
	// HighPriorityRunningOverlapMs counts only high-priority running time from
	// other threads that overlapped this thread's runnable wait interval
	// (displacement evidence, §7.30.2 R5g).
	HighPriorityRunningOverlapMs float64 `json:"high_priority_running_overlap_ms,omitempty"`
	// SameCPUTopRunning lists only threads whose running overlapped this
	// thread's runnable wait; DurationMs is the overlapped portion (§7.30.2
	// R5g).
	SameCPUTopRunning    []ThreadDuration       `json:"same_cpu_top_running,omitempty"`
	TopBackgroundThreads []ThreadCPULoadSummary `json:"top_background_threads,omitempty"`
	SameProcessLoad      *ProcessCPULoadSummary `json:"same_process_load,omitempty"`
	TopBackgroundProcess *ProcessCPULoadSummary `json:"top_background_process,omitempty"`
	CPUConstraint        *CPUConstraintSummary  `json:"cpu_constraint,omitempty"`
	Verdict              string                 `json:"verdict,omitempty"`
	Confidence           float64                `json:"confidence,omitempty"`
	LineStart            int                    `json:"line_start,omitempty"`
	LineEnd              int                    `json:"line_end,omitempty"`
	Summary              string                 `json:"summary,omitempty"`
}

type CPUPressureStats struct {
	CPU            int     `json:"cpu"`
	CoreClass      string  `json:"core_class,omitempty"`
	RunnableWaitMs float64 `json:"runnable_wait_ms,omitempty"`
	// RunnableWaitDensity = RunnableWaitMs / window wall-clock ms (CMP-9
	// §7.3): this CPU's average runnable backlog depth — the only
	// cross-window-comparable reading of the cross-thread wait sum. 0 when
	// the query window is unbounded (never an estimate).
	RunnableWaitDensity float64 `json:"runnable_wait_density,omitempty"`
	RunnableEvents      int     `json:"runnable_events,omitempty"`
	RunningMs           float64 `json:"running_ms,omitempty"`
	// HighPriorityRunningMs sums ALL high-priority running time on this CPU in
	// the window with no overlap check against any waiting thread — background
	// pressure only (methodology audit §7.30.2 R5g). Competition/displacement
	// verdicts must use HighPriorityRunningOverlapMs / OverlapCompetitors.
	HighPriorityRunningMs float64 `json:"high_priority_running_ms,omitempty"`
	// HighPriorityRunningOverlapMs sums only the high-priority running time
	// that overlapped some OTHER thread's runnable wait on this CPU — the
	// displacement-evidenced share of HighPriorityRunningMs (§7.30.2 R5g).
	HighPriorityRunningOverlapMs float64 `json:"high_priority_running_overlap_ms,omitempty"`
	// OverlapCompetitors lists threads whose running time overlapped another
	// thread's runnable wait on this CPU (any priority); DurationMs carries
	// the overlapped portion only, not the window running total.
	OverlapCompetitors []ThreadDuration `json:"overlap_competitors,omitempty"`
	TopRunnable        []ThreadDuration `json:"top_runnable,omitempty"`
	TopRunning         []ThreadDuration `json:"top_running,omitempty"`
	// runningSegments / runnableSegments keep the raw per-thread scheduling
	// intervals on this CPU (sorted by start, running segments disjoint) so
	// consumers can compute per-target displacement overlap (§7.30.2 R5g).
	// Unexported: verdict input only, never serialized.
	runningSegments  []pressureSegment
	runnableSegments []pressureSegment
}

type CPUFrequencyResidency struct {
	Frequency  int     `json:"frequency"`
	DurationMs float64 `json:"duration_ms"`
	StartTs    float64 `json:"start_ts,omitempty"`
	EndTs      float64 `json:"end_ts,omitempty"`
	LineStart  int     `json:"line_start,omitempty"`
	LineEnd    int     `json:"line_end,omitempty"`
}

type CoreClassStats struct {
	Class               string  `json:"class,omitempty"`
	CPUs                []int   `json:"cpus,omitempty"`
	BusyMs              float64 `json:"busy_ms,omitempty"`
	IdleMs              float64 `json:"idle_ms,omitempty"`
	RunnableWaitMs      float64 `json:"runnable_wait_ms,omitempty"`
	HighPriorityRunMs   float64 `json:"high_priority_running_ms,omitempty"`
	MaxFrequency        int     `json:"max_frequency,omitempty"`
	TopologySource      string  `json:"topology_source,omitempty"`
	ComputeSupplySignal string  `json:"compute_supply_signal,omitempty"`
}

type ThreadDuration struct {
	Thread     ThreadRef `json:"thread"`
	DurationMs float64   `json:"duration_ms"`
	CPU        int       `json:"cpu"`
	CoreClass  string    `json:"core_class,omitempty"`
	// Frequency is the legacy single cpu_frequency sample at the last judged
	// segment start (context only); weighted judgements use the unexported
	// accumulators below (methodology audit §7.30.2 R5e).
	Frequency     int     `json:"frequency,omitempty"`
	StartTs       float64 `json:"start_ts,omitempty"`
	EndTs         float64 `json:"end_ts,omitempty"`
	LineStart     int     `json:"line_start,omitempty"`
	LineEnd       int     `json:"line_end,omitempty"`
	Priority      int     `json:"priority,omitempty"`
	PriorityClass string  `json:"priority_class,omitempty"`
	// R5e duration-weighted frequency accumulation over the judged segments
	// (§7.30.2). Unexported: in-package verdict input, never serialized.
	// freqWeightKHzMs is Σ(segment_ms × segment weighted kHz); freqKnownMs is
	// Σ segment_ms that had any frequency coverage; freqObservedMaxKHz is the
	// max sample observed inside/nearest the segments; freqInSegmentSamples
	// counts cpu_frequency change points strictly inside the segments.
	freqWeightKHzMs      float64
	freqKnownMs          float64
	freqObservedMaxKHz   int
	freqInSegmentSamples int
}

// weightedFrequencyKHz returns the duration-weighted CPU frequency across the
// accumulated judged segments, zero when no cpu_frequency data covered them
// (§7.30.2 R5e: missing data yields no claim, never a default).
func (td ThreadDuration) weightedFrequencyKHz() int {
	if td.freqKnownMs <= 0 {
		return 0
	}
	return int(math.Round(td.freqWeightKHzMs / td.freqKnownMs))
}

type ThreadStateChurnSummary struct {
	Thread            ThreadRef `json:"thread"`
	DominantState     string    `json:"dominant_state,omitempty"`
	TotalMs           float64   `json:"total_ms,omitempty"`
	DominantImpactMs  float64   `json:"dominant_impact_ms,omitempty"`
	RunningMs         float64   `json:"running_ms,omitempty"`
	RunnableMs        float64   `json:"runnable_ms,omitempty"`
	SleepMs           float64   `json:"sleep_ms,omitempty"`
	DStateMs          float64   `json:"d_state_ms,omitempty"`
	IOWaitMs          float64   `json:"io_wait_ms,omitempty"`
	FragmentCount     int       `json:"fragment_count,omitempty"`
	StateSwitches     int       `json:"state_switches,omitempty"`
	MaxSegmentMs      float64   `json:"max_segment_ms,omitempty"`
	P95SegmentMs      float64   `json:"p95_segment_ms,omitempty"`
	RunnableCPU       int       `json:"runnable_cpu,omitempty"`
	RunnableCoreClass string    `json:"runnable_core_class,omitempty"`
	RunnableCPUKnown  bool      `json:"-"`
	// TopCompetitor is only set when that thread's running time actually
	// overlapped this thread's runnable waits on the same CPU (§7.30.2 R5g);
	// zero-overlap co-residents (serial hand-offs) never qualify.
	TopCompetitor string `json:"top_competitor,omitempty"`
	// TopCompetitorOverlapMs is the running time of TopCompetitor that
	// overlapped this thread's runnable waits — the displacement evidence.
	TopCompetitorOverlapMs float64 `json:"top_competitor_overlap_ms,omitempty"`
	// TopCompetitorRunningMs is the competitor's window running total on that
	// CPU — background context only (§7.30.2 R5g).
	TopCompetitorRunningMs float64 `json:"top_competitor_running_ms,omitempty"`
	NextStep               string  `json:"next_step,omitempty"`
	// NextStepKind is the deterministic typed enumeration behind the English
	// NextStep guidance prose (NextStepKind* constants), so renderers can
	// localize without parsing prose.
	NextStepKind string  `json:"next_step_kind,omitempty"`
	LineStart    int     `json:"line_start,omitempty"`
	LineEnd      int     `json:"line_end,omitempty"`
	Confidence   float64 `json:"confidence,omitempty"`
	Summary      string  `json:"summary,omitempty"`
}

type StateDrilldownStep struct {
	Rank     int       `json:"rank,omitempty"`
	Thread   ThreadRef `json:"thread"`
	State    string    `json:"state,omitempty"`
	ImpactMs float64   `json:"impact_ms,omitempty"`
	TotalMs  float64   `json:"total_ms,omitempty"`
	// RankImpactMs is the ranking-only composite weight for fragmented
	// state-churn rows (dominant impact plus half the remaining churn). It
	// exists so fragmentation can outrank a plain duration of the same size
	// and MUST NOT be read as a physical duration: summed with the sibling
	// state durations it exceeds the window (a real customer report published
	// it as the running time and contradicted the churn totals, methodology
	// audit §7.30 S1). Zero for non-churn sources; ImpactMs stays physical.
	RankImpactMs float64 `json:"rank_impact_ms,omitempty"`
	// WindowProportion is ImpactMs as a fraction (0..1) of the selected
	// window duration — how much of the window this state consumed. Zero
	// when the window duration is unknown. Distinct from the perf-sample
	// Percent field elsewhere in this package.
	WindowProportion float64 `json:"window_proportion,omitempty"`
	// Significant marks whether this state occupied a materially large share
	// of the window (the top state is always significant; lower-ranked states
	// are significant only when their proportion clears the floor). Advisory
	// soft-guidance so the drilldown consumer can prioritize which lower-ranked
	// states are worth root-causing per R3, without dropping any step (R4).
	Significant      bool     `json:"significant,omitempty"`
	Source           string   `json:"source,omitempty"`
	RecommendedViews []string `json:"recommended_views,omitempty"`
	ChainRequired    bool     `json:"chain_required,omitempty"`
	Recursive        bool     `json:"recursive,omitempty"`
	StartTs          float64  `json:"start_ts,omitempty"`
	EndTs            float64  `json:"end_ts,omitempty"`
	LineStart        int      `json:"line_start,omitempty"`
	LineEnd          int      `json:"line_end,omitempty"`
	Summary          string   `json:"summary,omitempty"`
}

type IOLatencySummary struct {
	Dev            string    `json:"dev,omitempty"`
	Op             string    `json:"op,omitempty"`
	Sector         int64     `json:"sector,omitempty"`
	Len            int64     `json:"len,omitempty"`
	IssueThread    ThreadRef `json:"issue_thread,omitempty"`
	CompleteThread ThreadRef `json:"complete_thread,omitempty"`
	IssueTs        float64   `json:"issue_ts,omitempty"`
	CompleteTs     float64   `json:"complete_ts,omitempty"`
	DurationMs     float64   `json:"duration_ms,omitempty"`
	IssueLine      int       `json:"issue_line,omitempty"`
	CompleteLine   int       `json:"complete_line,omitempty"`
}

type FileIOSummary struct {
	Dev             string    `json:"dev,omitempty"`
	Inode           string    `json:"inode,omitempty"`
	ParentInode     string    `json:"parent_inode,omitempty"`
	EntryName       string    `json:"entry_name,omitempty"`
	Operation       string    `json:"operation,omitempty"`
	Thread          ThreadRef `json:"thread,omitempty"`
	Count           int       `json:"count,omitempty"`
	CompletionCount int       `json:"completion_count,omitempty"`
	Bytes           int64     `json:"bytes,omitempty"`
	TotalLatencyMs  float64   `json:"total_latency_ms,omitempty"`
	MaxLatencyMs    float64   `json:"max_latency_ms,omitempty"`
	MinOffset       int64     `json:"min_offset,omitempty"`
	MaxOffset       int64     `json:"max_offset,omitempty"`
	Ret             int64     `json:"ret,omitempty"`
	LineStart       int       `json:"line_start,omitempty"`
	LineEnd         int       `json:"line_end,omitempty"`
	StartTs         float64   `json:"start_ts,omitempty"`
	EndTs           float64   `json:"end_ts,omitempty"`
	Example         string    `json:"example,omitempty"`
	Summary         string    `json:"summary,omitempty"`
}

type PageCacheSummary struct {
	Dev       string    `json:"dev,omitempty"`
	Inode     string    `json:"inode,omitempty"`
	Thread    ThreadRef `json:"thread,omitempty"`
	Adds      int       `json:"adds,omitempty"`
	Deletes   int       `json:"deletes,omitempty"`
	Churn     int       `json:"churn,omitempty"`
	Bytes     int64     `json:"bytes,omitempty"`
	MinOffset int64     `json:"min_offset,omitempty"`
	MaxOffset int64     `json:"max_offset,omitempty"`
	LineStart int       `json:"line_start,omitempty"`
	LineEnd   int       `json:"line_end,omitempty"`
	StartTs   float64   `json:"start_ts,omitempty"`
	EndTs     float64   `json:"end_ts,omitempty"`
	Example   string    `json:"example,omitempty"`
	Summary   string    `json:"summary,omitempty"`
}

type StorageLatencySummary struct {
	Layer              string    `json:"layer,omitempty"`
	Event              string    `json:"event,omitempty"`
	Dev                string    `json:"dev,omitempty"`
	Inode              string    `json:"inode,omitempty"`
	EntryName          string    `json:"entry_name,omitempty"`
	Operation          string    `json:"operation,omitempty"`
	Thread             ThreadRef `json:"thread,omitempty"`
	Count              int       `json:"count,omitempty"`
	PairedCount        int       `json:"paired_count,omitempty"`
	UnpairedStartCount int       `json:"unpaired_start_count,omitempty"`
	UnpairedDoneCount  int       `json:"unpaired_done_count,omitempty"`
	Bytes              int64     `json:"bytes,omitempty"`
	MaxLatencyMs       float64   `json:"max_latency_ms,omitempty"`
	AvgLatencyMs       float64   `json:"avg_latency_ms,omitempty"`
	LineStart          int       `json:"line_start,omitempty"`
	LineEnd            int       `json:"line_end,omitempty"`
	StartTs            float64   `json:"start_ts,omitempty"`
	EndTs              float64   `json:"end_ts,omitempty"`
	Example            string    `json:"example,omitempty"`
	Summary            string    `json:"summary,omitempty"`
}

type IOPressureSummary struct {
	Signal              string  `json:"signal,omitempty"`
	Score               float64 `json:"score,omitempty"`
	BlockMaxLatencyMs   float64 `json:"block_max_latency_ms,omitempty"`
	StorageMaxLatencyMs float64 `json:"storage_max_latency_ms,omitempty"`
	FileIOBytes         int64   `json:"file_io_bytes,omitempty"`
	FileIOEvents        int     `json:"file_io_events,omitempty"`
	PageCacheChurn      int     `json:"page_cache_churn,omitempty"`
	IOWaitBlockedCount  int     `json:"io_wait_blocked_count,omitempty"`
	DStateMs            float64 `json:"d_state_ms,omitempty"`
	TopInode            string  `json:"top_inode,omitempty"`
	TopDev              string  `json:"top_dev,omitempty"`
	TopEntryName        string  `json:"top_entry_name,omitempty"`
	LineStart           int     `json:"line_start,omitempty"`
	LineEnd             int     `json:"line_end,omitempty"`
	Summary             string  `json:"summary,omitempty"`
}

type IOBurstEpisodeSummary struct {
	Thread              ThreadRef  `json:"thread,omitempty"`
	ChainRelevance      string     `json:"chain_relevance,omitempty"`
	DominantSignal      string     `json:"dominant_signal,omitempty"`
	StartTs             float64    `json:"start_ts,omitempty"`
	EndTs               float64    `json:"end_ts,omitempty"`
	DurationMs          float64    `json:"duration_ms,omitempty"`
	DStateMs            float64    `json:"d_state_ms,omitempty"`
	IOWaitMs            float64    `json:"io_wait_ms,omitempty"`
	BlockMaxLatencyMs   float64    `json:"block_max_latency_ms,omitempty"`
	StorageMaxLatencyMs float64    `json:"storage_max_latency_ms,omitempty"`
	FileIOBytes         int64      `json:"file_io_bytes,omitempty"`
	PageCacheChurn      int        `json:"page_cache_churn,omitempty"`
	TopInode            string     `json:"top_inode,omitempty"`
	TopDev              string     `json:"top_dev,omitempty"`
	TopEntryName        string     `json:"top_entry_name,omitempty"`
	NearestChainThread  ThreadRef  `json:"nearest_chain_thread,omitempty"`
	NearestChainWindow  TimeWindow `json:"nearest_chain_window,omitempty"`
	OverlapMs           float64    `json:"overlap_ms,omitempty"`
	LineStart           int        `json:"line_start,omitempty"`
	LineEnd             int        `json:"line_end,omitempty"`
	Confidence          float64    `json:"confidence,omitempty"`
	Summary             string     `json:"summary,omitempty"`
}

type BlockIOByInodeSummary struct {
	Dev                 string    `json:"dev,omitempty"`
	Inode               string    `json:"inode,omitempty"`
	EntryName           string    `json:"entry_name,omitempty"`
	Thread              ThreadRef `json:"thread,omitempty"`
	BlockDev            string    `json:"block_dev,omitempty"`
	Operation           string    `json:"operation,omitempty"`
	FileIOBytes         int64     `json:"file_io_bytes,omitempty"`
	PageCacheChurn      int       `json:"page_cache_churn,omitempty"`
	BlockMaxLatencyMs   float64   `json:"block_max_latency_ms,omitempty"`
	StorageMaxLatencyMs float64   `json:"storage_max_latency_ms,omitempty"`
	NearestBlockThread  ThreadRef `json:"nearest_block_thread,omitempty"`
	NearestBlockTs      float64   `json:"nearest_block_ts,omitempty"`
	StartTs             float64   `json:"start_ts,omitempty"`
	EndTs               float64   `json:"end_ts,omitempty"`
	LineStart           int       `json:"line_start,omitempty"`
	LineEnd             int       `json:"line_end,omitempty"`
	Confidence          float64   `json:"confidence,omitempty"`
	Summary             string    `json:"summary,omitempty"`
}

type InterruptActivity struct {
	Kind            string  `json:"kind,omitempty"`
	CPU             int     `json:"cpu"`
	CoreClass       string  `json:"core_class,omitempty"`
	Vector          int     `json:"vector,omitempty"`
	Name            string  `json:"name,omitempty"`
	Count           int     `json:"count,omitempty"`
	PairedCount     int     `json:"paired_count,omitempty"`
	ActiveMs        float64 `json:"active_ms,omitempty"`
	MaxActiveMs     float64 `json:"max_active_ms,omitempty"`
	WindowOverlapMs float64 `json:"window_overlap_ms,omitempty"`
	TargetMask      string  `json:"target_mask,omitempty"`
	TargetCPUs      []int   `json:"target_cpus,omitempty"`
	LineStart       int     `json:"line_start,omitempty"`
	LineEnd         int     `json:"line_end,omitempty"`
	StartTs         float64 `json:"start_ts,omitempty"`
	EndTs           float64 `json:"end_ts,omitempty"`
	Summary         string  `json:"summary,omitempty"`
}

type SchedStatSummary struct {
	Thread          ThreadRef `json:"thread,omitempty"`
	Kind            string    `json:"kind,omitempty"`
	Count           int       `json:"count,omitempty"`
	TotalDelayMs    float64   `json:"total_delay_ms,omitempty"`
	MaxDelayMs      float64   `json:"max_delay_ms,omitempty"`
	TotalRuntimeMs  float64   `json:"total_runtime_ms,omitempty"`
	MaxRuntimeMs    float64   `json:"max_runtime_ms,omitempty"`
	TotalVRuntimeMs float64   `json:"total_vruntime_ms,omitempty"`
	LineStart       int       `json:"line_start,omitempty"`
	LineEnd         int       `json:"line_end,omitempty"`
	StartTs         float64   `json:"start_ts,omitempty"`
	EndTs           float64   `json:"end_ts,omitempty"`
	Summary         string    `json:"summary,omitempty"`
}

type WorkqueueActivity struct {
	Thread       ThreadRef `json:"thread,omitempty"`
	Work         string    `json:"work,omitempty"`
	Function     string    `json:"function,omitempty"`
	Count        int       `json:"count,omitempty"`
	PairedCount  int       `json:"paired_count,omitempty"`
	DurationMs   float64   `json:"duration_ms,omitempty"`
	MaxLatencyMs float64   `json:"max_latency_ms,omitempty"`
	StartTs      float64   `json:"start_ts,omitempty"`
	EndTs        float64   `json:"end_ts,omitempty"`
	LineStart    int       `json:"line_start,omitempty"`
	LineEnd      int       `json:"line_end,omitempty"`
	Summary      string    `json:"summary,omitempty"`
}

type DMAFenceActivity struct {
	Thread      ThreadRef `json:"thread,omitempty"`
	Driver      string    `json:"driver,omitempty"`
	Timeline    string    `json:"timeline,omitempty"`
	Context     string    `json:"context,omitempty"`
	Seqno       string    `json:"seqno,omitempty"`
	Count       int       `json:"count,omitempty"`
	PairedCount int       `json:"paired_count,omitempty"`
	WaitMs      float64   `json:"wait_ms,omitempty"`
	MaxWaitMs   float64   `json:"max_wait_ms,omitempty"`
	StartTs     float64   `json:"start_ts,omitempty"`
	EndTs       float64   `json:"end_ts,omitempty"`
	LineStart   int       `json:"line_start,omitempty"`
	LineEnd     int       `json:"line_end,omitempty"`
	Summary     string    `json:"summary,omitempty"`
}

type SupplyPressureSummary struct {
	Signal                 string                  `json:"signal,omitempty"`
	CPUPressureMs          float64                 `json:"cpu_pressure_ms,omitempty"`
	RunnableWaitMs         float64                 `json:"runnable_wait_ms,omitempty"`
	HighPriorityRunningMs  float64                 `json:"high_priority_running_ms,omitempty"`
	SchedStatWaitMs        float64                 `json:"sched_stat_wait_ms,omitempty"`
	SchedStatIOWaitMs      float64                 `json:"sched_stat_iowait_ms,omitempty"`
	SchedStatBlockedMs     float64                 `json:"sched_stat_blocked_ms,omitempty"`
	IPIEventCount          int                     `json:"ipi_event_count,omitempty"`
	IPIActiveMs            float64                 `json:"ipi_active_ms,omitempty"`
	LowFrequencyCPUs       []int                   `json:"low_frequency_cpus,omitempty"`
	ClockSetRateCount      int                     `json:"clock_set_rate_count,omitempty"`
	ThermalEventCount      int                     `json:"thermal_event_count,omitempty"`
	DDREventCount          int                     `json:"ddr_event_count,omitempty"`
	L3EventCount           int                     `json:"l3_event_count,omitempty"`
	ThroughputEventCount   int                     `json:"throughput_event_count,omitempty"`
	TopBackgroundThreads   []ThreadCPULoadSummary  `json:"top_background_threads,omitempty"`
	TopBackgroundProcesses []ProcessCPULoadSummary `json:"top_background_processes,omitempty"`
	// WindowMs is the wall-clock length of the selected window backing this
	// aggregate (CMP-9 §7.3). 0 when the query window is unbounded — then no
	// density is published (never an estimate).
	WindowMs float64 `json:"window_ms,omitempty"`
	// PressureDensity = CPUPressureMs / WindowMs — the ONLY
	// cross-window-comparable reading of this cross-thread aggregate
	// (≈ average runnable queue depth, CMP-9 §7.3). 0 when WindowMs is 0.
	PressureDensity float64 `json:"pressure_density,omitempty"`
	LineStart       int     `json:"line_start,omitempty"`
	LineEnd         int     `json:"line_end,omitempty"`
	Summary         string  `json:"summary,omitempty"`
}

type TraceMarkCategory struct {
	Category      string    `json:"category,omitempty"`
	Subcategory   string    `json:"subcategory,omitempty"`
	Count         int       `json:"count,omitempty"`
	TotalMs       float64   `json:"total_ms,omitempty"`
	MaxDurationMs float64   `json:"max_duration_ms,omitempty"`
	TopSpan       string    `json:"top_span,omitempty"`
	TopThread     ThreadRef `json:"top_thread,omitempty"`
	LineStart     int       `json:"line_start,omitempty"`
	LineEnd       int       `json:"line_end,omitempty"`
	Summary       string    `json:"summary,omitempty"`
}

type AsyncFileWorkSummary struct {
	Thread     ThreadRef `json:"thread,omitempty"`
	Name       string    `json:"name,omitempty"`
	Category   string    `json:"category,omitempty"`
	StartTs    float64   `json:"start_ts,omitempty"`
	EndTs      float64   `json:"end_ts,omitempty"`
	DurationMs float64   `json:"duration_ms,omitempty"`
	LineStart  int       `json:"line_start,omitempty"`
	LineEnd    int       `json:"line_end,omitempty"`
	Summary    string    `json:"summary,omitempty"`
}

type CPUFrequencyLimit struct {
	CPU          int     `json:"cpu"`
	MinFrequency int     `json:"min_frequency,omitempty"`
	MaxFrequency int     `json:"max_frequency,omitempty"`
	Count        int     `json:"count,omitempty"`
	Line         int     `json:"line,omitempty"`
	Ts           float64 `json:"ts,omitempty"`
}

type SubsystemEventSummary struct {
	Kind      string    `json:"kind,omitempty"`
	EventType EventType `json:"event_type,omitempty"`
	Count     int       `json:"count,omitempty"`
	Line      int       `json:"line,omitempty"`
	Ts        float64   `json:"ts,omitempty"`
	Example   string    `json:"example,omitempty"`
}

type BinderWaitSummary struct {
	Thread            ThreadRef `json:"thread"`
	Peer              ThreadRef `json:"peer,omitempty"`
	TransactionID     int       `json:"transaction_id,omitempty"`
	Flags             string    `json:"flags,omitempty"`
	Oneway            bool      `json:"oneway"`
	SyncLike          bool      `json:"sync_like"`
	BlockingCandidate bool      `json:"blocking_candidate"`
	SendLine          int       `json:"send_line,omitempty"`
	ReceiveLine       int       `json:"receive_line,omitempty"`
	SleepLine         int       `json:"sleep_line,omitempty"`
	WakeupLine        int       `json:"wakeup_line,omitempty"`
	SendTs            float64   `json:"send_ts,omitempty"`
	SleepStartTs      float64   `json:"sleep_start_ts,omitempty"`
	WakeupTs          float64   `json:"wakeup_ts,omitempty"`
	DurationMs        float64   `json:"duration_ms,omitempty"`
	Confidence        float64   `json:"confidence,omitempty"`
	Summary           string    `json:"summary,omitempty"`
	Caveats           []string  `json:"caveats,omitempty"`
}

type TraceSpanSummary struct {
	Thread        ThreadRef `json:"thread"`
	Kind          string    `json:"kind,omitempty"`
	Name          string    `json:"name,omitempty"`
	Category      string    `json:"category,omitempty"`
	Subcategory   string    `json:"subcategory,omitempty"`
	SemanticClass string    `json:"semantic_class,omitempty"`
	StartTs       float64   `json:"start_ts,omitempty"`
	EndTs         float64   `json:"end_ts,omitempty"`
	DurationMs    float64   `json:"duration_ms,omitempty"`
	StartLine     int       `json:"start_line,omitempty"`
	EndLine       int       `json:"end_line,omitempty"`
}

// TraceCounterDeltaSummary is the C1 (2026-07-03) numeric aggregation of a
// C| counter series within the window: one row per (thread, counter name)
// with first/last/min/max/delta over samples whose values parse as numbers.
// Additive beside TraceCounterSummary (which keeps per-value latest rows) so
// existing surfaces stay byte-identical; ranked by |delta| descending.
type TraceCounterDeltaSummary struct {
	Thread    ThreadRef `json:"thread"`
	Name      string    `json:"name,omitempty"`
	Samples   int       `json:"samples,omitempty"`
	First     float64   `json:"first"`
	Last      float64   `json:"last"`
	Min       float64   `json:"min"`
	Max       float64   `json:"max"`
	Delta     float64   `json:"delta"`
	FirstLine int       `json:"first_line,omitempty"`
	LastLine  int       `json:"last_line,omitempty"`
}

type TraceCounterSummary struct {
	Thread ThreadRef `json:"thread"`
	Name   string    `json:"name,omitempty"`
	Value  string    `json:"value,omitempty"`
	Count  int       `json:"count,omitempty"`
	Line   int       `json:"line,omitempty"`
	Ts     float64   `json:"ts,omitempty"`
}

type IRQBurstSummary struct {
	CPU        int     `json:"cpu"`
	Name       string  `json:"name,omitempty"`
	IRQ        int     `json:"irq,omitempty"`
	Count      int     `json:"count,omitempty"`
	StartTs    float64 `json:"start_ts,omitempty"`
	EndTs      float64 `json:"end_ts,omitempty"`
	DurationMs float64 `json:"duration_ms,omitempty"`
	LineStart  int     `json:"line_start,omitempty"`
	LineEnd    int     `json:"line_end,omitempty"`
}

type MemoryKindSummary struct {
	Kind  string  `json:"kind,omitempty"`
	Count int     `json:"count,omitempty"`
	Line  int     `json:"line,omitempty"`
	Ts    float64 `json:"ts,omitempty"`
}

type RuntimeResourceSummary struct {
	Kind           string    `json:"kind,omitempty"`
	Operation      string    `json:"operation,omitempty"`
	Path           string    `json:"path,omitempty"`
	Thread         ThreadRef `json:"thread,omitempty"`
	Count          int       `json:"count,omitempty"`
	TotalLatencyMs float64   `json:"total_latency_ms,omitempty"`
	MaxLatencyMs   float64   `json:"max_latency_ms,omitempty"`
	Bytes          int64     `json:"bytes,omitempty"`
	Address        string    `json:"address,omitempty"`
	Line           int       `json:"line,omitempty"`
	Ts             float64   `json:"ts,omitempty"`
	Example        string    `json:"example,omitempty"`
	Callstack      string    `json:"callstack,omitempty"`
}

type TracePluginSummary struct {
	Kind      string    `json:"kind,omitempty"`
	Domain    string    `json:"domain,omitempty"`
	EventName string    `json:"event_name,omitempty"`
	Metric    string    `json:"metric,omitempty"`
	Value     string    `json:"value,omitempty"`
	Category  string    `json:"category,omitempty"`
	Thread    ThreadRef `json:"thread,omitempty"`
	Count     int       `json:"count,omitempty"`
	Line      int       `json:"line,omitempty"`
	Ts        float64   `json:"ts,omitempty"`
	Example   string    `json:"example,omitempty"`
}

type ThreadDriftSummary struct {
	PID       int      `json:"pid"`
	Names     []string `json:"names,omitempty"`
	TGIDs     []int    `json:"tgids,omitempty"`
	LineStart int      `json:"line_start,omitempty"`
	LineEnd   int      `json:"line_end,omitempty"`
	Caveat    string   `json:"caveat,omitempty"`
}

type RootCauseRankResult struct {
	Target      ThreadRef           `json:"target,omitempty"`
	Window      TimeWindow          `json:"window"`
	Items       []RootCauseRankItem `json:"items,omitempty"`
	Caveats     []string            `json:"caveats,omitempty"`
	Compactions []ViewCompaction    `json:"compactions,omitempty"`
}

// RootCauseSubjectKindAggregateMetric is the typed SubjectKind for root-cause
// rows whose subject is a window/CPU-scoped aggregate metric (cpu_pressure,
// io_pressure without a representative file-IO thread, cpu_frequency_limit,
// irq_burst, irq_activity, ipi_activity, supply_pressure) rather than a
// resolvable thread. Renderers must not present such rows as an
// "unknown thread": the empty ThreadRef is structural, not a resolution gap.
const RootCauseSubjectKindAggregateMetric = "aggregate_metric"

type RootCauseRankItem struct {
	Rank int    `json:"rank"`
	Tier string `json:"tier,omitempty"`
	Type string `json:"type,omitempty"`
	// SubjectKind is empty when the row's subject is a (possibly unresolved)
	// thread, and RootCauseSubjectKindAggregateMetric when the row is a
	// window/CPU-scoped aggregate metric with no single subject thread.
	// Deterministic typed signal set at construction time.
	SubjectKind        string                     `json:"subject_kind,omitempty"`
	Thread             ThreadRef                  `json:"thread,omitempty"`
	PerfContext        *PerfContext               `json:"perf_context,omitempty"`
	PerfContexts       []RootCausePerfRoleContext `json:"perf_contexts,omitempty"`
	StartTs            float64                    `json:"start_ts,omitempty"`
	EndTs              float64                    `json:"end_ts,omitempty"`
	ActualStartTs      float64                    `json:"actual_start_ts,omitempty"`
	ActualEndTs        float64                    `json:"actual_end_ts,omitempty"`
	DominantState      string                     `json:"dominant_state,omitempty"`
	RunningMs          float64                    `json:"running_ms,omitempty"`
	RunnableMs         float64                    `json:"runnable_ms,omitempty"`
	SleepMs            float64                    `json:"sleep_ms,omitempty"`
	DStateMs           float64                    `json:"d_state_ms,omitempty"`
	IOWaitMs           float64                    `json:"io_wait_ms,omitempty"`
	ImpactMs           float64                    `json:"impact_ms,omitempty"`
	ProjectedImpactMs  float64                    `json:"projected_impact_ms,omitempty"`
	CumulativeImpactMs float64                    `json:"cumulative_impact_ms,omitempty"`
	EffectiveImpactMs  float64                    `json:"effective_impact_ms,omitempty"`
	// GatedRunnableMs / GatedRunningDeficitMs mirror the R5d gated-impact
	// composition for priority_inversion_candidate rows (§7.30.3 D3); zero on
	// every other row type.
	GatedRunnableMs       float64 `json:"gated_runnable_ms,omitempty"`
	GatedRunningDeficitMs float64 `json:"gated_running_deficit_ms,omitempty"`
	// PeriodicSource / DetectedPeriodMs / LatenessMs mirror the VS-1 (§7.8)
	// periodic-signal-source accounting of the backing causal impact/aggregate.
	// On a periodic row EffectiveImpactMs carries the discounted attribution
	// (runnable in full + lateness; in-period sleep excluded) and IS the
	// ranking value even when it is exactly 0 (pure cadence) — the boolean is
	// the precise signal that stops the cumulative fallback from resurrecting
	// the raw sleep. ImpactMs/CumulativeImpactMs stay raw (window projection
	// is lossless).
	PeriodicSource   bool    `json:"periodic_source,omitempty"`
	DetectedPeriodMs float64 `json:"detected_period_ms,omitempty"`
	LatenessMs       float64 `json:"lateness_ms,omitempty"`
	// SupplyFoldDeficitMs / SupplyFoldIdealMs / SupplyFoldBasis mirror the
	// VS-2 (§7.10) supply-fold accounting of the backing causal impact /
	// aggregate (running-dominant on-chain rows only; nil basis = fold not
	// computed). Display/wording inputs only — the rank/score lanes never
	// read them (§7.10 red line: deficit 不参赛).
	SupplyFoldDeficitMs float64                  `json:"supply_fold_deficit_ms,omitempty"`
	SupplyFoldIdealMs   float64                  `json:"supply_fold_ideal_ms,omitempty"`
	SupplyFoldBasis     *SupplyFoldBasis         `json:"supply_fold_basis,omitempty"`
	TargetImpactMs      float64                  `json:"target_impact_ms,omitempty"`
	ActualImpactMs      float64                  `json:"actual_impact_ms,omitempty"`
	ActualTotalMs       float64                  `json:"actual_total_ms,omitempty"`
	Score               float64                  `json:"score,omitempty"`
	Confidence          float64                  `json:"confidence,omitempty"`
	LineStart           int                      `json:"line_start,omitempty"`
	LineEnd             int                      `json:"line_end,omitempty"`
	Source              string                   `json:"source,omitempty"`
	Causality           string                   `json:"causality,omitempty"`
	ChainRelevance      string                   `json:"chain_relevance,omitempty"`
	ChainDepth          int                      `json:"chain_depth,omitempty"`
	OverlapMs           float64                  `json:"overlap_ms,omitempty"`
	EdgeCount           int                      `json:"edge_count,omitempty"`
	NearestChainThread  ThreadRef                `json:"nearest_chain_thread,omitempty"`
	NearestChainWindow  TimeWindow               `json:"nearest_chain_window,omitempty"`
	OccurrenceWindows   []WakeupCausalOccurrence `json:"occurrence_windows,omitempty"`
	SpanName            string                   `json:"span_name,omitempty"`
	SpanKind            string                   `json:"span_kind,omitempty"`
	SpanCategory        string                   `json:"span_category,omitempty"`
	SpanSubcategory     string                   `json:"span_subcategory,omitempty"`
	SemanticClass       string                   `json:"semantic_class,omitempty"`
	Summary             string                   `json:"summary,omitempty"`
}

type RootCausePerfRoleContext struct {
	Role        string       `json:"role,omitempty"`
	Thread      ThreadRef    `json:"thread,omitempty"`
	CPU         int          `json:"cpu"`
	Window      TimeWindow   `json:"window,omitempty"`
	Reason      string       `json:"reason,omitempty"`
	PerfContext *PerfContext `json:"perf_context,omitempty"`
}

type FrameRootCauseBundle struct {
	Target                ThreadRef               `json:"target,omitempty"`
	TargetResolution      *FrameTargetResolution  `json:"target_resolution,omitempty"`
	Window                TimeWindow              `json:"window"`
	WakeupChain           *ChainResult            `json:"wakeup_chain,omitempty"`
	FrameTimeline         *FrameTimelineResult    `json:"frame_timeline,omitempty"`
	RootCauseRank         *RootCauseRankResult    `json:"root_cause_rank,omitempty"`
	CriticalBlocking      *CriticalBlockingResult `json:"critical_blocking_calls,omitempty"`
	PerfSamples           *PerfContext            `json:"perf_samples,omitempty"`
	TargetRunningPerf     *PerfContext            `json:"target_running_perf,omitempty"`
	OnChainPerf           *PerfContext            `json:"on_chain_perf,omitempty"`
	BinderPeerPerf        *PerfContext            `json:"binder_peer_perf,omitempty"`
	SameCPUCompetitorPerf *PerfContext            `json:"same_cpu_competitor_perf,omitempty"`
	IOBurstEpisodes       []IOBurstEpisodeSummary `json:"io_burst_episodes,omitempty"`
	BlockIOByInode        []BlockIOByInodeSummary `json:"block_io_by_inode,omitempty"`
	IRQActivity           []InterruptActivity     `json:"irq_activity,omitempty"`
	SoftIRQActivity       []InterruptActivity     `json:"softirq_activity,omitempty"`
	WorkqueueActivity     []WorkqueueActivity     `json:"workqueue_activity,omitempty"`
	DMAFenceActivity      []DMAFenceActivity      `json:"dma_fence_activity,omitempty"`
	SupplyPressureSummary *SupplyPressureSummary  `json:"supply_pressure_summary,omitempty"`
	TraceMarkCategories   []TraceMarkCategory     `json:"trace_mark_categories,omitempty"`
	AsyncFileWork         []AsyncFileWorkSummary  `json:"async_file_work,omitempty"`
	Caveats               []string                `json:"caveats,omitempty"`

	windowStats *WindowStats `json:"-"`
}

type FrameTargetResolution struct {
	Target        ThreadRef              `json:"target,omitempty"`
	Source        string                 `json:"source,omitempty"`
	Confidence    float64                `json:"confidence,omitempty"`
	Window        TimeWindow             `json:"window,omitempty"`
	WindowSource  string                 `json:"window_source,omitempty"`
	SelectedFrame *FrameTargetCandidate  `json:"selected_frame,omitempty"`
	Candidates    []FrameTargetCandidate `json:"candidates,omitempty"`
	Caveats       []string               `json:"caveats,omitempty"`
}

type FrameTargetCandidate struct {
	Thread    ThreadRef  `json:"thread,omitempty"`
	Role      string     `json:"role,omitempty"`
	Phase     string     `json:"phase,omitempty"`
	Name      string     `json:"name,omitempty"`
	FrameID   string     `json:"frame_id,omitempty"`
	Window    TimeWindow `json:"window,omitempty"`
	StartLine int        `json:"start_line,omitempty"`
	EndLine   int        `json:"end_line,omitempty"`
	Score     float64    `json:"score,omitempty"`
	Reason    string     `json:"reason,omitempty"`
}

type InteractionStatsResult struct {
	Target      ThreadRef            `json:"target,omitempty"`
	Window      TimeWindow           `json:"window"`
	Direction   string               `json:"direction,omitempty"`
	Items       []InteractionSummary `json:"items,omitempty"`
	Caveats     []string             `json:"caveats,omitempty"`
	Compactions []ViewCompaction     `json:"compactions,omitempty"`
}

type InteractionSummary struct {
	Peer              ThreadRef `json:"peer,omitempty"`
	WakeupsToTarget   int       `json:"wakeups_to_target,omitempty"`
	WakeupsFromTarget int       `json:"wakeups_from_target,omitempty"`
	BinderToTarget    int       `json:"binder_to_target,omitempty"`
	BinderFromTarget  int       `json:"binder_from_target,omitempty"`
	TotalInteractions int       `json:"total_interactions,omitempty"`
	FirstTs           float64   `json:"first_ts,omitempty"`
	LastTs            float64   `json:"last_ts,omitempty"`
	FirstLine         int       `json:"first_line,omitempty"`
	LastLine          int       `json:"last_line,omitempty"`
	Summary           string    `json:"summary,omitempty"`
}

type FramePipelineResult struct {
	Window      TimeWindow          `json:"window"`
	Items       []FramePhaseSummary `json:"items,omitempty"`
	Caveats     []string            `json:"caveats,omitempty"`
	Compactions []ViewCompaction    `json:"compactions,omitempty"`
}

type FrameTimelineResult struct {
	Window      TimeWindow          `json:"window"`
	Items       []FrameTimelineItem `json:"items,omitempty"`
	Flows       []FrameFlowEdge     `json:"flows,omitempty"`
	Caveats     []string            `json:"caveats,omitempty"`
	Compactions []ViewCompaction    `json:"compactions,omitempty"`
}

type FrameTimelineItem struct {
	Index      int       `json:"index"`
	Thread     ThreadRef `json:"thread,omitempty"`
	Phase      string    `json:"phase,omitempty"`
	Role       string    `json:"role,omitempty"`
	Name       string    `json:"name,omitempty"`
	FrameID    string    `json:"frame_id,omitempty"`
	StartTs    float64   `json:"start_ts,omitempty"`
	EndTs      float64   `json:"end_ts,omitempty"`
	DurationMs float64   `json:"duration_ms,omitempty"`
	StartLine  int       `json:"start_line,omitempty"`
	EndLine    int       `json:"end_line,omitempty"`
	Summary    string    `json:"summary,omitempty"`
}

type FrameFlowEdge struct {
	FromIndex int       `json:"from_index,omitempty"`
	ToIndex   int       `json:"to_index,omitempty"`
	From      ThreadRef `json:"from,omitempty"`
	To        ThreadRef `json:"to,omitempty"`
	FromPhase string    `json:"from_phase,omitempty"`
	ToPhase   string    `json:"to_phase,omitempty"`
	LatencyMs float64   `json:"latency_ms,omitempty"`
	LineStart int       `json:"line_start,omitempty"`
	LineEnd   int       `json:"line_end,omitempty"`
	Summary   string    `json:"summary,omitempty"`
}

type FramePhaseSummary struct {
	Thread     ThreadRef `json:"thread,omitempty"`
	Phase      string    `json:"phase,omitempty"`
	Name       string    `json:"name,omitempty"`
	StartTs    float64   `json:"start_ts,omitempty"`
	EndTs      float64   `json:"end_ts,omitempty"`
	DurationMs float64   `json:"duration_ms,omitempty"`
	StartLine  int       `json:"start_line,omitempty"`
	EndLine    int       `json:"end_line,omitempty"`
	Summary    string    `json:"summary,omitempty"`
}

type CriticalBlockingResult struct {
	Window      TimeWindow                  `json:"window"`
	Items       []CriticalBlockingCandidate `json:"items,omitempty"`
	Caveats     []string                    `json:"caveats,omitempty"`
	Compactions []ViewCompaction            `json:"compactions,omitempty"`
}

type CriticalBlockingCandidate struct {
	Type   string    `json:"type,omitempty"`
	Thread ThreadRef `json:"thread,omitempty"`
	// Peer is the counterpart thread: the binder receiver / IO completer, or —
	// for lock-contention blocking spans (§7.30.3 D1) — the LOCK OWNER parsed
	// deterministically from the structured contention print payload.
	Peer ThreadRef `json:"peer,omitempty"`
	// BlockingKind is the typed contention semantics parsed from a structured
	// blocking print payload ("monitor_contention" / "lock_contention"); empty
	// for rows whose payload carried no such structured format.
	BlockingKind string `json:"blocking_kind,omitempty"`
	// HolderSite is the lock holder's code location from the payload's
	// "at <sig>(<file:line>)" segment, verbatim.
	HolderSite string `json:"holder_site,omitempty"`
	// Waiters is the payload's "waiters=<n>" count (0 = not reported).
	Waiters            int                   `json:"waiters,omitempty"`
	PeerState          *ThreadStateBreakdown `json:"peer_state,omitempty"`
	Flags              string                `json:"flags,omitempty"`
	Oneway             *bool                 `json:"oneway,omitempty"`
	SyncLike           *bool                 `json:"sync_like,omitempty"`
	BlockingCandidate  *bool                 `json:"blocking_candidate,omitempty"`
	ChainRelevance     string                `json:"chain_relevance,omitempty"`
	OverlapMs          float64               `json:"overlap_ms,omitempty"`
	EdgeCount          int                   `json:"edge_count,omitempty"`
	NearestChainThread ThreadRef             `json:"nearest_chain_thread,omitempty"`
	NearestChainWindow TimeWindow            `json:"nearest_chain_window,omitempty"`
	DurationMs         float64               `json:"duration_ms,omitempty"`
	StartTs            float64               `json:"start_ts,omitempty"`
	EndTs              float64               `json:"end_ts,omitempty"`
	LineStart          int                   `json:"line_start,omitempty"`
	LineEnd            int                   `json:"line_end,omitempty"`
	Confidence         float64               `json:"confidence,omitempty"`
	Summary            string                `json:"summary,omitempty"`
}

type ThreadStateBreakdown struct {
	Thread        ThreadRef  `json:"thread,omitempty"`
	Window        TimeWindow `json:"window,omitempty"`
	DominantState string     `json:"dominant_state,omitempty"`
	TotalMs       float64    `json:"total_ms,omitempty"`
	RunningMs     float64    `json:"running_ms,omitempty"`
	RunnableMs    float64    `json:"runnable_ms,omitempty"`
	SleepMs       float64    `json:"sleep_ms,omitempty"`
	DStateMs      float64    `json:"d_state_ms,omitempty"`
	IOWaitMs      float64    `json:"io_wait_ms,omitempty"`
	FragmentCount int        `json:"fragment_count,omitempty"`
	MaxSegmentMs  float64    `json:"max_segment_ms,omitempty"`
	LineStart     int        `json:"line_start,omitempty"`
	LineEnd       int        `json:"line_end,omitempty"`
	Summary       string     `json:"summary,omitempty"`
}

type RecipeResult struct {
	Name          string   `json:"name,omitempty"`
	IncludedViews []string `json:"included_views,omitempty"`
	Summary       string   `json:"summary,omitempty"`
	Caveats       []string `json:"caveats,omitempty"`
}

type ChainResult struct {
	Target            ThreadRef               `json:"target"`
	Window            TimeWindow              `json:"window"`
	Nodes             []ChainNode             `json:"nodes"`
	Edges             []WakeupEdge            `json:"edges,omitempty"`
	CausalImpacts     []WakeupCausalImpact    `json:"causal_impacts,omitempty"`
	AggregatedImpacts []WakeupCausalAggregate `json:"aggregated_impacts,omitempty"`
	IPCEdges          []IPCEdge               `json:"ipc_edges,omitempty"`
	BinderWaits       []BinderWaitSummary     `json:"binder_waits,omitempty"`
	RootEvidence      []RootEvidence          `json:"root_evidence,omitempty"`
	// ViaThread is the RN-14a (§7.9) via verdict, present only when
	// Query.ViaThread was set: either the via thread is ON a wakeup path to
	// the target (depth + per-hop latency from existing wakeup edges, zero
	// new parsing) or it is NOT connected by any wakeup edge, in which case
	// its influence is scheduling contention (runnable queuing), not a
	// wakeup dependency — the decisive on-chain-root-cause vs
	// scheduling-contention distinction the customer session lacked.
	ViaThread *ChainViaThreadReport `json:"via_thread,omitempty"`
	Caveats   []string              `json:"caveats,omitempty"`
}

// ChainViaThreadReport is the typed RN-14a via verdict for one wakeup chain.
type ChainViaThreadReport struct {
	// Requested is the raw via_thread selector from the tool call.
	Requested string `json:"requested"`
	// Thread is the resolved on-chain thread; zero when OnChain is false.
	Thread  ThreadRef `json:"thread,omitempty"`
	OnChain bool      `json:"on_chain"`
	// Depth is the via thread's minimum chain depth (hops from the target).
	Depth int `json:"depth,omitempty"`
	// Hops walks the wakeup edges from the via thread down to the target,
	// one entry per hop, each with its wakeup latency.
	Hops    []ChainViaHop `json:"hops,omitempty"`
	Summary string        `json:"summary,omitempty"`
}

// ChainViaHop is one waker→wakee hop on the via thread's path to the target.
type ChainViaHop struct {
	Waker      ThreadRef `json:"waker"`
	Wakee      ThreadRef `json:"wakee"`
	LatencyMs  float64   `json:"latency_ms,omitempty"`
	WakeupTs   float64   `json:"wakeup_ts,omitempty"`
	WakeupLine int       `json:"wakeup_line,omitempty"`
}

type IPCGraphResult struct {
	Window       TimeWindow           `json:"window"`
	Edges        []IPCEdge            `json:"edges,omitempty"`
	BinderEvents []BinderEventSummary `json:"binder_events,omitempty"`
	Caveats      []string             `json:"caveats,omitempty"`
	Compactions  []ViewCompaction     `json:"compactions,omitempty"`
}

type IPCEdge struct {
	TransactionID     int       `json:"transaction_id,omitempty"`
	Sender            ThreadRef `json:"sender"`
	Receiver          ThreadRef `json:"receiver,omitempty"`
	DestProc          int       `json:"dest_proc,omitempty"`
	DestThread        int       `json:"dest_thread,omitempty"`
	SendTs            float64   `json:"send_ts,omitempty"`
	ReceiveTs         float64   `json:"receive_ts,omitempty"`
	SendLine          int       `json:"send_line,omitempty"`
	ReceiveLine       int       `json:"receive_line,omitempty"`
	Reply             int       `json:"reply,omitempty"`
	Flags             string    `json:"flags,omitempty"`
	Code              string    `json:"code,omitempty"`
	Oneway            bool      `json:"oneway"`
	SyncLike          bool      `json:"sync_like"`
	BlockingCandidate bool      `json:"blocking_candidate"`
	// Interface is the userspace binder wrapper interface joined from the
	// enclosing `transact[Interface:code]` trace-mark span on the sender
	// thread (C2, 2026-07-03) — a verbatim same-thread span-name join,
	// empty when no wrapper span encloses the send.
	Interface  string   `json:"interface,omitempty"`
	LatencyMs  float64  `json:"latency_ms,omitempty"`
	Confidence float64  `json:"confidence,omitempty"`
	Caveats    []string `json:"caveats,omitempty"`
}

type BinderEventSummary struct {
	Type             EventType `json:"type,omitempty"`
	Thread           ThreadRef `json:"thread,omitempty"`
	TransactionID    int       `json:"transaction_id,omitempty"`
	DebugID          int       `json:"debug_id,omitempty"`
	DataSize         int64     `json:"data_size,omitempty"`
	OffsetsSize      int64     `json:"offsets_size,omitempty"`
	ExtraBuffersSize int64     `json:"extra_buffers_size,omitempty"`
	Tag              string    `json:"tag,omitempty"`
	Ts               float64   `json:"ts,omitempty"`
	Line             int       `json:"line,omitempty"`
	Summary          string    `json:"summary,omitempty"`
}

type ChainNode struct {
	ID           string              `json:"id"`
	Thread       ThreadRef           `json:"thread"`
	Window       TimeWindow          `json:"window"`
	Dominant     ThreadState         `json:"dominant_state"`
	DurationMs   float64             `json:"duration_ms,omitempty"`
	EvidenceLine int                 `json:"evidence_line,omitempty"`
	Impact       *WakeupCausalImpact `json:"impact,omitempty"`
	Summary      string              `json:"summary,omitempty"`
}

type WakeupEdge struct {
	From                       string    `json:"from"`
	To                         string    `json:"to"`
	Waker                      ThreadRef `json:"waker"`
	Wakee                      ThreadRef `json:"wakee"`
	WakeupTs                   float64   `json:"wakeup_ts"`
	WakeupLine                 int       `json:"wakeup_line"`
	LatencyMs                  float64   `json:"latency_ms,omitempty"`
	WakerPriority              int       `json:"waker_priority,omitempty"`
	WakerPriorityClass         string    `json:"waker_priority_class,omitempty"`
	WakeePriority              int       `json:"wakee_priority,omitempty"`
	WakeePriorityClass         string    `json:"wakee_priority_class,omitempty"`
	PriorityRelation           string    `json:"priority_relation,omitempty"`
	PriorityInversionCandidate bool      `json:"priority_inversion_candidate,omitempty"`
	EvidenceLine               int       `json:"evidence_line,omitempty"`
}

type WakeupCausalImpact struct {
	Thread                     ThreadRef  `json:"thread"`
	Window                     TimeWindow `json:"window"`
	ActualWindow               TimeWindow `json:"actual_window,omitempty"`
	ChainDepth                 int        `json:"chain_depth,omitempty"`
	OnChain                    bool       `json:"on_chain,omitempty"`
	DominantState              string     `json:"dominant_state,omitempty"`
	DominantImpactMs           float64    `json:"dominant_impact_ms,omitempty"`
	ProjectedImpactMs          float64    `json:"projected_impact_ms,omitempty"`
	TotalMs                    float64    `json:"total_ms,omitempty"`
	ProjectedTotalMs           float64    `json:"projected_total_ms,omitempty"`
	ActualImpactMs             float64    `json:"actual_impact_ms,omitempty"`
	ActualTotalMs              float64    `json:"actual_total_ms,omitempty"`
	RunningMs                  float64    `json:"running_ms,omitempty"`
	RunnableMs                 float64    `json:"runnable_ms,omitempty"`
	SleepMs                    float64    `json:"sleep_ms,omitempty"`
	DStateMs                   float64    `json:"d_state_ms,omitempty"`
	IOWaitMs                   float64    `json:"io_wait_ms,omitempty"`
	ActualRunningMs            float64    `json:"actual_running_ms,omitempty"`
	ActualRunnableMs           float64    `json:"actual_runnable_ms,omitempty"`
	ActualSleepMs              float64    `json:"actual_sleep_ms,omitempty"`
	ActualDStateMs             float64    `json:"actual_d_state_ms,omitempty"`
	ActualIOWaitMs             float64    `json:"actual_io_wait_ms,omitempty"`
	FragmentCount              int        `json:"fragment_count,omitempty"`
	StateSwitches              int        `json:"state_switches,omitempty"`
	MaxSegmentMs               float64    `json:"max_segment_ms,omitempty"`
	P95SegmentMs               float64    `json:"p95_segment_ms,omitempty"`
	TargetBlockedMs            float64    `json:"target_blocked_ms,omitempty"`
	LineStart                  int        `json:"line_start,omitempty"`
	LineEnd                    int        `json:"line_end,omitempty"`
	Priority                   int        `json:"priority,omitempty"`
	PriorityClass              string     `json:"priority_class,omitempty"`
	TargetPriority             int        `json:"target_priority,omitempty"`
	TargetPriorityClass        string     `json:"target_priority_class,omitempty"`
	PriorityRelation           string     `json:"priority_relation,omitempty"`
	PriorityInversionCandidate bool       `json:"priority_inversion_candidate,omitempty"`
	// PriorityInversionGatedMs is the R5d-gated inversion impact (§7.30.1):
	// only the dependency's RUNNABLE time, plus RUNNING time on a CPU whose
	// frequency is below its downstream chain consumer's CPU frequency at
	// that moment, counts. The dependency's own sleep/D/IO time is its own
	// upstream problem and never inflates the inversion row. This value —
	// not the whole blocked/dominant duration — is what an inversion
	// candidate publishes and ranks with.
	PriorityInversionGatedMs float64 `json:"priority_inversion_gated_ms,omitempty"`
	// GatedRunnableMs / GatedRunningDeficitMs split the gated impact into its
	// two R5d components (§7.30.3 D3): runnable time counted in full, and the
	// capacity-proportional weak-core running deficit. Their sum IS
	// PriorityInversionGatedMs, so renderers can show the composition instead
	// of claiming a single scheduler state for the composite.
	GatedRunnableMs       float64 `json:"gated_runnable_ms,omitempty"`
	GatedRunningDeficitMs float64 `json:"gated_running_deficit_ms,omitempty"`
	// VS-1 (§7.8, customer ruling): periodic-signal-source causal accounting.
	// A periodic waker (e.g. a VSync generator) sleeping between its ticks is
	// normal cadence, not root-cause impact. PeriodicSource is stamped on the
	// sleep-dominant member occurrences of a (waker→target) aggregate whose
	// actual-window start intervals hold the robust cadence (F1/F3/F4,
	// adversarial review 2026-07-04: observation-gap carve + lower-median
	// period, early fires veto, ≥2/3 in-band ratio — see the
	// wakeupPeriodicIntervalTolerance doc) — deterministic interval
	// arithmetic, never a thread-name heuristic. DetectedPeriodMs is that
	// robust period. LatenessMs is THIS occurrence's blocked caliber
	// max(0, TargetBlockedMs − period): how much the target's wait for this
	// signal exceeded one period — independent of whether the selected
	// occurrences are adjacent ticks (intervals are never a lateness source).
	// EffectivePeriodicImpactMs = RunnableMs (counted in full) + LatenessMs,
	// capped at the raw blocking value; in-period sleep never counts. All raw
	// impact/total/actual fields stay untouched (lossless) — only
	// ranking/attribution consume the discount.
	PeriodicSource            bool    `json:"periodic_source,omitempty"`
	DetectedPeriodMs          float64 `json:"detected_period_ms,omitempty"`
	LatenessMs                float64 `json:"lateness_ms,omitempty"`
	EffectivePeriodicImpactMs float64 `json:"effective_periodic_impact_ms,omitempty"`
	// VS-2 (§7.10): supply-fold accounting of an on-chain RUNNING-dominant
	// node (typed gate: OnChain ∧ dominant_state==running ∧ RunningMs>
	// RunnableMs). SupplyFoldIdealMs is the node's running wall clock folded
	// per slice to the big-cluster governed fmax (see supply_fold.go);
	// SupplyFoldDeficitMs = RunningMs − ideal (clamped ≥0) — the running-SLOW
	// share, a LOWER BOUND (frequency ratio only, no microarchitecture
	// claim). SupplyFoldBasis (nil = fold not computed — the typed presence
	// signal) keeps the known/unknown frequency-coverage wall split; unknown
	// slices fold at ratio 1 and never fabricate deficit. Display/wording
	// inputs only: ranking and every raw impact field stay untouched.
	SupplyFoldDeficitMs float64          `json:"supply_fold_deficit_ms,omitempty"`
	SupplyFoldIdealMs   float64          `json:"supply_fold_ideal_ms,omitempty"`
	SupplyFoldBasis     *SupplyFoldBasis `json:"supply_fold_basis,omitempty"`
	Summary             string           `json:"summary,omitempty"`
	NextStep            string           `json:"next_step,omitempty"`
	// NextStepKind is the deterministic typed enumeration behind the English
	// NextStep guidance prose (NextStepKind* constants), so renderers can
	// localize without parsing prose.
	NextStepKind string `json:"next_step_kind,omitempty"`
}

// NextStepKind* enumerate the deterministic kinds behind every system-fixed
// next_step guidance string. The English prose stays the model-facing surface;
// the kind is the render-facing typed signal (e.g. for localization).
const (
	NextStepKindRunnable          = "runnable"
	NextStepKindSSleep            = "s_sleep"
	NextStepKindDSleepIO          = "d_sleep_io"
	NextStepKindRunning           = "running"
	NextStepKindGeneric           = "generic"
	NextStepKindPriorityInversion = "priority_inversion"
)

type WakeupCausalOccurrence struct {
	Window            TimeWindow `json:"window,omitempty"`
	ActualWindow      TimeWindow `json:"actual_window,omitempty"`
	DominantState     string     `json:"dominant_state,omitempty"`
	DominantImpactMs  float64    `json:"dominant_impact_ms,omitempty"`
	ProjectedImpactMs float64    `json:"projected_impact_ms,omitempty"`
	TotalMs           float64    `json:"total_ms,omitempty"`
	ProjectedTotalMs  float64    `json:"projected_total_ms,omitempty"`
	ActualImpactMs    float64    `json:"actual_impact_ms,omitempty"`
	ActualTotalMs     float64    `json:"actual_total_ms,omitempty"`
	TargetBlockedMs   float64    `json:"target_blocked_ms,omitempty"`
	RunningMs         float64    `json:"running_ms,omitempty"`
	RunnableMs        float64    `json:"runnable_ms,omitempty"`
	SleepMs           float64    `json:"sleep_ms,omitempty"`
	DStateMs          float64    `json:"d_state_ms,omitempty"`
	IOWaitMs          float64    `json:"io_wait_ms,omitempty"`
	ActualRunningMs   float64    `json:"actual_running_ms,omitempty"`
	ActualRunnableMs  float64    `json:"actual_runnable_ms,omitempty"`
	ActualSleepMs     float64    `json:"actual_sleep_ms,omitempty"`
	ActualDStateMs    float64    `json:"actual_d_state_ms,omitempty"`
	ActualIOWaitMs    float64    `json:"actual_io_wait_ms,omitempty"`
	FragmentCount     int        `json:"fragment_count,omitempty"`
	StateSwitches     int        `json:"state_switches,omitempty"`
	MaxSegmentMs      float64    `json:"max_segment_ms,omitempty"`
	P95SegmentMs      float64    `json:"p95_segment_ms,omitempty"`
	LineStart         int        `json:"line_start,omitempty"`
	LineEnd           int        `json:"line_end,omitempty"`
	Summary           string     `json:"summary,omitempty"`
}

type WakeupCausalAggregate struct {
	Thread            ThreadRef                `json:"thread"`
	Path              string                   `json:"path,omitempty"`
	ChainDepth        int                      `json:"chain_depth,omitempty"`
	OccurrenceCount   int                      `json:"occurrence_count,omitempty"`
	DominantState     string                   `json:"dominant_state,omitempty"`
	DominantImpactMs  float64                  `json:"dominant_impact_ms,omitempty"`
	ProjectedImpactMs float64                  `json:"projected_impact_ms,omitempty"`
	TotalMs           float64                  `json:"total_ms,omitempty"`
	ProjectedTotalMs  float64                  `json:"projected_total_ms,omitempty"`
	ActualImpactMs    float64                  `json:"actual_impact_ms,omitempty"`
	ActualTotalMs     float64                  `json:"actual_total_ms,omitempty"`
	RunningMs         float64                  `json:"running_ms,omitempty"`
	RunnableMs        float64                  `json:"runnable_ms,omitempty"`
	SleepMs           float64                  `json:"sleep_ms,omitempty"`
	DStateMs          float64                  `json:"d_state_ms,omitempty"`
	IOWaitMs          float64                  `json:"io_wait_ms,omitempty"`
	ActualRunningMs   float64                  `json:"actual_running_ms,omitempty"`
	ActualRunnableMs  float64                  `json:"actual_runnable_ms,omitempty"`
	ActualSleepMs     float64                  `json:"actual_sleep_ms,omitempty"`
	ActualDStateMs    float64                  `json:"actual_d_state_ms,omitempty"`
	ActualIOWaitMs    float64                  `json:"actual_io_wait_ms,omitempty"`
	TargetBlockedMs   float64                  `json:"target_blocked_ms,omitempty"`
	FragmentCount     int                      `json:"fragment_count,omitempty"`
	StateSwitches     int                      `json:"state_switches,omitempty"`
	MaxSegmentMs      float64                  `json:"max_segment_ms,omitempty"`
	FirstTs           float64                  `json:"first_ts,omitempty"`
	LastTs            float64                  `json:"last_ts,omitempty"`
	ActualFirstTs     float64                  `json:"actual_first_ts,omitempty"`
	ActualLastTs      float64                  `json:"actual_last_ts,omitempty"`
	LineStart         int                      `json:"line_start,omitempty"`
	LineEnd           int                      `json:"line_end,omitempty"`
	PriorityRelation  string                   `json:"priority_relation,omitempty"`
	PriorityInversion bool                     `json:"priority_inversion_candidate,omitempty"`
	OccurrenceWindows []WakeupCausalOccurrence `json:"occurrence_windows,omitempty"`
	// VS-1 (§7.8): periodic-signal-source accounting, aggregate face — see the
	// WakeupCausalImpact field docs. LatenessMs here is the SUM of the member
	// occurrences' blocked-caliber lateness amounts, capped at raw blocking −
	// RunnableMs (F1(c): occurrences sharing one branch window must not
	// double-count the same target wait into the Summary);
	// EffectivePeriodicImpactMs = the aggregate RunnableMs (full) + LatenessMs,
	// capped at the raw blocking value. Raw sums above stay untouched.
	PeriodicSource            bool    `json:"periodic_source,omitempty"`
	DetectedPeriodMs          float64 `json:"detected_period_ms,omitempty"`
	LatenessMs                float64 `json:"lateness_ms,omitempty"`
	EffectivePeriodicImpactMs float64 `json:"effective_periodic_impact_ms,omitempty"`
	// VS-2 (§7.10): the SUM of the member occurrences' supply-fold fields
	// (only members whose fold ran contribute — see WakeupCausalImpact). The
	// per-member identity ideal+deficit==RunningMs therefore holds for the
	// folded SUBSET, not necessarily the aggregate RunningMs. nil basis =
	// no member folded.
	SupplyFoldDeficitMs float64          `json:"supply_fold_deficit_ms,omitempty"`
	SupplyFoldIdealMs   float64          `json:"supply_fold_ideal_ms,omitempty"`
	SupplyFoldBasis     *SupplyFoldBasis `json:"supply_fold_basis,omitempty"`
	Summary             string           `json:"summary,omitempty"`
}

type RootEvidence struct {
	Type       string    `json:"type"`
	Thread     ThreadRef `json:"thread"`
	DurationMs float64   `json:"duration_ms,omitempty"`
	LineStart  int       `json:"line_start,omitempty"`
	LineEnd    int       `json:"line_end,omitempty"`
	Summary    string    `json:"summary,omitempty"`
	Confidence float64   `json:"confidence,omitempty"`
}

type EvidenceFact struct {
	Subject    string  `json:"subject"`
	Predicate  string  `json:"predicate,omitempty"`
	Object     string  `json:"object,omitempty"`
	Summary    string  `json:"summary"`
	LineStart  int     `json:"line_start,omitempty"`
	LineEnd    int     `json:"line_end,omitempty"`
	StartTs    float64 `json:"start_ts,omitempty"`
	EndTs      float64 `json:"end_ts,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
}
