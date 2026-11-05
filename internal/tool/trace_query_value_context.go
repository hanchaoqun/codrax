package tool

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

const traceComputeSupplyEstimateCaliber = "Frequency-weighted model relative to this window's observed maximum frequency, not measured throughput or physical capacity. Known running coverage requires a positive governing sample, not nearest-following estimation. Missing frequency is unmeasured; fallback weight 1.0 and a numeric zero loss do not prove no frequency loss or no capacity constraint. core_limited is a residual approximation."

// Preserve the engine's calculation caliber, without recomputing values,
// parsing caveat prose or turning a fallback into a measured physical state.
func traceQueryComputeSupplyValueContext(bal *tracequery.ComputeSupplyBalance) []types.ObservationValueContext {
	if bal == nil {
		return nil
	}
	out := []types.ObservationValueContext{{
		Fields: "delivered_cpu_ms,low_freq_loss_cpu_ms,core_limited_cpu_ms,supply_ratio",
		Method: "frequency_weighted_estimate",
		Detail: traceComputeSupplyEstimateCaliber,
	}}
	for _, cpu := range bal.PerCPU {
		detail := fmt.Sprintf("cpu=%d frequency_known=%t max_frequency_khz=%d", cpu.CPU, cpu.FrequencyKnown, cpu.MaxFrequencyKHz)
		if cpu.FrequencyCoverageKnown {
			detail += fmt.Sprintf(" frequency_known_running_ms=%g frequency_unknown_running_ms=%g", cpu.FrequencyKnownRunningMs, cpu.FrequencyUnknownRunningMs)
		} else {
			detail += " running-frequency coverage unmeasured (not published)"
		}
		if cpu.FrequencyClusterDonorCPU != nil {
			detail += fmt.Sprintf(" same_cluster_donor_cpu=%d membership_source=%s", *cpu.FrequencyClusterDonorCPU, cpu.FrequencyClusterDonorSource)
		}
		out = append(out, types.ObservationValueContext{Fields: "delivered_cpu_ms,low_freq_loss_cpu_ms,supply_ratio", Method: "frequency_input_coverage", Detail: detail})
	}
	for _, caveat := range bal.Caveats {
		out = append(out, types.ObservationValueContext{Method: "source_calculation_limit", Detail: caveat})
	}
	return out
}
