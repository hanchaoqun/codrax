package types

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

func handoffDerivationCandidateFixture() EvidenceItem {
	return EvidenceItem{ID: "candidate-call", Kind: EvidenceConcrete, Scope: ScopeLine,
		Source: "src/factory.ts", LineStart: 12, LineEnd: 12,
		Subject: "Factory.build", AnchorSymbol: "Factory.build", AnchorKind: AnchorCall,
		Predicate: "binds", Object: "Handler", GroundingStatus: GroundingGrounded,
		DerivationCandidate: true}
}

func assertHandoffCandidateJSON(t *testing.T, ref AcceptedEvidenceRef, want bool) {
	t.Helper()
	data, err := json.Marshal(ref)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	got, present := fields["derivation_candidate"]
	if (want && got != true) || (!want && present) {
		t.Fatalf("candidate limitation did not survive compact reference: want=%v JSON=%s", want, data)
	}
}

func TestB1627HandoffCandidateLifecycleAndSnapshot(t *testing.T) {
	item := handoffDerivationCandidateFixture()
	before := item
	ref, ok := AcceptedEvidenceRefFromEvidenceItem(item)
	if !ok {
		t.Fatal("candidate location was discarded")
	}
	assertHandoffCandidateJSON(t, ref, true)
	if ref.Source != item.Source || ref.LineStart != item.LineStart || ref.LineEnd != item.LineEnd || ref.ClaimForm != ClaimFormOf(item) || ref.GroundingStatus != item.GroundingStatus {
		t.Fatalf("source shape or grounding was rewritten: %+v", ref)
	}
	m := NewMutableState("inspect candidate")
	m.AppendEvidence([]EvidenceItem{item})
	fork := m.ForkForExploreDispatch()
	for _, state := range []*MutableState{m, fork} {
		refs := state.EvidenceClosure().AcceptedEvidenceRefs()
		if len(refs) != 1 {
			t.Fatalf("candidate refs=%+v", refs)
		}
		assertHandoffCandidateJSON(t, refs[0], true)
		refs[0].Source = "mutated"
		if state.EvidenceClosure().AcceptedEvidenceRefs()[0].Source != item.Source {
			t.Fatal("ref clone aliases storage")
		}
	}
	m.MergeExploreFork(fork)
	snapshot := ReadRunSnapshotFromBusContext(&BusContext{Mutable: m}, "candidate-roundtrip")
	path := filepath.Join(t.TempDir(), "read-run.json")
	if err := WriteReadRunSnapshotToFile(&snapshot, path); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadReadRunSnapshotFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.AcceptedEvidence) != 1 {
		t.Fatalf("snapshot refs=%+v", loaded.AcceptedEvidence)
	}
	assertHandoffCandidateJSON(t, loaded.AcceptedEvidence[0], true)
	if !reflect.DeepEqual(item, before) {
		t.Fatal("handoff mutated original evidence")
	}
}

func TestB1627HandoffCandidateMergeCannotEraseLimitation(t *testing.T) {
	item := handoffDerivationCandidateFixture()
	candidate, _ := AcceptedEvidenceRefFromEvidenceItem(item)
	item.DerivationCandidate = false
	plain, _ := AcceptedEvidenceRefFromEvidenceItem(item)
	assertHandoffCandidateJSON(t, plain, false)
	for _, pair := range [][2]AcceptedEvidenceRef{{candidate, plain}, {plain, candidate}} {
		got := mergeAcceptedEvidenceRefs(pair[:1], pair[1:])
		if len(got) != 1 {
			t.Fatalf("exact duplicate expanded: %+v", got)
		}
		assertHandoffCandidateJSON(t, got[0], true)
		again := normalizeAcceptedEvidenceRefs(got)
		if !reflect.DeepEqual(got, again) {
			t.Fatal("normalization is not idempotent")
		}
	}
	plain.ID = "independently-proved-call"
	for _, pair := range [][]AcceptedEvidenceRef{{candidate, plain}, {plain, candidate}} {
		got := normalizeAcceptedEvidenceRefs(pair)
		if len(got) != 2 {
			t.Fatalf("independent identity collapsed: %+v", got)
		}
		for _, ref := range got {
			assertHandoffCandidateJSON(t, ref, ref.ID == candidate.ID)
		}
	}
	// Sharing an ID is not enough to transfer the limitation to another
	// physical source reference; the existing exact location key still owns it.
	for _, mutate := range []func(*AcceptedEvidenceRef){
		func(ref *AcceptedEvidenceRef) { ref.Source = "src/other.ts" },
		func(ref *AcceptedEvidenceRef) { ref.LineStart++ },
		func(ref *AcceptedEvidenceRef) { ref.LineEnd++ },
	} {
		other := plain
		other.ID = candidate.ID
		mutate(&other)
		got := normalizeAcceptedEvidenceRefs([]AcceptedEvidenceRef{candidate, other})
		if len(got) != 2 {
			t.Fatalf("distinct source references collapsed: %+v", got)
		}
		assertHandoffCandidateJSON(t, got[0], true)
		assertHandoffCandidateJSON(t, got[1], false)
	}
}

func TestB1627HandoffCandidateAfterCapOnlyLimitsExistingRef(t *testing.T) {
	item := handoffDerivationCandidateFixture()
	candidate, _ := AcceptedEvidenceRefFromEvidenceItem(item)
	item.DerivationCandidate = false
	plain, _ := AcceptedEvidenceRefFromEvidenceItem(item)
	refs := []AcceptedEvidenceRef{plain}
	for i := 1; i < toolHandoffMaxAcceptedEvidence+3; i++ {
		refs = append(refs, AcceptedEvidenceRef{ID: "other-" + strconv.Itoa(i)})
	}
	refs = append(refs, candidate)
	got := normalizeAcceptedEvidenceRefs(refs)
	if len(got) != toolHandoffMaxAcceptedEvidence || got[0].ID != plain.ID || got[len(got)-1].ID != "other-63" {
		t.Fatalf("original selection/count changed: %+v", got)
	}
	assertHandoffCandidateJSON(t, got[0], true)
}
