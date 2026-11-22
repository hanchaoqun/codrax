package types

import (
	"testing"
)

// HMC377 round 13 (batch-owner ruling R-377-13, 2026-11-22), lane face: the
// projection carriers the tree face reads.

// R-377-13 ②: the typed analysed-thread identity — the ranked member is its
// rank board's own target, by TID (the engine's tid-first identity); a nil
// carrier, an unknown TID or a comm label never matches.
func TestHMC377R13MemberIsTarget(t *testing.T) {
	coordinate := func(tid int, comm string) TraceThreadCoordinate {
		return TraceThreadCoordinate{TID: tid, TGID: 10, Comm: comm}
	}
	for _, c := range []struct {
		name     string
		subjects *TraceRankSubjects
		want     bool
	}{
		{"nil carrier", nil, false},
		{"same thread", &TraceRankSubjects{Target: coordinate(42, "app"), Member: coordinate(42, "app")}, true},
		{"same thread, renamed comm", &TraceRankSubjects{Target: coordinate(42, "app"), Member: coordinate(42, "worker")}, true},
		{"other thread, same comm", &TraceRankSubjects{Target: coordinate(42, "app"), Member: coordinate(43, "app")}, false},
		{"unknown target tid", &TraceRankSubjects{Target: coordinate(0, "app"), Member: coordinate(0, "app")}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := c.subjects.MemberIsTarget(); got != c.want {
				t.Fatalf("MemberIsTarget = %t, want %t", got, c.want)
			}
		})
	}
}
