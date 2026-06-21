package orchestrator

import (
	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
)

func appendStageOutputEvidenceToMutable(mut *types.MutableState, items []types.EvidenceItem) {
	if mut == nil || len(items) == 0 {
		return
	}
	existing := mut.EmittedEvidence()
	var delta []types.EvidenceItem
	for _, item := range items {
		merged, changed := agent.MergeEvidenceItemsIfChanged(existing, []types.EvidenceItem{item})
		if !changed {
			continue
		}
		delta = append(delta, item)
		existing = merged
	}
	if len(delta) > 0 {
		mut.AppendEvidence(delta)
	}
}
