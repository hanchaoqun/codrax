package tracequery

// ProcessProfile describes observed, natively identified members. Its tree is
// membership -> scheduler state -> blocked caller, never a dependency tree.
type ProcessProfile struct {
	Status                   string                 `json:"status"`
	SourcePath               string                 `json:"source_path"`
	Reason                   string                 `json:"reason,omitempty"`
	Window                   TimeWindow             `json:"window"`
	WindowMs                 float64                `json:"window_ms"`
	SourceThread             ThreadRef              `json:"source_thread"`
	TGID                     int                    `json:"tgid,omitempty"`
	MembershipBasis          string                 `json:"membership_basis"`
	ThreadCount              int                    `json:"thread_count"`
	EmittedThreads           int                    `json:"emitted_threads"`
	OmittedThreads           int                    `json:"omitted_threads"`
	UnavailableThreads       int                    `json:"unavailable_threads"`
	UnknownMembershipThreads int                    `json:"unknown_membership_threads"`
	Threads                  []ProcessProfileThread `json:"threads"`
	Caveats                  []string               `json:"caveats,omitempty"`
}

type ProcessProfileThread struct {
	Thread ThreadRef              `json:"thread"`
	States *TraceMarkerTreeStates `json:"states"`
	// Whole-window wall clock, not the sum of observed states. Nil means no
	// usable scheduler measurement, not zero running time.
	RunningWindowPct      *float64                 `json:"running_window_pct,omitempty"`
	SleepGroups           []ProcessSleepGroup      `json:"sleep_groups,omitempty"`
	SleepGroupCount       int                      `json:"sleep_group_count"`
	OmittedSleepGroups    int                      `json:"omitted_sleep_groups"`
	BusinessHotspots      []ProcessBusinessHotspot `json:"business_hotspots,omitempty"`
	BusinessGroupCount    int                      `json:"business_group_count"`
	OmittedBusinessGroups int                      `json:"omitted_business_groups"`
}

// Closed synchronous marker instances, grouped within one emitter only.
// Inclusive elapsed time includes nested work and is not CPU time.
type ProcessBusinessHotspot struct {
	Name          string  `json:"name"`
	InstanceCount int     `json:"instance_count"`
	InclusiveMs   float64 `json:"inclusive_ms"`
	MaxInstanceMs float64 `json:"max_instance_ms"`
	LineStart     int     `json:"line_start"`
	LineEnd       int     `json:"line_end"`
}

type ProcessSleepGroup struct {
	State         ThreadState `json:"state"`
	Caller        string      `json:"caller,omitempty"`
	CallerKnown   bool        `json:"caller_known"`
	IntervalCount int         `json:"interval_count"`
	DurationMs    float64     `json:"duration_ms"`
	// On S this refines, rather than adds to, DurationMs. D-IO already has its
	// own mutually exclusive state lane. Missing iowait flags remain unknown.
	IOFlagKnownIntervals int     `json:"io_flag_known_intervals"`
	IOFlagPositiveMs     float64 `json:"io_flag_positive_ms"`
	LineStart            int     `json:"line_start,omitempty"`
	LineEnd              int     `json:"line_end,omitempty"`
}

const processProfileSleepGroupLimit = 8
