package types

import (
	"context"
	"crypto/sha256"
	"encoding/json"
)

const ToolRepairCodeTraceEventSelectionSourceRequired = "trace_query_event_selection_source_required"

// TraceEventSelectionAcquisitionRef is a native, investigation-local execution
// receipt, not answer truth or a new population contract. A rejected selection
// must actually be retried before previews can encourage completion. Filters
// remain repairable: a genuine source quote does not prove their semantics.
// Immutable private fields survive Go copies but cannot be restored from JSON.
type TraceEventSelectionAcquisitionRef struct {
	source   TraceQuerySourceReadRef
	scope    string
	pending  bool
	snapshot [32]byte
	prepared *traceIntervalNavigation
	sequence uint64
}

func traceEventSelectionScope(params json.RawMessage) string {
	var args map[string]json.RawMessage
	if json.Unmarshal(params, &args) != nil || args == nil {
		return ""
	}
	window := make(map[string]json.RawMessage)
	for _, key := range []string{"time_start", "time_end", "line_start", "line_end"} {
		if value, ok := args[key]; ok {
			window[key] = value
		}
	}
	raw, _ := json.Marshal(window)
	return string(raw)
}

func traceEventSelectionSnapshot(result ToolResult) [32]byte {
	raw, _ := json.Marshal(result.Observations)
	return sha256.Sum256(raw)
}

// StampTraceEventSelectionAcquisition is called at the real native producer's
// tail, after normal source/preparation stamps. Preparing a source alone can
// record a rejection, but never prove the query ran. Success needs a current
// native complete census, including a measured zero. Display paging does not
// prevent a complete count, nor does this receipt prove all members delivered.
func (m *MutableState) StampTraceEventSelectionAcquisition(source TraceQuerySourceReadRef, result *ToolResult, params json.RawMessage) {
	if result == nil {
		return
	}
	result.TraceEventSelectionAcquisition = TraceEventSelectionAcquisitionRef{}
	if m == nil || CanonicalToolName(result.ToolName) != "trace_query" || result.ReusedFromRunMemo || result.TraceViewCancellation != nil {
		return
	}
	if _, current := m.resolveTraceQuerySourceIdentity(source); !current {
		return
	}
	scope := traceEventSelectionScope(params)
	if scope == "" {
		return
	}
	ref := TraceEventSelectionAcquisitionRef{source: source, scope: scope}
	if !result.Success && result.Repair != nil && result.Repair.Code == ToolRepairCodeTraceEventSelectionSourceRequired {
		ref.pending = true
		result.TraceEventSelectionAcquisition = issueTraceEventSelectionRef(ref)
		return
	}
	if !result.Success {
		return
	}
	nativePath, native := m.resolveTraceQuerySourceIdentity(result.TraceQuerySourceRead)
	native = native && nativePath == source.path && source.identity.SameVersion(result.TraceQuerySourceRead.identity)
	if !native {
		prepared := result.TraceQueryWindowReplay.preparedSource
		path, _, current := m.resolveTracePreparedQuerySource(context.Background(), prepared)
		if !current || path != source.path {
			return
		}
		ref.prepared = prepared
	}
	for _, row := range result.Observations {
		if IsValidTraceEventSearchInventoryRecord(row) && row.EventSearchInventory.Coverage.ScopeComplete && row.EventSearchInventory.Coverage.EnumerationComplete {
			ref.snapshot = traceEventSelectionSnapshot(*result)
			result.TraceEventSelectionAcquisition = issueTraceEventSelectionRef(ref)
			return
		}
	}
}

// PendingTraceEventSelectionRepair compiles this investigation's real call
// history, in native call order. A later rejection reopens the debt; one scope's success
// cannot pay another's. Source replacement keeps old debt pending, while a new
// run epoch cannot import it. Previews, model facts and serialized successes
// carry no native execution receipt. This is not a global census requirement.
func (m *MutableState) PendingTraceEventSelectionRepair(results []ToolResult) *ToolRepair {
	if m == nil {
		return nil
	}
	var pending []TraceEventSelectionAcquisitionRef
	for _, ref := range m.traceEventSelectionHistory(results) {
		if ref.pending {
			pending = append(pending, ref)
			continue
		}
		if _, current := m.resolveTraceQuerySourceIdentity(ref.source); !current {
			continue
		}
		if ref.prepared != nil {
			if _, _, current := m.resolveTracePreparedQuerySource(context.Background(), ref.prepared); !current {
				continue
			}
		}
		remaining := pending[:0]
		for _, debt := range pending {
			if debt.source.path != ref.source.path || !debt.source.identity.SameVersion(ref.source.identity) || debt.scope != ref.scope {
				remaining = append(remaining, debt)
			}
		}
		pending = remaining
	}
	if len(pending) == 0 {
		return nil
	}
	return &ToolRepair{Code: "trace_query_event_selection_execution_pending", Hint: "A requested population query was rejected before execution and has not been successfully retried for the same capture generation and original time/line range. Repair event_field_filters using verbatim request source_quote, then run the native query again. Preview rows remain valid individual observations, not a complete count or member census. A complete native census (including zero matches) clears this execution repair; it does not by itself validate the filter semantics or deliver every paged member. Do not replace the missing query with model aggregate_facts or mark the investigation resolved."}
}
