package types

// LOCKNS-FIX wire pins (§29.104.12, 2026-07-16): the two note→node read-ins
// this batch added ride the registered rich notes into the typed node fields;
// absence parses to empty/false (never guesses).
//
//	件6  holder_ns_unification (display_only→hard_consumer, OM-10 关账) →
//	     Node.BlockingHolderNsUnification — gated INSIDE the contention block
//	     (a unification without contention semantics is meaningless).
//	件3  blocking_owner_key_unregistered → Node.BlockingOwnerKeyUnregistered —
//	     parsed OUTSIDE the BlockingKind gate (it mints exactly on
//	     payload-less rows).

import "testing"

func TestTraceCausalProjectionParsesLocknsNotes(t *testing.T) {
	unified := aggregateTestRecord("L1", "critical_blocking", "critical_blocking:L1",
		"aweme-41999", "nsworker-42500", "0.295", 0.295, 10, 20,
		"chain_relevance=on_chain", "type=blocking_span",
		"blocking_kind=monitor_contention",
		"holder_source=ns_span_derivation",
		"holder_ns_unification=owner_ns_tid=62020 host=nsworker-42500 lanes=ns_span_derivation+wakeup_edge")
	unregistered := aggregateTestRecord("L2", "critical_blocking", "critical_blocking:L2",
		"aweme-41999", "", "12.000", 12.0, 30, 40,
		"chain_relevance=on_chain", "type=blocking_span",
		"blocking_owner_key_unregistered=true")
	bare := aggregateTestRecord("L3", "critical_blocking", "critical_blocking:L3",
		"aweme-41999", "holder-51000", "5.000", 5.0, 50, 60,
		"chain_relevance=on_chain", "type=blocking_span",
		"blocking_kind=lock_contention")
	got := TraceCausalProjectionFromObservationRecords([]ObservationRecord{unified, unregistered, bare})
	byID := map[string]TraceCausalProjectionNode{}
	for _, node := range xerr1FixBAllNodes(got) {
		byID[node.EvidenceID] = node
	}
	l1, ok := byID["L1"]
	if !ok || l1.BlockingHolderNsUnification != "owner_ns_tid=62020 host=nsworker-42500 lanes=ns_span_derivation+wakeup_edge" {
		t.Fatalf("件6: the unification note must parse into the typed node field: %+v", l1)
	}
	l2, ok := byID["L2"]
	if !ok || !l2.BlockingOwnerKeyUnregistered {
		t.Fatalf("件3: the unknown-morphology marker must parse into the typed node field: %+v", l2)
	}
	if l2.BlockingHolderNsUnification != "" {
		t.Fatalf("件3 rows carry no unification value: %+v", l2)
	}
	l3, ok := byID["L3"]
	if !ok || l3.BlockingHolderNsUnification != "" || l3.BlockingOwnerKeyUnregistered {
		t.Fatalf("absence must parse to empty/false (never guesses): %+v", l3)
	}
}

// 修补轮 件A (冷读 P2-F1+P3-F7, 2026-07-16): the owner_tid_presence note →
// Node.BlockingOwnerTidPresence read-in (contention-gated beside
// owner_tid_raw); absence parses to empty (fail-open — the display keeps the
// legacy sentence byte-identically).
func TestTraceCausalProjectionParsesOwnerTidPresenceLOCKNSRepair(t *testing.T) {
	collision := aggregateTestRecord("P1", "critical_blocking", "critical_blocking:P1",
		"aweme-41999", "releaser-800", "10.000", 10.0, 10, 20,
		"chain_relevance=on_chain", "type=blocking_span",
		"blocking_kind=lock_contention",
		"holder_source=wakeup_edge",
		"owner_tid_raw=51000",
		"owner_tid_presence=present_collision")
	legacy := aggregateTestRecord("P2", "critical_blocking", "critical_blocking:P2",
		"aweme-41999", "releaser-800", "5.000", 5.0, 30, 40,
		"chain_relevance=on_chain", "type=blocking_span",
		"blocking_kind=lock_contention",
		"holder_source=wakeup_edge",
		"owner_tid_raw=987654")
	got := TraceCausalProjectionFromObservationRecords([]ObservationRecord{collision, legacy})
	byID := map[string]TraceCausalProjectionNode{}
	for _, node := range xerr1FixBAllNodes(got) {
		byID[node.EvidenceID] = node
	}
	p1, ok := byID["P1"]
	if !ok || p1.BlockingOwnerTidPresence != "present_collision" || p1.BlockingOwnerTidRaw != 51000 {
		t.Fatalf("件A: the presence note must parse into the typed node field: %+v", p1)
	}
	p2, ok := byID["P2"]
	if !ok || p2.BlockingOwnerTidPresence != "" {
		t.Fatalf("件A: absence must parse to empty (legacy wire fail-open): %+v", p2)
	}
}
