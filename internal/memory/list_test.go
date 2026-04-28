package memory

import (
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestStore_List_RecencyOrder pins the load-bearing semantic: List
// returns entries by timestamp desc. Mixes recent + index entries
// to also verify dedup (same ID once) and merge-order correctness.
func TestStore_List_RecencyOrder(t *testing.T) {
	t0 := time.Now().Add(-30 * time.Minute) // oldest
	t1 := time.Now().Add(-15 * time.Minute)
	t2 := time.Now().Add(-5 * time.Minute) // newest

	idx := []IndexEntry{
		// Compacted entry — turn-<unix-nano>-<pid>; the unix-nano
		// here is t0 so the helper extracts it correctly.
		{
			ID:    "turn-" + uniNano(t0) + "-1",
			Topic: "old compacted",
		},
		{
			ID:    "turn-" + uniNano(t1) + "-1",
			Topic: "mid compacted",
		},
	}
	recent := []Turn{
		{
			ID:        "turn-" + uniNano(t2) + "-1",
			Request:   "newest recent",
			Response:  "...",
			Timestamp: t2,
		},
	}
	s := newSearchStubStore(t, idx, recent)

	out := s.List(ListOpts{Limit: 10})
	if len(out) != 3 {
		t.Fatalf("List returned %d entries, want 3", len(out))
	}
	// Assert recency desc.
	wantOrder := []string{
		"newest recent",
		"mid compacted",
		"old compacted",
	}
	for i, want := range wantOrder {
		if out[i].Topic != want {
			t.Errorf("out[%d].Topic = %q, want %q (full order: %v)",
				i, out[i].Topic, want, topicsOf(out))
		}
	}
}

// TestStore_List_LimitCap verifies the 30-entry hard cap and the
// caller-default of 10 when limit ≤ 0.
func TestStore_List_LimitCap(t *testing.T) {
	idx := make([]IndexEntry, 50)
	for i := range idx {
		idx[i] = IndexEntry{
			ID:    "turn-" + uniNano(time.Now().Add(-time.Duration(i)*time.Minute)) + "-1",
			Topic: "e",
		}
	}
	s := newSearchStubStore(t, idx, nil)

	if got := s.List(ListOpts{Limit: 0}); len(got) != 10 {
		t.Errorf("limit=0 default = %d, want 10", len(got))
	}
	if got := s.List(ListOpts{Limit: 999}); len(got) != 30 {
		t.Errorf("limit=999 hard cap = %d, want 30", len(got))
	}
	if got := s.List(ListOpts{Limit: 5}); len(got) != 5 {
		t.Errorf("limit=5 = %d, want 5", len(got))
	}
}

// TestStore_List_KindFilter pins the optional Kind filter. Drops
// entries from both compacted and recent paths uniformly.
func TestStore_List_KindFilter(t *testing.T) {
	idx := []IndexEntry{
		{ID: "turn-100-1", Kind: KindChitchat, Topic: "chat-c"},
		{ID: "turn-200-1", Kind: KindPlan, Topic: "plan-c"},
	}
	recent := []Turn{
		{ID: "turn-300-1", Kind: KindChitchat, Request: "chat-r", Timestamp: time.Now()},
		{ID: "turn-400-1", Kind: KindPlan, Request: "plan-r", Timestamp: time.Now()},
	}
	s := newSearchStubStore(t, idx, recent)

	out := s.List(ListOpts{Limit: 10, Kind: string(KindPlan)})
	if len(out) != 2 {
		t.Fatalf("Kind=plan filter returned %d, want 2", len(out))
	}
	for _, e := range out {
		if e.Kind != KindPlan {
			t.Errorf("Kind filter leak: %+v", e)
		}
	}
}

// TestStore_List_SinceFloor pins the optional Since cutoff: entries
// older than Since are dropped.
func TestStore_List_SinceFloor(t *testing.T) {
	cutoff := time.Now().Add(-10 * time.Minute)
	idx := []IndexEntry{
		{ID: "turn-" + uniNano(time.Now().Add(-30*time.Minute)) + "-1", Topic: "ancient"},
		{ID: "turn-" + uniNano(time.Now().Add(-1*time.Minute)) + "-1", Topic: "fresh"},
	}
	s := newSearchStubStore(t, idx, nil)

	out := s.List(ListOpts{Limit: 10, Since: cutoff})
	if len(out) != 1 {
		t.Fatalf("since cutoff returned %d, want 1", len(out))
	}
	if out[0].Topic != "fresh" {
		t.Errorf("since cutoff kept wrong entry: %+v", out[0])
	}
}

// TestStore_List_DedupesByID locks the merge dedup: when the same
// ID appears in both index and recent (mid-compaction race), the
// compacted version wins.
func TestStore_List_DedupesByID(t *testing.T) {
	id := "turn-" + uniNano(time.Now()) + "-1"
	idx := []IndexEntry{
		{ID: id, Topic: "compacted-version", Keywords: []string{"k"}},
	}
	recent := []Turn{
		{ID: id, Request: "raw-version", Response: "...", Timestamp: time.Now()},
	}
	s := newSearchStubStore(t, idx, recent)

	out := s.List(ListOpts{Limit: 10})
	if len(out) != 1 {
		t.Fatalf("dedup returned %d, want 1", len(out))
	}
	if out[0].Topic != "compacted-version" {
		t.Errorf("dedup picked wrong: %+v (compacted should win)", out[0])
	}
}

// TestStore_List_NilStoreSafe matches the pattern of Search nil-
// safety. Defensive — production paths always have a real Store.
func TestStore_List_NilStoreSafe(t *testing.T) {
	var s *Store
	if got := s.List(ListOpts{Limit: 5}); got != nil {
		t.Errorf("nil Store List should return nil; got %v", got)
	}
}

// TestAdapter_List delegates through the public types boundary —
// ensures the lift from local IndexEntry to types.MemoryIndexEntry
// preserves Topic / Timestamp / Kind.
func TestAdapter_List(t *testing.T) {
	idx := []IndexEntry{
		{
			ID:    "turn-100000000-1",
			Topic: "T",
			Kind:  KindChitchat,
		},
	}
	s := newSearchStubStore(t, idx, nil)
	a := NewAdapter(s)
	out := a.List(types.MemoryListOpts{Limit: 10})
	if len(out) != 1 {
		t.Fatalf("Adapter.List returned %d, want 1", len(out))
	}
	if out[0].Topic != "T" || out[0].Kind != string(KindChitchat) {
		t.Errorf("Adapter.List lift lost field: %+v", out[0])
	}
}

// uniNano formats a time.Time as a unix-nano string suitable for the
// turn-<unix-nano>-<pid> ID convention.
func uniNano(t time.Time) string {
	return formatInt(t.UnixNano())
}

func formatInt(n int64) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	var buf [32]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func topicsOf(es []IndexEntry) []string {
	out := make([]string, 0, len(es))
	for _, e := range es {
		out = append(out, e.Topic)
	}
	return out
}
