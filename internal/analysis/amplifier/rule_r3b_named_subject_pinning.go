package amplifier

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// r3bNamedSubjectMustInclude (A1, 2026-07-03) pins RESOLVED user-named
// subject identifiers into AnswerContract.MustIncludeTerms for
// single-subject explanation shapes. The s1a failure class: the user asked
// about `gate.Run`, the model resolved everything through the sibling
// RunWith, and the user-named symbol never appeared on the answer surface.
// R3 covers enumeration questions only; this rule covers the complement —
// "the user named a real code thing; the answer must at least mention it".
//
// Gates (all typed/precise):
//  1. NOT an enumeration-member lane (R3's territory):
//     !CanUseAnalyzerEntitiesAsHardPrincipalMembers.
//  2. No runtime-artifact identity (log/trace runs carry artifact tokens
//     as entities; those are provenance, not answer-surface obligations).
//  3. Entity provenance Resolved==true with Resolution symbol or file —
//     the oracle/file-index PROVED the user-named thing exists.
//  4. Identifier-shaped surface (structural check via the shared
//     anchor-token shape rule).
//  5. Bounded: at most 3 pins (named subjects, not lists); dedupe against
//     existing MustInclude/typed terms case-insensitively.
//
// Enforcement rides the EXISTING checkMustIncludeOracle lane —
// ViolMustInclude is soft-by-default (caveat, strict-promotable), so a
// miss surfaces as a disclosure, never a hard loop.
func r3bNamedSubjectMustInclude(rm types.RequestModel, contract *types.AnswerContract) *Observation {
	if types.CanUseAnalyzerEntitiesAsHardPrincipalMembers(rm) {
		return nil
	}
	if rm.HasTypedRuntimeArtifactPathIdentity() {
		return nil
	}
	existing := map[string]bool{}
	for _, term := range types.NormalizedMustIncludeTerms(*contract) {
		existing[strings.ToLower(strings.TrimSpace(term.Text))] = true
	}
	var pinned []string
	for _, prov := range rm.AnalyzerHints.EntityProvenance {
		if len(pinned) >= 3 {
			break
		}
		if !prov.Resolved {
			continue
		}
		if prov.Resolution != types.EntityResolutionSymbol && prov.Resolution != types.EntityResolutionFile {
			continue
		}
		surface := strings.TrimSpace(prov.Surface)
		if !types.AnchorTokenShaped(surface) {
			continue
		}
		key := strings.ToLower(surface)
		if existing[key] {
			continue
		}
		existing[key] = true
		contract.MustInclude = append(contract.MustInclude, surface)
		contract.MustIncludeTerms = append(contract.MustIncludeTerms, types.ContractTerm{
			Text:   surface,
			Kind:   types.InferContractTermKind(surface),
			Source: types.ContractTermSourceAnalyzerEntity,
		})
		pinned = append(pinned, surface)
	}
	if len(pinned) == 0 {
		return nil
	}
	return &Observation{
		Rule:   "R3b_named_subject_must_include",
		Field:  "answer_contract.must_include",
		After:  strings.Join(pinned, ", "),
		Reason: fmt.Sprintf("pinned %d resolved user-named subject identifier(s) so a single-subject explanation cannot ship without naming the asked subject", len(pinned)),
	}
}

func init() {
	postCompileRules = append(postCompileRules, r3bNamedSubjectMustInclude)
}
