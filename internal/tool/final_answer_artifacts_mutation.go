package tool

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/types"
)

// resolveTraceFindingForEmit enforces the opt-in contract before the shared
// document persist path is allowed to commit either artifact.
func resolveTraceFindingForEmit(ctx *types.BusContext, submitted *types.TraceFindingV1, patch bool) (*types.TraceFindingV1, error) {
	if ctx == nil || ctx.Mutable == nil {
		return nil, nil
	}
	contract := ctx.Mutable.TraceFindingContract()
	if contract == nil || !contract.Required {
		if submitted != nil {
			return nil, fmt.Errorf("trace_finding is not enabled for this request")
		}
		return nil, nil
	}
	finding := submitted
	if patch && finding == nil {
		finding = ctx.Mutable.TraceFinding()
	}
	if err := tracefinding.Validate(finding, contract); err != nil {
		return nil, err
	}
	return finding, nil
}

// resolveTraceRootCauseReportForEmit validates the user-facing JSON sidecar.
// Patch emits inherit the last accepted report when they only repair answer
// blocks, but a first/full emit must submit the report explicitly.
func resolveTraceRootCauseReportForEmit(ctx *types.BusContext, submitted *types.TraceRootCauseReportV1, patch bool) (*types.TraceRootCauseReportV1, error) {
	if ctx == nil || ctx.Mutable == nil {
		return nil, nil
	}
	contract := ctx.Mutable.TraceFindingContract()
	if contract == nil || !contract.RootCauseReportRequired {
		if submitted != nil {
			return nil, fmt.Errorf("trace_root_causes is not enabled for this request")
		}
		return nil, nil
	}
	report := submitted
	if patch && report == nil {
		report = ctx.Mutable.TraceRootCauseReport()
	}
	return types.NormalizeAndValidateTraceRootCauseReport(report)
}
