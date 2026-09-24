package tracequery

const (
	IOInFlightPopulationCompletePairs = "accepted_complete_pairs"
	IOInFlightIssuerScopeAll          = "all_issuers"
	IOInFlightCoverageAvailable       = "available"
	IOInFlightCoveragePartial         = "partial"
	IOInFlightCoverageUnavailable     = "unavailable"
)

// IOInFlightStats measures only requests admitted by the existing physical
// endpoint pairing contracts. It does not establish capture completeness,
// device queue occupancy, target-thread blocking, or causal eligibility.
type IOInFlightStats struct {
	Window                  *IOInFlightWindow `json:"window,omitempty"`
	WindowUnavailableReason string            `json:"window_unavailable_reason,omitempty"`
	Population              string            `json:"population"`
	IssuerScope             string            `json:"issuer_scope"`
	// QueryPID records the query target, not a filter on this all-issuer face.
	QueryPID      int                         `json:"query_pid,omitempty"`
	LineStart     int                         `json:"line_start,omitempty"`
	LineEnd       int                         `json:"line_end,omitempty"`
	GroupCount    int                         `json:"group_count"`
	Groups        []IOInFlightGroup           `json:"groups,omitempty"`
	OmittedGroups int                         `json:"omitted_groups"`
	Coverage      []IOInFlightPairingCoverage `json:"coverage"`
}

// Bounds on the trace's seconds axis. IOInFlightStats.Window requires a
// determined, finite, positive-width selection. A member's contribution can
// instead be a measured zero-width intersection. Real zero remains explicit.
type IOInFlightWindow struct {
	StartTs float64 `json:"start_ts"`
	EndTs   float64 `json:"end_ts"`
}

type IOInFlightGroup struct {
	SourcePath        string `json:"source_path"`
	Layer             string `json:"layer"`
	EndpointFamily    string `json:"endpoint_family"`
	Dev               string `json:"dev,omitempty"`
	Operation         string `json:"operation,omitempty"`
	AcceptedPairCount int    `json:"accepted_pair_count"`
	// IssueCount counts admitted start endpoints inside the selected query,
	// independently of whether they later formed an unambiguous complete pair.
	IssueCount              int                 `json:"issue_count"`
	Values                  *IOInFlightValues   `json:"values,omitempty"`
	ValuesUnavailableReason string              `json:"values_unavailable_reason,omitempty"`
	Segments                []IOInFlightSegment `json:"segments,omitempty"`
	OmittedSegments         int                 `json:"omitted_segments"`
	// Members are bounded witnesses of accepted pairs, not the population
	// used to recompute Values or a roster for each merged depth segment.
	// These three disjoint sets exactly partition AcceptedPairCount:
	// len(Members) + OmittedMembers + MemberWitnessUnavailableCount.
	Members                       []IOInFlightMember `json:"members,omitempty"`
	OmittedMembers                int                `json:"omitted_members"`
	MemberWitnessUnavailableCount int                `json:"member_witness_unavailable_count"`
}

// IOInFlightMember preserves endpoints from the existing successful matcher.
// It conveys request residence, not issuer blocking or causal eligibility.
// Members sort by physical issue/completion line, independently of latency
// Top-N. Identity binds source, layer/family and actual endpoint instances;
// reused device/sector/name values cannot merge independent requests.
type IOInFlightMember struct {
	ID                string    `json:"id"`
	SourcePath        string    `json:"source_path"`
	IssueThread       ThreadRef `json:"issue_thread"`
	CompleteThread    ThreadRef `json:"complete_thread"`
	IssueLine         int       `json:"issue_line"`          // index-global virtual line
	CompleteLine      int       `json:"complete_line"`       // index-global virtual line
	IssueLocalLine    int       `json:"issue_local_line"`    // physical source-local line
	CompleteLocalLine int       `json:"complete_local_line"` // physical source-local line
	ActualStartTs     float64   `json:"actual_start_ts"`
	ActualEndTs       float64   `json:"actual_end_ts"`
	// Nil contribution_ms means the selected continuous time window is not
	// established (e.g. line-selected query), never a measured zero. A known
	// zero has a non-nil value; its interval is nil only for disjoint bounds.
	WindowContribution   *IOInFlightWindow `json:"window_contribution,omitempty"`
	WindowContributionMs *float64          `json:"window_contribution_ms,omitempty"`
}

// Values are exact for the admitted-pair population, never an estimate of
// withheld/incomplete requests. Nil is unavailable; measured zero is explicit.
type IOInFlightValues struct {
	PeakRequests int     `json:"peak_requests"`
	MeanRequests float64 `json:"mean_requests"`
	BusyMs       float64 `json:"busy_ms"`
	RequestMs    float64 `json:"request_ms"`
}

// Constant depth on [StartTs, EndTs), including retained idle segments.
type IOInFlightSegment struct {
	StartTs  float64 `json:"start_ts"`
	EndTs    float64 `json:"end_ts"`
	Requests int     `json:"requests"`
}

// Coverage belongs to the block or non-block pairing family, not an inferred
// device group. These typed counters describe the existing pairing query
// cohort; zero exclusions never prove that capture enabled every IO event.
type IOInFlightPairingCoverage struct {
	Family                 string   `json:"family"`
	Status                 string   `json:"status"`
	TopologyComplete       bool     `json:"topology_complete"`
	AcceptedPairCount      int      `json:"accepted_pair_count"`
	UnpairedStartCount     int      `json:"unpaired_start_count"`
	UnpairedDoneCount      int      `json:"unpaired_done_count"`
	AmbiguousCohortCount   int      `json:"ambiguous_cohort_count"`
	PairingSuppressedCount int      `json:"pairing_suppressed_count"`
	RejectedEndpointRows   int      `json:"rejected_endpoint_rows"`
	UnresolvedSources      int      `json:"unresolved_sources"`
	Reasons                []string `json:"reasons,omitempty"`
}
