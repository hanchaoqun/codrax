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
