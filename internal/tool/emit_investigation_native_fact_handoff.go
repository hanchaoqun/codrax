package tool

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/types"
)

type completionNativeFactHandoff struct {
	RuntimeMeasurements []types.AnswerRuntimeMeasurementReceipt `json:"runtime_measurements"`
	Interpretation      *string                                 `json:"interpretation"`
}

func completionNativeFactHandoffSelection(ctx *types.BusContext, p *completionNativeFactHandoff) (*types.InvestigationNativeFactHandoff, error) {
	if p == nil {
		return nil, nil
	}
	fail := func() (*types.InvestigationNativeFactHandoff, error) {
		return nil, fmt.Errorf("emit_investigation_complete rejected: native_fact_handoff requires current published runtime_measurements observation_id/view pairs and an explicit interpretation string (empty only when no independent interpretation is needed). Do not copy values or invent IDs; keep diagnostic hypotheses, missing evidence, source findings and boundaries in interpretation. Omit the optional handoff to retain the legacy reason path")
	}
	if ctx == nil || ctx.Mutable == nil || p.Interpretation == nil {
		return fail()
	}
	h := &types.InvestigationNativeFactHandoff{RuntimeMeasurements: p.RuntimeMeasurements, Interpretation: *p.Interpretation}
	view := types.BuildAnswerSemanticViewForBusContext(ctx)
	if view == nil {
		return fail()
	}
	if _, ok := types.BindInvestigationNativeFactHandoff(h, view.RuntimeMeasurementContract); !ok {
		return fail()
	}
	return h.Clone(), nil
}
