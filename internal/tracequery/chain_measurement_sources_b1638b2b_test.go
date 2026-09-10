package tracequery

import (
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1638B2BActualChainUsesOwnLocalTimeline(t *testing.T) {
	idx := runsplitF2ProbeTrace(t)
	q := Query{View: "wakeup_chain", PID: 100, TimeStart: 1, TimeEnd: 1.2, MaxDepth: 4, MinDurationMs: .05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 100}
	result := Run(idx, q)
	if result.WakeupChain == nil || len(result.WakeupChain.CausalImpacts) < 2 {
		t.Fatal("fixture must construct actual local chain occurrences")
	}
	partitions := map[string]bool{}
	for _, impact := range result.WakeupChain.CausalImpacts {
		if impact.TotalMs <= 0 {
			continue
		}
		sources := impact.MeasurementSources
		if sources == nil || sources.HasUnknown || len(sources.Domains) != 1 {
			t.Fatalf("actual local timeline origin lost: tid=%d window=%+v sources=%+v", impact.Thread.PID, impact.Window, sources)
		}
		domain := sources.Domains[0]
		if domain.TargetTID != impact.Thread.PID || domain.Method != "thread_timeline" || domain.WindowStartTs != impact.Window.StartTs || domain.WindowEndTs != impact.Window.EndTs {
			t.Fatalf("occurrence borrowed parent/sibling timeline: %+v / %+v", impact, domain)
		}
		partitions[domain.PartitionID] = true
		// Re-query the exact native local timeline rather than deriving its
		// identity from the impact's duration or physical account key.
		local := q
		local.View, local.PID, local.Thread = "thread_timeline", impact.Thread.PID, impact.Thread.Comm
		local.TimeStart, local.TimeEnd = impact.Window.StartTs, impact.Window.EndTs
		tl := Run(idx, local).Timeline
		if tl == nil || tl.MeasurementDomain == nil || !reflect.DeepEqual(domain, *tl.MeasurementDomain) {
			t.Fatalf("source differs from its native local producer: %+v / %+v", domain, tl)
		}
	}
	if len(partitions) < 2 {
		t.Fatal("fixture did not distinguish recursive local partitions")
	}
	// The node mirror and public occurrence own separate source storage.
	for _, node := range result.WakeupChain.Nodes {
		if node.Impact != nil && node.Impact.MeasurementSources != nil {
			node.Impact.MeasurementSources.Domains[0].PartitionID = "mutation"
		}
	}
	for _, impact := range result.WakeupChain.CausalImpacts {
		if impact.MeasurementSources != nil && impact.MeasurementSources.Domains[0].PartitionID == "mutation" {
			t.Fatal("node mirror aliases the public occurrence source")
		}
	}
}

func TestB1638B2BAggregateKeepsAllMemberSources(t *testing.T) {
	chain := &ChainResult{}
	for i := 0; i < 40; i++ {
		d := types.TraceSchedulerMeasurementDomain{Version: 1, Method: "thread_timeline", TargetTID: 7, WindowStartTs: float64(i + 1), WindowEndTs: float64(i+1) + .01, PartitionID: "native"}
		chain.CausalImpacts = append(chain.CausalImpacts, WakeupCausalImpact{Thread: ThreadRef{PID: 7, Comm: "worker"}, ChainDepth: 1, OnChain: true, DominantState: string(StateSSleep), TotalMs: 10, SleepMs: 10, Window: TimeWindow{StartTs: d.WindowStartTs, EndTs: d.WindowEndTs}, MeasurementSources: types.TraceSchedulerMeasurementSourcesFromDomain(&d)})
	}
	aggregates := aggregateWakeupCausalImpacts(chain)
	if len(aggregates) != 1 || aggregates[0].OccurrenceCount != 40 {
		t.Fatal("native aggregate changed")
	}
	s := aggregates[0].MeasurementSources
	if s == nil || s.HasUnknown || len(s.Domains) != 40 {
		t.Fatalf("full pre-display-cap source inventory lost: %+v", s)
	}
	chain.CausalImpacts[0].MeasurementSources = nil
	s = aggregateWakeupCausalImpacts(chain)[0].MeasurementSources
	if s == nil || !s.HasUnknown || len(s.Domains) != 39 {
		t.Fatalf("unknown contributor borrowed sibling source: %+v", s)
	}
}
