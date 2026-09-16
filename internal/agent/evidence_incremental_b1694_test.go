package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1694IncrementalMergeDoesNotSkipTypedClaimsOrCompletion(t *testing.T) {
	base := types.EvidenceItem{Kind: types.EvidenceRegistration, Scope: types.ScopeLine, Source: "factory.go",
		LineStart: 4, LineEnd: 4, Predicate: "constructs", Object: "Sink", AnchorKind: types.AnchorReturn,
		AnchorSymbol: "create", OwnerSymbol: "create", Summary: "same description", Snippet: "same source",
		GroundingStatus: types.GroundingGrounded}
	base.ID = types.StableEvidenceID(base)
	complete := base
	complete.Subject = "Factory.create"
	complete.ID = types.StableEvidenceID(complete)
	for _, sameID := range []bool{false, true} {
		incoming := complete
		if sameID {
			incoming.ID = base.ID
		}
		got, changed := MergeEvidenceItemsIfChanged([]types.EvidenceItem{base}, []types.EvidenceItem{incoming})
		if !changed || len(got) != 1 || got[0].Subject != complete.Subject || got[0].ID != base.ID {
			t.Fatalf("typed completion was skipped, sameID=%v: changed=%v got=%+v", sameID, changed, got)
		}
		if _, changed := MergeEvidenceItemsIfChanged(got, got); changed {
			t.Fatal("exact replay of canonical row must remain a no-op")
		}
	}
	sibling := complete
	sibling.Kind, sibling.Predicate, sibling.Object = types.EvidenceConcrete, "returns", "false"
	sibling.ID = types.StableEvidenceID(sibling)
	got, changed := MergeEvidenceItemsIfChanged([]types.EvidenceItem{complete}, []types.EvidenceItem{sibling})
	if !changed || len(got) != 2 {
		t.Fatalf("independent claim was mistaken for a no-op: %+v", got)
	}
}
