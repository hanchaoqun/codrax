package orchestrator

import "github.com/hanchaoqun/codrax/internal/types"

func (o *Orchestrator) ingestExtractConsumedArtifacts(
	node *types.TaskNode,
	view *types.ArtifactReadinessView,
) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || node == nil || view == nil {
		return
	}
	records := types.ConsumedNodeArtifactRecordsFromReadiness(node.ID, *view)
	if len(records) == 0 {
		return
	}
	o.busCtx.Mutable.EvidenceClosure().IngestEvidenceReducerInput(types.EvidenceReducerInput{
		Class:         types.EvidenceReducerInputNodeArtifactDelta,
		NodeArtifacts: records,
	}, o.busCtx.RepoRoot)
}
