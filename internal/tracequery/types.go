package tracequery

import "time"

const ParserVersion = "tracequery-v2"

type EventType string

const (
	EventUnknown            EventType = "unknown"
	EventSchedSwitch        EventType = "sched_switch"
	EventSchedWakeup        EventType = "sched_wakeup"
	EventSchedWaking        EventType = "sched_waking"
	EventSchedBlockedReason EventType = "sched_blocked_reason"
	EventCPUIdle            EventType = "cpu_idle"
	EventCPUFrequency       EventType = "cpu_frequency"
	EventBlockIssue         EventType = "block_rq_issue"
	EventBlockRemap         EventType = "block_bio_remap"
	EventBlockComplete      EventType = "block_rq_complete"
	EventBinderTransaction  EventType = "binder_transaction"
	EventBinderReceived     EventType = "binder_transaction_received"
	EventIRQ                EventType = "irq"
	EventTraceMark          EventType = "trace_mark"
	EventMemory             EventType = "memory"
)

type Event struct {
	Line int       `json:"line"`
	Ts   float64   `json:"ts"`
	CPU  int       `json:"cpu,omitempty"`
	Type EventType `json:"type"`

	Comm string `json:"comm,omitempty"`
	PID  int    `json:"pid,omitempty"`
	TGID int    `json:"tgid,omitempty"`

	PrevComm      string `json:"prev_comm,omitempty"`
	PrevPID       int    `json:"prev_pid,omitempty"`
	PrevPrio      int    `json:"prev_prio,omitempty"`
	PrevPrioClass string `json:"prev_prio_class,omitempty"`
	PrevState     string `json:"prev_state,omitempty"`
	NextComm      string `json:"next_comm,omitempty"`
	NextPID       int    `json:"next_pid,omitempty"`
	NextPrio      int    `json:"next_prio,omitempty"`
	NextPrioClass string `json:"next_prio_class,omitempty"`

	WakeeComm      string `json:"wakee_comm,omitempty"`
	WakeePID       int    `json:"wakee_pid,omitempty"`
	WakeePrio      int    `json:"wakee_prio,omitempty"`
	WakeePrioClass string `json:"wakee_prio_class,omitempty"`
	TargetCPU      int    `json:"target_cpu,omitempty"`

	State       int    `json:"state,omitempty"`
	Frequency   int    `json:"frequency,omitempty"`
	CPUForField int    `json:"cpu_for_field,omitempty"`
	Reason      string `json:"reason,omitempty"`
	IOWait      int    `json:"io_wait,omitempty"`
	SpanAction  string `json:"span_action,omitempty"`
	SpanName    string `json:"span_name,omitempty"`
	SpanValue   string `json:"span_value,omitempty"`

	BinderTransactionID int    `json:"binder_transaction_id,omitempty"`
	BinderDestProc      int    `json:"binder_dest_proc,omitempty"`
	BinderDestThread    int    `json:"binder_dest_thread,omitempty"`
	BinderReply         int    `json:"binder_reply,omitempty"`
	BinderFlags         string `json:"binder_flags,omitempty"`
	BinderCode          string `json:"binder_code,omitempty"`

	FieldText string `json:"field_text,omitempty"`
}

type Index struct {
	Path        string
	Size        int64
	ModTime     time.Time
	LineCount   int
	Events      []Event
	FirstTs     float64
	LastTs      float64
	ParsedKnown int
}

type Query struct {
	View               string
	Thread             string
	PID                int
	TimeStart          float64
	TimeEnd            float64
	LineStart          int
	LineEnd            int
	EventTypes         []EventType
	MaxDepth           int
	MaxBranches        int
	MinDurationMs      float64
	IncludeWindowStats bool
	Limit              int
}

type Result struct {
	View              string          `json:"view"`
	SourcePath        string          `json:"source_path"`
	TimeUnit          string          `json:"time_unit,omitempty"`
	PrioritySemantics string          `json:"priority_semantics,omitempty"`
	LineCount         int             `json:"line_count,omitempty"`
	EventCount        int             `json:"event_count,omitempty"`
	TimeStart         float64         `json:"time_start,omitempty"`
	TimeEnd           float64         `json:"time_end,omitempty"`
	Events            []EventView     `json:"events,omitempty"`
	Timeline          *TimelineResult `json:"timeline,omitempty"`
	WindowStats       *WindowStats    `json:"window_stats,omitempty"`
	IPCGraph          *IPCGraphResult `json:"ipc_graph,omitempty"`
	WakeupChain       *ChainResult    `json:"wakeup_chain,omitempty"`
	EvidencePack      []EvidenceFact  `json:"evidence_pack,omitempty"`
	Caveats           []string        `json:"caveats,omitempty"`
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
	Thread       ThreadRef   `json:"thread"`
	State        ThreadState `json:"state"`
	StartTs      float64     `json:"start_ts"`
	EndTs        float64     `json:"end_ts"`
	DurationMs   float64     `json:"duration_ms"`
	StartLine    int         `json:"start_line,omitempty"`
	EndLine      int         `json:"end_line,omitempty"`
	WakeupLine   int         `json:"wakeup_line,omitempty"`
	PrevStateRaw string      `json:"prev_state_raw,omitempty"`
	Summary      string      `json:"summary,omitempty"`
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
	Window              TimeWindow             `json:"window"`
	EventCounts         map[EventType]int      `json:"event_counts,omitempty"`
	CPU                 []CPUStats             `json:"cpu,omitempty"`
	TopRunning          []ThreadDuration       `json:"top_running,omitempty"`
	RunnableTop         []ThreadDuration       `json:"runnable_top,omitempty"`
	DStateTop           []ThreadDuration       `json:"d_state_top,omitempty"`
	BlockIssueCount     int                    `json:"block_issue_count,omitempty"`
	BlockRemapCount     int                    `json:"block_remap_count,omitempty"`
	BlockCompleteCount  int                    `json:"block_complete_count,omitempty"`
	BinderCount         int                    `json:"binder_count,omitempty"`
	BinderReceivedCount int                    `json:"binder_received_count,omitempty"`
	IRQCount            int                    `json:"irq_count,omitempty"`
	MemoryEventCount    int                    `json:"memory_event_count,omitempty"`
	BlockedReasonCount  int                    `json:"blocked_reason_count,omitempty"`
	IOWaitBlockedCount  int                    `json:"io_wait_blocked_count,omitempty"`
	BlockedReasons      []BlockedReasonSummary `json:"blocked_reasons,omitempty"`
	Caveats             []string               `json:"caveats,omitempty"`
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
	CPU       int     `json:"cpu"`
	BusyMs    float64 `json:"busy_ms,omitempty"`
	IdleMs    float64 `json:"idle_ms,omitempty"`
	Frequency int     `json:"frequency,omitempty"`
}

type ThreadDuration struct {
	Thread        ThreadRef `json:"thread"`
	DurationMs    float64   `json:"duration_ms"`
	LineStart     int       `json:"line_start,omitempty"`
	LineEnd       int       `json:"line_end,omitempty"`
	Priority      int       `json:"priority,omitempty"`
	PriorityClass string    `json:"priority_class,omitempty"`
}

type ChainResult struct {
	Target       ThreadRef      `json:"target"`
	Window       TimeWindow     `json:"window"`
	Nodes        []ChainNode    `json:"nodes"`
	Edges        []WakeupEdge   `json:"edges,omitempty"`
	IPCEdges     []IPCEdge      `json:"ipc_edges,omitempty"`
	RootEvidence []RootEvidence `json:"root_evidence,omitempty"`
	Caveats      []string       `json:"caveats,omitempty"`
}

type IPCGraphResult struct {
	Window  TimeWindow `json:"window"`
	Edges   []IPCEdge  `json:"edges,omitempty"`
	Caveats []string   `json:"caveats,omitempty"`
}

type IPCEdge struct {
	TransactionID int       `json:"transaction_id,omitempty"`
	Sender        ThreadRef `json:"sender"`
	Receiver      ThreadRef `json:"receiver,omitempty"`
	DestProc      int       `json:"dest_proc,omitempty"`
	DestThread    int       `json:"dest_thread,omitempty"`
	SendTs        float64   `json:"send_ts,omitempty"`
	ReceiveTs     float64   `json:"receive_ts,omitempty"`
	SendLine      int       `json:"send_line,omitempty"`
	ReceiveLine   int       `json:"receive_line,omitempty"`
	Reply         int       `json:"reply,omitempty"`
	Flags         string    `json:"flags,omitempty"`
	Code          string    `json:"code,omitempty"`
	Oneway        bool      `json:"oneway,omitempty"`
	LatencyMs     float64   `json:"latency_ms,omitempty"`
	Confidence    float64   `json:"confidence,omitempty"`
	Caveats       []string  `json:"caveats,omitempty"`
}

type ChainNode struct {
	ID           string      `json:"id"`
	Thread       ThreadRef   `json:"thread"`
	Window       TimeWindow  `json:"window"`
	Dominant     ThreadState `json:"dominant_state"`
	DurationMs   float64     `json:"duration_ms,omitempty"`
	EvidenceLine int         `json:"evidence_line,omitempty"`
	Summary      string      `json:"summary,omitempty"`
}

type WakeupEdge struct {
	From         string    `json:"from"`
	To           string    `json:"to"`
	Waker        ThreadRef `json:"waker"`
	Wakee        ThreadRef `json:"wakee"`
	WakeupTs     float64   `json:"wakeup_ts"`
	WakeupLine   int       `json:"wakeup_line"`
	LatencyMs    float64   `json:"latency_ms,omitempty"`
	EvidenceLine int       `json:"evidence_line,omitempty"`
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
