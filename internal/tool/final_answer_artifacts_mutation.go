package tool

import (
	"encoding/json"
	"fmt"
	"strings"

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

// resolveTraceRootCauseReportForEmit validates and binds the optional selector.
// Patch emits inherit the last accepted report when they only repair answer
// blocks. Sidecar errors are returned for diagnostics but never own whether
// the full answer mutation is accepted.
func resolveTraceRootCauseReportForEmit(ctx *types.BusContext, submitted json.RawMessage, patch bool) (*types.TraceRootCauseReportV2, error) {
	if ctx == nil || ctx.Mutable == nil {
		return nil, nil
	}
	contract := ctx.Mutable.TraceFindingContract()
	if contract == nil || !contract.RootCauseReportEnabled {
		if len(strings.TrimSpace(string(submitted))) > 0 && string(submitted) != "null" {
			return nil, fmt.Errorf("trace_root_causes is not enabled for this request")
		}
		return nil, nil
	}
	if len(strings.TrimSpace(string(submitted))) == 0 || string(submitted) == "null" {
		if patch {
			return ctx.Mutable.TraceRootCauseReport(), nil
		}
		return nil, nil
	}
	var selection types.TraceRootCauseReportV2
	if err := json.Unmarshal(submitted, &selection); err != nil {
		if patch {
			return ctx.Mutable.TraceRootCauseReport(), fmt.Errorf("decode optional trace_root_causes selector: %w", err)
		}
		return nil, fmt.Errorf("decode optional trace_root_causes selector: %w", err)
	}
	report, err := tracefinding.BindRootCauseReportSelection(&selection, contract)
	if err != nil && patch {
		return ctx.Mutable.TraceRootCauseReport(), err
	}
	return report, err
}
