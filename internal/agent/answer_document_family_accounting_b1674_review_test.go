package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Independent public-entrypoint review: matching a thread name and publishing
// more family records do not transfer chain authority between observations.
func TestB1674ReviewPublicFamilyKeepsNonChainEvidenceOutOfCausalSeats(t *testing.T) {
	for _, lang := range []string{"en", "zh"} {
		for _, lane := range []string{"adjacent", "background"} {
			for _, reverse := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/reverse=%t", lang, lane, reverse), func(t *testing.T) {
					rows := b1674FamilyRows("io_latency", 47, "0.782", "sum_disjoint")[:2]
					other := rows[1]
					other.ID = "trace_query:nonchain#root_cause_rank:1"
					other.SourceRef.ToolCallID, other.SourceRef.PayloadRef = "nonchain-query", "/payload/nonchain.json"
					other.Role = types.AnswerAggregateRoleSupportingCoverage
					other.RichNotes = append([]string(nil), other.RichNotes...)
					for i, note := range other.RichNotes {
						switch note {
						case "chain_relevance=on_chain":
							other.RichNotes[i] = "chain_relevance=" + lane
						case "causality=self_wall_clock":
							other.RichNotes[i] = "causality=" + lane
						case "member_count=47":
							other.RichNotes[i] = "member_count=99"
						case "member_max_ms=0.782":
							other.RichNotes[i] = "member_max_ms=8.125"
						case "rank=1":
							if lane == "background" {
								other.RichNotes[i] = "rank=0"
							}
						}
					}
					rows = append(rows, other)
					if reverse {
						rows[1], rows[2] = rows[2], rows[1]
					}
					board, prompt, set := b1674FamilyPublicPrompt(t, rows, lang)
					if !strings.Contains(board, "47 measurement records") {
						t.Fatal("the on-chain positive publication disappeared")
					}
					if lane == "adjacent" {
						line := b1674LineWith(board, "channel=adjacent")
						if !strings.Contains(line, "99 measurement records") || !strings.Contains(line, "record maximum 8.125 ms") {
							t.Fatalf("the adjacent measurement must remain visible in its own channel: %s", line)
						}
					}
					seatCount := 0
					for _, projection := range set.Projections {
						for _, seat := range types.TraceAnswerDecisionEliminableSeats(projection, 0) {
							seatCount++
							if seat.EvidenceID != "trace_query:family#root_cause_rank:1" || seat.FamilyMemberCount != 47 || seat.ChainRelevance != "on_chain" {
								t.Fatalf("non-chain accounting changed a causal seat: %+v", seat)
							}
						}
					}
					if seatCount != 1 {
						t.Fatalf("expected one on-chain positive seat, got %d", seatCount)
					}
					readerNeedle, readerCount := "Rank 1, client-100:", "47 measurement records"
					if lang == "zh" {
						readerNeedle, readerCount = "第 1 位，client-100：", "47 条计量记录"
					}
					for face, line := range map[string]string{
						"axis B": b1674LineWith(prompt, "rank=#1; subject=`client-100`; kind=`io_latency`"),
						"reader": b1674LineWith(prompt, readerNeedle),
					} {
						want := "47 measurement records"
						if face == "reader" {
							want = readerCount
						}
						if !strings.Contains(line, want) || !strings.Contains(line, "0.782") || strings.Contains(line, "99") || strings.Contains(line, "8.125") {
							t.Fatalf("%s borrowed a same-subject non-chain family: %s", face, line)
						}
					}
				})
			}
		}
	}
}

func TestB1674ReviewPublicFamilyDoesNotBorrowDisplayFoldMaximum(t *testing.T) {
	rows := b1674FamilyRows("io_latency", 47, "", "sum_disjoint")[:2]
	rows[1].RichNotes = append(rows[1].RichNotes, "folded_rows=98", "folded_max_ms=9.000")
	board, _, set := b1674FamilyPublicPrompt(t, rows, "en")
	node := b1674FamilyNode(t, set)
	if node.FamilyMemberCount != 47 || node.FamilyMemberMaxMS != 0 || node.MergedCount != 98 || node.MergedMaxMS != 9 {
		t.Fatalf("independent family/display-fold premise lost: %+v", node)
	}
	line := b1674LineWith(board, "#1 root-cause seat")
	if !strings.Contains(line, "47 measurement records") || strings.Contains(line, "record maximum") || strings.Contains(line, "98 measurement records") {
		t.Fatalf("display-fold count/maximum filled an absent family measurement: %s", line)
	}
}
