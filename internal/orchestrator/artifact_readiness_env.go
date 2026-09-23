package orchestrator

import (
	"github.com/hanchaoqun/codrax/internal/types"
)

func (o *Orchestrator) buildArtifactReadinessView(ir *types.AnalysisIR) *types.ArtifactReadinessView {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || ir == nil {
		return nil
	}
	contracts := types.TaskArtifactContractsForNodeType(ir.TaskGraph, types.NodeExtract)
	docRequested := ir.RequestModel.ToolDocumentationRequest != nil
	if len(contracts) == 0 && !docRequested {
		return nil
	}
	closure := o.busCtx.Mutable.EvidenceClosure()
	var ledger types.NodeArtifactLedger
	if closure != nil {
		ledger = closure.NodeArtifactLedger()
	}
	aggregateFacts := o.busCtx.Mutable.StableInvestigationAggregateFacts()
	if turnA := o.busCtx.Mutable.TurnAArtifacts(); turnA != nil {
		aggregateFacts = types.MergeAnswerAggregateFacts(aggregateFacts, turnA.AcceptedAggregateFacts)
	}
	view := types.BuildArtifactReadinessView(types.ArtifactReadinessInput{
		Contracts:      contracts,
		Ledger:         ledger,
		EvidenceItems:  o.busCtx.EvidenceItems,
		AnswerChains:   o.busCtx.AnswerChains,
		AggregateFacts: aggregateFacts,
		Consumer:       types.RuntimeArtifactConsumerExtract,
	})
	if docRequested {
		view.ToolDocumentationReady = o.busCtx.Mutable.HasAcceptedToolDocumentationCompletion(&ir.RequestModel)
	}
	if !view.Active && !docRequested {
		return nil
	}
	return &view
}
