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

	*ConstraintFields

	State            int    `json:"state,omitempty"`
	Frequency        int    `json:"frequency,omitempty"`
	FrequencyMin     int    `json:"frequency_min,omitempty"`
	FrequencyMax     int    `json:"frequency_max,omitempty"`
	CPUForField      int    `json:"cpu_for_field,omitempty"`
	CPUForFieldValid bool   `json:"cpu_for_field_valid,omitempty"`
	ClockName        string `json:"clock_name,omitempty"`
	Reason           string `json:"reason,omitempty"`
	IOWait           int    `json:"io_wait,omitempty"`
	*SchedStatFields
	SpanAction string `json:"span_action,omitempty"`
	SpanPID    int    `json:"span_pid,omitempty"`
	SpanName   string `json:"span_name,omitempty"`
	SpanValue  string `json:"span_value,omitempty"`

	*BinderFields

	*BlockIOFields

	IRQName string `json:"irq_name,omitempty"`
	IRQID   int    `json:"irq_id,omitempty"`

	IPITargetMask string `json:"ipi_target_mask,omitempty"`
	IPITargetCPUs []int  `json:"ipi_target_cpus,omitempty"`

	MemoryKind    string `json:"memory_kind,omitempty"`
	SubsystemKind string `json:"subsystem_kind,omitempty"`

	*ResourceFields

	*FileFields

	*PluginFields

	*PerfFields

	FieldText string `json:"field_text,omitempty"`
}

// Kind-specific side tables (P4, trace_query_perf_parse_audit_20260703.md).
//
// The flat Event used to carry every per-kind payload inline (140 fields,
// 1736 bytes), so a 250K-event index paid ~434MB of struct bytes alone even
// when the sparse kinds never appeared. Each block below is now an anonymous
// EMBEDDED POINTER group: ParseLine allocates a group if and only if the
// event's kind belongs to that group's family, so the invariant every reader
// relies on is
//
//	group pointer non-nil  ⇔  the event kind populates that family
//	group pointer nil      ≡  every group field at its zero value (old flat
//	                          semantics for foreign kinds)
//
// JSON stays byte-identical to the historical flat struct: anonymous embeds
// promote tagged fields IN PLACE (encoding/json orders promoted fields at the
// embed position), a nil group emits nothing (exactly what all-zero omitempty
// fields emitted), and the tags are copied verbatim. Pinned by
// TestEventJSONSurfaceGolden.
//
// Go field names inside the groups are deliberately DIFFERENT from the
// historical flat names (PerfComm → PerfFields.Comm, Inode → FileFields.Ino,
// …) so that every historical access site failed to compile and was rewritten
// nil-aware. NEVER read a group field through promotion (ev.Symbol): it
// compiles with zero warnings and panics at runtime when the group is nil —
// always take the group pointer and nil-check it
// (pf := ev.PerfFields; if pf != nil { pf.Symbol }). Two adjacent traps:
//
//   - Shadowing: group field names that also exist on the Event core
//     (Comm/PID/CPU, and Event under EventView) ALWAYS resolve to the
//     shallower core field under promotion — ev.Comm is Event.Comm, never
//     PerfFields.Comm — so a promoted read can silently bind the WRONG
//     field instead of panicking.
//   - Ambiguity: names shared by two groups (Dev/Op/Len between
//     BlockIOFields and FileFields, EventName between PerfFields and
//     PluginFields, Kind between ConstraintFields and SchedStatFields) are
//     a compile error only at an actual promoted selection; do not rely on
//     that as protection for the unique names.
//
// This ban is MECHANICALLY enforced, not comment-only:
// TestEventSideTablePromotionBan type-checks tracequery and its downstream
// importers and fails on any field selection whose path runs through an
// embedded pointer group. Groups are immutable after ParseLine returns
// (windowed views and EventView copies share the pointers); never mutate
// them post-parse.

// ConstraintFields is the EventCPUConstraint side table.
type ConstraintFields struct {
	Comm        string `json:"constraint_comm,omitempty"`
	PID         int    `json:"constraint_pid,omitempty"`
	Kind        string `json:"constraint_kind,omitempty"`
	Policy      string `json:"constraint_policy,omitempty"`
	CPU         int    `json:"constraint_cpu,omitempty"`
	CPUValid    bool   `json:"-"`
	OrigCPU     int    `json:"constraint_orig_cpu,omitempty"`
	OrigCPUSet  bool   `json:"-"`
	DestCPU     int    `json:"constraint_dest_cpu,omitempty"`
	DestCPUSet  bool   `json:"-"`
	AllowedText string `json:"allowed_cpus_text,omitempty"`
	Allowed     []int  `json:"allowed_cpus,omitempty"`
	CPUSetName  string `json:"cpuset,omitempty"`
}

// SchedStatFields is the EventSchedStat side table.
type SchedStatFields struct {
	Kind    string `json:"sched_stat_kind,omitempty"`
	Comm    string `json:"sched_stat_comm,omitempty"`
	PID     int    `json:"sched_stat_pid,omitempty"`
	DelayNs int64  `json:"sched_stat_delay_ns,omitempty"`
	RunNs   int64  `json:"sched_stat_runtime_ns,omitempty"`
	VRunNs  int64  `json:"sched_stat_vruntime_ns,omitempty"`
}

// BinderFields is the binder_* event-family side table.
type BinderFields struct {
	TransactionID int    `json:"binder_transaction_id,omitempty"`
	DestProc      int    `json:"binder_dest_proc,omitempty"`
	DestThread    int    `json:"binder_dest_thread,omitempty"`
	Reply         int    `json:"binder_reply,omitempty"`
	Flags         string `json:"binder_flags,omitempty"`
	Code          string `json:"binder_code,omitempty"`
	DebugID       int    `json:"binder_debug_id,omitempty"`
	DataSize      int64  `json:"binder_data_size,omitempty"`
	OffsetsSize   int64  `json:"binder_offsets_size,omitempty"`
	ExtraSize     int64  `json:"binder_extra_size,omitempty"`
	LockTag       string `json:"binder_lock_tag,omitempty"`
}

// BlockIOFields is the block_* event-family side table.
type BlockIOFields struct {
	Dev       string `json:"block_dev,omitempty"`
	Op        string `json:"block_op,omitempty"`
	Sector    int64  `json:"block_sector,omitempty"`
	Len       int64  `json:"block_len,omitempty"`
	Error     string `json:"block_error,omitempty"`
	SrcDev    string `json:"block_src_dev,omitempty"`
	SrcSector int64  `json:"block_src_sector,omitempty"`
}

// ResourceFields is the memory/storage/filesystem resource side table.
type ResourceFields struct {
	Path      string  `json:"resource_path,omitempty"`
	Op        string  `json:"resource_op,omitempty"`
	LatencyMs float64 `json:"resource_latency_ms,omitempty"`
	Bytes     int64   `json:"resource_bytes,omitempty"`
	Address   string  `json:"resource_address,omitempty"`
	Callstack string  `json:"resource_callstack,omitempty"`
}

// FileFields is the memory/storage/filesystem file-IO side table.
type FileFields struct {
	Dev       string `json:"fs_dev,omitempty"`
	Ino       string `json:"inode,omitempty"`
	ParentIno string `json:"parent_inode,omitempty"`
	Entry     string `json:"entry_name,omitempty"`
	Offset    int64  `json:"file_offset,omitempty"`
	Len       int64  `json:"file_len,omitempty"`
	RW        string `json:"file_rw,omitempty"`
	Ret       int64  `json:"file_ret,omitempty"`
	Size      int64  `json:"file_size,omitempty"`
}

// PluginFields is the ability/xpower/hisysevent plugin side table.
type PluginFields struct {
	Domain    string `json:"plugin_domain,omitempty"`
	EventName string `json:"plugin_event_name,omitempty"`
	Metric    string `json:"plugin_metric,omitempty"`
	Value     string `json:"plugin_value,omitempty"`
	Category  string `json:"plugin_category,omitempty"`
}

// PerfFields is the EventPerfSample side table — the single largest block of
// the historical flat Event (32 fields, ~416 bytes) carried by every
// sched_switch of every systrace that had no perf samples at all.
type PerfFields struct {
	PID                 int    `json:"perf_pid,omitempty"`
	TID                 int    `json:"perf_tid,omitempty"`
	Comm                string `json:"perf_comm,omitempty"`
	Period              int64  `json:"perf_period,omitempty"`
	EventName           string `json:"perf_event,omitempty"`
	Symbol              string `json:"perf_symbol,omitempty"`
	DSO                 string `json:"perf_dso,omitempty"`
	IP                  string `json:"perf_ip,omitempty"`
	Addr                string `json:"perf_addr,omitempty"`
	SampleID            string `json:"perf_sample_id,omitempty"`
	StreamID            string `json:"perf_stream_id,omitempty"`
	RawWeight           int64  `json:"perf_raw_weight,omitempty"`
	DataSrc             string `json:"perf_data_src,omitempty"`
	Transaction         string `json:"perf_transaction,omitempty"`
	PhysAddr            string `json:"perf_phys_addr,omitempty"`
	CGroupID            string `json:"perf_cgroup_id,omitempty"`
	DataPageSize        int64  `json:"perf_data_page_size,omitempty"`
	CodePageSize        int64  `json:"perf_code_page_size,omitempty"`
	RawSize             int64  `json:"perf_raw_size,omitempty"`
	BranchCount         int64  `json:"perf_branch_count,omitempty"`
	UserRegsABI         string `json:"perf_user_regs_abi,omitempty"`
	UserRegsCount       int64  `json:"perf_user_regs_count,omitempty"`
	UserStackSize       int64  `json:"perf_user_stack_size,omitempty"`
	AuxSize             int64  `json:"perf_aux_size,omitempty"`
	Callchain           string `json:"perf_callchain,omitempty"`
	Source              string `json:"perf_source,omitempty"`
	SampleKind          string `json:"perf_sample_kind,omitempty"`
	SymbolizationStatus string `json:"perf_symbolization_status,omitempty"`
	Clock               string `json:"perf_clock,omitempty"`
	CPUKnown            *bool  `json:"perf_cpu_known,omitempty"`
	ClockConfidence     string `json:"perf_clock_confidence,omitempty"`
	CallchainStatus     string `json:"perf_callchain_status,omitempty"`
}

// UnparsedLineSample is one retained unparseable-line witness on the Index
// (TDIAG B4): the 1-based line number plus the line text truncated rune-safely
// to indexUnparsedSampleTextBytes bytes.
type UnparsedLineSample struct {
	Line int
	Text string
}

const (
	// IndexUnparsedSampleCap bounds Index.UnparsedSamples (帽 5, §28.12
	// census sample cap — the parse side and the tracediag census face share
	// this one constant).
	IndexUnparsedSampleCap = 5
	// indexUnparsedSampleTextBytes bounds one retained sample's text —
	// deliberately ABOVE the tracediag rendered-token cap (480), so a
	// parse-side-capped sample still overflows the render clamp and carries
	// the render-side 截断 marker: a cut sample can never silently read as a
	// whole line.
	indexUnparsedSampleTextBytes = 512
)

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
	// RetainedSideTableBytes accumulates the struct bytes of the per-kind
	// side-table groups (P4: *PerfFields, *BinderFields, …) hanging off this
	// index's events. eventSizeBytes covers only the shrunk core Event, so
	// cache accounting without this would under-charge side-table-heavy
	// (perf/binder/file-IO) traces. Telemetry + cache cost input only —
	// never a gate signal.
	RetainedSideTableBytes int64
	ClockRegressions       int
	// UnparsedLines counts non-empty scanned lines that matched no known
	// trace line format (ParseLine returned no event, without panicking)
	// — typed input for the query layer's coverage caveat, never a hard
	// gate.
	UnparsedLines int
	// UnparsedSamples retains the FIRST IndexUnparsedSampleCap unparseable
	// line samples (line number + rune-safe byte-capped text) collected at
	// the parse site itself — the TDIAG B4 typed diagnostic face (§28.13,
	// real_trace_campaign_20260705.md, 2026-07-09). Covers both no-format
	// lines and parse-panic lines; windowed builds collect inside their
	// scanned range exactly like full builds (the old census second-read
	// reconstruction had to honestly skip windowed indexes). Hot-path
	// discipline: the recorder is only reached on the unparsed arm and
	// returns immediately once the cap is full — zero allocation on normal
	// lines. Diagnostic display input only, never a gate.
	UnparsedSamples  []UnparsedLineSample
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
	// tidPresenceOnce/tidPresence back the P0-E2a (§10/§11/§12) tid-presence
	// set: the memoized set of every tid that appears in ANY event's
	// PID/PrevPID/NextPID/WakeePID field in THIS trace. It is the PRECISE
	// judgement "this id is RESOLVABLE in this trace" versus "a payload merely
	// PRINTED this id" — a structured contention/binder payload may carry a
	// cross-namespace owner/dest tid that names no thread this trace ever
	// scheduled. Lazily built once by tidPresenceSet(); non-exported and never
	// serialized. Copy-safe like tidTgidVoteOnce (Index is pointer-only).
	//
	// tidScheduled (P0-E2b, E2a correction ①) is the STRICTER subset built in
	// the same pass: tids that appear as sched_switch Prev/Next — i.e. threads
	// with actual context-switch timeline material. The drill verdict's
	// "drilled" claim requires THIS set (a counterpart merely named as a
	// waker/wakee has nothing to drill).
	tidPresenceOnce sync.Once
	tidPresence     map[int]bool
	tidScheduled    map[int]bool
	// nsSpanOnce/nsSpanMaps back the LCK-2 rung-② ns-span derivation (§18.E):
	// the memoized ns→host emission-pair maps (SpanPID → host tgid; (SpanPID,
	// self-reported ns-tid) → host tid) built from every trace_mark row's
	// typed (payload pid ↔ row-header tid/tgid) pair. Structural uniqueness is
	// the hard gate — a second distinct host id marks the entry Ambiguous and
	// rung ② refuses it. Lazily built once by nsSpanDerivedMaps();
	// non-exported, never serialized, copy-safe like tidPresenceOnce.
	nsSpanOnce sync.Once
	nsSpanMaps *nsSpanDerivation
	// freqTimelinesOnce/freqTimelines back the CAP-3 (§29.11, 复核 P3)
	// Index-global per-CPU cpu_frequency sample memo
	// (indexFreqSampleTimelines, cluster_freq_share.go): the window faces'
	// cluster-domain derivation reads the full event stream per resolver
	// construction, and re-scanning ≤250k events on every ComputeWindowStats/
	// BuildSchedulerLatencyStats call is avoidable. READ-ONLY BY CONTRACT:
	// every consumer (chainQueryCache.buildFreqIndex shares this exact map)
	// treats the map and its slices as immutable. Lazily built once;
	// non-exported, never serialized, copy-safe like tidPresenceOnce (Index
	// is pointer-only throughout the package).
	freqTimelinesOnce sync.Once
	freqTimelines     map[int][]freqSample
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
	// CPUFrequencyCensus is the RFC #71 (§8.2 c4) pre-truncation frequency
	// tier ladder for event_search results whose chronological display cap
	// hid matched cpu_frequency rows: distinct kHz tiers + per-tier row
	// counts + cpu set, aggregated in the SAME match pass as the display
	// rows. Additive only — nil whenever the Events face already shows every
	// matched cpu_frequency row.
	CPUFrequencyCensus *CPUFrequencyCensus `json:"cpu_frequency_census,omitempty"`
	EvidencePack       []EvidenceFact      `json:"evidence_pack,omitempty"`
	Caveats            []string            `json:"caveats,omitempty"`
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

// WindowClamped reports whether the query window cut this interval: the
// clamped face (StartTs/EndTs/DurationMs) is narrower than the actual
// scheduler segment (ActualStartTs/ActualEndTs/ActualDurationMs). Intervals
// from builders that never populated the actual fields report false (the
// clampIntervals backfill equalises both ledgers). Single authority for the
// E1-a dual-ledger disclosure (RTC-R1 e1, 2026-07-05): the tracequery Summary
// regeneration and the tool-side timeline row rendering must agree on which
// segments carry actual_duration/actual_window tokens.
func (it Interval) WindowClamped() bool {
	if it.ActualStartTs == 0 && it.ActualEndTs == 0 {
		return false
	}
	return it.ActualStartTs < it.StartTs || it.ActualEndTs > it.EndTs
}

// ActualDurationMsResolved returns the interval's actual (unclamped) duration
// with the two-level fallback: the typed ActualDurationMs when positive, else
// derived from the actual bounds, else the clamped DurationMs. Single
// authority for EVERY face that publishes an actual duration — the tracequery
// Summary regeneration (clampedIntervalSummary), the causal-impact accounting
// (summarizeWakeupCausalImpact) and the tool-side timeline row
// (traceQueryIntervalActualFields). Do not copy the fallback logic and do not
// read ActualDurationMs bare on a display face: a bounds-only interval
// (actual bounds set, ActualDurationMs zero) is WindowClamped, and a bare
// read would publish actual_duration=0.000ms on one face while the Summary
// face publishes the derived value (PTV4 review finding, RTC-R1 2026-07-05).
func (it Interval) ActualDurationMsResolved() float64 {
	if it.ActualDurationMs > 0 {
		return it.ActualDurationMs
	}
	if it.ActualEndTs > it.ActualStartTs {
		return (it.ActualEndTs - it.ActualStartTs) * 1000
	}
	return it.DurationMs
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
	// TopIOInodes is the INODE (§28.6, 2026-07-09) whole-window (dev,inode)
	// IO frequency carrier: folded from the FULL pre-truncation fileIO /
	// pageCache accumulator maps (never from the truncated top-8 slices
	// above), PID/op key dimensions collapsed, ordered Count → Bytes →
	// MaxLatency with TotalGroups truncation disclosure. Latency follows the
	// wall-clock red line: max single event + per-thread within-thread sums,
	// never a cross-thread latency sum. Nil when the window has no IO-family
	// evidence.
	TopIOInodes *TopIOInodeStats `json:"top_io_inodes,omitempty"`
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
	// ProcessDomainCensus is the WSR §8 b3 pid-scoped process-domain rollup
	// lane: the query target's process aggregated from the SAME
	// pre-truncation running buckets CMP-8 consumes (true thread census +
	// cross-CPU merged top threads). Additive only — the global faces above
	// (TopRunning / ThreadCPULoad / ProcessCPULoad) are byte-untouched. Nil
	// when the query names no pid/thread or the process is unresolvable.
	ProcessDomainCensus *ProcessDomainCensus `json:"process_domain_census,omitempty"`
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

// ProcessDomainCensus is the WSR §8 b3 pid-scoped process-domain rollup lane
// (real_trace_campaign_20260705 §8.1): when the query names a pid/thread, the
// process face for that thread's process is aggregated from the SAME
// pre-truncation running buckets CMP-8 occupancy consumes — NOT from the
// display-truncated global thread roster — so the census reports the true
// thread count and the true top threads of the process. The legacy
// process_cpu_load face (a rollup of the surviving global top-N thread
// roster) is untouched: this is an additive lane, nil on target-less queries
// and on targets whose process identity cannot be resolved.
type ProcessDomainCensus struct {
	// Process is the tgid-level process ref the target thread resolved to.
	Process ThreadRef `json:"process"`
	// Target is the query's resolved target thread that anchored this lane
	// (may be the process main thread itself or any member tid).
	Target ThreadRef `json:"target,omitempty"`
	// ThreadCount is the honest census caliber: distinct threads attributed
	// to this process among the threads observed INSIDE the line+time window
	// (same admission predicate as the running side; the catalog supplies
	// attribution only), not the surviving-roster count the legacy
	// process_cpu_load face reports as threads=.
	ThreadCount int `json:"thread_count"`
	// RunningThreadCount is how many of those threads actually accumulated
	// in-window running time (full pre-truncation buckets).
	RunningThreadCount int `json:"running_thread_count,omitempty"`
	// TotalRunningMs is the cross-thread running sum — cpu·ms (CMP-3
	// discipline): cross-thread cpu-time is NOT wall-clock additive and may
	// exceed the wall window on multi-CPU overlap.
	TotalRunningMs float64 `json:"total_running_ms,omitempty"`
	// TopThreads is the per-thread running roster, cross-CPU merged,
	// descending, capped at the shared up-to-8 roster bound. Per-thread
	// values are plain ms: one thread occupies at most one CPU at any
	// instant, so its cross-CPU running segments never overlap in wall time.
	TopThreads []ProcessDomainThread `json:"top_threads,omitempty"`
	// FoldedThreadCount/FoldedRunningMs carry the PTS fold discipline for
	// running threads beyond the roster cap: count + their running sum
	// (cpu·ms — cross-thread), never silent truncation.
	FoldedThreadCount int      `json:"folded_thread_count,omitempty"`
	FoldedRunningMs   float64  `json:"folded_running_ms,omitempty"`
	CPUs              []int    `json:"cpus,omitempty"`
	CoreClasses       []string `json:"core_classes,omitempty"`
	LineStart         int      `json:"line_start,omitempty"`
	LineEnd           int      `json:"line_end,omitempty"`
	Summary           string   `json:"summary,omitempty"`
	Caveats           []string `json:"caveats,omitempty"`
}

// ProcessDomainThread is one census roster row: a single thread's running
// time merged across every CPU it touched in the window (plain ms — a
// thread's own segments are mutually exclusive in wall time).
type ProcessDomainThread struct {
	Thread        ThreadRef `json:"thread"`
	RunningMs     float64   `json:"running_ms"`
	CPUs          []int     `json:"cpus,omitempty"`
	CoreClasses   []string  `json:"core_classes,omitempty"`
	Priority      int       `json:"priority,omitempty"`
	PriorityClass string    `json:"priority_class,omitempty"`
	LineStart     int       `json:"line_start,omitempty"`
	LineEnd       int       `json:"line_end,omitempty"`
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
	// FrequencyClusterDonorCPU (CFR #75 簇共频): non-nil when this CPU had no
	// own cpu_frequency samples and its fmax/weighting reused the named
	// same-cluster sampled core (cluster_freq_share.go single authority).
	// Typed disclosure only; nil means the row rests on the CPU's own
	// samples (or none at all — FrequencyKnown false).
	// FrequencyClusterDonorSource (CFR-2 #80) names the membership source:
	// ClusterFreqSourceExplicit (explicit core_topology) or
	// ClusterFreqSourceDerived (frequency change-point derivation). Set iff
	// the donor field is non-nil.
	FrequencyClusterDonorCPU    *int   `json:"frequency_cluster_donor_cpu,omitempty"`
	FrequencyClusterDonorSource string `json:"frequency_cluster_donor_source,omitempty"`
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

// TopIOInodeThreadLatency is one per-thread latency contributor row of a
// TopIOInodeSummary group. TotalLatencyMs sums file-IO event latencies WITHIN
// this one thread only (same-thread wall clock is additive); the group never
// publishes a cross-thread latency sum (CLAUDE.md red line).
type TopIOInodeThreadLatency struct {
	Thread         ThreadRef `json:"thread,omitempty"`
	TotalLatencyMs float64   `json:"total_latency_ms,omitempty"`
	Count          int       `json:"count,omitempty"`
}

// TopIOInodeSummary is one whole-window (dev,inode) IO frequency group
// (INODE §28.6, 2026-07-09). Unlike the three per-context carriers above
// (FileIOSummary keyed (dev,inode,op,pid), PageCacheSummary keyed
// (dev,inode,pid), BlockIOByInodeSummary built on their truncated outputs),
// this group collapses the PID and op key dimensions so "which inodes see
// the most IO" gets one whole-window answer per inode.
//
// Additivity discipline: every published sum is an event count or byte count
// (additive across threads). Latency is NEVER summed across threads —
// MaxLatencyMs is the single largest member event, and TopThreadLatencies
// carries per-thread within-thread sums only.
type TopIOInodeSummary struct {
	Dev   string `json:"dev,omitempty"`
	Inode string `json:"inode,omitempty"`
	// EntryName is opportunistically backfilled from the first member (event
	// order) carrying a non-empty entry label — a trace file-name label, not
	// an absolute path (same discipline as the accumulator carriers).
	EntryName string `json:"entry_name,omitempty"`
	// Count is the group's frequency caliber and primary sort key: TOTAL
	// in-window IO-family events for this (dev,inode) — file-IO activity +
	// completion events + page-cache add/delete events. Platforms whose FS
	// layer surfaces only mm_filemap rows (Harmony hmfs-adjacent shape) still
	// rank by their real event frequency.
	Count int `json:"count,omitempty"`
	// FileIOCount/CompletionCount/ReadCount/WriteCount decompose the file-IO
	// side. Read/Write cover ONLY activity events whose normalized op is
	// exactly read/read_bio or write/write_bio; ops outside that closed set
	// (direct_io, sync, raw rwbs values, ...) count toward Count/FileIOCount
	// only — the op domain is open-ended and is not guessed into a bucket.
	FileIOCount     int   `json:"file_io_count,omitempty"`
	CompletionCount int   `json:"completion_count,omitempty"`
	ReadCount       int   `json:"read_count,omitempty"`
	WriteCount      int   `json:"write_count,omitempty"`
	Bytes           int64 `json:"bytes,omitempty"`
	// Page-cache side (mm_filemap add/delete churn). PageCache byte fields are
	// deliberately NOT merged into Bytes — different measurement caliber.
	PageCacheAdds    int `json:"page_cache_adds,omitempty"`
	PageCacheDeletes int `json:"page_cache_deletes,omitempty"`
	PageCacheChurn   int `json:"page_cache_churn,omitempty"`
	// MaxLatencyMs is the largest SINGLE member event latency (max over
	// members, never a sum).
	MaxLatencyMs float64 `json:"max_latency_ms,omitempty"`
	// ThreadCount is the number of distinct threads observed touching this
	// (dev,inode); TopThreadLatencies lists the top per-thread latency
	// contributors (within-thread sums, bounded roster).
	ThreadCount        int                       `json:"thread_count,omitempty"`
	TopThreadLatencies []TopIOInodeThreadLatency `json:"top_thread_latencies,omitempty"`
	LineStart          int                       `json:"line_start,omitempty"`
	LineEnd            int                       `json:"line_end,omitempty"`
	StartTs            float64                   `json:"start_ts,omitempty"`
	EndTs              float64                   `json:"end_ts,omitempty"`
	Summary            string                    `json:"summary,omitempty"`
}

// TopIOInodeStats is the WindowStats.TopIOInodes carrier: the ranked group
// rows plus the truncation-honesty disclosure (§28.6 ④ — silent truncation is
// forbidden: TotalGroups always states how many (dev,inode) groups the whole
// window produced, whether or not they all fit in Groups).
type TopIOInodeStats struct {
	Groups []TopIOInodeSummary `json:"groups,omitempty"`
	// TotalGroups counts EVERY identified (dev,inode) group folded from the
	// full pre-truncation accumulator maps, not just the rows kept in Groups.
	TotalGroups int `json:"total_groups,omitempty"`
	// UnidentifiedEvents counts in-window IO-family events that carried no
	// inode identity (inode unknown) — they cannot be enumerated per-inode
	// and folding them into one pseudo-group would fabricate an identity, so
	// they are disclosed as a count instead (they remain visible in the
	// legacy per-context carriers).
	UnidentifiedEvents int `json:"unidentified_events,omitempty"`
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
	Thread ThreadRef `json:"thread"`
	Peer   ThreadRef `json:"peer,omitempty"`
	// PeerSource (P0-E2a, §11 N8): the typed origin of Peer when it is
	// resolved — CounterpartSourceContentionPayload for a receive-row/dest-hint
	// match, CounterpartSourceWakeupEdge when the Android dest_thread=0 form
	// left no receiver and the waiter's direct wakeup edge recovered the peer.
	// Empty when the peer stayed unresolved.
	PeerSource        string   `json:"peer_source,omitempty"`
	TransactionID     int      `json:"transaction_id,omitempty"`
	Flags             string   `json:"flags,omitempty"`
	Oneway            bool     `json:"oneway"`
	SyncLike          bool     `json:"sync_like"`
	BlockingCandidate bool     `json:"blocking_candidate"`
	SendLine          int      `json:"send_line,omitempty"`
	ReceiveLine       int      `json:"receive_line,omitempty"`
	SleepLine         int      `json:"sleep_line,omitempty"`
	WakeupLine        int      `json:"wakeup_line,omitempty"`
	SendTs            float64  `json:"send_ts,omitempty"`
	SleepStartTs      float64  `json:"sleep_start_ts,omitempty"`
	WakeupTs          float64  `json:"wakeup_ts,omitempty"`
	DurationMs        float64  `json:"duration_ms,omitempty"`
	Confidence        float64  `json:"confidence,omitempty"`
	Summary           string   `json:"summary,omitempty"`
	Caveats           []string `json:"caveats,omitempty"`
}

type TraceSpanSummary struct {
	Thread ThreadRef `json:"thread"`
	Kind   string    `json:"kind,omitempty"`
	Name   string    `json:"name,omitempty"`
	// SpanPID is the trace-mark payload pid of the opening B row (`B|{pid}|…`)
	// — the emitter's OWN pid-namespace process id, which for a containerized
	// process differs from the row-header host Thread.TGID (§18.E emission
	// pair). Carried so the LCK-2 rung-② ns-span owner derivation can key the
	// contention span to its container namespace; 0 when the payload carried
	// no pid.
	SpanPID       int     `json:"span_pid,omitempty"`
	Category      string  `json:"category,omitempty"`
	Subcategory   string  `json:"subcategory,omitempty"`
	SemanticClass string  `json:"semantic_class,omitempty"`
	StartTs       float64 `json:"start_ts,omitempty"`
	EndTs         float64 `json:"end_ts,omitempty"`
	DurationMs    float64 `json:"duration_ms,omitempty"`
	// ActualStartTs/ActualEndTs/ActualDurationMs (DCS E4, ledger §23/§23.1 H1,
	// 2026-07-08): the span's FULL B/E extent when the pair straddles the query
	// window boundary — StartTs/EndTs/DurationMs then carry the WINDOW-CLIPPED
	// projection (so every "raw duration" consumer keeps the raw≡in-window
	// invariant), and these three carry the physical extent for cross-window
	// disclosure. All zero when the span lay entirely inside the window:
	// absence is the precise "not clipped" signal, never a guessed window.
	ActualStartTs    float64 `json:"actual_start_ts,omitempty"`
	ActualEndTs      float64 `json:"actual_end_ts,omitempty"`
	ActualDurationMs float64 `json:"actual_duration_ms,omitempty"`
	StartLine        int     `json:"start_line,omitempty"`
	EndLine          int     `json:"end_line,omitempty"`
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

// RootCauseTierDeterministicOptimization (DCS E1, ledger §23/§23.1 user
// ruling 2026-07-07): the independent Tier word for IN-WINDOW ∧ ON-CHAIN
// semantic compile span rows (jit_compile / class_verification /
// shader_compile / runtime_compile / texture_upload — TEX §28.1 fifth class,
// 2026-07-09). Rows wearing it hold rank-board reserved
// seats and participate in ORDERING, but are transparent to the
// primary/secondary/tertiary positional election and NEVER ride the
// co-primary lane — a deterministic optimization point is reported as an
// optimization, not as the root cause. Wire token: it appears verbatim in the
// typed tier note / root_cause_<tier> predicate faces; user-panel prose uses
// the 确定性优化点 display family instead.
const RootCauseTierDeterministicOptimization = "deterministic_optimization"

// RootCauseTierTargetSelfState (SYM, ledger §24.13 裁定一, user ruling
// 2026-07-08, real_trace_campaign_20260705.md): the independent Tier word for
// rank rows whose SUBJECT thread is the analysis target itself AND whose cause
// token belongs to the 等待症状族 (own binder wait / self-held or waited
// blocking_span / own sleep-before-wakeup segments). In a "why is the target
// stuck" question these rows are the SYMPTOM being explained: they keep their
// rank-board ordinal (榜位照发) and every score/weight/sort lane untouched,
// but are TRANSPARENT to the primary/secondary/tertiary positional election
// and never ride the co-primary lane — the deterministic_optimization
// precedent's ladder mechanics applied to the self-symptom lane. Identity is
// the typed tid-first subject==target match (SubjectIsAnalysisTarget) plus
// the typed wait-family token closed set (rootCauseItemIsTargetWaitSymptomType
// — registry wakeup_chain/lock_contention lanes), never a label heuristic:
// the counterpart side of the same contention (subject != target) keeps
// competing unchanged. Wire token: verbatim in the typed tier note / rank text
// face / root_cause_<tier> predicate — the root_cause_primary prefix no longer
// matches, so the projection primary bucket excludes self-symptom rows by
// construction (four witnesses: opendir_78 E1 self-held AssetManager lock
// rank#1 crowned 主根因, huadong_78 binder lead, cmp_78_01 both sides binder
// rank#1→lead).
//
// EVOLUTION RECORD (SYM-2, ledger §24.17, 2026-07-08): scope narrowed from
// every stamped self row to the 等待症状族 only — the 自因可拆解族 (self
// runnable / running / IO / D-state) re-entered the election as decomposable
// self causes (调度压力候选 / 算力供给候选 / IO阻塞候选 / D状态候选) and may
// be crowned lead; the SubjectIsAnalysisTarget stamp itself stays
// full-population (identity fact).
//
// EVOLUTION RECORD (G9 engine renumbering, §27.3/§28.1 user ruling
// 2026-07-09, real_trace_campaign_20260705.md): the §24.13 "榜位照发" clause
// above is superseded — rank ordinals are now assigned ONLY to rows carrying
// a rank-board display identity, so a demoted self-symptom row carries
// Rank=0 (no ordinal, no board seat) instead of pre-consuming an ordinal the
// display never shows (huadong_79/opendir_79 witness: visible boards read
// #6/#7/#12 with #1-#5 pre-consumed by demoted rows). Everything else in
// this comment (election transparency, score/sort lanes untouched, predicate
// prefix exclusion) is unchanged.
const RootCauseTierTargetSelfState = "target_self_state"

// RootCauseTierDataGap (G2 引擎半场, §27.2 audit + §28.1 user ruling
// 2026-07-09, real_trace_campaign_20260705.md): the independent Tier word for
// trace_gap diagnostic rows — a data BLIND SPOT (数据盲区), never a cause. A
// row wearing it neither takes a primary/secondary/tertiary election slot nor
// shifts the slots of the causal rows below it, and it carries NO rank
// ordinal (Rank=0 — the G9 renumbering rule: ordinals only for rows with a
// rank-board display identity; pre-G2 the blind-spot rows occupied board
// seats #6-#12 on the customer face). The observation keeps publishing
// unchanged (the ◇ display arm consumes it in the follow-up display batch);
// identity is the PRECISE mint-time type token "trace_gap" (single mint site:
// the expandChain nil-interesting arm), never a prose heuristic. Wire token:
// verbatim in the typed tier note / root_cause_<tier> predicate — the
// root_cause_primary prefix never matches, so projection primary buckets
// exclude blind-spot rows by construction (the target_self_state precedent).
const RootCauseTierDataGap = "data_gap"

// TraceGapKind* (G2 判据 typed 化, §27.2 + §28.1, 2026-07-09): the PRECISE
// typed criterion split behind a trace_gap mint. The legacy single wording
// "窗内无调度数据" over-claimed: the same (thread, window) could carry a
// depth-0 running rank row (#3, 0.051ms) beside a "no scheduler data" blind
// spot — the window HAD intervals, they just all sat below the MinDurationMs
// floor. Two closed enum forms, decided at the single mint site from the
// thread's own timeline (len(intervals)):
//   - no_sched_data     — the thread timeline holds NO interval at all inside
//                         the aligned window (the only shape the old wording
//                         was true for);
//   - no_eligible_wait  — intervals exist but ALL sit below MinDurationMs
//                         (复核 P3-5 precise fact: mostInterestingInterval's
//                         fallback admits any state at/above the floor, so
//                         nil ⟺ all below it — a running interval at/above
//                         the floor never reaches the mint).
// Published as the trace_gap_kind rich note (display wording is the follow-up
// tool batch; this batch fixes the criterion and the wire identity).
const (
	TraceGapKindNoSchedData    = "no_sched_data"
	TraceGapKindNoEligibleWait = "no_eligible_wait"
)

type RootCauseRankItem struct {
	Rank int    `json:"rank"`
	Tier string `json:"tier,omitempty"`
	// BackgroundRank (DCS E1b/E6, ledger §23.1 rulings ②/③, 2026-07-08): the
	// row's 1-based position on the NON-on-chain composite board (the
	// position COUNTS every published row where rootCauseItemIsOnChain is
	// false — background, adjacent and chainless rows alike, the §23.1 binary
	// lane split). The FIELD is stamped on semantic compile span rows ONLY
	// (复核 F-2): the text and typed-note faces gate to semantic rows, so the
	// JSON payload face gates identically and a semantic-free trace's rank
	// payload stays byte-stable. 0 everywhere else (on-chain rows included).
	// This is the PRECISE typed 榜位 the mention-obligation double gate
	// reads: a non-chain optimization span earns prose mention only at
	// background_rank<=3 (user-adjustable default). Assigned after the final
	// sort/truncation, never an ordering input.
	BackgroundRank int    `json:"background_rank,omitempty"`
	Type           string `json:"type,omitempty"`
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
	// every other row type. GatedCapabilitySource (CAP §26 C3) mirrors the
	// backing impact/aggregate's typed capability caliber for the discounted
	// running component (CoreCapabilitySource* tokens; wording input only).
	GatedRunnableMs       float64 `json:"gated_runnable_ms,omitempty"`
	GatedRunningDeficitMs float64 `json:"gated_running_deficit_ms,omitempty"`
	GatedCapabilitySource string  `json:"gated_capability_source,omitempty"`
	// GatedClusterTopology (CAP-2 §28.4/§28.5): typed cluster-topology source
	// of the discounted running component's capability map
	// (CoreCapabilityTopology* tokens; empty on explicit/legacy — mirror of
	// SupplyFoldBasis.ClusterTopologySource). Wording input only.
	GatedClusterTopology string `json:"gated_cluster_topology,omitempty"`
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
	SupplyFoldDeficitMs float64          `json:"supply_fold_deficit_ms,omitempty"`
	SupplyFoldIdealMs   float64          `json:"supply_fold_ideal_ms,omitempty"`
	SupplyFoldBasis     *SupplyFoldBasis `json:"supply_fold_basis,omitempty"`
	TargetImpactMs      float64          `json:"target_impact_ms,omitempty"`
	ActualImpactMs      float64          `json:"actual_impact_ms,omitempty"`
	ActualTotalMs       float64          `json:"actual_total_ms,omitempty"`
	Score               float64          `json:"score,omitempty"`
	Confidence          float64          `json:"confidence,omitempty"`
	LineStart           int              `json:"line_start,omitempty"`
	LineEnd             int              `json:"line_end,omitempty"`
	Source              string           `json:"source,omitempty"`
	Causality           string           `json:"causality,omitempty"`
	ChainRelevance      string           `json:"chain_relevance,omitempty"`
	ChainDepth          int              `json:"chain_depth,omitempty"`
	// ChainBranch is the owning branch ordinal of the impact/aggregate this
	// rank row was minted from (0 = no single branch identity — window-stats
	// lanes, cross-branch aggregates, legacy rows). Display attach domain only
	// (P0-E CHAIN-PATH, ledger §22.1); no gate and no Score input reads it.
	ChainBranch        int                      `json:"chain_branch,omitempty"`
	OverlapMs          float64                  `json:"overlap_ms,omitempty"`
	EdgeCount          int                      `json:"edge_count,omitempty"`
	NearestChainThread ThreadRef                `json:"nearest_chain_thread,omitempty"`
	NearestChainWindow TimeWindow               `json:"nearest_chain_window,omitempty"`
	OccurrenceWindows  []WakeupCausalOccurrence `json:"occurrence_windows,omitempty"`
	// StatsWindowStartTs/StatsWindowEndTs (§21.1 CWD-2 ②, cmp_01 C7 witness,
	// real_trace_campaign_20260705.md): the typed query-window identity of a
	// window_stats-derived rank row — the window the backing window_stats
	// summary was computed over (stats.Window, i.e. the query's own selected
	// window). Identity carriage ONLY: the rank/score/impact lanes never read
	// these; the tool observation face uses them to emit the row-level
	// selected_window note when the result envelope carries no window of its
	// own, so a window-1 stats row can never silently project into a window-2
	// anchored tree without a window base. Zero when the stats window was
	// unbounded — absence never guesses a window.
	StatsWindowStartTs float64 `json:"stats_window_start_ts,omitempty"`
	StatsWindowEndTs   float64 `json:"stats_window_end_ts,omitempty"`
	// BlockingKind / BlockingPeer / HolderSite (Q4-A 修1, ledger §12.1/§12.3-5):
	// typed lock-contention semantics on a type=blocking_span rank row. The row
	// SUBJECT is the parsed lock HOLDER when the structured payload resolved
	// one (impact stays the MEASURED contention duration); BlockingPeer is then
	// the blocked waiter — i.e. always the contention counterpart of the row
	// subject. When the payload carried no resolvable holder the subject stays
	// the waiter and BlockingPeer stays EMPTY, so "BlockingKind non-empty AND
	// BlockingPeer resolved" is the PRECISE typed admission pair for the
	// direct-on-chain lane (rootCauseItemCanBeDirectOnChain) — an unresolved
	// contention row can never take the head of the rank on the strength of a
	// span-name substring.
	BlockingKind string    `json:"blocking_kind,omitempty"`
	BlockingPeer ThreadRef `json:"blocking_peer,omitempty"`
	HolderSite   string    `json:"holder_site,omitempty"`
	// BlockingFromSite (BLOCKFROM, §27.4 G13 配套, 2026-07-09): the blocked
	// WAITER's own call site from the payload's "blocking from …" tail — rides
	// the rank row verbatim exactly like HolderSite (same mint funnel).
	BlockingFromSite string `json:"blocking_from_site,omitempty"`
	// SubjectIsAnalysisTarget (SYM §24.13 裁定一, 2026-07-08): true when this
	// row's SUBJECT thread is the analysis target the rank was computed FOR —
	// the typed tid-first identity match (sameThreadRef against
	// RootCauseRankResult.Target: PID equality decides whenever both sides
	// carry a tid; the comm arm engages only when a side has none). Minted by
	// stampRootCauseRankAnalysisTargetSubject before tier assignment;
	// assignRootCauseRanksAndTiers reads it for the election-ladder skip arm
	// (Tier=RootCauseTierTargetSelfState). The judgment is SUBJECT identity,
	// never state type: a peer thread's binder_wait/blocking row keeps
	// competing. False when the rank ran without a resolved target (absence
	// never guesses).
	SubjectIsAnalysisTarget bool `json:"subject_is_analysis_target,omitempty"`
	// RunnableBelowRTPreempted (SYM-2 §24.17 R2, 2026-07-08): typed scheduling
	// disclosure on a SELF runnable-family row (subject==target, type ∈
	// runnable_wait / fragmented_runnable_wait / scheduler_latency): the
	// target's own priority class is below RT (Harmony ohos_cfs) while an
	// RT-class competitor's running OVERLAPPED this runnable wait on the same
	// CPU (RunnableContext SameCPUTopRunning displacement evidence — the R5g
	// overlap set, never window-total background load). Display-only wording
	// input (the 行2 「(优先级低于RT)」 tail); rank/score/sort lanes never read
	// it. Only Harmony priority semantics can mint it — Android/generic raw
	// priorities carry no RT class and stamp nothing (absence never guesses).
	RunnableBelowRTPreempted bool `json:"runnable_below_rt_preempted,omitempty"`
	// SubjectIsLockHolder (BLK §15.C, 2026-07-06): on a resolved blocking_span
	// rank row the SUBJECT is the lock HOLDER and BlockingPeer is the blocked
	// WAITER (the reverse of the waiter-subject critical_blocking row for the
	// SAME physical lock). This precise typed flag tells the projection renderer
	// the row is a HOLD, not a wait — "持锁 X ms 阻塞了 <waiter>" instead of the
	// reversed "锁竞争等待(持有者 <waiter>)" — and steers the next-step drilldown
	// to name the HOLDER (the subject), never the waiter. False (payload named
	// no resolvable holder) keeps the waiter-subject shape unchanged.
	SubjectIsLockHolder bool `json:"subject_is_lock_holder,omitempty"`
	// HolderSource / OwnerTidRaw (P0-E2a, §12 Q4-C): the typed origin of the
	// resolved lock HOLDER on a blocking_span rank row —
	// CounterpartSourceContentionPayload (payload tid present in trace,
	// unchanged), CounterpartSourceNsSpanDerivation (LCK-2 rung ②: the phantom
	// container tid was mapped to a host thread via trace_mark emission
	// pairs), or CounterpartSourceWakeupEdge (the holder was recovered from
	// the waiter's direct wakeup edge). OwnerTidRaw carries the phantom
	// payload tid for audit when rung ① failed.
	HolderSource string `json:"holder_source,omitempty"`
	OwnerTidRaw  int    `json:"owner_tid_raw,omitempty"`
	// HolderNsUnification / HolderHostProcess (LCK-2, §18.E/§18.E.1): ported
	// verbatim from the folded blocking candidate — the typed ②×③
	// identity-unification declaration and the process-level ns-span identity
	// display value. See CriticalBlockingCandidate for full semantics.
	HolderNsUnification string `json:"holder_ns_unification,omitempty"`
	HolderHostProcess   string `json:"holder_host_process,omitempty"`
	// HolderHandoff / HolderSelfContradiction (P0-E 锁车道修2, §24.9-C F2):
	// ported verbatim from the folded blocking candidate — the payload
	// hand-off chain witness (holder changed during the wait; the subject is
	// the FINAL holder, never the whole-span holder) and the same-lock
	// self-contradiction demotion witness. See CriticalBlockingCandidate.
	HolderHandoff           []string `json:"holder_handoff,omitempty"`
	HolderSelfContradiction string   `json:"holder_self_contradiction,omitempty"`
	// DrillStatus (RCX① engine side, §12.3 ruling 1): whether this row's
	// contention counterpart/holder was itself examined by a subject==peer
	// observation inside THIS report's observation universe. See the
	// DrillStatus* constants in drill_status.go.
	DrillStatus string `json:"drill_status,omitempty"`
	// InheritedTargetBlockedMs (Q4-B, §12.3 ruling 2): the wakeup-dependency
	// window's target-blocked duration formerly folded into
	// EffectiveImpactMs/TargetImpactMs by the on-chain resource attribution.
	// 承自只作注记,永不作硬排序键 — this field is display/annotation input
	// only; every ranking channel (EffectiveImpactMs, Score, sort keys) stays
	// on the row's own measurement.
	InheritedTargetBlockedMs float64 `json:"inherited_target_blocked_ms,omitempty"`
	// PriorityInversionLockDominated (Q4-D): a resolved monitor/lock
	// contention observation on the target covers this inversion candidate's
	// whole wait interval — the wait is lock-holder dominated and the
	// inversion reading is demoted to an annotation (typed gate: parsed
	// BlockingKind + resolved owner + interval containment; the observation
	// itself is preserved).
	PriorityInversionLockDominated bool   `json:"priority_inversion_lock_dominated,omitempty"`
	SpanName                       string `json:"span_name,omitempty"`
	SpanKind                       string `json:"span_kind,omitempty"`
	SpanCategory                   string `json:"span_category,omitempty"`
	SpanSubcategory                string `json:"span_subcategory,omitempty"`
	SemanticClass                  string `json:"semantic_class,omitempty"`
	// --- RCM family-merge typed carriers (§24.7.1 / §24.10, user rulings
	// 2026-07-08, real_trace_campaign_20260705.md) ---------------------------
	//
	// MemberCount > 1 marks an ENGINE-side same-(thread,type) family merge: the
	// row is ONE ranked contender whose value channels carry the family total
	// (合并量参赛 — split participation weakens ordering, §24.7.1), and the
	// members' real distinguishing keys (inode/dev/op/…) ride MemberRoster so
	// they are never lost (§24.7.1 ①). This typed lane is DELIBERATELY separate
	// from the display-side R2 ×N fold carriers (projection MergedCount /
	// MergedMaxMS): the display lead selector folds MergedCount>1 rows to their
	// member MAX (墙钟跨线程不可加和), while a same-thread family total is
	// legally additive — reusing the Merged* carriers would collapse the family
	// total back to its largest member (§24.12 dimension-A design mandate ①).
	//
	// MemberFoldCaliber is the closed-set typed ruler that produced the merged
	// value (see the RootCauseMemberFoldCaliber* constants); MemberSumMs keeps
	// the lossless raw member Σ ONLY when the published value is below it
	// (union / max fallback disclosure — zero means published == Σ).
	MemberCount       int      `json:"member_count,omitempty"`
	MemberRoster      []string `json:"member_roster,omitempty"`
	MemberMaxMs       float64  `json:"member_max_ms,omitempty"`
	MemberMinMs       float64  `json:"member_min_ms,omitempty"`
	MemberSumMs       float64  `json:"member_sum_ms,omitempty"`
	MemberFoldCaliber string   `json:"member_fold_caliber,omitempty"`
	// MemberKey is the row's OWN typed distinguishing key within its
	// (thread,type) family, minted at construction time from typed source
	// fields (inode=/dev=/op=/work=/… — NEVER re-parsed from Summary prose,
	// §24.9 dim-B F3 red line). The family fold consumes it verbatim as the
	// roster entry identity; cleared on the merged row (the roster carries it).
	MemberKey string `json:"member_key,omitempty"`
	// Inode / Dev are the typed real distinguishing keys of the inode-keyed IO
	// families (block_io_by_inode / file_io_hot_inode / page_cache_churn) —
	// promoted out of the free-text Summary (§24.9 dim-B F3: the key survived
	// ONLY in prose and every display face dropped it). On a merged family row
	// they stay set only when every member agrees; otherwise they clear and the
	// per-member values live in MemberRoster.
	Inode string `json:"inode,omitempty"`
	Dev   string `json:"dev,omitempty"`
	// TraceGapKind (G2 判据 typed 化, §27.2/§28.1, 2026-07-09): on Type ==
	// "trace_gap" rows only — the precise blind-spot criterion form
	// (TraceGapKindNoSchedData / TraceGapKindNoEligibleWait), propagated
	// verbatim from RootEvidence.GapKind. Empty on every other row type.
	TraceGapKind string `json:"trace_gap_kind,omitempty"`
	// --- G1 cross-lane reconciliation, family side (§27.2-G1, user ruling
	// 收口批准 §28.1, 2026-07-09, real_trace_campaign_20260705.md) -----------
	//
	// RankFamilyKey / AbsorbedChainRows are stamped by
	// reconcileCriticalBlockingWithRankFamilies ONLY on a family-merged row
	// (MemberCount ≥ 2) that absorbed at least one same-(thread, type family,
	// query window) critical_blocking row whose interval lies inside the
	// family's member interval union. RankFamilyKey is the canonical
	// reconciliation identity (rankFamilyReconKey — the SAME engine-rendered
	// string the absorbed rows carry in AbsorbedIntoFamily, so the display
	// join is a verbatim string match, never a cross-package label
	// re-derivation). AbsorbedChainRows counts the absorbed observations.
	// Information conservation: the absorbed rows KEEP publishing (观测照发
	// 不删 — evidence index / system supplement / audit tokens stay lossless);
	// only their tree/stanza RENDER seat folds into this row.
	RankFamilyKey     string `json:"rank_family_key,omitempty"`
	AbsorbedChainRows int    `json:"absorbed_chain_rows,omitempty"`
	// familyMemberIntervals is the merged family's member interval inventory
	// (engine-internal, never serialized): mergeSameThreadTypeRankFamily
	// stamps the validated member [start,end] pairs so the G1 reconciliation
	// can test a critical_blocking row's interval against the member UNION —
	// the precise membership signal (§27.2-G1 修向) — instead of the lossy
	// merged-row hull (hull gaps would absorb non-members).
	familyMemberIntervals []foldInterval
	// memberSegmentsProducerDisjoint (ORD, ledger §29.11 补充 cap2 观察①,
	// 2026-07-10; engine-internal, never serialized): the mint site
	// guarantees that this row's underlying scheduler SEGMENTS are pairwise
	// disjoint with every same-(thread,type,source) sibling's segments —
	// computeOffCPUStats keeps at most ONE open segment per PID in a single
	// sequential pass, so the per-CPU bucket rows of one thread partition
	// that thread's own timeline even though their line/ts ENVELOPES
	// interleave. The family-fold caliber ladder reads it as a PRECISE
	// structural disjointness proof (same-thread Σ legal, §24.7.1) where the
	// envelope check alone would honestly degrade to the member MAX. Only the
	// off-CPU top mint sites set it; a merged row carries the AND of its
	// members (idempotent re-fold).
	memberSegmentsProducerDisjoint bool
	Summary                        string `json:"summary,omitempty"`
}

// RootCauseMemberFoldCaliber* — the closed set of typed rulers a same-thread
// family merge may publish its combined value under (RCM §24.7.1/§24.10,
// 2026-07-08). 墙钟红线: same-thread disjoint wall-clock segments sum legally;
// overlapping or unprovable segments must never publish a naive Σ.
const (
	// RootCauseMemberFoldCaliberSumDisjoint — published value == member Σ
	// (same-thread wall clock, legal; opendir_78 E5/E6 witness
	// 1.136+0.462=1.598). Two proof arms admit it (ORD, 2026-07-10):
	//   (a) envelope proof — every member interval is typed and pairwise
	//       disjoint;
	//   (b) producer proof — every member carries
	//       memberSegmentsProducerDisjoint (the off-CPU top mint sites: one
	//       open-segment state machine per PID partitions the thread's own
	//       timeline even though the per-CPU bucket ENVELOPES interleave;
	//       minted only on regression-free indexes, ClockRegressions==0 —
	//       复核 P3-1 ordered-stream premise).
	RootCauseMemberFoldCaliberSumDisjoint = "sum_disjoint"
	// RootCauseMemberFoldCaliberIntervalUnion — member intervals overlap and
	// the interval union is computable: published value == union length
	// (< member Σ, disclosed via MemberSumMs).
	RootCauseMemberFoldCaliberIntervalUnion = "interval_union"
	// RootCauseMemberFoldCaliberMaxOverlapFallback — overlap without a usable
	// union deduction (missing interval identity, or the member value is not an
	// interval length — composite/advisory calibers): published value == member
	// MAX, an honest lower bound (the §21 CWD fallback precedent).
	RootCauseMemberFoldCaliberMaxOverlapFallback = "max_overlap_fallback"
	// RootCauseMemberFoldCaliberCountSum — count-class advisory members
	// (registry Additivity=count): counts add regardless of interval overlap.
	RootCauseMemberFoldCaliberCountSum = "count_sum"
)

// SemanticSpanFamily is the RCM §24.10 semantic-span family carrier: all
// window-clipped spans of ONE (thread, semantic class, chain lane) folded into
// one contender whose participation value is the WINDOW-PROJECTION TOTAL of
// the member segments (interval union; disjoint == Σ; union < Σ discloses via
// SumMs). Built exclusively by FoldSemanticSpanFamilies — the ONE fold point
// both consumers (rank minting and the typed observation channel) read, so the
// two faces can never publish two different family shapes (§24.12 dim-A
// mandate: two consumers, one function). stats.TraceSpans stays untouched —
// the family is a VIEW, never a rewrite of the span inventory.
type SemanticSpanFamily struct {
	Thread        ThreadRef `json:"thread"`
	SemanticClass string    `json:"semantic_class,omitempty"`
	// OnChain is the family's 道别 (chain lane), decided per member by the
	// SAME overlap predicate as the DCS E2 mint-time lane (same-thread chain
	// node / causal-impact window overlap — thread membership alone never
	// flips a lane). Members of one (thread,class) that split lanes form TWO
	// families: on-chain and background never cross-merge (§24.7.1 道别键).
	OnChain       bool    `json:"on_chain,omitempty"`
	ChainDepth    int     `json:"chain_depth,omitempty"`
	DominantState string  `json:"dominant_state,omitempty"`
	TotalMs       float64 `json:"total_ms"`
	// SumMs is the raw member Σ; TotalMs < SumMs means overlapping member
	// segments were deduplicated to the interval union (typed disclosure).
	SumMs   float64 `json:"sum_ms,omitempty"`
	MaxMs   float64 `json:"max_ms,omitempty"`
	MinMs   float64 `json:"min_ms,omitempty"`
	StartTs float64 `json:"start_ts,omitempty"`
	EndTs   float64 `json:"end_ts,omitempty"`
	// ActualTotalMs/ActualStartTs/ActualEndTs mirror the DCS E4 dual-basis
	// discipline: set only when at least one member was window-clipped, they
	// carry the physical member extent (absence = nothing clipped).
	ActualTotalMs float64 `json:"actual_total_ms,omitempty"`
	ActualStartTs float64 `json:"actual_start_ts,omitempty"`
	ActualEndTs   float64 `json:"actual_end_ts,omitempty"`
	StartLine     int     `json:"start_line,omitempty"`
	EndLine       int     `json:"end_line,omitempty"`
	// FoldCaliber ∈ {sum_disjoint, interval_union} (semantic member values ARE
	// window-clipped interval lengths, so the union is always computable).
	FoldCaliber string `json:"fold_caliber,omitempty"`
	// Members are the VERBATIM window-clipped member spans, largest first —
	// the lossless roster (§24.7.1 ①: distinguishing keys — here the span
	// names — must never be dropped). Members[0] is the family representative
	// (span identity faces: name/kind/category).
	Members []TraceSpanSummary `json:"members,omitempty"`
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
	// Skeleton (RCX③, ledger §12.3 ruling 1 item ③): the typed model-facing
	// causal skeleton — target dominant wait → direct explainer → upstream chain
	// head → supply background, each node carrying measured ms + layer +
	// DrillStatus + counterpart source. The ENGINE gives structure; the model
	// writes prose over it (feedback_no_system_backfill). nil when no target/rank
	// evidence exists to skeletonize.
	Skeleton *CausalSkeleton `json:"skeleton,omitempty"`
	Caveats  []string        `json:"caveats,omitempty"`

	windowStats *WindowStats `json:"-"`
}

// CausalSkeletonLayer is the typed layer of a skeleton node (§12.3-1 ②/③): the
// deterministic causal role, NOT a ranking. Renderers show the layer verbatim;
// nodes are ordered target_state → direct_blocker → upstream_chain → adjacent →
// background and NEVER re-sorted by the incommensurable ms values across layers.
type CausalSkeletonLayer string

const (
	// CausalSkeletonLayerTargetState — the target thread's own dominant blocking
	// state (the wait being explained).
	CausalSkeletonLayerTargetState CausalSkeletonLayer = "target_state"
	// CausalSkeletonLayerDirectBlocker — the resolved direct explainer: lock
	// holder / binder peer / blocking-span object owner + holder point.
	CausalSkeletonLayerDirectBlocker CausalSkeletonLayer = "direct_blocker"
	// CausalSkeletonLayerUpstreamChain — the head of the wakeup dependency chain
	// upstream of the target.
	CausalSkeletonLayerUpstreamChain CausalSkeletonLayer = "upstream_chain"
	// CausalSkeletonLayerAdjacent — near-window contention/competition not on the
	// direct chain but overlapping the target window. F6: NO engine producer
	// emits this layer today — buildCausalSkeleton builds exactly
	// target_state / direct_blocker / upstream_chain / background. The constant
	// is reserved for the P0-A projection/answer-face batch (which owns adjacent
	// tiering); removing it would force P0-A to re-adjudicate the layer enum.
	CausalSkeletonLayerAdjacent CausalSkeletonLayer = "adjacent"
	// CausalSkeletonLayerBackground — window/CPU-scoped supply background
	// (cpu_pressure / low-frequency / idle mismatch), never a per-thread cause.
	CausalSkeletonLayerBackground CausalSkeletonLayer = "background"
)

// CausalSkeletonNode is one typed node of the model-facing skeleton. Every field
// is a precise engine-computed signal; the node carries NO prose verdict — the
// model configures the narrative. MeasuredMs is the node's own measured wall
// clock (per-layer, never summed across layers — 墙钟不可加和).
type CausalSkeletonNode struct {
	Layer      CausalSkeletonLayer `json:"layer"`
	Thread     ThreadRef           `json:"thread,omitempty"`
	State      string              `json:"state,omitempty"`
	MeasuredMs float64             `json:"measured_ms,omitempty"`
	// HolderSite is the direct blocker's code location (lock holder point) when
	// the payload carried one; empty otherwise.
	HolderSite string `json:"holder_site,omitempty"`
	// BlockingFromSite is the waiting side's own blocking call site (the
	// payload's "blocking from …" tail) when carried; empty otherwise
	// (BLOCKFROM §27.4 G13, same shape as HolderSite).
	BlockingFromSite string `json:"blocking_from_site,omitempty"`
	// DrillStatus / CounterpartSource are the P0-E1/P0-E2a typed verdicts for a
	// direct_blocker node: whether the named counterpart was itself examined, and
	// whether it came from the payload or a wakeup-edge inference. Empty on
	// layers with no counterpart (target_state / background).
	DrillStatus       string `json:"drill_status,omitempty"`
	CounterpartSource string `json:"counterpart_source,omitempty"`
	Note              string `json:"note,omitempty"`
}

// CausalSkeleton is the RCX③ typed skeleton (§12.3-1 ③): an ordered,
// layer-tagged spine the model narrates. It is a STRUCTURE, not prose — the
// engine never writes the verdict, only the deterministic causal spine.
type CausalSkeleton struct {
	Target ThreadRef            `json:"target,omitempty"`
	Window TimeWindow           `json:"window,omitempty"`
	Nodes  []CausalSkeletonNode `json:"nodes,omitempty"`
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
	// BlockingFromSite (BLOCKFROM, §27.4 G13 配套, 2026-07-09): the WAITER's own
	// blocking call site from the payload's "blocking from <sig>(<file:line>)"
	// tail, verbatim — the typed 等待点 counterpart of HolderSite (持有点). Empty
	// when the payload carried no such segment; display rendering ("等待点: …")
	// is the DISP-2 batch's half.
	BlockingFromSite string `json:"blocking_from_site,omitempty"`
	// HolderSource / PeerSource (P0-E2a, §10 A2 / §11 N8 / §12 Q4-C): the typed
	// origin of the resolved Peer counterpart — CounterpartSourceContentionPayload
	// when the payload tid is present in this trace (unchanged path), or
	// CounterpartSourceWakeupEdge when the payload tid named no in-trace thread
	// and the direct 1-hop wakeup edge recovered the peer. Empty on rows whose
	// peer was never resolved by either route. HolderSource is stamped on
	// lock-contention (blocking_span) rows; PeerSource on binder_wait rows.
	HolderSource string `json:"holder_source,omitempty"`
	PeerSource   string `json:"peer_source,omitempty"`
	// OwnerTidRaw (P0-E2a): the raw owner tid the contention payload printed
	// when it is NOT present in this trace (the phantom cross-namespace id).
	// Non-zero only when rung ① failed (HolderSource=wakeup_edge /
	// ns_span_derivation, or the row stayed unresolved); carried as an audit
	// note so the original payload claim is never lost.
	OwnerTidRaw int `json:"owner_tid_raw,omitempty"`
	// HolderNsUnification (LCK-2, §18.E.1): the typed ②×③ identity-unification
	// declaration — set when the rung-② ns-span derivation and the rung-③
	// closing wakeup edge INDEPENDENTLY name the same host thread for the
	// payload owner ns-tid ("owner_ns_tid=<N> host=<thread> lanes=…"). Two
	// independent lanes cross-corroborating one physical thread; comm
	// mismatches never veto it (soft disclosure only). System-produced value.
	HolderNsUnification string `json:"holder_ns_unification,omitempty"`
	// HolderHostProcess (LCK-2, §18.E ②c / §19 typed-pair pin): the
	// PROCESS-LEVEL ns-span derivation result ("tgid=<G> ns_pid=<P>
	// level=process[ comm=<name>]") when the owner's container tid could not
	// be mapped to a host THREAD. The host tgid is deliberately NEVER stuffed
	// into Peer.PID — the peer stays unresolved (or a rung-③ waker) and the
	// process identity rides this display note.
	HolderHostProcess string `json:"holder_host_process,omitempty"`
	// HolderHandoff (P0-E 锁车道修2, ledger §24.9-C F2, 2026-07-09): the
	// verbatim payload "#A -->#B" hand-off chain elements when the owner
	// segment recorded MORE THAN ONE holder — a PRECISE payload witness that
	// the lock changed hands during this wait, so a single holder never held
	// for the whole span. The resolved Peer is the FINAL holder; per-holder
	// tenure boundaries are NOT in the payload, so the attribution stays the
	// conservative whole-span value WITH the typed disclosure (segmenting
	// without boundaries would invent data). nil = single-owner payload.
	HolderHandoff []string `json:"holder_handoff,omitempty"`
	// HolderSelfContradiction (P0-E 锁车道修2, §24.9-C F2 同锁自相矛盾守护):
	// the typed demotion witness — the INFERRED holder thread itself carried a
	// same-owner-tid contention span overlapping the majority of this span
	// (it was QUEUED on the same lock, so it cannot have been the whole-span
	// holder; opendir_78: the closing-wake "last releaser" main thread was
	// itself waiting 112.2ms of the 115.9ms span). The row's Peer is cleared
	// back to unresolved (§12.3 未解析不准入 keeps it out of the direct lane
	// and the 1.35 weight automatically); this value names the contradicting
	// span for the disclosure faces. Empty = guard never fired.
	HolderSelfContradiction string `json:"holder_self_contradiction,omitempty"`
	// WaitObject (P0-E2a, §10 A2): the blocking span's own name, published as
	// the wait object for payload-less blocking spans so the row can at least
	// say what it was blocked on when no structured owner was parseable.
	WaitObject string `json:"wait_object,omitempty"`
	// Waiters is the payload's "waiters=<n>" count (0 = not reported).
	Waiters int `json:"waiters,omitempty"`
	// MergedLines (P2-3, Q4-F root fold): line starts of same-lock duplicate
	// contention spans folded into this row at the engine source — fold gate
	// is fully typed (equal BlockingKind ∧ equal resolved owner PID ∧
	// overlapping intervals). The surviving row is the information-richer
	// form (owner comm + holder_site) and its DurationMs is the MAX of the
	// folded forms.
	MergedLines []int `json:"merged_lines,omitempty"`
	// DrillStatus (RCX① engine side, §12.3 ruling 1): drill verdict for the
	// row's counterpart lane (binder_wait / io_latency / resolved-contention
	// blocking_span rows only; empty on rows with no peer lane). See the
	// DrillStatus* constants in drill_status.go.
	DrillStatus string                `json:"drill_status,omitempty"`
	PeerState   *ThreadStateBreakdown `json:"peer_state,omitempty"`
	// PeerChain (A1 bounded continuation, ledger §12.3-5 ruling 5): ONE
	// sub-goal continuation hop off the resolved counterpart — the peer's own
	// state decomposition plus its single direct (1-hop) blocker. Depth is hard
	// capped at 1: the peer's peer is never expanded (q1 L31-33 deep-chain-blowup
	// lesson). Nil on rows whose peer is unresolved, or when the peer produced no
	// usable continuation. Display tier (peer_chain_* notes); the P0-A
	// projection/answer face consumes it, exactly like PeerState.
	PeerChain          *PeerChainStep `json:"peer_chain,omitempty"`
	Flags              string         `json:"flags,omitempty"`
	Oneway             *bool          `json:"oneway,omitempty"`
	SyncLike           *bool          `json:"sync_like,omitempty"`
	BlockingCandidate  *bool          `json:"blocking_candidate,omitempty"`
	ChainRelevance     string         `json:"chain_relevance,omitempty"`
	OverlapMs          float64        `json:"overlap_ms,omitempty"`
	EdgeCount          int            `json:"edge_count,omitempty"`
	NearestChainThread ThreadRef      `json:"nearest_chain_thread,omitempty"`
	NearestChainWindow TimeWindow     `json:"nearest_chain_window,omitempty"`
	DurationMs         float64        `json:"duration_ms,omitempty"`
	StartTs            float64        `json:"start_ts,omitempty"`
	EndTs              float64        `json:"end_ts,omitempty"`
	// ActualStartTs/ActualEndTs/ActualDurationMs (DCS E4 复核 F-1, ledger
	// §23.2, 2026-07-08): the blocking span's FULL B/E extent when the pair
	// straddled the query window boundary — DurationMs/StartTs/EndTs then
	// carry the WINDOW-CLIPPED projection (same dual-basis discipline as
	// TraceSpanSummary / the semantic rank lane). All zero when the span lay
	// entirely inside the window: absence is the precise "not clipped"
	// signal, never a guessed extent.
	ActualStartTs    float64 `json:"actual_start_ts,omitempty"`
	ActualEndTs      float64 `json:"actual_end_ts,omitempty"`
	ActualDurationMs float64 `json:"actual_duration_ms,omitempty"`
	// AbsorbedByRankFamily / AbsorbedIntoFamily (G1 跨车道对账, §27.2-G1 user
	// ruling 收口批准 §28.1, 2026-07-09, real_trace_campaign_20260705.md): the
	// typed cross-lane reconciliation marker. Stamped by
	// reconcileCriticalBlockingWithRankFamilies when the SAME result carries a
	// rank FAMILY row (foldSameThreadTypeRankFamilies merge, MemberCount ≥ 2)
	// of the same (thread, adjudicated type family, query window) whose member
	// interval union contains this row's own interval — the two lanes then
	// published the SAME batch of source events twice (opendir_79 E3↔E6-E9 /
	// huadong_79 E10↔E13,E19-E22: raw sums strictly equal). The row KEEPS
	// publishing on every observation face (观测照发不删 — evidence index /
	// audit lossless); the display layer folds only its tree/stanza RENDER
	// seat into the family row and notes the absorption there.
	// AbsorbedIntoFamily is the engine-rendered canonical family identity
	// (rankFamilyReconKey) — verbatim-equal to the family row's RankFamilyKey,
	// never a display-side label re-derivation. Absent family row → both stay
	// zero and rendering is byte-identical (负向保护).
	AbsorbedByRankFamily bool    `json:"absorbed_by_rank_family,omitempty"`
	AbsorbedIntoFamily   string  `json:"absorbed_into,omitempty"`
	LineStart            int     `json:"line_start,omitempty"`
	LineEnd              int     `json:"line_end,omitempty"`
	Confidence           float64 `json:"confidence,omitempty"`
	Summary              string  `json:"summary,omitempty"`
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

// PeerChainStep (A1 bounded continuation, ledger §12.3-5 ruling 5) is ONE
// sub-goal hop off a resolved blocking counterpart. It carries the peer's OWN
// state decomposition (State) and, when the peer is itself sleep-dominated, the
// single DIRECT (1-hop) blocker that woke the peer (DirectBlocker) plus that
// blocker's dominant state note. It is DELIBERATELY not recursive: the depth
// hard cap is 1, so DirectBlocker's own blocker is never expanded (the q1
// L31-33 deep-chain-blowup lesson). When the counterpart it hangs off was
// itself only wakeup-edge-inferred, Presumptive=true and the whole step is
// carried as inference, never as direct evidence.
type PeerChainStep struct {
	// Peer is the resolved counterpart this step decomposes (the lock holder /
	// binder peer / blocking-span object owner of the parent row).
	Peer ThreadRef `json:"peer,omitempty"`
	// State is the peer's own state breakdown over the parent blocking window —
	// what the counterpart itself was doing while it held the parent waiter up.
	State *ThreadStateBreakdown `json:"state,omitempty"`
	// DirectBlocker is the peer's own single 1-hop blocker (the thread that woke
	// the peer, when the peer was sleep-dominated). Empty when the peer was not
	// itself blocked on someone else (running/runnable/D-state dominant), no
	// wakeup edge names a real waker, or the only usable edge points back at the
	// PARENT WAITER itself (F1: in a sync request-reply shape the waiter wakes
	// the peer inside its own blocking window — naming the waiter as "the
	// blocker of its own blocker" is a causal inversion loop and such an edge is
	// DISCARDED outright, never annotated). NEVER expanded further — depth cap 1.
	DirectBlocker ThreadRef `json:"direct_blocker,omitempty"`
	// DirectBlockerState is the DirectBlocker's dominant state word only (never
	// a full second-hop breakdown — that would be depth 2). Empty when there is
	// no DirectBlocker.
	DirectBlockerState string `json:"direct_blocker_state,omitempty"`
	// DirectBlockerSource is the typed origin of DirectBlocker. The hop-2 name
	// has exactly ONE resolution lane today — the peer's own wakeup edge — so a
	// present DirectBlocker ALWAYS carries CounterpartSourceWakeupEdge (F2: the
	// hop-2 inference never rides a payload-direct facade, even when the peer
	// itself was payload-resolved). Empty when there is no DirectBlocker.
	DirectBlockerSource string `json:"direct_blocker_source,omitempty"`
	// Presumptive is true when the parent counterpart itself was resolved via
	// the wakeup-edge fallback (HolderSource/PeerSource=wakeup_edge): the whole
	// continuation then inherits presumptive confidence — an inference built on
	// an inference, never presented as direct evidence.
	Presumptive bool `json:"presumptive,omitempty"`
	// Confidence rides the counterpart-inference ceiling whenever ANY inference
	// is aboard: Presumptive (parent counterpart inferred) OR a present
	// DirectBlocker (hop-2 is structurally always a wakeup-edge inference, F2).
	// Only a payload-direct peer with no named blocker keeps direct-evidence
	// confidence (the state decomposition alone is timeline fact).
	Confidence float64 `json:"confidence,omitempty"`
	Summary    string  `json:"summary,omitempty"`
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
	// AggregatedImpactsFold is the PTS-2 (#69 用户条件裁定 2026-07-06) bounded
	// synthetic fold member for the aggregate top-8 trim: the rank>8 overflow
	// groups fold into this ONE O(1) summary (count + min–max DominantImpactMs
	// range + up-to-8 subject roster + line/ts envelope) instead of vanishing
	// behind the caveat count. nil when the trim never fired (≤8 groups —
	// zero-emission anti-noise). Deliberately NOT a member of
	// AggregatedImpacts: that slice is the wire VIEW and feeds
	// chainThreadRefs directly, and a synthetic member would contaminate it.
	// ORD (复核 P3-3 更正, 2026-07-10): root-cause ranking no longer reads
	// AggregatedImpacts — seat allocation reads the FULL pre-trim census
	// (rankAggregateCensus below), so the trim is a pure display-capacity
	// measure and never a seat gate. The min/max carry per-group display
	// values — wall clock never sums across threads.
	AggregatedImpactsFold *WakeupCausalAggregateFold `json:"aggregated_impacts_fold,omitempty"`
	IPCEdges              []IPCEdge                  `json:"ipc_edges,omitempty"`
	BinderWaits           []BinderWaitSummary        `json:"binder_waits,omitempty"`
	RootEvidence          []RootEvidence             `json:"root_evidence,omitempty"`
	// ViaThread is the RN-14a (§7.9) via verdict, present only when
	// Query.ViaThread was set: either the via thread is ON a wakeup path to
	// the target (depth + per-hop latency from existing wakeup edges, zero
	// new parsing) or it is NOT connected by any wakeup edge, in which case
	// its influence is scheduling contention (runnable queuing), not a
	// wakeup dependency — the decisive on-chain-root-cause vs
	// scheduling-contention distinction the customer session lacked.
	ViaThread *ChainViaThreadReport `json:"via_thread,omitempty"`
	Caveats   []string              `json:"caveats,omitempty"`
	// rankAggregateCensus (ORD, ledger §29.8 P2③, 2026-07-10; engine-internal,
	// never serialized): the FULL pre-trim aggregate list. AggregatedImpacts
	// above is the top-8 VIEW (PTS: a derived view with a capacity trim +
	// fold disclosure); seat allocation must read the full family census so a
	// family beyond the view trim still competes for its rank seat
	// (aggregate top-8 折叠吞携榜席成员 — the trim is display capacity, never
	// a seat gate). Empty (e.g. a JSON-roundtripped chain) degrades to the
	// trimmed view — see chainRankAggregateCensus.
	rankAggregateCensus []WakeupCausalAggregate
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
	// Depth is the node's TRUE recursion depth inside its own branch chain
	// (target = 0, its waker = 1, …) — set unconditionally by expandChain,
	// INCLUDING nil-Impact transit nodes (P0-E CHAIN-PATH 根修, ledger §22.1:
	// the pre-P0-E flattened walk defaulted nil-impact nodes to depth 0 and
	// minted fake L26/L27 trunk positions downstream). Zero-value on
	// hand-built legacy fixtures: consumers fall back to Impact.ChainDepth.
	Depth int `json:"depth,omitempty"`
	// Branch is the 1-based ordinal of the top-level target segment expansion
	// this node belongs to (one BuildWakeupChain call expands one branch per
	// interesting target interval; each branch is a LINEAR parent chain by
	// construction — the visited map forbids revisits). 0 = legacy fixture
	// with no branch identity. The publication layer serializes ONE path per
	// branch instead of the retired cross-branch flattened walk (§22.1).
	Branch int `json:"branch,omitempty"`
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
	// Branch mirrors the owning branch ordinal of the edge's From/To nodes
	// (they share one branch by construction — edges never cross branches).
	// 0 = legacy fixture (P0-E CHAIN-PATH, ledger §22.1).
	Branch int `json:"branch,omitempty"`
}

type WakeupCausalImpact struct {
	Thread       ThreadRef  `json:"thread"`
	Window       TimeWindow `json:"window"`
	ActualWindow TimeWindow `json:"actual_window,omitempty"`
	ChainDepth   int        `json:"chain_depth,omitempty"`
	// ChainBranch is the 1-based branch ordinal this impact row was measured
	// in (same identity as ChainNode.Branch — P0-E CHAIN-PATH, ledger §22.1).
	// It rides the chain_branch rich note so the display tree can key its
	// depth attach to (branch, depth) instead of a cross-branch flat position.
	// 0 = legacy row with no branch identity (absence never guesses).
	ChainBranch                int     `json:"chain_branch,omitempty"`
	OnChain                    bool    `json:"on_chain,omitempty"`
	DominantState              string  `json:"dominant_state,omitempty"`
	DominantImpactMs           float64 `json:"dominant_impact_ms,omitempty"`
	ProjectedImpactMs          float64 `json:"projected_impact_ms,omitempty"`
	TotalMs                    float64 `json:"total_ms,omitempty"`
	ProjectedTotalMs           float64 `json:"projected_total_ms,omitempty"`
	ActualImpactMs             float64 `json:"actual_impact_ms,omitempty"`
	ActualTotalMs              float64 `json:"actual_total_ms,omitempty"`
	RunningMs                  float64 `json:"running_ms,omitempty"`
	RunnableMs                 float64 `json:"runnable_ms,omitempty"`
	SleepMs                    float64 `json:"sleep_ms,omitempty"`
	DStateMs                   float64 `json:"d_state_ms,omitempty"`
	IOWaitMs                   float64 `json:"io_wait_ms,omitempty"`
	ActualRunningMs            float64 `json:"actual_running_ms,omitempty"`
	ActualRunnableMs           float64 `json:"actual_runnable_ms,omitempty"`
	ActualSleepMs              float64 `json:"actual_sleep_ms,omitempty"`
	ActualDStateMs             float64 `json:"actual_d_state_ms,omitempty"`
	ActualIOWaitMs             float64 `json:"actual_io_wait_ms,omitempty"`
	FragmentCount              int     `json:"fragment_count,omitempty"`
	StateSwitches              int     `json:"state_switches,omitempty"`
	MaxSegmentMs               float64 `json:"max_segment_ms,omitempty"`
	P95SegmentMs               float64 `json:"p95_segment_ms,omitempty"`
	TargetBlockedMs            float64 `json:"target_blocked_ms,omitempty"`
	LineStart                  int     `json:"line_start,omitempty"`
	LineEnd                    int     `json:"line_end,omitempty"`
	Priority                   int     `json:"priority,omitempty"`
	PriorityClass              string  `json:"priority_class,omitempty"`
	TargetPriority             int     `json:"target_priority,omitempty"`
	TargetPriorityClass        string  `json:"target_priority_class,omitempty"`
	PriorityRelation           string  `json:"priority_relation,omitempty"`
	PriorityInversionCandidate bool    `json:"priority_inversion_candidate,omitempty"`
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
	// GatedCapabilitySource (CAP §26 C3): typed capability caliber of the
	// discounted running component (CoreCapabilitySource* tokens) — set iff
	// GatedRunningDeficitMs > 0; wording input only, no gate reads it.
	GatedRunnableMs       float64 `json:"gated_runnable_ms,omitempty"`
	GatedRunningDeficitMs float64 `json:"gated_running_deficit_ms,omitempty"`
	GatedCapabilitySource string  `json:"gated_capability_source,omitempty"`
	// GatedClusterTopology (CAP-2 §28.4/§28.5): typed cluster-topology source
	// of the capability map that priced the discounted running component —
	// CoreCapabilityTopology* tokens, empty on explicit/legacy records
	// (byte-preserving absence). Wording input only, no gate reads it.
	GatedClusterTopology string `json:"gated_cluster_topology,omitempty"`
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

// GatedCaliber* enumerate WakeupCausalAggregate.GatedAggregationCaliber
// (P0-E §20 E-Gap②): whether the aggregate's gated inversion value is the
// SUM of its members (member occurrence windows pairwise disjoint — wall
// additive for one thread) or the honest MAX fallback (windows overlap, a sum
// could double-count the shared physical segment).
const (
	GatedCaliberSumDisjointOccurrences = "sum_disjoint_occurrences"
	GatedCaliberMaxOverlapFallback     = "max_overlap_fallback"
)

type WakeupCausalAggregate struct {
	Thread ThreadRef `json:"thread"`
	Path   string    `json:"path,omitempty"`
	// ChainDepth is the MIN member depth; ChainBranch is the members' shared
	// branch ordinal when ALL members were measured in ONE branch, 0 when the
	// occurrences span branches (a cross-branch aggregate has no single branch
	// identity — absence never guesses; P0-E CHAIN-PATH, ledger §22.1).
	ChainDepth        int     `json:"chain_depth,omitempty"`
	ChainBranch       int     `json:"chain_branch,omitempty"`
	OccurrenceCount   int     `json:"occurrence_count,omitempty"`
	DominantState     string  `json:"dominant_state,omitempty"`
	DominantImpactMs  float64 `json:"dominant_impact_ms,omitempty"`
	ProjectedImpactMs float64 `json:"projected_impact_ms,omitempty"`
	TotalMs           float64 `json:"total_ms,omitempty"`
	ProjectedTotalMs  float64 `json:"projected_total_ms,omitempty"`
	ActualImpactMs    float64 `json:"actual_impact_ms,omitempty"`
	ActualTotalMs     float64 `json:"actual_total_ms,omitempty"`
	RunningMs         float64 `json:"running_ms,omitempty"`
	RunnableMs        float64 `json:"runnable_ms,omitempty"`
	SleepMs           float64 `json:"sleep_ms,omitempty"`
	DStateMs          float64 `json:"d_state_ms,omitempty"`
	IOWaitMs          float64 `json:"io_wait_ms,omitempty"`
	ActualRunningMs   float64 `json:"actual_running_ms,omitempty"`
	ActualRunnableMs  float64 `json:"actual_runnable_ms,omitempty"`
	ActualSleepMs     float64 `json:"actual_sleep_ms,omitempty"`
	ActualDStateMs    float64 `json:"actual_d_state_ms,omitempty"`
	ActualIOWaitMs    float64 `json:"actual_io_wait_ms,omitempty"`
	TargetBlockedMs   float64 `json:"target_blocked_ms,omitempty"`
	FragmentCount     int     `json:"fragment_count,omitempty"`
	StateSwitches     int     `json:"state_switches,omitempty"`
	MaxSegmentMs      float64 `json:"max_segment_ms,omitempty"`
	FirstTs           float64 `json:"first_ts,omitempty"`
	LastTs            float64 `json:"last_ts,omitempty"`
	ActualFirstTs     float64 `json:"actual_first_ts,omitempty"`
	ActualLastTs      float64 `json:"actual_last_ts,omitempty"`
	LineStart         int     `json:"line_start,omitempty"`
	LineEnd           int     `json:"line_end,omitempty"`
	PriorityRelation  string  `json:"priority_relation,omitempty"`
	PriorityInversion bool    `json:"priority_inversion_candidate,omitempty"`
	// PriorityInversionGatedMs / GatedRunnableMs / GatedRunningDeficitMs
	// (P0-E §20 E-Gap②, 2026-07-07): the R5d gated caliber on the AGGREGATE
	// face — R5d formerly landed only on the per-occurrence lane, so
	// inversion-typed aggregate rank rows competed with their RAW blocking
	// magnitude. Aggregation legality (per-thread wall clock): the gated
	// components are wall-clock subsets of each member occurrence's own
	// window (runnable intervals in full + the weak-core deficit share of
	// running intervals), so when the member windows are PAIRWISE
	// NON-OVERLAPPING the underlying intervals are disjoint and the SUM is a
	// genuine wall figure for that one thread (same disjointness argument as
	// the N2 distinct-fact ruling and the cpu_occupancy per-thread merge).
	// Overlapping member windows (branch windows can share one physical
	// segment — the PTV6 envelope-overlap veto shape) may double-count the
	// same interval, so the value honestly degrades to the member MAX (a
	// lower bound). GatedAggregationCaliber says which caliber was used
	// (GatedCaliber* constants); empty when no member carried a gated value.
	PriorityInversionGatedMs float64 `json:"priority_inversion_gated_ms,omitempty"`
	GatedRunnableMs          float64 `json:"gated_runnable_ms,omitempty"`
	GatedRunningDeficitMs    float64 `json:"gated_running_deficit_ms,omitempty"`
	GatedAggregationCaliber  string  `json:"gated_aggregation_caliber,omitempty"`
	// GatedCapabilitySource (CAP §26 C3): typed capability caliber of the
	// members' discounted running components (one query resolves ONE
	// capability judgment, so members never disagree); set iff
	// GatedRunningDeficitMs > 0.
	GatedCapabilitySource string `json:"gated_capability_source,omitempty"`
	// GatedClusterTopology (CAP-2 §28.4/§28.5): the members' typed cluster-
	// topology source (uniform per query, first non-empty wins — same rule as
	// GatedCapabilitySource above).
	GatedClusterTopology string                   `json:"gated_cluster_topology,omitempty"`
	OccurrenceWindows    []WakeupCausalOccurrence `json:"occurrence_windows,omitempty"`
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

// WakeupCausalAggregateFold is the PTS-2 engine-level aggregate-trim fold
// (see ChainResult.AggregatedImpactsFold): a bounded synthesis of the rank>8
// aggregate groups. Groups counts the folded GROUPS (each already ≥2
// occurrences by the aggregate threshold); MinImpactMs/MaxImpactMs carry the
// members' DominantImpactMs display range (MAX is the fold's headline value —
// wall clock never sums across threads); Subjects keeps up to 8 member thread
// labels (mirror of the PTV5 wire-cap fold roster bound). The envelope fields
// span the members' line/ts extents for evidence anchoring.
type WakeupCausalAggregateFold struct {
	Groups      int      `json:"groups"`
	MinImpactMs float64  `json:"min_impact_ms,omitempty"`
	MaxImpactMs float64  `json:"max_impact_ms,omitempty"`
	Subjects    []string `json:"subjects,omitempty"`
	LineStart   int      `json:"line_start,omitempty"`
	LineEnd     int      `json:"line_end,omitempty"`
	FirstTs     float64  `json:"first_ts,omitempty"`
	LastTs      float64  `json:"last_ts,omitempty"`
	// SameValueMembers (P2-1, §29.6 G12-ENG batch, 2026-07-09): the members
	// whose DominantImpactMs ties the fold's published MAX to the µs (strict
	// |v−max| < types.TraceCausalProjectionSameValueTieMS band — the same
	// ruler every other take-MAX merge point uses), each with its OWN line
	// interval so a suspected same-segment double can be checked from the
	// report. Cap 4, minted only when ≥2 labeled members tie. Disclosure
	// ONLY — Groups/Min/Max and the published value are final before this
	// roster is computed (zero weight).
	SameValueMembers []WakeupCausalAggregateFoldTieMember `json:"same_value_members,omitempty"`
}

// WakeupCausalAggregateFoldTieMember is one (label, line-range) entry of the
// P2-1 tie roster — the engine-side twin of the projection fold's
// TraceCausalProjectionSameValueMember shape.
type WakeupCausalAggregateFoldTieMember struct {
	Label     string `json:"label"`
	LineStart int    `json:"line_start,omitempty"`
	LineEnd   int    `json:"line_end,omitempty"`
}

type RootEvidence struct {
	Type       string    `json:"type"`
	Thread     ThreadRef `json:"thread"`
	DurationMs float64   `json:"duration_ms,omitempty"`
	LineStart  int       `json:"line_start,omitempty"`
	LineEnd    int       `json:"line_end,omitempty"`
	Summary    string    `json:"summary,omitempty"`
	Confidence float64   `json:"confidence,omitempty"`
	// GapKind (G2 判据 typed 化, §27.2/§28.1, 2026-07-09): set on Type ==
	// "trace_gap" evidence only — the precise blind-spot criterion
	// (TraceGapKindNoSchedData / TraceGapKindNoEligibleWait) decided at the
	// expandChain mint site from the thread's own timeline shape.
	GapKind string `json:"gap_kind,omitempty"`
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
