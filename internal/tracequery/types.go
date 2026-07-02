package tracequery

import "time"

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
	ParseLinePanics  int
	ClockRegressions int
	// UnparsedLines counts non-empty scanned lines that matched no known
	// trace line format (ParseLine returned no event, without panicking)
	// — typed input for the query layer's coverage caveat, never a hard
	// gate.
	UnparsedLines    int
	TraceFlavor      TraceFlavor
	FlavorConfidence float64
	FlavorSignals    []string
	Caveats          []string
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
	IncludeWindowStats     bool
	Limit                  int
	CoreTopology           string
	TraceFlavor            TraceFlavor
	TraceFlavorHint        TraceFlavor
	TraceFlavorHintSource  string
	TracePlatform          TracePlatform
	TracePlatformHint      TracePlatform
	TracePlatformSource    string
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
	Recipe                      *RecipeResult           `json:"recipe,omitempty"`
	EvidencePack                []EvidenceFact          `json:"evidence_pack,omitempty"`
	Caveats                     []string                `json:"caveats,omitempty"`
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
	StateUnknown  ThreadState = "unknown"
)

type Interval struct {
	Thread           ThreadRef   `json:"thread"`
	State            ThreadState `json:"state"`
	StartTs          float64     `json:"start_ts"`
	EndTs            float64     `json:"end_ts"`
	DurationMs       float64     `json:"duration_ms"`
	ActualStartTs    float64     `json:"actual_start_ts,omitempty"`
	ActualEndTs      float64     `json:"actual_end_ts,omitempty"`
	ActualDurationMs float64     `json:"actual_duration_ms,omitempty"`
	StartLine        int         `json:"start_line,omitempty"`
	EndLine          int         `json:"end_line,omitempty"`
	WakeupLine       int         `json:"wakeup_line,omitempty"`
	PrevStateRaw     string      `json:"prev_state_raw,omitempty"`
	Summary          string      `json:"summary,omitempty"`
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
	Window                TimeWindow                `json:"window"`
	EventCounts           map[EventType]int         `json:"event_counts,omitempty"`
	CPU                   []CPUStats                `json:"cpu,omitempty"`
	CoreTopology          []CoreClassStats          `json:"core_topology,omitempty"`
	TopRunning            []ThreadDuration          `json:"top_running,omitempty"`
	RunnableTop           []ThreadDuration          `json:"runnable_top,omitempty"`
	SleepTop              []ThreadDuration          `json:"sleep_top,omitempty"`
	DStateTop             []ThreadDuration          `json:"d_state_top,omitempty"`
	IOWaitTop             []ThreadDuration          `json:"io_wait_top,omitempty"`
	CPUPressure           []CPUPressureStats        `json:"cpu_pressure,omitempty"`
	CPUConstraints        []CPUConstraintSummary    `json:"cpu_constraints,omitempty"`
	ThreadCPULoad         []ThreadCPULoadSummary    `json:"thread_cpu_load,omitempty"`
	ProcessCPULoad        []ProcessCPULoadSummary   `json:"process_cpu_load,omitempty"`
	RunnableContext       []RunnableContextSummary  `json:"runnable_context,omitempty"`
	IOLatencies           []IOLatencySummary        `json:"io_latencies,omitempty"`
	CPUFrequencyLimits    []CPUFrequencyLimit       `json:"cpu_frequency_limits,omitempty"`
	SubsystemEvents       []SubsystemEventSummary   `json:"subsystem_events,omitempty"`
	BlockIssueCount       int                       `json:"block_issue_count,omitempty"`
	BlockRemapCount       int                       `json:"block_remap_count,omitempty"`
	BlockCompleteCount    int                       `json:"block_complete_count,omitempty"`
	BinderCount           int                       `json:"binder_count,omitempty"`
	BinderReceivedCount   int                       `json:"binder_received_count,omitempty"`
	BinderAuxCount        int                       `json:"binder_aux_count,omitempty"`
	IRQCount              int                       `json:"irq_count,omitempty"`
	SoftIRQCount          int                       `json:"softirq_count,omitempty"`
	MemoryEventCount      int                       `json:"memory_event_count,omitempty"`
	StorageEventCount     int                       `json:"storage_event_count,omitempty"`
	FilesystemEventCount  int                       `json:"filesystem_event_count,omitempty"`
	PowerEventCount       int                       `json:"power_event_count,omitempty"`
	AbilityEventCount     int                       `json:"ability_event_count,omitempty"`
	XPowerEventCount      int                       `json:"xpower_event_count,omitempty"`
	HiSystemEventCount    int                       `json:"hi_sysevent_event_count,omitempty"`
	WorkqueueEventCount   int                       `json:"workqueue_event_count,omitempty"`
	DMAFenceEventCount    int                       `json:"dma_fence_event_count,omitempty"`
	BlockedReasonCount    int                       `json:"blocked_reason_count,omitempty"`
	SchedStatCount        int                       `json:"sched_stat_count,omitempty"`
	IPICount              int                       `json:"ipi_count,omitempty"`
	IOWaitBlockedCount    int                       `json:"io_wait_blocked_count,omitempty"`
	BlockedReasons        []BlockedReasonSummary    `json:"blocked_reasons,omitempty"`
	TraceSpans            []TraceSpanSummary        `json:"trace_spans,omitempty"`
	TraceCounters         []TraceCounterSummary     `json:"trace_counters,omitempty"`
	IRQBursts             []IRQBurstSummary         `json:"irq_bursts,omitempty"`
	MemoryKinds           []MemoryKindSummary       `json:"memory_kinds,omitempty"`
	BIOResources          []RuntimeResourceSummary  `json:"bio_resources,omitempty"`
	FilesystemResources   []RuntimeResourceSummary  `json:"filesystem_resources,omitempty"`
	PageFaultResources    []RuntimeResourceSummary  `json:"page_fault_resources,omitempty"`
	FileIOByInode         []FileIOSummary           `json:"file_io_by_inode,omitempty"`
	PageCacheByInode      []PageCacheSummary        `json:"page_cache_by_inode,omitempty"`
	StorageLatencyByLayer []StorageLatencySummary   `json:"storage_latency_by_layer,omitempty"`
	IOPressureSummary     *IOPressureSummary        `json:"io_pressure_summary,omitempty"`
	IOBurstEpisodes       []IOBurstEpisodeSummary   `json:"io_burst_episodes,omitempty"`
	BlockIOByInode        []BlockIOByInodeSummary   `json:"block_io_by_inode,omitempty"`
	IRQActivity           []InterruptActivity       `json:"irq_activity,omitempty"`
	SoftIRQActivity       []InterruptActivity       `json:"softirq_activity,omitempty"`
	IPIActivity           []InterruptActivity       `json:"ipi_activity,omitempty"`
	WorkqueueActivity     []WorkqueueActivity       `json:"workqueue_activity,omitempty"`
	DMAFenceActivity      []DMAFenceActivity        `json:"dma_fence_activity,omitempty"`
	SchedStatAccounting   []SchedStatSummary        `json:"sched_stat_accounting,omitempty"`
	SupplyPressureSummary *SupplyPressureSummary    `json:"supply_pressure_summary,omitempty"`
	TraceMarkCategories   []TraceMarkCategory       `json:"trace_mark_categories,omitempty"`
	AsyncFileWork         []AsyncFileWorkSummary    `json:"async_file_work,omitempty"`
	AbilityEvents         []TracePluginSummary      `json:"ability_events,omitempty"`
	XPowerEvents          []TracePluginSummary      `json:"xpower_events,omitempty"`
	HiSystemEvents        []TracePluginSummary      `json:"hi_sysevent_events,omitempty"`
	ThreadDrifts          []ThreadDriftSummary      `json:"thread_drifts,omitempty"`
	ComputeSupply         []ComputeSupplySummary    `json:"compute_supply,omitempty"`
	StateChurn            []ThreadStateChurnSummary `json:"state_churn,omitempty"`
	StateDrilldownPlan    []StateDrilldownStep      `json:"state_drilldown_plan,omitempty"`
	PerfSamples           *PerfContext              `json:"perf_samples,omitempty"`
	Caveats               []string                  `json:"caveats,omitempty"`
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
	Target  ThreadRef              `json:"target,omitempty"`
	Window  TimeWindow             `json:"window"`
	Count   int                    `json:"count,omitempty"`
	MeanMs  float64                `json:"mean_ms,omitempty"`
	P50Ms   float64                `json:"p50_ms,omitempty"`
	P95Ms   float64                `json:"p95_ms,omitempty"`
	P99Ms   float64                `json:"p99_ms,omitempty"`
	MaxMs   float64                `json:"max_ms,omitempty"`
	Items   []SchedulerLatencyItem `json:"items,omitempty"`
	Caveats []string               `json:"caveats,omitempty"`
}

type SchedulerLatencyItem struct {
	Thread                ThreadRef        `json:"thread"`
	StartTs               float64          `json:"start_ts,omitempty"`
	EndTs                 float64          `json:"end_ts,omitempty"`
	DurationMs            float64          `json:"duration_ms,omitempty"`
	CPU                   int              `json:"cpu"`
	CoreClass             string           `json:"core_class,omitempty"`
	Frequency             int              `json:"frequency,omitempty"`
	Priority              int              `json:"priority,omitempty"`
	PriorityClass         string           `json:"priority_class,omitempty"`
	StartLine             int              `json:"start_line,omitempty"`
	EndLine               int              `json:"end_line,omitempty"`
	SameCPUBusyMs         float64          `json:"same_cpu_busy_ms,omitempty"`
	SameCPUIdleMs         float64          `json:"same_cpu_idle_ms,omitempty"`
	OtherCPUIdleMs        float64          `json:"other_cpu_idle_ms,omitempty"`
	HighPriorityRunningMs float64          `json:"high_priority_running_ms,omitempty"`
	SameCPUTopRunning     []ThreadDuration `json:"same_cpu_top_running,omitempty"`
	Summary               string           `json:"summary,omitempty"`
}

type ComputeSupplySummary struct {
	Thread                ThreadRef `json:"thread,omitempty"`
	State                 string    `json:"state,omitempty"`
	CPU                   int       `json:"cpu"`
	CoreClass             string    `json:"core_class,omitempty"`
	DurationMs            float64   `json:"duration_ms,omitempty"`
	Frequency             int       `json:"frequency,omitempty"`
	CPUBusyMs             float64   `json:"cpu_busy_ms,omitempty"`
	CPUIdleMs             float64   `json:"cpu_idle_ms,omitempty"`
	RunnableWaitMs        float64   `json:"runnable_wait_ms,omitempty"`
	HighPriorityRunningMs float64   `json:"high_priority_running_ms,omitempty"`
	Verdict               string    `json:"verdict,omitempty"`
	Confidence            float64   `json:"confidence,omitempty"`
	LineStart             int       `json:"line_start,omitempty"`
	LineEnd               int       `json:"line_end,omitempty"`
	Summary               string    `json:"summary,omitempty"`
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
	Thread                ThreadRef              `json:"thread"`
	RunnableWaitMs        float64                `json:"runnable_wait_ms,omitempty"`
	CPU                   int                    `json:"cpu"`
	CoreClass             string                 `json:"core_class,omitempty"`
	Frequency             int                    `json:"frequency,omitempty"`
	Priority              int                    `json:"priority,omitempty"`
	PriorityClass         string                 `json:"priority_class,omitempty"`
	SameCPUBusyMs         float64                `json:"same_cpu_busy_ms,omitempty"`
	SameCPUIdleMs         float64                `json:"same_cpu_idle_ms,omitempty"`
	OtherCPUIdleMs        float64                `json:"other_cpu_idle_ms,omitempty"`
	HighPriorityRunningMs float64                `json:"high_priority_running_ms,omitempty"`
	SameCPUTopRunning     []ThreadDuration       `json:"same_cpu_top_running,omitempty"`
	TopBackgroundThreads  []ThreadCPULoadSummary `json:"top_background_threads,omitempty"`
	SameProcessLoad       *ProcessCPULoadSummary `json:"same_process_load,omitempty"`
	TopBackgroundProcess  *ProcessCPULoadSummary `json:"top_background_process,omitempty"`
	CPUConstraint         *CPUConstraintSummary  `json:"cpu_constraint,omitempty"`
	Verdict               string                 `json:"verdict,omitempty"`
	Confidence            float64                `json:"confidence,omitempty"`
	LineStart             int                    `json:"line_start,omitempty"`
	LineEnd               int                    `json:"line_end,omitempty"`
	Summary               string                 `json:"summary,omitempty"`
}

type CPUPressureStats struct {
	CPU                   int              `json:"cpu"`
	CoreClass             string           `json:"core_class,omitempty"`
	RunnableWaitMs        float64          `json:"runnable_wait_ms,omitempty"`
	RunnableEvents        int              `json:"runnable_events,omitempty"`
	RunningMs             float64          `json:"running_ms,omitempty"`
	HighPriorityRunningMs float64          `json:"high_priority_running_ms,omitempty"`
	TopRunnable           []ThreadDuration `json:"top_runnable,omitempty"`
	TopRunning            []ThreadDuration `json:"top_running,omitempty"`
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
	Thread        ThreadRef `json:"thread"`
	DurationMs    float64   `json:"duration_ms"`
	CPU           int       `json:"cpu"`
	CoreClass     string    `json:"core_class,omitempty"`
	Frequency     int       `json:"frequency,omitempty"`
	StartTs       float64   `json:"start_ts,omitempty"`
	EndTs         float64   `json:"end_ts,omitempty"`
	LineStart     int       `json:"line_start,omitempty"`
	LineEnd       int       `json:"line_end,omitempty"`
	Priority      int       `json:"priority,omitempty"`
	PriorityClass string    `json:"priority_class,omitempty"`
}

type ThreadStateChurnSummary struct {
	Thread                 ThreadRef `json:"thread"`
	DominantState          string    `json:"dominant_state,omitempty"`
	TotalMs                float64   `json:"total_ms,omitempty"`
	DominantImpactMs       float64   `json:"dominant_impact_ms,omitempty"`
	RunningMs              float64   `json:"running_ms,omitempty"`
	RunnableMs             float64   `json:"runnable_ms,omitempty"`
	SleepMs                float64   `json:"sleep_ms,omitempty"`
	DStateMs               float64   `json:"d_state_ms,omitempty"`
	IOWaitMs               float64   `json:"io_wait_ms,omitempty"`
	FragmentCount          int       `json:"fragment_count,omitempty"`
	StateSwitches          int       `json:"state_switches,omitempty"`
	MaxSegmentMs           float64   `json:"max_segment_ms,omitempty"`
	P95SegmentMs           float64   `json:"p95_segment_ms,omitempty"`
	RunnableCPU            int       `json:"runnable_cpu,omitempty"`
	RunnableCoreClass      string    `json:"runnable_core_class,omitempty"`
	RunnableCPUKnown       bool      `json:"-"`
	TopCompetitor          string    `json:"top_competitor,omitempty"`
	TopCompetitorRunningMs float64   `json:"top_competitor_running_ms,omitempty"`
	NextStep               string    `json:"next_step,omitempty"`
	LineStart              int       `json:"line_start,omitempty"`
	LineEnd                int       `json:"line_end,omitempty"`
	Confidence             float64   `json:"confidence,omitempty"`
	Summary                string    `json:"summary,omitempty"`
}

type StateDrilldownStep struct {
	Rank     int       `json:"rank,omitempty"`
	Thread   ThreadRef `json:"thread"`
	State    string    `json:"state,omitempty"`
	ImpactMs float64   `json:"impact_ms,omitempty"`
	TotalMs  float64   `json:"total_ms,omitempty"`
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
	LineStart              int                     `json:"line_start,omitempty"`
	LineEnd                int                     `json:"line_end,omitempty"`
	Summary                string                  `json:"summary,omitempty"`
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
	Target  ThreadRef           `json:"target,omitempty"`
	Window  TimeWindow          `json:"window"`
	Items   []RootCauseRankItem `json:"items,omitempty"`
	Caveats []string            `json:"caveats,omitempty"`
}

type RootCauseRankItem struct {
	Rank               int                        `json:"rank"`
	Tier               string                     `json:"tier,omitempty"`
	Type               string                     `json:"type,omitempty"`
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
	TargetImpactMs     float64                    `json:"target_impact_ms,omitempty"`
	ActualImpactMs     float64                    `json:"actual_impact_ms,omitempty"`
	ActualTotalMs      float64                    `json:"actual_total_ms,omitempty"`
	Score              float64                    `json:"score,omitempty"`
	Confidence         float64                    `json:"confidence,omitempty"`
	LineStart          int                        `json:"line_start,omitempty"`
	LineEnd            int                        `json:"line_end,omitempty"`
	Source             string                     `json:"source,omitempty"`
	Causality          string                     `json:"causality,omitempty"`
	ChainRelevance     string                     `json:"chain_relevance,omitempty"`
	ChainDepth         int                        `json:"chain_depth,omitempty"`
	OverlapMs          float64                    `json:"overlap_ms,omitempty"`
	EdgeCount          int                        `json:"edge_count,omitempty"`
	NearestChainThread ThreadRef                  `json:"nearest_chain_thread,omitempty"`
	NearestChainWindow TimeWindow                 `json:"nearest_chain_window,omitempty"`
	OccurrenceWindows  []WakeupCausalOccurrence   `json:"occurrence_windows,omitempty"`
	SpanName           string                     `json:"span_name,omitempty"`
	SpanKind           string                     `json:"span_kind,omitempty"`
	SpanCategory       string                     `json:"span_category,omitempty"`
	SpanSubcategory    string                     `json:"span_subcategory,omitempty"`
	SemanticClass      string                     `json:"semantic_class,omitempty"`
	Summary            string                     `json:"summary,omitempty"`
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
	Target    ThreadRef            `json:"target,omitempty"`
	Window    TimeWindow           `json:"window"`
	Direction string               `json:"direction,omitempty"`
	Items     []InteractionSummary `json:"items,omitempty"`
	Caveats   []string             `json:"caveats,omitempty"`
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
	Window  TimeWindow          `json:"window"`
	Items   []FramePhaseSummary `json:"items,omitempty"`
	Caveats []string            `json:"caveats,omitempty"`
}

type FrameTimelineResult struct {
	Window  TimeWindow          `json:"window"`
	Items   []FrameTimelineItem `json:"items,omitempty"`
	Flows   []FrameFlowEdge     `json:"flows,omitempty"`
	Caveats []string            `json:"caveats,omitempty"`
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
	Window  TimeWindow                  `json:"window"`
	Items   []CriticalBlockingCandidate `json:"items,omitempty"`
	Caveats []string                    `json:"caveats,omitempty"`
}

type CriticalBlockingCandidate struct {
	Type               string                `json:"type,omitempty"`
	Thread             ThreadRef             `json:"thread,omitempty"`
	Peer               ThreadRef             `json:"peer,omitempty"`
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
	Caveats           []string                `json:"caveats,omitempty"`
}

type IPCGraphResult struct {
	Window       TimeWindow           `json:"window"`
	Edges        []IPCEdge            `json:"edges,omitempty"`
	BinderEvents []BinderEventSummary `json:"binder_events,omitempty"`
	Caveats      []string             `json:"caveats,omitempty"`
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
	LatencyMs         float64   `json:"latency_ms,omitempty"`
	Confidence        float64   `json:"confidence,omitempty"`
	Caveats           []string  `json:"caveats,omitempty"`
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
	Summary                    string     `json:"summary,omitempty"`
	NextStep                   string     `json:"next_step,omitempty"`
}

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
	Summary           string                   `json:"summary,omitempty"`
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
