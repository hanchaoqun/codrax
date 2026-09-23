package tracequery

const (
	SchedulerConcurrencyPopulationClosedIntervals = "accepted_closed_intervals"
	SchedulerConcurrencyThreadScopeAll            = "all_positive_tids"
	SchedulerConcurrencyStateRunnable             = "runnable"
	SchedulerConcurrencyStateRunning              = "running"
)

// SchedulerConcurrencyStats is an observational account of confirmed, closed
// scheduler intervals. Neither its values nor coverage prove that every CPU or
// task was captured, and neither can authorize a causal/root-cause claim.
type SchedulerConcurrencyStats struct {
	Window                  *SchedulerConcurrencyWindow  `json:"window,omitempty"`
	WindowUnavailableReason string                       `json:"window_unavailable_reason,omitempty"`
	Population              string                       `json:"population"`
	ThreadScope             string                       `json:"thread_scope"`
	QueryPID                int                          `json:"query_pid,omitempty"`
	LineStart               int                          `json:"line_start,omitempty"`
	LineEnd                 int                          `json:"line_end,omitempty"`
	GroupCount              int                          `json:"group_count"`
	Groups                  []SchedulerConcurrencyGroup  `json:"groups,omitempty"`
	OmittedGroups           int                          `json:"omitted_groups"`
	Coverage                SchedulerConcurrencyCoverage `json:"coverage"`
}

type SchedulerConcurrencyWindow struct {
	StartTs float64 `json:"start_ts"`
	EndTs   float64 `json:"end_ts"`
}

type SchedulerConcurrencyGroup struct {
	SourcePath              string                        `json:"source_path"`
	State                   string                        `json:"state"`
	AcceptedIntervalCount   int                           `json:"accepted_interval_count"`
	ThreadCount             int                           `json:"thread_count"`
	Values                  *SchedulerConcurrencyValues   `json:"values,omitempty"`
	ValuesUnavailableReason string                        `json:"values_unavailable_reason,omitempty"`
	Segments                []SchedulerConcurrencySegment `json:"segments,omitempty"`
	OmittedSegments         int                           `json:"omitted_segments"`
	Coverage                SchedulerConcurrencyCoverage  `json:"coverage"`
}

type SchedulerConcurrencyValues struct {
	PeakThreads int     `json:"peak_threads"`
	MeanThreads float64 `json:"mean_threads"`
	BusyMs      float64 `json:"busy_ms"`
	ThreadMs    float64 `json:"thread_ms"`
}

// Zero Threads denotes zero contribution from the confirmed-interval
// population, never proven system idle or absence of an unobserved queue.
type SchedulerConcurrencySegment struct {
	StartTs float64 `json:"start_ts"`
	EndTs   float64 `json:"end_ts"`
	Threads int     `json:"threads"`
}

type SchedulerConcurrencyCoverage struct {
	Status                    string   `json:"status"`
	CandidateIntervals        int      `json:"candidate_intervals"`
	AcceptedIntervals         int      `json:"accepted_intervals"`
	OpenEndedIntervals        int      `json:"open_ended_intervals"`
	SourceConflictIntervals   int      `json:"source_conflict_intervals"`
	UnresolvedSourceIntervals int      `json:"unresolved_source_intervals"`
	IdentityExcludedIntervals int      `json:"identity_excluded_intervals"`
	IdentityExcludedTIDs      int      `json:"identity_excluded_tids"`
	InvalidIntervals          int      `json:"invalid_intervals"`
	Reasons                   []string `json:"reasons,omitempty"`
}
