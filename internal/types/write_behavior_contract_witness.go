package types

import "strings"

// write_behavior_contract_witness.go — V5-1 (colleague_merge_audit §40.10):
// the contract-kind → witness-kind matrix. A verification-confidence record
// may discharge a behavior contract only when the KIND of witness it carries
// is one the contract's KIND admits. The write-domain form of the §29.21 proof
// lane: static presence never mints a runtime observation. `file_layout` is
// the only kind a post-apply source/file-system reading can satisfy; every
// runtime kind needs an executed verification probe or project test.
//
// This table is the single source for the ledger/profile consumers, the
// controller coverage row, the producer census and the analyzer teaching
// (WriteBehaviorContractKindWitnessTeaching) — R2' same-source rule.

// WriteBehaviorWitnessKind names the closed set of witness sources a
// VerificationConfidenceRecord can carry.
type WriteBehaviorWitnessKind string

const (
	// WriteBehaviorWitnessVerificationProbe: an executed, identity-coupled
	// verification probe passed and referenced the contract.
	WriteBehaviorWitnessVerificationProbe WriteBehaviorWitnessKind = "verification_probe"
	// WriteBehaviorWitnessProjectTest: an exact typed project-test candidate
	// executed successfully and its declared observation named the contract.
	WriteBehaviorWitnessProjectTest WriteBehaviorWitnessKind = "project_test"
	// WriteBehaviorWitnessSourceText: a plan-owned post-apply source line,
	// bound through the applied PatchEffect, matched the contract value.
	WriteBehaviorWitnessSourceText WriteBehaviorWitnessKind = "source_text"
)

// AllWriteBehaviorWitnessKinds is the closed witness set in stable order.
func AllWriteBehaviorWitnessKinds() []WriteBehaviorWitnessKind {
	return []WriteBehaviorWitnessKind{
		WriteBehaviorWitnessVerificationProbe,
		WriteBehaviorWitnessProjectTest,
		WriteBehaviorWitnessSourceText,
	}
}

// IsKnownWriteBehaviorWitnessKind reports membership in the closed set.
func IsKnownWriteBehaviorWitnessKind(v WriteBehaviorWitnessKind) bool {
	switch v {
	case WriteBehaviorWitnessVerificationProbe, WriteBehaviorWitnessProjectTest, WriteBehaviorWitnessSourceText:
		return true
	default:
		return false
	}
}

// WriteBehaviorContractWitnessKinds is THE matrix: the witness kinds that may
// discharge a contract of the given kind. file_layout is the only kind a
// source reading can satisfy. Every other known kind — and, exactly like
// NormalizeWriteBehaviorContracts (which defaults an unknown kind to
// observable), any unknown or unnormalized kind reaching the verifier
// through a persisted plan — admits executed witnesses only; an unknown
// kind therefore never becomes an obligation nothing can discharge.
func WriteBehaviorContractWitnessKinds(kind WriteBehaviorContractKind) []WriteBehaviorWitnessKind {
	if kind == WriteBehaviorFileLayout {
		return []WriteBehaviorWitnessKind{
			WriteBehaviorWitnessVerificationProbe,
			WriteBehaviorWitnessProjectTest,
			WriteBehaviorWitnessSourceText,
		}
	}
	return []WriteBehaviorWitnessKind{
		WriteBehaviorWitnessVerificationProbe,
		WriteBehaviorWitnessProjectTest,
	}
}

// WriteBehaviorWitnessSatisfies reports whether (kind, witness) is in the
// matrix.
func WriteBehaviorWitnessSatisfies(kind WriteBehaviorContractKind, witness WriteBehaviorWitnessKind) bool {
	for _, allowed := range WriteBehaviorContractWitnessKinds(kind) {
		if allowed == witness {
			return true
		}
	}
	return false
}

// verificationConfidenceContractCategories are the record categories that
// speak about behavior contracts by contract_ref. Rendered-text placement
// records bind placement_refs and stay on their own lane.
var verificationConfidenceContractCategories = map[string]bool{
	"probe_contract_refs":        true,
	"probe_soft_contract_refs":   true,
	"project_test_contract_refs": true,
	"source_contract_refs":       true,
}

// VerificationConfidenceRecordIsContractWitness reports whether the record's
// category is a behavior-contract lane (any status).
func VerificationConfidenceRecordIsContractWitness(rec VerificationConfidenceRecord) bool {
	return verificationConfidenceContractCategories[strings.TrimSpace(rec.Category)]
}

// WriteBehaviorWitnessKindForConfidenceCategory maps a producer category to
// the witness kind that category can only ever carry. It is the legacy
// fallback for records persisted before WitnessKind was stamped, and the
// consistency reference the producer census checks against.
func WriteBehaviorWitnessKindForConfidenceCategory(category string) (WriteBehaviorWitnessKind, bool) {
	switch strings.TrimSpace(category) {
	case "probe_contract_refs", "probe_soft_contract_refs", "probe_placement_refs":
		return WriteBehaviorWitnessVerificationProbe, true
	case "project_test_contract_refs":
		return WriteBehaviorWitnessProjectTest, true
	case "source_contract_refs":
		return WriteBehaviorWitnessSourceText, true
	default:
		return "", false
	}
}

// VerificationConfidenceRecordWitnessKind resolves the record's witness kind:
// the stamped field when present, else the category fallback. A stamp that
// contradicts the only witness its category can carry is not trusted (the
// category is the producer's structural fact; the stamp must agree with it).
func VerificationConfidenceRecordWitnessKind(rec VerificationConfidenceRecord) (WriteBehaviorWitnessKind, bool) {
	byCategory, categoryKnown := WriteBehaviorWitnessKindForConfidenceCategory(rec.Category)
	if stamped := WriteBehaviorWitnessKind(strings.TrimSpace(string(rec.WitnessKind))); stamped != "" {
		if !IsKnownWriteBehaviorWitnessKind(stamped) || (categoryKnown && stamped != byCategory) {
			return "", false
		}
		return stamped, true
	}
	return byCategory, categoryKnown
}

// VerificationConfidenceRecordCoversContract is the single covering
// predicate: a satisfied contract-lane record whose witness kind the
// contract's kind admits, naming the contract by exact id.
func VerificationConfidenceRecordCoversContract(rec VerificationConfidenceRecord, contract WriteBehaviorContract) bool {
	if strings.TrimSpace(rec.Status) != "satisfied" || !VerificationConfidenceRecordIsContractWitness(rec) {
		return false
	}
	witness, ok := VerificationConfidenceRecordWitnessKind(rec)
	if !ok || !WriteBehaviorWitnessSatisfies(contract.Kind, witness) {
		return false
	}
	id := strings.TrimSpace(contract.ID)
	if id == "" {
		return false
	}
	for _, ref := range rec.ContractRefs {
		if strings.TrimSpace(ref) == id {
			return true
		}
	}
	return false
}

// CoveredWriteBehaviorContractIDs returns the ids of the given contracts that
// some record covers under the matrix.
func CoveredWriteBehaviorContractIDs(contracts []WriteBehaviorContract, records []VerificationConfidenceRecord) map[string]struct{} {
	out := map[string]struct{}{}
	for _, contract := range contracts {
		for _, rec := range records {
			if VerificationConfidenceRecordCoversContract(rec, contract) {
				out[strings.TrimSpace(contract.ID)] = struct{}{}
				break
			}
		}
	}
	return out
}

// WriteBehaviorContractKindWitnessTeaching renders the matrix as the one
// sentence the analyzer schema and skill prompt both carry (R2' same source).
// It names contract kinds and witness sources in analyzer vocabulary only.
func WriteBehaviorContractKindWitnessTeaching() string {
	var sourceKinds, runtimeKinds []string
	for _, kind := range AllWriteBehaviorContractKinds() {
		if WriteBehaviorWitnessSatisfies(kind, WriteBehaviorWitnessSourceText) {
			sourceKinds = append(sourceKinds, string(kind))
		} else {
			runtimeKinds = append(runtimeKinds, string(kind))
		}
	}
	return "Choose kind by how the contract can be witnessed: " + strings.Join(sourceKinds, ", ") +
		" describes what the repository must contain after the change (files, sections, declarations that must be present) and can be read from the changed files; " +
		strings.Join(runtimeKinds, ", ") + " describe runtime facts that only an executed verification probe or project test can observe, so their expected value states the runtime outcome rather than the source text that produces it (citing the producing line in evidence_ref is fine)."
}
