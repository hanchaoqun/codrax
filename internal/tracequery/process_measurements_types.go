package tracequery

import "github.com/hanchaoqun/codrax/internal/tracewire"

const ViewProcessMeasurements = "process_measurements"
const ProcessMeasurementsLimit = 64
const ProcessMeasurementsTeaching = "process_measurements preserves native SQL process_measure rows and their exact integer values, nulls and storage classes. Ownership defaults to process; optional pid filters the process ID, while thread selectors cannot establish process ownership. Explicit intervals intersect the selected right-open window, including carry-in; unknown duration is a timestamped observation, not filled time. Overlaps remain separate rows, not a last-value stream. Unit and aggregation meaning are unknown without a source protocol; metric names do not establish bytes, Hz, deltas or stock. Process observations are not thread execution, CPU activity or response causes."

type ProcessMeasurementsWindow struct {
	StartTs      float64 `json:"start_ts"`
	EndTs        float64 `json:"end_ts"`
	EndInclusive bool    `json:"end_inclusive,omitempty"`
}

type ProcessMeasurementsResult struct {
	Status           string                    `json:"status"`
	SourcePath       string                    `json:"source_path"`
	Window           ProcessMeasurementsWindow `json:"window"`
	TargetPID        int                       `json:"target_pid,omitempty"`
	TargetScope      string                    `json:"target_scope,omitempty"`
	Rows             []ProcessMeasurementRow   `json:"rows"`
	TotalRows        int                       `json:"total_rows"`
	OmittedRows      int                       `json:"omitted_rows"`
	UnpositionedRows int                       `json:"unpositioned_rows"`
	// Optional native interpretations found in the complete selected inventory,
	// collected before row display limits. Raw rows retain their original units.
	AvailableDerivedViews []string `json:"available_derived_views,omitempty"`
	Caveats               []string `json:"caveats,omitempty"`
}

type ProcessMeasurementRow struct {
	SourcePath     string                           `json:"source_path"`
	Line           int                              `json:"line"`
	SourceLine     int                              `json:"source_line"`
	Record         tracewire.ProcessMeasureInterval `json:"record"`
	ClippedStartNS *int64                           `json:"clipped_start_ns,string,omitempty"`
	ClippedEndNS   *int64                           `json:"clipped_end_ns,string,omitempty"`
	Selection      string                           `json:"selection"`
	Unit           string                           `json:"unit"`
}
