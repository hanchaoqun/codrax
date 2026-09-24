package tracequery

// TraceMarkerTreeStats describes observed synchronous marker instances, not a
// causal chain. Its totals are computed before the independent display bounds.
// A root is only a root of the observed stack, never proof of a program root.
type TraceMarkerTreeStats struct {
	Window TraceMarkerTreeWindow `json:"window"`
	// A line-only query has no determined continuous time window. Its nodes
	// still own exact physical/query-clipped interval sets, but zero query
	// placeholders must not be presented as real clock endpoints.
	WindowUnavailableReason string                `json:"window_unavailable_reason,omitempty"`
	NodeCount               int                   `json:"node_count"`
	Nodes                   []TraceMarkerTreeNode `json:"nodes,omitempty"`
	OmittedNodes            int                   `json:"omitted_nodes"`
	Coverage                string                `json:"coverage"`
	Caveats                 []string              `json:"caveats,omitempty"`
}

// Marker windows contain determined physical/query-clipped endpoints. Both
// endpoints survive JSON even when the trace begins at zero.
type TraceMarkerTreeWindow struct {
	StartTs float64 `json:"start_ts"`
	EndTs   float64 `json:"end_ts"`
}

// ID is source + emitter TID + physical begin instance, not a marker name or
// payload process id. Line coordinates retain the Index's virtual line space;
// the ordinary artifact ledger maps them to physical source-local lines.
type TraceMarkerTreeNode struct {
	ID            string    `json:"id"`
	ParentID      string    `json:"parent_id,omitempty"`
	ParentStatus  string    `json:"parent_status"`
	SourcePath    string    `json:"source_path"`
	Thread        ThreadRef `json:"thread"`
	Name          string    `json:"name"`
	StartLine     int       `json:"start_line"`
	EndLine       int       `json:"end_line,omitempty"`
	ActualStartTs float64   `json:"actual_start_ts"`
	ActualEndTs   *float64  `json:"actual_end_ts,omitempty"`
	Closure       string    `json:"closure"`
	// DirectChildCount is the full physical instance's child count, including
	// children outside the query. It is not the number of displayed child rows.
	DirectChildCount int                     `json:"direct_child_count"`
	Inclusive        *TraceMarkerTreeAccount `json:"inclusive,omitempty"`
	Self             *TraceMarkerTreeAccount `json:"self,omitempty"`
}

// Segments are query-clipped half-open extents. Self can be a disconnected
// set: its denominator is DurationMs, not the enclosing continuous window.
type TraceMarkerTreeAccount struct {
	DurationMs      float64                 `json:"duration_ms"`
	Segments        []TraceMarkerTreeWindow `json:"segments,omitempty"`
	OmittedSegments int                     `json:"omitted_segments"`
	States          *TraceMarkerTreeStates  `json:"states,omitempty"`
}

// Nil Values is unavailable, not an all-zero scheduler account. UnknownMs
// includes gaps and unclassified time. SleepIOWaitMs refines SleepMs and must
// not be added again to the mutually exclusive scheduler lanes.
type TraceMarkerTreeStates struct {
	Coverage  string                      `json:"coverage"`
	Values    *TraceMarkerTreeStateValues `json:"values,omitempty"`
	UnknownMs float64                     `json:"unknown_ms"`
	Reasons   []string                    `json:"reasons,omitempty"`
}

type TraceMarkerTreeStateValues struct {
	RunningMs     float64 `json:"running_ms"`
	RunnableMs    float64 `json:"runnable_ms"`
	SleepMs       float64 `json:"sleep_ms"`
	DStateMs      float64 `json:"d_state_ms"`
	IOWaitMs      float64 `json:"io_wait_ms"`
	StoppedMs     float64 `json:"stopped_ms"`
	DeadMs        float64 `json:"dead_ms"`
	SleepIOWaitMs float64 `json:"sleep_io_wait_ms"`
	AccountedMs   float64 `json:"accounted_ms"`
}
