package tool

import "github.com/hanchaoqun/codrax/internal/types"

func init() {
	types.RegisterRuntimeDiagramSupportResolver(runtimeDiagramSupportedKinds)
}

// Use exactly the same source/window/target/conflict-qualified relations as
// authoring recipes, emit, repair and post-validation. This is presentation
// support only: observe never gains call, wakeup or containment semantics.
func runtimeDiagramSupportedKinds(ledger types.ObservationLedger, request *types.RequestModel) []types.DiagramKind {
	supported := map[types.DiagramKind]bool{}
	for _, row := range RuntimeDiagramRelations(ledger, request) {
		switch row.Kind {
		case types.DiagramRelObserve, types.DiagramRelWakeup:
			supported[types.DiagramSequence] = true
			supported[types.DiagramFlow] = true
			supported[types.DiagramCallDAG] = true
		case types.DiagramRelContain:
			supported[types.DiagramFlow] = true
			supported[types.DiagramCallDAG] = true
		}
	}
	var out []types.DiagramKind
	for _, kind := range []types.DiagramKind{types.DiagramSequence, types.DiagramFlow, types.DiagramCallDAG} {
		if supported[kind] {
			out = append(out, kind)
		}
	}
	return out
}
