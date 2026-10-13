package types

import (
	"sort"
	"sync"
)

// Owned by the existing investigation source epoch, shared by explorer forks.
// Only small immutable receipts are retained, never observations or results.
// Replacing the source epoch starts a fresh ledger; resetting a dispatch does
// not erase an unfinished query. No ledger lock is held while acquiring m.mu.
type traceEventSelectionLedger struct {
	mu      sync.Mutex
	next    uint64
	records map[uint64]TraceEventSelectionAcquisitionRef
}

func issueTraceEventSelectionRef(ref TraceEventSelectionAcquisitionRef) TraceEventSelectionAcquisitionRef {
	ledger := &ref.source.generation.eventSelections
	ledger.mu.Lock()
	ledger.next++
	ref.sequence = ledger.next
	ledger.mu.Unlock()
	return ref
}

func validTraceEventSelectionPublication(result ToolResult, generation *traceSourceReadGeneration) bool {
	ref := result.TraceEventSelectionAcquisition
	if ref.source.generation == nil || ref.source.generation != generation || ref.scope == "" || ref.sequence == 0 {
		return false
	}
	return ref.pending || result.Success && !result.ReusedFromRunMemo && result.TraceViewCancellation == nil &&
		CanonicalToolName(result.ToolName) == "trace_query" && traceEventSelectionSnapshot(result) == ref.snapshot
}

func (m *MutableState) registerTraceEventSelectionAcquisition(result ToolResult) {
	m.mu.RLock()
	generation := m.traceSourceReadGeneration
	m.mu.RUnlock()
	if !validTraceEventSelectionPublication(result, generation) {
		return
	}
	ref := result.TraceEventSelectionAcquisition
	ledger := &generation.eventSelections
	ledger.mu.Lock()
	if ledger.records == nil {
		ledger.records = make(map[uint64]TraceEventSelectionAcquisitionRef)
	}
	ledger.records[ref.sequence] = ref
	ledger.mu.Unlock()
}

func (m *MutableState) traceEventSelectionHistory(results []ToolResult) []TraceEventSelectionAcquisitionRef {
	m.mu.RLock()
	generation := m.traceSourceReadGeneration
	m.mu.RUnlock()
	if generation == nil {
		return nil
	}
	ledger := &generation.eventSelections
	ledger.mu.Lock()
	records := make(map[uint64]TraceEventSelectionAcquisitionRef, len(ledger.records))
	for sequence, ref := range ledger.records {
		records[sequence] = ref
	}
	ledger.mu.Unlock()
	// Unregistered current-call handoffs can be assessed without mutating the
	// ledger. Old Go copies retain their original order, never a new event ID.
	for _, result := range results {
		ref := result.TraceEventSelectionAcquisition
		if _, exists := records[ref.sequence]; !exists && validTraceEventSelectionPublication(result, generation) {
			records[ref.sequence] = ref
		}
	}
	history := make([]TraceEventSelectionAcquisitionRef, 0, len(records))
	for _, ref := range records {
		history = append(history, ref)
	}
	sort.Slice(history, func(i, j int) bool { return history[i].sequence < history[j].sequence })
	return history
}
