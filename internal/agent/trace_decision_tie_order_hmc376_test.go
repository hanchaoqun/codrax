package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc376PublicationID(digest byte, suffix string) string {
	return "trace_query:publication-" + strings.Repeat(string(digest), 64) + "#" + suffix
}

// Two rows tied on every published value keep one order and one selection,
// whichever digests the capture's location gave their publications.
func TestHMC376TiedDecisionRowsDoNotFollowThePublicationDigest(t *testing.T) {
	row := func(digest byte, suffix, subject string) types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{EvidenceID: hmc376PublicationID(digest, suffix), Subject: subject, ImpactMS: 123.745}
	}
	for _, digests := range [][2]byte{{'a', 'f'}, {'f', 'a'}} {
		projection := types.TraceCausalProjection{AdjacentCauses: []types.TraceCausalProjectionNode{
			row(digests[0], "root_cause_rank:2", "later-ordinal"),
			row(digests[1], "root_cause_rank:1", "earlier-ordinal"),
		}}
		rows := traceDecisionNonCausalContextRows(projection, 1)
		if len(rows) != 1 || rows[0].node.Subject != "earlier-ordinal" {
			t.Fatalf("digests %c/%c: bounded context row=%+v, want the earlier in-publication ordinal", digests[0], digests[1], rows)
		}
	}
	for _, digests := range [][2]byte{{'a', 'f'}, {'f', 'a'}} {
		first := row(digests[0], "root_cause_rank:1", "published-first")
		second := row(digests[1], "root_cause_rank:1", "published-second")
		if traceDecisionNodeTieBefore(first, second) || traceDecisionNodeTieBefore(second, first) {
			t.Fatalf("digests %c/%c: rows equal apart from the digest must stay in publication order", digests[0], digests[1])
		}
	}
}

// Every bounded reader that breaks a full tie keeps the earlier in-publication
// ordinal. The two rows of a case are tied on every value the site compares and
// the later ordinal is given first, so neither the digest nor a stable sort can
// produce the expected row.
func TestHMC376EveryTieBreakSiteKeepsTheEarlierPublicationOrdinal(t *testing.T) {
	const subject = "tied-subject"
	type node = types.TraceCausalProjectionNode
	boardSeat := func(id, direction string) node {
		return node{EvidenceID: id, Subject: subject, Rank: 1, EffectiveImpactMS: 5, EffectiveImpactPublished: true,
			ChainRelevance: "on_chain", FixDirection: direction, RankBoardTarget: "app-100", RankBoardParamsFingerprint: "fp",
			RankQueryWindowStartTs: 10, RankQueryWindowEndTs: 10.1}
	}
	board := func(seats ...node) types.TraceCausalProjection {
		return types.TraceCausalProjection{ArtifactPath: "/captures/x.trace", WindowStartTs: 10, WindowEndTs: 10.1,
			WakeupPathQueryWindowStartTs: 10, WakeupPathQueryWindowEndTs: 10.1, RankedSeats: seats}
	}
	for _, site := range []struct {
		name string
		kept func(t *testing.T, later, earlier string) []node
	}{
		{"different_window_ranked_seats", func(_ *testing.T, later, earlier string) []node {
			seat := func(id string) node {
				return node{EvidenceID: id, Subject: subject, Rank: 1, EffectiveImpactMS: 5, RankQueryWindowStartTs: 20, RankQueryWindowEndTs: 20.1}
			}
			return traceFinalDifferentWindowRankedSeats(types.TraceCausalProjection{WindowStartTs: 10, WindowEndTs: 10.1,
				WakeupPathQueryWindowStartTs: 10, WakeupPathQueryWindowEndTs: 10.1, RankedSeats: []node{seat(later), seat(earlier)}}, 1)
		}},
		{"fix_direction_leaders_inside_one_board", func(_ *testing.T, later, earlier string) []node {
			return traceFinalFixDirectionLeaders(board(boardSeat(later, "dir_a"), boardSeat(earlier, "dir_b")), 1)
		}},
		{"fix_direction_leader_of_one_direction", func(_ *testing.T, later, earlier string) []node {
			return traceFinalFixDirectionLeaders(board(boardSeat(later, "dir_a"), boardSeat(earlier, "dir_a")), 0)
		}},
		{"evidence_boundaries", func(_ *testing.T, later, earlier string) []node {
			row := func(id string) node {
				return node{EvidenceID: id, Subject: subject, UndrillableReason: "missing_wakeup", StartTs: 10}
			}
			return traceDecisionEvidenceBoundaries(types.TraceCausalProjection{AdjacentCauses: []node{row(later), row(earlier)}}, 1)
		}},
		{"occupancy_order", func(_ *testing.T, later, earlier string) []node {
			first, second := node{EvidenceID: later, Subject: subject, ImpactMS: 5}, node{EvidenceID: earlier, Subject: subject, ImpactMS: 5}
			if traceDecisionOccupancyRanksBefore(first, second) {
				return []node{first}
			}
			return []node{second}
		}},
		{"semantic_optimization_spans", func(_ *testing.T, later, earlier string) []node {
			// Spans equal on the whole inventory identity are one span; only
			// their recorded lines tell these two apart, and no order reads those.
			row := func(id string, line int) node {
				return node{EvidenceID: id, Subject: subject, ImpactMS: 5, SemanticClass: "gc", LineStart: line}
			}
			return traceDecisionSemanticOptimizationSpans(types.TraceCausalProjection{SemanticSpans: []node{row(later, 10), row(earlier, 20)}}, 1)
		}},
		{"rank_display_groups_without_board_identity", func(t *testing.T, later, earlier string) []node {
			row := func(id string) node { return node{EvidenceID: id, Subject: subject, Rank: 1} }
			groups := traceFinalRankDisplayGroups(types.TraceCausalProjection{}, []node{row(later), row(earlier)}, 1)
			if len(groups) != 2 {
				t.Fatalf("control: rows without a board identity keep one group each, got %d", len(groups))
			}
			return groups[0].shown
		}},
	} {
		t.Run(site.name, func(t *testing.T) {
			for _, digests := range [][2]byte{{'a', 'f'}, {'f', 'a'}} {
				later, earlier := hmc376PublicationID(digests[0], "root_cause_rank:2"), hmc376PublicationID(digests[1], "root_cause_rank:1")
				kept := site.kept(t, later, earlier)
				if len(kept) != 1 || kept[0].EvidenceID != earlier {
					t.Errorf("digests %c/%c: kept %+v, want only the earlier in-publication ordinal", digests[0], digests[1], kept)
				}
			}
		})
	}
}

// Conflicting target measurements are all disclosed, in the order of their
// values, whichever digests the capture's location gave their records.
func TestHMC376ConflictingFrequencyTargetsDoNotFollowThePublicationDigest(t *testing.T) {
	for _, digests := range [][2]byte{{'a', 'f'}, {'f', 'a'}} {
		result := b1631JoinResult(t, "/captures/same.ftrace", "result.json", "child-a", "1.000", 2100000)
		low := result.Observations[0]
		high := low
		high.RichNotes = append([]string(nil), low.RichNotes...)
		high.Value = "2.000"
		low.ID, high.ID = hmc376PublicationID(digests[0], "target_cpu_running:1"), hmc376PublicationID(digests[1], "target_cpu_running:2")
		result.Observations = []types.ObservationRecord{high, low}
		ctx := boundedRuntimeReaderHandoffTestContext()
		ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
		var running []string
		for _, row := range answerDocFrequencySourceJoinRows(ctx, result.TraceEvidenceAuthority.FrequencyLimitWitnesses) {
			running = append(running, row.running)
		}
		if strings.Join(running, ",") != "1.000,2.000" {
			t.Fatalf("digests %c/%c: disclosed running values %v, want [1.000 2.000]", digests[0], digests[1], running)
		}
	}
}
