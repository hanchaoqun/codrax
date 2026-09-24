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
	BucketMs                float64                      `json:"bucket_ms"`
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
	// These disjoint witness sets partition AcceptedIntervalCount. They are
	// physical producer intervals, not a roster reconstructed from depth.
	Members                       []SchedulerConcurrencyMember      `json:"members,omitempty"`
	OmittedMembers                int                               `json:"omitted_members"`
	MemberWitnessUnavailableCount int                               `json:"member_witness_unavailable_count"`
	Distribution                  *SchedulerConcurrencyDistribution `json:"distribution,omitempty"`
	Buckets                       []SchedulerConcurrencyBucket      `json:"buckets,omitempty"`
	BucketCount                   uint64                            `json:"bucket_count"`
	OmittedBuckets                uint64                            `json:"omitted_buckets"`
	BucketsUnavailableReason      string                            `json:"buckets_unavailable_reason,omitempty"`
}

// A member identifies one interval admitted by the existing scheduler
// producer. Overlapping same-thread members are unioned for numeric statistics;
// summing these individual contributions does not reproduce ThreadMs.
type SchedulerConcurrencyMember struct {
	ID         string    `json:"id"`
	SourcePath string    `json:"source_path"`
	Thread     ThreadRef `json:"thread"`
	// CPU is the existing producer's verified attribution, not a sampled wake
	// target. Nil retains an unknown/conflicting CPU while counting the thread.
	CPU     *int   `json:"cpu,omitempty"`
	Closure string `json:"closure"`
	// StartLine/EndLine use the index's virtual coordinates; LocalLine fields
	// are physical rows in SourcePath and remain stable under composite rebasing.
	StartLine      int     `json:"start_line"`
	EndLine        int     `json:"end_line"`
	StartLocalLine int     `json:"start_local_line"`
	EndLocalLine   int     `json:"end_local_line"`
	ActualStartTs  float64 `json:"actual_start_ts"`
	ActualEndTs    float64 `json:"actual_end_ts"`
	// Nil means no continuous query window, not a measured zero.
	WindowContribution   *SchedulerConcurrencyWindow `json:"window_contribution,omitempty"`
	WindowContributionMs *float64                    `json:"window_contribution_ms,omitempty"`
}

// Distribution uses complete same-thread-unioned closed intervals. Its CDF
// is weighted by wall-clock duration, including zero contribution from this
// population (not a claim of observed system idle). Quantiles are the smallest
// integer depth whose cumulative duration reaches the requested fraction.
type SchedulerConcurrencyDistribution struct {
	WindowMs      float64                             `json:"window_ms"`
	DepthCount    int                                 `json:"depth_count"`
	Depths        []SchedulerConcurrencyDepthDuration `json:"depths"`
	OmittedDepths int                                 `json:"omitted_depths"`
	P50Threads    int                                 `json:"p50_threads"`
	P95Threads    int                                 `json:"p95_threads"`
	P99Threads    int                                 `json:"p99_threads"`
}

type SchedulerConcurrencyDepthDuration struct {
	Threads     int     `json:"threads"`
	DurationMs  float64 `json:"duration_ms"`
	WindowShare float64 `json:"window_share"`
}

// Each bucket uses its actual half-open width, including a short tail. Peak
// and mean describe different quantities; no percentile is calculated from
// displayed buckets. Omitted buckets do not change the complete distribution.
type SchedulerConcurrencyBucket struct {
	Window SchedulerConcurrencyWindow `json:"window"`
	Values SchedulerConcurrencyValues `json:"values"`
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
