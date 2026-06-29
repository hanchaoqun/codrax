package orchestrator

import (
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func sourceInventoryAuthoritySnapshotForReadScheduler(ctx *types.BusContext, ir *types.AnalysisIR, observation types.SourceInventoryObservation) types.SourceInventoryAuthoritySnapshot {
	if ctx == nil || ctx.Mutable == nil {
		return types.SourceInventoryAuthoritySnapshot{}
	}
	if ir == nil {
		ir = ctx.AnalysisIR
	}
	if ir == nil {
		return types.SourceInventoryAuthoritySnapshot{}
	}
	facts := ctx.Mutable.StableInvestigationAggregateFacts()
	snapshot := types.BuildSourceInventoryAuthoritySnapshot(types.SourceInventoryAuthoritySnapshotInput{
		Observation:               observation,
		RequestModel:              ir.RequestModel,
		ExistingAggregateFacts:    facts,
		AcceptedExactUniverse:     tool.SourceInventoryAcceptedClosureCoversExactUniverse(ctx, facts),
		AcceptedRequestedUniverse: tool.SourceInventoryAcceptedClosureCoversRequestedUniverse(ctx, facts),
		RequiredFiles:             ir.EvidencePlan.RequiredFiles,
		MaxPrincipalRows:          32,
		MaxSupportRows:            16,
		MaxAuditRows:              8,
	})
	return types.NormalizeSourceInventoryAuthoritySnapshot(snapshot)
}
