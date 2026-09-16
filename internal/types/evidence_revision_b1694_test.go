package types

import (
	"reflect"
	"testing"
)

func b1694Fact(kind EvidenceKind, predicate, object string) EvidenceItem {
	item := EvidenceItem{Kind: kind, Scope: ScopeLine, Source: "sample.go", LineStart: 7, LineEnd: 7,
		Subject: "choose", Predicate: predicate, Object: object, AnchorKind: AnchorReturn, AnchorSymbol: "choose",
		OwnerSymbol: "choose", GroundingStatus: GroundingGrounded, Summary: "same description", Snippet: "same source"}
	item.ID = StableEvidenceID(item)
	return item
}

func TestB1694MutablePreservesSameCoordinateDistinctClaims(t *testing.T) {
	definition := b1694Fact(EvidenceDirect, "defines", "")
	definition.AnchorKind = AnchorDefinition
	first := b1694Fact(EvidenceConcrete, "returns", "false")
	second := b1694Fact(EvidenceConcrete, "returns", "true")
	for _, order := range [][]EvidenceItem{{definition, first, second}, {second, first, definition}} {
		mu := NewMutableState("independent claims")
		for _, item := range order {
			mu.AppendEvidence([]EvidenceItem{item})
		}
		mu.AppendEvidence(order)
		got := mu.EmittedEvidence()
		if len(got) != 3 {
			t.Fatalf("same coordinate is not same claim: got %+v", got)
		}
		for _, want := range order {
			found := false
			for _, row := range got {
				if row.ID == want.ID && row.Kind == want.Kind && row.Predicate == want.Predicate && row.Object == want.Object {
					found = true
				}
			}
			if !found {
				t.Errorf("claim identity or typed value lost: %+v", want)
			}
		}
	}
}

func TestB1694MutableDoesNotChooseAmbiguousRevision(t *testing.T) {
	a := b1694Fact(EvidenceRegistration, "constructs", "Sink")
	a.Subject = "left"
	a.ID = StableEvidenceID(a)
	b := a
	b.Subject = "right"
	b.ID = StableEvidenceID(b)
	sparse := a
	sparse.Subject = ""
	sparse.ID = StableEvidenceID(sparse)
	mu := NewMutableState("ambiguous endpoint")
	mu.AppendEvidence([]EvidenceItem{a, b})
	mu.AppendEvidence([]EvidenceItem{sparse})
	if got := mu.EmittedEvidence(); len(got) != 3 {
		t.Fatalf("ambiguous sparse row must not pick a sibling: %+v", got)
	}
}

func TestB1694MutableKeepsKnownProvenanceAndCandidateBoundaries(t *testing.T) {
	for name, change := range map[string]func(*EvidenceItem){
		"origin":    func(e *EvidenceItem) { e.Origin = ClaimOriginLog },
		"authority": func(e *EvidenceItem) { e.Authority = AuthorityHistorical },
		"candidate": func(e *EvidenceItem) { e.DerivationCandidate = true },
	} {
		t.Run(name, func(t *testing.T) {
			a := b1694Fact(EvidenceDirect, "returns", "false")
			a.Origin, a.Authority = ClaimOriginCurrentRepo, AuthorityFactual
			a.ID = StableEvidenceID(a)
			b := a
			change(&b)
			b.ID = StableEvidenceID(b)
			mu := NewMutableState("source identity")
			mu.AppendEvidence([]EvidenceItem{a, b})
			got := mu.EmittedEvidence()
			if len(got) != 2 || !reflect.DeepEqual(got[0], a) || !reflect.DeepEqual(got[1], b) {
				t.Fatalf("source or candidate identity conflated: %+v", got)
			}
		})
	}
}

func TestB1694MatchIndexPrefersExactIdentityAndRetiresMovedKeys(t *testing.T) {
	a := b1694Fact(EvidenceDirect, "returns", "false")
	b := b1694Fact(EvidenceDirect, "returns", "true")
	idx := NewEvidenceMatchIndex([]EvidenceItem{a, b})
	if i, ok := idx.Find(a); !ok || i != 0 {
		t.Fatal("exact identity lost to a coordinate sibling")
	}
	// A stable ID is not enough across different source coordinates.
	moved := a
	moved.Source = "moved.go"
	if _, ok := idx.Find(moved); ok {
		t.Fatal("bare ID borrowed another source coordinate")
	}
	idx.Set(0, moved)
	if _, ok := idx.Find(a); ok {
		t.Fatal("replaced slot retained its old source alias")
	}
	if i, ok := idx.Find(moved); !ok || i != 0 || !reflect.DeepEqual(idx.Item(i), moved) {
		t.Fatal("replacement slot not indexed")
	}
}

func TestB1694MetadataBackfillAndSparsePromotionRemainLegal(t *testing.T) {
	base := b1694Fact(EvidenceDirect, "returns", "false")
	metadata := base
	metadata.Origin, metadata.Authority = ClaimOriginCurrentRepo, AuthorityFactual
	metadata.ID = StableEvidenceID(metadata)
	for _, order := range [][]EvidenceItem{{base, metadata}, {metadata, base}} {
		mu := NewMutableState("metadata backfill")
		mu.AppendEvidence(order)
		if got := mu.EmittedEvidence(); len(got) != 1 || got[0].Origin != ClaimOriginCurrentRepo || got[0].Authority != AuthorityFactual {
			t.Fatalf("non-conflicting provenance backfill stopped working: %+v", got)
		}
	}
	sparse := b1694Fact(EvidenceRegistration, "constructs", "Sink")
	sparse.Subject, sparse.Condition = "", "branch"
	sparse.ID = StableEvidenceID(sparse)
	complete := sparse
	complete.Subject, complete.Condition = "Factory.create", ""
	complete.ID = StableEvidenceID(complete)
	for _, order := range [][]EvidenceItem{{sparse, complete}, {complete, sparse}} {
		mu := NewMutableState("endpoint completion")
		mu.AppendEvidence(order)
		mu.AppendEvidence(order)
		got := mu.EmittedEvidence()
		if len(got) != 1 || got[0].Subject != complete.Subject || got[0].Condition != "" || got[0].ID != order[0].ID {
			t.Fatalf("unique promotion must remain atomic and preserve accepted identity: %+v", got)
		}
	}
}
