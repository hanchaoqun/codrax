package types

import (
	"strings"
	"testing"
)

// write_behavior_contract_witness_test.go — V5-1 (colleague_merge_audit
// §40.10): the contract-kind → witness-kind matrix is total over the closed
// kind set, source text is admitted for file_layout only, and the single
// covering predicate applies it.

func TestWriteBehaviorWitnessMatrixTotalOverKnownKinds(t *testing.T) {
	kinds := AllWriteBehaviorContractKinds()
	if len(kinds) != 8 || !IsKnownWriteBehaviorContractKind("file_layout") || IsKnownWriteBehaviorContractKind("layout") {
		t.Fatalf("closed kind source drift: %v", kinds)
	}
	for _, kind := range kinds {
		if !IsKnownWriteBehaviorContractKind(string(kind)) {
			t.Fatalf("fixture drift: %q is not a known kind", kind)
		}
		if len(WriteBehaviorContractWitnessKinds(kind)) == 0 {
			t.Fatalf("kind %q admits no witness — the matrix must be total", kind)
		}
		if !WriteBehaviorWitnessSatisfies(kind, WriteBehaviorWitnessVerificationProbe) ||
			!WriteBehaviorWitnessSatisfies(kind, WriteBehaviorWitnessProjectTest) {
			t.Fatalf("kind %q must admit executed probe and project-test witnesses", kind)
		}
		if got, want := WriteBehaviorWitnessSatisfies(kind, WriteBehaviorWitnessSourceText), kind == WriteBehaviorFileLayout; got != want {
			t.Fatalf("kind %q source_text admission = %v, want %v (file_layout only)", kind, got, want)
		}
	}
	// An unknown / unnormalized kind (persisted plans bypass the normalizer)
	// admits executed witnesses like the normalizer's observable default —
	// never an obligation nothing can discharge, never a source reading.
	if !WriteBehaviorWitnessSatisfies("made_up", WriteBehaviorWitnessVerificationProbe) ||
		!WriteBehaviorWitnessSatisfies("", WriteBehaviorWitnessProjectTest) ||
		WriteBehaviorWitnessSatisfies("made_up", WriteBehaviorWitnessSourceText) {
		t.Fatal("an unknown kind must admit executed witnesses only")
	}
	for _, witness := range AllWriteBehaviorWitnessKinds() {
		if !IsKnownWriteBehaviorWitnessKind(witness) {
			t.Fatalf("closed set drift: %q", witness)
		}
	}
	if IsKnownWriteBehaviorWitnessKind("prose") {
		t.Fatal("open witness kind accepted")
	}
	// The category fallback covers exactly the contract-lane categories.
	want := map[string]WriteBehaviorWitnessKind{
		"probe_contract_refs":        WriteBehaviorWitnessVerificationProbe,
		"probe_soft_contract_refs":   WriteBehaviorWitnessVerificationProbe,
		"probe_placement_refs":       WriteBehaviorWitnessVerificationProbe,
		"project_test_contract_refs": WriteBehaviorWitnessProjectTest,
		"source_contract_refs":       WriteBehaviorWitnessSourceText,
	}
	for category, witness := range want {
		if got, ok := WriteBehaviorWitnessKindForConfidenceCategory(category); !ok || got != witness {
			t.Fatalf("category %q → %q/%v, want %q", category, got, ok, witness)
		}
	}
	for _, category := range []string{"source_text_presence", "probe_changed_symbol", "probe_execution", ""} {
		if _, ok := WriteBehaviorWitnessKindForConfidenceCategory(category); ok {
			t.Fatalf("category %q must not carry a contract witness", category)
		}
	}
}

func TestVerificationConfidenceRecordCoversContractAppliesTheMatrix(t *testing.T) {
	observable := WriteBehaviorContract{ID: "c1", Kind: WriteBehaviorObservable}
	layout := WriteBehaviorContract{ID: "c1", Kind: WriteBehaviorFileLayout}
	legacySource := VerificationConfidenceRecord{Category: "source_contract_refs", Status: "satisfied", ContractRefs: []string{"c1"}}
	if VerificationConfidenceRecordCoversContract(legacySource, observable) {
		t.Fatal("a legacy (unstamped) source witness must not cover an observable contract")
	}
	if !VerificationConfidenceRecordCoversContract(legacySource, layout) {
		t.Fatal("a source witness covers a file_layout contract")
	}
	stamped := legacySource
	stamped.WitnessKind = WriteBehaviorWitnessSourceText
	if VerificationConfidenceRecordCoversContract(stamped, observable) || !VerificationConfidenceRecordCoversContract(stamped, layout) {
		t.Fatal("the stamped source witness must agree with the fallback")
	}
	probe := VerificationConfidenceRecord{Category: "probe_contract_refs", Status: "satisfied", WitnessKind: WriteBehaviorWitnessVerificationProbe, ContractRefs: []string{"c1"}}
	if !VerificationConfidenceRecordCoversContract(probe, observable) || !VerificationConfidenceRecordCoversContract(probe, layout) {
		t.Fatal("an executed probe witness covers every kind")
	}
	spoof := legacySource
	spoof.WitnessKind = WriteBehaviorWitnessVerificationProbe
	if VerificationConfidenceRecordCoversContract(spoof, observable) {
		t.Fatal("a stamp contradicting its category must not be trusted")
	}
	placement := VerificationConfidenceRecord{Category: "probe_placement_refs", Status: "satisfied", WitnessKind: WriteBehaviorWitnessVerificationProbe, ContractRefs: []string{"c1"}}
	if VerificationConfidenceRecordCoversContract(placement, observable) {
		t.Fatal("placement records bind placement_refs and never cover a behavior contract")
	}
	advisory := VerificationConfidenceRecord{Category: "source_text_presence", Status: "advisory", WitnessKind: WriteBehaviorWitnessSourceText, ContractRefs: []string{"c1"}}
	if VerificationConfidenceRecordCoversContract(advisory, layout) {
		t.Fatal("an advisory disclosure never covers")
	}
	missing := probe
	missing.Status = "missing"
	if VerificationConfidenceRecordCoversContract(missing, observable) {
		t.Fatal("a missing record never covers")
	}
	other := probe
	other.ContractRefs = []string{"c2"}
	if VerificationConfidenceRecordCoversContract(other, observable) {
		t.Fatal("coverage is by exact contract id")
	}
	covered := CoveredWriteBehaviorContractIDs([]WriteBehaviorContract{observable, {ID: "c2", Kind: WriteBehaviorFileLayout}},
		[]VerificationConfidenceRecord{legacySource, {Category: "source_contract_refs", Status: "satisfied", ContractRefs: []string{"c2"}}})
	if _, ok := covered["c1"]; ok {
		t.Fatal("c1 (observable) must not be covered by source text")
	}
	if _, ok := covered["c2"]; !ok {
		t.Fatal("c2 (file_layout) must be covered by source text")
	}
}

func TestWriteBehaviorContractKindWitnessTeachingNamesEveryKindWithoutInternalNames(t *testing.T) {
	teaching := WriteBehaviorContractKindWitnessTeaching()
	for _, kind := range AllWriteBehaviorContractKinds() {
		if !strings.Contains(teaching, string(kind)) {
			t.Fatalf("teaching omits kind %q: %s", kind, teaching)
		}
	}
	if strings.Contains(teaching, "never a source-line fragment") || !strings.Contains(teaching, "evidence_ref") {
		t.Fatalf("teaching must not contradict the grounding lane that cites source evidence_refs for runtime kinds: %s", teaching)
	}
	if strings.Index(teaching, "file_layout") > strings.Index(teaching, "observable") {
		t.Fatalf("the source-readable group must be named first: %s", teaching)
	}
	for _, leak := range []string{"source_contract_refs", "WitnessKind", "witness_kind", "PatchEffect", "source_text_presence", "ledger"} {
		if strings.Contains(teaching, leak) {
			t.Fatalf("teaching leaks internal name %q: %s", leak, teaching)
		}
	}
}
