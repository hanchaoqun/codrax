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
		RelatedContextTerms:  exactResolutionValidatedContextTerms(rm.AnswerSubject.Kind, policy, targets, rm.AnalyzerHints.ExactContextTerms, rm.AnalyzerHints.Keywords),
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
	if exactResolutionImplicitTargetsDisabled(rm) {
		return nil
	}
	if targets := exactResolutionSubjectCompatibleCandidates(rm, MentionedEntitiesFromRawRequest(rm.RawRequest, rm.AnalyzerHints.ExactTargets)); len(targets) > 0 {
		return dedupeExactResolutionTargets(exactResolutionFindingKindForRM(rm), targets)
	}
	if exactResolutionNeedsExplicitDisambiguation(rm) {
		return nil
	}
	candidates := exactResolutionSubjectCompatibleCandidates(rm, exactResolutionMentionedCandidates(rm))
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

func exactResolutionImplicitTargetsDisabled(rm RequestModel) bool {
	if len(rm.AnalyzerHints.ExactTargets) > 0 {
		return false
	}
	switch rm.AnswerSubject.Kind {
	case SubjectFunctionName,
		SubjectTypeName,
		SubjectFilePath,
		SubjectHandlerRoute,
		SubjectStructField,
		SubjectInterface,
		SubjectEnumValue,
		SubjectStringLiteral:
	default:
		return false
	}
	// Role-locate questions often mention the OUTPUT / context entity
	// ("which function produces AnalysisIR?") rather than an exact
	// answer target the user wants resolved. For these latent-answer
	// lookups, exact-resolution must stay opt-in via explicit
	// exact_targets; otherwise nearby context entities get promoted into
	// the exact-target lane and downstream leads / validators become
	// misleading.
	if strings.EqualFold(strings.TrimSpace(rm.AnalyzerHints.Kind), "return_value") &&
		rm.AnswerSubject.Kind != SubjectReturnValue {
		return true
	}
	if rm.PredicateAxis == AxisReturn && rm.AnswerSubject.Kind != SubjectReturnValue {
		return true
	}
	return false
}

func exactResolutionNeedsExplicitDisambiguation(rm RequestModel) bool {
	if len(rm.AnalyzerHints.ExactTargets) > 0 {
		return false
	}
	switch exactResolutionSubjectLabel(rm) {
	case "config key", "file path":
		return false
	}
	primary := rm.AnalyzerHints.PrimaryEntities
	if len(primary) == 0 {
		primary = rm.AnalyzerHints.Entities
	}
	if len(primary) == 0 {
		return false
	}
	filtered := primary[:0]
	for _, candidate := range primary {
		if exactResolutionCandidateMatchesSubjectKind(rm.AnswerSubject.Kind, candidate) {
			filtered = append(filtered, candidate)
		}
	}
	primary = filtered
	return len(primary) > 1
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

func exactResolutionSubjectCompatibleCandidates(rm RequestModel, candidates []string) []string {
	if len(candidates) == 0 {
		return nil
	}
	primary := rm.AnalyzerHints.PrimaryEntities
	if len(primary) == 0 {
		var out []string
		for _, candidate := range candidates {
			if exactResolutionCandidateMatchesSubjectKind(rm.AnswerSubject.Kind, candidate) {
				out = append(out, candidate)
			}
		}
		return out
	}
	kind := exactResolutionFindingKindForRM(rm)
	allowed := make(map[string]bool, len(primary))
	for _, item := range primary {
		if !exactResolutionCandidateMatchesSubjectKind(rm.AnswerSubject.Kind, item) {
			continue
		}
		key := normalizeExactResolutionToken(kind, item)
		if key != "" {
			allowed[key] = true
		}
	}
	if len(allowed) == 0 {
		return candidates
	}
	var out []string
	for _, candidate := range candidates {
		if !exactResolutionCandidateMatchesSubjectKind(rm.AnswerSubject.Kind, candidate) {
			continue
		}
		key := normalizeExactResolutionToken(kind, candidate)
		if key != "" && allowed[key] {
			out = append(out, candidate)
		}
	}
	return out
}

func exactResolutionCandidateMatchesSubjectKind(kind AnswerSubjectKind, candidate string) bool {
	candidate = strings.TrimSpace(strings.Trim(candidate, "`\"' "))
	if candidate == "" {
		return false
	}
	switch kind {
	case SubjectFilePath:
		return looksLikeExactPathToken(candidate)
	case SubjectHandlerRoute:
		return looksLikeRouteLikeToken(candidate)
	case SubjectConfigKey:
		if looksLikeExactPathToken(candidate) || looksLikeRouteLikeToken(candidate) {
			return false
		}
		return looksLikeConfigKeyToken(candidate)
	case SubjectStringLiteral, SubjectNumeric, SubjectUnknown:
		return true
	default:
		return !looksLikeExactPathToken(candidate) && !looksLikeRouteLikeToken(candidate)
	}
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

// ExactResolutionTextsMentionAnyTarget reports whether any provided text
// fragment explicitly names one of the contract's exact targets under
// the same normalization used by pending-target matching.
func ExactResolutionTextsMentionAnyTarget(c *ExactResolutionContract, texts ...string) bool {
	if c == nil || len(c.Targets) == 0 {
		return false
	}
	joined := strings.Join(texts, "\n")
	for _, target := range c.Targets {
		if ExactResolutionTextMentionsTarget(c, joined, target) {
			return true
		}
	}
	return false
}

// ExactResolutionDirectAnchorMatchesAnyTarget reports whether any
// structured anchor field (subject / anchor symbol / object) explicitly
// names one of the exact targets. Callers use this to separate genuine
// defining anchors from free-form summary/snippet mentions.
func ExactResolutionDirectAnchorMatchesAnyTarget(c *ExactResolutionContract, subject, anchorSymbol, object string) bool {
	return ExactResolutionTextsMentionAnyTarget(c, subject) ||
		ExactResolutionTextsMentionAnyTarget(c, anchorSymbol) ||
		ExactResolutionTextsMentionAnyTarget(c, object)
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

// ExactResolutionRequiresDefiningPrimaryProof reports whether exact-match
// and alias-match answers should require a production-like / non-auxiliary
// defining anchor rather than any grounded mention. Exact file-path
// lookups remain more permissive because the target itself may live under
// examples/tests/docs without that making it a substitute answer.
func ExactResolutionRequiresDefiningPrimaryProof(c *ExactResolutionContract) bool {
	if c == nil {
		return false
	}
	switch c.TargetKind {
	case SubjectFilePath:
		return false
	case SubjectConfigKey,
		SubjectFunctionName,
		SubjectTypeName,
		SubjectStructField,
		SubjectInterface,
		SubjectEnumValue,
		SubjectHandlerRoute,
		SubjectStringLiteral:
		return true
	default:
		return false
	}
}

// ExactResolutionSourceIsDefiningPrimaryProofLike reports whether a
// grounded repo-relative source path can serve as a defining proof for
// the contract's target kind. This is a structural source-role check,
// not a semantic classifier.
func ExactResolutionSourceIsDefiningPrimaryProofLike(c *ExactResolutionContract, source string) bool {
	source = strings.TrimSpace(source)
	if source == "" {
		return false
	}
	if !ExactResolutionRequiresDefiningPrimaryProof(c) {
		return true
	}
	return !LooksLikeAuxiliaryEvidencePath(source)
}

func exactResolutionFindingKindMatches(expected, got string) bool {
	got = strings.TrimSpace(strings.ToLower(got))
	if got == "" {
		got = "symbol"
	}
	return got == expected
}

func exactResolutionValidatedContextTerms(targetKind AnswerSubjectKind, policy ExactResolutionContextPolicy, targets, explicit, keywords []string) []string {
	if len(targets) == 0 {
		return nil
	}
	allowed := make(map[string]bool)
	for _, term := range exactResolutionAllowedContextTerms(targetKind, policy, targets) {
		if len(term) >= 3 {
			allowed[term] = true
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
	if len(out) == 0 {
		for term := range allowed {
			if seen[term] {
				continue
			}
			seen[term] = true
			out = append(out, term)
		}
	}
	sort.Strings(out)
	return out
}

func exactResolutionAllowedContextTerms(targetKind AnswerSubjectKind, policy ExactResolutionContextPolicy, targets []string) []string {
	if policy == ExactContextSameFamilyGrounded && targetKind == SubjectConfigKey {
		return exactResolutionConfigRootTerms(targets)
	}
	var out []string
	for _, target := range targets {
		out = append(out, ExactResolutionIdentifierTerms(target)...)
	}
	return dedupeExactResolutionTerms(out)
}

func exactResolutionConfigRootTerms(targets []string) []string {
	var out []string
	for _, target := range targets {
		segments := exactResolutionConfigSegments(target)
		if len(segments) == 0 {
			segments = ExactResolutionIdentifierTerms(target)
		}
		if len(segments) == 0 {
			continue
		}
		if root := strings.TrimSpace(strings.ToLower(segments[0])); len(root) >= 3 {
			out = append(out, root)
		}
	}
	return dedupeExactResolutionTerms(out)
}

func exactResolutionConfigSegments(target string) []string {
	target = strings.TrimSpace(strings.ToLower(strings.ReplaceAll(target, `\`, `/`)))
	if target == "" {
		return nil
	}
	parts := strings.FieldsFunc(target, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
	var out []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func dedupeExactResolutionTerms(items []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, item := range items {
		item = strings.TrimSpace(strings.ToLower(item))
		if len(item) < 3 || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

// ExactResolutionSameFamilyMatchScore returns a structural same-family
// match score for an arbitrary candidate surface. It is intentionally
// deterministic: the LLM may recommend related-context anchors, but the
// system decides whether a candidate belongs to the exact target's
// family by validating root/prefix signals derived from the user-named
// target itself rather than from free-form nearby prose.
func ExactResolutionSameFamilyMatchScore(c *ExactResolutionContract, surface string) int {
	if c == nil || c.RelatedContextPolicy != ExactContextSameFamilyGrounded {
		return 0
	}
	terms, compounds := exactResolutionSameFamilySurfaceSets(surface)
	if len(terms) == 0 && len(compounds) == 0 {
		return 0
	}
	signals := exactResolutionSameFamilySignals(c)
	if len(signals.roots) == 0 && len(signals.compounds) == 0 {
		return 0
	}
	score := 0
	for _, root := range signals.roots {
		if root == "" || !terms[root] {
			continue
		}
		score += exactResolutionSameFamilyRootWeight(root)
	}
	for _, compound := range signals.compounds {
		if compound == "" || !compounds[compound] {
			continue
		}
		score += 5
	}
	return score
}

func exactResolutionSameFamilySurfaceSets(surface string) (map[string]bool, map[string]bool) {
	tokens := ExactResolutionIdentifierTerms(surface)
	if len(tokens) == 0 {
		return nil, nil
	}
	termSet := make(map[string]bool)
	for _, token := range tokens {
		token = strings.TrimSpace(strings.ToLower(token))
		if len(token) >= 3 {
			termSet[token] = true
		}
	}
	compoundSet := make(map[string]bool)
	for i := 0; i < len(tokens); i++ {
		for n := 2; n <= 3 && i+n <= len(tokens); n++ {
			compound := strings.Join(tokens[i:i+n], "")
			if len(compound) >= 8 {
				compoundSet[compound] = true
			}
		}
	}
	return termSet, compoundSet
}

type exactResolutionFamilySignals struct {
	roots     []string
	compounds []string
}

func exactResolutionSameFamilySignals(c *ExactResolutionContract) exactResolutionFamilySignals {
	if c == nil {
		return exactResolutionFamilySignals{}
	}
	isConfigKey := c.TargetKind == SubjectConfigKey || strings.EqualFold(strings.TrimSpace(c.TargetLabel), "config key")
	if !isConfigKey {
		return exactResolutionFamilySignals{
			roots: ExactResolutionContextTerms(c),
		}
	}
	roots := exactResolutionConfigRootTerms(c.Targets)
	compoundSeen := make(map[string]bool)
	var compounds []string
	for _, target := range c.Targets {
		segments := exactResolutionConfigSegments(target)
		if len(segments) < 2 {
			continue
		}
		for n := 2; n <= 3 && n <= len(segments); n++ {
			compound := strings.Join(segments[:n], "")
			if len(compound) < 8 || compoundSeen[compound] {
				continue
			}
			compoundSeen[compound] = true
			compounds = append(compounds, compound)
		}
	}
	sort.Strings(compounds)
	return exactResolutionFamilySignals{
		roots:     roots,
		compounds: compounds,
	}
}

func exactResolutionSameFamilyRootWeight(root string) int {
	switch {
	case len(root) >= 7:
		return 5
	case len(root) >= 5:
		return 3
	default:
		return 2
	}
}

func looksLikeExactPathToken(s string) bool {
	s = strings.TrimSpace(strings.ReplaceAll(s, `\`, `/`))
	if s == "" {
		return false
	}
	return strings.Contains(s, "/") || (filepath.Ext(s) != "" && !strings.Contains(s, " "))
}

func looksLikeRouteLikeToken(s string) bool {
	s = strings.TrimSpace(strings.ReplaceAll(s, `\`, `/`))
	if s == "" {
		return false
	}
	if strings.HasPrefix(s, "/") {
		return true
	}
	parts := strings.Fields(strings.ToUpper(s))
	if len(parts) >= 2 {
		switch parts[0] {
		case "GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD":
			return strings.HasPrefix(parts[1], "/")
		}
	}
	return false
}

func looksLikeConfigKeyToken(s string) bool {
	s = strings.TrimSpace(strings.Trim(s, "`\"' "))
	if s == "" || strings.ContainsAny(s, " \t\r\n") {
		return false
	}
	if strings.Contains(s, "/") || filepath.Ext(strings.ReplaceAll(s, `\`, `/`)) != "" {
		return false
	}
	if strings.ContainsAny(s, "._-") {
		return true
	}
	first := s[0]
	return first >= 'a' && first <= 'z'
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

// ExactResolutionLookupKey exposes the stable exact-target normalization
// used by matching / pending-target checks so other stages can reuse the
// same token equivalence without re-implementing it.
func ExactResolutionLookupKey(kind, s string) string {
	return normalizeExactResolutionToken(kind, s)
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
