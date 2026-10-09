package tracequery

const ViewPreferredFrameRate = "preferred_frame_rate"
const PreferredFrameRateSeriesLimit = 32
const PreferredFrameRateDetailLimit = 128
const PreferredFrameRateTeaching = "preferred_frame_rate measures the exact H:PreferredFrameRate process_measure protocol in Hz, not actual display refresh, frame execution, a deadline or vote causality. Use an explicit right-open time window and optional process pid; no thread selector or fallback to another process. Separate source/process/filter series. Equal-rate intervals union; disagreeing rates conflict, invalid/missing values and uncovered time stay unknown, never default 60Hz or last-write-wins. Percentages use the full requested wall-clock window per series; do not sum across owners or filters."

type PreferredFrameRateResult struct {
	Status              string                     `json:"status"`
	SourcePath          string                     `json:"source_path"`
	Window              ProcessMeasurementsWindow  `json:"window"`
	TargetPID           int                        `json:"target_pid,omitempty"`
	TargetScope         string                     `json:"target_scope"`
	Series              []PreferredFrameRateSeries `json:"series"`
	TotalSeries         int                        `json:"total_series"`
	OmittedSeries       int                        `json:"omitted_series"`
	Observations        []ProcessMeasurementRow    `json:"observations"`
	TotalObservations   int                        `json:"total_observations"`
	OmittedObservations int                        `json:"omitted_observations"`
	UnpositionedRows    int                        `json:"unpositioned_rows"`
	Caveats             []string                   `json:"caveats,omitempty"`
}

type PreferredFrameRateSeries struct {
	SourcePath  string `json:"source_path"`
	IPID        string `json:"ipid"`
	FilterID    string `json:"filter_id"`
	PID         *int   `json:"pid,omitempty"`
	ProcessName string `json:"process_name,omitempty"`
	OwnerStatus string `json:"owner_status"`
	// Unknown owners are never grouped across records.
	UnknownOwnerRowID      string                           `json:"unknown_owner_row_id,omitempty"`
	Observations           int                              `json:"observations"`
	UnpositionedRows       int                              `json:"unpositioned_rows"`
	KnownDurationNS        int64                            `json:"known_duration_ns,string"`
	ConflictDurationNS     int64                            `json:"conflict_duration_ns,string"`
	UnknownValueDurationNS int64                            `json:"unknown_value_duration_ns,string"`
	UnobservedDurationNS   int64                            `json:"unobserved_duration_ns,string"`
	Distribution           []PreferredFrameRateDistribution `json:"distribution"`
	TotalDistribution      int                              `json:"total_distribution"`
	OmittedDistribution    int                              `json:"omitted_distribution"`
	Timeline               []PreferredFrameRateInterval     `json:"timeline"`
	TotalIntervals         int                              `json:"total_intervals"`
	OmittedIntervals       int                              `json:"omitted_intervals"`
}

type PreferredFrameRateDistribution struct {
	RateHz        string  `json:"rate_hz"`
	DurationNS    int64   `json:"duration_ns,string"`
	Intervals     int     `json:"intervals"`
	WindowPercent float64 `json:"window_percent"`
}

type PreferredFrameRateInterval struct {
	StartNS int64  `json:"start_ns,string"`
	EndNS   int64  `json:"end_ns,string"`
	State   string `json:"state"`
	RateHz  string `json:"rate_hz,omitempty"`
	// Every source reference contributing to an interval is counted; bounded
	// examples are not presented as a complete observation population.
	SourceLines        []int `json:"source_lines,omitempty"`
	TotalSourceLines   int   `json:"total_source_lines"`
	OmittedSourceLines int   `json:"omitted_source_lines"`
}
