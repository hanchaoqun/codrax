package tracequery

import (
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

const ParserVersion = "tracequery-v33"

type EventType string

const (
	EventUnknown            EventType = "unknown"
	EventSchedSwitch        EventType = "sched_switch"
	EventSchedWakeup        EventType = "sched_wakeup"
	EventSchedWaking        EventType = "sched_waking"
	EventSchedBlockedReason EventType = "sched_blocked_reason"
	EventSchedStat          EventType = "sched_stat"
	// EventPriorityMutation is an exact-name scheduler-priority mutation
	// observation. Until a producer-specific old/new priority mapping is
	// proven, it is a range poison only: WakeePID identifies the uniquely
	// parsed subject, or zero means the mutation could not be scoped safely.
	EventPriorityMutation EventType = "priority_mutation"
	EventTaskRename       EventType = "task_rename"
	// EventRSSStat and EventPhaseTaskDelta are exact-name, context-only
	// inventory types. They deliberately carry no memory, scheduler, span,
	// plugin, causal, or root-rank authority.
	EventRSSStat           EventType = "rss_stat"
	EventPhaseTaskDelta    EventType = "phase_task_delta"
	EventCPUIdle           EventType = "cpu_idle"
	EventCPUFrequency      EventType = "cpu_frequency"
	EventCPUFrequencyLimit EventType = "cpu_frequency_limits"
	EventCPUConstraint     EventType = "cpu_constraint"
	EventClockSetRate      EventType = "clock_set_rate"
	EventBlockIssue        EventType = "block_rq_issue"
	EventBlockRemap        EventType = "block_bio_remap"
	EventBlockComplete     EventType = "block_rq_complete"
	EventBinderTransaction EventType = "binder_transaction"
	EventBinderReceived    EventType = "binder_transaction_received"
	EventBinderAllocBuf    EventType = "binder_transaction_alloc_buf"
	EventBinderLock        EventType = "binder_lock"
	EventBinderLocked      EventType = "binder_locked"
	EventBinderUnlock      EventType = "binder_unlock"
	EventBinderReply       EventType = "binder_reply"
	EventIRQ               EventType = "irq"
	EventSoftIRQ           EventType = "softirq"
	EventIPI               EventType = "ipi"
	EventTraceMark         EventType = "trace_mark"
	EventMemory            EventType = "memory"
	EventStorage           EventType = "storage"
	EventFilesystem        EventType = "filesystem"
	EventPower             EventType = "power"
	EventAbilityMonitor    EventType = "ability_monitor"
	EventXPower            EventType = "xpower"
	EventHiSystemEvent     EventType = "hi_sysevent"
	EventHiLog             EventType = "hilog"
	EventWorkqueue         EventType = "workqueue"
	EventDMAFence          EventType = "dma_fence"
	EventPerfSample        EventType = "perf_sample"
)

const (
	WakeePrioritySourceInferredNextSchedSlice = "inferred_next_sched_slice"
	WakeePrioritySourceUnknown                = "unknown"
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
	// Line is the index-global evidence coordinate. It equals the physical
	// 1-based line for a single artifact; a composite index rebases it into a
	// stable non-overlapping virtual range. ResolveArtifactSpans is the only
	// supported conversion back to source artifact + local line.
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

	// CPU scalar transports are fixed-width before Event construction. int64
	// keeps uint32 idle/frequency sentinels and clock rates byte-identical on
	// 32- and 64-bit hosts; semantic consumers apply their own narrower gate.
	State            int64 `json:"state,omitempty"`
	Frequency        int64 `json:"frequency,omitempty"`
	FrequencyMin     int64 `json:"frequency_min,omitempty"`
	FrequencyMax     int64 `json:"frequency_max,omitempty"`
	CPUForField      int   `json:"cpu_for_field,omitempty"`
	CPUForFieldValid bool  `json:"cpu_for_field_valid,omitempty"`
	// CPUForFieldPresent prevents a malformed explicit cpu_id from silently
	// falling back to the row-header CPU.
	CPUForFieldPresent bool `json:"-"`
	// TargetCPUValid separates an explicit, validated cpu0 from an absent or
	// malformed target_cpu token. It sits beside the other CPU validity bits
	// so the hot Event struct uses existing alignment padding.
	TargetCPUValid bool `json:"-"`
	// CPUInputInvalid is a zero-allocation parse-to-index handoff. The
	// raw validator is invoked only for marked/rejected rows, keeping the
	// overwhelmingly common valid parse lane allocation-flat.
	CPUInputInvalid bool `json:"-"`
	// Two bits encode exact/inferred/unknown/untrusted wakeup-priority
	// authority. Keep them beside the existing validity bits so they consume
	// alignment padding rather than adding a string to every scheduler event.
	WakeePrioInferred bool `json:"-"`
	WakeePrioUnknown  bool `json:"-"`
	// BlockedReasonIOWaitKnown separates a proven iowait=0 from a malformed
	// or missing declaration. BlockedDelayKnown distinguishes a canonical
	// positive vendor delay from an absent, zero, or malformed declaration.
	// Both occupy the Event core's existing bool padding.
	BlockedReasonIOWaitKnown bool   `json:"-"`
	BlockedDelayKnown        bool   `json:"-"`
	ClockName                string `json:"clock_name,omitempty"`
	Reason                   string `json:"reason,omitempty"`
	// IOWait is int32 so the BlockedDelay pair below shares the former
	// 8-byte int slot (P4 core-size ratchet: the Event core must not grow;
	// iowait is a 0/1 flag domain).
	IOWait int32 `json:"io_wait,omitempty"`
	// BlockedDelay (件1 census 根修, 2026-07-13): the sched_blocked_reason
	// row's vendor delay field, RAW as printed (HarmonyOS prints µs; the
	// mainline format carries no delay → 0; int32 caps at ~35.8min of µs —
	// far above any kernel block delay). Only the blocked_reason census
	// consumes it; absence never guesses.
	BlockedDelay int32 `json:"blocked_delay,omitempty"`
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

func eventWakeePriorityForHardUse(ev Event) int {
	if ev.WakeePrioInferred || ev.WakeePrioUnknown {
		return 0
	}
	return ev.WakeePrio
}

func (ev Event) WakeePrioritySource() string {
	switch {
	case ev.WakeePrioInferred && ev.WakeePrioUnknown:
		return "untrusted"
	case ev.WakeePrioInferred:
		return WakeePrioritySourceInferredNextSchedSlice
	case ev.WakeePrioUnknown:
		return WakeePrioritySourceUnknown
	default:
		return ""
	}
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
	Comm           string `json:"constraint_comm,omitempty"`
	PID            int    `json:"constraint_pid,omitempty"`
	Kind           string `json:"constraint_kind,omitempty"`
	Policy         string `json:"constraint_policy,omitempty"`
	CPU            int    `json:"constraint_cpu,omitempty"`
	CPUPresent     bool   `json:"-"`
	CPUValid       bool   `json:"-"`
	OrigCPU        int    `json:"constraint_orig_cpu,omitempty"`
	OrigCPUPresent bool   `json:"-"`
	OrigCPUSet     bool   `json:"-"`
	DestCPU        int    `json:"constraint_dest_cpu,omitempty"`
	DestCPUPresent bool   `json:"-"`
	DestCPUSet     bool   `json:"-"`
	AllowedText    string `json:"allowed_cpus_text,omitempty"`
	Allowed        []int  `json:"allowed_cpus,omitempty"`
	AllowedPresent bool   `json:"-"`
	AllowedValid   bool   `json:"-"`
	CPUSetName     string `json:"cpuset,omitempty"`
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

	// The private argset receipt separates valid producer zero from
	// absent/duplicate/malformed wire. Production ParseLine always sets
	// argsetParsed for endpoint rows; hand-built typed Events retain their
	// compatibility lane without becoming a text-parser authority.
	argsetParsed     bool   `json:"-"`
	argsetLexValid   bool   `json:"-"`
	transactionKnown bool   `json:"-"`
	destProcKnown    bool   `json:"-"`
	destThreadKnown  bool   `json:"-"`
	replyKnown       bool   `json:"-"`
	flagsKnown       bool   `json:"-"`
	codeKnown        bool   `json:"-"`
	flagsValue       uint32 `json:"-"`
	codeValue        uint32 `json:"-"`
}

// BlockIOFields is the block_* event-family side table.
type BlockIOFields struct {
	Dev              string `json:"block_dev,omitempty"`
	Op               string `json:"block_op,omitempty"`
	Sector           int64  `json:"block_sector,omitempty"`
	Len              int64  `json:"block_len,omitempty"`
	Error            string `json:"block_error,omitempty"`
	SrcDev           string `json:"block_src_dev,omitempty"`
	SrcSector        int64  `json:"block_src_sector,omitempty"`
	RemapBios        int64  `json:"block_remap_bios,omitempty"`
	RemapBiosPresent bool   `json:"block_remap_bios_present,omitempty"`
	// IdentityParsed distinguishes production parser output from hand-built
	// compatibility Events. Production block endpoints must carry an explicit,
	// fully validated dev/op/sector/len tuple; in particular sector=0 is valid,
	// while a missing/overflowed sector must not collapse to the same value.
	IdentityParsed bool `json:"-"`
	IdentityValid  bool `json:"-"`
}

// ResourceFields is the memory/storage/filesystem resource side table.
type ResourceFields struct {
	Path      string  `json:"resource_path,omitempty"`
	Op        string  `json:"resource_op,omitempty"`
	LatencyMs float64 `json:"resource_latency_ms,omitempty"`
	Bytes     int64   `json:"resource_bytes,omitempty"`
	Address   string  `json:"resource_address,omitempty"`
	Callstack string  `json:"resource_callstack,omitempty"`

	// mmcPairing carries the full-right-edge admission verdict for the two
	// exact MMC request endpoints. ResourceFields is already the sparse side
	// table for storage rows, so scheduler/core Events pay no size cost.
	mmcPairing *mmcPairingAdmission `json:"-"`
	// f2fsPairing is the sole retained exact-body verdict for the six governed
	// F2FS request endpoints. Display-friendly FileFields never substitutes it.
	f2fsPairing *f2fsPairingAdmission `json:"-"`
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

	// pageCacheMutation is the sole parse-to-aggregate authority for the
	// mm_filemap add/delete mutation lane.  It is deliberately internal: the
	// public projection remains the historically stable dev/inode/offset
	// tuple, while malformed or merely similar event names retain inventory
	// visibility without acquiring page-cache churn semantics.
	pageCacheMutation pageCacheMutationKind `json:"-"`
}

// PluginFields is the rare ability/xpower/hisysevent and extended trace-mark
// auxiliary side table. EventTraceMark allocates it for G/H/N rows whose exact
// Android track_name must survive as a typed field and for C rows whose full
// admission-time payload verdict must outlive Event.FieldText's 300-byte
// inventory clamp. Ordinary B/E/S/F/I rows and every scheduler event keep the
// pointer nil.
type PluginFields struct {
	Domain    string              `json:"plugin_domain,omitempty"`
	EventName string              `json:"plugin_event_name,omitempty"`
	Metric    string              `json:"plugin_metric,omitempty"`
	Value     string              `json:"plugin_value,omitempty"`
	Category  string              `json:"plugin_category,omitempty"`
	SpanTrack string              `json:"span_track,omitempty"`
	Counter   *TraceCounterFields `json:"-"`
}

// TraceCounterFields is the sparse, internal admission-time C| handoff. It is
// intentionally absent from Event's historical JSON surface; public counter
// provenance lives on TraceCounterSummary / TraceCounterDeltaSummary.
type TraceCounterFields struct {
	OwnerRaw      string
	OwnerScope    string
	Metadata      string
	OutputLevel   string
	TagBits       string
	IssueReason   string
	NumericValue  float64
	Parsed        bool
	NumericValid  bool
	IdentityValid bool
}

// PerfFields is the EventPerfSample side table — the single largest block of
// the historical flat Event (39 fields after typed SQL-perf provenance was
// added, ~416 bytes before that addition) carried by every
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
	ThreadIdentityKnown *bool  `json:"perf_thread_identity_known,omitempty"`
	Resolution          string `json:"perf_resolution,omitempty"`
	LifecycleUnverified *bool  `json:"perf_lifecycle_unverified,omitempty"`
	SourcePID           int    `json:"perf_source_pid,omitempty"`
	SourceTID           int    `json:"perf_source_tid,omitempty"`
	SourceComm          string `json:"perf_source_comm,omitempty"`
	SampleKind          string `json:"perf_sample_kind,omitempty"`
	SampleKindSource    string `json:"perf_sample_kind_source,omitempty"`
	SymbolizationStatus string `json:"perf_symbolization_status,omitempty"`
	Clock               string `json:"perf_clock,omitempty"`
	CPUKnown            *bool  `json:"perf_cpu_known,omitempty"`
	ClockConfidence     string `json:"perf_clock_confidence,omitempty"`
	CallchainStatus     string `json:"perf_callchain_status,omitempty"`
	ParserCaveats       string `json:"perf_parser_caveats,omitempty"`
	// PerfTextIntegrity and PerfWeightInvalid are parser-owned internal
	// provenance. They drive bounded quality disclosure and prevent an invalid
	// explicit weight from silently becoming the ordinary legacy default 1;
	// they are not part of the historical flat Event JSON surface.
	PerfTextIntegrity string `json:"-"`
	PerfWeightInvalid bool   `json:"-"`
}

// PerfThreadIdentity is the typed, capture-local identity of one admitted
// perf thread cohort. Comm is deliberately display metadata: numeric TID plus
// the exact observed lifecycle generation are the hard identity. TGID is a
// cohort consistency check and is omitted when it was not positively proved.
// CommAliases is a bounded, sorted projection. CommAliasCount is exact only
// when CommAliasesTruncated is false; an authority-cap hit instead publishes
// CommAliasCountAtLeast plus the typed truncation bit and never fabricates an
// exact total.
type PerfThreadIdentity struct {
	TID                   int      `json:"tid"`
	TGID                  int      `json:"tgid,omitempty"`
	Generation            int      `json:"generation"`
	DisplayComm           string   `json:"display_comm,omitempty"`
	CommAliases           []string `json:"comm_aliases,omitempty"`
	CommAliasCount        int      `json:"comm_alias_count,omitempty"`
	CommAliasCountAtLeast int      `json:"comm_alias_count_at_least,omitempty"`
	CommAliasesTruncated  bool     `json:"comm_aliases_truncated,omitempty"`
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

// TraceTimestampOrder is a complete-artifact proof about timestamp order in
// physical line order. Unknown is deliberately the zero value: a prefix with
// no observed regression is NOT proof that the unread suffix cannot move back
// into a requested time window. Only a scan that reaches EOF may publish one
// of the two complete states.
type TraceTimestampOrder uint8

const (
	TraceTimestampOrderUnknown TraceTimestampOrder = iota
	TraceTimestampOrderMonotonic
	TraceTimestampOrderRegressed
)

// AllowsTimeEndEarlyStop is the precise hard-gate signal for stopping a
// physical line scan at the first timestamp beyond time_end. No heuristic,
// prefix counter, or running maximum is accepted here.
func (o TraceTimestampOrder) AllowsTimeEndEarlyStop() bool {
	return o == TraceTimestampOrderMonotonic
}

type Index struct {
	Path    string
	Size    int64
	ModTime time.Time
	// TraceArtifacts is the physical-source ledger for this index. Event.Line
	// is an index-global virtual coordinate; ResolveArtifactSpans maps it back
	// to the source artifact's 1-based local line and clock domain. A normal
	// single-file index has one entry with VirtualLineBase=0. Multi-artifact
	// indexes keep incompatible clock domains in this ledger with
	// CausalCompatible=false, but never admit their events into Events.
	TraceArtifacts   []TraceArtifactSource
	LineCount        int
	ScannedLineCount int
	Windowed         bool
	// pairingTopologyComplete is the precise proof that a Windowed index carries
	// the complete physical endpoint topology required by elapsed/IPC pairing.
	// Current cropped and derive-from-full builders do not materialize such a
	// sidecar and therefore leave this false. Non-windowed indexes are complete
	// by construction and do not consult the bit. A future bounded topology
	// sidecar may set it only after prefix, tail, source and lifecycle coverage
	// have all been proven.
	pairingTopologyComplete bool
	// RelationScoped is true only when the streamed parser actually pruned
	// Events to a target/waker relation closure.  It is a precise consumer
	// gate: global all-thread aggregates must never combine this subset with
	// full-artifact scheduler carry-in state.
	RelationScoped    bool
	relationScopeTIDs map[int]bool
	// relationScopePriorityComplete is the parser-owned positive proof that
	// relation pruning retained every priority mutation for relationScopeTIDs
	// plus every PID-0 global mutation. RelationScoped alone is a display/view
	// flag and cannot authorize absence claims on legacy hand-built indexes.
	relationScopePriorityComplete bool
	// platformSurfaces is the per-trace platform-detection input record
	// (W-1 修根, platform_surfaces.go): stamped at build time (per-file
	// anchor record preferred, else this build's own full-parse vote;
	// composite bundles OR-merge children). Query-time platform resolution
	// consumes THIS record — never a per-query window/filter-scoped event
	// subset — so every view of one trace answers with one label.
	platformSurfaces platformSurfaceScan
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
	// TimestampOrder is set only from a complete EOF scan (or from a merged
	// event stream that is deterministically sorted after every child has been
	// scanned). It is the typed authority for any query-side time_end early
	// stop. ClockRegressions remains the diagnostic count; zero by itself is
	// never a completeness proof.
	TimestampOrder TraceTimestampOrder
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
	// only inside the safety padding tail: a complete-artifact timestamp-order
	// proof is monotonic and the budget-tripping event's ts lies STRICTLY beyond
	// the requested TimeEnd, so the core [TimeStart,TimeEnd] window lost zero
	// events. Typed input for the query layer's
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
	// CPU frequency has two memoized views over one typed authority:
	// freqTransitionTimelines retains canonical zero carry barriers for state
	// governance; freqTimelines is its positive-only hard-evidence projection
	// for cluster/CAP/rail/fmax. Neither map is mutable after publication.
	freqTransitionTimelinesOnce sync.Once
	freqTransitionTimelines     map[int][]freqSample
	freqTimelinesOnce           sync.Once
	freqTimelines               map[int][]freqSample
	// freqTimelinesBasis / freqTimelinesDropped (CLUSTER-FIX-1, user ruling
	// 2026-07-18): the typed ClusterSampleBasis* token for the ACTIVE sample
	// basis behind the shared transition authority (full_index / side_scan /
	// window_carve — precise chain of collected/degrade flags, set beside the
	// freqTransitionTimelines memo), and the sorted cpu_frequency lanes the
	// physical order-integrity audit REMOVED from that basis (S4 收披露: a
	// dropped lane can silently lower the cluster count when it was a
	// cluster's only sampled member — judgment unchanged, the caveat lane
	// discloses). Disclosure inputs only, no gate reads either.
	freqTimelinesBasis   string
	freqTimelinesDropped []int
	// freqLimitTimelinesDropped (CLUSTER-FIX-2 件6 = §29.150 用户裁定⑨): the
	// cpu_frequency_limits twin of freqTimelinesDropped — sorted limits lanes
	// the physical order-integrity audit removed from the ACTIVE basis
	// (full_index / side_scan roster from the curve set's poison application;
	// window_carve roster from the events-fallback integrity filter). Written
	// inside the same freqTransitionTimelinesOnce.Do (identical concurrency
	// contract); the caveat lane discloses 「fmax 阶梯可能低估」. Disclosure
	// input only, no gate reads it; the drop judgment is byte-unchanged.
	freqLimitTimelinesDropped []int
	// freqLimitTimelines (CLUSTERTIE-1 件A, §29.200, 2026-07-21): the KEPT
	// cpu_frequency_limits lanes of the SAME basis decision above (the
	// complement of freqLimitTimelinesDropped, written inside the same
	// freqTransitionTimelinesOnce.Do — the two frequency lanes can never read
	// two collection generations of one file). Input to the announcement
	// snapshot partition's limits extra-evidence sub-veto only
	// (deriveClusterFreqDomainsLimits); the fold's fmax ladder keeps its own
	// buildFreqLimitIndex view untouched. READ-ONLY once built.
	freqLimitTimelines map[int][]freqSample
	// sideFreqOnce/sideFreq/sideFreqDegrade back the CLUSTER-FIX-1 streaming
	// full-file frequency side-scan memo (freq_side_scan.go): assembled at
	// most once per Index from the per-artifact side-scan cache; degrade is
	// the typed freqSideScanDegrade* token ("" = curves served). Copy-safe
	// like freqTimelinesOnce (Index is pointer-only throughout the package).
	sideFreqOnce    sync.Once
	sideFreq        fullFreqCurves
	sideFreqDegrade string
	// fullFreq is the R6 rule-4 full-file per-CPU frequency curve set
	// (full_freq_curves.go): collected in the same BuildIndex pass, EXEMPT
	// from the window gate / relation prune / MaxEvents admission, published
	// only when the scan covered the whole file. collected=false → the
	// consumers fall back to the CLUSTER-FIX-1 side-scan, then the historical
	// idx.Events basis. READ-ONLY once built.
	fullFreq fullFreqCurves
	// derivedClassOnce/derivedCapability back the R6 (§29.88.9) derived
	// trace-global capability memo (indexDerivedCoreCapability,
	// core_capability.go): the class judgment every window face reads when no
	// explicit core_topology exists, plus the cluster fmax tiers the R5a
	// 按核档 comparison consumes. A non-usable map = cluster structure
	// unjudgeable (freq_only) — faces degrade to the honest unclassified
	// form, never a positional guess. Lazily built once; copy-safe like
	// freqTimelinesOnce.
	derivedClassOnce  sync.Once
	derivedCapability coreCapabilityMap
	// clusterDomainsOnce/clusterDomains back the CLUSTERSTREAM-1 (§29.193.1)
	// Index-level lazy single derivation of the frequency-domain membership
	// (indexDerivedClusterFreqDomains, cluster_freq_share.go): the pairwise
	// witness derivation runs once per Index over the Index-global sample
	// basis and every query shares the memo (件1 复用纪律). Copy-safe like
	// freqTimelinesOnce; maps READ-ONLY once built.
	clusterDomainsOnce sync.Once
	clusterDomains     clusterFreqDomains
	// schedulerHeads retains the one immutable scheduler state snapshot built
	// for a bounded index's explicit window head. Full indexes derive requested
	// checkpoints on demand instead of retaining them outside the global LRU's
	// accounting. The map is private because these checkpoints are computation
	// inputs, not a JSON evidence face; TimelineResult publishes status.
	schedulerHeadMu    sync.Mutex
	schedulerHeads     map[uint64]*schedulerHeadSnapshot
	schedulerHeadOrder []uint64
	schedulerHeadBytes int64
	// schedulerOrderFailures preserve physical child/file exact same-CPU/TID
	// rollbacks across window pruning and composite canonical sorting. They are
	// bounded private
	// hard-gate input; duration consumers fail closed rather than treating the
	// sorted merge as proof that the source clock was monotonic.
	schedulerOrderFailures []schedulerOrderViolation
	// durationOrderFailures preserve physical child/file rollback proofs for
	// non-scheduler elapsed-time state machines (trace marks, counters,
	// interrupt lanes, workqueues and DMA fences). Composite canonical sorting
	// and warm window derivation must never erase these family-scoped poisons.
	durationOrderFailures       []durationOrderViolation
	durationOrderFailuresCapped map[durationOrderFamily]bool
	// schedulerRowIntegrityFailures preserve exact critical scheduler rows
	// that could not supply their required typed fields. They are kept outside
	// Events because such a row must never be materialized with fabricated
	// zero-valued PIDs/state, yet duration consumers still need a bounded
	// fail-closed witness.
	schedulerRowIntegrityFailures []schedulerRowIntegrityFailure
	// blockedReasonIntegrityFailures are field-local sched_blocked_reason
	// parser verdicts. They are deliberately separate from scheduler-row
	// failures: malformed optional marker metadata may withdraw D/IO/caller
	// refinement, but must never erase independently proven scheduler states.
	blockedReasonIntegrityFailures []blockedReasonIntegrityFailure
	// cpuInputIntegrityFailures retain a bounded typed witness for malformed
	// or out-of-range CPU scalar/range tokens that were deliberately excluded
	// from every attribution consumer.
	cpuInputIntegrityFailures       []cpuInputIntegrityFailure
	cpuInputIntegrityFailuresCapped bool
	// traceMarkIntegrityFailures retain bounded malformed endpoint/header
	// witnesses outside Event's public JSON surface. Unknown emitter identity
	// is a global pairing poison; known emitters reset locally and recover.
	traceMarkIntegrityFailures       []traceMarkIntegrityFailure
	traceMarkIntegrityFailuresCapped bool
	// traceMarkIntegrityDroppedGlobalPoison distinguishes a witness-cap hit
	// made only of materialized, known-emitter rows from a dropped endpoint
	// whose emitter/physical position was unavailable. The former remains
	// locally recoverable because every malformed Event still resets its own
	// emitter in the pairing scan; only the latter may globally fail-close.
	traceMarkIntegrityDroppedGlobalPoison bool
	// traceTrackIntegrityDroppedPoison is the G/H counterpart. Track pairing
	// cannot recover a dropped malformed endpoint from emitter-local state,
	// because its logical owner is the payload pid rather than the row emitter.
	traceTrackIntegrityDroppedPoison bool
	// threadIncarnationFailures preserve exact child/file-order lifecycle
	// conflicts across window pruning and composite canonical sorting. A set
	// overflow is itself a fail-closed signal; hard-gate evidence is never
	// silently truncated.
	threadIncarnationFailures       []threadIncarnationConflict
	threadIncarnationFailuresCapped bool
	// generationMetadataOnce/generationMetadataBoundaries provide the lazy
	// full-index lifecycle boundary table used by priority/CPU/TGID metadata
	// lookups. Full single-file indexes intentionally avoid eager lifecycle
	// audit allocation; this memo combines their event scan with any preserved
	// child/window proofs exactly once and is immutable after publication.
	generationMetadataOnce       sync.Once
	generationMetadataBoundaries map[int][]threadIncarnationConflict
	generationMetadataCapped     bool
	// perfGenerationHeads are candidate-filtered lifecycle checkpoints at a
	// bounded index's inclusive left edge, keyed by the same frozen capture or
	// source scope used by perfThreadKey. They are independent of
	// threadIncarnationFailures: a lifecycle cut wholly before the query window
	// advances perf generation without becoming a query-relevant scheduler
	// conflict. Invalid scopes fail closed only for perf thread attribution.
	perfGenerationHeads       map[string]*threadIncarnationTracker
	perfGenerationHeadInvalid map[perfThreadScopeTID]string
	// perfIdentityOnce/perfIdentity memoize the immutable ordinal-indexed
	// perf thread identity ledger. It is intentionally distinct from the
	// generic ThreadRef and lifecycle metadata caches: every derived Index is
	// built with an explicit literal, so its changed Events slice starts with a
	// zero Once and rebuilds ordinal ownership instead of copying a parent
	// ledger whose ordinals refer to a different slice.
	perfIdentityOnce                    sync.Once
	perfIdentity                        *perfIdentityLedger
	schedulerOrderFailuresCapped        bool
	schedulerRowIntegrityFailuresCapped bool
	// schedulerRowIntegrityOverflowSources retains the exact physical source
	// paths whose malformed scheduler witnesses overflowed the bounded audit
	// ledger. A naked capped bit would force unrelated tracebundle siblings to
	// lose hard priority authority. OverflowGlobal is raised only when even the
	// physical source cannot be retained exactly.
	schedulerRowIntegrityOverflowSources []string
	schedulerRowIntegrityOverflowGlobal  bool
	// Priority-mutation rows are proof poison, not scheduler-state
	// transitions. Their bounded audit lane is therefore independent from the
	// scheduler-state cap above: a flood of malformed sched_pi_setprio/
	// binder_set_priority rows must withdraw priority ranges without erasing a
	// separately valid running/runnable timeline (and vice versa).
	priorityMutationIntegrityFailuresCapped  bool
	priorityMutationIntegrityOverflowSources []string
	priorityMutationIntegrityOverflowGlobal  bool
	blockedReasonIntegrityFailuresCapped     bool
	blockedReasonIntegrityOverflow           blockedReasonIntegrityOverflowScope
	// Only PID-identity failures lose matcher-side candidate information when
	// Event is withdrawn as EventUnknown. Other malformed dimensions remain
	// fully represented on Event (known bits / Reason=unknown), so their audit
	// overflow must never withdraw valid D/IO classification.
	blockedReasonIdentityOverflow blockedReasonIntegrityOverflowScope
	// durationOrderEventScanOnce backs the lazy full-event duration-order
	// audit needed by non-monotonic indexes (perf audit #24, §29.25 处置委托
	// 2026-07-10): the scan core is query-independent (relevance is a pure
	// post-filter), so it runs once per index instead of once per view.
	// Same house pattern as generationMetadataOnce; Index is pointer-only
	// throughout the package, so the sync.Once is copy-safe.
	durationOrderEventScanOnce     sync.Once
	durationOrderEventScanFailures []durationOrderViolation
	durationOrderEventScanCapped   map[durationOrderFamily]bool
}

type Query struct {
	View              string
	Thread            string
	ThreadInput       string
	ThreadPIDInferred bool
	PID               int
	// TargetScope controls how PID applies to span/frame discovery. Empty and
	// "thread" preserve the exact scheduler-TID contract. "process" is
	// explicit opt-in and admits only spans whose emitter TGID or trace-mark
	// payload SpanPID exactly equals PID.
	TargetScope            string
	TimeStart              float64
	TimeEnd                float64
	TimeStartSet           bool
	TimeEndSet             bool
	LineStart              int
	LineEnd                int
	EventTypes             []EventType
	TraceMarkActions       []TraceMarkAction
	Pattern                string
	SpanName               string
	FrameWindowAutoDerived bool
	InteractionDirection   string
	RecipeName             string
	MaxDepth               int
	MaxBranches            int
	// MaxChainNodes is the CHAIN-BUDGET (user ruling 2026-07-18) global
	// chain-node budget for view=wakeup_chain and its rank/bundle consumers:
	// extra (top-2..k) sleep-segment expansions are admitted only while the
	// chain's node count sits below it. The guaranteed tier — depth-0
	// top-MaxBranches branches, each recursing its single most interesting
	// interval per node — is never gated by it, so MaxChainNodes=1 is the
	// tightest tier and reproduces the pre-CHAIN-BUDGET chain byte-for-byte,
	// modulo the single budget disclosure caveat (退化恒等; the honest
	// chain_expansion_budget_reached line is the one face the legacy build
	// never carried). Unset/zero collapses to the wakeup_chain capacity-table
	// default; larger requests clamp to it with a resource_clamped caveat.
	// Board identity: this knob is part of the XLANE-3 closed rank-knob
	// fingerprint set (预算变=异板).
	MaxChainNodes int
	MinDurationMs float64
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
	// normalizationCaveats records deterministic resource clamps applied by
	// normalizeQuery. It is unexported plumbing copied by value into every
	// nested view so both the top-level Result and direct builder faces can
	// disclose that an explicit request was narrowed.
	normalizationCaveats []string
	// timeStartBackfilled (WINFLAG-1, §29.190④) records that normalizeQuery
	// filled TimeStart from idx.FirstTs (the whole-trace window start — a
	// determined index fact that is legally 0 on a rebased export).
	// Unexported RESULT-side provenance consumed only by
	// queryResultWindowStartSet / queryWindowStartsAtDeterminedZero
	// (window_start_flag.go); it is NOT the API sentinel — TimeStartSet
	// keeps meaning "caller explicitly set time_start" and every existing
	// predicate on it is untouched.
	timeStartBackfilled bool
	// chainAnchorWindowsByPID (RSPA §29.61.10a/b/c, 2026-07-14). Unexported
	// in-package plumbing, never serialized: the merged typed wakeup-
	// dependency jump-window unions per chain pid (chainAnchorWindowsByPID
	// helper output), set ONLY by the rank/bundle lanes that computed the
	// chain BEFORE the stats sweep. computeOffCPUStats accumulates the
	// per-segment anchored overlap against these at its single ledger close
	// site. nil (every other caller) keeps the sweep byte-identical.
	chainAnchorWindowsByPID map[int][]TimeWindow
	// runCancel (SUPP-CANCEL, 2026-07-14) is the in-view cooperative-
	// cancellation carrier, set ONLY through Query.WithRunContext
	// (run_cancel.go). Unexported in-package plumbing, never serialized;
	// nil (tracediag lane, direct builder calls, tests) keeps every scan
	// byte-identical.
	runCancel *runCancelState
	// perfRoleContextScanProbe is test-only, per-Query instrumentation for the
	// root-rank role-index full-scan bound. nil in production.
	perfRoleContextScanProbe *perfRoleContextScanProbe
	// statsSweepProbe (RSPA-HYG 件②, §29.77 立案②, 2026-07-14). Unexported
	// TEST-ONLY sweep counter: when non-nil, ComputeWindowStats increments it
	// once per full sweep. The 单 Run 恰一次 sweep pin sets it on the Query it
	// hands to Run — a per-Run-scoped precise count, immune to parallel-test
	// pollution. nil everywhere in production; zero behavior effect.
	statsSweepProbe *int
}

// ViewCancellation is the typed in-view cooperative-cancellation wire record
// (SUPP-CANCEL, 2026-07-14). See Result.ViewCancellation for the publication
// contract.
type ViewCancellation struct {
	// View is the canonical view name of the canceled run.
	View string `json:"view"`
	// Reason is the context error class: "deadline_exceeded" | "canceled".
	Reason string `json:"reason"`
	// ScannedUnits is the cooperative sampling counter value (scan units —
	// indexed events / visit steps) at the moment the fire was observed.
	ScannedUnits int64 `json:"scanned_units,omitempty"`
	// DiscardedFaces lists the result faces (canonical view/face tokens)
	// whose construction had not completed before the fire — they were
	// discarded whole rather than published as partial aggregates.
	DiscardedFaces []string `json:"discarded_faces,omitempty"`
}

type Result struct {
	View                        string                      `json:"view"`
	SourcePath                  string                      `json:"source_path"`
	TraceArtifacts              []TraceArtifactSource       `json:"trace_artifacts,omitempty"`
	TraceFlavor                 string                      `json:"trace_flavor,omitempty"`
	Platform                    string                      `json:"platform,omitempty"`
	PlatformCandidate           string                      `json:"platform_candidate,omitempty"`
	PlatformCandidateConfidence float64                     `json:"platform_candidate_confidence,omitempty"`
	PlatformCandidateSignals    []string                    `json:"platform_candidate_signals,omitempty"`
	FlavorConfidence            float64                     `json:"trace_flavor_confidence,omitempty"`
	FlavorSignals               []string                    `json:"trace_flavor_signals,omitempty"`
	FrameworkMode               string                      `json:"framework_mode,omitempty"`
	FrameworkSurfaces           []FrameworkSurface          `json:"framework_surfaces,omitempty"`
	TimeUnit                    string                      `json:"time_unit,omitempty"`
	PrioritySemantics           string                      `json:"priority_semantics,omitempty"`
	LineCount                   int                         `json:"line_count,omitempty"`
	ScannedLineCount            int                         `json:"scanned_line_count,omitempty"`
	IndexWindowed               bool                        `json:"index_windowed,omitempty"`
	IndexTimeStart              float64                     `json:"index_time_start,omitempty"`
	IndexTimeEnd                float64                     `json:"index_time_end,omitempty"`
	IndexLineStart              int                         `json:"index_line_start,omitempty"`
	IndexLineEnd                int                         `json:"index_line_end,omitempty"`
	EventCount                  int                         `json:"event_count,omitempty"`
	UnparsedLineCount           int                         `json:"unparsed_line_count,omitempty"`
	ParseLinePanics             int                         `json:"parse_line_panics,omitempty"`
	ClockRegressions            int                         `json:"clock_regressions,omitempty"`
	TimeStart                   float64                     `json:"time_start,omitempty"`
	TimeEnd                     float64                     `json:"time_end,omitempty"`
	TargetScope                 string                      `json:"target_scope,omitempty"`
	ThreadSelection             *ThreadSelectorResolution   `json:"thread_selection,omitempty"`
	LifecycleSuppressions       []TraceLifecycleSuppression `json:"lifecycle_suppressions,omitempty"`
	Events                      []EventView                 `json:"events,omitempty"`
	Timeline                    *TimelineResult             `json:"timeline,omitempty"`
	WindowStats                 *WindowStats                `json:"window_stats,omitempty"`
	SchedulerLatency            *SchedulerLatencyResult     `json:"scheduler_latency_stats,omitempty"`
	IPCGraph                    *IPCGraphResult             `json:"ipc_graph,omitempty"`
	WakeupChain                 *ChainResult                `json:"wakeup_chain,omitempty"`
	SpanWindows                 []TraceSpanSummary          `json:"span_windows,omitempty"`
	FramePipeline               *FramePipelineResult        `json:"frame_pipeline,omitempty"`
	FrameTimeline               *FrameTimelineResult        `json:"frame_timeline,omitempty"`
	CriticalBlocking            *CriticalBlockingResult     `json:"critical_blocking_calls,omitempty"`
	RootCauseRank               *RootCauseRankResult        `json:"root_cause_rank,omitempty"`
	FrameRootCauseBundle        *FrameRootCauseBundle       `json:"frame_root_cause_bundle,omitempty"`
	// TargetWindowStates (§29.27② 常态发布, SMR-1 修复轮 引擎件① 2026-07-13;
	// 冷读 F-0 放大器: the 40422 non-bundle run had NO four-state account, so
	// the prose「全程 s_sleep」inversion had no typed counter-face): the
	// focused thread's four-state window partition publishes on EVERY
	// target-anchored bounded-window run — not only the frame-bundle path
	// (体积小账; the bundle copy stays authoritative when both exist). nil
	// when no target/window or the timeline has no measurable intervals
	// (absence never fabricates zeros).
	TargetWindowStates *TargetWindowStateAccount `json:"target_window_states,omitempty"`
	InteractionStats   *InteractionStatsResult   `json:"interaction_stats,omitempty"`
	PerfStats          *PerfContext              `json:"perf_stats,omitempty"`
	PerfTimeline       *PerfTimelineResult       `json:"perf_timeline,omitempty"`
	WindowSweep        *WindowSweepResult        `json:"window_sweep,omitempty"`
	Recipe             *RecipeResult             `json:"recipe,omitempty"`
	// CPUFrequencyCensus is the RFC #71 (§8.2 c4) pre-truncation frequency
	// tier ladder for event_search results whose chronological display cap
	// hid matched cpu_frequency rows: distinct kHz tiers + per-tier row
	// counts + cpu set, aggregated in the SAME match pass as the display
	// rows. Additive only — nil whenever the Events face already shows every
	// matched cpu_frequency row.
	CPUFrequencyCensus *CPUFrequencyCensus `json:"cpu_frequency_census,omitempty"`
	// VsyncGeneratorCensus (SA-F2, DISPATCH-IND 批4, 2026-07-14) is the
	// event_search-side vsync/frame-pacing generator account: per-generator
	// event/wakeup counts + the authoritative period parsed from the
	// generator's own period print, aggregated over ALL matched rows in the
	// SAME match pass BEFORE the chronological display cap (matched_rows
	// caliber; the window_stats face carries the window_population twin).
	// Additive only — nil when no generator row matched.
	VsyncGeneratorCensus *VsyncGeneratorCensus `json:"vsync_generator_census,omitempty"`
	EvidencePack         []EvidenceFact        `json:"evidence_pack,omitempty"`
	// ViewCancellation (SUPP-CANCEL, 2026-07-14) is the typed in-view
	// cooperative-cancellation record: non-nil only when the run's context
	// fired at a sampling point inside this view. Faces present on this
	// Result are complete builder outputs; every face whose construction
	// had not finished before the fire was discarded whole (禁半账) and is
	// listed in DiscardedFaces. Accompanied by exactly one
	// "in_view_cancellation=true; …" caveat (禁裸丢).
	ViewCancellation *ViewCancellation `json:"view_cancellation,omitempty"`
	Caveats          []string          `json:"caveats,omitempty"`
	// Compactions are the typed truncation records for this result (E4).
	// They ride ALONGSIDE the prose compaction caveats (which stay verbatim);
	// the tool refinement layer reads these first and keeps caveat-substring
	// matching only as a fallback for paths not yet publishing typed records.
	Compactions []ViewCompaction `json:"compactions,omitempty"`
}

// TraceLifecycleSuppression is the actionable typed consequence of one exact
// task-incarnation boundary. It distinguishes global PID-keyed withdrawals
// from target-specific withdrawals and names the CPU-global lane that remains
// valid, so consumers need not parse a generic "split the window" caveat.
type TraceLifecycleSuppression struct {
	ConflictTID          int      `json:"conflict_tid"`
	Signal               string   `json:"signal,omitempty"`
	PreviousLine         int      `json:"previous_line,omitempty"`
	BoundaryLine         int      `json:"boundary_line,omitempty"`
	BoundaryTs           float64  `json:"boundary_ts,omitempty"`
	Scope                string   `json:"scope,omitempty"`
	AffectsTarget        bool     `json:"affects_target,omitempty"`
	AffectedLanes        []string `json:"affected_lanes,omitempty"`
	PreservedLanes       []string `json:"preserved_lanes,omitempty"`
	CandidateSelectors   []string `json:"candidate_selectors,omitempty"`
	SuggestedQueries     []string `json:"suggested_queries,omitempty"`
	FrameOwnershipStatus string   `json:"frame_ownership_status,omitempty"`
}

// ThreadSelectorResolution publishes the deterministic routing decision when
// a query carries a hard numeric TID and optional display-name selector.
// NameCandidates are discovery hints only: Selected remains the numeric TID
// even when the requested name resolves to another thread.
type ThreadSelectorResolution struct {
	Status         string      `json:"status"`
	RequestedPID   int         `json:"requested_pid,omitempty"`
	RequestedName  string      `json:"requested_name,omitempty"`
	Selected       ThreadRef   `json:"selected,omitempty"`
	NameMismatch   bool        `json:"name_mismatch,omitempty"`
	NameCandidates []ThreadRef `json:"name_candidates,omitempty"`
	Routing        string      `json:"routing,omitempty"`
}

type FrameworkSurface struct {
	Surface        string      `json:"surface"`
	ProcessCount   int         `json:"process_count,omitempty"`
	ExampleThreads []ThreadRef `json:"example_threads,omitempty"`
	Signals        []string    `json:"signals,omitempty"`
}

type EventView struct {
	Event
	WakeePrioSource      string  `json:"wakee_prio_source,omitempty"`
	Raw                  string  `json:"raw,omitempty"`
	SourcePath           string  `json:"source_path,omitempty"`
	LocalLine            int     `json:"local_line,omitempty"`
	TimeDomain           string  `json:"time_domain,omitempty"`
	CanonicalTimeDomain  string  `json:"canonical_time_domain,omitempty"`
	SourceTs             float64 `json:"source_ts,omitempty"`
	ClockAligned         bool    `json:"clock_aligned,omitempty"`
	RawUnavailableReason string  `json:"raw_unavailable_reason,omitempty"`
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
	Thread           ThreadRef          `json:"thread"`
	Window           TimeWindow         `json:"window"`
	HeadState        *TimelineHeadState `json:"head_state,omitempty"`
	IntegrityFailure string             `json:"integrity_failure,omitempty"`
	Intervals        []Interval         `json:"intervals"`
	Caveats          []string           `json:"caveats,omitempty"`
}

// TimelineHeadState is the typed completeness verdict for the first instant
// of a bounded scheduler timeline. status=recovered means an EOF/clock-order-
// proven pre-window checkpoint seeded the interval; observed_in_index means the
// bounded index itself contained enough history; unknown is an explicit data
// gap and must never be narrated as zero scheduler time.
type TimelineHeadState struct {
	Status        string      `json:"status"`
	BoundaryTs    float64     `json:"boundary_ts,omitempty"`
	State         ThreadState `json:"state,omitempty"`
	ActualStartTs float64     `json:"actual_start_ts,omitempty"`
	SourceLine    int         `json:"source_line,omitempty"`
	Reason        string      `json:"reason,omitempty"`
}

// SchedulerHeadCoverage reports whether scheduler-derived aggregate faces
// know the governing state for every CPU/thread they observe after an explicit
// window head.  Complete artifact scanning and complete subject coverage are
// intentionally separate signals.
type SchedulerHeadCoverage struct {
	Status     string  `json:"status"`
	BoundaryTs float64 `json:"boundary_ts,omitempty"`
	Reason     string  `json:"reason,omitempty"`
	// SubjectCensusStatus distinguishes an evaluated empty missing-subject
	// census from a census that never ran. Without this typed bit an early
	// fail-close leaves the integer/slice zero values looking like
	// "missing_cpus=0:[]" even though no such claim was made.
	SubjectCensusStatus string `json:"subject_census_status,omitempty"`
	MissingCPUCount     int    `json:"missing_cpu_count,omitempty"`
	MissingCPUs         []int  `json:"missing_cpus,omitempty"`
	MissingThreadCount  int    `json:"missing_thread_count,omitempty"`
	MissingThreadPIDs   []int  `json:"missing_thread_pids,omitempty"`
}

type TimeWindow struct {
	StartTs float64 `json:"start_ts,omitempty"`
	EndTs   float64 `json:"end_ts,omitempty"`
	// StartSet (WINFLAG-1, §29.190④, 2026-07-21) is the RESULT-side typed
	// start_set flag: true when StartTs is a DETERMINED window start — an
	// explicit query time_start (TimeStartSet, including an explicit 0 on a
	// rebased trace), a positive start value, or the normalizeQuery
	// whole-trace backfill (idx.FirstTs, which is legally 0 on a rebased
	// export). false exactly on the ambiguous unset-0 forms (line-anchored
	// queries whose TimeStart stays at the 0=unset sentinel, un-normalized
	// unbounded windows). Deliberately `json:"-"`: the flag is an in-process
	// carrier for the (a)/(b)/(c) consumers in window_start_flag.go — the
	// result-JSON wire, the tracediag reflect dump (formatInlineStruct reads
	// StartTs/EndTs only) and every persisted artifact stay byte-identical,
	// which is also the XLANE-3 board-fingerprint argument: no serialized
	// face changes, no new query knob exists, so the flag stays OUT of the
	// board-identity closed set (同板). Query-API params keep their own
	// 0=unset sentinel untouched (the flag never flows back into Query).
	StartSet bool `json:"-"`
}

// StartDetermined (WINFLAG-1) reports the window's StartTs is a real
// determined endpoint: positive, or a flagged real 0. The line-anchored
// unset form (StartTs==0 without StartSet) and negative values stay
// indeterminate — consumers must treat them as absence, never guess
// (宁漏勿假指).
func (w TimeWindow) StartDetermined() bool {
	return w.StartTs > 0 || (w.StartTs == 0 && w.StartSet)
}

// StartsAtDeterminedZero (WINFLAG-1) is the engine-fold gate: true exactly
// when this result window's start is a DETERMINED real 0 (explicit
// time_start=0, or the whole-trace backfill on a rebased FirstTs==0 trace).
// Only under this gate may a fold admit a member StartTs==0 as a real
// timestamp instead of the zero-value absence sentinel.
func (w TimeWindow) StartsAtDeterminedZero() bool {
	return w.StartSet && w.StartTs == 0
}

type WindowStats struct {
	Window                TimeWindow             `json:"window"`
	EventCounts           map[EventType]int      `json:"event_counts,omitempty"`
	SchedulerHeadCoverage *SchedulerHeadCoverage `json:"scheduler_head_coverage,omitempty"`
	CPU                   []CPUStats             `json:"cpu,omitempty"`
	CoreTopology          []CoreClassStats       `json:"core_topology,omitempty"`
	// ClusterFrequencyCeilings is the CFC (§7.10 VS-2c 设计) single-point
	// per-cluster fmax snapshot for this window (VS-2b ladder per cluster:
	// window-governing limits Max > highest governed observed sample),
	// computed once so the scattered fmax consumers share one source.
	// INTERNAL computation structure, not an observation face: json:"-"
	// keeps it off every JSON surface and out of the causal token registry
	// (CFC ruling: no new token). Sole display consumer: the soft
	// window_stats stanza line (writeTraceClusterFrequencyCeilings,
	// internal/tool/trace_query.go).
	ClusterFrequencyCeilings []ClusterFrequencyCeiling `json:"-"`
	TopRunning               []ThreadDuration          `json:"top_running,omitempty"`
	RunnableTop              []ThreadDuration          `json:"runnable_top,omitempty"`
	// RunnableCPUContinuity separates the CPU-independent runnable wall-clock
	// account from narrower CPU-specific attribution. Mismatch/open segments
	// remain in RunnableTop on the CPU-unknown sentinel and are excluded from
	// pressure/frequency. Customer/model text renders the sentinel as unknown.
	RunnableCPUContinuity *RunnableCPUContinuitySummary `json:"runnable_cpu_continuity,omitempty"`
	SleepTop              []ThreadDuration              `json:"sleep_top,omitempty"`
	// DStateTop and IOWaitTop are mutually exclusive scheduler-state ledgers:
	// an interval refined by sched_blocked_reason iowait=1 appears only in
	// IOWaitTop; DStateTop contains the remaining non-IO uninterruptible waits.
	// Their same-thread sum is the complete D/IO blocking account.
	DStateTop []ThreadDuration `json:"d_state_top,omitempty"`
	IOWaitTop []ThreadDuration `json:"io_wait_top,omitempty"`
	// DStateTopOverflow*/IOWaitTopOverflow* (修复轮二 件A, 2026-07-13): the
	// per-lane cap-overflow disclosure — how many (thread,cpu) census groups
	// sit beyond the top-8 display list and their summed account. Disclosure
	// only (the family seats already carry the full account); zero = nothing
	// evicted.
	DStateTopOverflowGroups int     `json:"d_state_top_overflow_groups,omitempty"`
	DStateTopOverflowMs     float64 `json:"d_state_top_overflow_ms,omitempty"`
	IOWaitTopOverflowGroups int     `json:"io_wait_top_overflow_groups,omitempty"`
	IOWaitTopOverflowMs     float64 `json:"io_wait_top_overflow_ms,omitempty"`
	// RunnableTopOverflow* (RSPA §29.61.10, 2026-07-14): the runnable lane's
	// cap-overflow disclosure — 件A 的同族补完 (the fourth 「帽基当全量」
	// instance): how many (thread,cpu) runnable census groups sit beyond the
	// top-8 display list and their summed account. Disclosure only.
	RunnableTopOverflowGroups int                      `json:"runnable_top_overflow_groups,omitempty"`
	RunnableTopOverflowMs     float64                  `json:"runnable_top_overflow_ms,omitempty"`
	CPUPressure               []CPUPressureStats       `json:"cpu_pressure,omitempty"`
	CPUConstraints            []CPUConstraintSummary   `json:"cpu_constraints,omitempty"`
	ThreadCPULoad             []ThreadCPULoadSummary   `json:"thread_cpu_load,omitempty"`
	ProcessCPULoad            []ProcessCPULoadSummary  `json:"process_cpu_load,omitempty"`
	RunnableContext           []RunnableContextSummary `json:"runnable_context,omitempty"`
	IOLatencies               []IOLatencySummary       `json:"io_latencies,omitempty"`
	CPUFrequencyLimits        []CPUFrequencyLimit      `json:"cpu_frequency_limits,omitempty"`
	SubsystemEvents           []SubsystemEventSummary  `json:"subsystem_events,omitempty"`
	BlockIssueCount           int                      `json:"block_issue_count,omitempty"`
	BlockRemapCount           int                      `json:"block_remap_count,omitempty"`
	BlockCompleteCount        int                      `json:"block_complete_count,omitempty"`
	BinderCount               int                      `json:"binder_count,omitempty"`
	BinderReceivedCount       int                      `json:"binder_received_count,omitempty"`
	BinderAuxCount            int                      `json:"binder_aux_count,omitempty"`
	IRQCount                  int                      `json:"irq_count,omitempty"`
	SoftIRQCount              int                      `json:"softirq_count,omitempty"`
	MemoryEventCount          int                      `json:"memory_event_count,omitempty"`
	StorageEventCount         int                      `json:"storage_event_count,omitempty"`
	FilesystemEventCount      int                      `json:"filesystem_event_count,omitempty"`
	PowerEventCount           int                      `json:"power_event_count,omitempty"`
	AbilityEventCount         int                      `json:"ability_event_count,omitempty"`
	XPowerEventCount          int                      `json:"xpower_event_count,omitempty"`
	HiSystemEventCount        int                      `json:"hi_sysevent_event_count,omitempty"`
	WorkqueueEventCount       int                      `json:"workqueue_event_count,omitempty"`
	DMAFenceEventCount        int                      `json:"dma_fence_event_count,omitempty"`
	BlockedReasonCount        int                      `json:"blocked_reason_count,omitempty"`
	SchedStatCount            int                      `json:"sched_stat_count,omitempty"`
	IPICount                  int                      `json:"ipi_count,omitempty"`
	IOWaitBlockedCount        int                      `json:"io_wait_blocked_count,omitempty"`
	BlockedReasons            []BlockedReasonSummary   `json:"blocked_reasons,omitempty"`
	// BlockedReasonCensus (件1 census 根修, 2026-07-13): the per-pid FULL
	// blocked_reason census on the wire face — per-caller 符号×count×Σms off
	// the full pre-truncation accumulator (never the top-8 display view),
	// bounded per-pid with an explicit caller-overflow count. The typed
	// observation lane and the model evidence feed consume THIS; the top-8
	// BlockedReasons view above stays a display face.
	BlockedReasonCensus []BlockedReasonPIDCensus `json:"blocked_reason_census,omitempty"`
	// BlockedReasonCensusOverflow is the number of pids beyond the census
	// pid cap (their records stay counted in BlockedReasonCount).
	BlockedReasonCensusOverflow int `json:"blocked_reason_census_overflow,omitempty"`
	// blockedReasonFullByPID (CR-3 修复轮 P2, 2026-07-12): the per-pid FULL
	// blocked_reason accumulator folded BEFORE the top-8 truncation (INODE
	// §28.6 precedent — never second-aggregate on truncated inputs; 冷读
	// 实锤: the ➎ residual said 17 while the window held 19, two buckets
	// having fallen off the top-8 inventory). Engine-internal computation
	// structure (unexported = off every wire/JSON face); sole consumer is
	// the D-family rank residual mint (CR-3 件② P10).
	blockedReasonFullByPID map[int]blockedReasonPIDTotal
	// dstateCensus/iowaitCensus (修复轮二 件A, 2026-07-13). Unexported:
	// in-package verdict input, never serialized — the FULL pre-cap
	// per-(thread,cpu) D/IO ledgers behind the capped top lists
	// (runnableCensus/ENG-1 precedent). The formal family seats mint from
	// THESE (全量账铸席); nil on legacy/direct-literal WindowStats, where the
	// mint fails open to the capped lists verbatim.
	dstateCensus map[string]ThreadDuration
	iowaitCensus map[string]ThreadDuration
	// runnableCensus (RSPA §29.61.10a/b/c, 2026-07-14): the FULL pre-cap
	// runnable ledger behind the capped RunnableTop display list — the
	// 「帽基当全量」 fix's fourth instance (D/IO 件A precedent verbatim: the
	// formal runnable seats mint from THIS 全量账; nil on legacy/direct-
	// literal WindowStats, where the mint fails open to the capped list).
	runnableCensus map[string]ThreadDuration
	// runnableSegments is the single continuity-judged interval ledger shared
	// by WindowStats and SchedulerLatency. It prevents two scheduler state
	// machines from assigning different CPUs to the same runnable wall time.
	runnableSegments []runnableWaitSegment
	// runnableContextCensus is the uncapped scheduler-latency context ledger.
	// RunnableContext above is a bounded display view; priority-inversion and
	// same-CPU correctness consumers must not lose a verified on-chain segment
	// merely because eight larger intervals occupied that public view.
	runnableContextCensus []RunnableContextSummary
	// cpuPressureCensus is the uncapped per-CPU accounting ledger. The public
	// CPUPressure slice is a display top-N only; arithmetic, CAP, state churn,
	// and same-CPU lookup must never treat that display cap as full coverage.
	cpuPressureCensus []CPUPressureStats
	// offCPUProducerDisjoint (件6 修复轮, 2026-07-14). Unexported: the
	// ordered-stream premise of the off-CPU state machine (ORD 复核 P3-1
	// same gate the mint sites read) — false on clock-regressed indexes. The
	// RSPA family decisions gate on it so a regressed trace can never
	// suppress a chain D-IO seat while the MAX-fallback window seats stayed
	// unclipped (板面失链值行).
	offCPUProducerDisjoint bool
	// chainAnchorsByPID (RSPA §29.61.10a/b/c, 2026-07-14). Unexported: the
	// typed wakeup-dependency jump-window unions (per chain pid, target
	// excluded — self-causality is fully anchored by definition) this stats
	// sweep accumulated anchoredMs against. nil == the sweep ran WITHOUT
	// anchor data — the seat re-anchoring pass fails open (no migration)
	// because anchored==0 would be indistinguishable from "never measured".
	chainAnchorsByPID map[int][]TimeWindow
	// anchoredDIOWakeups (RSPA M-IO closure lane): the wakeup-closed anchored
	// D/IO segment ends of chain threads — (waker pid, wakeup ts) pairs
	// recorded when a sched_wakeup terminated a D/IO segment whose anchored
	// overlap is positive. The per-IO completion-closure check reads these
	// (io_latency completion thread + ts window); bounded by
	// anchoredDIOWakeupCap, and the overflow direction is honest: a dropped
	// record can only DEMOTE an io row to ◇ (宁漏勿猜), never promote.
	anchoredDIOWakeups []anchoredDIOWakeupRecord
	// traceSpanFullInventory (SPANVIS-1, 2026-07-19). Unexported: the FULL
	// pre-bound trace-mark span inventory behind the bounded TraceSpans
	// display view (dstateCensus/runnableCensus precedent — engine-internal
	// computation structure, never serialized). Sole consumer is the advisory
	// business-span mention face (computeBusinessSpanMentions); the seat /
	// carve machinery (blocking carve, semantic families, generic trace_span
	// rank rows) keeps consuming the bounded TraceSpans view byte-identically
	// (§29.137 LOCKSPAN 调查 / §29.143 备案). nil on legacy/direct-literal
	// WindowStats and on every trace-mark fail-closed path, where the mention
	// face fails open to absence.
	traceSpanFullInventory []TraceSpanSummary
	TraceSpans             []TraceSpanSummary `json:"trace_spans,omitempty"`
	// TraceTrackSpans is the isolated Android ASYNC_FOR_TRACK G/H lane. These
	// spans have logical track ownership, not emitter-thread ownership, and
	// therefore never feed TraceSpans, semantic classification or root rank.
	TraceTrackSpans []TraceTrackSpanSummary `json:"trace_track_spans,omitempty"`
	// TraceInstants contains Android I/N point markers. They are inventory
	// observations only and never mint elapsed duration.
	TraceInstants       []TraceInstantSummary       `json:"trace_instants,omitempty"`
	TraceCounters       []TraceCounterSummary       `json:"trace_counters,omitempty"`
	CounterDeltas       []TraceCounterDeltaSummary  `json:"counter_deltas,omitempty"`
	CounterQuality      *TraceCounterQualitySummary `json:"counter_quality,omitempty"`
	IRQBursts           []IRQBurstSummary           `json:"irq_bursts,omitempty"`
	MemoryKinds         []MemoryKindSummary         `json:"memory_kinds,omitempty"`
	BIOResources        []RuntimeResourceSummary    `json:"bio_resources,omitempty"`
	FilesystemResources []RuntimeResourceSummary    `json:"filesystem_resources,omitempty"`
	PageFaultResources  []RuntimeResourceSummary    `json:"page_fault_resources,omitempty"`
	FileIOByInode       []FileIOSummary             `json:"file_io_by_inode,omitempty"`
	PageCacheByInode    []PageCacheSummary          `json:"page_cache_by_inode,omitempty"`
	// TopIOInodes is the INODE (§28.6, 2026-07-09) whole-window (dev,inode)
	// IO frequency carrier: folded from the FULL pre-truncation fileIO /
	// pageCache accumulator maps (never from the truncated top-8 slices
	// above), PID/op key dimensions collapsed, ordered Count → Bytes →
	// MaxLatency with TotalGroups truncation disclosure. Latency follows the
	// wall-clock red line: max single event + per-thread within-thread sums,
	// never a cross-thread latency sum. Nil when the window has no IO-family
	// evidence.
	TopIOInodes           *TopIOInodeStats        `json:"top_io_inodes,omitempty"`
	StorageLatencyByLayer []StorageLatencySummary `json:"storage_latency_by_layer,omitempty"`
	IOPressureSummary     *IOPressureSummary      `json:"io_pressure_summary,omitempty"`
	IOBurstEpisodes       []IOBurstEpisodeSummary `json:"io_burst_episodes,omitempty"`
	BlockIOByInode        []BlockIOByInodeSummary `json:"block_io_by_inode,omitempty"`
	IRQActivity           []InterruptActivity     `json:"irq_activity,omitempty"`
	SoftIRQActivity       []InterruptActivity     `json:"softirq_activity,omitempty"`
	IPIActivity           []InterruptActivity     `json:"ipi_activity,omitempty"`
	WorkqueueActivity     []WorkqueueActivity     `json:"workqueue_activity,omitempty"`
	DMAFenceActivity      []DMAFenceActivity      `json:"dma_fence_activity,omitempty"`
	SchedStatAccounting   []SchedStatSummary      `json:"sched_stat_accounting,omitempty"`
	SupplyPressureSummary *SupplyPressureSummary  `json:"supply_pressure_summary,omitempty"`
	TraceMarkCategories   []TraceMarkCategory     `json:"trace_mark_categories,omitempty"`
	AsyncFileWork         []AsyncFileWorkSummary  `json:"async_file_work,omitempty"`
	AbilityEvents         []TracePluginSummary    `json:"ability_events,omitempty"`
	XPowerEvents          []TracePluginSummary    `json:"xpower_events,omitempty"`
	HiSystemEvents        []TracePluginSummary    `json:"hi_sysevent_events,omitempty"`
	ThreadDrifts          []ThreadDriftSummary    `json:"thread_drifts,omitempty"`
	ComputeSupply         []ComputeSupplySummary  `json:"compute_supply,omitempty"`
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
	// VsyncGeneratorCensus (SA-F2, DISPATCH-IND 批4, 2026-07-14) is the
	// window_population-caliber vsync/frame-pacing generator account:
	// per-generator event/wakeup counts + the authoritative period parsed
	// from the generator's own period print, aggregated over EVERY event in
	// the selected window (population-wide, same admission as the other
	// window faces — no pid predicate). Additive only — nil when no
	// generator thread was sighted in the window.
	VsyncGeneratorCensus *VsyncGeneratorCensus `json:"vsync_generator_census,omitempty"`
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
	SampleCount int   `json:"sample_count,omitempty"`
	TotalPeriod int64 `json:"total_period,omitempty"`
	// Cohorts are independent event/unit weight domains. TotalPeriod and the
	// legacy Top* fields below are populated only when exactly one cohort has
	// an exact aggregate, so consumers cannot compare cycles, instructions,
	// nanoseconds, and event counts through one denominator.
	CohortCount         int          `json:"cohort_count,omitempty"`
	Cohorts             []PerfCohort `json:"cohorts,omitempty"`
	ThreadIdentityCount int          `json:"thread_identity_count,omitempty"`
	// ThreadIdentityCountExact is pointer-typed so an absent field from an
	// older producer cannot be mistaken for a new producer's proof that every
	// sample had a typed (TID,generation) identity.
	ThreadIdentityCountExact         *bool               `json:"thread_identity_count_exact,omitempty"`
	ThreadIdentityUnknownSampleCount int                 `json:"thread_identity_unknown_sample_count,omitempty"`
	Quality                          *PerfQualitySummary `json:"quality,omitempty"`
	TopSymbols                       []PerfHotspot       `json:"top_symbols,omitempty"`
	TopDSO                           []PerfHotspot       `json:"top_dso,omitempty"`
	TopCallchains                    []PerfHotspot       `json:"top_callchains,omitempty"`
	TopThreads                       []PerfThreadSummary `json:"top_threads,omitempty"`
	TopEvents                        []PerfHotspot       `json:"top_events,omitempty"`
	Caveats                          []string            `json:"caveats,omitempty"`
}

// PerfCohort is one exact perf event/unit aggregation domain. WeightStatus is
// "exact" or "aggregate_overflow". An overflow cohort retains sample and
// identity inventory but intentionally publishes no weighted Top-N.
type PerfCohort struct {
	Event                            string              `json:"event"`
	WeightUnit                       string              `json:"weight_unit"`
	WeightStatus                     string              `json:"weight_status"`
	SampleCount                      int                 `json:"sample_count,omitempty"`
	TotalPeriod                      int64               `json:"total_period,omitempty"`
	ThreadIdentityCount              int                 `json:"thread_identity_count,omitempty"`
	ThreadIdentityCountExact         *bool               `json:"thread_identity_count_exact,omitempty"`
	ThreadIdentityUnknownSampleCount int                 `json:"thread_identity_unknown_sample_count,omitempty"`
	Quality                          *PerfQualitySummary `json:"quality,omitempty"`
	TopSymbols                       []PerfHotspot       `json:"top_symbols,omitempty"`
	TopDSO                           []PerfHotspot       `json:"top_dso,omitempty"`
	TopCallchains                    []PerfHotspot       `json:"top_callchains,omitempty"`
	TopThreads                       []PerfThreadSummary `json:"top_threads,omitempty"`
	TopEvents                        []PerfHotspot       `json:"top_events,omitempty"`
}

type PerfHotspot struct {
	Symbol                           string               `json:"symbol,omitempty"`
	DSO                              string               `json:"dso,omitempty"`
	Callchain                        string               `json:"callchain,omitempty"`
	Event                            string               `json:"event,omitempty"`
	WeightUnit                       string               `json:"weight_unit,omitempty"`
	Source                           string               `json:"source,omitempty"`
	SymbolizationStatus              string               `json:"symbolization_status,omitempty"`
	SampleCount                      int                  `json:"sample_count,omitempty"`
	Period                           int64                `json:"period,omitempty"`
	Percent                          float64              `json:"percent,omitempty"`
	ThreadIdentityCount              int                  `json:"thread_identity_count,omitempty"`
	ThreadIdentityCountExact         *bool                `json:"thread_identity_count_exact,omitempty"`
	ThreadIdentityUnknownSampleCount int                  `json:"thread_identity_unknown_sample_count,omitempty"`
	ThreadIdentities                 []PerfThreadIdentity `json:"thread_identities,omitempty"`
	Threads                          []ThreadRef          `json:"threads,omitempty"`
	CPUs                             []int                `json:"cpus,omitempty"`
	LineStart                        int                  `json:"line_start,omitempty"`
	LineEnd                          int                  `json:"line_end,omitempty"`
	Example                          string               `json:"example,omitempty"`
}

type PerfQualitySummary struct {
	WeightStatus          string           `json:"weight_status,omitempty"`
	Sources               []PerfValueCount `json:"sources,omitempty"`
	InputIntegrityIssues  []PerfValueCount `json:"input_integrity_issues,omitempty"`
	ParserCaveats         []PerfValueCount `json:"parser_caveats,omitempty"`
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
	Identity    *PerfThreadIdentity `json:"identity,omitempty"`
	Thread      ThreadRef           `json:"thread,omitempty"`
	SampleCount int                 `json:"sample_count,omitempty"`
	Period      int64               `json:"period,omitempty"`
	Percent     float64             `json:"percent,omitempty"`
	CPUs        []int               `json:"cpus,omitempty"`
	LineStart   int                 `json:"line_start,omitempty"`
	LineEnd     int                 `json:"line_end,omitempty"`
	Example     string              `json:"example,omitempty"`
}

type PerfTimelineResult struct {
	Window   TimeWindow           `json:"window"`
	BucketMs float64              `json:"bucket_ms,omitempty"`
	Buckets  []PerfTimelineBucket `json:"buckets,omitempty"`
	Caveats  []string             `json:"caveats,omitempty"`
}

type PerfTimelineBucket struct {
	StartTs                          float64              `json:"start_ts,omitempty"`
	EndTs                            float64              `json:"end_ts,omitempty"`
	SampleCount                      int                  `json:"sample_count,omitempty"`
	Period                           int64                `json:"period,omitempty"`
	TopSymbol                        string               `json:"top_symbol,omitempty"`
	TopDSO                           string               `json:"top_dso,omitempty"`
	TopEvent                         string               `json:"top_event,omitempty"`
	ThreadIdentityCount              int                  `json:"thread_identity_count,omitempty"`
	ThreadIdentityCountExact         *bool                `json:"thread_identity_count_exact,omitempty"`
	ThreadIdentityUnknownSampleCount int                  `json:"thread_identity_unknown_sample_count,omitempty"`
	ThreadIdentities                 []PerfThreadIdentity `json:"thread_identities,omitempty"`
	Threads                          []ThreadRef          `json:"threads,omitempty"`
	CPUs                             []int                `json:"cpus,omitempty"`
	LineStart                        int                  `json:"line_start,omitempty"`
	LineEnd                          int                  `json:"line_end,omitempty"`
	Example                          string               `json:"example,omitempty"`
	CohortCount                      int                  `json:"cohort_count,omitempty"`
	Cohorts                          []PerfTimelineCohort `json:"cohorts,omitempty"`
}

// PerfTimelineCohort is the per-time-bucket counterpart of PerfCohort. The
// legacy bucket Period/Top* fields are compatibility mirrors for a single
// exact cohort only.
type PerfTimelineCohort struct {
	Event        string `json:"event"`
	WeightUnit   string `json:"weight_unit"`
	WeightStatus string `json:"weight_status"`
	SampleCount  int    `json:"sample_count,omitempty"`
	Period       int64  `json:"period,omitempty"`
	TopSymbol    string `json:"top_symbol,omitempty"`
	TopDSO       string `json:"top_dso,omitempty"`
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
	// itemsCensus retains the sorted pre-compaction interval inventory for
	// in-package correctness consumers. Items remains the bounded public face.
	itemsCensus []SchedulerLatencyItem
}

type SchedulerLatencyItem struct {
	Thread        ThreadRef `json:"thread"`
	StartTs       float64   `json:"start_ts,omitempty"`
	EndTs         float64   `json:"end_ts,omitempty"`
	DurationMs    float64   `json:"duration_ms,omitempty"`
	CPU           int       `json:"cpu"`
	CPUContinuity string    `json:"cpu_continuity,omitempty"`
	CoreClass     string    `json:"core_class,omitempty"`
	// Frequency is the legacy single cpu_frequency sample at the wait start,
	// kept for context only. Low-frequency judgements use WeightedFrequency /
	// ObservedMaxFrequency (methodology audit §7.30.2 R5e).
	Frequency     int64  `json:"frequency,omitempty"`
	Priority      int    `json:"priority,omitempty"`
	PriorityClass string `json:"priority_class,omitempty"`
	StartLine     int    `json:"start_line,omitempty"`
	EndLine       int    `json:"end_line,omitempty"`
	// WeightedFrequency is the duration-weighted CPU frequency (kHz) across
	// this wait interval, integrated over cpu_frequency change points
	// (§7.30.2 R5e). Zero when the CPU has no frequency samples at all.
	WeightedFrequency int64 `json:"weighted_frequency,omitempty"`
	// ObservedMaxFrequency is the max cpu_frequency sample observed inside or
	// nearest to this interval — the low-frequency benchmark (§7.30.2 R5e:
	// never the window-wide residency max).
	ObservedMaxFrequency int64 `json:"observed_max_frequency,omitempty"`
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
	// SystemOrKernelRunningMs is the full-window running total whose Harmony
	// priority token is outside the documented user-space bands (>159). It is
	// raw competition context, never part of HighPriorityRunningMs.
	SystemOrKernelRunningMs float64 `json:"system_or_kernel_running_ms,omitempty"`
	// SystemOrKernelRunningOverlapMs and SystemOrKernelCompetitorCount are the
	// raw/system running share and distinct competitor count that overlapped
	// THIS runnable wait. They are disclosed separately and never drive a
	// high-priority or priority-inversion verdict.
	SystemOrKernelRunningOverlapMs float64 `json:"system_or_kernel_running_overlap_ms,omitempty"`
	SystemOrKernelCompetitorCount  int     `json:"system_or_kernel_competitor_count,omitempty"`
	// SameCPUTopRunning lists only threads whose running time overlapped this
	// wait interval; DurationMs is the overlapped portion, not the window
	// running total. Serial hand-offs (zero overlap) are excluded (§7.30.2
	// R5g).
	SameCPUTopRunning []ThreadDuration `json:"same_cpu_top_running,omitempty"`
	// sameCPURunningSegments is the uncapped, per-contiguous-overlap
	// correctness inventory. SameCPUTopRunning remains the bounded,
	// thread-aggregated public view.
	sameCPURunningSegments []ThreadDuration
	Summary                string `json:"summary,omitempty"`
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
	Frequency int64 `json:"frequency,omitempty"`
	// WeightedFrequency is the duration-weighted CPU frequency (kHz) across
	// the judged running/runnable segments, integrated over cpu_frequency
	// change points (§7.30.2 R5e).
	WeightedFrequency int64 `json:"weighted_frequency,omitempty"`
	// ObservedMaxFrequency is the max cpu_frequency sample observed inside or
	// nearest to the judged segments — the 0.65× low-frequency benchmark
	// (§7.30.2 R5e: never the window-wide residency max).
	ObservedMaxFrequency int64 `json:"observed_max_frequency,omitempty"`
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
	// Raw Harmony >159 scheduler tokens stay in a separate accounting bucket;
	// they are observable system/kernel competition, not ohos_rt evidence.
	SystemOrKernelRunningMs        float64 `json:"system_or_kernel_running_ms,omitempty"`
	SystemOrKernelRunningOverlapMs float64 `json:"system_or_kernel_running_overlap_ms,omitempty"`
	SystemOrKernelCompetitorCount  int     `json:"system_or_kernel_competitor_count,omitempty"`
	Verdict                        string  `json:"verdict,omitempty"`
	Confidence                     float64 `json:"confidence,omitempty"`
	LineStart                      int     `json:"line_start,omitempty"`
	LineEnd                        int     `json:"line_end,omitempty"`
	Summary                        string  `json:"summary,omitempty"`
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
	MaxFrequencyKHz int64 `json:"max_frequency_khz,omitempty"`
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
	// IOWaitKnown distinguishes a proven iowait=0 from a malformed/unknown
	// declaration. Caller and delay remain independently usable in both cases.
	IOWaitKnown bool    `json:"io_wait_known,omitempty"`
	Reason      string  `json:"reason,omitempty"`
	Count       int     `json:"count,omitempty"`
	Line        int     `json:"line,omitempty"`
	Ts          float64 `json:"ts,omitempty"`
	// DelayTotal / DelayCount (件1 census 根修, 2026-07-13): Σ of the rows'
	// RAW vendor delay fields (µs on HarmonyOS) and how many rows carried
	// one — the census publishes Σms only when every row did (宁缺勿假).
	DelayTotal int64 `json:"delay_total,omitempty"`
	DelayCount int   `json:"delay_count,omitempty"`
}

// BlockedReasonCensusCaller is ONE caller symbol's full-window account for
// one thread (件1 census 根修, 2026-07-13): 符号×count×Σms off the FULL
// pre-truncation accumulator (INODE §28.6 discipline). DelayTotalMs is
// published only when EVERY row of this caller carried a vendor delay field
// (µs→ms); partial delay coverage keeps the count and omits the Σ.
type BlockedReasonCensusCaller struct {
	Caller       string  `json:"caller"`
	Count        int     `json:"count"`
	DelayTotalMs float64 `json:"delay_total_ms,omitempty"`
}

// BlockedReasonPIDCensus is one thread's full-window blocked_reason census:
// the kernel's own record of what THIS pid was waiting on (keyed by the
// row's pid field, never by the trace line a record happens to print on).
type BlockedReasonPIDCensus struct {
	Thread ThreadRef `json:"thread"`
	// Count is the pid's TOTAL in-window blocked_reason record count (the
	// per-caller counts below sum to it when CallerOverflow is 0).
	Count   int                         `json:"count"`
	Callers []BlockedReasonCensusCaller `json:"callers,omitempty"`
	// CallerOverflow is the number of DISTINCT caller symbols beyond the
	// per-pid caller cap (their record counts stay inside Count).
	CallerOverflow int `json:"caller_overflow,omitempty"`
}

const (
	CPUBusyIdleStatusMeasured    = "measured"
	CPUBusyIdleStatusPartial     = "partial"
	CPUBusyIdleStatusUnavailable = "unavailable"
)

type CPUStats struct {
	CPU       int     `json:"cpu"`
	CoreClass string  `json:"core_class,omitempty"`
	BusyMs    float64 `json:"busy_ms,omitempty"`
	IdleMs    float64 `json:"idle_ms,omitempty"`
	// BusyIdleStatus is the authority for BusyMs/IdleMs. A frequency-only CPU
	// row carries unavailable rather than relying on Go's indistinguishable
	// numeric zero value. Partial means a measured suffix exists but the
	// window-head CPU state was not fully recovered.
	BusyIdleStatus     string                  `json:"busy_idle_status,omitempty"`
	BusyIdleReason     string                  `json:"busy_idle_reason,omitempty"`
	Frequency          int64                   `json:"frequency,omitempty"`
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
	// AllowedMaxTierKHz / GlobalMaxTierKHz (R5a §29.88.4 + §29.88.7 场景②,
	// 2026-07-15; 判据按核档 — §29.88.8 B锚点: core CLASS cannot express the
	// donghu mask=ffb exclusion, the frequency TIER can): minted as a PAIR
	// exactly when the binding provably excludes a bigger core tier — every
	// allowed CPU's R6-derived cluster fmax is known and their maximum sits
	// strictly below the trace-global maximum tier. Zero pair = no exclusion
	// claim (binding includes the top tier, no bigger tier exists, or a tier
	// is unresolvable — 禁无中生有, the mention only renders on proof).
	AllowedMaxTierKHz int64   `json:"allowed_max_tier_khz,omitempty"`
	GlobalMaxTierKHz  int64   `json:"global_max_tier_khz,omitempty"`
	ObservedCPU       int     `json:"observed_cpu,omitempty"`
	ObservedCPUKnown  bool    `json:"-"`
	ObservedCoreClass string  `json:"observed_core_class,omitempty"`
	MigrationCount    int     `json:"migration_count,omitempty"`
	ConstraintCount   int     `json:"constraint_count,omitempty"`
	RunnableWaitMs    float64 `json:"runnable_wait_ms,omitempty"`
	OtherCPUIdleMs    float64 `json:"other_cpu_idle_ms,omitempty"`
	StartTs           float64 `json:"start_ts,omitempty"`
	EndTs             float64 `json:"end_ts,omitempty"`
	LineStart         int     `json:"line_start,omitempty"`
	LineEnd           int     `json:"line_end,omitempty"`
	Summary           string  `json:"summary,omitempty"`
}

type ThreadCPULoadSummary struct {
	Thread                  ThreadRef `json:"thread"`
	RunningMs               float64   `json:"running_ms,omitempty"`
	RunnableWaitMs          float64   `json:"runnable_wait_ms,omitempty"`
	HighPriorityRunningMs   float64   `json:"high_priority_running_ms,omitempty"`
	SystemOrKernelRunningMs float64   `json:"system_or_kernel_running_ms,omitempty"`
	CPU                     int       `json:"cpu"`
	CoreClass               string    `json:"core_class,omitempty"`
	Frequency               int64     `json:"frequency,omitempty"`
	Priority                int       `json:"priority,omitempty"`
	PriorityClass           string    `json:"priority_class,omitempty"`
	LineStart               int       `json:"line_start,omitempty"`
	LineEnd                 int       `json:"line_end,omitempty"`
	Summary                 string    `json:"summary,omitempty"`
}

type ProcessCPULoadSummary struct {
	Process                 ThreadRef `json:"process"`
	ThreadCount             int       `json:"thread_count,omitempty"`
	RunningMs               float64   `json:"running_ms,omitempty"`
	RunnableWaitMs          float64   `json:"runnable_wait_ms,omitempty"`
	HighPriorityRunningMs   float64   `json:"high_priority_running_ms,omitempty"`
	SystemOrKernelRunningMs float64   `json:"system_or_kernel_running_ms,omitempty"`
	TopThread               ThreadRef `json:"top_thread,omitempty"`
	TopThreadMs             float64   `json:"top_thread_ms,omitempty"`
	CPUs                    []int     `json:"cpus,omitempty"`
	CoreClasses             []string  `json:"core_classes,omitempty"`
	LineStart               int       `json:"line_start,omitempty"`
	LineEnd                 int       `json:"line_end,omitempty"`
	Summary                 string    `json:"summary,omitempty"`
}

type RunnableContextSummary struct {
	Thread         ThreadRef `json:"thread"`
	RunnableWaitMs float64   `json:"runnable_wait_ms,omitempty"`
	CPU            int       `json:"cpu"`
	CoreClass      string    `json:"core_class,omitempty"`
	Frequency      int64     `json:"frequency,omitempty"`
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
	HighPriorityRunningOverlapMs   float64 `json:"high_priority_running_overlap_ms,omitempty"`
	SystemOrKernelRunningMs        float64 `json:"system_or_kernel_running_ms,omitempty"`
	SystemOrKernelRunningOverlapMs float64 `json:"system_or_kernel_running_overlap_ms,omitempty"`
	SystemOrKernelCompetitorCount  int     `json:"system_or_kernel_competitor_count,omitempty"`
	// SameCPUTopRunning lists only threads whose running overlapped this
	// thread's runnable wait; DurationMs is the overlapped portion (§7.30.2
	// R5g).
	SameCPUTopRunning []ThreadDuration `json:"same_cpu_top_running,omitempty"`
	// sameCPURunningSegments is the full exact overlap inventory behind the
	// thread-aggregated display roster above. Each entry retains the priority
	// in force for that contiguous segment, so a later priority change cannot
	// retroactively classify the competitor's whole aggregate.
	sameCPURunningSegments []ThreadDuration
	TopBackgroundThreads   []ThreadCPULoadSummary `json:"top_background_threads,omitempty"`
	SameProcessLoad        *ProcessCPULoadSummary `json:"same_process_load,omitempty"`
	TopBackgroundProcess   *ProcessCPULoadSummary `json:"top_background_process,omitempty"`
	CPUConstraint          *CPUConstraintSummary  `json:"cpu_constraint,omitempty"`
	Verdict                string                 `json:"verdict,omitempty"`
	Confidence             float64                `json:"confidence,omitempty"`
	LineStart              int                    `json:"line_start,omitempty"`
	LineEnd                int                    `json:"line_end,omitempty"`
	Summary                string                 `json:"summary,omitempty"`
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
	// Harmony priorities >159 are raw system/kernel scheduler tokens. Their
	// window total, displacement-overlap duration and distinct overlapped
	// competitor count stay observable here without entering the RT bucket.
	SystemOrKernelRunningMs        float64 `json:"system_or_kernel_running_ms,omitempty"`
	SystemOrKernelRunningOverlapMs float64 `json:"system_or_kernel_running_overlap_ms,omitempty"`
	SystemOrKernelCompetitorCount  int     `json:"system_or_kernel_competitor_count,omitempty"`
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
	Frequency  int64   `json:"frequency"`
	DurationMs float64 `json:"duration_ms"`
	StartTs    float64 `json:"start_ts,omitempty"`
	EndTs      float64 `json:"end_ts,omitempty"`
	LineStart  int     `json:"line_start,omitempty"`
	LineEnd    int     `json:"line_end,omitempty"`
}

type CoreClassStats struct {
	Class                   string  `json:"class,omitempty"`
	CPUs                    []int   `json:"cpus,omitempty"`
	BusyMs                  float64 `json:"busy_ms,omitempty"`
	IdleMs                  float64 `json:"idle_ms,omitempty"`
	RunnableWaitMs          float64 `json:"runnable_wait_ms,omitempty"`
	HighPriorityRunMs       float64 `json:"high_priority_running_ms,omitempty"`
	SystemOrKernelRunningMs float64 `json:"system_or_kernel_running_ms,omitempty"`
	MaxFrequency            int64   `json:"max_frequency,omitempty"`
	TopologySource          string  `json:"topology_source,omitempty"`
	ComputeSupplySignal     string  `json:"compute_supply_signal,omitempty"`
}

// offCPUCauseSlice is ONE proof partition slice of a D/IO ledger group's
// segments (§29.50.5 逐片段证明门, v5 P1 批 件②): every fragment summed here
// carried the SAME typed wait-object proof (or none, on the "" slice). The
// accounting mirrors the group's own F-1 segment-truth carriers so a slice
// can mint an honest seat (true single-segment extrema, extents, lines).
// DStateAllNonIOProvenGroup / UnanimousCauseSymbol (修复轮二 件B): the
// chain-lane candidate's per-group proof surface (mint-stamped from the SAME
// ThreadDuration donor the family seats read — 单一值源).
func (c CriticalBlockingCandidate) DStateAllNonIOProvenGroup() bool { return c.proofRefined }
func (c CriticalBlockingCandidate) UnanimousCauseSymbol() string    { return c.proofCaller }

// DStateAllNonIOProvenGroup (修复轮二 件B, 2026-07-13) reports whether EVERY
// segment folded into this D-ledger group carried a sched_blocked_reason
// marker with iowait=0 (the per-GROUP refined-D proof — the record-level
// donor the display's refined 「D-state」 word consumes when no rank-family
// row reached the ledger; dispatch 无关化). False on non-D groups and on any
// coverage gap (absence never proves).
func (td ThreadDuration) DStateAllNonIOProvenGroup() bool {
	return td.dFamilySegments > 0 && td.dFamilyNonIOMarked == td.dFamilySegments
}

// UnanimousCauseSymbol (修复轮二 件B) returns the ONE proven wait-object
// symbol when every fragment of this group proved the same cause (exactly
// one cause slice, non-empty key); "" otherwise (partial coverage, conflict,
// or no slice inventory — absence never guesses).
func (td ThreadDuration) UnanimousCauseSymbol() string {
	if len(td.causeSlices) != 1 {
		return ""
	}
	for cause := range td.causeSlices {
		return cause
	}
	return ""
}

type offCPUCauseSlice struct {
	durMs     float64
	segCount  int
	segMinMs  float64
	segMaxMs  float64
	startTs   float64
	endTs     float64
	lineStart int
	lineEnd   int
	// anchoredMs (RSPA §29.61.10a/b/c, 2026-07-14): the slice's segment time
	// that intersects the thread's typed wakeup-dependency jump windows
	// (chain-anchored credential portion). Accumulated segment-by-segment at
	// the ONE ledger close site (addDurationCause) so 全窗账 = anchored +
	// remainder is an exact same-segment-set bipartition, never a second
	// aggregation. 0 when no anchor windows were supplied to the sweep.
	anchoredMs float64
}

type ThreadDuration struct {
	Thread     ThreadRef `json:"thread"`
	DurationMs float64   `json:"duration_ms"`
	CPU        int       `json:"cpu"`
	CoreClass  string    `json:"core_class,omitempty"`
	// Frequency is the legacy single cpu_frequency sample at the last judged
	// segment start (context only); weighted judgements use the unexported
	// accumulators below (methodology audit §7.30.2 R5e).
	Frequency     int64   `json:"frequency,omitempty"`
	StartTs       float64 `json:"start_ts,omitempty"`
	EndTs         float64 `json:"end_ts,omitempty"`
	LineStart     int     `json:"line_start,omitempty"`
	LineEnd       int     `json:"line_end,omitempty"`
	Priority      int     `json:"priority,omitempty"`
	PriorityClass string  `json:"priority_class,omitempty"`
	// F-1 segment-truth carriers (CAL-1 修复轮, 冷读 P1, 2026-07-12).
	// Unexported: in-package verdict input, never serialized. addDuration
	// accumulates them per aggregation key: segCount = folded segment count,
	// segMinMs/segMaxMs = TRUE single-segment extrema — the ×N(a–b) range
	// and the member-roster wording must never present a per-CPU group SUM
	// as a "段" (donghu E8: "单段 3.774–16.064ms" was 4 group sums over 11
	// raw segments whose true single-segment max is 3.853ms).
	segCount int
	segMinMs float64
	segMaxMs float64
	// anchoredMs (RSPA §29.61.10a/b/c, 2026-07-14). Unexported: in-package
	// verdict input, never serialized. The group's segment time intersecting
	// the thread's typed wakeup-dependency jump windows (union of the chain's
	// depth>0 node windows for this pid), accumulated at the ONE ledger close
	// site with the exact clamped segment endpoints — the chain-anchored
	// credential portion of this (thread,cpu) account. 0 when the sweep ran
	// without anchor windows (legacy paths; absence never guesses).
	anchoredMs float64
	// cpuUnknownReason (§29.104.21 DISPLAY-HYG 件4, 2026-07-17). Unexported:
	// in-package verdict input, never serialized. The UNIFORM typed continuity
	// reason of a runnable cpu=-1 bucket's segments (window_end_unverified /
	// wakeup_target_conflict / …), accumulated at addRunnableDuration: first
	// unknown segment stamps its verdict.reason, a differing later reason
	// collapses to the mixed sentinel ("" = never stamped — known-CPU buckets,
	// non-runnable state ledgers and legacy paths). Word-face consumer: the
	// runnable member-roster why word (采集端截断) mints ONLY on the uniform
	// window_end_unverified reason — a mixed or absent reason keeps the bare
	// cpu=unknown honest (有 reason 才佩; typed gate, never a guess).
	cpuUnknownReason string
	// runnableIntervals preserves the exact disjoint segment inventory behind
	// a runnable (thread,cpu) aggregate. Chain-lane admission must intersect
	// these intervals, never the aggregate StartTs..EndTs hull across gaps.
	runnableIntervals []foldInterval
	// dioIntervals (HULL-CRED, §29.104 终判③, 2026-07-17). Unexported:
	// in-package verdict input, never serialized. The exact clamped evidence
	// segments behind a D-state / IO-wait (thread,cpu) ledger group,
	// accumulated at the ONE close site (addDurationCause) with the same
	// endpoints DurationMs sums — the keep-⛓ credential inventory of the
	// chain-lane D/IO VIEW verdict.
	// ONCHAIN-FIX-2 件3 (Q6 已追认, 2026-07-18) — EVOLUTION RECORD over the
	// original all-or-nothing rule: a group beyond
	// CriticalBlockingCredentialSegmentCap now KEEPS the first cap segments
	// as an immutable checked prefix (proven lower bound) and latches
	// dioIntervalsOverflow; the latch means 「清单不完整」 — consumers must
	// read it before treating the list as complete (partial evidence proves
	// presence, never absence; a partial all-disjoint sweep must fall to the
	// envelope tier, never mint a disjoint verdict — 缺证≠证无).
	dioIntervals         []foldInterval
	dioIntervalsOverflow bool
	// priorityRange* identifies the real scheduler endpoints bracketing this
	// exact segment. It is deliberately unexported: only the priority-point
	// authority may turn the pair into a hard range verdict. Aggregated or
	// hand-built ThreadDuration values leave priorityRangeExact=false and can
	// never mint a direct priority-inversion relation from midpoint values.
	priorityRangeStartTs   float64
	priorityRangeEndTs     float64
	priorityRangeStartLine int
	priorityRangeEndLine   int
	priorityRangeExact     bool

	// DSTATE-REFINE arm a carriers (CAL-1 件③, §29.39②/§29.47.2, 2026-07-12).
	// Unexported: in-package verdict input, never serialized. dFamilySegments
	// counts the D-family segments folded into this duration on the DSTATE
	// (non-IO) ledger; dFamilyNonIOMarked counts the subset whose
	// sched_blocked_reason marker was FOUND with iowait=0 (coverage proof —
	// a segment without a marker can never prove the refined 「D-state」
	// word); dFamilyCallers collects the distinct semantic caller symbols
	// (blockedReasonSemanticCaller — dma_fence_default_wait family), capped,
	// for the 行2 等待对象 disclosure.
	dFamilySegments    int
	dFamilyNonIOMarked int
	dFamilyCallers     []string
	// causeSlices (§29.50.5 证明分区, v5 P1 批 件②, 2026-07-13). Unexported:
	// in-package verdict input, never serialized. The per-PROVEN-wait-object
	// slice accounting of this D/IO ledger group's segments (key = semantic
	// caller symbol via offCPUCauseSymbol; "" = the unproven slice). The
	// LEDGER GROUPING itself stays (thread,cpu) — 修复轮 (h1 ∿ 回归, 2026-07-13):
	// keying the ledger on the cause inflated the top-8 DStateTop/IOWaitTop
	// entry counts and the downstream wire caps evicted unrelated rows (the
	// pacing ∿ seat lost its record to a root_evidence capacity cut) — so
	// the proof partition consumes THESE slices at the family-mint layer
	// instead. Populated only on the D/IO buckets (bounded rows); nil on
	// runnable/sleep.
	causeSlices map[string]offCPUCauseSlice

	// R5e duration-weighted frequency accumulation over the judged segments
	// (§7.30.2). Unexported: in-package verdict input, never serialized.
	// freqWeightKHzMs is Σ(segment_ms × segment weighted kHz); freqKnownMs is
	// Σ segment_ms that had any frequency coverage; freqObservedMaxKHz is the
	// max sample observed inside/nearest the segments; freqInSegmentSamples
	// counts cpu_frequency change points strictly inside the segments.
	freqWeightKHzMs      float64
	freqKnownMs          float64
	freqObservedMaxKHz   int64
	freqInSegmentSamples int
}

// weightedFrequencyKHz returns the duration-weighted CPU frequency across the
// accumulated judged segments, zero when no cpu_frequency data covered them
// (§7.30.2 R5e: missing data yields no claim, never a default).
func (td ThreadDuration) weightedFrequencyKHz() int64 {
	if td.freqKnownMs <= 0 {
		return 0
	}
	return roundedCPUFrequencyKHz(td.freqWeightKHzMs / td.freqKnownMs)
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
	// Rank is the state-drilldown Top-N ordinal. Wire/text word is
	// `drill_rank` (RANKDIS-EXT A1, §29.104.16/.16.1 2026-07-16): the bare
	// `rank` key was shared with the root-cause board and the auto-window
	// candidate list, and a customer model grep'd the raw payload, read the
	// state ordinals as root-cause board seats and reconciled a phantom rank
	// contradiction for six turns (witness cust_span_vs_prio.txt). Readers of
	// pre-rename artifacts keep a fail-open `rank=` arm
	// (traceQueryStateDrilldownRecord).
	Rank     int       `json:"drill_rank,omitempty"`
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
	// RANKDIS-M18 (§29.104.17 裁定② 2026-07-16, implemented): the wire key
	// left the ms-semantic slot — `rank_impact_score` on the JSON payload and
	// the observation note face both (the note key was `rank_impact`); census
	// found zero Go readers of the old key, so the rename ships with no
	// compatibility arm. The Go field keeps its ms-scale name (the weight IS
	// ms-scaled), matching the Rank/`drill_rank` precedent above.
	RankImpactMs float64 `json:"rank_impact_score,omitempty"`
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
	SourcePath     string    `json:"source_path,omitempty"`
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
	SourcePath         string    `json:"source_path,omitempty"`
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
	// AmbiguousCohortCount counts exact coarse-key cohorts whose concurrent
	// depth exceeded one. PairingSuppressedCount is the number of operations
	// deliberately withheld rather than FIFO-guessed inside those cohorts.
	AmbiguousCohortCount   int     `json:"ambiguous_cohort_count,omitempty"`
	PairingSuppressedCount int     `json:"pairing_suppressed_count,omitempty"`
	Bytes                  int64   `json:"bytes,omitempty"`
	MaxLatencyMs           float64 `json:"max_latency_ms,omitempty"`
	AvgLatencyMs           float64 `json:"avg_latency_ms,omitempty"`
	LineStart              int     `json:"line_start,omitempty"`
	LineEnd                int     `json:"line_end,omitempty"`
	StartTs                float64 `json:"start_ts,omitempty"`
	EndTs                  float64 `json:"end_ts,omitempty"`
	Example                string  `json:"example,omitempty"`
	Summary                string  `json:"summary,omitempty"`
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
	IOWaitMs            float64 `json:"io_wait_ms,omitempty"`
	TopInode            string  `json:"top_inode,omitempty"`
	TopDev              string  `json:"top_dev,omitempty"`
	TopEntryName        string  `json:"top_entry_name,omitempty"`
	LineStart           int     `json:"line_start,omitempty"`
	LineEnd             int     `json:"line_end,omitempty"`
	Summary             string  `json:"summary,omitempty"`
}

type IOBurstEpisodeSummary struct {
	Thread         ThreadRef `json:"thread,omitempty"`
	ChainRelevance string    `json:"chain_relevance,omitempty"`
	// OnChainBasis (SELF-ALL, §29.61.2 2026-07-13): same closed set and
	// semantics as RootCauseRankItem.OnChainBasis — non-empty ONLY when the
	// episode's on-chain relevance was granted by the typed self wall-clock
	// predicate ("self_wall_clock_interval") instead of chain-window overlap.
	OnChainBasis        string     `json:"on_chain_basis,omitempty"`
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
	SourcePath      string  `json:"source_path,omitempty"`
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
	SourcePath             string    `json:"source_path,omitempty"`
	Thread                 ThreadRef `json:"thread,omitempty"`
	Work                   string    `json:"work,omitempty"`
	Function               string    `json:"function,omitempty"`
	Count                  int       `json:"count,omitempty"`
	PairedCount            int       `json:"paired_count,omitempty"`
	UnpairedStartCount     int       `json:"unpaired_start_count,omitempty"`
	UnpairedDoneCount      int       `json:"unpaired_done_count,omitempty"`
	AmbiguousCohortCount   int       `json:"ambiguous_cohort_count,omitempty"`
	PairingSuppressedCount int       `json:"pairing_suppressed_count,omitempty"`
	DurationMs             float64   `json:"duration_ms,omitempty"`
	MaxLatencyMs           float64   `json:"max_latency_ms,omitempty"`
	StartTs                float64   `json:"start_ts,omitempty"`
	EndTs                  float64   `json:"end_ts,omitempty"`
	LineStart              int       `json:"line_start,omitempty"`
	LineEnd                int       `json:"line_end,omitempty"`
	Summary                string    `json:"summary,omitempty"`
}

type DMAFenceActivity struct {
	SourcePath             string    `json:"source_path,omitempty"`
	Thread                 ThreadRef `json:"thread,omitempty"`
	Driver                 string    `json:"driver,omitempty"`
	Timeline               string    `json:"timeline,omitempty"`
	Context                string    `json:"context,omitempty"`
	Seqno                  string    `json:"seqno,omitempty"`
	Count                  int       `json:"count,omitempty"`
	PairedCount            int       `json:"paired_count,omitempty"`
	UnpairedStartCount     int       `json:"unpaired_start_count,omitempty"`
	UnpairedDoneCount      int       `json:"unpaired_done_count,omitempty"`
	AmbiguousCohortCount   int       `json:"ambiguous_cohort_count,omitempty"`
	PairingSuppressedCount int       `json:"pairing_suppressed_count,omitempty"`
	WaitMs                 float64   `json:"wait_ms,omitempty"`
	MaxWaitMs              float64   `json:"max_wait_ms,omitempty"`
	StartTs                float64   `json:"start_ts,omitempty"`
	EndTs                  float64   `json:"end_ts,omitempty"`
	LineStart              int       `json:"line_start,omitempty"`
	LineEnd                int       `json:"line_end,omitempty"`
	Summary                string    `json:"summary,omitempty"`
}

type SupplyPressureSummary struct {
	Signal string `json:"signal,omitempty"`
	// CPUPressureMs is the cross-thread sum of runnable-wait backlog only
	// (cpu·ms). HighPriorityRunningMs and the overlap fields are typed
	// occupancy/competition context for that same wait account; they are never
	// added to this numerator. Dividing this value by WindowMs yields average
	// runnable queue depth.
	CPUPressureMs                  float64                 `json:"cpu_pressure_ms,omitempty"`
	RunnableWaitMs                 float64                 `json:"runnable_wait_ms,omitempty"`
	CPUAttributedRunnableWaitMs    float64                 `json:"cpu_attributed_runnable_wait_ms,omitempty"`
	CPUUnattributedRunnableWaitMs  float64                 `json:"cpu_unattributed_runnable_wait_ms,omitempty"`
	HighPriorityRunningMs          float64                 `json:"high_priority_running_ms,omitempty"`
	SystemOrKernelRunningMs        float64                 `json:"system_or_kernel_running_ms,omitempty"`
	SystemOrKernelRunningOverlapMs float64                 `json:"system_or_kernel_running_overlap_ms,omitempty"`
	SystemOrKernelCompetitorCount  int                     `json:"system_or_kernel_competitor_count,omitempty"`
	SchedStatWaitMs                float64                 `json:"sched_stat_wait_ms,omitempty"`
	SchedStatIOWaitMs              float64                 `json:"sched_stat_iowait_ms,omitempty"`
	SchedStatBlockedMs             float64                 `json:"sched_stat_blocked_ms,omitempty"`
	IPIEventCount                  int                     `json:"ipi_event_count,omitempty"`
	IPIActiveMs                    float64                 `json:"ipi_active_ms,omitempty"`
	LowFrequencyCPUs               []int                   `json:"low_frequency_cpus,omitempty"`
	ClockSetRateCount              int                     `json:"clock_set_rate_count,omitempty"`
	ThermalEventCount              int                     `json:"thermal_event_count,omitempty"`
	DDREventCount                  int                     `json:"ddr_event_count,omitempty"`
	L3EventCount                   int                     `json:"l3_event_count,omitempty"`
	ThroughputEventCount           int                     `json:"throughput_event_count,omitempty"`
	TopBackgroundThreads           []ThreadCPULoadSummary  `json:"top_background_threads,omitempty"`
	TopBackgroundProcesses         []ProcessCPULoadSummary `json:"top_background_processes,omitempty"`
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
	MinFrequency int64   `json:"min_frequency,omitempty"`
	MaxFrequency int64   `json:"max_frequency,omitempty"`
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

// PacingIdleSummary is one P9 arm-c (§29.42 案1 BINDER-MISATTR,
// docs/design/real_trace_campaign_20260705.md, 2026-07-12) frame-pacing idle
// segment: a sleep segment whose binder-wait candidates were all written off
// (reply completed before the segment / waker process ≠ peer process), whose
// length is within binderPacingFramePeriodToleranceMs of one frame period,
// and whose segment-ending waker is on the frame-signal (vsync) dispatch
// chain. Published on its own semantic lane (rank rows carry
// RootCauseTierContextOnly — never a root-cause contender).
type PacingIdleSummary struct {
	Thread        ThreadRef `json:"thread"`
	Waker         ThreadRef `json:"waker,omitempty"`
	WindowStartTs float64   `json:"window_start_ts,omitempty"`
	WindowEndTs   float64   `json:"window_end_ts,omitempty"`
	DurationMs    float64   `json:"duration_ms,omitempty"`
	FramePeriodMs float64   `json:"frame_period_ms,omitempty"`
	// PeriodSource is the typed provenance of FramePeriodMs:
	// waker_periodic_aggregate (VS-1 PeriodicSource DetectedPeriodMs) or
	// waker_wakeup_cadence (the waker's consecutive wakeups of this thread
	// bracketing the segment).
	PeriodSource string `json:"period_source,omitempty"`
	// Kind is the typed lane token (复核 P2-1, 2026-07-12): pacing_idle
	// (frame-chain waker — 帧间空闲 frame wording) or periodic_idle (generic
	// periodic waker — 周期空闲 wording; the frame promise words never leak
	// to timer/audio style sources).
	Kind       string  `json:"kind,omitempty"`
	SleepLine  int     `json:"sleep_line,omitempty"`
	WakeupLine int     `json:"wakeup_line,omitempty"`
	WakeupTs   float64 `json:"wakeup_ts,omitempty"`
	// EvidenceLineStart/End (ENG-2 追修, 2026-07-12): the segment's causal
	// impact record's evidence span — the idle row's published coordinates
	// align to it so the display same-fact fold engages by construction (the
	// raw sleep/wakeup line pair above stays the audit-honest event locator).
	// Zero when no impact record measured this exact segment.
	EvidenceLineStart int `json:"evidence_line_start,omitempty"`
	EvidenceLineEnd   int `json:"evidence_line_end,omitempty"`
	// RejectedTransactionIDs lists the synchronous binder transactions the
	// write-off arms rejected for this segment (audit trail — the pre-P9
	// classifier would have attributed the segment to one of these).
	RejectedTransactionIDs []int  `json:"rejected_transaction_ids,omitempty"`
	Summary                string `json:"summary,omitempty"`
}

type TraceSpanSummary struct {
	SourcePath string    `json:"source_path,omitempty"`
	Thread     ThreadRef `json:"thread"`
	Kind       string    `json:"kind,omitempty"`
	Name       string    `json:"name,omitempty"`
	// SpanPID is the trace-mark payload pid of the opening B row (`B|{pid}|…`)
	// — the emitter's OWN pid-namespace process id, which for a containerized
	// process differs from the row-header host Thread.TGID (§18.E emission
	// pair). Carried so the LCK-2 rung-② ns-span owner derivation can key the
	// contention span to its container namespace; 0 when the payload carried
	// no pid.
	SpanPID     int    `json:"span_pid,omitempty"`
	TargetScope string `json:"target_scope,omitempty"`
	// ProcessMembershipSource is set only for explicit process-scope
	// discovery and names the exact typed field that proved membership.
	ProcessMembershipSource string  `json:"process_membership_source,omitempty"`
	Category                string  `json:"category,omitempty"`
	Subcategory             string  `json:"subcategory,omitempty"`
	SemanticClass           string  `json:"semantic_class,omitempty"`
	StartTs                 float64 `json:"start_ts,omitempty"`
	EndTs                   float64 `json:"end_ts,omitempty"`
	DurationMs              float64 `json:"duration_ms,omitempty"`
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

// TraceTrackSpanSummary is one complete Android atrace G/H
// ASYNC_FOR_TRACK interval. OwnerPID/TrackName/Cookie are the logical wire
// identity. BeginEmitter/EndEmitter are endpoint provenance only; neither is
// asserted to own or execute the tracked work.
type TraceTrackSpanSummary struct {
	SourcePath       string    `json:"source_path"`
	OwnerPID         int       `json:"owner_pid"`
	TrackName        string    `json:"track_name"`
	Name             string    `json:"name"`
	Cookie           string    `json:"cookie"`
	BeginEmitter     ThreadRef `json:"begin_emitter"`
	EndEmitter       ThreadRef `json:"end_emitter"`
	BeginPayload     string    `json:"begin_payload,omitempty"`
	EndPayload       string    `json:"end_payload,omitempty"`
	StartTs          float64   `json:"start_ts"`
	EndTs            float64   `json:"end_ts"`
	DurationMs       float64   `json:"duration_ms"`
	ActualStartTs    float64   `json:"actual_start_ts,omitempty"`
	ActualEndTs      float64   `json:"actual_end_ts,omitempty"`
	ActualDurationMs float64   `json:"actual_duration_ms,omitempty"`
	StartLine        int       `json:"start_line"`
	EndLine          int       `json:"end_line"`
}

// TraceInstantSummary is one Android atrace I/N point marker. Action stays
// explicit because I (process instant) and N (instant-for-track) have
// different ownership shapes. No duration field exists by design.
type TraceInstantSummary struct {
	SourcePath string    `json:"source_path"`
	Action     string    `json:"action"`
	OwnerPID   int       `json:"owner_pid"`
	TrackName  string    `json:"track_name,omitempty"`
	Name       string    `json:"name"`
	Emitter    ThreadRef `json:"emitter"`
	Payload    string    `json:"payload,omitempty"`
	Ts         float64   `json:"ts"`
	Line       int       `json:"line"`
}

// TraceCounterDeltaSummary is the numeric aggregation of one exact atrace /
// Harmony C| counter identity inside the selected window. Counter identity is
// the verbatim (physical source, payload owner pid, logical name) tuple --
// never the ftrace row-header tid. OpenHarmony's terminal level/tag-bits token
// is publication metadata, not a track identity. Thread retains the historical wire
// contract: it is the row-header emitter of the first sample. Payload ownership
// is carried only by OwnerPID/OwnerScope. A payload pid of zero is an explicit
// global counter and is distinguished by OwnerScope.
//
// Baseline is always "in_window_first_sample": no pre-window state is guessed
// from index padding. UnitStatus is always "unknown" because the C| wire
// grammar carries no unit field; unit-looking name text is deliberately not
// parsed as authority. A series containing any invalid/non-finite value is
// omitted from this face and disclosed through CounterQuality instead.
type TraceCounterDeltaSummary struct {
	Thread     ThreadRef `json:"thread"`
	OwnerPID   int       `json:"owner_pid"`
	OwnerScope string    `json:"owner_scope"`
	Name       string    `json:"name,omitempty"`
	// TrailingTag is the retained raw OpenHarmony metadata token for wire
	// compatibility. It is never part of logical series identity. New clients
	// should consume OutputLevel/TagBits/MetadataStatus.
	TrailingTag    string  `json:"trailing_tag,omitempty"`
	OutputLevel    string  `json:"output_level,omitempty"`
	TagBits        string  `json:"tag_bits,omitempty"`
	MetadataStatus string  `json:"metadata_status,omitempty"`
	SourcePath     string  `json:"source_path,omitempty"`
	Baseline       string  `json:"baseline"`
	UnitStatus     string  `json:"unit_status"`
	Samples        int     `json:"samples,omitempty"`
	First          float64 `json:"first"`
	Last           float64 `json:"last"`
	Min            float64 `json:"min"`
	Max            float64 `json:"max"`
	Delta          float64 `json:"delta"`
	FirstLine      int     `json:"first_line,omitempty"`
	LastLine       int     `json:"last_line,omitempty"`
	FirstLocalLine int     `json:"first_local_line,omitempty"`
	LastLocalLine  int     `json:"last_local_line,omitempty"`
}

// TraceCounterQualitySummary is the bounded fail-loud face for C| rows that
// cannot participate in a deterministic numeric series. Counts cover the full
// selected window; Issues retains only a small sample per reason. Suppression
// is series-wide: skipping a malformed middle or final sample and still
// publishing first/last would falsely claim the observed endpoint state.
type TraceCounterQualitySummary struct {
	Rows                 int                        `json:"rows"`
	ValidIdentityRows    int                        `json:"valid_identity_rows,omitempty"`
	NumericRows          int                        `json:"numeric_rows,omitempty"`
	InvalidRows          int                        `json:"invalid_rows,omitempty"`
	NonNumericRows       int                        `json:"non_numeric_rows,omitempty"`
	DerivedInvalidSeries int                        `json:"derived_invalid_series,omitempty"`
	TotalSeries          int                        `json:"total_series,omitempty"`
	TotalSeriesStatus    string                     `json:"total_series_status,omitempty"`
	PublishedSeries      int                        `json:"published_series,omitempty"`
	SuppressedSeries     int                        `json:"suppressed_series,omitempty"`
	TruncatedSeries      int                        `json:"truncated_series,omitempty"`
	SeriesBudget         int                        `json:"series_budget,omitempty"`
	SeriesBudgetExceeded bool                       `json:"series_budget_exceeded,omitempty"`
	OverflowRows         int                        `json:"overflow_rows,omitempty"`
	BaselinePolicy       string                     `json:"baseline_policy"`
	UnitPolicy           string                     `json:"unit_policy"`
	Issues               []TraceCounterIssueSummary `json:"issues,omitempty"`
}

type TraceCounterIssueSummary struct {
	Reason  string                    `json:"reason"`
	Count   int                       `json:"count"`
	Samples []TraceCounterIssueSample `json:"samples,omitempty"`
}

type TraceCounterIssueSample struct {
	Line        int    `json:"line,omitempty"`
	LocalLine   int    `json:"local_line,omitempty"`
	SourcePath  string `json:"source_path,omitempty"`
	OwnerRaw    string `json:"owner_raw,omitempty"`
	Name        string `json:"name,omitempty"`
	Value       string `json:"value,omitempty"`
	TrailingTag string `json:"trailing_tag,omitempty"`
	OutputLevel string `json:"output_level,omitempty"`
	TagBits     string `json:"tag_bits,omitempty"`
}

type TraceCounterSummary struct {
	// Thread is the ftrace row-header emitter. Owner* is the independent C|
	// payload owner and deliberately matches CounterDeltas' typed identity.
	Thread      ThreadRef `json:"thread"`
	OwnerPID    int       `json:"owner_pid"`
	OwnerRaw    string    `json:"owner_raw,omitempty"`
	OwnerScope  string    `json:"owner_scope,omitempty"`
	Name        string    `json:"name,omitempty"`
	Value       string    `json:"value,omitempty"`
	TrailingTag string    `json:"trailing_tag,omitempty"`
	OutputLevel string    `json:"output_level,omitempty"`
	TagBits     string    `json:"tag_bits,omitempty"`
	Count       int       `json:"count,omitempty"`
	Line        int       `json:"line,omitempty"`
	Ts          float64   `json:"ts,omitempty"`
}

type IRQBurstSummary struct {
	CPU     int     `json:"cpu"`
	Name    string  `json:"name,omitempty"`
	IRQ     int     `json:"irq,omitempty"`
	Count   int     `json:"count,omitempty"`
	StartTs float64 `json:"start_ts,omitempty"`
	EndTs   float64 `json:"end_ts,omitempty"`
	// SpanMs is first-to-last inventory coverage, not interrupt active time.
	// Only InterruptActivity.ActiveMs may carry paired entry/exit duration.
	SpanMs float64 `json:"span_ms,omitempty"`
	// DurationMs is retained for wire compatibility but intentionally remains
	// zero: an IRQ burst envelope must never masquerade as elapsed work.
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
	Target ThreadRef  `json:"target,omitempty"`
	Window TimeWindow `json:"window"`
	// BoardParamsFingerprint (XLANE-3 件1, §29.104.2 定谳③, 2026-07-16) is the
	// typed params half of the rank BOARD identity triple (query window, board
	// target subject, params fingerprint). Minted once at the rank build entry
	// from the NORMALIZED rank-shaping knobs (rootCauseBoardParamsFingerprint —
	// closed set MaxDepth/MaxBranches/MinDurationMs/Limit; window and target
	// are deliberately excluded because they are the triple's other two
	// components, and View is excluded so a frame_root_cause_bundle run and a
	// root_cause_rank run with one spec stay ONE board). Two same-window
	// same-target boards whose knobs differ are genuinely different ordinal
	// domains; identical specs re-queried collapse to one board (identity, not
	// call provenance). Display board-identity input only — no gate, score or
	// sort lane reads it.
	BoardParamsFingerprint string              `json:"board_params_fingerprint,omitempty"`
	Items                  []RootCauseRankItem `json:"items,omitempty"`
	// AbsorbedItems (B4 cross-type rank-seat reconciliation, 2026-07-10):
	// lossless observations that no longer consume a competing rank seat
	// because an adjudicated row in Items proved it was the exact same
	// physical segment. The row keeps its typed source/interval/line evidence
	// plus AbsorbedByRankFamily/AbsorbedIntoFamily; renderers join it back to
	// the absorbing row by the verbatim RankFamilyKey. Keeping this carrier
	// separate both reclaims the rank/capacity seat and preserves the raw
	// observation for audit/evidence publication.
	AbsorbedItems []RootCauseRankItem `json:"absorbed_items,omitempty"`
	// BusinessSpanMentions (SPANVIS-1 定形原则, user ruling 2026-07-19): the
	// PURE-ADVISORY business-lens mention face — on-chain (incl. self)
	// span families whose in-window wall-clock total clears the significance
	// floor, aggregated from the FULL span inventory (never the bounded
	// display view). It mints NO seat, joins NO ordinal population, and no
	// gate/score/sort lane reads it (不参与根因排序); nil when the window has
	// no admissible family.
	BusinessSpanMentions *BusinessSpanMentionResult `json:"business_span_mentions,omitempty"`
	// GatedCompositeEdgeShareDisclosures (PARTSPLIT-1, §29.150④ user ruling
	// 2026-07-19): the result-level NON-SEAT disclosure side channel for
	// R4-mirror-refused gated composite seats — one record per refused seat,
	// harvested from the pre-truncation pool ∪ the published board (the
	// SPANVIS BusinessSpanMentions side-channel family: the refused seat may
	// die at the publication cap — the tieba 23088 live form — yet its
	// pre-edge share disclosure must still reach the ◎ face). Mints NO seat,
	// joins NO ordinal population, enters NO conservation/census denominator,
	// and no gate/score/sort lane reads it; nil when no refusal is on record.
	// POOL2-1 件③ (§29.160③): rows are value-ordered (pre-edge share desc)
	// and floor-gated (SPANVIS two-component floor) at the harvest — a
	// below-floor refusal keeps only its per-item typed stamp (audit).
	GatedCompositeEdgeShareDisclosures []GatedCompositeEdgeShareDisclosure `json:"gated_composite_edge_share_disclosures,omitempty"`
	// SelfRunnableTwoRuler (RULER2-1, §29.150② user ruling / R-19-b,
	// 2026-07-19): the target's own runnable seats split across the TWO
	// closed rulers (self_wall_clock vs on_wakeup_chain) — the typed
	// cross-row accounting record behind the 按两把尺记账 disclosure sentence.
	// Minted ONLY when the published board carries self runnable seats on
	// BOTH rulers (single-ruler boards stay nil — the §29.136 single-ruler
	// fold faces own that shape). Display wording input ONLY: no gate,
	// score, sort, seat or value lane reads it, and NO cross-ruler total
	// exists anywhere (M3 禁混尺 — Σ across rulers is a mixed-ruler number).
	SelfRunnableTwoRuler *SelfRunnableTwoRulerAccounting `json:"self_runnable_two_ruler,omitempty"`
	// SelfRunningFoldUnmeasured (SELFRUN-DISC, §29.192① (b) user ruling /
	// A2 件11(b) handoff §29.194, 2026-07-21): the self supply-fold 「量不了」
	// absence disclosure — minted ONLY when the target ran inside the window
	// while the fold basis was ENTIRELY unknown (KnownMs==0 ∧ UnknownMs>0,
	// every slice folded at ratio 1 因数据缺席), so the zero deficit must not
	// wear the 「无损失」 face. NON-SEAT side channel (the two-ruler /
	// edge-share family): no seat, no ordinal, no conservation/census
	// membership, and no gate/score/sort lane reads it; nil whenever a
	// deficit seat minted (缺口>0) or the zero rides a known basis (真满频).
	SelfRunningFoldUnmeasured *SelfRunningFoldUnmeasuredDisclosure `json:"self_running_fold_unmeasured,omitempty"`
	Caveats                   []string                             `json:"caveats,omitempty"`
	Compactions               []ViewCompaction                     `json:"compactions,omitempty"`
	// preTruncationItems (RSPA-HYG 件⑤, §29.77 立案⑤, 2026-07-14). Unexported,
	// never serialized: the UNION of the boards AS HANDED to the capacity
	// truncations (the build lane seeds it, the enrich lane appends its own
	// truncation input — a Run truncates twice and a counterpart may die at
	// EITHER cap while the ◇ remainder survives on the side lane; donghu
	// witness: udk-irq-12-92's 0.039ms ⛓ chain seat died in the build-lane
	// 60→12 cap). The bipartition population-conservation sweep reads it as
	// the typed release arm: a ◇ remainder whose ⛓ counterpart is absent from
	// the published board must find it HERE with the truncation disclosed via
	// Compactions — a direct membership assertion instead of the former noisy
	// "compacted ∧ anchored<1ms" magnitude release.
	preTruncationItems []RootCauseRankItem
	// censusDemotedLabels / runnableFallbackLabels (DISPFIX-1 件1, §29.213
	// 排期件5, 2026-07-23). Unexported, never serialized: the demoted seat-name
	// SETS the build lane published on its census / runnable-ledger-fallback
	// caveat, carried into the enrich re-publication so the two lanes' sets MERGE
	// (build 车道席集 ∪ enrich 车道席集) into ONE union-rendered sentence instead
	// of the enrich lane DROPPING its own on the sentinel-prefix collision — the
	// former swallowed a seat the enrich lane demoted that the build lane never
	// named (G14 / §29.210+§29.211 候办; F-4 记档候办 in the two dedupe helpers).
	// The carry is truncation-robust: a build-demoted seat truncated out of the
	// enrich board before its census is still named because its label rides here
	// (the item-set scan alone would lose it). Real-fleet none/fallback
	// population is zero (八板, §29.204.1) so this never fires live — a pure
	// look-after upgrade; single-lane forms stay byte-identical.
	censusDemotedLabels    []string
	runnableFallbackLabels []string
	// p3MeasureCtx (P3MEASURE-1, §29.169, 2026-07-20). Unexported, never
	// serialized: the chain-derived typed inputs of the silent on-chain
	// measurement (anchor windows, VS-1 periodic flags, census-access edge
	// inventory), stashed by the build lane (chain + cache in scope) and
	// consumed once at the shared finalize tail (stampP3CounterfactualMeasure
	// — rank_p3_measure.go). nil = no chain universe, nothing measured.
	p3MeasureCtx *p3MeasureContext
}

// RootCauseSubjectKindAggregateMetric is the typed SubjectKind for root-cause
// rows whose subject is a window/CPU-scoped aggregate metric (cpu_pressure,
// io_pressure without a representative file-IO thread, cpu_frequency_limit,
// irq_burst, irq_activity, ipi_activity, supply_pressure) rather than a
// resolvable thread. Renderers must not present such rows as an
// "unknown thread": the empty ThreadRef is structural, not a resolution gap.
const RootCauseSubjectKindAggregateMetric = "aggregate_metric"

// RootCauseTierDeterministicOptimization is retained for backward wire
// compatibility with persisted DCS-era records. The current engine never
// mints it: on-chain semantic span-work participates in the ordinary
// primary/secondary/tertiary election, while off-chain semantic work carries
// tertiary plus BackgroundRank. Consumers must still render legacy records
// honestly and keep their independent deterministic-optimization mention.
//
// EVOLUTION RECORD (审计 #60/#66 追认, §29.25 处置委托 + §29.26 待主会话落账,
// 2026-07-10). §29.7-2 ① 原裁定原文: "on-chain 语义类行无条件全权参赛、可登顶
// (board/lead/➊➋➌ 全开,tier 词'确定性优化候选'身份保留)"; §29.22 as-built had
// kept an independent tier + empty-primary-bucket display crowning. The tier
// MINT retirement (direct primary/secondary/tertiary competition) is RATIFIED
// as the fuller reading of 全权参赛 — but the second half of the clause
// (tier-word IDENTITY) is display-load-bearing and must survive the wire
// change: every display face that keyed on tier=="deterministic_optimization"
// now ALSO keys on the typed SemanticClass token (行2 类别词 form table,
// priority/layer cells — internal/tool), so semantic rows keep the
// 确定性优化候选/确定优化/确定性优化点 identity words without the tier.
// Flipped pins (semantic_lead_semlead_test.go, trace_query_dcs_test.go) carry
// their own EVOLUTION RECORDs.
const RootCauseTierDeterministicOptimization = "deterministic_optimization"

// RootCauseTierContextOnly marks a typed row whose authoritative effective
// attribution is exactly zero. The raw scheduler/span duration remains useful
// chain context, but the row has no root-cause board seat: Rank stays 0 and it
// cannot consume capacity, become a ranked cause, or receive a TOP badge.
// The source views (wakeup_chain/window_stats) retain the full raw evidence.
const RootCauseTierContextOnly = "context_only"

// RootCauseTierTargetSelfState (SYM, ledger §24.13 裁定一, user ruling
// 2026-07-08, real_trace_campaign_20260705.md): the independent Tier word for
// rank rows whose SUBJECT thread is the analysis target itself AND whose cause
// token belongs to the 等待症状族 (own binder wait / self-held or waited
// blocking_span / own sleep-before-wakeup segments). In a "why is the target
// stuck" question these rows are the SYMPTOM being explained: they keep their
// raw evidence and every score/weight/sort input untouched, but carry Rank=0,
// consume no board capacity, and are TRANSPARENT to the
// primary/secondary/tertiary positional election. Identity is
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

// RootCauseTierAbsorbed is the non-competing identity of a rank-lane
// observation moved to RootCauseRankResult.AbsorbedItems by a precise
// cross-type reconciliation ruling. It carries Rank=0 and is published only
// as supporting evidence; the absorbing row owns the single board seat.
const RootCauseTierAbsorbed = "absorbed"

// RootCauseTierCaliberSide (V2-P0 行级尺守卫, rank_order_v2_design_20260712.md
// §6.1 新裁定 A, GREENLIT 2026-07-12): the independent Tier word for
// count-additivity / composite-score rows on the chain/◇ ordinal channels —
// the two-scale-ruler red line dropped from channel level to row level. A
// row wearing it publishes its value under an explicit caliber word
// (计数当量 / 复合分数, ⌗ 口径旁栏) but carries NO rank ordinal (Rank=0),
// consumes no candidate capacity, never sorts against wall-clock rows and
// never wears a badge. Rendering obligation is preserved: the row keeps its
// channel seat downstream (no silent-disappearance path). Identity = the
// SHARED registry arm CausalTokenCaliberSideClass (registry typed
// Additivity==count OR the typed composite-score marker) evaluated on
// non-background channels only (▒ has no ordinals to guard). Wire token:
// verbatim in the typed tier note / root_cause_<tier> predicate — the
// root_cause_primary prefix never matches (target_self_state precedent).
const RootCauseTierCaliberSide = "caliber_side"

// RootCauseChainRelevanceSelfCaliberSide (RNB-5B 件②, §29.96.2 终判②, user
// ruling 2026-07-15): the ChainRelevance wire token of the analysis TARGET's
// own COUNT-additivity rows (计数当量族 — e.g. page_cache_churn) — a
// NON-CHANNEL semantic. R8 (自身恒为链上) forbids the self row wearing the ◇
// adjacent channel, while the §29.83 caliber discipline keeps count-equivalent
// magnitudes out of the wall-clock chain lanes — the two rulings meet on the
// ⌗ 口径旁栏 side rail: the row keeps its rendered seat and its evidence
// obligation but claims NO causal channel (the token replaces the former
// "adjacent" proximity verdict). Non-self count rows and composite-score self
// rows keep their legacy lanes byte-identically.
const RootCauseChainRelevanceSelfCaliberSide = "self_caliber_side"

// RootCauseOnChainBasisSelfDeterministicSpan (SELF-SEM, §29.61.1 user ruling
// 2026-07-13): a member of the RootCauseRankItem.OnChainBasis closed set
// {""|self_deterministic_span|self_wall_clock_interval} — the analysis
// target's own deterministic semantic span(s) admitted to the on-chain channel
// by typed self identity (chain universe present ∧ chain.Target resolved ∧
// sameThreadRef(span, target) ∧ deterministic semantic class ∧ in-window),
// never by chain-window overlap. The row's Causality carries
// RootCauseCausalitySelfDeterministic; the wakeup edge set is untouched
// (不铸唤醒边不宣称跨线程关系).
const RootCauseOnChainBasisSelfDeterministicSpan = "self_deterministic_span"

// RootCauseOnChainBasisSelfWallClockInterval (SELF-ALL, §29.61.2/§29.61.2a
// user rulings 2026-07-13, extending §29.61.1): the SECOND non-empty member of
// the OnChainBasis closed set — the analysis target's own WALL-CLOCK seat
// (blocked-state family / IO facet seat / runnable / running) admitted to the
// on-chain channel by typed self identity plus a typed wall-clock interval
// inside the query window (selfWallClockSeatLane — the shared SELF-ALL
// admission predicate), never by chain-window overlap. Non-wall-clock calibers
// (composite score / count equivalents, V2-P0 registry verdict) never take
// this basis and stay on the ⌗ caliber side rail. The row's Causality carries
// RootCauseCausalitySelfWallClock; no wakeup edge is minted (零唤醒边宣称).
// Naming note: §29.61.2 suggested "self_blocking_wait" as an example, but the
// §29.61.2a effective-attribution ladder explicitly includes the target's own
// RUNNING seats (supply-fold caliber) — "blocking" would be a false claim on
// those rows, so the honest interval-scoped token is used instead.
const RootCauseOnChainBasisSelfWallClockInterval = "self_wall_clock_interval"

// RootCauseOnChainBasisHostWakeupEdge (R3-IMPL, §29.88.1 user ruling
// 2026-07-14, ledger real_trace_campaign_20260705.md; R4 §29.88.2 通则):
// the THIRD non-empty member of the OnChainBasis closed set — a NON-target
// thread's deterministic semantic span admitted to the on-chain channel by
// the HOST thread's own in-window typed wakeup edge toward the analysis
// target (direct raw census edge host→target, or the host's own chain edge —
// 凭证沿链传递, the 60595 depth-2 multi-hop form), with the span lying BEFORE
// that edge (边=凭证,边前=有效,边后=解除 — the R2/RSPA anchoring semantics
// shared verbatim). The participation value is the pre-edge in-window
// projection; a span crossing the edge bisects at the boundary (pre-edge part
// stays on this seat, the post-edge part rides a ◇ ChainAnchorRemainderSeat
// clone). No credential edge → the span keeps the legacy adjacent/background
// lane byte-identically (SCAN-3 negative sentinel; the donghu 17267 decoy pin:
// a span straddling SOMEONE ELSE's edge earns nothing — the credential is the
// host's OWN edge, never window co-presence). Causality carries the honest
// "on_wakeup_chain" (unlike the self bases, a REAL typed wakeup edge exists).
const RootCauseOnChainBasisHostWakeupEdge = "host_wakeup_edge_pre_span"

// RootCauseOnChainBasisHostWakeupEdgeState (ONCHAIN-3c — R3 凭证臂扩射程到
// 状态席, mint audit 反向缺口5, 2026-07-19): the FOURTH non-empty member of
// the OnChainBasis closed set — a NON-target, NON-chain-member thread's
// runnable / D-IO STATE seat admitted to the on-chain channel by the same
// credential function as the span basis above (hostSemanticSpanEdgeAnchor,
// zero-change reuse: the host's own in-window typed wakeup edge toward the
// analysis target; in practice always via=direct — a chain-member host is
// excluded structurally because RSPA owns chain members' state-credential
// vocabulary). Same boundary semantics verbatim (边=凭证,边前=有效,边后=
// 解除): the participation value is the Σ of the seat's TRUE segment
// inventory's pre-edge shares (runnableIntervals / dioSegmentIntervals* —
// never the StartTs..EndTs hull), a boundary-crossing account bisects
// per-segment (pre-edge share keeps this seat, the post-edge share rides a ◇
// ChainAnchorRemainderSeat clone; full = pre + post is an exact partition).
// A SIBLING token rather than a reuse of the span token: the display wording
// forks on the single OnChainBasis field (types 纪律 below — never a
// basis∧type recomposition), and the two carriers publish different value
// forms (span pre-edge window projection vs state-segment pre-edge Σ with a
// per-state D/IO split). SCAN-3 61839 判例: the bare census edge host whose
// dio 3.550ms + runnable 0.370ms pre-edge shares had no door onto the chain
// tier while its semantic span already rode the span basis. Causality
// carries the honest "on_wakeup_chain" (a REAL typed edge exists).
const RootCauseOnChainBasisHostWakeupEdgeState = "host_wakeup_edge_pre_state"

// Closed set for RootCauseRankItem.HostWakeupEdgeAnchorVia (R3-IMPL): which
// typed edge inventory supplied the credential. Disclosure wording input only.
const (
	HostWakeupEdgeAnchorViaDirect         = "direct"
	HostWakeupEdgeAnchorViaChainHop       = "chain_hop"
	HostWakeupEdgeAnchorViaDirectChainHop = "direct+chain_hop"
)

// RootCauseCausalitySelfDeterministic (SELF-SEM causality token, 开放裁定点①
// adopted 2026-07-13): the honest causality word for a self-basis on-chain row.
// "on_wakeup_chain" would be a fabricated cross-thread claim for a row that
// carries no wakeup edge; "" would break the 21-consumer causality fallback
// chain. chainRelevanceFromCausality maps it to "on_chain" so every
// relevance-fallback consumer keeps the row in the chain universe.
const RootCauseCausalitySelfDeterministic = "self_deterministic"

// RootCauseCausalitySelfWallClock (SELF-ALL causality token, §29.61.2
// 2026-07-13): the honest causality word for a self wall-clock-basis on-chain
// row — same closed-set discipline as RootCauseCausalitySelfDeterministic
// (chainRelevanceFromCausality maps it to "on_chain"; every consumer that
// enumerates the causality closed set carries this member; "on_wakeup_chain"
// on such a row would fabricate a cross-thread wakeup claim).
const RootCauseCausalitySelfWallClock = "self_wall_clock"

// TraceGapKind* (G2 判据 typed 化, §27.2 + §28.1, 2026-07-09): the PRECISE
// typed criterion split behind a trace_gap mint. The legacy single wording
// "窗内无调度数据" over-claimed: the same (thread, window) could carry a
// depth-0 running rank row (#3, 0.051ms) beside a "no scheduler data" blind
// spot — the window HAD intervals, they just all sat below the MinDurationMs
// floor. Two closed enum forms, decided at the single mint site from the
// thread's own timeline (len(intervals)):
//   - no_sched_data     — the thread timeline holds NO interval at all inside
//     the aligned window (the only shape the old wording
//     was true for);
//   - no_eligible_wait  — intervals exist but ALL sit below MinDurationMs
//     (复核 P3-5 precise fact: mostInterestingInterval's
//     fallback admits any state at/above the floor, so
//     nil ⟺ all below it — a running interval at/above
//     the floor never reaches the mint).
//
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
	SubjectKind string    `json:"subject_kind,omitempty"`
	Thread      ThreadRef `json:"thread,omitempty"`
	// ProcessComm (CR-3 件③ P11, 2026-07-12): the owning PROCESS comm
	// resolved from the window thread catalog by the row thread's TGID (the
	// tgid==tid main-thread entry; 冷读案8 裸线程名死指针 witness). "" when
	// the catalog holds no main-thread comm for the tgid (absence never
	// guesses — the thread's own comm is NOT a process name). Display /
	// board-summary identity slot only; rank lanes never read it.
	ProcessComm string `json:"process_comm,omitempty"`
	// PhysicalSourcePath is the bundle-local physical artifact identity behind
	// duration rows. Logical Source (window_stats/semantic/...) describes the
	// producer lane; this field prevents lookalike intervals from different
	// attachments being folded into one rank family.
	PhysicalSourcePath string                     `json:"physical_source_path,omitempty"`
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
	// DStateAllNonIOProven (DSTATE-REFINE arm a, CAL-1 件③ §29.39②/§29.47.2,
	// 2026-07-12): true ONLY when this merged D/IO row's io_wait share is
	// zero AND every member segment on the D ledger carried a
	// sched_blocked_reason marker with iowait=0 (blocked_reason 全覆盖∧全0 —
	// the typed proof behind the refined 「D-state」 display word; a segment
	// without a marker keeps the honest merged form). Wording input only —
	// rank/score lanes never read it.
	DStateAllNonIOProven bool `json:"d_state_all_noniowait,omitempty"`
	// BlockedReasonCaller (件③ caller 等待对象族): the UNANIMOUS semantic
	// caller symbol across the row's marked D-ledger segments
	// (dma_fence_default_wait family); "" when members disagree or none was
	// marked (absence never guesses). 行2 等待对象 disclosure input.
	BlockedReasonCaller string `json:"blocked_reason_caller,omitempty"`
	// BlockedReasonWindowCount / BlockedReasonWindowCaller (CR-3 件② P10,
	// 2026-07-12): the UNCONSUMED residual — when the unanimous-caller lane
	// minted nothing, the window may still hold sched_blocked_reason records
	// for this thread (冷读案7: the root-cause row said 未解析 while the
	// GPU-fence marker sat in hand). Count = the thread's in-window marker
	// count from the window account (top-N capped inventory; a thread
	// outside the cap keeps zero — loose, disclosure-only); caller = the
	// distinct semantic symbols (cap 2, "/"-joined; "" when all opaque/hex).
	// Wording input only — rank/score lanes never read them, and rows whose
	// BlockedReasonCaller already consumed the marker never mint them.
	BlockedReasonWindowCount  int    `json:"blocked_reason_window_count,omitempty"`
	BlockedReasonWindowCaller string `json:"blocked_reason_window_caller,omitempty"`
	// DStateCauseUnprovenRemainder (§29.50.5 证明分区, v5 P1 批 件②,
	// 2026-07-13): true ONLY on the honest-remainder D/IO seat — the
	// unproven fragments of a thread whose OTHER fragments proved a concrete
	// wait object and were carved into sibling cause seat(s) (逐片段证明门:
	// 未证片段留通用 (线程,类型) 席; 绝不灌根因席). Drives the
	// 「D-state(原因未证)」 display form. A thread with no cause seat never
	// wears it (a lone generic seat is not a remainder). Wording input only
	// — rank/score lanes never read it.
	DStateCauseUnprovenRemainder bool `json:"d_state_cause_unproven_remainder,omitempty"`
	// ChainAnchoredMs / ChainAnchorFullMs / ChainAnchorRemainderSeat (RSPA
	// §29.61.10a/b/c user rulings, 2026-07-13/14): the on-chain seat-value
	// re-anchoring decomposition. A window_stats state seat of a chain thread
	// no longer publishes its FULL-window account on the chain tier: the
	// credential-anchored portion (segments ∩ the thread's typed wakeup-
	// dependency jump windows — constructively equal to the chain lane's own
	// per-state value, µs-identity gated) is owned by the chain-lane seat,
	// and THIS row becomes the ◇ adjacent remainder seat:
	//   ChainAnchorFullMs   — the same-source full-window account (census
	//                         basis; the pre-migration seat value);
	//   ChainAnchoredMs     — the anchored portion that moved to the chain
	//                         seat (0 ≤ anchored ≤ full);
	//   ChainAnchorRemainderSeat — true on the ◇ remainder seat; its
	//                         published value channels carry full − anchored.
	// 同源二分: the two seats partition ONE segment set (mutually disjoint,
	// additive back to the full account — the ONLY additive relation form;
	// wall-clock across different accounts stays non-additive). Absent
	// (0/0/false) on every non-migrated row: no chain, no jump windows, no
	// census basis, µs-identity gate failed — all fail open to the legacy
	// full-window publication (无凭证形禁猜).
	ChainAnchoredMs          float64 `json:"chain_anchored_ms,omitempty"`
	ChainAnchorFullMs        float64 `json:"chain_anchor_full_ms,omitempty"`
	ChainAnchorRemainderSeat bool    `json:"chain_anchor_remainder_seat,omitempty"`
	// ChainAnchorOwnershipDivergent + ChainAnchorChainLaneMs /
	// ChainAnchorCensusMs (RNB-1, §29.88 R2/R4 user rulings, 2026-07-14):
	// the case-A OWNERSHIP qualification verdict, decoupled from the census
	// bipartition (which is self-sufficiently exact at its single ledger
	// close site). The former §6.3 µs-identity gate no longer fails the whole
	// migration open (fail-open kept the FULL window value on the chain tier
	// — the §29.88 W1/W2 disease); it now only decides whether the chain seat
	// provably OWNS the anchored portion. When it diverges (chain-lane Σ ≠
	// census-anchored Σ, or the chain seat's published value ≠ the anchored
	// account), the window seat still migrates to the ◇ remainder but the
	// relation sentence downgrades from 同源二分(可加还原) to the honest
	// double-account form (账目关系): both Σs and their delta are disclosed
	// typed here — the armed-tick disclosure face (§29.84 件① 同构; one
	// customer replay pins each seat's diverging gate):
	//   ChainAnchorOwnershipDivergent — true on the ◇ remainder seat of a
	//                         divergent pid (never on case-B clipped pairs:
	//                         without a chain seat the ownership question is
	//                         void and the census pair bisects exempt);
	//   ChainAnchorChainLaneMs — the chain lane's own per-state Σ for the pid;
	//   ChainAnchorCensusMs    — the pid-level census-anchored Σ (the other
	//                         account of the double-Σ disclosure; the row's
	//                         ChainAnchoredMs stays its OWN seat-group share).
	ChainAnchorOwnershipDivergent bool    `json:"chain_anchor_ownership_divergent,omitempty"`
	ChainAnchorChainLaneMs        float64 `json:"chain_anchor_chain_lane_ms,omitempty"`
	ChainAnchorCensusMs           float64 `json:"chain_anchor_census_ms,omitempty"`
	// ChainCredentialLaneDemoted (RNB-1, §29.88 R4 排他通则, 2026-07-14):
	// the R4 lane demotion for indivisible on-chain rows that cannot show a
	// typed causal-edge anchored share for their whole account — the
	// cpu_affinity_or_cpuset satellite (capped-basis duplicate, no per-row
	// interval inventory) and the priority-inversion-retyped window seat
	// (its gated eff is a same-CPU displacement measurement — 10a: pure time
	// overlap is adjacency-level — and clipping it would mint a value equal
	// to neither the measurement nor any partition term, §29.83 件③). The
	// whole seat rides the ◇ adjacent lane with every published value
	// channel untouched (值零动,通道位归位); a fully-anchored pid account
	// (census remainder ≤ tol) keeps the chain lane byte-identically.
	// LEVELMERGE-1 修补轮件3 (2026-07-18) joins the population: the
	// gated-share residual seat (B) whose credential segments were ALL
	// claimed by the interval-accounting split (∪occ − ∪claim empty) —
	// it cannot show one residual segment of its own, so it rides the same
	// ◇ credential lane (the demotion moves only the channel; the residual
	// value was set by the split itself, not by this flag).
	ChainCredentialLaneDemoted bool `json:"chain_credential_lane_demoted,omitempty"`
	// ChainIdentityInheritance (ONCHAIN-FIX-1 件1, mint audit 命题2 不一致①,
	// 2026-07-18): the typed ADMISSION RECORD of the interval-less same-pid
	// fail-open arm — this row published no typed interval (end<=start) and
	// inherited the on-chain lane from bare thread identity (its pid is a
	// chain member; chainContextForCandidate). The lane keep is the
	// documented conservative boundary (无凭证形禁猜); what the pre-fix shape
	// ALSO did — fabricating OverlapMs from the whole node-window wall clock —
	// is retired: OverlapMs stays 0 and this bit drives the honest
	// 「成员继承(链窗级,无区间凭证)」 disclosure word instead. The bit records
	// the admission basis; disclosure consumers gate on the CURRENT on-chain
	// lane (链上面与降道面不同行共存), and stronger adjudication vocabularies
	// (HULL-CRED per-segment / envelope words) suppress it. The analysis
	// target's own rows never carry it (R8 self-causality, SELF 族词面).
	ChainIdentityInheritance bool `json:"chain_identity_inheritance,omitempty"`
	// ChainCredentialEnvelopeLevel (ONCHAIN-FIX-2 件1 — 包络泛化, mint audit
	// 命题2 不一致②, 2026-07-18): the rank-lane mirror of
	// CriticalBlockingCandidate.ChainCredentialEnvelopeLevel (same json name,
	// same note key, same 「交集证明(包络级)」 legend word — 零新词). Stamped by
	// the chain-context enrich on a keep-⛓ legacy-basis row whose on-chain
	// verdict rests ONLY on its StartTs..EndTs envelope intersecting the
	// same-pid chain windows — no per-segment inventory, no single-segment
	// µs identity (measured wall clock ≈ envelope length would make the hull
	// the one true segment), no typed credential arm (M-IO completion
	// closure / host-window containment / resolved lock pair / mint-time
	// semantic intersection / wakeup-chain construction), and no RSPA-owned
	// lane (the re-anchoring machinery owns those types' credential
	// vocabulary). Fail-open honest word only: the lane and every published
	// value are untouched (禁一刀切硬拒); the analysis target's own rows and
	// self/R3 bases never carry it. Interval-less rows wear the
	// identity-inheritance word instead (the two words are exclusive by
	// construction).
	ChainCredentialEnvelopeLevel bool `json:"chain_credential_envelope_level,omitempty"`
	// ChainCredentialCensus (CHAINGUARD-1 件1, §29.204/§29.204.1, 2026-07-22):
	// the closed chain-credential census verdict, minted at exactly one point
	// (censusChainSeatCredential) at the tail of assignRootCauseRanksAndTiers
	// on every chain-channel Rank>0 eff>0 seat of a chained board —
	// wakeup_anchored / target_self / interval_proven / member_inherited, or
	// "none" (the violation record: the seat carried ZERO typed credential
	// stamps and was demoted to the ▒ background lane, values untouched,
	// result caveat raised). "" everywhere outside the census population
	// (chainless boards, Rank==0/eff≤0 seats, ◇/▒/⌗ lanes). The ◎ credential
	// chip word maps this enum (件3 同源) and the display board second gate
	// rejects census=none seats (件2 跨 query 合并洞); wording/channel input
	// only — no value or sort lane reads it.
	ChainCredentialCensus string `json:"chain_credential_census,omitempty"`
	// ChainAnchorRepresentedByChainSeat (XLANE-1 件1, §29.104.1/§29.104.2,
	// 2026-07-15): the fully-anchored runnable-family SATELLITE whole-seat ◇
	// demotion. A scheduler_latency / low_frequency diagnostic projection
	// whose OWN interval inventory lies entirely inside the thread's typed
	// wakeup-dependency windows (anchored ≈ full) used to keep its FULL value
	// on the chain tier (`:791 continue`) — the runnable2 customer escape:
	// E11 调度延迟 23.471 full beside chain seats E26/E28 (17.635+8.608) of
	// the SAME physical runnable, chain-lane runnable eff Σ 53.5ms on a
	// 26.725ms window (2.0×). Per the B4 header semantics (the satellite
	// "must not mint a second seat"): when a same-pid chain-lane runnable
	// seat (wakeup_chain.causal_impacts / aggregated_impacts) is in the same
	// pool AND physically intersects the satellite's intervals, the anchored
	// share is ALREADY represented on the chain tier — the whole satellite
	// rides the ◇ adjacent lane with every published value untouched. This
	// marker is deliberately DISTINCT from ChainCredentialLaneDemoted: the
	// satellite HAS chain credential; the demotion reason is seat
	// representation, so the display word face must never speak 无链上凭证
	// on it. A pid whose chain-lane runnable seat is ABSENT keeps the chain
	// lane byte-identically — the satellite is the anchored share's only
	// representative (禁把锚定份丢出链). The exact interval-twin subset is
	// additionally absorbed into the chain seat by the extended B4 recon
	// pair (single seat + E# merge); this ◇ demotion is the non-twin
	// fallback layer.
	ChainAnchorRepresentedByChainSeat bool `json:"chain_anchor_represented_by_chain_seat,omitempty"`
	// GatedShare* (LEVELMERGE-1 件2 方案 P 区间分账, user ruling 2026-07-18):
	// the (pid, runnable) chain AGGREGATE seat's interval-accounting split
	// against the same thread's priority-inversion chain seat(s), whose R5d
	// gated composite already counts the runnable share inside their branch
	// windows at full value (the runnable2 E26+E28 Σ>物理 double-count
	// mechanism — cross-group-key, so no earlier fold/recon reaches it):
	//   GatedShareClaimedMs — A: |∪(claiming inversion seats' segment
	//                         windows) ∩ ∪(aggregate occurrence windows)|,
	//                         clamped to the pre-split account (pure interval
	//                         measure over merged unions; multi-claimant
	//                         unions FIRST, subtracts once);
	//   GatedShareFullMs    — the pre-split aggregate account; identity
	//                         claimed + residual(=published RunnableMs of the
	//                         surviving seat) == full is pinned (GATED-CAL
	//                         three-way identity precedent);
	//   GatedShareConstituentSeat — true on the demoted CONSTITUENT row (the
	//                         A share): adjacent lane, never competes, points
	//                         at the inversion seat; the claimed value rides
	//                         its published channels (链上面与降道面不得同行
	//                         共存 — this row carries no on-chain marker);
	//   GatedShareClaimSeats — the claiming inversion seat(s)' own line
	//                         intervals ("start..end"), the display's typed
	//                         pointer input for the [E#] cross-reference
	//                         (all-or-nothing at render, 宁漏勿假指);
	//   GatedShareOverlapDisclosureMs — the fail-open arm (裁定④ sentence
	//                         form 「其中 X ms 与[E#]重叠」): a partial typed
	//                         inventory witnesses the overlap over the REAL
	//                         segments available (lower bound) but never
	//                         bounds a value split — every published value
	//                         unchanged. Zero everywhere the split ran.
	// All zero/absent on every untouched row (three-state honesty: split,
	// disclosure, or byte-identical).
	GatedShareClaimedMs           float64  `json:"gated_share_claimed_ms,omitempty"`
	GatedShareFullMs              float64  `json:"gated_share_full_ms,omitempty"`
	GatedShareConstituentSeat     bool     `json:"gated_share_constituent_seat,omitempty"`
	GatedShareClaimSeats          []string `json:"gated_share_claim_seats,omitempty"`
	GatedShareOverlapDisclosureMs float64  `json:"gated_share_overlap_disclosure_ms,omitempty"`
	// HostWakeupEdgeAnchorTs / HostWakeupEdgeAnchorVia (R3-IMPL, §29.88.1
	// user ruling 2026-07-14): the typed edge-anchoring disclosure pair on a
	// RootCauseOnChainBasisHostWakeupEdge semantic seat, a
	// RootCauseOnChainBasisHostWakeupEdgeState state seat (ONCHAIN-3c,
	// 2026-07-19) and their ◇ remainder clones. AnchorTs is the LATEST
	// in-window credential edge timestamp — the bisection boundary (every
	// span instant ≤ it lies before SOME credential edge; instants after it
	// are 边后=解除). Via names the typed edge inventory that supplied the
	// credential (closed set: "direct" = raw wakeup-census pair host→target;
	// "chain_hop" = the host's own chain edge, 凭证沿链传递;
	// "direct+chain_hop" = both). Absent (0/"") on every other row.
	HostWakeupEdgeAnchorTs  float64 `json:"host_wakeup_edge_anchor_ts,omitempty"`
	HostWakeupEdgeAnchorVia string  `json:"host_wakeup_edge_anchor_via,omitempty"`
	// GatedCompositeEdge* (PARTSPLIT-1, §29.150④ user ruling 2026-07-19): the
	// R4-mirror REFUSAL record + its disclosure-only bisection measures on a
	// gated composite seat (priority_inversion_runnable_wait family). The
	// ONCHAIN-3c inversion arm computed a true pre/post bisection of the
	// seat's runnable census inventory at the host's own credential-edge
	// boundary but REFUSED the lane conversion because a post-edge share
	// exists (the gated eff is an indivisible composite — RSPA R4/§29.83 件③:
	// splitting it would mint a value equal to neither the measurement nor
	// any partition term). These four fields are stamped ATOMICALLY at that
	// single refusal site and nowhere else — their presence IS the typed
	// refusal record (LEVELMERGE 披露拆分范式: split the MEASURE for
	// disclosure, never the published authority):
	//   GatedCompositeEdgePreShareMs  — X: the pre-edge share of the runnable
	//                                   census account (Σ of the pre-boundary
	//                                   segment clips);
	//   GatedCompositeEdgePostShareMs — Y: the post-edge share; X + Y == the
	//                                   seat's runnable census account
	//                                   (RunnableMs) to the µs by
	//                                   construction (the arm's own Σ gate);
	//   GatedCompositeEdgeAnchorTs/Via — WHICH boundary bisected the account
	//                                   (the same closed-set via vocabulary
	//                                   as HostWakeupEdgeAnchorVia; a
	//                                   DEDICATED pair so the R3 keep arms
	//                                   never read a refused seat).
	// Disclosure/wording inputs only — every published value channel, lane,
	// ordinal and score stays byte-identical (R4 整席不拆 floor); never a
	// rank/score/sort input. Absent (0/"") on every other row.
	GatedCompositeEdgePreShareMs  float64 `json:"gated_composite_edge_pre_share_ms,omitempty"`
	GatedCompositeEdgePostShareMs float64 `json:"gated_composite_edge_post_share_ms,omitempty"`
	GatedCompositeEdgeAnchorTs    float64 `json:"gated_composite_edge_anchor_ts,omitempty"`
	GatedCompositeEdgeAnchorVia   string  `json:"gated_composite_edge_anchor_via,omitempty"`
	// CPUConstraint* (RNB-2 件5 AFF-EVID, §29.88.6, 2026-07-15): the affinity/
	// cpuset seat's typed judgment payload — the mint's own decision inputs
	// (computeCPUConstraintSummaries → cpuConstraintRestrictsExecution) carried
	// to the display so the seat is never a bare assertion (病根=有料不上桌:
	// the W5 witness row said only 「CPU亲和/cpuset限制 · runnable」 with a
	// near-whole-trace evidence span):
	//   CPUConstraintKind        — the judgment-basis event kind (e.g.
	//                              sched_switch_next_info / the raw constraint
	//                              event name);
	//   CPUConstraintCPUSet      — the cpuset/cgroup group name ("" = none);
	//   CPUConstraintPolicy      — the verbatim policy string (carries
	//                              restricted=true when present; audit face);
	//   CPUConstraintAllowedCPUs — the sorted allowed-CPU union the constraint
	//                              events published;
	//   CPUConstraintExcludedCPUs— the in-window OBSERVED CPUs absent from the
	//                              allowed set (the very comparison
	//                              cpuConstraintRestrictsExecution decides on;
	//                              §29.88.4 R5a reserve: this is the
	//                              「限制上更大核可能性」 comparison input —
	//                              the per-core-档 refinement lands with the
	//                              RNB-4 R6 cluster work).
	// All zero-valued on every non-affinity row (fields ride only the
	// window_stats.cpu_constraints mint).
	CPUConstraintKind         string `json:"cpu_constraint_kind,omitempty"`
	CPUConstraintCPUSet       string `json:"cpu_constraint_cpuset,omitempty"`
	CPUConstraintPolicy       string `json:"cpu_constraint_policy,omitempty"`
	CPUConstraintAllowedCPUs  []int  `json:"cpu_constraint_allowed_cpus,omitempty"`
	CPUConstraintExcludedCPUs []int  `json:"cpu_constraint_excluded_cpus,omitempty"`
	// CPUConstraintAllowedMaxTierKHz / CPUConstraintGlobalMaxTierKHz (R5a
	// §29.88.4 场景②, 2026-07-15): the 按核档 exclusion proof pair — minted
	// together exactly when the binding provably excludes a bigger core TIER
	// (see CPUConstraintSummary.AllowedMaxTierKHz); drives the obligatory
	// 「绑核排除更大核档」 mention. Zero pair = no claim (禁无中生有).
	CPUConstraintAllowedMaxTierKHz int64 `json:"cpu_constraint_allowed_max_tier_khz,omitempty"`
	CPUConstraintGlobalMaxTierKHz  int64 `json:"cpu_constraint_global_max_tier_khz,omitempty"`
	// ResourceCompletionClosure (RSPA M-IO, §29.61.10c): typed per-IO
	// completion-closure credential on an io_latency row — the IO's
	// completion thread is recorded as the WAKER that ended an ANCHORED D/IO
	// segment of a chain thread inside the IO's lifetime (block completion →
	// 段尾 wakeup closure). With the RSPA anchor basis present, a non-target
	// resource-attribution row keeps the on-chain lane ONLY with this
	// credential; pure interval overlap demotes to ◇ (10c: 纯时间重叠恒邻近).
	ResourceCompletionClosure bool `json:"resource_completion_closure,omitempty"`
	// resourceClosureEvaluated (unexported): true when the RSPA anchor basis
	// existed at mint time so the closure credential above was actually
	// computable — the M-IO demotion arm engages ONLY then (legacy fixtures /
	// anchor-less sweeps keep the pre-RSPA overlap behavior byte-identically).
	resourceClosureEvaluated bool
	// resourceHostContainment* (unexported; RSPA-HYG 件③, §29.77 立案③ /
	// §29.61.10c per-edge criterion, 2026-07-14; §29.83 残余③ extended the
	// facet set — see stampResourceClosureEvaluation for the per-edge
	// dispositions): the io_burst_episode / block_io_by_inode /
	// file_io_hot_inode / workqueue_activity / dma_fence_activity host-form
	// credential refined from "any interval
	// overlap" to typed CONTAINMENT — the facet's credential is the anchored
	// host thread's own wait/work OCCUPYING its dependency window, which holds
	// exactly when the row's typed interval sits inside the thread's merged
	// anchor-window union (µs tolerance). Evaluated=true when the anchor basis
	// existed at mint time AND the row carries a positive typed interval;
	// Contained=true when interval ⊆ anchor windows. A partially-contained
	// non-target row demotes to ◇ adjacent (values untouched — the D-IO
	// partition's additive bisection never reads this facet's composite/episode
	// caliber, so the lane move is the mechanically compatible narrowing;
	// clipping would mint a value equal to neither the measured episode nor
	// any partition term). Anchor-less builds keep the legacy overlap lane.
	resourceHostContainmentEvaluated bool
	resourceHostWindowContained      bool
	// ledgerAnchor* (unexported): mint-time stamps of the census ledger's
	// anchored/full split for this seat (Σ over the seat's members). Working
	// inputs for the re-anchoring pass; never serialized.
	//
	// RNB-1 T1 root fix (§29.88 R2, 2026-07-14): the runnable lane joins the
	// stamp — each window runnable seat carries its OWN census group's
	// anchored overlap (ledgerAnchoredRunnableMs, Σ'd across members by the
	// family fold under the sum_disjoint caliber only). The migration
	// reconciles the seat against ITS OWN group ledger account instead of the
	// former seat-value == pid-census-full identity, whose failure on any
	// mixed-lane split fold (one dust group on another CPU re-laned adjacent
	// by the enrich overlap arm) failed the whole pid open and kept the full
	// multi-fragment value on the chain tier (customer E8/E22, INV-D S5/S6).
	ledgerAnchorStamped      bool
	ledgerAnchoredDMs        float64
	ledgerAnchoredIOMs       float64
	ledgerAnchoredRunnableMs float64
	ImpactMs                 float64 `json:"impact_ms,omitempty"`
	ProjectedImpactMs        float64 `json:"projected_impact_ms,omitempty"`
	CumulativeImpactMs       float64 `json:"cumulative_impact_ms,omitempty"`
	EffectiveImpactMs        float64 `json:"effective_impact_ms,omitempty"`
	// RankSortBoostedEffectiveMs (SEM-LEAD §29.7-2 ② + 复核 P1-1 修向(a),
	// ledger real_trace_campaign_20260705.md §29.22, 2026-07-10) is the
	// ENGINE-INTERNAL boost channel for on-chain semantic span work: the
	// deterministic hidden-cost heuristic (ImpactMultiplier × window
	// projection, bounded by semanticTraceSpanEffectiveImpactMs) that used to
	// publish AS EffectiveImpactMs. `json:"-"` keeps it off every
	// wire/view/observation face — the published effective attribution AND
	// the on-chain rank ordinal key are always the real projection
	// (家族真实合计; 792-textup 214.561 表值泄漏修根 + 复核序值倒挂修根:
	// ordinal ≡ board ≡ badges). The heuristic feeds exactly one soft face:
	// rootCauseRankScoreBasisMs, a same-published-effective tie-break. It never
	// changes capacity: the main board is the strict effective-sorted prefix,
	// and below-cut semantic work is disclosed by the independent optimization
	// channel.
	// Zero on every non-semantic row and whenever the boost would not exceed
	// the real projection.
	RankSortBoostedEffectiveMs float64 `json:"-"`
	// GatedRunnableMs / GatedRunningDeficitMs mirror the R5d gated-impact
	// composition for priority_inversion_candidate rows (§7.30.3 D3); zero on
	// every other row type. GatedCapabilitySource (CAP §26 C3) mirrors the
	// backing impact/aggregate's typed capability caliber for the discounted
	// running component (CoreCapabilitySource* tokens; wording input only).
	GatedRunnableMs       float64 `json:"gated_runnable_ms,omitempty"`
	GatedRunningDeficitMs float64 `json:"gated_running_deficit_ms,omitempty"`
	GatedCapabilitySource string  `json:"gated_capability_source,omitempty"`
	// PriorityRelation* carries the typed proof coverage behind an inversion
	// seat. Only lower-priority relation intervals contribute to Effective;
	// every unknown/equal/higher remainder stays visible and contributes zero.
	PriorityRelationCaliber             string  `json:"priority_relation_caliber,omitempty"`
	PriorityRelationProvenLowerMs       float64 `json:"priority_relation_proven_lower_ms,omitempty"`
	PriorityRelationUnknownOrNonLowerMs float64 `json:"priority_relation_unknown_or_nonlower_ms,omitempty"`
	// PriorityRelationArtifactSources is the sorted, deduplicated union of the
	// physical scheduler artifacts whose point/range verdicts authorized this
	// relation. PriorityRelationCaliber intentionally remains the compatibility
	// proof-strength field; it is not a physical source identity.
	PriorityRelationArtifactSources []string `json:"priority_relation_artifact_sources,omitempty"`
	// GatedClusterTopology (CAP-2 §28.4/§28.5): typed cluster-topology source
	// of the discounted running component's capability map
	// (CoreCapabilityTopology* tokens; empty on explicit/legacy — mirror of
	// SupplyFoldBasis.ClusterTopologySource). Wording input only.
	GatedClusterTopology string `json:"gated_cluster_topology,omitempty"`
	// GatedCapabilityFreqOnlyReason (DISPHYG-3 件7 — the CLUSTER-FIX-2 D5
	// gated reason twin, 2026-07-20): the typed freq_only cause token of the
	// SAME per-query capability judgment (CoreCapabilityFreqOnlyReason*
	// closed set; non-empty iff the capability source is freq_only — S1
	// discipline). Wording input only: the gated caliber suffix forks its
	// 簇结构不可判 wording on it exactly like the supply-fold clause, so the
	// two faces can never contradict on one page.
	GatedCapabilityFreqOnlyReason string `json:"gated_capability_freq_only_reason,omitempty"`
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
	// OnChainBasis (SELF-SEM, §29.61.1 user ruling 2026-07-13, ledger
	// real_trace_campaign_20260705.md): the typed PROOF BASIS behind an
	// on-chain ChainRelevance. Closed set:
	//   ""                        — legacy chain-window OVERLAP basis (every
	//                               pre-SELF-SEM on-chain row; zero wire drift);
	//   "self_deterministic_span" — the analysis TARGET's own deterministic
	//                               semantic span(s) inside the query window
	//                               (RootCauseOnChainBasisSelfDeterministicSpan):
	//                               the row rides the on-chain channel/universe
	//                               WITHOUT any chain-window overlap and WITHOUT
	//                               claiming a cross-thread wakeup relation (不
	//                               铸唤醒边不宣称跨线程关系 — Causality carries
	//                               "self_deterministic", never
	//                               "on_wakeup_chain", and OverlapMs stays 0);
	//   "self_wall_clock_interval" — SELF-ALL (§29.61.2/§29.61.2a): the
	//                               target's own WALL-CLOCK seat (blocked-state
	//                               family / IO facet / runnable / running)
	//                               whose typed interval lies inside the query
	//                               window — same no-overlap / no-wakeup-edge
	//                               discipline; Causality carries
	//                               "self_wall_clock". Effective attribution
	//                               consumes the SAME per-state ladder as every
	//                               on-chain row (running=supply-fold,
	//                               runnable=full, D/IO=wall-clock sum — 零特判).
	//   "host_wakeup_edge_pre_span" — R3-IMPL (§29.88.1/§29.88.2): a NON-target
	//                               thread's deterministic semantic span whose
	//                               HOST holds an in-window typed wakeup edge
	//                               toward the target (direct or via its own
	//                               chain edge), the span lying BEFORE that
	//                               edge (边=凭证,边前=有效,边后=解除); the
	//                               participation value is the pre-edge
	//                               in-window projection. Causality carries the
	//                               honest "on_wakeup_chain" (a REAL typed edge
	//                               exists, unlike the self bases).
	//   "host_wakeup_edge_pre_state" — ONCHAIN-3c (R3 射程扩展, 2026-07-19): a
	//                               NON-target, NON-chain-member thread's
	//                               runnable / D-IO STATE seat anchored by the
	//                               SAME host-edge credential (same boundary
	//                               semantics; the participation value is the
	//                               seat's true segment inventory's pre-edge Σ,
	//                               per-state for D/IO — see the sibling const
	//                               doc). Same honest "on_wakeup_chain"
	//                               causality; the post-edge share rides a ◇
	//                               ChainAnchorRemainderSeat clone.
	// Minted ONCE (mint time for SELF-SEM, the ONE enrich lane decision for
	// SELF-ALL); the enrich pass KEEPS an existing self basis instead of
	// re-deciding the lane (§23.1 lane-decided-once discipline).
	// Display wording forks on THIS single field (「目标自身·确定性优化」限定词) —
	// never on a SubjectIsAnalysisTarget∧SemanticClass∧relevance recomposition.
	OnChainBasis string `json:"on_chain_basis,omitempty"`
	ChainDepth   int    `json:"chain_depth,omitempty"`
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
	// OwnerTidPresence (LOCKNS-FIX 修补 件A, 2026-07-16): ported verbatim from
	// the folded blocking candidate — the typed presence verdict of the
	// payload owner tid on a rung-①-diverged row (absent /
	// present_collision / present_comm_mismatch). See
	// CriticalBlockingCandidate.OwnerTidPresence.
	OwnerTidPresence string `json:"owner_tid_presence,omitempty"`
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
	// HolderSelfContradictionParts (G10-EN 根修, QH2-A 2026-07-14): the typed
	// components of the witness above, ported verbatim from the folded
	// blocking candidate — the zh/EN display lanes each word their own
	// sentence from these (types.TraceHolderSelfContradictionWitness).
	// nil = guard never fired.
	HolderSelfContradictionParts *types.TraceHolderSelfContradictionWitness `json:"holder_self_contradiction_parts,omitempty"`
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
	MemberCount  int      `json:"member_count,omitempty"`
	MemberRoster []string `json:"member_roster,omitempty"`
	// MemberLineRanges (XLANE-2 件1, §29.104.1/.2 定谳④, 2026-07-17): the
	// COMPLETE per-member trace line ranges ("start..end", member order) of a
	// SEMANTIC family seat — minted all-or-nothing (any member without a
	// valid line range, or a family beyond the emission cap, mints nothing:
	// a partial set could fake a member-subset verdict downstream). Unlike
	// the bounded MemberRoster (semantic family lane bounded by the line-range
	// cap 32 since SPANTOP-1 件2; generic same-thread-type fold keeps cap 8)
	// this carries EVERY member, because the display-side subset judgment
	// (为[E#]成员子集 demotion) must never compare truncated sets. Display
	// judgment input only — no gate, score or sort lane reads it.
	MemberLineRanges []string `json:"member_line_ranges,omitempty"`
	// MemberWallMs (SPANTOP-1 件1, §29.131, 2026-07-18): the COMPLETE
	// per-member in-window wall-clock durations ("%.3f", member order — the
	// same fam.Members order as MemberLineRanges) of a SEMANTIC family seat —
	// minted all-or-nothing under the same discipline (any member without a
	// positive duration, or a family beyond the emission cap, mints nothing:
	// a partial list could fake a member decomposition downstream). Display
	// constituent-sub-row input only — no gate, score or sort lane reads it;
	// the display side additionally requires the µs identity Σ(members) ==
	// seat 行1 value before rendering anything from it.
	MemberWallMs      []string `json:"member_wall_ms,omitempty"`
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
	// --- G1/B4 cross-lane reconciliation -----------------------------------
	//
	// RankFamilyKey is the canonical reconciliation identity rendered once by
	// the engine and copied verbatim to the absorbed row's
	// AbsorbedIntoFamily. G1 stamps it on an io_latency family that absorbed
	// critical_blocking rows; B4 stamps it on a d_state_or_io_wait row that
	// absorbed an exactly coincident io_burst_episode rank row. Display joins
	// never re-derive this identity from labels or values.
	//
	// AbsorbedChainRows counts G1 critical-blocking observations;
	// AbsorbedRankRows counts B4 root-rank observations. Both keep publishing
	// through their lossless carriers; only the duplicate render/board seat is
	// folded.
	// Information conservation: the absorbed rows KEEP publishing (观测照发
	// 不删 — evidence index / system supplement / audit tokens stay lossless);
	// only their tree/stanza RENDER seat folds into this row.
	RankFamilyKey     string `json:"rank_family_key,omitempty"`
	AbsorbedChainRows int    `json:"absorbed_chain_rows,omitempty"`
	AbsorbedRankRows  int    `json:"absorbed_rank_rows,omitempty"`
	// AbsorbedByRankFamily / AbsorbedIntoFamily are the absorbed-side typed
	// markers used by B4 rank-to-rank reconciliation. They intentionally share
	// the established G1 wire keys: projection compilation has one exact-key
	// fold choke point, while the JSON fields keep the provenance explicit.
	AbsorbedByRankFamily bool   `json:"absorbed_by_rank_family,omitempty"`
	AbsorbedIntoFamily   string `json:"absorbed_into,omitempty"`
	// familyMemberIntervals is the merged family's member interval inventory
	// (engine-internal, never serialized): mergeSameThreadTypeRankFamily
	// stamps the validated member [start,end] pairs so the G1 reconciliation
	// can test a critical_blocking row's interval against the member UNION —
	// the precise membership signal (§27.2-G1 修向) — instead of the lossy
	// merged-row hull (hull gaps would absorb non-members). Second consumer
	// (INTERSECT-FIX, 2026-07-19): the AXIOM-V2 direction population's
	// family-segments basis arm, gated by exact fold caliber + µs union
	// identity (rank_direction_axiom.go — census hull inventories fail the
	// identity and never enter).
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
	// runnableCPU/runnableCPUKnown carry the exact CPU scope of a runnable
	// rank row for context joins. They are never serialized; an aggregate or
	// continuity-degraded row keeps runnableCPUKnown=false so no known-CPU
	// context can be inherited by TID-only coincidence.
	runnableCPU       int
	runnableCPUKnown  bool
	runnableIntervals []foldInterval
	// runnableCPUUnknownReason (§29.104.21 DISPLAY-HYG 件4, 2026-07-17;
	// engine-internal, never serialized): the census bucket's UNIFORM typed
	// continuity reason carried onto the runnable rank row (see
	// ThreadDuration.cpuUnknownReason). Word-face consumer only — the family
	// roster's cpu=unknown why word; no gate/ordinal/value lane reads it.
	runnableCPUUnknownReason string
	// hostEdgeRemainderStartTs/EndTs (unexported, R3-IMPL §29.88.1): the
	// post-boundary (边后) extent of a host-edge-anchored semantic seat's
	// span/family union — the mint loop clones the ◇ remainder seat from
	// this stash (跨边按边界二分). Zero-width on fully pre-edge seats.
	hostEdgeRemainderStartTs float64
	hostEdgeRemainderEndTs   float64
	// selfGapRunningIntervals (unexported, XLANE-2 件2 裁定④ §29.104.17,
	// 2026-07-17): the self running supply-fold deficit seat's OWN typed
	// running interval inventory (window-projected, the mint's decomposition
	// source) — the overlap-disclosure pass intersects it with the target's
	// own semantic seats' member intervals. Never serialized.
	selfGapRunningIntervals []foldInterval
	// semanticMemberIntervals (unexported, XLANE-2 件2): a semantic family
	// seat's COMPLETE member span intervals (all-or-nothing at the mint — a
	// partial set could overstate nothing but understate the overlap, and
	// the disclosure claims the exact X). Never serialized.
	semanticMemberIntervals []foldInterval
	// dioSegmentIntervals (unexported, ONCHAIN-FIX-2 件4 — AXIOM-V2 偏离④
	// 衔接, 2026-07-18): the formal D/IO seat's TRUE close-site segment
	// inventory, pushed down from its member groups' ThreadDuration
	// dioIntervals ledgers. Minted all-or-nothing at
	// mintRootCauseDIOStateSeat: exact sum_disjoint caliber only, every
	// member a whole-td group, no member's ledger overflowed, and every
	// member's Σ(segments) reproduces its own account (µs tol) — otherwise
	// absent (fail-open; the seat then stays out of the AXIOM-V2 direction
	// population, 宁漏勿假指). This is the per-segment carrier the direction
	// support closed set reads (dio_segment_intervals basis) — the
	// census-minted familyMemberIntervals hulls above stay OUT of that
	// population (their internal gaps fail the family-segments arm's µs
	// identity; fold-pass exact-caliber families DO enter through that arm —
	// INTERSECT-FIX 2026-07-19, see rank_direction_axiom.go). Never
	// serialized.
	dioSegmentIntervals []foldInterval
	// dioSegmentIntervalsD / dioSegmentIntervalsIO (unexported, ONCHAIN-3c,
	// 2026-07-19): the SAME validated close-site segments partitioned by
	// owning ledger state (dstateCensus buckets vs iowaitCensus buckets —
	// each bucket is single-state by construction, so the split is exact,
	// never an apportionment). Stamped in the same all-or-nothing block as
	// dioSegmentIntervals (present together or absent together; their union
	// IS dioSegmentIntervals). Consumer: the bare-census-edge state-seat
	// bisection (anchorBareCensusEdgeStateSeats), which must split the
	// DStateMs / IOWaitMs channels at the credential boundary PER STATE —
	// a proportional split would fabricate per-state values (禁摊派).
	// Never serialized.
	dioSegmentIntervalsD  []foldInterval
	dioSegmentIntervalsIO []foldInterval
	// SelfGapSemanticOverlaps (XLANE-2 件2, user ruling §29.104.17 ④
	// 「披露式拆分」, 2026-07-17): on the self running supply-fold deficit
	// seat ONLY — the typed interval-intersection wall clock this seat
	// shares with each of the target's own semantic seats (per-partner:
	// overlap ms + the partner's line envelope for the display-side [E#]
	// resolution). Pure disclosure lane: the main value channels stay
	// untouched (主值零动,硬扣除不做), no gate/score/sort reads it.
	SelfGapSemanticOverlaps []RootCauseSelfGapSemanticOverlap `json:"self_gap_semantic_overlaps,omitempty"`
	// FixDirection (AXIOM-V2 件1, user rulings 2026-07-18): the registry
	// repair-direction attribute of this row's token, published verbatim from
	// the ONE declaration (CausalTokenFixDirectionFor — never a second
	// implementation). Empty = unresolved (fail-open; absence never guesses).
	// ATTRIBUTE AXIS ONLY: no gate, ordinal, tier, sort or value lane reads
	// it (根因排序三护栏之②: 序数芯片本体零动,方向为属性轴).
	FixDirection string `json:"fix_direction,omitempty"`
	// CrossDirectionOverlaps (AXIOM-V2 件2, 公理 v2 跨方向重叠=合法共存全额 +
	// 互指披露「同段收益不叠加」, user ruling 2026-07-18): the typed
	// cross-direction overlap pair table — on a strict on-chain full seat,
	// one entry per SAME-thread SAME-board SAME-window wall-clock seat whose
	// registry fix direction DIFFERS and whose typed support-interval unions
	// intersect (∩ > 0). Each entry carries the exact interval-intersection
	// wall clock (口径词 同段重叠; identity pin overlap ≤ min of the two
	// support unions), the PARTNER's line envelope (the display resolves the
	// [E#] pointer verbatim — 宁漏勿假指), the partner's fix direction and
	// the partner's support-interval basis token. Entries are SYMMETRIC (both
	// seats of a pair list each other; the display renders both-or-neither).
	// Pure disclosure lane: 主值零动 — no gate, score or sort reads it.
	CrossDirectionOverlaps []RootCauseCrossDirectionOverlap `json:"cross_direction_overlaps,omitempty"`
	// CrossDirectionOverlapUndisclosed (AXIOM-V2 件3 与件2 闭环): partner
	// TYPE tokens of detected cross-direction overlaps whose mutual-pointer
	// carrier is ABSENT (the partner lacks a line envelope, or the bounded
	// roster capped the pair out) — the checker reports the un-pointable pair
	// by its honest type token instead of minting a fake [E#] (宁漏勿假指).
	// 立案素材 disclosure only.
	CrossDirectionOverlapUndisclosed []string `json:"cross_direction_overlap_undisclosed,omitempty"`
	// DirectionConservationExcess (AXIOM-V2 件3 守恒检查器, 纯披露道 —
	// §29.104.13 非致命不硬拦): stamped on EVERY member seat of a
	// (thread, direction) population whose Σ of per-seat support-interval
	// union lengths exceeds the physical window length (公理 v2 违宪形:
	// 同方向同线程重叠 ⇒ 恰一全额席). The finding is identical across the
	// member seats (display dedupes per direction); emission always proceeds
	// (永不拦发射) and every published value channel stays untouched.
	DirectionConservationExcess *RootCauseDirectionConservation `json:"direction_conservation_excess,omitempty"`
	// directionSupportIntervals/directionSupportBasis (unexported, AXIOM-V2):
	// the resolved per-segment support inventory this row entered the
	// direction population with (closed basis set, see
	// rootCauseItemDirectionSupport). Working stash for the display-side
	// overlap recomputation pins; never serialized.
	directionSupportIntervals []foldInterval
	directionSupportBasis     string
	// P3MCounterfactualValidMs / P3MCounterfactualInvalidMs /
	// P3MEdgeWitnessedMs / P3MDisposition (P3MEASURE-1, §29.169 user ruling
	// chain, 2026-07-20): the ONCHAIN-P3 stage-one SILENT two-dimension
	// measurement of an on-chain seat — see rank_p3_measure.go for the ruled
	// caliber (counterfactual edge-advance validity; typed counterexample
	// family ① periodic-pinned only, family ② honestly out of coverage) and
	// the closed disposition set. DISPLAY-ONLY AUDIT WIRE, model/user double-
	// invisible: the p3m_* rich-note keys are registered display_only with NO
	// parsing or rendering consumer, and NO gate, lane, ordinal, score, sort
	// or value channel may EVER read these fields (advisory-only red line,
	// supply_pressure 分离先例; promotion to any gate requires a new user
	// ruling). Invariants (pinned): µs(valid) + µs(invalid) == the seat's
	// anchor-window time exactly; edge_witnessed ≤ (+1µs float-dust guard slack) the seat's published
	// value; absent numbers on a "measured_*" disposition read as 0.
	P3MCounterfactualValidMs   float64 `json:"p3m_counterfactual_valid_ms,omitempty"`
	P3MCounterfactualInvalidMs float64 `json:"p3m_counterfactual_invalid_ms,omitempty"`
	P3MEdgeWitnessedMs         float64 `json:"p3m_edge_witnessed_ms,omitempty"`
	P3MDisposition             string  `json:"p3m_disposition,omitempty"`
	Summary                    string  `json:"summary,omitempty"`
}

// RootCauseCrossDirectionOverlap (AXIOM-V2 件2, 2026-07-18) is one partner
// entry of a strict on-chain full seat's cross-direction overlap disclosure:
// the exact typed interval-intersection wall clock plus the PARTNER seat's
// line envelope, fix direction and support-interval basis. The display
// resolves the [E#] pointer from the envelope (verbatim typed identity, never
// a name match) and renders the 互指句 on both seats or neither. Disclosure
// only; no value channel consumes it.
type RootCauseCrossDirectionOverlap struct {
	OverlapMs float64 `json:"overlap_ms"`
	LineStart int     `json:"line_start"`
	LineEnd   int     `json:"line_end"`
	// Direction is the PARTNER's registry fix direction token (never this
	// row's own — the reader of one entry sees the other side of the pair).
	Direction string `json:"direction"`
	// Basis is the PARTNER's support-interval basis token (the closed set of
	// rootCauseItemDirectionSupport).
	Basis string `json:"basis"`
}

// RootCauseDirectionConservation (AXIOM-V2 件3, 2026-07-18) is the
// direction-conservation violation finding: within one (thread, direction)
// strict on-chain full-seat population, the Σ of per-seat support-interval
// UNION lengths exceeded the physical window length. Pure disclosure /
// 立案素材 — never a gate.
type RootCauseDirectionConservation struct {
	Direction string  `json:"direction"`
	SumMs     float64 `json:"sum_ms"`
	WindowMs  float64 `json:"window_ms"`
	SeatCount int     `json:"seat_count"`
}

// RootCauseSelfGapSemanticOverlap (XLANE-2 件2, 裁定④ §29.104.17, 2026-07-17)
// is one per-partner entry of the self-gap seat's semantic-overlap disclosure:
// the exact typed interval-intersection wall clock (running ∩ member spans)
// plus the partner semantic seat's line envelope — the display resolves the
// [E#] pointer from the envelope (verbatim typed identity, never a name
// match). Disclosure only; no value channel consumes it.
type RootCauseSelfGapSemanticOverlap struct {
	OverlapMs float64 `json:"overlap_ms"`
	LineStart int     `json:"line_start"`
	LineEnd   int     `json:"line_end"`
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
// window-clipped spans of ONE (physical artifact, thread, semantic class,
// chain lane) folded into one contender. TotalMs is the complete selected-
// window member union
// (interval union; disjoint == Σ; union < Σ discloses via SumMs). For an
// on-chain family, ProjectedImpactMs is the narrower exact intersection union
// that alone participates in causal ranking/effective attribution. Built
// exclusively by FoldSemanticSpanFamilies — the ONE fold point
// both consumers (rank minting and the typed observation channel) read, so the
// two faces can never publish two different family shapes (§24.12 dim-A
// mandate: two consumers, one function). stats.TraceSpans stays untouched —
// the family is a VIEW, never a rewrite of the span inventory.
type SemanticSpanFamily struct {
	Thread        ThreadRef `json:"thread"`
	SemanticClass string    `json:"semantic_class,omitempty"`
	// SourcePath is part of the family identity. Two attached artifacts may
	// reuse thread ids, span names and timestamps; they must never cross-fold.
	SourcePath string `json:"source_path,omitempty"`
	// OnChain is the family's 道别 (chain lane), decided per member by the
	// SAME overlap predicate as the DCS E2 mint-time lane (same-thread chain
	// node / causal-impact window overlap — thread membership alone never
	// flips a lane). Members of one (thread,class) that split lanes form TWO
	// families: on-chain and background never cross-merge (§24.7.1 道别键).
	//
	// SELF-SEM (§29.61.1, 2026-07-13): OnChain=true now has a SECOND typed
	// basis — OnChainBasis=self_deterministic_span (the analysis target's own
	// deterministic spans, zero chain-window overlap). The three lanes never
	// cross-merge (three-valued fold key): a self family carries NO
	// Projected*/DominantState* intersection fields (there is no overlap to
	// project — fabricating one would be the §23.1 伪造重叠 shape) and its
	// participation caliber is the complete window-projection TotalMs.
	OnChainBasis  string  `json:"on_chain_basis,omitempty"`
	OnChain       bool    `json:"on_chain,omitempty"`
	ChainDepth    int     `json:"chain_depth,omitempty"`
	ChainBranch   int     `json:"chain_branch,omitempty"`
	DominantState string  `json:"dominant_state,omitempty"`
	TotalMs       float64 `json:"total_ms"`
	// ProjectedImpactMs is set only for an on-chain family and is the exact
	// interval union of every member's intersections with every same-thread
	// chain node/impact window. It is the ONLY family value admitted to causal
	// ranking/effective attribution. TotalMs remains the complete selected-
	// window member union for lossless disclosure and off-chain behavior.
	ProjectedImpactMs float64 `json:"projected_impact_ms,omitempty"`
	ProjectedStartTs  float64 `json:"projected_start_ts,omitempty"`
	ProjectedEndTs    float64 `json:"projected_end_ts,omitempty"`
	// DominantStateImpactMs is the exact union attributable to the selected
	// dominant chain state. It may be smaller than ProjectedImpactMs when the
	// family intersects multiple chain states; state decomposition must never
	// stamp the whole cross-state projection into one state bucket.
	DominantStateImpactMs float64 `json:"dominant_state_impact_ms,omitempty"`
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
	// EdgeAnchor* (R3-IMPL, §29.88.1 user ruling 2026-07-14): set only on a
	// RootCauseOnChainBasisHostWakeupEdge family. BoundaryTs is the latest
	// in-window credential edge (the bisection boundary); Via is the typed
	// credential inventory word (HostWakeupEdgeAnchorVia* closed set). The
	// Remainder pair is the post-boundary (边后) member-union extent — the
	// mint loop clones the ◇ remainder seat from it; zero-width when every
	// member lies fully pre-edge. ProjectedImpactMs on this lane carries the
	// pre-edge in-window union (the participation value, 边前段窗内投影).
	EdgeAnchorBoundaryTs       float64 `json:"edge_anchor_boundary_ts,omitempty"`
	EdgeAnchorVia              string  `json:"edge_anchor_via,omitempty"`
	EdgeAnchorRemainderMs      float64 `json:"edge_anchor_remainder_ms,omitempty"`
	EdgeAnchorRemainderStartTs float64 `json:"edge_anchor_remainder_start_ts,omitempty"`
	EdgeAnchorRemainderEndTs   float64 `json:"edge_anchor_remainder_end_ts,omitempty"`
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
	// TargetWindowStates (§29.27 ruling ②, COV-4 2026-07-11): the focused
	// thread's full-window four-state wall-clock partition (io_wait = the
	// typed IO refinement inside the D-state wall clock) + the deterministic
	// running intersection. nil when the target timeline has no measurable
	// intervals. The display renders the account ONLY when Σ(states) balances
	// against the window (不平衡拒渲不造数).
	TargetWindowStates *TargetWindowStateAccount `json:"target_window_states,omitempty"`
	Caveats            []string                  `json:"caveats,omitempty"`

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
	Target              ThreadRef              `json:"target,omitempty"`
	TargetScope         string                 `json:"target_scope,omitempty"`
	ProcessID           int                    `json:"process_id,omitempty"`
	MembershipAuthority string                 `json:"membership_authority,omitempty"`
	TargetRoleAuthority *FrameRoleAuthority    `json:"target_role_authority,omitempty"`
	Source              string                 `json:"source,omitempty"`
	Confidence          float64                `json:"confidence,omitempty"`
	Window              TimeWindow             `json:"window,omitempty"`
	WindowSource        string                 `json:"window_source,omitempty"`
	SelectedFrame       *FrameTargetCandidate  `json:"selected_frame,omitempty"`
	Candidates          []FrameTargetCandidate `json:"candidates,omitempty"`
	Caveats             []string               `json:"caveats,omitempty"`
}

type FrameTargetCandidate struct {
	Thread              ThreadRef           `json:"thread,omitempty"`
	TargetScope         string              `json:"target_scope,omitempty"`
	ProcessID           int                 `json:"process_id,omitempty"`
	MembershipAuthority string              `json:"membership_authority,omitempty"`
	Role                string              `json:"role,omitempty"`
	RoleAuthority       *FrameRoleAuthority `json:"role_authority,omitempty"`
	Phase               string              `json:"phase,omitempty"`
	Name                string              `json:"name,omitempty"`
	FrameID             string              `json:"frame_id,omitempty"`
	Window              TimeWindow          `json:"window,omitempty"`
	StartLine           int                 `json:"start_line,omitempty"`
	EndLine             int                 `json:"end_line,omitempty"`
	Score               float64             `json:"score,omitempty"`
	Reason              string              `json:"reason,omitempty"`
}

// FrameRoleAuthority separates a marker/item label from a proven thread role.
// Kind=thread_role is the only authority that may name a selected thread UI or
// render-service. Frame-marker roles such as expected/actual/jank describe the
// item, not the owning thread.
type FrameRoleAuthority struct {
	Role       string  `json:"role,omitempty"`
	Kind       string  `json:"kind,omitempty"`
	Source     string  `json:"source,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
	Evidence   string  `json:"evidence,omitempty"`
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
	Index                   int                 `json:"index"`
	Thread                  ThreadRef           `json:"thread,omitempty"`
	TargetScope             string              `json:"target_scope,omitempty"`
	ProcessID               int                 `json:"process_id,omitempty"`
	ProcessMembershipSource string              `json:"process_membership_source,omitempty"`
	Phase                   string              `json:"phase,omitempty"`
	Role                    string              `json:"role,omitempty"`
	RoleAuthority           *FrameRoleAuthority `json:"role_authority,omitempty"`
	Name                    string              `json:"name,omitempty"`
	FrameID                 string              `json:"frame_id,omitempty"`
	StartTs                 float64             `json:"start_ts,omitempty"`
	EndTs                   float64             `json:"end_ts,omitempty"`
	DurationMs              float64             `json:"duration_ms,omitempty"`
	StartLine               int                 `json:"start_line,omitempty"`
	EndLine                 int                 `json:"end_line,omitempty"`
	Summary                 string              `json:"summary,omitempty"`
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
	Thread                  ThreadRef `json:"thread,omitempty"`
	TargetScope             string    `json:"target_scope,omitempty"`
	ProcessID               int       `json:"process_id,omitempty"`
	ProcessMembershipSource string    `json:"process_membership_source,omitempty"`
	Phase                   string    `json:"phase,omitempty"`
	Name                    string    `json:"name,omitempty"`
	StartTs                 float64   `json:"start_ts,omitempty"`
	EndTs                   float64   `json:"end_ts,omitempty"`
	DurationMs              float64   `json:"duration_ms,omitempty"`
	StartLine               int       `json:"start_line,omitempty"`
	EndLine                 int       `json:"end_line,omitempty"`
	Summary                 string    `json:"summary,omitempty"`
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
	// OwnerTidPresence (LOCKNS-FIX 修补 件A, 冷读 P2-F1+P3-F7 同族,
	// 2026-07-16): the typed presence verdict of the payload owner tid on a
	// rung-①-diverged row — OwnerTidPresenceAbsent / OwnerTidPresenceCollision
	// (G1 ns-divergent numeric collision) / OwnerTidPresenceCommMismatch (tid
	// present, payload owner comm never observed on it). Minted from the
	// engine's existing determination bits at the divergence point, never a
	// new heuristic; empty on rung-①-resolved rows and legacy artifacts. The
	// detail 持有者来历 presence clause forks on it so the legacy "not present
	// in this trace" claim never rides a shape where the tid IS present
	// (absence fail-opens to the legacy sentence byte-identically).
	OwnerTidPresence string `json:"owner_tid_presence,omitempty"`
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
	//
	// G10-EN 根修 (QH2-A, 2026-07-14): the string stays the zh mint
	// (byte-frozen legacy wording — the audit verbatim lane and the zh 明细
	// face consume it unchanged); HolderSelfContradictionParts carries the
	// typed components so the zh/EN display lanes each word their own
	// sentence (types.TraceHolderSelfContradictionWitness.WitnessText) and
	// the EN faces stop embedding the zh body verbatim. nil = guard never
	// fired.
	HolderSelfContradiction      string                                     `json:"holder_self_contradiction,omitempty"`
	HolderSelfContradictionParts *types.TraceHolderSelfContradictionWitness `json:"holder_self_contradiction_parts,omitempty"`
	// WaitObject (P0-E2a, §10 A2): the blocking span's own name, published as
	// the wait object for payload-less blocking spans so the row can at least
	// say what it was blocked on when no structured owner was parseable.
	WaitObject string `json:"wait_object,omitempty"`
	// OwnerKeyUnregistered (LOCKNS-FIX 件3, §29.104.12, 2026-07-16): the span
	// name speaks lock-owner vocabulary (word-boundary `owner`) but matched NO
	// registered lock-contention morphology — the row fail-opened to the
	// payload-less blocking lane (BlockingKind stays empty, no holder is ever
	// minted from an unregistered shape, the value rides the XERR1-FIX basis
	// discipline untouched) and this typed marker drives the soft
	// 「owner 未解析(形态未注册)」 disclosure on the Summary and detail faces.
	// NOISY detection signal → disclosure only, never a gate (§1 red line).
	OwnerKeyUnregistered bool `json:"owner_key_unregistered,omitempty"`
	// BlockingValueBasis (XERR1-FIX 件1, §29.104.3/.4, 2026-07-15; XERR1-EXT
	// 裁定⑤ §29.104.17, 2026-07-16): the typed value basis of a blocking_span
	// row — BOTH payload lanes since XERR1-EXT. The customer E1 lesion: the
	// row's published value was the traversal span's window-envelope
	// projection (199.992ms = 100% of the window) dressed as 「阻塞等待」
	// while the same report's four-state account said running 54% — a span
	// envelope CONTAINS running time and is not a blocking wait.
	//
	//	BlockingValueBasisWaitSegments — DurationMs converged to the WAITER's
	//	    Σ(sleep+D-state+io_wait) segments inside span∩window (runnable is
	//	    scheduling pressure, never blocking-wait semantics); the envelope
	//	    is preserved on SpanEnvelopeMs. Payload-typed rows window the Σ on
	//	    the fold value-winner interval (件A 值胜出区间纪律).
	//	BlockingValueBasisSpanEnvelope — convergence impossible (no waiter
	//	    timeline inside the value interval, or — payload lanes only,
	//	    theoretically unreachable — no value-winner interval): DurationMs
	//	    stays the envelope. On payload-less rows the DISPLAY word family
	//	    must say 「span 包络(含运行)」 instead of 「阻塞等待」 (件2 词面
	//	    退路); payload-typed rows keep the lock family words and let the
	//	    值口径 detail line carry the honest envelope claim.
	//
	// Empty only on legacy wire artifacts (fail-open: every display fork
	// keeps the legacy wording byte-identically).
	BlockingValueBasis string `json:"blocking_value_basis,omitempty"`
	// WaitSegmentMs / WaitSleepMs / WaitDStateMs / WaitIOWaitMs (件1): the
	// waiter's proven wait-segment account inside span∩window — the value
	// DurationMs converges to on the wait_segments basis, plus its per-state
	// decomposition (WaitSleepMs>0 additionally gates the sleep-seat 互指
	// disclosure — the same physical sleep time also lives in the thread's
	// window sleep account, and the two rows must cross-reference instead of
	// inviting addition).
	WaitSegmentMs float64 `json:"wait_segment_ms,omitempty"`
	WaitSleepMs   float64 `json:"wait_sleep_ms,omitempty"`
	WaitDStateMs  float64 `json:"wait_d_state_ms,omitempty"`
	WaitIOWaitMs  float64 `json:"wait_io_wait_ms,omitempty"`
	// SpanEnvelopeMs (件1): the span's in-window envelope projection — the
	// pre-convergence published value, preserved as disclosure (never the row
	// value on the wait_segments basis). Deliberately NOT ActualDurationMs:
	// that field's absence is the precise "not window-clipped" signal for the
	// physical B/E extent and must not be overloaded (容差/语义禁跨借用).
	SpanEnvelopeMs float64 `json:"span_envelope_ms,omitempty"`
	// WaitBudgetExceeded (XERR1-FIX 件3 sanity invariant): the row's blocking
	// claim (span envelope) EXCEEDS the waiter's own non-running total over
	// the same span∩window interval — arithmetically impossible for a true
	// blocking wait, so the display forks the word face and adds the ⚠
	// disclosure 「span 包络 X > 窗内非 running Y:含 running Z,非阻塞等待段」.
	// Value and budget ride the row (禁 clamp 禁硬拒 — the observation always
	// publishes); the verdict is minted ONLY when the waiter's state account
	// fully covers the span window (F-2 同基: partial coverage undercounts
	// the budget and would false-fire — 禁判 keeps the marker precise).
	WaitBudgetExceeded     bool    `json:"wait_budget_exceeded,omitempty"`
	WaitBudgetNonRunningMs float64 `json:"wait_budget_non_running_ms,omitempty"`
	WaitBudgetRunningMs    float64 `json:"wait_budget_running_ms,omitempty"`
	// WaitCoveragePartial + WaitAccountCoveredMs (XERR1-FIX 修补 件F, 冷读
	// P3-3, 2026-07-16): the waiter's state account did NOT tile the whole
	// span∩window interval (the same coverage gap that 禁判s the 件3 budget
	// above), so the converged wait_segments value is a PROVEN LOWER BOUND —
	// the typed marker + the covered total drive the zh/EN 覆盖核查 display
	// line and the Summary sentence. wait_segments basis only (both payload
	// lanes since XERR1-EXT); never minted on span_envelope rows.
	WaitCoveragePartial  bool    `json:"wait_coverage_partial,omitempty"`
	WaitAccountCoveredMs float64 `json:"wait_account_covered_ms,omitempty"`
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
	PeerChain         *PeerChainStep `json:"peer_chain,omitempty"`
	Flags             string         `json:"flags,omitempty"`
	Oneway            *bool          `json:"oneway,omitempty"`
	SyncLike          *bool          `json:"sync_like,omitempty"`
	BlockingCandidate *bool          `json:"blocking_candidate,omitempty"`
	ChainRelevance    string         `json:"chain_relevance,omitempty"`
	// ChainCredentialLaneDemoted (RNB-1 B-4, §29.88 R4, 2026-07-14): the
	// interval-less chain-lane D/IO VIEW row of a non-target chain pid whose
	// census family account proves ZERO anchored credential (D-IO decision
	// present ∧ census-anchored Σ ≤ µs tolerance — the census IS the complete
	// in-window ledger, so no D/IO row of that pid can sit before a typed
	// causal edge). The row rides the ◇ adjacent channel with its published
	// value untouched (值零动,通道位归位; the customer E9/E10 witness shape:
	// two D-state view rows on the ⛓ channel while the same pid's bipartition
	// sentence said 锚定0.000). Pids with ANY anchored credential keep the
	// legacy lane byte-identically (tieba 60555 negative control).
	ChainCredentialLaneDemoted bool `json:"chain_credential_lane_demoted,omitempty"`
	// ChainCredentialSegments / ChainCredentialSegmentDisjoint /
	// ChainCredentialEnvelopeLevel (HULL-CRED, §29.104 终判③ / §29.99 裁定池,
	// 2026-07-17): the keep-⛓ per-segment credential family of the chain-lane
	// D/IO VIEW verdict (criticalBlockingDioRowCredentialVerdict).
	//
	//   - ChainCredentialSegments carries the row's typed evidence segment
	//     inventory ("start..end" seconds, chronological), minted from the
	//     D/IO ledger's exact clamped close-site segments (never the
	//     reconStartTs/reconEndTs hull — hull endpoints are NOISE and must
	//     never pose as segments). Published ONLY on the segment-adjudicated
	//     verdicts: the ≥1-true-intersection keep and the all-disjoint
	//     demotion (the claim and its proof travel on one row). Capped by
	//     CriticalBlockingCredentialSegmentCap.
	//   - ChainCredentialSegmentDisjoint marks the NEW demotion form: the
	//     hull intersected the anchor windows but EVERY real segment lies in
	//     the hull's occurrence gaps (the pre-fix fake-credential keep-⛓
	//     shape). Always rides beside ChainCredentialLaneDemoted=true and the
	//     published segment inventory; values untouched (值零动). Minted from
	//     COMPLETE inventories only — a truncated prefix can never prove
	//     absence (缺证≠证无, ONCHAIN-FIX-2 件3).
	//   - ChainCredentialEnvelopeLevel marks the honest fail-open keep: the
	//     row KEEPS the ⛓ lane on the conservative envelope/census verdict
	//     (segment inventory absent, or a truncated prefix that proved no
	//     intersection — 部分清单不交≠证无) and wears the 「交集证明(包络级)」
	//     disclosure word instead of a per-segment credential. Never set on a
	//     demoted row.
	//   - ChainCredentialSegmentsTruncated (ONCHAIN-FIX-2 件3, Q6 已追认,
	//     2026-07-18): the published inventory is the ledger's immutable
	//     checked PREFIX of a beyond-cap group (dioIntervalsOverflow latched
	//     at the source) — a proven LOWER BOUND, not the complete account.
	//     Rides ONLY beside a non-empty ChainCredentialSegments on the
	//     ≥1-true-intersection keep (the one arm allowed to adjudicate on a
	//     prefix); the display adds the 「凭证清单不完整,实际锚定不小于所证」
	//     wording (下界 caliber family — the proven intersection can only
	//     grow with the uncollected segments, never shrink).
	ChainCredentialSegments          []string `json:"chain_credential_segments,omitempty"`
	ChainCredentialSegmentDisjoint   bool     `json:"chain_credential_segment_disjoint,omitempty"`
	ChainCredentialEnvelopeLevel     bool     `json:"chain_credential_envelope_level,omitempty"`
	ChainCredentialSegmentsTruncated bool     `json:"chain_credential_segments_truncated,omitempty"`
	// ChainIdentityInheritance (ONCHAIN-FIX-1 件1, 2026-07-18): mirror of
	// RootCauseRankItem.ChainIdentityInheritance — the interval-less same-pid
	// fail-open admission record. The D/IO VIEW rows (DStateTop/IOWaitTop
	// publish no StartTs/EndTs on the wire) were THE main fabricated-overlap
	// face; the lane keeps, OverlapMs is honest zero, and this bit drives the
	// 「成员继承(链窗级,无区间凭证)」 disclosure. Cleared by the HULL-CRED
	// adjudication (dioDecisions present): adjudicated rows speak the stronger
	// credential vocabulary (per-segment / envelope / demote words). Never set
	// on the analysis target's own rows (R8 self-causality).
	ChainIdentityInheritance bool `json:"chain_identity_inheritance,omitempty"`
	// OnChainBasis (SELF-ALL, §29.61.2 2026-07-13): same closed set and
	// semantics as RootCauseRankItem.OnChainBasis — non-empty ONLY when this
	// candidate's on-chain relevance was granted by the typed self wall-clock
	// predicate ("self_wall_clock_interval") instead of chain-window overlap.
	// The critical_blocking face is a witness feeder for the ◇/self display
	// stanzas, so its lane verdict and proof basis must ride the same wire the
	// rank face uses (one 道别 predicate, two consumers).
	OnChainBasis       string     `json:"on_chain_basis,omitempty"`
	OverlapMs          float64    `json:"overlap_ms,omitempty"`
	EdgeCount          int        `json:"edge_count,omitempty"`
	NearestChainThread ThreadRef  `json:"nearest_chain_thread,omitempty"`
	NearestChainWindow TimeWindow `json:"nearest_chain_window,omitempty"`
	DurationMs         float64    `json:"duration_ms,omitempty"`
	StartTs            float64    `json:"start_ts,omitempty"`
	EndTs              float64    `json:"end_ts,omitempty"`
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
	AbsorbedByRankFamily bool `json:"absorbed_by_rank_family,omitempty"`
	// reconStartTs/reconEndTs (CASE-1 gap (a), §29.52 → v5 P1 批, 2026-07-13;
	// 修复轮 h1 ∿ 回归). Unexported: engine-internal recon identity only —
	// the D/IO rows' per-(thread,cpu) group interval (same-source floats
	// copied from the ThreadDuration the rank family also copies). It is a
	// segment HULL, deliberately NOT the published StartTs/EndTs (a hull is
	// not an occurrence segment, and publishing it made the projection's
	// span-overlap fold arms fire on hull noise). Read exclusively by
	// reconcileCriticalBlockingWithRankFamilies via
	// criticalBlockingReconInterval.
	reconStartTs float64
	reconEndTs   float64
	// credentialSegments (HULL-CRED, §29.104 终判③, 2026-07-17). Unexported:
	// engine-internal carriage of the D/IO ledger group's exact clamped
	// evidence segments (ThreadDuration.dioIntervals — the same close-site
	// floats the hull above envelopes). The keep-⛓ verdict intersects THESE
	// per segment against the anchor windows; the validated inventory is
	// re-rendered onto the exported ChainCredentialSegments wire field only
	// on the segment-adjudicated verdict arms. Nil = legacy ledger shapes
	// (the envelope tier). ONCHAIN-FIX-2 件3: credentialSegmentsTruncated
	// mirrors the donor ledger's dioIntervalsOverflow latch — true means the
	// carried list is the checked PREFIX of a beyond-cap group (proven lower
	// bound; partial evidence proves presence, never absence).
	credentialSegments          []foldInterval
	credentialSegmentsTruncated bool
	// proofRefined/proofCaller (修复轮二 件B, 2026-07-13). Unexported:
	// engine-internal per-GROUP proof carriage for the D/IO chain-lane
	// candidates (same accessors as the ThreadDuration donor) — the display
	// merged-word gate consumes them through the observation notes, so the
	// refined 「D-state」 word survives the rank-family-less dispatch shapes
	// whose only D seats are these rows.
	proofRefined       bool
	proofCaller        string
	AbsorbedIntoFamily string  `json:"absorbed_into,omitempty"`
	LineStart          int     `json:"line_start,omitempty"`
	LineEnd            int     `json:"line_end,omitempty"`
	Confidence         float64 `json:"confidence,omitempty"`
	Summary            string  `json:"summary,omitempty"`
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
	// PacingIdles: P9 arm-c frame-pacing idle segments (§29.42 案1) — sleep
	// segments the binder write-off arms rejected that read as normal
	// frame-cadence idle. Own semantic lane; never root-cause contenders.
	PacingIdles  []PacingIdleSummary `json:"pacing_idles,omitempty"`
	RootEvidence []RootEvidence      `json:"root_evidence,omitempty"`
	// WakeupEdgeCensus (WAKE-CENSUS, ledger §29.58 立案, 2026-07-13): the
	// bounded per-(waker → wakee) census folded over this result's FULL edge
	// set BEFORE any publication row cap (禁截断库存二次聚合 — the typed
	// per-family row cap bounds the per-edge observation rows, so per-pair
	// counts re-derived from that face are silent lower bounds; PRC-F1
	// witness: the model then invented「OS_IPC_14_34911 ×4」for a pair whose
	// only raw edge ran the OPPOSITE direction). Rows order deterministically
	// (count desc + typed tie keys) and the pair cap discloses its overflow
	// explicitly — blocked_reason_census 同构 (§29.57 件1).
	WakeupEdgeCensus []WakeupEdgeCensusPair `json:"wakeup_edge_census,omitempty"`
	// WakeupEdgeCensusOverflowPairs / ...OverflowEdges disclose the census
	// pair-cap trim: how many distinct pairs, carrying how many deduplicated
	// edges, exist beyond the listed census rows. 0/0 ⇔ the census pair
	// enumeration is complete for this result.
	WakeupEdgeCensusOverflowPairs int `json:"wakeup_edge_census_overflow_pairs,omitempty"`
	WakeupEdgeCensusOverflowEdges int `json:"wakeup_edge_census_overflow_edges,omitempty"`
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
	Hops []ChainViaHop `json:"hops,omitempty"`
	// PathComplete (§29.183 G5, additive split of the ON verdict — RN-14's
	// OnChain node-set-membership binding stays untouched): true when Hops is
	// a complete time-consistent (non-decreasing wakeup_ts) hop sequence from
	// the via thread to the target; false when the via thread is on the
	// chain's node set but no such complete sequence exists — Hops is then
	// the reachable prefix only (viaMonotonicHops truncation arm). Always
	// false on the NOT arm (OnChain=false: no wakeup path at all), so the
	// formerly-prose contradiction shape OnChain=true ∧ PathComplete=false is
	// typed-discernible without re-parsing Summary.
	PathComplete bool   `json:"path_complete"`
	Summary      string `json:"summary,omitempty"`
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

type BinderCallSemantics string

const (
	BinderCallSemanticsSyncRequest   BinderCallSemantics = "sync_request"
	BinderCallSemanticsOnewayRequest BinderCallSemantics = "oneway_request"
	BinderCallSemanticsReply         BinderCallSemantics = "reply"
	BinderCallSemanticsUnknown       BinderCallSemantics = "unknown"
)

type BinderReceiverSource string

const (
	BinderReceiverSourceMatchedReceive BinderReceiverSource = "matched_receive"
	BinderReceiverSourceDestHint       BinderReceiverSource = "dest_hint"
	BinderReceiverSourceUnresolved     BinderReceiverSource = "unresolved"
)

type IPCEdge struct {
	TransactionID        int                  `json:"transaction_id,omitempty"`
	Sender               ThreadRef            `json:"sender"`
	Receiver             ThreadRef            `json:"receiver,omitempty"`
	DestProc             int                  `json:"dest_proc,omitempty"`
	DestThread           int                  `json:"dest_thread,omitempty"`
	SendTs               float64              `json:"send_ts,omitempty"`
	ReceiveTs            float64              `json:"receive_ts,omitempty"`
	SendLine             int                  `json:"send_line,omitempty"`
	ReceiveLine          int                  `json:"receive_line,omitempty"`
	Reply                int                  `json:"reply,omitempty"`
	Flags                string               `json:"flags,omitempty"`
	Code                 string               `json:"code,omitempty"`
	CallSemantics        BinderCallSemantics  `json:"call_semantics"`
	DestinationHintKnown bool                 `json:"destination_hint_known"`
	ReplyKnown           bool                 `json:"reply_known"`
	FlagsKnown           bool                 `json:"flags_known"`
	CodeKnown            bool                 `json:"code_known"`
	ReceiverSource       BinderReceiverSource `json:"receiver_source"`
	Oneway               bool                 `json:"oneway"`
	SyncLike             bool                 `json:"sync_like"`
	BlockingCandidate    bool                 `json:"blocking_candidate"`
	// Interface is the userspace binder wrapper interface joined from the
	// enclosing `transact[Interface:code]` trace-mark span on the sender
	// thread (C2, 2026-07-03) — a verbatim same-thread span-name join,
	// empty when no wrapper span encloses the send.
	Interface  string   `json:"interface,omitempty"`
	LatencyMs  float64  `json:"latency_ms,omitempty"`
	Confidence float64  `json:"confidence,omitempty"`
	Caveats    []string `json:"caveats,omitempty"`

	// physicalSource is the exact source-scoped pairing authority. It is kept
	// private so cross-artifact reply write-off cannot leak absolute paths into
	// the public report schema.
	physicalSource string `json:"-"`
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
	// interesting target interval; the visited map forbids revisits along one
	// path). 0 = legacy fixture with no branch identity. Since CHAIN-BUDGET
	// (2026-07-18) a branch is a TREE whose primary spine is the guaranteed
	// top-1 recursion (SegmentOrdinal 0 on every spine hop) and whose side
	// chains are budget-gated extra segment expansions (SegmentOrdinal >= 2).
	// The publication layer serializes ONE path per branch — the primary
	// spine — instead of the retired cross-branch flattened walk (§22.1);
	// side chains are disclosed by the side_chains note and publish their
	// edges individually.
	Branch int `json:"branch,omitempty"`
	// SegmentOrdinal is the CHAIN-BUDGET expansion-lane identity: the 1-based
	// value-order position of the PARENT-node sleep segment this node was
	// expanded from. 0 ⇔ the guaranteed top-1 lane (and every legacy result —
	// wire-compatible absence); values start at 2 (top-2, top-3, …) for
	// budget-gated extra expansions. Never a ranking — segment identity only.
	SegmentOrdinal int `json:"segment_ordinal,omitempty"`
}

type WakeupEdge struct {
	From                        string    `json:"from"`
	To                          string    `json:"to"`
	Waker                       ThreadRef `json:"waker"`
	Wakee                       ThreadRef `json:"wakee"`
	WakeupTs                    float64   `json:"wakeup_ts"`
	WakeupLine                  int       `json:"wakeup_line"`
	LatencyMs                   float64   `json:"latency_ms,omitempty"`
	WakerPriority               int       `json:"waker_priority,omitempty"`
	WakerPriorityClass          string    `json:"waker_priority_class,omitempty"`
	WakerPrioritySource         string    `json:"waker_priority_source,omitempty"`
	WakerPriorityArtifactSource string    `json:"waker_priority_artifact_source,omitempty"`
	WakeePriority               int       `json:"wakee_priority,omitempty"`
	WakeePriorityClass          string    `json:"wakee_priority_class,omitempty"`
	WakeePrioritySource         string    `json:"wakee_priority_source,omitempty"`
	WakeePriorityArtifactSource string    `json:"wakee_priority_artifact_source,omitempty"`
	WakeePriorityAuthority      string    `json:"wakee_priority_authority,omitempty"`
	PriorityRelation            string    `json:"priority_relation,omitempty"`
	PriorityRelationCaliber     string    `json:"priority_relation_caliber,omitempty"`
	PriorityInversionCandidate  bool      `json:"priority_inversion_candidate,omitempty"`
	EvidenceLine                int       `json:"evidence_line,omitempty"`
	// Branch mirrors the owning branch ordinal of the edge's From/To nodes
	// (they share one branch by construction — edges never cross branches).
	// 0 = legacy fixture (P0-E CHAIN-PATH, ledger §22.1).
	Branch int `json:"branch,omitempty"`
	// SegmentOrdinal mirrors the child node's CHAIN-BUDGET expansion-lane
	// identity (see ChainNode.SegmentOrdinal): 0 = guaranteed top-1 lane /
	// legacy, >= 2 = budget-gated extra segment expansion. The wakeup edge
	// itself is the SAME hard per-hop sched_wakeup credential on both lanes.
	SegmentOrdinal int `json:"segment_ordinal,omitempty"`
}

// WakeupEdgeCensusPair is ONE (waker → wakee) pair's window-total account for
// a wakeup_chain result (WAKE-CENSUS §29.58 + WAKE-CENSUS-D 2A 换源 §29.58.4,
// 2026-07-13). Count counts every raw sched_wakeup row waking this wakee
// inside the result's window (the wakee population is the chain-thread set:
// target ∪ chain nodes; counting is chain-INDEPENDENT — D exits and off-path
// S exits count alike, the §29.58.4 structural absence is closed).
// FirstTs/LastTs bound the pair's counted wakeup timestamps. The direction is
// the sched_wakeup row's own waker → wakee direction, verbatim — never
// inferred, never reversed.
//
// EVOLUTION RECORD (2A): pre-2A the Count folded the engine's FULL edge set —
// a structurally partial supply (edges mint only on the S-sleep expansion
// arm; the donghu gpu-token ×12 D-exit wakeups were invisible). The wire keys
// are unchanged; only the caliber strengthened (observed-edges → window-total
// raw rows).
type WakeupEdgeCensusPair struct {
	Waker ThreadRef `json:"waker"`
	Wakee ThreadRef `json:"wakee"`
	Count int       `json:"count"`
	// SleepExitCount/DExitCount/OtherExitCount split Count by the scheduler
	// state the wakee LEFT at each counted wakeup (typed classifier over the
	// wakee's own timeline: S-sleep / D-or-IO-wait / everything else incl.
	// timeline-unclassifiable — absence never guesses). The three columns
	// partition Count exactly (双加恒等式); split columns on ONE pair row,
	// never split rows (同 pair 两行会被再演算互减). Measurement-face counts
	// ONLY — the D-state CAUSAL lane stays with sched_blocked_reason (双重归因
	// 防护; the census never claims why a D wait ended, only who woke it).
	SleepExitCount int     `json:"sleep_exit_count,omitempty"`
	DExitCount     int     `json:"d_exit_count,omitempty"`
	OtherExitCount int     `json:"other_or_unclassified_exit_count,omitempty"`
	FirstTs        float64 `json:"first_ts,omitempty"`
	LastTs         float64 `json:"last_ts,omitempty"`
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
	ChainBranch                         int      `json:"chain_branch,omitempty"`
	OnChain                             bool     `json:"on_chain,omitempty"`
	DominantState                       string   `json:"dominant_state,omitempty"`
	DominantImpactMs                    float64  `json:"dominant_impact_ms,omitempty"`
	ProjectedImpactMs                   float64  `json:"projected_impact_ms,omitempty"`
	TotalMs                             float64  `json:"total_ms,omitempty"`
	ProjectedTotalMs                    float64  `json:"projected_total_ms,omitempty"`
	ActualImpactMs                      float64  `json:"actual_impact_ms,omitempty"`
	ActualTotalMs                       float64  `json:"actual_total_ms,omitempty"`
	RunningMs                           float64  `json:"running_ms,omitempty"`
	RunnableMs                          float64  `json:"runnable_ms,omitempty"`
	SleepMs                             float64  `json:"sleep_ms,omitempty"`
	DStateMs                            float64  `json:"d_state_ms,omitempty"`
	IOWaitMs                            float64  `json:"io_wait_ms,omitempty"`
	ActualRunningMs                     float64  `json:"actual_running_ms,omitempty"`
	ActualRunnableMs                    float64  `json:"actual_runnable_ms,omitempty"`
	ActualSleepMs                       float64  `json:"actual_sleep_ms,omitempty"`
	ActualDStateMs                      float64  `json:"actual_d_state_ms,omitempty"`
	ActualIOWaitMs                      float64  `json:"actual_io_wait_ms,omitempty"`
	FragmentCount                       int      `json:"fragment_count,omitempty"`
	StateSwitches                       int      `json:"state_switches,omitempty"`
	MaxSegmentMs                        float64  `json:"max_segment_ms,omitempty"`
	P95SegmentMs                        float64  `json:"p95_segment_ms,omitempty"`
	TargetBlockedMs                     float64  `json:"target_blocked_ms,omitempty"`
	LineStart                           int      `json:"line_start,omitempty"`
	LineEnd                             int      `json:"line_end,omitempty"`
	Priority                            int      `json:"priority,omitempty"`
	PriorityClass                       string   `json:"priority_class,omitempty"`
	PrioritySource                      string   `json:"priority_source,omitempty"`
	PriorityArtifactSource              string   `json:"priority_artifact_source,omitempty"`
	TargetPriority                      int      `json:"target_priority,omitempty"`
	TargetPriorityClass                 string   `json:"target_priority_class,omitempty"`
	TargetPrioritySource                string   `json:"target_priority_source,omitempty"`
	TargetPriorityArtifactSource        string   `json:"target_priority_artifact_source,omitempty"`
	PriorityRelation                    string   `json:"priority_relation,omitempty"`
	PriorityRelationCaliber             string   `json:"priority_relation_caliber,omitempty"`
	PriorityRelationProvenLowerMs       float64  `json:"priority_relation_proven_lower_ms,omitempty"`
	PriorityRelationUnknownOrNonLowerMs float64  `json:"priority_relation_unknown_or_nonlower_ms,omitempty"`
	PriorityRelationArtifactSources     []string `json:"priority_relation_artifact_sources,omitempty"`
	PriorityInversionCandidate          bool     `json:"priority_inversion_candidate,omitempty"`
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
	// GatedCapabilityFreqOnlyReason (DISPHYG-3 件7): the gated freq_only
	// cause token twin — see the WakeupCausalImpact field doc.
	GatedCapabilityFreqOnlyReason string `json:"gated_capability_freq_only_reason,omitempty"`
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
	// WakeupAggregateCaliberOverlapSafe means occurrence windows overlapped:
	// full-window metrics use the exact interval union, while partial metrics
	// conservatively use a MAX per overlap-connected cohort. This prevents a
	// multi-branch projection of one physical segment from being summed twice.
	WakeupAggregateCaliberOverlapSafe = "interval_union_or_max_overlap_fallback"
)

type WakeupCausalAggregate struct {
	Thread ThreadRef `json:"thread"`
	Path   string    `json:"path,omitempty"`
	// ChainDepth is the MIN member depth; ChainBranch is the members' shared
	// branch ordinal when ALL members were measured in ONE branch, 0 when the
	// occurrences span branches (a cross-branch aggregate has no single branch
	// identity — absence never guesses; P0-E CHAIN-PATH, ledger §22.1).
	ChainDepth      int `json:"chain_depth,omitempty"`
	ChainBranch     int `json:"chain_branch,omitempty"`
	OccurrenceCount int `json:"occurrence_count,omitempty"`
	// AggregationCaliber describes the additivity proof for the ordinary
	// aggregate duration fields (independent from the gated inversion caliber).
	// sum_disjoint_occurrences means every occurrence window was pairwise
	// disjoint; interval_union_or_max_overlap_fallback is the overlap-safe
	// union/MAX rule above.
	AggregationCaliber                  string   `json:"aggregation_caliber,omitempty"`
	DominantState                       string   `json:"dominant_state,omitempty"`
	DominantImpactMs                    float64  `json:"dominant_impact_ms,omitempty"`
	ProjectedImpactMs                   float64  `json:"projected_impact_ms,omitempty"`
	TotalMs                             float64  `json:"total_ms,omitempty"`
	ProjectedTotalMs                    float64  `json:"projected_total_ms,omitempty"`
	ActualImpactMs                      float64  `json:"actual_impact_ms,omitempty"`
	ActualTotalMs                       float64  `json:"actual_total_ms,omitempty"`
	RunningMs                           float64  `json:"running_ms,omitempty"`
	RunnableMs                          float64  `json:"runnable_ms,omitempty"`
	SleepMs                             float64  `json:"sleep_ms,omitempty"`
	DStateMs                            float64  `json:"d_state_ms,omitempty"`
	IOWaitMs                            float64  `json:"io_wait_ms,omitempty"`
	ActualRunningMs                     float64  `json:"actual_running_ms,omitempty"`
	ActualRunnableMs                    float64  `json:"actual_runnable_ms,omitempty"`
	ActualSleepMs                       float64  `json:"actual_sleep_ms,omitempty"`
	ActualDStateMs                      float64  `json:"actual_d_state_ms,omitempty"`
	ActualIOWaitMs                      float64  `json:"actual_io_wait_ms,omitempty"`
	TargetBlockedMs                     float64  `json:"target_blocked_ms,omitempty"`
	FragmentCount                       int      `json:"fragment_count,omitempty"`
	StateSwitches                       int      `json:"state_switches,omitempty"`
	MaxSegmentMs                        float64  `json:"max_segment_ms,omitempty"`
	FirstTs                             float64  `json:"first_ts,omitempty"`
	LastTs                              float64  `json:"last_ts,omitempty"`
	ActualFirstTs                       float64  `json:"actual_first_ts,omitempty"`
	ActualLastTs                        float64  `json:"actual_last_ts,omitempty"`
	LineStart                           int      `json:"line_start,omitempty"`
	LineEnd                             int      `json:"line_end,omitempty"`
	PriorityRelation                    string   `json:"priority_relation,omitempty"`
	PriorityInversion                   bool     `json:"priority_inversion_candidate,omitempty"`
	PriorityRelationCaliber             string   `json:"priority_relation_caliber,omitempty"`
	PriorityRelationProvenLowerMs       float64  `json:"priority_relation_proven_lower_ms,omitempty"`
	PriorityRelationUnknownOrNonLowerMs float64  `json:"priority_relation_unknown_or_nonlower_ms,omitempty"`
	PriorityRelationArtifactSources     []string `json:"priority_relation_artifact_sources,omitempty"`
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
	GatedClusterTopology string `json:"gated_cluster_topology,omitempty"`
	// GatedCapabilityFreqOnlyReason (DISPHYG-3 件7): the members' typed gated
	// freq_only cause token (uniform per query, first non-empty wins — same
	// rule as GatedClusterTopology above).
	GatedCapabilityFreqOnlyReason string                   `json:"gated_capability_freq_only_reason,omitempty"`
	OccurrenceWindows             []WakeupCausalOccurrence `json:"occurrence_windows,omitempty"`
	// VS-1 (§7.8): periodic-signal-source accounting, aggregate face — see the
	// WakeupCausalImpact field docs. LatenessMs here is the SUM of the member
	// occurrences' blocked-caliber lateness amounts, capped at raw blocking −
	// RunnableMs (F1(c): occurrences sharing one branch window must not
	// double-count the same target wait into the Summary);
	// EffectivePeriodicImpactMs = the aggregate RunnableMs (full) + LatenessMs,
	// capped at the aggregate's overlap-safe blocking value. Per-occurrence raw
	// rows stay lossless; aggregate duration fields follow AggregationCaliber
	// and therefore never sum overlapping physical windows.
	PeriodicSource            bool    `json:"periodic_source,omitempty"`
	DetectedPeriodMs          float64 `json:"detected_period_ms,omitempty"`
	LatenessMs                float64 `json:"lateness_ms,omitempty"`
	EffectivePeriodicImpactMs float64 `json:"effective_periodic_impact_ms,omitempty"`
	// VS-2 (§7.10): the overlap-safe merge of member supply-fold vectors (only
	// members whose fold ran contribute — see WakeupCausalImpact). Disjoint
	// components sum; an overlap-connected component contributes one complete
	// representative vector, never independent field maxima. The selected
	// vector identity ideal+deficit==known+unknown remains mechanical. nil
	// basis = no selected member folded.
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
	Groups      int     `json:"groups"`
	MinImpactMs float64 `json:"min_impact_ms,omitempty"`
	MaxImpactMs float64 `json:"max_impact_ms,omitempty"`
	// MaxSubject / MaxStateKind (A2 件5②, §29.179 委托, 2026-07-21): the
	// label and dominant state of the member holding MaxImpactMs — the
	// RUN2FIX-A 件2 max-member disclosure's wire carriers (folded_max_subject
	// / folded_max_state_kind). All-or-nothing: an unlabeled max member
	// clears both (宁漏勿假 — the display then keeps the range-only line).
	MaxSubject   string   `json:"max_subject,omitempty"`
	MaxStateKind string   `json:"max_state_kind,omitempty"`
	Subjects     []string `json:"subjects,omitempty"`
	LineStart    int      `json:"line_start,omitempty"`
	LineEnd      int      `json:"line_end,omitempty"`
	FirstTs      float64  `json:"first_ts,omitempty"`
	LastTs       float64  `json:"last_ts,omitempty"`
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
	// DominantState (v5 P1 件① B.2, 2026-07-13): the scheduler-state word of
	// the causal-impact twin this reduced-shape witness was derived from
	// (rootEvidenceFromCausalImpact copies item.DominantState verbatim) — the
	// producer-side typed identity that lets the projection engine's one-seat
	// arms prove duration-family membership for the raw lane without any
	// display-side registry lookup. Empty on the literal-built shapes
	// (missing_wakeup / trace_gap honesty rows), which never converge.
	DominantState string `json:"dominant_state,omitempty"`
}

type EvidenceFact struct {
	Subject     string              `json:"subject"`
	Predicate   string              `json:"predicate,omitempty"`
	Object      string              `json:"object,omitempty"`
	Summary     string              `json:"summary"`
	LineStart   int                 `json:"line_start,omitempty"`
	LineEnd     int                 `json:"line_end,omitempty"`
	StartTs     float64             `json:"start_ts,omitempty"`
	EndTs       float64             `json:"end_ts,omitempty"`
	Confidence  float64             `json:"confidence,omitempty"`
	SourceSpans []TraceArtifactSpan `json:"source_spans,omitempty"`
}
