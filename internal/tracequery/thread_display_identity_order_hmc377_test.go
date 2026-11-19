package tracequery

import "testing"

// HMC377 D3-R4: the bounded identity disclosure keeps a renamed on-chain
// thread ahead of census and background threads of the same causal faces
// (unseen Honeybird bg-kill: on-chain rank #6/#8 TID 3256 dropped while
// TID-ascending background threads filled the 32 slots).
func TestHMC377IdentityDisclosureKeepsChainThreadsAheadOfBackground(t *testing.T) {
	referenced := map[int]int{2991: 0}
	for tid := 3000; tid < 3040; tid++ {
		referenced[tid] = 2 // census / background rows of the causal faces
	}
	referenced[3256] = 2 // the on-chain rank thread, seen on the same faces
	got := threadDisplayIdentityOrder(referenced, map[int]bool{3256: true}, 32)
	if len(got) != 32 || got[0] != 2991 || got[1] != 3256 {
		t.Fatalf("analysed thread first, then the chain thread, then the rest: %v", got)
	}
	// Without the chain lift the chain thread falls off the bounded list.
	if order := threadDisplayIdentityOrder(referenced, nil, 32); containsInt(order, 3256) {
		t.Fatalf("fixture: the TID-ascending background population must fill the cap: %v", order)
	}
}

