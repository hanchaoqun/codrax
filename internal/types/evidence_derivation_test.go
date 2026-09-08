package types

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func derivationTestEvidence() EvidenceItem {
	return EvidenceItem{Kind: EvidenceConcrete, Scope: ScopeLine, Subject: "Factory.build", Predicate: "binds", Object: "Handler",
		Source: "src/factory.ts", LineStart: 12, LineEnd: 12, AnchorSymbol: "Factory.build", AnchorKind: AnchorCall,
		Snippet: "return prefix + Helper.transform(value)", Summary: "Factory.build may bind Handler", Producer: "concrete_values_extractor",
		GroundingStatus: GroundingGrounded, Origin: ClaimOriginCurrentRepo, Authority: AuthorityFactual, Salience: SalienceLoadBearing}
}

func TestDerivationCandidateIdentityAndRoundTrip(t *testing.T) {
	proved := derivationTestEvidence()
	candidate := proved
	candidate.DerivationCandidate = true
	if StableEvidenceID(candidate) == StableEvidenceID(proved) || EvidenceRevisionKey(candidate) == EvidenceRevisionKey(proved) {
		t.Fatal("candidate and precise evidence must not share an identity/revision")
	}
	candidate.ID, proved.ID = "explicit-collision", "explicit-collision"
	if EvidenceStableMergeKey(candidate) == EvidenceStableMergeKey(proved) {
		t.Fatal("explicit ID bypasses the derivation boundary")
	}
	for _, pair := range [][2]EvidenceItem{{candidate, proved}, {proved, candidate}} {
		merged := MergeEvidenceItemByStableID(pair[0], pair[1])
		if !EvidenceIsDerivationCandidate(merged) {
			t.Fatal("a forced merge silently upgraded a candidate")
		}
		data, err := json.Marshal(merged)
		if err != nil {
			t.Fatal(err)
		}
		var decoded EvidenceItem
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatal(err)
		}
		if !EvidenceIsDerivationCandidate(decoded) || decoded.Source != proved.Source || decoded.Snippet != proved.Snippet {
			t.Fatalf("serialization lost limitation or original source: %+v", decoded)
		}
	}
	if !candidate.IsCitable() || EvidenceDerivationBoundary(candidate) == "" || EvidenceDerivationBoundary(proved) != "" {
		t.Fatal("locatable source and proof qualification must stay separate")
	}
	candidate.Authority = AuthorityHistorical
	for _, pair := range [][2]EvidenceItem{{candidate, proved}, {proved, candidate}} {
		merged := MergeEvidenceItemByStableID(pair[0], pair[1])
		if merged.Authority != AuthorityHistorical || !merged.DerivationCandidate {
			t.Fatalf("forced merge discarded weaker source qualification: %+v", merged)
		}
	}
}

func TestDerivationCandidateLedgerDoesNotUpgradeSourceLocation(t *testing.T) {
	for _, status := range []GroundingStatus{"", GroundingGrounded, GroundingRecovered} {
		candidate := derivationTestEvidence()
		candidate.ID, candidate.GroundingStatus, candidate.DerivationCandidate = "candidate", status, true
		before := candidate
		ledger := CompileObservationLedger(ObservationLedgerInput{EvidenceItems: []EvidenceItem{candidate}})
		if len(ledger.Records) != 1 {
			t.Fatalf("candidate lost: %+v", ledger)
		}
		r := ledger.Records[0]
		if r.ClaimAuthority == ObservationClaimAuthorityIndependentlyProven || r.Origin != AnswerEvidenceOriginSystemInference || ObservationRecordHasCurrentSourceLineSpan(r) {
			t.Errorf("source location upgraded candidate: %+v", r)
		}
		if r.SourceRef.Path != candidate.Source || r.Span.LineStart != candidate.LineStart || r.Summary != candidate.Summary || r.RawExcerpt != candidate.Snippet || !strings.Contains(strings.Join(r.RichNotes, "\n"), EvidenceDerivationBoundary(candidate)) {
			t.Errorf("candidate context lost: %+v", r)
		}
		if !reflect.DeepEqual(candidate, before) {
			t.Fatal("ledger mutated source evidence")
		}
		idx := compileCurrentSourceSupportWitnessIndex([]EvidenceItem{candidate}, nil)
		if len(idx.witnesses) != 0 {
			t.Fatal("candidate laundered aggregate source support")
		}
		proved := candidate
		proved.ID, proved.DerivationCandidate, proved.GroundingStatus = "precise", false, GroundingGrounded
		for _, rows := range [][]EvidenceItem{{candidate, proved}, {proved, candidate}} {
			mixed := CompileObservationLedger(ObservationLedgerInput{EvidenceItems: rows})
			if len(mixed.Records) != 2 {
				t.Fatalf("proof and candidate collapsed: %+v", mixed)
			}
			strong := 0
			for _, r := range mixed.Records {
				if r.ClaimAuthority == ObservationClaimAuthorityIndependentlyProven {
					strong++
				}
			}
			if strong != 1 {
				t.Fatalf("independent precise evidence not retained: %+v", mixed)
			}
		}
	}
}

func TestDerivationCandidateAttachmentIdentityIsNotProof(t *testing.T) {
	candidate := derivationTestEvidence()
	candidate.ID, candidate.DerivationCandidate = "candidate", true
	proved := candidate
	proved.ID, proved.DerivationCandidate = "precise", false
	for _, rows := range [][]EvidenceItem{{candidate}, {candidate, proved}, {proved, candidate}} {
		ledger := CompileObservationLedger(ObservationLedgerInput{
			RepoRoot: "/repo", EvidenceItems: rows,
			RuntimeArtifactPreflight: NormalizeRuntimeArtifactPreflightProfile(RuntimeArtifactPreflightProfile{
				Artifacts: []RuntimeArtifactPreflightArtifact{{Kind: "log", Source: "/repo/" + candidate.Source, Carrier: "attachment"}},
			}),
		})
		if len(ledger.Records) != len(rows) {
			t.Fatalf("independent proof and candidate merged: %+v", ledger.Records)
		}
		for _, record := range ledger.Records {
			if record.SourceRef.ArtifactID == "" || record.SourceRef.CaptureIdentityPath == "" || record.SourceRef.Path != candidate.Source {
				t.Errorf("source/capture identity lost: %+v", record)
			}
			if record.ID == "evidence:candidate" {
				if record.ClaimAuthority != ObservationClaimAuthorityModelInference || record.Origin != AnswerEvidenceOriginSystemInference {
					t.Errorf("attachment identity upgraded candidate: %+v", record)
				}
			} else if record.ClaimAuthority != ObservationClaimAuthorityDirectObservation || record.Origin != AnswerEvidenceOriginRuntimeArtifact {
				t.Errorf("ordinary attached observation changed: %+v", record)
			}
		}
	}
}
