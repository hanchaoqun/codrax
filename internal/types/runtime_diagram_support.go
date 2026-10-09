package types

import "sync"

// RuntimeDiagramSupportResolver projects supported presentation kinds from
// the canonical, scoped native relation providers. It grants neither an edge
// identity nor a causal permission; those remain with the relation validators.
// The callback keeps types independent of tool/tracequery implementations.
type RuntimeDiagramSupportResolver func(ObservationLedger, *RequestModel) []DiagramKind

var (
	runtimeDiagramSupportMu       sync.RWMutex
	runtimeDiagramSupportResolver RuntimeDiagramSupportResolver
)

// RegisterRuntimeDiagramSupportResolver installs the native resolver before
// any answer-plan compilation or Run dispatch. Production registration is
// fixed at tool package initialization. Nil and repeated registration panic:
// cached Agent/Bus plans must never observe a replacement authority source.
func RegisterRuntimeDiagramSupportResolver(resolver RuntimeDiagramSupportResolver) {
	runtimeDiagramSupportMu.Lock()
	defer runtimeDiagramSupportMu.Unlock()
	if resolver == nil {
		panic("types: runtime diagram support resolver must be non-nil")
	}
	if runtimeDiagramSupportResolver != nil {
		panic("types: runtime diagram support resolver already registered")
	}
	runtimeDiagramSupportResolver = resolver
}

func runtimeSupportedDiagramKinds(ledger ObservationLedger, request *RequestModel) []DiagramKind {
	runtimeDiagramSupportMu.RLock()
	resolver := runtimeDiagramSupportResolver
	runtimeDiagramSupportMu.RUnlock()
	if resolver != nil {
		// An installed provider's empty result is authoritative. A second
		// fallback path must not bypass its scope/target/conflict decisions.
		return resolver(ledger, request)
	}
	// Types-only consumers retain the existing wakeup capability. Native
	// tool consumers use the shared provider pool above, including wakeups.
	if len(RuntimeWakeupDiagramEvents(ledger, request)) > 0 {
		return []DiagramKind{DiagramSequence, DiagramFlow, DiagramCallDAG}
	}
	return nil
}
