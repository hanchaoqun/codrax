package tool

import (
	"math"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// AB2: an exact occurrence interval carried by a rank item must survive the
// tool projection unchanged. NearestChainWindow is a topology anchor for a
// different purpose and may not replace the value-owning interval.
func TestTraceQueryTypedObservationsPreserveRankOccurrenceInterval(t *testing.T) {
	const (
		start = 13762.992415
		end   = 13763.008173
	)
	result := tracequery.Result{RootCauseRank: &tracequery.RootCauseRankResult{
		Target: tracequery.ThreadRef{PID: 17267, Comm: ".ugc.aweme.lite"},
		Items: []tracequery.RootCauseRankItem{{
			Tier:               tracequery.RootCauseTierContextOnly,
			Type:               "pacing_idle",
			Thread:             tracequery.ThreadRef{PID: 17267, Comm: ".ugc.aweme.lite"},
			StartTs:            start,
			EndTs:              end,
			ImpactMs:           15.758,
			ProjectedImpactMs:  15.758,
			CumulativeImpactMs: 15.758,
			EffectiveImpactMs:  15.758,
			LineStart:          23092,
			LineEnd:            24545,
			Source:             "wakeup_chain",
			Causality:          tracequery.RootCauseCausalitySelfWallClock,
			ChainRelevance:     "on_chain",
			NearestChainWindow: tracequery.TimeWindow{StartTs: 13762.984951, EndTs: 13762.985960},
			Summary:            "frame-pacing idle",
		}},
	}}

	records := traceQueryTypedObservations(result, "customer.systrace", "payload", "raw", "", time.Unix(0, 0).UTC())
	for _, record := range records {
		if record.Object != "pacing_idle" {
			continue
		}
		if math.Abs(record.Span.StartTs-start) > 1e-9 || math.Abs(record.Span.EndTs-end) > 1e-9 {
			t.Fatalf("rank observation borrowed a topology anchor instead of preserving its occurrence interval: %+v", record)
		}
		if math.Abs((record.Span.EndTs-record.Span.StartTs)*1000-15.758) > 0.001 {
			t.Fatalf("rank observation value and interval no longer identify the same occurrence: %+v", record)
		}
		return
	}
	t.Fatalf("pacing rank observation missing: %+v", records)
}
