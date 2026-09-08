package tracequery

import (
	"math"
	"sort"
)

// binderInventoryRequestIndex is a source-scoped interval-stabbing index over
// the complete request set. Unknown completion stays open (+Inf); no temporal
// lookback or candidate cap removes a possibly pending request. The maxEnd
// tree skips already completed prefixes in long serial workloads.
type binderInventoryRequestIndex struct {
	requests []*binderInventoryRequest
	maxEnd   []float64
}

func newBinderInventoryRequestIndex(requests []*binderInventoryRequest) *binderInventoryRequestIndex {
	index := &binderInventoryRequestIndex{requests: append([]*binderInventoryRequest(nil), requests...)}
	sort.SliceStable(index.requests, func(i, j int) bool {
		a, b := index.requests[i], index.requests[j]
		if binderInventoryIndexedStart(a) != binderInventoryIndexedStart(b) {
			return binderInventoryIndexedStart(a) < binderInventoryIndexedStart(b)
		}
		return a.send.Line < b.send.Line
	})
	index.maxEnd = make([]float64, 4*len(index.requests)+1)
	var build func(int, int, int) float64
	build = func(node, start, end int) float64 {
		if end-start == 1 {
			value := math.Inf(1)
			request := index.requests[start]
			if request.replyReceive.Line > 0 && finiteSleepInventoryTime(request.replyReceive.Ts) {
				value = request.replyReceive.Ts
			}
			if request.clientScope.known && request.clientScope.hasEnd && finiteSleepInventoryTime(request.clientScope.end.ts) {
				value = math.Min(value, request.clientScope.end.ts)
			}
			index.maxEnd[node] = value
			return value
		}
		mid := start + (end-start)/2
		index.maxEnd[node] = math.Max(build(node*2, start, mid), build(node*2+1, mid, end))
		return index.maxEnd[node]
	}
	if len(index.requests) > 0 {
		build(1, 0, len(index.requests))
	}
	return index
}

func binderInventoryIndexedStart(request *binderInventoryRequest) float64 {
	if !finiteSleepInventoryTime(request.send.Ts) {
		return math.Inf(-1) // invalid time cannot disappear as proof of uniqueness
	}
	return request.send.Ts
}

// candidates returns at most two exact matches: the consumer distinguishes
// zero, unique, and ambiguous, not a ranked subset. visits is optional test
// instrumentation for the number of tree nodes touched, not a runtime budget.
func (index *binderInventoryRequestIndex) candidates(interval Interval, cancel *runCancelState, visits *int) []*binderInventoryRequest {
	if index == nil || len(index.requests) == 0 {
		return nil
	}
	start, end := interval.ActualStartTs, interval.ActualEndTs
	if end <= start {
		start, end = interval.StartTs, interval.EndTs // classification only
	}
	limit := sort.Search(len(index.requests), func(i int) bool { return binderInventoryIndexedStart(index.requests[i]) >= end })
	var found []*binderInventoryRequest
	var visit func(int, int, int)
	visit = func(node, lo, hi int) {
		if len(found) == 2 || lo >= limit || cancel.tick() {
			return
		}
		if visits != nil {
			*visits++
		}
		// Equality is retained for the physical-line lifecycle predicate;
		// the index is only a conservative accelerator, never a new gate.
		if index.maxEnd[node] < start {
			return
		}
		if hi-lo == 1 {
			request := index.requests[lo]
			if request.replyReceive.Line > 0 && request.replyReceive.Ts <= start {
				return
			}
			if request.clientScope.known && !request.clientScope.contains(start, interval.StartLine) {
				return
			}
			found = append(found, request)
			return
		}
		mid := lo + (hi-lo)/2
		visit(node*2, lo, mid)
		visit(node*2+1, mid, hi)
	}
	visit(1, 0, len(index.requests))
	return found
}
