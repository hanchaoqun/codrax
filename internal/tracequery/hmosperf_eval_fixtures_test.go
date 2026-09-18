package tracequery

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Keep eval truth tied to the same checked-in bytes the model receives. The
// README/oracle is never copied into the eval's analysis repository.
func hmosperfEvalIndex(t *testing.T, fixture string) *Index {
	t.Helper()
	path := filepath.Join("..", "..", "eval", "fixtures", fixture, "events.systrace")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return buildTraceIndex(t, fixture+".systrace", string(body))
}

func TestHmosperfNativeResourceEvalFixtureFactsAndScope(t *testing.T) {
	idx := hmosperfEvalIndex(t, "hmosperf_native_resource_metadata")
	q := Query{View: "event_search", Pattern: "NativeHook:", TimeStart: 0.0005, TimeEnd: 0.0035, Limit: 32}
	res := Run(idx, q)
	if len(res.Events) != 3 {
		t.Fatalf("want three in-window resource observations, got %+v", res.Events)
	}
	want := []string{
		"NativeHook:AllocEvent resource_end_ts_ns=9223372036854775807 source_heap_size=9007199254740993 source_callchain_id=9007199254740995",
		"NativeHook:FreeEvent resource_end_ts_ns=0 source_heap_size=0 source_callchain_id=-1",
		"NativeHook:MmapEvent resource_end_ts_ns=null source_heap_size=null source_callchain_id=null",
	}
	for i, event := range res.Events {
		if event.SpanAction != "I" || event.SpanName != want[i] {
			t.Fatalf("resource %d lost precise metadata/instant semantics: %+v", i, event)
		}
	}
	counters := Run(idx, Query{View: "event_search", Pattern: "HeapSize", TimeStart: q.TimeStart, TimeEnd: q.TimeEnd, Limit: 32})
	if len(counters.Events) != 3 {
		t.Fatalf("want three independent heap counter observations: %+v", counters.Events)
	}
	for i, wantValue := range []string{"8192", "4096", "4096"} {
		if counters.Events[i].SpanAction != "C" || counters.Events[i].SpanValue != wantValue {
			t.Fatalf("counter %d changed source value: %+v", i, counters.Events[i])
		}
	}
	q.PID = 100
	rank := BuildRootCauseRank(idx, q)
	for _, item := range append(rank.Items, rank.AbsorbedItems...) {
		if strings.Contains(item.SpanName, "NativeHook:") {
			t.Fatalf("resource lifetime became execution/root-cause evidence: %+v", item)
		}
	}
}

func TestHmosperfBusinessIOEvalFixtureDiscoveryAndCausalRulers(t *testing.T) {
	idx := hmosperfEvalIndex(t, "hmosperf_business_io_chain")
	spans, _ := FindSpanWindows(idx, Query{SpanName: "OpenDocument"}, 10)
	if len(spans) != 1 || math.Abs(spans[0].StartTs-1.0) > 1e-9 || math.Abs(spans[0].EndTs-1.05) > 1e-9 {
		t.Fatalf("business window is not discoverable: %+v", spans)
	}
	q := Query{PID: 100, TimeStart: spans[0].StartTs, TimeEnd: spans[0].EndTs, MaxDepth: 4, Limit: 32}
	stats := ComputeWindowStats(idx, q)
	var request, background *IOLatencySummary
	for i := range stats.ioLatencyCensus {
		item := &stats.ioLatencyCensus[i]
		switch item.Sector {
		case 923339752:
			request = item
		case 800000:
			background = item
		}
	}
	if request == nil || !request.CompletionWokeIssuer || request.IssuerBlockedState != string(StateSSleep) || math.Abs(request.DurationMs-35) > 1e-6 || math.Abs(request.IssuerBlockedMs-31) > 1e-6 {
		t.Fatalf("request lost distinct residence and response-impact rulers: %+v", request)
	}
	if background == nil || background.CompletionWokeIssuer || math.Abs(background.DurationMs-47) > 1e-6 {
		t.Fatalf("longer background request gained response authority: %+v", background)
	}
	chain := BuildWakeupChain(idx, q)
	foundIRQ, foundWorker := false, false
	for _, edge := range chain.Edges {
		foundIRQ = foundIRQ || (edge.Waker.PID == 80 && edge.Wakee.PID == 200)
		foundWorker = foundWorker || (edge.Waker.PID == 200 && edge.Wakee.PID == 100)
	}
	if !foundIRQ || !foundWorker {
		t.Fatalf("missing precise completion/worker dependency edges: %+v", chain.Edges)
	}
	rank := BuildRootCauseRank(idx, q)
	foundIO := false
	for _, item := range rank.Items {
		if item.Type == "io_latency" && item.Thread.PID == 200 {
			foundIO = item.ChainRelevance == "on_chain" && item.ResourceCompletionClosure && math.Abs(item.EffectiveImpactMs-31) < 1e-6
		}
		if item.Thread.PID == 900 && item.ChainRelevance == "on_chain" {
			t.Fatalf("background request entered target chain: %+v", item)
		}
	}
	if !foundIO {
		t.Fatalf("S-state IO absent from on-chain response ranking: %+v", rank.Items)
	}
}
