package tracequery

import (
	"math"
	"math/rand"
	"testing"
)

func TestB1607BRequestIndexMatchesUnboundedLinearPopulation(t *testing.T) {
	random := rand.New(rand.NewSource(1607))
	var requests []*binderInventoryRequest
	for i := 0; i < 1200; i++ {
		start := random.Float64() * 100
		request := &binderInventoryRequest{send: Event{Ts: start, Line: i + 1}, source: []string{"a", "b"}[i%2]}
		if i%5 != 0 {
			request.replyReceive = Event{Line: i + 1201, Ts: start + random.Float64()*4}
		}
		if i%7 != 0 {
			request.clientScope = threadGenerationScope{known: true, hasEnd: true, end: threadLifecyclePoint{ts: start + 10, line: i + 2401}}
		}
		requests = append(requests, request)
	}
	for _, source := range []string{"", "a", "b"} {
		var selected []*binderInventoryRequest
		for _, request := range requests {
			if source == "" || request.source == source {
				selected = append(selected, request)
			}
		}
		index := newBinderInventoryRequestIndex(selected)
		for i := 0; i < 2000; i++ {
			start := random.Float64() * 140
			interval := Interval{StartTs: start, EndTs: start + .1, ActualStartTs: start, ActualEndTs: start + .1, StartLine: 5000}
			var expected []*binderInventoryRequest
			for _, request := range selected {
				if request.send.Ts >= interval.ActualEndTs || (request.replyReceive.Line > 0 && request.replyReceive.Ts <= start) ||
					(request.clientScope.known && !request.clientScope.contains(start, interval.StartLine)) {
					continue
				}
				expected = append(expected, request)
				if len(expected) == 2 {
					break
				}
			}
			got := index.candidates(interval, nil, nil)
			if len(got) != len(expected) || (len(got) == 1 && got[0] != expected[0]) {
				t.Fatalf("index changed exact zero/unique/ambiguous classification source=%q start=%f got=%v expected=%v", source, start, got, expected)
			}
		}
	}
}

func TestB1607BRequestIndexSerialThousandsHasBoundedVisits(t *testing.T) {
	const count = 6000
	requests := make([]*binderInventoryRequest, count)
	for i := range requests {
		requests[i] = &binderInventoryRequest{send: Event{Ts: float64(i), Line: i*10 + 1}, replyReceive: Event{Ts: float64(i) + .4, Line: i*10 + 9}, clientScope: threadGenerationScope{known: true}}
	}
	index := newBinderInventoryRequestIndex(requests)
	visits := 0
	for i := range requests {
		interval := Interval{ActualStartTs: float64(i) + .1, ActualEndTs: float64(i) + .3, StartLine: i*10 + 2}
		got := index.candidates(interval, nil, &visits)
		if len(got) != 1 || got[0] != requests[i] {
			t.Fatalf("serial request %d lost exact identity: %v", i, got)
		}
	}
	if visits > count*32 {
		t.Fatalf("serial request lookup regressed toward a full-prefix scan: visits=%d requests=%d", visits, count)
	}
	t.Logf("%d exact serial lookups touched %d tree nodes (not %d prefix candidates)", count, visits, count*(count+1)/2)
}

func TestB1607BActualRunThousandsOfSubMillisecondClosedWaits(t *testing.T) {
	const count = 3000
	idx := buildTraceIndex(t, "thousands-binder.ftrace", b1607RepeatedClosedBinderTrace(count))
	got := b1607BinderInventory(t, Run(idx, Query{View: "window_stats", PID: 41, TimeStart: 10, TimeEnd: 13.01, Limit: 1}))
	if got.ConfirmedCount != count || got.TargetSleepCount != count || got.Emitted != 32 || math.Abs(got.ConfirmedMs-600) > 1e-6 {
		t.Fatalf("large serial population was capped or discarded: %+v", got)
	}
}
