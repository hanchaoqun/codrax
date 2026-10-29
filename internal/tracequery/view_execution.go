package tracequery

import "fmt"

// Execution modes describe implemented adapters, not names or result presence.
// Every capacity row declares one; a new catalog row gets no implicit authority.
type ViewExecutionMode string

const (
	ExecutionIndexedTrace    ViewExecutionMode = "indexed_trace"
	ExecutionStreamingTrace  ViewExecutionMode = "streaming_trace"
	ExecutionStreamingPacket ViewExecutionMode = "streaming_packet"
)

type ViewExecutionEntry string

const (
	ExecutionEntryNativeTool ViewExecutionEntry = "trace_query"
	ExecutionEntryTraceDiag  ViewExecutionEntry = "tracediag"
	ExecutionEntryIndexedRun ViewExecutionEntry = "indexed_run"
)

type ViewExecutionCapability struct {
	Mode             ViewExecutionMode    `json:"mode"`
	Entries          []ViewExecutionEntry `json:"entries"`
	CoordinateDomain string               `json:"coordinate_domain"`
}

// ViewExecutionFor is shared by admission and model-facing capability teaching.
// Returned slices are independent so a consumer cannot change admission rights.
func ViewExecutionFor(view string) ViewExecutionCapability {
	mode := ViewCapacityFor(view).Execution
	switch mode {
	case ExecutionIndexedTrace:
		return ViewExecutionCapability{mode, []ViewExecutionEntry{ExecutionEntryNativeTool, ExecutionEntryTraceDiag, ExecutionEntryIndexedRun}, "trace_query_scope"}
	case ExecutionStreamingTrace:
		return ViewExecutionCapability{mode, []ViewExecutionEntry{ExecutionEntryNativeTool, ExecutionEntryTraceDiag}, "trace_query_scope"}
	case ExecutionStreamingPacket:
		return ViewExecutionCapability{mode, []ViewExecutionEntry{ExecutionEntryNativeTool}, "packet_source_native_clock"}
	default:
		return ViewExecutionCapability{}
	}
}

func ValidateViewExecution(view string, entry ViewExecutionEntry) error {
	if err := ValidateViewName(view); err != nil {
		return err
	}
	capability := ViewExecutionFor(view)
	for _, supported := range capability.Entries {
		if entry == supported {
			return nil
		}
	}
	if CanonicalViewName(view) == ViewWindowSweep && entry == ExecutionEntryIndexedRun {
		return fmt.Errorf("view %q has no indexed_run execution; use StreamWindowSweep", ViewWindowSweep)
	}
	return fmt.Errorf("view %q has no %s execution adapter; supported execution entries: %v", CanonicalViewName(view), entry, capability.Entries)
}

// ValidateTraceViewInput checks the source domain before a streaming recovery
// adapter replaces the requested view with its own observation operation. A
// known Trace view may intentionally use a lighter recovery operation; this
// does not claim that the adapter executes that requested view unchanged.
func ValidateTraceViewInput(view string) error {
	if err := ValidateViewName(view); err != nil {
		return err
	}
	switch ViewCapacityFor(view).Execution {
	case ExecutionIndexedTrace, ExecutionStreamingTrace:
		return nil
	default:
		return fmt.Errorf("streaming Trace recovery requires a Trace-domain input view; view %q declares execution mode %q", CanonicalViewName(view), ViewCapacityFor(view).Execution)
	}
}
