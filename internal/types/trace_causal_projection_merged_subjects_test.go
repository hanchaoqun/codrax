package types

import (
	"fmt"
	"testing"
)

// MergedSubjects (customer complaint 2026-07-03): a merged row must preserve
// the distinct member thread names (capped) so renderers can keep the thread
// identities on the "其余 N 项合并" fold line instead of dropping them all.

// TestTraceCausalProjectionR3FoldKeepsMemberSubjects pins the load-bearing
// case: the R3 subjectless background fold row carries the folded members'
// thread subjects in bucket order.
func TestTraceCausalProjectionR3FoldKeepsMemberSubjects(t *testing.T) {
	records := []ObservationRecord{
		aggregateTestRecord("E21", "root_cause_background", "root_cause_background:b1", "isplogcat-1764", "unknown-thread", "117.928", 117.928, 45524, 45524,
			"chain_relevance=background", "causality=background"),
		aggregateTestRecord("E23", "root_cause_background", "root_cause_background:b2", "VSyncGenerator-2290", "unknown-thread", "98.501", 98.501, 53244, 81000,
			"chain_relevance=background", "causality=background"),
		aggregateTestRecord("E24", "root_cause_background", "root_cause_background:b3", "#tp-io-2036-16781", "unknown-thread", "12.689", 12.689, 46055, 50000,
			"chain_relevance=background", "causality=background"),
		aggregateTestRecord("E25", "root_cause_background", "root_cause_background:b4", "CodecLooper-17604", "unknown-thread", "6.886", 6.886, 60560, 65000,
			"chain_relevance=background", "causality=background"),
		aggregateTestRecord("E26", "root_cause_background", "root_cause_background:b5", "ProcessEvent_t-17599", "unknown-thread", "4.355", 4.355, 79382, 81000,
			"chain_relevance=background", "causality=background"),
	}
	got := TraceCausalProjectionFromObservationRecords(records)
	var fold *TraceCausalProjectionNode
	for i, node := range got.BackgroundCauses {
		if node.MergedCount > 1 && node.Subject == "" {
			fold = &got.BackgroundCauses[i]
		}
	}
	if fold == nil {
		t.Fatalf("R3 fold row missing: %+v", got.BackgroundCauses)
	}
	want := []string{"#tp-io-2036-16781", "CodecLooper-17604", "ProcessEvent_t-17599"}
	if len(fold.MergedSubjects) != len(want) {
		t.Fatalf("fold row must keep every folded member subject %v, got %v", want, fold.MergedSubjects)
	}
	for i, subject := range want {
		if fold.MergedSubjects[i] != subject {
			t.Fatalf("fold subjects must keep bucket order, want %v got %v", want, fold.MergedSubjects)
		}
	}
}

// TestTraceCausalProjectionR3FoldSubjectCapOverflow pins the cap: >4 folded
// members keep only the first 4 subjects; overflow is expressed by MergedCount.
func TestTraceCausalProjectionR3FoldSubjectCapOverflow(t *testing.T) {
	records := []ObservationRecord{
		aggregateTestRecord("K1", "root_cause_background", "root_cause_background:k1", "keep-1", "unknown-thread", "300.000", 300.0, 100, 110,
			"chain_relevance=background", "causality=background"),
		aggregateTestRecord("K2", "root_cause_background", "root_cause_background:k2", "keep-2", "unknown-thread", "200.000", 200.0, 120, 130,
			"chain_relevance=background", "causality=background"),
	}
	for i := 0; i < 6; i++ {
		records = append(records, aggregateTestRecord(
			fmt.Sprintf("F%d", i), "root_cause_background", fmt.Sprintf("root_cause_background:f%d", i),
			fmt.Sprintf("fold-%d", i), "unknown-thread", "1.000", float64(6-i), 200+10*i, 205+10*i,
			"chain_relevance=background", "causality=background"))
	}
	got := TraceCausalProjectionFromObservationRecords(records)
	var fold *TraceCausalProjectionNode
	for i, node := range got.BackgroundCauses {
		if node.MergedCount > 1 && node.Subject == "" {
			fold = &got.BackgroundCauses[i]
		}
	}
	if fold == nil || fold.MergedCount != 6 {
		t.Fatalf("6-member fold row missing: %+v", got.BackgroundCauses)
	}
	if len(fold.MergedSubjects) != traceCausalProjectionMergedSubjectCap {
		t.Fatalf("fold subjects must cap at %d, got %v", traceCausalProjectionMergedSubjectCap, fold.MergedSubjects)
	}
}

// TestTraceCausalProjectionR2AggregateFillsMergedSubjects covers the second
// MergedCount producer: the R2 ×N same-kind aggregate carries its (single,
// deduplicated) member subject too, so every merged row exposes the roster
// through the same typed field.
func TestTraceCausalProjectionR2AggregateFillsMergedSubjects(t *testing.T) {
	records := []ObservationRecord{
		aggregateTestRecord("E3", "root_cause_primary", "root_cause_primary:io1", "worker-9", "io_latency", "0.568", 0.568, 59938, 60100,
			"rank=5", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1"),
		aggregateTestRecord("E4", "root_cause_primary", "root_cause_primary:io2", "worker-9", "io_latency", "0.500", 0.500, 63943, 64100,
			"rank=6", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1"),
		aggregateTestRecord("E5", "root_cause_primary", "root_cause_primary:io3", "worker-9", "io_latency", "0.499", 0.499, 59809, 59900,
			"rank=7", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1"),
	}
	got := TraceCausalProjectionFromObservationRecords(records)
	var agg *TraceCausalProjectionNode
	for i, node := range got.PrimaryRootCauses {
		if node.MergedCount > 1 {
			agg = &got.PrimaryRootCauses[i]
		}
	}
	if agg == nil {
		t.Fatalf("R2 aggregate row missing: %+v", got.PrimaryRootCauses)
	}
	if len(agg.MergedSubjects) != 1 || agg.MergedSubjects[0] != "worker-9" {
		t.Fatalf("R2 aggregate must dedupe the shared member subject to one entry, got %v", agg.MergedSubjects)
	}
}

// TestTraceCausalProjectionAppendMergedSubjectGuards pins the helper's typed
// guards directly: empty and unknown-sentinel subjects never enter the roster,
// duplicates dedupe on the canonical key, and the cap is hard.
func TestTraceCausalProjectionAppendMergedSubjectGuards(t *testing.T) {
	var node TraceCausalProjectionNode
	for _, subject := range []string{"", "  ", "unknown-thread", "Unknown", "isplogcat-1494", "isplogcat-1494", " ISPLOGCAT-1494 "} {
		traceCausalProjectionAppendMergedSubject(&node, subject)
	}
	if len(node.MergedSubjects) != 1 || node.MergedSubjects[0] != "isplogcat-1494" {
		t.Fatalf("guards must keep exactly one real deduped subject, got %v", node.MergedSubjects)
	}
	for i := 0; i < 10; i++ {
		traceCausalProjectionAppendMergedSubject(&node, fmt.Sprintf("thread-%d", i))
	}
	if len(node.MergedSubjects) != traceCausalProjectionMergedSubjectCap {
		t.Fatalf("roster must cap at %d, got %v", traceCausalProjectionMergedSubjectCap, node.MergedSubjects)
	}
}
