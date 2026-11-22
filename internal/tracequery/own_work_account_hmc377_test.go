package tracequery

import (
	"testing"
)

// HMC377 round 11 (R10P-1, owner ruling R-377-11 ③). The analysed thread's
// own-work family and the wait family of the same spans are two disjoint
// parts of ONE own-work account. The engine stamps one typed identity on both
// (thread, semantic class and subcategory, the account's in-window instants,
// the physical artifact) and names the part each family prices, so the
// projection and every face recognise a seat and its own record, and the two
// parts, by identity — never by value or name. Expected values are written
// down from the synthetic inputs.

func hmc377R11AccountFamilies(t *testing.T, chain *ChainResult, spans ...TraceSpanSummary) (own, wait *SemanticSpanFamily, all []SemanticSpanFamily) {
	t.Helper()
	stats := WindowStats{Window: TimeWindow{StartTs: 5.0, EndTs: 6.0}, TraceSpans: spans}
	all = SemanticSpanFamiliesForStats(chain, &stats)
	for i := range all {
		if !sameThreadRef(all[i].Thread, chain.Target) || !all[i].OnChain {
			continue
		}
		switch all[i].OnChainBasis {
		case "":
			wait = &all[i]
		case RootCauseOnChainBasisSelfDeterministicSpan:
			own = &all[i]
		}
	}
	return own, wait, all
}

func TestHMC377R11OwnWorkAccountIdentityIsTyped(t *testing.T) {
	target := ThreadRef{Comm: "app", PID: 100}
	sleepInsideB := ChainNode{Thread: target, Window: TimeWindow{StartTs: 5.110, EndTs: 5.116}, Dominant: StateSSleep}
	t.Run("split_account", func(t *testing.T) {
		a := rcmSpan(target, "VerifyClass com.example.A", 5.050, 5.060, 1, 2)
		b := rcmSpan(target, "VerifyClass com.example.B", 5.100, 5.130, 10, 20)
		own, wait, _ := hmc377R11AccountFamilies(t, hmc377Chain(target, sleepInsideB), a, b)
		if own == nil || wait == nil {
			t.Fatalf("an own-work family and the wait family of span B expected: own=%v wait=%v", own, wait)
		}
		ownID, ownPart := own.OwnWorkAccount()
		waitID, waitPart := wait.OwnWorkAccount()
		if ownID == "" || ownID != waitID || ownPart != SemanticOwnWorkAccountPartOwnWork || waitPart != SemanticOwnWorkAccountPartChainWait {
			t.Fatalf("one account, two parts: own=(%q,%q) wait=(%q,%q)", ownID, ownPart, waitID, waitPart)
		}
		// The parts are disjoint and add up to the account's member union:
		// own work 34 (10 + 30 − 6) + wait part 6 = 40.
		ownMs, waitMs, split := own.OwnWorkSplit()
		if !split || !hmc377Near(ownMs, 34.0) || !hmc377Near(wait.ProjectedImpactMs, 6.0) || !hmc377Near(ownMs+waitMs, own.TotalMs) {
			t.Fatalf("own %.6f + wait %.6f must be the union %.6f", ownMs, wait.ProjectedImpactMs, own.TotalMs)
		}
		q := Query{TimeStart: 5.0, TimeEnd: 6.0}
		ownItem, ok := rootCauseItemFromSemanticSpanFamily(q, *own, true)
		if !ok {
			t.Fatal("own-work row expected")
		}
		ownItem.OwnWorkAccount, ownItem.OwnWorkAccountPart = own.OwnWorkAccount()
		if ownItem.OwnWorkAccount != ownID || ownItem.OwnWorkAccountPart != SemanticOwnWorkAccountPartOwnWork {
			t.Fatalf("the own-work row carries the account identity: %+v", ownItem)
		}
	})
	t.Run("instants_identify_the_account", func(t *testing.T) {
		b := rcmSpan(target, "VerifyClass com.example.B", 5.100, 5.130, 10, 20)
		own1, _, _ := hmc377R11AccountFamilies(t, hmc377Chain(target, sleepInsideB), b)
		bShorter := rcmSpan(target, "VerifyClass com.example.B", 5.100, 5.128, 10, 20)
		own2, _, _ := hmc377R11AccountFamilies(t, hmc377Chain(target, sleepInsideB), bShorter)
		id1, _ := own1.OwnWorkAccount()
		id2, _ := own2.OwnWorkAccount()
		if id1 == "" || id2 == "" || id1 == id2 {
			t.Fatalf("different instants are different accounts: %q vs %q", id1, id2)
		}
		again, _, _ := hmc377R11AccountFamilies(t, hmc377Chain(target, sleepInsideB), b)
		if id3, _ := again.OwnWorkAccount(); id3 != id1 {
			t.Fatalf("the same instants are the same account across publications: %q vs %q", id1, id3)
		}
	})
	t.Run("unsplit_family_carries_no_identity", func(t *testing.T) {
		a := rcmSpan(target, "VerifyClass com.example.A", 5.050, 5.060, 1, 2)
		running := ChainNode{Thread: ThreadRef{Comm: "waker", PID: 500}, Window: TimeWindow{StartTs: 5.5, EndTs: 5.6}, Dominant: StateRunning}
		own, wait, _ := hmc377R11AccountFamilies(t, hmc377Chain(target, running), a)
		if own == nil || wait != nil {
			t.Fatalf("one unsplit own-work family expected: own=%v wait=%v", own, wait)
		}
		if id, part := own.OwnWorkAccount(); id != "" || part != "" {
			t.Fatalf("an own-work family whose price is its wall needs no account identity: (%q,%q)", id, part)
		}
	})
	t.Run("recorded_wait_is_no_part", func(t *testing.T) {
		// A marker that records a wait is never own work: it is no part of an
		// own-work account even beside one.
		a := rcmSpan(target, "VerifyClass com.example.A", 5.050, 5.060, 1, 2)
		b := rcmSpan(target, "VerifyClass com.example.B", 5.100, 5.130, 10, 20)
		gcWait := rcmSpan(target, "GC: Wait For Completion Alloc", 5.111, 5.115, 30, 31)
		_, _, all := hmc377R11AccountFamilies(t, hmc377Chain(target, sleepInsideB), a, b, gcWait)
		for _, fam := range all {
			if semanticSpanRecordsWait(fam.Members[0].Name) {
				if id, part := fam.OwnWorkAccount(); id != "" || part != "" {
					t.Fatalf("a recorded wait carries an account identity: %+v", fam)
				}
			}
		}
	})
}
