package amplifier

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// r3TypedIdentifierMustInclude pins identifier-shaped entities from
// rm.AnalyzerHints.Entities into AnswerContract.MustInclude when the
// question is enumeration. Forces the finalizer to emit each named
// entity in the answer; without this the LLM is free to drop
// individual items from a longer set when verbosity budget is tight.
//
// See docs/design/analyzer_amplifier_layer.md §5 R3.
//
// Gates:
//
//  1. types.CanUseAnalyzerEntitiesAsHardPrincipalMembers(rm) == true.
//     This shared typed trait proves AnalyzerHints.Entities is a
//     principal-member lane, not merely a mixed search / relation-target
//     hint. Relation-shaped enumerations ("which handlers call X",
//     "which modules import Y", "which agents can invoke Z") fail here:
//     they need extractor AnswerSymbols / support lanes to establish
//     principal members after exploration, instead of pinning every
//     analyzer entity up front.
//  2. distinctEntityCount(rm.AnalyzerHints.Entities) ≥ 1. Empty
//     entity list = nothing to pin.
//  3. At least one entity is identifier-shaped. By default this uses
//     the strict isIdentifierLike filter (CamelCase / snake_case /
//     dotted names) so concept-words ("stage" / "agent" / lowercase
//     prose) are dropped. For an explicit exhaustive / bounded
//     category enumeration, however, the analyzer contract says
//     Entities already ARE the enumerated members when
//     IsRelationalLookup=false, so lowercase package/module/directory
//     names are valid code entities and must be pinned too.
//  4. The entity is not already present in
//     out.AnswerContract.MustInclude (dedupe).
//
// Source choice — same lesson as R1/R2 (Phase 2.1-fix-2 + 3.1):
// rm.AnalyzerHints.Entities is the LLM-emit slot for "subjects the
// LLM identified", not rm.TermGraph.Canonical (which is raw-text
// surfaces only).
//
// axis_collapse safety: R3 modifies AnswerContract.MustInclude
// only. It does NOT touch SubTopics or IsCategoryEnumeration, so
// it cannot steer rm into the axis_collapse trigger conditions
// (those require ≥2 SubTopics + cat=true). Safe to fire even when
// R1's gate #6 / R2's gate #2 would have blocked.
//
// Action: append each qualifying entity to MustInclude (preserving
// AnalyzerHints.Entities order, deduping case-insensitively against
// existing MustInclude entries). Emits one Observation summarising
// the pin count + first few names.
func r3TypedIdentifierMustInclude(rm types.RequestModel, contract *types.AnswerContract) *Observation {
	if !types.CanUseAnalyzerEntitiesAsHardPrincipalMembers(rm) {
		return nil
	}
	trustEnumerationMembers := exhaustiveEnumerationPinsMembers(rm)
	if distinctEntityCount(rm.AnalyzerHints.Entities) == 0 {
		return nil
	}

	// Build a case-folded lookup of existing MustInclude / typed terms
	// so the dedupe is robust against case-only differences in user-emit.
	existing := make(map[string]struct{}, len(contract.MustInclude)+len(contract.MustIncludeTerms))
	for _, m := range contract.MustInclude {
		key := strings.ToLower(strings.TrimSpace(m))
		if key == "" {
			continue
		}
		existing[key] = struct{}{}
	}
	for _, m := range contract.MustIncludeTerms {
		key := strings.ToLower(strings.TrimSpace(m.Text))
		if key == "" {
			continue
		}
		existing[key] = struct{}{}
	}

	var added []string
	seen := make(map[string]struct{}, len(rm.AnalyzerHints.Entities))
	for _, e := range rm.AnalyzerHints.Entities {
		t := strings.TrimSpace(e)
		if !isMustIncludePinCandidate(t, trustEnumerationMembers) {
			continue
		}
		key := strings.ToLower(t)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		if _, alreadyIn := existing[key]; alreadyIn {
			continue
		}
		added = append(added, t)
		existing[key] = struct{}{}
	}
	if len(added) == 0 {
		return nil
	}
	contract.MustInclude = append(contract.MustInclude, added...)
	for _, term := range added {
		contract.MustIncludeTerms = append(contract.MustIncludeTerms, types.ContractTerm{
			Text:   term,
			Kind:   pinnedEnumerationTermKind(term, trustEnumerationMembers),
			Source: types.ContractTermSourceAnalyzerEntity,
		})
	}

	preview := added
	if len(preview) > 3 {
		preview = preview[:3]
	}
	suffix := ""
	if len(added) > len(preview) {
		suffix = fmt.Sprintf(" + %d more", len(added)-len(preview))
	}
	return &Observation{
		Rule:   "R3_typed_identifier_mustinclude",
		Field:  "answer_contract.must_include",
		Before: fmt.Sprintf("%d", len(contract.MustInclude)-len(added)),
		After:  fmt.Sprintf("%d", len(contract.MustInclude)),
		Reason: fmt.Sprintf("pinned %d typed identifier(s): %s%s under enumeration intent",
			len(added), strings.Join(preview, ", "), suffix),
	}
}

func exhaustiveEnumerationPinsMembers(rm types.RequestModel) bool {
	return types.HasBoundedCategoryEnumerationMembers(rm)
}

func isMustIncludePinCandidate(s string, trustEnumerationMember bool) bool {
	if isIdentifierLike(s) {
		return true
	}
	if !trustEnumerationMember {
		return false
	}
	return types.IsCodeIdentitySurface(s)
}

func pinnedEnumerationTermKind(term string, trustEnumerationMember bool) types.ContractTermKind {
	kind := types.InferContractTermKind(term)
	if !trustEnumerationMember || kind != types.ContractTermSymbol {
		return kind
	}
	if looksLikePackageModuleMember(term) {
		return types.ContractTermFileStem
	}
	return kind
}

func looksLikePackageModuleMember(term string) bool {
	t := strings.TrimSpace(term)
	if !types.IsCodeIdentitySurface(t) {
		return false
	}
	for _, r := range t {
		if r == '.' || r == '-' || r == '/' || r == ':' || r == '@' {
			return true
		}
		if r >= 'A' && r <= 'Z' {
			return false
		}
		if r >= 'a' && r <= 'z' {
			return true
		}
	}
	return false
}

func init() {
	postCompileRules = append(postCompileRules, r3TypedIdentifierMustInclude)
}
