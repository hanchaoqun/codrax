package types

import (
	"strings"
	"testing"
)

// Equally ranked seats keep one selection whichever digests the capture's
// location gave their publications.
func TestHMC376EliminableSeatsDoNotFollowThePublicationDigest(t *testing.T) {
	seat := func(digest, suffix, subject string) TraceCausalProjectionNode {
		return TraceCausalProjectionNode{
			EvidenceID: "trace_query:publication-" + strings.Repeat(digest, 64) + "#" + suffix,
			Subject:    subject, Rank: 1, EffectiveImpactMS: 10, EffectiveImpactPublished: true, ChainRelevance: "on_chain",
		}
	}
	for _, digests := range [][2]string{{"a", "f"}, {"f", "a"}} {
		seats := TraceAnswerDecisionEliminableSeats(TraceCausalProjection{RankedSeats: []TraceCausalProjectionNode{
			seat(digests[0], "root_cause_rank:2", "later-ordinal"),
			seat(digests[1], "root_cause_rank:1", "earlier-ordinal"),
		}}, 1)
		if len(seats) != 1 || seats[0].Subject != "earlier-ordinal" {
			t.Fatalf("digests %v: seats=%+v, want the earlier in-publication ordinal", digests, seats)
		}
	}
}

// A list of source records reads the same whichever digests the capture's
// location gave its publications.
func TestHMC376RecordListsFollowPublicationOrderNotTheDigest(t *testing.T) {
	id := func(digest, suffix string) string {
		return "trace_query:publication-" + strings.Repeat(digest, 64) + "#" + suffix
	}
	for _, digests := range [][2]string{{"a", "f"}, {"f", "a"}} {
		first, second := digests[0], digests[1]
		got := TraceObservationIDsInPublicationOrder([]string{
			id(first, "root_cause_rank:7"), id(second, "root_cause_rank:1"), " ", id(first, "root_cause_rank:6"), id(first, "root_cause_rank:7"),
		})
		want := []string{id(first, "root_cause_rank:6"), id(first, "root_cause_rank:7"), id(second, "root_cause_rank:1")}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("digests %v: got %v, want %v", digests, got, want)
		}
	}
}

// One blocking occurrence published by two records lists them in publication
// order, whichever digests the capture's location gave those records.
func TestHMC376BlockingOccurrenceRecordsFollowPublicationOrderNotTheDigest(t *testing.T) {
	request := traceValueOccurrenceAuthorityRequest()
	id := func(digest string) string {
		return "trace_query:publication-" + strings.Repeat(digest, 64) + "#critical_blocking:1"
	}
	for _, digests := range [][2]string{{"a", "f"}, {"f", "a"}} {
		first := traceBlockingWallClockAuthorityRecord(id(digests[0]), "binder_wait", 1.409, 13762.835861, 13762.837270,
			"peer=binder:496_9-10961", "flags=0x10", "blocking_candidate=true")
		second := first
		second.ID = id(digests[1])
		authorities := BuildTraceBlockingWallClockAuthorities(ObservationLedger{Records: []ObservationRecord{first, second}}, &request)
		if len(authorities) != 1 || len(authorities[0].Occurrences) != 1 {
			t.Fatalf("digests %v: control: want one occurrence, got %+v", digests, authorities)
		}
		got := authorities[0].Occurrences[0].RecordIDs
		if len(got) != 2 || got[0] != first.ID || got[1] != second.ID {
			t.Fatalf("digests %v: record ids %v, want publication order [%s %s]", digests, got, first.ID, second.ID)
		}
	}
}
