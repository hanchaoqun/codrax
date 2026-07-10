package tracequery

import (
	"fmt"
	"sort"
	"strings"
)

// TraceMarkAction is the closed, parser-validated tracing marker action set.
// It deliberately mirrors the exact wire token stored in Event.SpanAction:
// callers filter the typed parse product, never a raw payload prefix.
type TraceMarkAction string

const (
	TraceMarkActionBegin        TraceMarkAction = "B"
	TraceMarkActionEnd          TraceMarkAction = "E"
	TraceMarkActionCounter      TraceMarkAction = "C"
	TraceMarkActionAsyncBegin   TraceMarkAction = "S"
	TraceMarkActionAsyncEnd     TraceMarkAction = "F"
	TraceMarkActionTrackBegin   TraceMarkAction = "G"
	TraceMarkActionTrackEnd     TraceMarkAction = "H"
	TraceMarkActionTrackInstant TraceMarkAction = "N"
	TraceMarkActionInstant      TraceMarkAction = "I"
)

var canonicalTraceMarkActions = [...]TraceMarkAction{
	TraceMarkActionBegin,
	TraceMarkActionEnd,
	TraceMarkActionCounter,
	TraceMarkActionAsyncBegin,
	TraceMarkActionAsyncEnd,
	TraceMarkActionTrackBegin,
	TraceMarkActionTrackEnd,
	TraceMarkActionTrackInstant,
	TraceMarkActionInstant,
}

// TraceMarkActionNames exports the canonical wire-token order to the tool and
// tracediag schemas. The returned slice is a copy and is safe for callers to
// mutate.
func TraceMarkActionNames() []string {
	out := make([]string, len(canonicalTraceMarkActions))
	for i, action := range canonicalTraceMarkActions {
		out[i] = string(action)
	}
	return out
}

// ValidateTraceMarkActionFilter is the shared hard boundary for the engine,
// LLM tool and deterministic tracediag script. A non-empty action filter is
// meaningful only for event_search and itself restricts the result to parsed
// trace_mark rows, so event_types may be omitted or contain trace_mark only.
func ValidateTraceMarkActionFilter(view string, eventTypes []EventType, actions []TraceMarkAction) error {
	if len(actions) == 0 {
		return nil
	}
	if CanonicalViewName(view) != FallbackViewEventSearch {
		return fmt.Errorf("trace_mark_actions is only valid for view=%s, got view=%s", FallbackViewEventSearch, CanonicalViewName(view))
	}
	valid := make(map[TraceMarkAction]bool, len(canonicalTraceMarkActions))
	for _, action := range canonicalTraceMarkActions {
		valid[action] = true
	}
	seen := make(map[TraceMarkAction]bool, len(actions))
	for _, action := range actions {
		if !valid[action] {
			return fmt.Errorf("unknown trace_mark action %q; supported: %s", action, strings.Join(TraceMarkActionNames(), ", "))
		}
		if seen[action] {
			return fmt.Errorf("duplicate trace_mark action %q", action)
		}
		seen[action] = true
	}
	for _, eventType := range eventTypes {
		if eventType != "" && eventType != EventTraceMark {
			return fmt.Errorf("trace_mark_actions requires event_types to be omitted or exactly [%s], got incompatible type %q", EventTraceMark, eventType)
		}
	}
	return nil
}

func traceMarkActionFilterSet(actions []TraceMarkAction) map[string]bool {
	if len(actions) == 0 {
		return nil
	}
	out := make(map[string]bool, len(actions))
	for _, action := range actions {
		out[string(action)] = true
	}
	return out
}

// CPUGlobalEventSearchTypes returns the exact CPU-owned state/control event
// families present in eventTypes. A pid/thread selector on an event_search
// filters the incidental emitter task, not ownership of these rows. Keeping
// this classification in tracequery lets the LLM tool and tracediag enforce
// one identical fail-loud contract.
func CPUGlobalEventSearchTypes(eventTypes []EventType) []EventType {
	seen := map[EventType]bool{}
	for _, eventType := range eventTypes {
		switch eventType {
		case EventCPUFrequency, EventCPUFrequencyLimit, EventCPUIdle, EventClockSetRate:
			seen[eventType] = true
		}
	}
	out := make([]EventType, 0, len(seen))
	for eventType := range seen {
		out = append(out, eventType)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
