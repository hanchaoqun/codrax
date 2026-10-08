package tool

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/types"
)

func completionRuntimeMeasurementMemberSets(ctx *types.BusContext, selected []types.AnswerRuntimeMeasurementReceipt) ([]types.AnswerRuntimeMeasurementReceipt, error) {
	if len(selected) == 0 {
		return nil, nil
	}
	failure := func() ([]types.AnswerRuntimeMeasurementReceipt, error) {
		return nil, fmt.Errorf("emit_investigation_complete rejected: runtime_measurement_member_sets must select complete current runtime populations matching the requested source, target and full window; partial or unrelated tables cannot replace required members. %s", types.RuntimeMeasurementMemberSetTeaching)
	}
	if ctx == nil || ctx.AnalysisIR == nil || len(selected) > 16 {
		return failure()
	}
	rm := &ctx.AnalysisIR.RequestModel
	authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(ctx, types.ObservationLedger{})
	if !types.RuntimeMeasurementMemberSetDomain(rm, authority) {
		return failure()
	}
	contract := types.BuildAnswerSemanticViewForBusContext(ctx).RuntimeMeasurementContract
	bound, valid := types.RuntimeMeasurementMemberSetSelections(selected, contract, rm)
	if !valid {
		return failure()
	}
	needed := 0
	if p := rm.RequestedAnswerDimensions; p != nil {
		for _, d := range p.Dimensions {
			if d.Required && d.Role == types.RequestedAnswerDimensionMemberSet {
				needed++
			}
		}
	}
	if needed > len(bound) {
		return failure()
	}
	return bound, nil
}
