package tracequery

import "github.com/hanchaoqun/codrax/internal/tracewire"

const ViewResourceStack = "resource_stack"

const ResourceStackTeaching = "resource_stack expands observed native resource events into same-capture callchain frames using time and optional pid/thread selectors. Frame depth is source order, not a business-leaf rule. Source frame census, missing symbols/depths and display omissions are distinct; complete source rows do not prove complete unwinding. Resource lifetimes and stack frames do not prove execution duration, CPU or response causality."

type ResourceStackResult struct {
	SourcePath    string               `json:"source_path"`
	Window        ResourceStackWindow  `json:"window"`
	Status        string               `json:"status"`
	Reason        string               `json:"reason,omitempty"`
	TargetPID     int                  `json:"target_pid,omitempty"`
	TargetThread  string               `json:"target_thread,omitempty"`
	TargetScope   string               `json:"target_scope,omitempty"`
	MatchedEvents int                  `json:"matched_events"`
	OmittedEvents int                  `json:"omitted_events"`
	Events        []ResourceStackEvent `json:"events,omitempty"`
	Caveats       []string             `json:"caveats,omitempty"`
}

type ResourceStackWindow struct {
	StartTs      float64 `json:"start_ts"`
	EndTs        float64 `json:"end_ts"`
	EndInclusive bool    `json:"end_inclusive,omitempty"`
}

type ResourceStackEvent struct {
	RowID                int64                   `json:"row_id,string"`
	Line                 int                     `json:"line"`
	TimestampNS          int64                   `json:"timestamp_ns,string"`
	Source               tracewire.ResourceEvent `json:"source"`
	SourceFramesComplete bool                    `json:"source_frames_complete"`
	UnwindCompleteness   string                  `json:"unwind_completeness"`
	DepthStatus          string                  `json:"depth_status"`
	MissingDepths        int64                   `json:"missing_depths"`
	DuplicateDepths      int                     `json:"duplicate_depths"`
	InvalidDepths        int                     `json:"invalid_depths"`
	UnknownSymbols       int                     `json:"unknown_symbols"`
	OmittedFrames        int                     `json:"omitted_frames"`
	Frames               []ResourceStackFrame    `json:"frames,omitempty"`
}

type ResourceStackFrame struct {
	Line int `json:"line"`
	tracewire.ResourceFrame
}
