package tool

import (
	"math"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestB1717BackgroundStatePublicationBoundaries(t *testing.T) {
	for _, typ := range []string{
		"running", "fragmented_running", "runnable", "runnable_wait",
		"fragmented_runnable_wait", "scheduler_latency", "sleep", "s_sleep",
		"sleep_wait", "fragmented_sleep_wait", "d", "d_sleep", "d_state", "io_wait",
	} {
		t.Run(typ, func(t *testing.T) {
			original := tracequery.RootCauseRankItem{
				Type: typ, ChainRelevance: "background", Causality: "background",
				ImpactMs: 7, ProjectedImpactMs: 7, EffectiveImpactMs: 7, Score: 3.5,
				CumulativeImpactMs: 61.5, StartTs: 1, EndTs: 2,
				RunningMs: 19.5, RunnableMs: 19.5, SleepMs: 19.5, DStateMs: 19.5, IOWaitMs: 19.5,
			}
			before := original
			published := traceQueryRootCauseRankWireItemForPublicationInUniverse(original, traceQueryPriorityOpenArtifactUniverse())
			if published.ImpactMs != 19.5 || published.ProjectedImpactMs != 19.5 || published.EffectiveImpactMs != 0 {
				t.Fatalf("pure state must expose its own measure without causal credit: %+v", published)
			}
			if published.Score != before.Score || published.CumulativeImpactMs != 61.5 ||
				published.StartTs != 1 || published.EndTs != 2 || published.ChainRelevance != "background" {
				t.Fatalf("unrelated ordering, cumulative or identity changed: %+v", published)
			}
			if !reflect.DeepEqual(original, before) {
				t.Fatal("publication mutated engine-owned input")
			}
			if again := traceQueryRootCauseRankWireItemForPublicationInUniverse(published, traceQueryPriorityOpenArtifactUniverse()); !reflect.DeepEqual(again, published) {
				t.Fatalf("publication is not idempotent: %+v", again)
			}
		})
	}
	for _, typ := range []string{"d_state_or_io_wait", "fragmented_d_state_or_io_wait"} {
		for _, parts := range [][2]float64{{19.5, 0}, {0, 19.5}, {11.5, 8}} {
			item := tracequery.RootCauseRankItem{Type: typ, DStateMs: parts[0], IOWaitMs: parts[1], ImpactMs: 7, CumulativeImpactMs: 100}
			if got, ok := traceQueryBackgroundStateMeasurement(item); !ok || got != 19.5 {
				t.Fatalf("typed D/IO partition %s %v: got=%v ok=%v", typ, parts, got, ok)
			}
		}
	}
}

func TestB1717BackgroundMeasurementDoesNotGuessFromOtherFields(t *testing.T) {
	for _, typ := range []string{"priority_inversion_candidate", "low_frequency", "io_pressure", "io_latency", "io_burst_episode", "blocking_span", "jit_compilation", "future_state", ""} {
		item := tracequery.RootCauseRankItem{Type: typ, DominantState: "d_sleep", DStateMs: 19.5, IOWaitMs: 8, CumulativeImpactMs: 100, ImpactMs: 7}
		if value, ok := traceQueryBackgroundStateMeasurement(item); ok {
			t.Fatalf("%q borrowed a scheduler measure for a non-state row: %v", typ, value)
		}
	}
	for _, value := range []float64{0, -1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		item := tracequery.RootCauseRankItem{Type: "runnable_wait", RunnableMs: value, CumulativeImpactMs: 100, ImpactMs: 7, ProjectedImpactMs: 7, ChainRelevance: "background"}
		if _, ok := traceQueryBackgroundStateMeasurement(item); ok {
			t.Fatalf("invalid/absent measurement %v was accepted", value)
		}
		published := traceQueryRootCauseRankWireItemForPublicationInUniverse(item, traceQueryPriorityOpenArtifactUniverse())
		if published.ImpactMs != 7 || published.ProjectedImpactMs != 7 {
			t.Fatalf("legacy missing measurement was guessed from another field: %+v", published)
		}
	}
	for _, parts := range [][2]float64{{-1, 20.5}, {20.5, -1}, {math.NaN(), 19.5}, {math.MaxFloat64, math.MaxFloat64}} {
		if value, ok := traceQueryBackgroundStateMeasurement(tracequery.RootCauseRankItem{Type: "d_state_or_io_wait", DStateMs: parts[0], IOWaitMs: parts[1]}); ok {
			t.Fatalf("invalid partition %v was accepted as %v", parts, value)
		}
	}
	for _, relevance := range []string{"on_chain", "adjacent"} {
		item := tracequery.RootCauseRankItem{Type: "io_wait", ChainRelevance: relevance, IOWaitMs: 19.5, ImpactMs: 7, ProjectedImpactMs: 7, EffectiveImpactMs: 7, Score: 3.5}
		published := traceQueryRootCauseRankWireItemForPublicationInUniverse(item, traceQueryPriorityOpenArtifactUniverse())
		if !reflect.DeepEqual(item, published) {
			t.Fatalf("%s lane changed: before=%+v after=%+v", relevance, item, published)
		}
	}
}
