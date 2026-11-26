package tracequery

import (
	"context"
	"reflect"
)

type frameWindowIndex struct {
	source     parseCacheKey
	opts       BuildOptions
	index      *Index
	bundle     bool
	chainQuery Query
	chain      *ChainResult
}

type frameWindowIndexViewKey struct{}

// UseFrameWindowIndex opts the initial unpruned build into request-local
// window reuse. Two recent windows retain both sides of a comparison.
func UseFrameWindowIndex(ctx context.Context, view string) context.Context {
	if ctx.Value(activeIndexLimitMemoKey{}) == nil {
		return ctx
	}
	switch view = CanonicalViewName(view); view {
	case "frame_root_cause_bundle", "thread_timeline", "window_stats", "root_cause_rank", "wakeup_chain":
		return context.WithValue(ctx, frameWindowIndexViewKey{}, view)
	default:
		return ctx
	}
}

func (m *indexLimitMemo) loadFrameWindow(source parseCacheKey, opts BuildOptions, view string) (*Index, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, saved := range m.frameWindows {
		idx := saved.index
		if idx == nil || saved.source != source ||
			saved.opts.TimeStart != opts.TimeStart || saved.opts.TimeEnd != opts.TimeEnd ||
			saved.opts.ScopePID != opts.ScopePID || saved.opts.ScopeThread != opts.ScopeThread ||
			len(idx.Events) > opts.MaxEvents {
			continue
		}
		if saved.opts.TimePaddingBefore == opts.TimePaddingBefore && saved.opts.TimePaddingAfter == opts.TimePaddingAfter {
			saved.bundle = saved.bundle || view == "frame_root_cause_bundle"
			m.frameWindows[i] = m.frameWindows[0]
			m.frameWindows[0] = saved
			return idx, true
		}
		if view == "thread_timeline" && saved.bundle && idx.TimestampOrder == TraceTimestampOrderMonotonic &&
			idx.IndexTimeStart <= paddedTimeStart(opts) && idx.IndexTimeEnd >= paddedTimeEnd(opts) {
			return idx, false
		}
	}
	return nil, false
}

func (m *indexLimitMemo) storeFrameWindow(source parseCacheKey, opts BuildOptions, idx *Index, view string) {
	if idx.RelationScoped || idx.PaddingTruncated {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, saved := range m.frameWindows {
		// Concurrent callers of the same build receive one index from
		// singleflight. Keep one slot for it and preserve any computed chain.
		if saved.index == idx {
			saved.bundle = saved.bundle || view == "frame_root_cause_bundle"
			m.frameWindows[i] = m.frameWindows[0]
			m.frameWindows[0] = saved
			return
		}
	}
	m.frameWindows[1] = m.frameWindows[0]
	m.frameWindows[0] = frameWindowIndex{source: source, opts: opts, index: idx, bundle: view == "frame_root_cause_bundle"}
}

// The normalized selector includes the chain budgets and source semantics.
// Only the outer face and its optional stats, plus per-Run plumbing, differ.
func frameChainSelector(q Query) Query {
	q.View, q.IncludeWindowStats, q.windowStatsSpecified = "", false, false
	q.runCancel, q.schedulerWakeCompletions = nil, nil
	return q
}

func (m *indexLimitMemo) loadFrameChain(idx *Index, q Query) (ChainResult, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, saved := range m.frameWindows {
		if saved.index == idx && saved.chain != nil && reflect.DeepEqual(saved.chainQuery, q) {
			return *saved.chain, true
		}
	}
	return ChainResult{}, false
}

func (m *indexLimitMemo) storeFrameChain(idx *Index, q Query, chain ChainResult) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.frameWindows {
		if m.frameWindows[i].index == idx {
			m.frameWindows[i].chainQuery = q
			m.frameWindows[i].chain = &chain
			return
		}
	}
}
