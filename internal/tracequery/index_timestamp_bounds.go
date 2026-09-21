package tracequery

// observeTimestampBounds records an already-admitted timestamp in this
// producer's existing observation domain. Presence is independent of a zero
// value, retained Events, known event kinds, and preservation-only carriers.
// Callers retain ownership of filtering and clock-mapping admission.
func (idx *Index) observeTimestampBounds(ts float64) {
	if idx == nil || !isSafeTraceTimestamp(ts) {
		return
	}
	if !idx.timestampBoundsSeen || ts < idx.FirstTs {
		idx.FirstTs = ts
	}
	// Keep the historical upper-envelope comparison, including its zero
	// baseline for legacy signed derived domains; this change does not add
	// support for negative canonical clocks.
	if ts > idx.LastTs {
		idx.LastTs = ts
	}
	idx.timestampBoundsSeen = true
}

func (idx *Index) resetTimestampBounds() {
	idx.FirstTs, idx.LastTs, idx.timestampBoundsSeen = 0, 0, false
}

func (idx *Index) hasTimestampBounds() bool {
	if idx == nil {
		return false
	}
	if idx.timestampBoundsSeen {
		return true
	}
	// Legacy synthetic indexes carry explicit positive-width metadata but
	// predate the parser-owned presence bit. Preserve that compatibility;
	// an empty all-zero index cannot manufacture a determined timestamp.
	return isSafeTraceTimestamp(idx.FirstTs) && isSafeTraceTimestamp(idx.LastTs) &&
		idx.FirstTs >= 0 && idx.LastTs > idx.FirstTs
}
