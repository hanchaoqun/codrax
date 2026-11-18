package types

import (
	"sort"
	"strings"
	"testing"
)

// Two real-shaped receipt digests; which one is lexically smaller is exactly
// what changes when the same capture is read from another directory.
const (
	hmc376DigestLow  = "0a11111111111111111111111111111111111111111111111111111111111111"
	hmc376DigestHigh = "f922222222222222222222222222222222222222222222222222222222222222"
)

func TestTraceObservationCoordinateOrderKeyReadsOnlyTheInPublicationSuffix(t *testing.T) {
	for _, tc := range []struct{ name, id, want string }{
		{"full digest", "trace_query:publication-" + hmc376DigestLow + "#root_cause_rank:3", "trace_query:publication#root_cause_rank:3"},
		{"same suffix under another digest", "trace_query:publication-" + hmc376DigestHigh + "#root_cause_rank:3", "trace_query:publication#root_cause_rank:3"},
		{"nested suffix", "trace_query:publication-" + hmc376DigestLow + "#ipc_request_edge:2:7", "trace_query:publication#ipc_request_edge:2:7"},
		// Everything below keeps its whole coordinate: the existing order of
		// legacy and foreign coordinates must not move.
		{"legacy result scope", "trace_query:p1-root_cause_rank#root_cause_rank:3", "trace_query:p1-root_cause_rank#root_cause_rank:3"},
		{"other producer", "read_file:internal/a.go#L10", "read_file:internal/a.go#L10"},
		{"short digest", "trace_query:publication-0a11#root_cause_rank:3", "trace_query:publication-0a11#root_cause_rank:3"},
		{"non-hex digest", "trace_query:publication-" + strings.Repeat("z", 64) + "#root_cause_rank:3", "trace_query:publication-" + strings.Repeat("z", 64) + "#root_cause_rank:3"},
		{"no suffix separator", "trace_query:publication-" + hmc376DigestLow, "trace_query:publication-" + hmc376DigestLow},
		{"empty suffix", "trace_query:publication-" + hmc376DigestLow + "#", "trace_query:publication-" + hmc376DigestLow + "#"},
		{"empty", "", ""},
	} {
		if got := TraceObservationCoordinateOrderKey(tc.id); got != tc.want {
			t.Errorf("%s: order key of %q = %q, want %q", tc.name, tc.id, got, tc.want)
		}
	}
	// The key is an ordering input, never an identity: two publications share
	// it, and the full coordinates stay distinct.
	a := "trace_query:publication-" + hmc376DigestLow + "#root_cause_rank:1"
	b := "trace_query:publication-" + hmc376DigestHigh + "#root_cause_rank:1"
	if TraceObservationCoordinateLess(a, b) || TraceObservationCoordinateLess(b, a) {
		t.Fatal("two publications of one in-publication coordinate must be left unordered")
	}
	if a == b {
		t.Fatal("control: the full coordinates differ")
	}
}

// The twin publications of one fact tie on every typed field of the node
// comparator. On the base commit the raw coordinate broke the tie, so swapping
// the two digests (what moving the capture file does) swapped the survivor.
func TestTraceCausalProjectionTieBetweenPublicationsIgnoresTheDigest(t *testing.T) {
	twin := func(digest, fingerprint string) TraceCausalProjectionNode {
		return TraceCausalProjectionNode{
			EvidenceID: "trace_query:publication-" + digest + "#root_cause_rank:1",
			Subject:    "worker-10", Predicate: "root_cause_primary", StateKind: "runnable",
			ImpactMS: 48.519, CumulativeImpactMS: 48.519, Confidence: 0.6, Rank: 1,
			RankBoardTarget: "worker-10", RankBoardParamsFingerprint: fingerprint,
		}
	}
	for _, digests := range [][2]string{{hmc376DigestLow, hmc376DigestHigh}, {hmc376DigestHigh, hmc376DigestLow}} {
		first, second := twin(digests[0], "board-first"), twin(digests[1], "board-second")
		if traceCausalProjectionNodeLess(first, second) || traceCausalProjectionNodeLess(second, first) {
			t.Fatalf("digests %s/%s: equal typed fields and equal in-publication coordinates must tie", digests[0][:4], digests[1][:4])
		}
		nodes := []TraceCausalProjectionNode{first, second}
		sort.SliceStable(nodes, func(i, j int) bool { return traceCausalProjectionNodeLess(nodes[i], nodes[j]) })
		if nodes[0].RankBoardParamsFingerprint != "board-first" {
			t.Fatalf("digests %s/%s: the earlier publication must keep its place, got %q first",
				digests[0][:4], digests[1][:4], nodes[0].RankBoardParamsFingerprint)
		}
	}
	// Different in-publication coordinates still order, and identically under
	// either digest order.
	for _, digests := range [][2]string{{hmc376DigestLow, hmc376DigestHigh}, {hmc376DigestHigh, hmc376DigestLow}} {
		early, late := twin(digests[0], "x"), twin(digests[1], "y")
		early.EvidenceID = "trace_query:publication-" + digests[0] + "#root_cause_rank:2"
		late.EvidenceID = "trace_query:publication-" + digests[1] + "#root_cause_rank:1"
		if !traceCausalProjectionNodeLess(late, early) || traceCausalProjectionNodeLess(early, late) {
			t.Fatalf("digests %s/%s: the in-publication coordinate must decide the order", digests[0][:4], digests[1][:4])
		}
	}
	// Legacy coordinates keep the whole-coordinate order they always had.
	legacyA, legacyB := twin(hmc376DigestLow, "a"), twin(hmc376DigestLow, "b")
	legacyA.EvidenceID, legacyB.EvidenceID = "trace_query:p1-root_cause_rank#root_cause_rank:1", "trace_query:p2-root_cause_rank#root_cause_rank:1"
	if !traceCausalProjectionNodeLess(legacyA, legacyB) || traceCausalProjectionNodeLess(legacyB, legacyA) {
		t.Fatal("legacy result-scoped coordinates keep their lexical order")
	}
}

// The coverage view orders its records by coordinate first and then cuts the
// list. On the base commit the digest decided which rank query supplied the
// whole top list; now the first-published query does, under either digest
// order, and one publication's rows stay together.
func TestTraceObservationCoverageTopListIgnoresTheDigest(t *testing.T) {
	record := func(digest string, ordinal int, subject string) ObservationRecord {
		return ObservationRecord{
			ID:     "trace_query:publication-" + digest + "#root_cause_rank:" + string(rune('0'+ordinal)),
			Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", GroundingPolicy: ClaimGroundingHard,
			Predicate: "root_cause_primary", Subject: subject, Value: "1", Unit: "ms",
		}
	}
	top := func(firstDigest, secondDigest string) string {
		var records []ObservationRecord
		for ordinal := 1; ordinal <= 3; ordinal++ {
			records = append(records, record(firstDigest, ordinal, "first-query"))
		}
		for ordinal := 1; ordinal <= 3; ordinal++ {
			records = append(records, record(secondDigest, ordinal, "second-query"))
		}
		coverage := TraceObservationCoverageFromObservationRecords(records)
		var order []string
		for _, observation := range coverage.TopObservations {
			_, suffix, _ := strings.Cut(observation.ID, "#")
			order = append(order, observation.Subject+"#"+suffix)
		}
		return strings.Join(order, " ")
	}
	want := "first-query#root_cause_rank:1 first-query#root_cause_rank:2 first-query#root_cause_rank:3 " +
		"second-query#root_cause_rank:1 second-query#root_cause_rank:2 second-query#root_cause_rank:3"
	for _, digests := range [][2]string{{hmc376DigestLow, hmc376DigestHigh}, {hmc376DigestHigh, hmc376DigestLow}} {
		if got := top(digests[0], digests[1]); got != want {
			t.Fatalf("digests %s/%s: top list order\n got %s\nwant %s", digests[0][:4], digests[1][:4], got, want)
		}
	}

	// The sort key keeps every other coordinate whole and keeps one
	// publication's in-publication order.
	var order TraceObservationPublicationOrder
	first := order.SortKey("trace_query:publication-" + hmc376DigestHigh + "#root_cause_rank:2")
	second := order.SortKey("trace_query:publication-" + hmc376DigestLow + "#root_cause_rank:1")
	again := order.SortKey("trace_query:publication-" + hmc376DigestHigh + "#root_cause_rank:1")
	if !(again < first && first < second) {
		t.Fatalf("publication ordinal then in-publication suffix: %q %q %q", again, first, second)
	}
	if strings.Contains(first, hmc376DigestHigh) || strings.Contains(second, hmc376DigestLow) {
		t.Fatal("the sort key must not carry the digest")
	}
	if legacy := "trace_query:p2-root_cause_rank#root_cause_rank:1"; order.SortKey(legacy) != legacy {
		t.Fatal("legacy coordinates keep their whole-coordinate order")
	}
}

// The same-fact merge keeps the first row in bucket order. With the digest out
// of the order, the surviving publication of a cross-board twin no longer
// follows the digest: both digest orders give the same survivor.
func TestSameFactSurvivorBetweenBoardsIgnoresTheDigest(t *testing.T) {
	survivorFingerprint := func(firstDigest, secondDigest string) string {
		node := func(digest, fingerprint string) TraceCausalProjectionNode {
			return TraceCausalProjectionNode{
				EvidenceID: "trace_query:publication-" + digest + "#root_cause_rank:1",
				Role:       TraceCausalRoleRootCauseContext, Subject: "worker-10", Predicate: "root_cause_tertiary",
				StateKind: "runnable", ImpactMS: 31.728, CumulativeImpactMS: 31.728, Confidence: 0.6, Rank: 1,
				LineStart: 100, LineEnd: 900, ChainRelevance: "on_chain",
				RankBoardTarget: "worker-10", RankBoardParamsFingerprint: fingerprint,
				QueryWindowStartTs: 10, QueryWindowEndTs: 11,
			}
		}
		out := TraceCausalProjection{OnChainCauses: []TraceCausalProjectionNode{
			node(firstDigest, "board-first"), node(secondDigest, "board-second"),
		}}
		sort.SliceStable(out.OnChainCauses, func(i, j int) bool {
			return traceCausalProjectionClassifiedLess(out.OnChainCauses[i], out.OnChainCauses[j], nil)
		})
		traceCausalProjectionMergeSameFacts(&out)
		if len(out.OnChainCauses) != 1 {
			t.Fatalf("the two publications of one fact must fold into one row, got %d", len(out.OnChainCauses))
		}
		return out.OnChainCauses[0].RankBoardParamsFingerprint
	}
	low := survivorFingerprint(hmc376DigestLow, hmc376DigestHigh)
	high := survivorFingerprint(hmc376DigestHigh, hmc376DigestLow)
	if low != high {
		t.Fatalf("the surviving board follows the digest order: %q vs %q", low, high)
	}
	if low != "board-first" {
		t.Fatalf("the earlier publication keeps the row, got %q", low)
	}
}
