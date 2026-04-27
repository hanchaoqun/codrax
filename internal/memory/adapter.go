package memory

import (
	"github.com/hanchaoqun/codrax/internal/types"
)

// Adapter wraps a *Store so it satisfies the types.MemoryReader
// interface. Lives in the memory package (the Store side of the
// boundary) because that's where the type-conversion lives — the
// types package must stay free of memory imports to avoid a cycle.
//
// The wrapping is mechanical: each interface method delegates to the
// matching Store method and translates between memory.IndexEntry /
// memory.SearchOpts and their types-package mirrors. Pointer
// receiver because Store itself is mutated by Search via
// withSharedLock.
type Adapter struct {
	store *Store
}

// NewAdapter returns an Adapter wrapping the given Store. nil-safe:
// returns nil when store is nil so callers can unconditionally hand
// the result to BusContext.Memory.
func NewAdapter(s *Store) *Adapter {
	if s == nil {
		return nil
	}
	return &Adapter{store: s}
}

// Search implements types.MemoryReader. Translates the interface
// types into the local SearchOpts, calls Store.Search, and lifts
// every returned IndexEntry into the public mirror struct. The
// in-memory `body` payload is read via bodyOf so the field stays
// unexported.
func (a *Adapter) Search(query string, opts types.MemorySearchOpts) []types.MemoryIndexEntry {
	if a == nil || a.store == nil {
		return nil
	}
	rows := a.store.Search(query, SearchOpts{
		Kind:        opts.Kind,
		SessionID:   opts.SessionID,
		Limit:       opts.Limit,
		IncludeBody: opts.IncludeBody,
	})
	if len(rows) == 0 {
		return nil
	}
	out := make([]types.MemoryIndexEntry, 0, len(rows))
	for _, e := range rows {
		out = append(out, types.MemoryIndexEntry{
			ID:        e.ID,
			Topic:     e.Topic,
			Summary:   e.Summary,
			Keywords:  append([]string(nil), e.Keywords...),
			Entities:  append([]string(nil), e.Entities...),
			Refs:      append([]string(nil), e.Refs...),
			Kind:      string(e.Kind),
			SessionID: e.SessionID,
			FullRef:   e.FullRef,
			Body:      bodyOf(e),
		})
	}
	return out
}
