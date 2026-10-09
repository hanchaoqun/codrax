package types

import "encoding/json"

// TraceQueryWindowReplayRef retains one successful native call, not a model
// plan. It shares the physical source and turn epoch with the read receipt.
// Parameters have already passed the producer schema and effective defaults.
type TraceQueryWindowReplayRef struct {
	source TraceQuerySourceReadRef
	params string
	// Independent navigation does not grant physical reads or change the
	// original replay contract (which may change only window endpoints).
	nativeNavigation *traceIntervalNavigation
}

func (m *MutableState) StampTraceQueryWindowReplay(ref TraceQuerySourceReadRef, result *ToolResult, params json.RawMessage) {
	if m == nil || result == nil || !result.Success || result.TraceViewCancellation != nil ||
		len(params) == 0 || len(params) > 16<<10 || !json.Valid(params) {
		return
	}
	// Navigation views intentionally have no hard causal/raw-read authority.
	// Their native single-physical-source roster still proves a query occurred.
	// Mint only the replay ticket; never register or upgrade source-read rights.
	physical, identity, ok := traceSourcePhysicalPath(result.TraceQuerySourceRead.physicalSource)
	if !ok || physical != ref.path || !ref.identity.SameVersion(identity) || result.ReusedFromRunMemo {
		return
	}
	m.mu.RLock()
	current := ref.generation != nil && ref.generation == m.traceSourceReadGeneration
	m.mu.RUnlock()
	if current && CanonicalToolName(result.ToolName) == "trace_query" {
		// Never replay through a mutable symlink after validating its former
		// destination. The receipt carries the canonical physical coordinate.
		var args map[string]json.RawMessage
		if json.Unmarshal(params, &args) != nil || args == nil {
			return
		}
		args["path"], _ = json.Marshal(ref.path)
		bound, err := json.Marshal(args)
		if err == nil {
			result.TraceQueryWindowReplay = TraceQueryWindowReplayRef{source: ref, params: string(bound), nativeNavigation: result.TraceQueryWindowReplay.nativeNavigation}
		}
	}
}

// ResolveTraceQueryWindowReplay rechecks actual file identity and the current
// run; a source path, JSON roundtrip or a stale memo alone is insufficient.
func (m *MutableState) ResolveTraceQueryWindowReplay(ref TraceQueryWindowReplayRef) (string, json.RawMessage, bool) {
	if m == nil || ref.source.generation == nil || ref.params == "" {
		return "", nil, false
	}
	path, identity, ok := traceSourcePhysicalPath(ref.source.path)
	if !ok || path != ref.source.path || !ref.source.identity.SameVersion(identity) {
		return "", nil, false
	}
	m.mu.RLock()
	current := ref.source.generation == m.traceSourceReadGeneration
	m.mu.RUnlock()
	if !current {
		return "", nil, false
	}
	return path, json.RawMessage(ref.params), true
}
