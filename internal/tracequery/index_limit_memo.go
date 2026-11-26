package tracequery

import (
	"context"
	"sync"
)

type indexLimitMemoKey struct {
	source parseCacheKey
	opts   BuildOptions
}

type indexLimitMemo struct {
	mu           sync.Mutex
	limits       map[indexLimitMemoKey]IndexEventLimitError
	frameWindows [2]frameWindowIndex
}

type indexLimitRequestKey struct{}
type activeIndexLimitMemoKey struct{}

// WithIndexLimitMemo starts the request's trace-index reuse scope.
// Callers opt in at the query entry point with UseIndexLimitMemo.
func WithIndexLimitMemo(ctx context.Context) context.Context {
	return context.WithValue(ctx, indexLimitRequestKey{}, &indexLimitMemo{})
}

// UseIndexLimitMemo enables reuse for the current query if it belongs to a request.
func UseIndexLimitMemo(ctx context.Context) context.Context {
	memo, _ := ctx.Value(indexLimitRequestKey{}).(*indexLimitMemo)
	if memo == nil {
		return ctx
	}
	return context.WithValue(ctx, activeIndexLimitMemoKey{}, memo)
}

func (m *indexLimitMemo) get(key indexLimitMemoKey) (IndexEventLimitError, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	limit, ok := m.limits[key]
	return limit, ok
}

func (m *indexLimitMemo) put(key indexLimitMemoKey, limit IndexEventLimitError) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.limits == nil {
		m.limits = make(map[indexLimitMemoKey]IndexEventLimitError)
	}
	m.limits[key] = limit
}
