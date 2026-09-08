package types

import (
	"reflect"
	"testing"
)

func TestB1627RelationProviderKeepsCandidatesAdvisory(t *testing.T) {
	for _, tc := range []struct {
		name     string
		kind     EvidenceKind
		relation TypedRelationKind
	}{
		{"registration", EvidenceRegistration, TypedRelationRegisters},
		{"configuration", EvidenceConcrete, TypedRelationConfigures},
		{"route", EvidenceRegistration, TypedRelationRoutesTo},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := EvidenceItem{
				ID: "candidate", Kind: tc.kind, Scope: ScopeLine,
				Subject: "Registrar", Object: "Target", Source: "pkg/wiring.rb", LineStart: 17,
				AnchorKind: AnchorCall, GroundingStatus: GroundingGrounded,
				ContextRole: EvidenceContextRoleDefining, DerivationCandidate: true,
			}
			exact := candidate
			exact.ID, exact.DerivationCandidate = "independent", false
			for _, items := range [][]EvidenceItem{{candidate}, {candidate, exact}, {exact, candidate}} {
				before := append([]EvidenceItem(nil), items...)
				provider := EvidenceRelationCandidateSource{Items: items}
				query := TypedRelationQuery{Kinds: []TypedRelationKind{tc.relation}, Sources: []string{"Registrar"}, Purpose: TypedRelationPurposePromptHint}
				rows := provider.TypedRelationCandidates(query)
				weak, strong := 0, 0
				for _, row := range rows {
					if row.SourceName != "Registrar" || row.Member.Name != "Target" || row.Member.File != "pkg/wiring.rb" || row.Member.Line != 17 {
						t.Fatalf("provider rewrote navigation: %+v", row)
					}
					if row.Precision == TypedRelationPrecisionHeuristic && !row.CoverageGateEligible() {
						weak++
					}
					if row.Precision == TypedRelationPrecisionExactEvidence {
						strong++
					}
				}
				if weak != 1 || strong != len(items)-1 {
					t.Errorf("candidate was upgraded or hid independent exact evidence: %+v", rows)
				}
				query.Purpose = TypedRelationPurposeCoverageGate
				rows = provider.TypedRelationCandidates(query)
				if len(rows) != len(items)-1 {
					t.Errorf("candidate became coverage authority or exact evidence was lost: %+v", rows)
				}
				if !reflect.DeepEqual(items, before) {
					t.Fatal("provider mutated evidence")
				}
			}
		})
	}
}

func b1627BridgeRelationItems() (EvidenceItem, EvidenceItem) {
	terminal := EvidenceItem{
		ID: "terminal", Kind: EvidenceConcrete, Subject: "Worker.Name", Predicate: "returns", Object: `"worker"`,
		Source: "pkg/worker.go", LineStart: 8, Producer: "bridge_literal_terminal", Scope: ScopeLine,
		AnchorKind: AnchorReturn, AnchorSymbol: "Name", OwnerSymbol: "Worker", GroundingStatus: GroundingGrounded,
	}
	bridge := EvidenceItem{
		ID: "bridge", Kind: EvidenceDataflowPath, Predicate: "resolution_chain", Object: terminal.Object,
		Source: "pkg/registry.go", LineStart: 20, DerivedFrom: []string{terminal.ID}, Producer: "bridge_literal", Scope: ScopeLine,
		AnchorSymbol: terminal.Subject, OwnerSymbol: "Register", GroundingStatus: GroundingGrounded,
	}
	return bridge, terminal
}

func TestB1627BridgeRelationPropagatesWeakestDependency(t *testing.T) {
	for _, mode := range []string{"bridge", "terminal", "transitive", "same_id_collision", "cycle_with_candidate"} {
		t.Run(mode, func(t *testing.T) {
			bridge, terminal := b1627BridgeRelationItems()
			items := []EvidenceItem{bridge, terminal}
			switch mode {
			case "bridge":
				items[0].DerivationCandidate = true
			case "terminal":
				items[1].DerivationCandidate = true
			case "transitive", "cycle_with_candidate":
				items[1].DerivedFrom = []string{"middle"}
				middle := EvidenceItem{ID: "middle", DerivedFrom: []string{"ancestor"}}
				ancestor := EvidenceItem{ID: "ancestor", DerivationCandidate: true}
				if mode == "cycle_with_candidate" {
					ancestor.DerivedFrom = []string{terminal.ID}
				}
				items = append(items, middle, ancestor)
			case "same_id_collision":
				other := terminal
				other.Producer, other.DerivationCandidate = "concrete_values", true
				items = append(items, other)
			}
			for _, reversed := range []bool{false, true} {
				pool := append([]EvidenceItem(nil), items...)
				if reversed {
					for i, j := 0, len(pool)-1; i < j; i, j = i+1, j-1 {
						pool[i], pool[j] = pool[j], pool[i]
					}
				}
				before := append([]EvidenceItem(nil), pool...)
				provider := EvidenceRelationCandidateSource{Items: pool}
				query := TypedRelationQuery{Kinds: []TypedRelationKind{TypedRelationRegisters}, Sources: []string{"Register"}, Purpose: TypedRelationPurposePromptHint}
				rows := provider.TypedRelationCandidates(query)
				if len(rows) != 1 || rows[0].Precision != TypedRelationPrecisionHeuristic || rows[0].CoverageGateEligible() || rows[0].Member.Name != "worker" || rows[0].Member.Line != 8 || rows[0].SourceLine != 20 {
					t.Errorf("weak dependency hidden or upgraded (reversed=%v): %+v", reversed, rows)
				}
				query.Purpose = TypedRelationPurposeCoverageGate
				if rows := provider.TypedRelationCandidates(query); len(rows) != 0 {
					t.Errorf("weak joined relation entered coverage gate: %+v", rows)
				}
				if !reflect.DeepEqual(pool, before) {
					t.Fatal("dependency propagation mutated source records")
				}
			}
		})
	}
}

func TestB1627BridgeIndependentExactRelationStillCovers(t *testing.T) {
	bridge, terminal := b1627BridgeRelationItems()
	candidate := bridge
	candidate.ID, candidate.DerivationCandidate = "candidate-bridge", true
	for _, items := range [][]EvidenceItem{{candidate, bridge, terminal}, {bridge, terminal, candidate}} {
		query := TypedRelationQuery{Kinds: []TypedRelationKind{TypedRelationRegisters}, Sources: []string{"Register"}, Purpose: TypedRelationPurposeCoverageGate}
		rows := (EvidenceRelationCandidateSource{Items: items}).TypedRelationCandidates(query)
		if len(rows) != 1 || rows[0].Precision != TypedRelationPrecisionExactEvidence || !rows[0].CoverageGateEligible() {
			t.Fatalf("independent exact bridge lost original authority: %+v", rows)
		}
	}
}
