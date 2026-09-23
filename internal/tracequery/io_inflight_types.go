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

// Non-nil only for a determined, finite, positive-width selected window.
// Real zero bounds remain explicit on the wire.
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
