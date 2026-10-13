package tracequery

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC233OriginalIOEndpointsRemainSeparateFromValueWindow(t *testing.T) {
	io := IOLatencySummary{EndpointFamily: "block_rq", Dev: "8,0", Op: "R", Sector: 123, Len: 8,
		IssueLine: 2, CompleteLine: 6, IssueTs: 5, CompleteTs: 5.01,
		IssuerBlockedLine: 3, WakeupLine: 7, IssuerBlockedStartTs: 5.001, IssuerBlockedEndTs: 5.011}
	pair := io.RequestEndpoints()
	if pair.Family != "block_rq" || pair.IssueLine != 2 || pair.CompleteLine != 6 || pair.IssueTs != 5 || pair.CompleteTs != 5.01 || pair.Sector != "123" {
		t.Fatalf("request endpoint bridge borrowed a wait/window coordinate: %+v", pair)
	}
	for _, row := range []any{
		RootCauseRankItem{IORequestEndpoints: pair},
		CriticalBlockingCandidate{IORequestEndpoints: pair},
		types.ObservationRecord{IORequestEndpoints: pair},
	} {
		wire, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(wire), "issue_line") || strings.Contains(string(wire), "io_request_endpoints") {
			t.Fatalf("internal identity bridge leaked into model wire: %s", wire)
		}
	}
}

func TestHMC233IOFamilyDoesNotInheritRepresentativeRequestIdentity(t *testing.T) {
	pair := (&IOLatencySummary{EndpointFamily: "block_rq", IssueLine: 2, CompleteLine: 6, IssueTs: 5, CompleteTs: 5.01}).RequestEndpoints()
	a := rcmRankItem("io_latency", ThreadRef{Comm: "issuer", PID: 41}, 10, 5, 5.01, 2, 6)
	a.IORequestEndpoints, a.IOValueCaliber = pair, types.TraceIOValueCaliberRQResidence
	b := a
	b.IORequestEndpoints = (&IOLatencySummary{EndpointFamily: "block_rq", IssueLine: 3, CompleteLine: 7, IssueTs: 5, CompleteTs: 5.01}).RequestEndpoints()
	b.LineStart, b.LineEnd = 3, 7
	merged := mergeSameThreadTypeRankFamily(Query{TimeStart: 5, TimeEnd: 5.02}, false, []RootCauseRankItem{a, b}, []int{0, 1})
	if merged.IORequestEndpoints != nil || merged.MemberCount != 2 {
		t.Fatalf("same-type family borrowed single-request identity: endpoints=%+v members=%d", merged.IORequestEndpoints, merged.MemberCount)
	}
	rank := RootCauseRankResult{Items: uxr1FacetFamilyFixture()}
	for i := range rank.Items {
		rank.Items[i].IORequestEndpoints = pair
	}
	reconcileAdjacentIOFacetFamilySeats(&rank)
	found := false
	for _, item := range rank.Items {
		if item.Source == adjacentIOFacetFamilySource {
			found = true
			if item.IORequestEndpoints != nil {
				t.Fatal("facet family borrowed its lead's single-request identity")
			}
		}
	}
	if !found {
		t.Fatal("expected a facet family")
	}
	for _, item := range rank.AbsorbedItems {
		if item.IORequestEndpoints == nil {
			t.Fatal("lossless member lost its original request coordinates")
		}
	}
}
