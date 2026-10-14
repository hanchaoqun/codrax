package tool

import "github.com/hanchaoqun/codrax/internal/types"

func nativeSequenceOrderRepairCandidates(ctx *types.BusContext, failures []types.AnswerDiagramRelationRepairFailure) []types.AnswerDiagramRelationRepairCandidate {
	rows := runtimeDiagramRelationsForContext(ctx)
	var out []types.AnswerDiagramRelationRepairCandidate
	for _, f := range failures {
		if f.Issue != diagramNativeSequenceEventOrderConflict || f.TargetCarrier != types.AnswerDiagramRelationRepairCarrierPriorAnchor || !f.AllowsAction("remove") {
			continue
		}
		for _, row := range rows {
			if row.eventOrder == nil || row.FromNode != f.FromNode || row.ToNode != f.ToNode || row.Kind != f.RelationKind || row.FromIdentity != f.FromIdentity || row.ToIdentity != f.ToIdentity {
				continue
			}
			out = appendRuntimeDiagramRepairCandidates(out, []RuntimeDiagramRelation{row}, []string{f.BlockID}, types.AnswerDiagramRelationRepairDeltaMaxEntries)
		}
	}
	return out
}
