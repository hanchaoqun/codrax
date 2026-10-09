package types

import (
	"encoding/json"
	"sort"
)

// TraceStatisticsRef is producer-owned availability of computation, NOT proof
// that a request is answered. Its immutable payload is safe in fork/handoff
// value copies. Neither fields nor constructor inputs are model JSON fields.
type TraceStatisticsRef struct {
	replay         TraceQueryWindowReplayRef
	availability   string
	physicalSource string
}

type traceStatisticsCandidate struct {
	Raw      []string `json:"raw,omitempty"`
	Computed []string `json:"computed,omitempty"`
}

// NativeTraceStatisticsCandidate is called only after a native producer has
// validated its complete inventory/computation. Domain tokens are optional
// query-view suggestions, not labels parsed from a summary or measurement table.
func NativeTraceStatisticsCandidate(source string, raw, computed []string) TraceStatisticsRef {
	if len(raw)+len(computed) == 0 || len(raw)+len(computed) > 32 {
		return TraceStatisticsRef{}
	}
	b, err := json.Marshal(traceStatisticsCandidate{Raw: raw, Computed: computed})
	if err != nil || len(b) > 4096 {
		return TraceStatisticsRef{}
	}
	return TraceStatisticsRef{availability: string(b), physicalSource: source}
}

// StampTraceStatistics binds the producer's private candidate to the actual
// successful query's current physical generation and effective selectors.
// JSON replay, memo, cancellation and file replacement cannot mint a receipt.
func (m *MutableState) StampTraceStatistics(source TraceQuerySourceReadRef, result *ToolResult, params json.RawMessage) {
	if result == nil {
		return
	}
	candidate, physical := result.TraceStatistics.availability, result.TraceStatistics.physicalSource
	result.TraceStatistics = TraceStatisticsRef{}
	if candidate == "" || !result.Success || result.ReusedFromRunMemo || result.TraceViewCancellation != nil || CanonicalToolName(result.ToolName) != "trace_query" {
		return
	}
	// Some complete streaming statistics producers do not publish an event
	// roster. Validate their declared native input against the held pre-query
	// identity in a private copy; do not give the public result source-read or
	// window-replay rights that it did not otherwise have.
	private := ToolResult{ToolName: result.ToolName, Success: true, TraceQuerySourceRead: TraceQueryPhysicalSourceReadCandidate(physical)}
	m.StampTraceQueryWindowReplay(source, &private, params)
	if _, _, ok := m.ResolveTraceQueryWindowReplay(private.TraceQueryWindowReplay); !ok {
		return
	}
	result.TraceStatistics = TraceStatisticsRef{replay: private.TraceQueryWindowReplay, availability: candidate}
}

// TraceStatisticsAvailability only controls advisory closure suggestions. A
// pending computation does not demand another query: raw-only answers and
// explicit insufficient-data completion remain available. ComputedViews means
// statistics are available, never that all semantic questions are resolved.
type TraceStatisticsAvailability struct {
	PendingViews  []string
	ComputedViews []string
}

func (a TraceStatisticsAvailability) HasPending() bool { return len(a.PendingViews) != 0 }

func (m *MutableState) TraceStatisticsAvailability(results []ToolResult) TraceStatisticsAvailability {
	// A statistic may substitute only within the same physical source, exact
	// window and selector set. In particular all-owner is not one PID; neither
	// broader windows nor partial filter populations count as an equivalent.
	raw, computed := map[string]string{}, map[string]string{}
	for _, result := range results {
		if !result.Success || result.TraceViewCancellation != nil {
			continue
		}
		ref := result.TraceStatistics
		path, params, ok := m.ResolveTraceQueryWindowReplay(ref.replay)
		if !ok || ref.availability == "" {
			continue
		}
		var shape map[string]json.RawMessage
		var candidate traceStatisticsCandidate
		if json.Unmarshal(params, &shape) != nil || json.Unmarshal([]byte(ref.availability), &candidate) != nil {
			continue
		}
		// Display limits do not change the computed population. All other
		// selectors remain exact; view itself differs for equivalent producers.
		for _, key := range []string{"view", "limit", "source", "path"} {
			delete(shape, key)
		}
		shapeJSON, err := json.Marshal(shape)
		if err != nil {
			continue
		}
		prefix := path + "\x00" + string(shapeJSON) + "\x00"
		for _, view := range candidate.Raw {
			if view != "" {
				raw[prefix+view] = view
			}
		}
		for _, view := range candidate.Computed {
			if view != "" {
				computed[prefix+view] = view
			}
		}
	}
	var out TraceStatisticsAvailability
	pendingSeen, computedSeen := map[string]bool{}, map[string]bool{}
	for key, view := range raw {
		if _, ok := computed[key]; !ok && !pendingSeen[view] {
			out.PendingViews = append(out.PendingViews, view)
			pendingSeen[view] = true
		}
	}
	for _, view := range computed {
		if !computedSeen[view] {
			out.ComputedViews = append(out.ComputedViews, view)
			computedSeen[view] = true
		}
	}
	sort.Strings(out.PendingViews)
	sort.Strings(out.ComputedViews)
	return out
}
