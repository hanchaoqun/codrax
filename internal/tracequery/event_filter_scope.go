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

// EventSearchPatternLimit caps the typed multi-literal OR carrier
// (Query.Patterns). The LLM tool schema's maxItems and the tracediag script
// validator both read this one constant.
const EventSearchPatternLimit = 16

// NormalizeEventSearchPatterns is the shared hard boundary for the typed
// multi-literal event_search carrier (V11-2, colleague_merge_audit §40.58):
// the LLM tool and the deterministic tracediag script validate `patterns`
// through this one function, exactly as ValidateTraceMarkActionFilter serves
// both faces for `trace_mark_actions`. Precise signals only: canonical view
// string equality, an integer length bound, and a trimmed-empty check.
// An absent carrier (len==0) is the typed escape lane — nil, nil, no gate.
// The legacy single Pattern contract is untouched: a vertical bar in Pattern
// remains an ordinary literal, and this function never guesses regex intent
// from caller-authored text. The returned slice is trimmed and case-
// insensitively de-duplicated (first spelling wins) so every consumer echoes
// and matches one canonical set.
func NormalizeEventSearchPatterns(view string, patterns []string) ([]string, error) {
	if len(patterns) == 0 {
		return nil, nil
	}
	if canonical := CanonicalViewName(view); canonical != FallbackViewEventSearch {
		return nil, fmt.Errorf("patterns is only valid for view=%s, got view=%s", FallbackViewEventSearch, canonical)
	}
	if len(patterns) > EventSearchPatternLimit {
		return nil, fmt.Errorf("received %d literals; maximum is %d", len(patterns), EventSearchPatternLimit)
	}
	seen := make(map[string]bool, len(patterns))
	out := make([]string, 0, len(patterns))
	for i, raw := range patterns {
		literal := strings.TrimSpace(raw)
		if literal == "" {
			return nil, fmt.Errorf("literal %d is empty after trimming", i+1)
		}
		key := strings.ToLower(literal)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, literal)
	}
	return out, nil
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
