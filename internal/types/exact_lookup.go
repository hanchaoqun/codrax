package types

import (
	"path/filepath"
	"sort"
	"strings"
)

// BuildExactResolutionContract derives the AnswerContract-level
// contract for exact-target questions. It is intentionally
// conservative: broad enumeration questions return nil so downstream
// target-mention / absence constraints do not over-trigger.
func BuildExactResolutionContract(rm RequestModel) *ExactResolutionContract {
	targets := ExactResolutionTargets(rm)
	if len(targets) == 0 {
		return nil
	}
	label := exactResolutionSubjectLabel(rm)
	if label == "" {
		label = "target"
	}
	policy := exactResolutionContextPolicyForRM(rm)
	contract := &ExactResolutionContract{
		TargetKind:           rm.AnswerSubject.Kind,
		TargetLabel:          label,
		Targets:              targets,
		AllowAbsence:         true,
		RequireTargetMention: true,
		AliasRequiresProof:   true,
		RelatedContextPolicy: policy,
		RelatedContextTerms:  exactResolutionValidatedContextTerms(targets, rm.AnalyzerHints.ExactContextTerms, rm.AnalyzerHints.Keywords),
	}
	if hint := exactResolutionScopeHintForPolicy(policy); hint != "" {
		contract.RelatedContextScopeHint = hint
	}
	return contract
}

// ExactResolutionTargets returns the user-named primary targets for
// exact-resolution questions (config keys, file paths, single-symbol
// lookups). It is intentionally conservative: broad enumeration
// questions should return nil so downstream absence contracts do not
// over-trigger.
func ExactResolutionTargets(rm RequestModel) []string {
	if !exactResolutionEnabled(rm) {
		return nil
	}
	if targets := MentionedEntitiesFromRawRequest(rm.RawRequest, rm.AnalyzerHints.ExactTargets); len(targets) > 0 {
		return dedupeExactResolutionTargets(exactResolutionFindingKindForRM(rm), targets)
	}
	candidates := exactResolutionMentionedCandidates(rm)
	switch len(candidates) {
	case 0:
		// Provenance-only contract: exact targets must come from the
		// RawRequest-aligned mention lane (either analyzer-proposed
		// exact_targets or deterministic recovery of MentionedEntities).
		// Do NOT promote primary / derived context entities into exact
		// targets when the current request text never named them.
		return nil
	case 1:
		return dedupeExactResolutionTargets(exactResolutionFindingKindForRM(rm), candidates)
	default:
		// Multiple raw-request-mentioned entities is ambiguous unless the
		// analyzer explicitly disambiguated via exact_targets.
		return nil
	}
}

func exactResolutionMentionedCandidates(rm RequestModel) []string {
	if len(rm.AnalyzerHints.MentionedEntities) > 0 {
		return append([]string(nil), rm.AnalyzerHints.MentionedEntities...)
	}
	if recovered := MentionedEntitiesFromRawRequest(rm.RawRequest, rm.AnalyzerHints.PrimaryEntities); len(recovered) > 0 {
		return recovered
	}
	return MentionedEntitiesFromRawRequest(rm.RawRequest, rm.AnalyzerHints.Entities)
}

func MentionedEntitiesFromRawRequest(raw string, candidates []string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(candidates) == 0 {
		return nil
	}
	lowerRaw := strings.ToLower(strings.ReplaceAll(raw, `\`, `/`))
	normRawSymbol := normalizeExactResolutionToken("symbol", raw)
	normRawPath := normalizeExactResolutionToken("path", raw)
	if lowerRaw == "" && normRawSymbol == "" && normRawPath == "" {
		return nil
	}
	seen := make(map[string]bool)
	var out []string
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(strings.Trim(candidate, "`\"' "))
		if candidate == "" {
			continue
		}
		key := normalizeExactResolutionToken("symbol", candidate)
		if looksLikeExactPathToken(candidate) {
			key = normalizeExactResolutionToken("path", candidate)
		}
		if key == "" || seen[key] {
			continue
		}
		explicit := strings.Contains(lowerRaw, strings.ToLower(strings.ReplaceAll(candidate, `\`, `/`))) ||
			strings.Contains(normRawSymbol, normalizeExactResolutionToken("symbol", candidate)) ||
			strings.Contains(normRawPath, normalizeExactResolutionToken("path", candidate))
		if !explicit {
			continue
		}
		seen[key] = true
		out = append(out, candidate)
	}
	return out
}

func DerivedEntitiesFromMentioned(all, mentioned []string) []string {
	if len(all) == 0 {
		return nil
	}
	seenMentioned := make(map[string]bool, len(mentioned))
	for _, item := range mentioned {
		key := normalizeExactResolutionToken("symbol", item)
		if looksLikeExactPathToken(item) {
			key = normalizeExactResolutionToken("path", item)
		}
		if key != "" {
			seenMentioned[key] = true
		}
	}
	seenOut := make(map[string]bool)
	var out []string
	for _, item := range all {
		key := normalizeExactResolutionToken("symbol", item)
		if looksLikeExactPathToken(item) {
			key = normalizeExactResolutionToken("path", item)
		}
		if key == "" || seenMentioned[key] || seenOut[key] {
			continue
		}
		seenOut[key] = true
		out = append(out, item)
	}
	return out
}

func dedupeExactResolutionTargets(kind string, candidates []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(strings.Trim(candidate, "`\"' "))
		if candidate == "" {
			continue
		}
		key := normalizeExactResolutionToken(kind, candidate)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, candidate)
	}
	return out
}

// ExactResolutionPendingTargets returns the subset of contract targets
// whose primary token remains unresolved in prior stage reports.
func ExactResolutionPendingTargets(c *ExactResolutionContract, finds []UnverifiedFinding) []string {
	if c == nil || len(c.Targets) == 0 || len(finds) == 0 {
		return nil
	}
	kind := exactResolutionFindingKind(c)
	seen := make(map[string]bool)
	for _, f := range finds {
		if !exactResolutionFindingKindMatches(kind, f.Kind) {
			continue
		}
		key := normalizeExactResolutionToken(kind, f.Token)
		if key != "" {
			seen[key] = true
		}
	}
	if len(seen) == 0 {
		return nil
	}
	var out []string
	for _, target := range c.Targets {
		key := normalizeExactResolutionToken(kind, target)
		if key != "" && seen[key] {
			out = append(out, target)
		}
	}
	return out
}

// ExactResolutionTextMentionsTarget reports whether text explicitly
// names the exact target under the same normalization the pending-
// target matcher uses.
func ExactResolutionTextMentionsTarget(c *ExactResolutionContract, text, target string) bool {
	kind := exactResolutionFindingKind(c)
	normTarget := normalizeExactResolutionToken(kind, target)
	if normTarget == "" {
		return false
	}
	return strings.Contains(normalizeExactResolutionToken(kind, text), normTarget)
}

// ExactResolutionContextTerms returns the validated same-scope term
// shortlist that the analyzer proposed for nearby-context narrowing.
// These terms are LLM-recommended, then system-validated against the
// request-mentioned exact-target lane before the contract is built.
func ExactResolutionContextTerms(c *ExactResolutionContract) []string {
	if c == nil || len(c.RelatedContextTerms) == 0 {
		return nil
	}
	return append([]string(nil), c.RelatedContextTerms...)
}

func exactResolutionSubjectLabel(rm RequestModel) string {
	switch rm.AnswerSubject.Kind {
	case SubjectConfigKey:
		return "config key"
	case SubjectFilePath:
		return "file path"
	case SubjectFunctionName, SubjectTypeName, SubjectStructField, SubjectInterface, SubjectEnumValue:
		return "symbol"
	case SubjectHandlerRoute:
		return "route"
	case SubjectStringLiteral:
		return "literal"
	}
	if strings.EqualFold(strings.TrimSpace(rm.AnalyzerHints.Kind), "config_mapping") || rm.Scenario == ScenarioConfigTrace {
		return "config key"
	}
	return "target"
}

func exactResolutionContextPolicyForRM(rm RequestModel) ExactResolutionContextPolicy {
	switch exactResolutionSubjectLabel(rm) {
	case "config key":
		return ExactContextSameFamilyGrounded
	case "file path":
		return ExactContextSameDirectoryGrounded
	default:
		return ExactContextGroundedOnly
	}
}

func exactResolutionScopeHintForPolicy(policy ExactResolutionContextPolicy) string {
	switch policy {
	case ExactContextSameFamilyGrounded:
		return "same namespace / prefix family"
	case ExactContextSameDirectoryGrounded:
		return "same directory / module subtree"
	case ExactContextGroundedOnly:
		return "grounded nearby context only"
	default:
		return ""
	}
}

func exactResolutionEnabled(rm RequestModel) bool {
	switch rm.AnswerSubject.Kind {
	case SubjectConfigKey, SubjectFilePath:
		return true
	case SubjectFunctionName, SubjectTypeName, SubjectStructField, SubjectInterface, SubjectEnumValue, SubjectHandlerRoute, SubjectStringLiteral:
		primary := rm.AnalyzerHints.PrimaryEntities
		if len(primary) == 0 {
			primary = rm.AnalyzerHints.Entities
		}
		return rm.Predicates.IsScalarAnswer || len(primary) <= 1
	}
	return strings.EqualFold(strings.TrimSpace(rm.AnalyzerHints.Kind), "config_mapping") || rm.Scenario == ScenarioConfigTrace
}

func exactResolutionFindingKindForRM(rm RequestModel) string {
	if exactResolutionSubjectLabel(rm) == "file path" {
		return "path"
	}
	return "symbol"
}

func exactResolutionFindingKind(c *ExactResolutionContract) string {
	if c == nil {
		return "symbol"
	}
	if c.TargetKind == SubjectFilePath || strings.EqualFold(strings.TrimSpace(c.TargetLabel), "file path") {
		return "path"
	}
	return "symbol"
}

func exactResolutionFindingKindMatches(expected, got string) bool {
	got = strings.TrimSpace(strings.ToLower(got))
	if got == "" {
		got = "symbol"
	}
	return got == expected
}

func exactResolutionValidatedContextTerms(targets, explicit, keywords []string) []string {
	if len(targets) == 0 {
		return nil
	}
	allowed := make(map[string]bool)
	for _, target := range targets {
		for _, term := range ExactResolutionIdentifierTerms(target) {
			if len(term) >= 3 {
				allowed[term] = true
			}
		}
	}
	if len(allowed) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	var out []string
	for _, item := range explicit {
		term := strings.TrimSpace(strings.ToLower(item))
		if len(term) < 3 || !allowed[term] || seen[term] {
			continue
		}
		seen[term] = true
		out = append(out, term)
	}
	if len(out) == 0 {
		for _, keyword := range keywords {
			for _, term := range ExactResolutionIdentifierTerms(keyword) {
				if len(term) < 3 || !allowed[term] || seen[term] {
					continue
				}
				seen[term] = true
				out = append(out, term)
			}
		}
	}
	sort.Strings(out)
	return out
}

func looksLikeExactPathToken(s string) bool {
	s = strings.TrimSpace(strings.ReplaceAll(s, `\`, `/`))
	if s == "" {
		return false
	}
	return strings.Contains(s, "/") || (filepath.Ext(s) != "" && !strings.Contains(s, " "))
}

func normalizeExactResolutionToken(kind, s string) string {
	s = strings.TrimSpace(strings.Trim(s, "`\"' "))
	switch kind {
	case "path":
		s = strings.ReplaceAll(s, `\`, `/`)
		s = strings.ToLower(strings.TrimSpace(s))
		s = strings.TrimPrefix(s, "./")
		return s
	default:
		var b strings.Builder
		for _, r := range strings.ToLower(s) {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				b.WriteRune(r)
			}
		}
		return b.String()
	}
}

// ExactResolutionIdentifierTerms splits an identifier/path-like surface
// into normalized lowercase alphanumeric terms for validator use. It
// is intentionally lexical only: downstream consumers may validate an
// LLM-suggested context term against this set, but they must not treat
// the returned terms themselves as a semantic recommendation.
func ExactResolutionIdentifierTerms(s string) []string {
	s = strings.TrimSpace(strings.ReplaceAll(s, `\`, `/`))
	if s == "" {
		return nil
	}
	var b strings.Builder
	var parts []string
	flush := func() {
		if b.Len() == 0 {
			return
		}
		parts = append(parts, strings.ToLower(b.String()))
		b.Reset()
	}
	prevLower := false
	for _, r := range s {
		switch {
		case (r >= 'A' && r <= 'Z') && prevLower:
			flush()
			b.WriteRune(r + ('a' - 'A'))
			prevLower = false
		case (r >= 'A' && r <= 'Z'):
			b.WriteRune(r + ('a' - 'A'))
			prevLower = false
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevLower = r >= 'a' && r <= 'z'
		default:
			flush()
			prevLower = false
		}
	}
	flush()
	return parts
}
