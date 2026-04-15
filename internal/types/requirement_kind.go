package types

import "strings"

// RequirementKind is the canonical, typed enum for the question-side
// evidence-requirement classification. It is the single source of
// truth consumed by:
//
//   - internal/agent/explorer_erm.go — EvidenceRequirement.Kind, the
//     satisfaction switch, terminal/origin predicate tables, and the
//     answer-chain predicate whitelist builder
//   - internal/tool/emit_analysis.go — normalizeQuestionKind, which
//     coerces LLM-emitted question_kind strings onto this enum
//   - internal/skill/analysis_contract.go — analysisQuestionKinds,
//     the skill-facing enum table rendered into the analyzer prompt
//
// Before this type existed, each of those sites carried its own set
// of bare string literals ("enumeration", "call_chain", ...), and the
// three sets were free to drift apart — there was no compile-time
// closure between "what the analyzer can emit", "what ERM knows how
// to satisfy", and "what the skill prompt advertises". The typed
// constants + AllRequirementKinds() + the RequirementToEvidenceKinds
// map close that loop, and a package-level consistency test asserts
// every RequirementKind has a valid entry in the map and every mapped
// EvidenceKind is a real canonical kind.
//
// RequirementKind is intentionally distinct from EvidenceKind even
// though a few string values happen to coincide ("registration",
// "conditional", "mechanism"). They live on different axes:
//
//   - EvidenceKind classifies the SHAPE of a single evidence item
//     produced by the explorer or a deterministic extractor
//     (direct/conditional/registration/...).
//   - RequirementKind classifies the QUESTION the user asked, so the
//     ERM layer can decide which evidence shapes count toward
//     satisfying it (enumeration questions need direct+registration
//     items, mechanism questions need mechanism+relationship items,
//     etc.). The mapping is explicit — see
//     RequirementToEvidenceKinds.
type RequirementKind string

const (
	// ReqUnknown is the zero value. Used when the analyzer could not
	// classify the question. Never written into an EvidenceRequirement
	// — extractEvidenceRequirementsWithHint treats "" / "unknown" as
	// "fall through to keyword inference".
	ReqUnknown RequirementKind = ""

	// ReqRegistration — "which/how many X register/bind Y", "X 是在哪注册的".
	// Satisfied by EvidenceRegistration items whose subject/object
	// match the requirement's entities and that carry a concrete
	// target symbol (not a bare interface type).
	ReqRegistration RequirementKind = "registration"

	// ReqMechanism — "how does X work", "explain the process of X",
	// "X 怎么实现". Satisfied by EvidenceMechanism items and
	// EvidenceRelationship items whose predicate ties two steps of
	// a process together.
	ReqMechanism RequirementKind = "mechanism"

	// ReqReturnValue — "what does X return", "X.Name() 是什么".
	// Satisfied by EvidenceConcrete items whose subject matches an
	// entity and whose object is a literal value.
	ReqReturnValue RequirementKind = "return_value"

	// ReqConditional — "when does X fire", "under what condition",
	// "什么时候". Satisfied by EvidenceConditional items.
	ReqConditional RequirementKind = "conditional"

	// ReqConfigMapping — "what does config key K control". Satisfied
	// by EvidenceConcrete items drawn from config files and
	// EvidenceMechanism items that describe how a config value
	// reaches runtime behavior.
	ReqConfigMapping RequirementKind = "config_mapping"

	// ReqEnumeration — "list all X", "count of X". Satisfied by a
	// minimum count (≥3 for "satisfied", ≥1 for "partial") of
	// EvidenceDirect + EvidenceRegistration items that each mention
	// at least one of the requirement's entities.
	ReqEnumeration RequirementKind = "enumeration"

	// ReqCallChain — "which X calls Y", "从 A 到 B 怎么调用的".
	// Satisfied by EvidenceRelationship or EvidenceMechanism items
	// AND a textual mention of at least two of the requirement's
	// entities in investigation notes (captures the linkage).
	ReqCallChain RequirementKind = "call_chain"
)

// allRequirementKinds is the canonical, ordered list of non-zero
// RequirementKind values. Order is stable because downstream renderers
// (analysis_contract.analysisQuestionKinds, the skill enum table) rely
// on it. ReqUnknown is excluded — it is the zero value, not a real
// classification, and is rendered separately by the skill table.
var allRequirementKinds = []RequirementKind{
	ReqRegistration,
	ReqMechanism,
	ReqReturnValue,
	ReqConditional,
	ReqConfigMapping,
	ReqEnumeration,
	ReqCallChain,
}

// AllRequirementKinds returns the canonical list of non-zero
// RequirementKind values in the order used by the skill enum table.
// Callers must not mutate the returned slice.
func AllRequirementKinds() []RequirementKind {
	out := make([]RequirementKind, len(allRequirementKinds))
	copy(out, allRequirementKinds)
	return out
}

// IsValid reports whether k is one of the canonical non-zero
// RequirementKind values. ReqUnknown is NOT valid — use k != ReqUnknown
// for "is set" checks.
func (k RequirementKind) IsValid() bool {
	for _, v := range allRequirementKinds {
		if v == k {
			return true
		}
	}
	return false
}

// NormalizeRequirementKind coerces a raw LLM-emitted question_kind
// string onto the canonical enum. Synonyms the analyzer prompt
// historically accepted (e.g. "register" → ReqRegistration, "count" →
// ReqEnumeration) are still recognised here so we do not break
// existing LLM outputs. Unknown strings return ReqUnknown.
//
// This function is the single coercion point; previously it was
// duplicated across tool/emit_analysis.go (normalizeQuestionKind)
// and agent/explorer_erm.go (extractEvidenceRequirementsWithHint's
// declaredKind path). Keeping it here means adding a synonym once
// updates both call sites.
func NormalizeRequirementKind(s string) RequirementKind {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "registration", "register":
		return ReqRegistration
	case "mechanism", "process", "flow":
		return ReqMechanism
	case "return_value", "return-value", "return", "value_of":
		return ReqReturnValue
	case "conditional", "condition":
		return ReqConditional
	case "config_mapping", "config-mapping":
		return ReqConfigMapping
	case "enumeration", "enumerate", "list", "count":
		return ReqEnumeration
	case "call_chain", "call-chain", "callchain", "calls":
		return ReqCallChain
	}
	return ReqUnknown
}

// RequirementToEvidenceKinds is the explicit, auditable map from a
// question-side RequirementKind to the EvidenceKinds that can
// contribute to its satisfaction. This is the single artifact that
// binds the two axes together — before it existed the mapping was
// implicit in the body of explorer_erm.checkRequirementSatisfaction,
// scattered across seven switch cases, and impossible to verify at
// the package boundary.
//
// Semantics:
//
//   - A kind appears in the slice if a single item of that kind CAN
//     contribute to satisfying the requirement. This is a necessary
//     condition for the closure test, not a full decision rule — the
//     actual satisfaction predicate (count thresholds, entity match,
//     concrete-value shape check, etc.) still lives in
//     checkRequirementSatisfaction. Think of this map as declaring
//     "these are the evidence shapes the ERM layer even looks at for
//     this requirement."
//
//   - Every RequirementKind in AllRequirementKinds() MUST have an
//     entry with len(entry) >= 1. The TestRequirementToEvidenceKinds_
//     Closure test asserts this at build time.
//
//   - Every EvidenceKind in any entry MUST be a canonical kind from
//     AllEvidenceKinds(). Same test enforces it.
//
// The map is intentionally narrow: it covers the primary evidence
// source for each requirement. Secondary sources (e.g. call_chain
// also consults investigation-note textual matches, return_value also
// uses a free-text "returns" heuristic) live inside
// checkRequirementSatisfaction because they are fuzzy, not structural.
var RequirementToEvidenceKinds = map[RequirementKind][]EvidenceKind{
	ReqRegistration:  {EvidenceRegistration, EvidenceConcrete},
	ReqMechanism:     {EvidenceMechanism, EvidenceRelationship},
	ReqReturnValue:   {EvidenceConcrete},
	ReqConditional:   {EvidenceConditional},
	ReqConfigMapping: {EvidenceConcrete, EvidenceMechanism},
	ReqEnumeration:   {EvidenceDirect, EvidenceRegistration},
	ReqCallChain:     {EvidenceRelationship, EvidenceMechanism},
}
