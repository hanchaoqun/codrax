package types

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// BuildExactResolutionContract derives the AnswerContract-level
// contract for exact-target questions. It is intentionally
// conservative: broad enumeration questions return nil so downstream
// target-mention / absence constraints do not over-trigger.
func BuildExactResolutionContract(rm RequestModel) *ExactResolutionContract {
	if HasCapabilitySurfaceHint(rm) {
		return nil
	}
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
		RequestedContextRoles: exactResolutionValidatedContextRoles(
			rm.AnswerSubject.Kind,
			rm.Scenario,
			policy,
			rm.AnalyzerHints.ExactContextRoles,
		),
	}
	if hint := exactResolutionScopeHintForPolicy(policy); hint != "" {
		contract.RelatedContextScopeHint = hint
	}
	return contract
}

func exactResolutionValidatedContextRoles(
	subjectKind AnswerSubjectKind,
	scenario Scenario,
	policy ExactResolutionContextPolicy,
	in []EvidenceDiagramRole,
) []EvidenceDiagramRole {
	if subjectKind != SubjectConfigKey || scenario != ScenarioConfigTrace || policy != ExactContextSameFamilyGrounded {
		return nil
	}
	var filtered []EvidenceDiagramRole
	for _, role := range NormalizeEvidenceDiagramRoles(in) {
		switch role {
		case EvidenceDiagramRoleDefault,
			EvidenceDiagramRoleConfig,
			EvidenceDiagramRoleRuntime,
			EvidenceDiagramRoleOverride:
			filtered = append(filtered, role)
		}
	}
	return filtered
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
		if exactResolutionCandidateMatchesRequest(rm, candidate) {
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
			if exactResolutionCandidateMatchesRequest(rm, candidate) {
				out = append(out, candidate)
			}
		}
		return out
	}
	kind := exactResolutionFindingKindForRM(rm)
	allowed := make(map[string]bool, len(primary))
	for _, item := range primary {
		if !exactResolutionCandidateMatchesRequest(rm, item) {
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
		if !exactResolutionCandidateMatchesRequest(rm, candidate) {
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
		return looksLikeConfigKeyToken(candidate) || looksLikeBareConfigWord(candidate)
	case SubjectStringLiteral, SubjectNumeric, SubjectUnknown:
		return true
	default:
		return !looksLikeExactPathToken(candidate) && !looksLikeRouteLikeToken(candidate)
	}
}

func exactResolutionCandidateMatchesRequest(rm RequestModel, candidate string) bool {
	switch exactResolutionSubjectLabel(rm) {
	case "config key":
		return exactResolutionCandidateMatchesSubjectKind(SubjectConfigKey, candidate)
	case "file path":
		return exactResolutionCandidateMatchesSubjectKind(SubjectFilePath, candidate)
	case "route":
		return exactResolutionCandidateMatchesSubjectKind(SubjectHandlerRoute, candidate)
	case "symbol":
		candidate = strings.TrimSpace(strings.Trim(candidate, "`\"' "))
		return candidate != "" &&
			!looksLikeExactPathToken(candidate) &&
			!looksLikeRouteLikeToken(candidate) &&
			!looksLikeConfigKeyToken(candidate)
	default:
		return exactResolutionCandidateMatchesSubjectKind(rm.AnswerSubject.Kind, candidate)
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

// ExactResolutionDirectAnchorMatchesAnyTarget reports whether the
// structural anchor identity itself explicitly names one of the exact
// targets. We intentionally restrict this to subject / anchor_symbol:
// object often carries explanatory payload ("X does not define Y",
// "defaults -> YAML -> runtime"), which is useful context but not a
// defining anchor for Y on its own.
func ExactResolutionDirectAnchorMatchesAnyTarget(c *ExactResolutionContract, subject, anchorSymbol, object string) bool {
	return ExactResolutionTextsMentionAnyTarget(c, subject) ||
		ExactResolutionTextsMentionAnyTarget(c, anchorSymbol)
}

// ExactResolutionProofAnchorMatchesAnyTarget is the stricter sibling
// used by proof/absence validators. The model often keeps the user
// requested exact target in Subject while AnchorSymbol names the real
// nearby symbol it actually read. That pattern is useful for related
// context, but it is NOT proof that the nearby symbol defines the exact
// target. Proof-grade matching therefore prefers AnchorSymbol, and only
// falls back to Subject when no explicit anchor symbol was supplied.
func ExactResolutionProofAnchorMatchesAnyTarget(c *ExactResolutionContract, subject, anchorSymbol, object string) bool {
	if ExactResolutionTextsMentionAnyTarget(c, anchorSymbol) {
		return true
	}
	if !ExactResolutionTextsMentionAnyTarget(c, subject) {
		return false
	}
	return strings.TrimSpace(anchorSymbol) == ""
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

// ExactResolutionFocusTerms returns a richer, user-visible follow-up
// term set for exploration prompts. It keeps the validated
// same-scope roots from ExactResolutionContextTerms, then augments
// config-key exact targets with a small number of more specific
// compounds / segments so the LLM does not over-focus on a single
// broad family root (for example `explore`) when choosing the next
// file to inspect.
//
// This helper is intentionally prompt-facing only: it does not change
// the core same-family contract or the structural validators.
func ExactResolutionFocusTerms(c *ExactResolutionContract) []string {
	base := ExactResolutionContextTerms(c)
	if c == nil || (c.TargetKind != SubjectConfigKey && !strings.EqualFold(strings.TrimSpace(c.TargetLabel), "config key")) {
		return base
	}
	var out []string
	seen := make(map[string]bool)
	add := func(items ...string) {
		for _, item := range items {
			item = strings.TrimSpace(strings.ToLower(item))
			if len(item) < 3 || seen[item] {
				continue
			}
			seen[item] = true
			out = append(out, item)
		}
	}
	add(base...)
	for _, target := range c.Targets {
		segments := exactResolutionConfigSegments(target)
		if len(segments) == 0 {
			continue
		}
		for i := 1; i < len(segments); i++ {
			if len(segments[i]) >= 4 {
				add(segments[i])
			}
		}
		for i := 1; i+1 < len(segments); i++ {
			compound := strings.Join(segments[i:i+2], "")
			if len(compound) >= 7 {
				add(compound)
			}
		}
	}
	if len(out) > 6 {
		out = out[:6]
	}
	return out
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
	if HasCapabilitySurfaceHint(rm) {
		return false
	}
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

// ExactResolutionHasDefiningTargetProof reports whether the current
// structured evidence pool already contains grounded production-like
// proof that directly anchors one of the exact targets. Callers use
// this to keep the target "pending" until the system sees either a
// real defining proof or a stable exact-absence closure.
func ExactResolutionHasDefiningTargetProof(c *ExactResolutionContract, items []EvidenceItem) bool {
	if c == nil || len(items) == 0 {
		return false
	}
	for _, item := range items {
		switch item.GroundingStatus {
		case GroundingGrounded, GroundingRecovered:
		default:
			continue
		}
		switch item.ContextRole {
		case EvidenceContextRoleIllustrativeOnly, EvidenceContextRoleAbsenceSupport:
			continue
		}
		if !ExactResolutionSourceIsDefiningPrimaryProofLike(c, item.Source) {
			continue
		}
		if exactResolutionEvidenceHasProofGradeAnchor(item) &&
			ExactResolutionDirectAnchorMatchesAnyTarget(c, item.Subject, item.AnchorSymbol, item.Object) {
			return true
		}
		if item.ContextRole == EvidenceContextRoleDefining &&
			exactResolutionEvidenceHasProofGradeAnchor(item) &&
			ExactResolutionTextsMentionAnyTarget(c,
				item.Subject, item.Predicate, item.Object, item.AnchorSymbol, item.Condition, item.Snippet, item.Summary) {
			return true
		}
	}
	return false
}

// ExactResolutionEvidenceCanSatisfyRelatedContext reports whether a
// grounded evidence item is a production-like same-scope anchor that
// can legitimately support the "related context" half of an exact-
// absence answer. The LLM may recommend related-context material, but
// the system decides structurally whether an emitted evidence item is
// strong enough to justify that contextual guidance before closing.
//
// requiredFiles is optional. When non-empty, the evidence must come
// from one of those repo-relative files; otherwise the contract's own
// policy (same-family / same-directory / grounded-only) is used as the
// structural scope check.
func ExactResolutionEvidenceCanSatisfyRelatedContext(c *ExactResolutionContract, item EvidenceItem, requiredFiles []string) bool {
	return exactResolutionEvidenceCanSatisfyRelatedContextWithStatuses(c, item, requiredFiles, false)
}

func exactResolutionEvidenceCanSatisfyRelatedContextWithStatuses(c *ExactResolutionContract, item EvidenceItem, requiredFiles []string, allowRecovered bool) bool {
	if c == nil {
		return false
	}
	switch item.GroundingStatus {
	case GroundingGrounded:
	case GroundingRecovered:
		if !allowRecovered {
			return false
		}
	default:
		return false
	}
	if !exactResolutionSourceAllowedForRelatedContext(c, item) {
		return false
	}
	switch item.ContextRole {
	case EvidenceContextRoleIllustrativeOnly, EvidenceContextRoleAbsenceSupport:
		return false
	}
	if ExactResolutionDirectAnchorMatchesAnyTarget(c, item.Subject, item.AnchorSymbol, item.Object) {
		return false
	}
	if item.ContextRole == EvidenceContextRoleDefining &&
		ExactResolutionTextsMentionAnyTarget(c,
			item.Subject, item.Predicate, item.Object, item.AnchorSymbol, item.Condition, item.Snippet, item.Summary) {
		return false
	}
	if len(requiredFiles) > 0 && !exactResolutionRequiredFilesContain(requiredFiles, item.Source) {
		return false
	}
	switch c.RelatedContextPolicy {
	case ExactContextGroundedOnly:
		return true
	case ExactContextSameFamilyGrounded:
		if ExactResolutionSameFamilyMatchScore(c, strings.Join([]string{
			item.Source,
			item.Subject,
			item.Object,
			item.AnchorSymbol,
			item.Condition,
			item.Snippet,
		}, "\n")) > 0 {
			return true
		}
		return EvidenceItemStructurallyMentionsAnyTerm(item, ExactResolutionContextTerms(c))
	case ExactContextSameDirectoryGrounded:
		if len(requiredFiles) > 0 {
			return true
		}
		return EvidenceItemStructurallyMentionsAnyTerm(item, ExactResolutionContextTerms(c))
	default:
		return false
	}
}

func exactResolutionFindingKindMatches(expected, got string) bool {
	got = strings.TrimSpace(strings.ToLower(got))
	if got == "" {
		got = "symbol"
	}
	return got == expected
}

// ConfigTraceGroundedContextAnchorAllowed reports whether an evidence
// item can serve as a grounded nearby-context anchor for an exact-
// absence config-trace answer. The rule is structural:
//
//   - primary absence-proof sources are allowed
//   - grounded nearby context must already carry a validated
//     precedence role
//   - illustrative / unresolved / truncated evidence is never
//     answer-grade context
//
// This helper is shared by the finalizer prompt and the
// emit_answer_document validator so both stages consume the same
// contract.
func ConfigTraceGroundedContextAnchorAllowed(contract *ExactResolutionContract, item EvidenceItem) bool {
	return ConfigTraceGroundedContextAnchorAllowedInFiles(contract, item, nil)
}

func ConfigTraceGroundedContextAnchorAllowedInFiles(contract *ExactResolutionContract, item EvidenceItem, requiredFiles []string) bool {
	if item.Source == "" {
		return false
	}
	switch item.GroundingStatus {
	case GroundingGrounded, GroundingRecovered:
	default:
		return false
	}
	if item.Kind == EvidenceUnresolved || item.Kind == EvidenceTruncated {
		return false
	}
	switch item.ContextRole {
	case EvidenceContextRoleIllustrativeOnly:
		return false
	case EvidenceContextRoleAbsenceSupport:
		return contract != nil && ExactResolutionSourceIsDefiningPrimaryProofLike(contract, item.Source)
	default:
		return ConfigTraceValidatedDiagramRoleInFiles(contract, item, requiredFiles) != EvidenceDiagramRoleUnknown
	}
}

func ConfigTraceValidatedDiagramRole(contract *ExactResolutionContract, item EvidenceItem) EvidenceDiagramRole {
	return ConfigTraceValidatedDiagramRoleInFiles(contract, item, nil)
}

func ConfigTraceValidatedDiagramRoleInFiles(contract *ExactResolutionContract, item EvidenceItem, requiredFiles []string) EvidenceDiagramRole {
	if item.Source == "" {
		return EvidenceDiagramRoleUnknown
	}
	switch item.GroundingStatus {
	case GroundingGrounded, GroundingRecovered:
	default:
		return EvidenceDiagramRoleUnknown
	}
	if item.Kind == EvidenceUnresolved || item.Kind == EvidenceTruncated {
		return EvidenceDiagramRoleUnknown
	}
	switch item.ContextRole {
	case EvidenceContextRoleAbsenceSupport:
		if !configTraceAbsenceSupportCanCarryDiagramRole(item) {
			return EvidenceDiagramRoleUnknown
		}
	case EvidenceContextRoleIllustrativeOnly:
		if item.DiagramRole != EvidenceDiagramRoleConfig || !LooksLikeConfigFilePath(item.Source) {
			return EvidenceDiagramRoleUnknown
		}
	}
	switch item.DiagramRole {
	case EvidenceDiagramRoleConfig:
		if ConfigTraceDiagramRoleAnchorCompatible(item.DiagramRole, item) &&
			LooksLikeConfigFilePath(item.Source) &&
			configTraceDiagramEvidenceWithinScope(contract, item, requiredFiles) {
			return item.DiagramRole
		}
	case EvidenceDiagramRoleDefault, EvidenceDiagramRoleRuntime, EvidenceDiagramRoleOverride:
		if ConfigTraceDiagramRoleAnchorCompatible(item.DiagramRole, item) &&
			configTraceDiagramCodeLayerAllowed(contract, item, requiredFiles) {
			return item.DiagramRole
		}
	}
	return EvidenceDiagramRoleUnknown
}

func configTraceAbsenceSupportCanCarryDiagramRole(item EvidenceItem) bool {
	return item.ContextRole == EvidenceContextRoleAbsenceSupport &&
		item.DiagramRole == EvidenceDiagramRoleConfig &&
		LooksLikeConfigFilePath(item.Source) &&
		IsNegativeEvidencePredicate(item.Predicate)
}

// ConfigTraceDiagramRoleAnchorCompatible applies role-specific
// structural validation to LLM-recommended precedence roles.
//
// The model is encouraged to recommend the semantic layer
// (`default`/`config`/`runtime`/`override`), but the system still
// validates whether the anchored line has the right structural shape
// for that layer. This keeps repo-specific naming out of the
// validator: we do not rely on words like "default" or "flag", only
// on the anchored file kind and code-location kind.
func ConfigTraceDiagramRoleAnchorCompatible(role EvidenceDiagramRole, item EvidenceItem) bool {
	switch role {
	case EvidenceDiagramRoleConfig:
		return item.Source != "" && LooksLikeConfigFilePath(item.Source)
	case EvidenceDiagramRoleDefault:
		switch item.AnchorKind {
		case AnchorDefinition, AnchorAssignment, AnchorReturn:
			return true
		default:
			return false
		}
	case EvidenceDiagramRoleRuntime, EvidenceDiagramRoleOverride:
		switch item.AnchorKind {
		case AnchorAssignment, AnchorCall, AnchorCondition:
			return true
		default:
			return false
		}
	default:
		return false
	}
}

type ConfigTraceRelatedContextCitationCandidate struct {
	Source string
	Line   int
	Role   EvidenceDiagramRole
}

func ConfigTraceRelatedContextCitationCandidates(contract *ExactResolutionContract, requiredFiles []string, items []EvidenceItem) []ConfigTraceRelatedContextCitationCandidate {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	var out []ConfigTraceRelatedContextCitationCandidate
	for _, item := range items {
		role := ConfigTraceValidatedDiagramRoleInFiles(contract, item, requiredFiles)
		if role == EvidenceDiagramRoleUnknown || item.Source == "" || item.LineStart <= 0 {
			continue
		}
		key := exactResolutionCanonicalPath(item.Source) + ":" + strconv.Itoa(item.LineStart)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, ConfigTraceRelatedContextCitationCandidate{
			Source: item.Source,
			Line:   item.LineStart,
			Role:   role,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if configTraceCitationCandidateScore(contract, out[i].Role) != configTraceCitationCandidateScore(contract, out[j].Role) {
			return configTraceCitationCandidateScore(contract, out[i].Role) > configTraceCitationCandidateScore(contract, out[j].Role)
		}
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		return out[i].Line < out[j].Line
	})
	return out
}

func configTraceCitationCandidateScore(contract *ExactResolutionContract, role EvidenceDiagramRole) int {
	if requested := ConfigTraceRequestedDiagramRoles(contract); len(requested) > 0 {
		for i, requestedRole := range requested {
			if requestedRole == role {
				return 100 - i
			}
		}
	}
	switch role {
	case EvidenceDiagramRoleOverride:
		return 40
	case EvidenceDiagramRoleConfig:
		return 36
	case EvidenceDiagramRoleRuntime:
		return 32
	case EvidenceDiagramRoleDefault:
		return 28
	default:
		return 0
	}
}

func configTraceDiagramEvidenceWithinScope(contract *ExactResolutionContract, item EvidenceItem, requiredFiles []string) bool {
	if len(requiredFiles) > 0 && exactResolutionRequiredFilesContain(requiredFiles, item.Source) {
		return true
	}
	if item.DiagramRole == EvidenceDiagramRoleConfig && LooksLikeConfigFilePath(item.Source) {
		return true
	}
	if contract == nil {
		return len(requiredFiles) == 0
	}
	return configTraceDiagramEvidenceMatchesContext(contract, item)
}

func configTraceDiagramCodeLayerAllowed(contract *ExactResolutionContract, item EvidenceItem, requiredFiles []string) bool {
	if item.Source == "" || LooksLikeConfigFilePath(item.Source) || LooksLikeAuxiliaryEvidencePath(item.Source) {
		return false
	}
	if item.AnchorKind == "" {
		return false
	}
	return configTraceDiagramEvidenceWithinScope(contract, item, requiredFiles)
}

func LooksLikeConfigFilePath(source string) bool {
	source = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(source, `\`, `/`)))
	if source == "" {
		return false
	}
	base := filepath.Base(source)
	parts := strings.Split(base, ".")
	for _, part := range parts[1:] {
		if isConfigLikeExtension(part) {
			return true
		}
	}
	return false
}

func exactResolutionSourceAllowedForRelatedContext(c *ExactResolutionContract, item EvidenceItem) bool {
	if ExactResolutionSourceIsDefiningPrimaryProofLike(c, item.Source) {
		return true
	}
	return c != nil &&
		c.TargetKind == SubjectConfigKey &&
		LooksLikeConfigFilePath(item.Source) &&
		(item.DiagramRole == EvidenceDiagramRoleConfig || item.RequestedDiagramRole == EvidenceDiagramRoleConfig)
}

func LooksLikeWrappedConfigFilePath(source string) bool {
	source = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(source, `\`, `/`)))
	if source == "" {
		return false
	}
	base := filepath.Base(source)
	parts := strings.Split(base, ".")
	if len(parts) < 3 {
		return false
	}
	last := parts[len(parts)-1]
	if isConfigLikeExtension(last) {
		return false
	}
	for _, part := range parts[1 : len(parts)-1] {
		if isConfigLikeExtension(part) {
			return true
		}
	}
	return false
}

func isConfigLikeExtension(part string) bool {
	switch strings.ToLower(strings.TrimSpace(part)) {
	case "yaml", "yml", "json", "json5", "jsonc", "toml", "ini", "cfg", "conf", "config", "properties", "xml", "hcl", "env":
		return true
	}
	return false
}

func configTraceDiagramEvidenceMatchesContext(contract *ExactResolutionContract, item EvidenceItem) bool {
	if contract == nil {
		return true
	}
	if configTraceAbsenceSupportCanCarryDiagramRole(item) {
		return true
	}
	if ExactResolutionDirectAnchorMatchesAnyTarget(contract, item.Subject, item.AnchorSymbol, item.Object) {
		return false
	}
	surface := strings.Join([]string{
		item.Source,
		item.Subject,
		item.Object,
		item.AnchorSymbol,
		item.Condition,
		item.Snippet,
	}, "\n")
	if ExactResolutionSameFamilyMatchScore(contract, surface) > 0 {
		return true
	}
	return EvidenceItemStructurallyMentionsAnyTerm(item, ExactResolutionContextTerms(contract))
}

// ExactResolutionAnswerContextAnchorAllowed reports whether an
// evidence item is strong enough to appear on the user-visible
// "nearby grounded context" side of an exact-resolution answer.
// The LLM may recommend surrounding context, but the system decides
// which items are answer-grade context before the finalizer is allowed
// to surface them.
func ExactResolutionAnswerContextAnchorAllowed(c *ExactResolutionContract, scenario Scenario, stableAbsent bool, item EvidenceItem) bool {
	return ExactResolutionAnswerContextAnchorAllowedInFiles(c, scenario, stableAbsent, item, nil)
}

func ExactResolutionAnswerContextAnchorAllowedInFiles(c *ExactResolutionContract, scenario Scenario, stableAbsent bool, item EvidenceItem, requiredFiles []string) bool {
	if c == nil {
		return false
	}
	if stableAbsent && scenario == ScenarioConfigTrace && c.TargetKind == SubjectConfigKey {
		if item.ContextRole == EvidenceContextRoleAbsenceSupport {
			switch item.GroundingStatus {
			case GroundingGrounded, GroundingRecovered:
				return ExactResolutionSourceIsDefiningPrimaryProofLike(c, item.Source)
			default:
				return false
			}
		}
		if ConfigTraceGroundedContextAnchorAllowedInFiles(c, item, nil) {
			return true
		}
		return ExactResolutionEvidenceCanSatisfyRelatedContext(c, item, requiredFiles)
	}
	if item.ContextRole == EvidenceContextRoleAbsenceSupport {
		switch item.GroundingStatus {
		case GroundingGrounded, GroundingRecovered:
			return true
		default:
			return false
		}
	}
	return ExactResolutionEvidenceCanSatisfyRelatedContext(c, item, nil)
}

// ExactResolutionCitationContextAnchorAllowedInFiles reports whether an
// evidence item is strong enough to appear in citations[] or fenced
// diagrams when the answer keeps nearby grounded context on the user-
// visible surface.
//
// The general rule is:
//   - exact-proof / absence-proof anchors may always stay citation-grade
//     when grounded
//   - scenario-specific stricter lanes (for example config-trace exact
//     absence) may further require a validated structural role before a
//     related-context anchor can be cited
//   - everything else can still be surface-allowed as prose-only nearby
//     context, but should not be surfaced as a citation-grade anchor
func ExactResolutionCitationContextAnchorAllowedInFiles(c *ExactResolutionContract, scenario Scenario, stableAbsent bool, item EvidenceItem, requiredFiles []string) bool {
	if c == nil {
		return false
	}
	if stableAbsent && scenario == ScenarioConfigTrace && c.TargetKind == SubjectConfigKey {
		if item.ContextRole == EvidenceContextRoleAbsenceSupport {
			switch item.GroundingStatus {
			case GroundingGrounded, GroundingRecovered:
				return ExactResolutionSourceIsDefiningPrimaryProofLike(c, item.Source)
			default:
				return false
			}
		}
		return ConfigTraceGroundedContextAnchorAllowedInFiles(c, item, requiredFiles)
	}
	return ExactResolutionAnswerContextAnchorAllowedInFiles(c, scenario, stableAbsent, item, requiredFiles)
}

// ExactResolutionRelatedContextProofAllowedInFiles is the stricter
// closure-time gate used before explorer / emit_investigation_complete
// are allowed to end an exact-absence investigation. It intentionally
// excludes pure absence-proof items from counting as "related context
// already gathered" in scenarios such as config-trace, where the
// system still requires one grounded nearby lineage anchor.
func ExactResolutionRelatedContextProofAllowedInFiles(c *ExactResolutionContract, scenario Scenario, stableAbsent bool, item EvidenceItem, requiredFiles []string) bool {
	if c == nil {
		return false
	}
	if item.GroundingStatus != GroundingGrounded {
		return false
	}
	if stableAbsent && scenario == ScenarioConfigTrace && c.TargetKind == SubjectConfigKey {
		if item.ContextRole == EvidenceContextRoleAbsenceSupport {
			return false
		}
		return ConfigTraceGroundedContextAnchorAllowedInFiles(c, item, requiredFiles)
	}
	return ExactResolutionAnswerContextAnchorAllowedInFiles(c, scenario, stableAbsent, item, requiredFiles)
}

// ExactResolutionEvidenceBlocksAbsence reports whether one structured
// evidence item disproves an exact-absence closure for the current
// target lane. Illustrative and absence-support items are ignored;
// grounded/recovered items that explicitly mention the exact target on
// a non-contextual lane block absence.
func ExactResolutionEvidenceBlocksAbsence(c *ExactResolutionContract, item EvidenceItem) bool {
	switch item.GroundingStatus {
	case GroundingGrounded, GroundingRecovered, "":
	default:
		return false
	}
	switch item.ContextRole {
	case EvidenceContextRoleIllustrativeOnly, EvidenceContextRoleAbsenceSupport:
		return false
	}
	if !ExactResolutionTextsMentionAnyTarget(c,
		item.Subject, item.Predicate, item.Object, item.AnchorSymbol, item.Condition, item.Snippet, item.Summary) {
		return false
	}
	if !exactResolutionEvidenceHasProofGradeAnchor(item) {
		return false
	}
	if IsNegativeEvidencePredicate(item.Predicate) {
		return false
	}
	if item.ContextRole == EvidenceContextRoleRelatedContext {
		return false
	}
	return true
}

// exactResolutionEvidenceHasProofGradeAnchor reports whether an
// evidence item carries a structural anchor strong enough to justify
// exact-match / alias-match / absence-contradiction decisions on its
// own. This intentionally excludes bare surface mentions that only
// survive because a deterministic parser extracted a string literal or
// prose fragment into Subject/Object without an explicit anchor kind.
//
// Citations stay as an independent proof lane in emit_answer_document:
// a cited line that explicitly names the target can still prove
// exact-match even when the matching EvidenceItem is only contextual.
// The restriction here applies only to raw evidence-pool items.
func exactResolutionEvidenceHasProofGradeAnchor(item EvidenceItem) bool {
	return strings.TrimSpace(string(item.AnchorKind)) != ""
}

// ExactResolutionEvidenceSupportsAbsence reports whether one grounded
// evidence item explicitly carries exact-target absence support rather
// than merely being nearby related context.
func ExactResolutionEvidenceSupportsAbsence(c *ExactResolutionContract, item EvidenceItem) bool {
	switch item.GroundingStatus {
	case GroundingGrounded, GroundingRecovered:
	default:
		return false
	}
	if item.ContextRole != EvidenceContextRoleAbsenceSupport {
		return false
	}
	if !ExactResolutionTextsMentionAnyTarget(c,
		item.Subject, item.Predicate, item.Object, item.AnchorSymbol, item.Condition, item.Snippet, item.Summary) {
		return false
	}
	return true
}

// ExactResolutionAbsenceClosureReady reports whether the current
// grounded evidence pool already supports closing an exact-target
// investigation as absence while keeping any remaining same-scope
// anchors as related context only.
func ExactResolutionAbsenceClosureReady(c *ExactResolutionContract, scenario Scenario, targets []string, evidence []EvidenceItem, requiredFiles []string) bool {
	if c == nil || len(targets) == 0 || len(evidence) == 0 {
		return false
	}
	scopeTerms := ExactResolutionContextTerms(c)
	hasScopedContext := len(scopeTerms) == 0
	hasAllowedRelatedContext := false
	hasAbsenceSupport := false
	for _, item := range evidence {
		if ExactResolutionEvidenceBlocksAbsence(c, item) {
			return false
		}
		if ExactResolutionEvidenceSupportsAbsence(c, item) {
			hasAbsenceSupport = true
		}
	}
	for _, item := range evidence {
		if ExactResolutionRelatedContextProofAllowedInFiles(c, scenario, true, item, requiredFiles) {
			hasScopedContext = true
			hasAllowedRelatedContext = true
			break
		}
		if len(requiredFiles) == 0 {
			switch item.GroundingStatus {
			case GroundingGrounded, GroundingRecovered, "":
			default:
				continue
			}
			if EvidenceItemStructurallyMentionsAnyTerm(item, scopeTerms) {
				hasScopedContext = true
				hasAllowedRelatedContext = true
				break
			}
		}
	}
	if hasAbsenceSupport && hasScopedContext {
		if scenario == ScenarioConfigTrace && c.TargetKind == SubjectConfigKey &&
			!ConfigTraceRequiredRoleCoverageSatisfied(c, requiredFiles, evidence) {
			return false
		}
		return true
	}
	if !hasAllowedRelatedContext {
		return false
	}
	if scenario == ScenarioConfigTrace && c.TargetKind == SubjectConfigKey &&
		!ConfigTraceRequiredRoleCoverageSatisfied(c, requiredFiles, evidence) {
		return false
	}
	return !ExactResolutionHasDefiningTargetProof(c, evidence)
}

func ConfigTraceNeedsMultiLayerClosure(requiredFiles []string) bool {
	if len(requiredFiles) < 2 {
		return false
	}
	seen := make(map[string]bool, len(requiredFiles))
	for _, file := range requiredFiles {
		file = exactResolutionCanonicalPath(file)
		if file == "" || seen[file] {
			continue
		}
		seen[file] = true
		if len(seen) >= 2 {
			return true
		}
	}
	return false
}

// ExactResolutionAutoAbsenceJustification synthesizes a deterministic
// absence justification for a validated exact-target absence closure.
func ExactResolutionAutoAbsenceJustification(c *ExactResolutionContract) string {
	if c == nil || len(c.Targets) == 0 {
		return ""
	}
	label := strings.TrimSpace(c.TargetLabel)
	if label == "" {
		label = "target"
	}
	return fmt.Sprintf(
		"grounded investigation found no defining repo anchor for exact %s %s; any remaining grounded same-scope items are related context only",
		label,
		exactResolutionBacktickJoin(c.Targets),
	)
}

func exactResolutionBacktickJoin(items []string) string {
	if len(items) == 0 {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		parts = append(parts, "`"+item+"`")
	}
	return strings.Join(parts, ", ")
}

func exactResolutionRequiredFilesContain(requiredFiles []string, source string) bool {
	source = exactResolutionCanonicalPath(source)
	if source == "" || len(requiredFiles) == 0 {
		return false
	}
	for _, file := range requiredFiles {
		if exactResolutionCanonicalPath(file) == source {
			return true
		}
	}
	return false
}

// ExactResolutionContextSurfaceRelevant reports whether an evidence
// item belongs to the exact target's semantic neighborhood strongly
// enough that surfacing it in the final answer should be an explicit
// decision. This lets prompt-shaping and answer validators treat
// nearby same-family/doc-example anchors as first-class candidates
// rather than relying on free-form prose heuristics.
func ExactResolutionContextSurfaceRelevant(c *ExactResolutionContract, item EvidenceItem) bool {
	if c == nil || item.Source == "" {
		return false
	}
	switch item.GroundingStatus {
	case GroundingGrounded, GroundingRecovered:
	default:
		return false
	}
	if ExactResolutionTextsMentionAnyTarget(c,
		item.Subject, item.Predicate, item.Object, item.AnchorSymbol, item.Condition, item.Snippet, item.Summary) {
		return true
	}
	if ExactResolutionSameFamilyMatchScore(c, exactResolutionEvidenceSurface(item)) > 0 {
		return true
	}
	return EvidenceItemMentionsAnyTerm(item, ExactResolutionContextTerms(c))
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

func exactResolutionEvidenceSurface(item EvidenceItem) string {
	return strings.Join([]string{
		item.Source,
		item.Subject,
		item.Predicate,
		item.Object,
		item.AnchorSymbol,
		item.Condition,
		item.Snippet,
		item.Summary,
	}, "\n")
}

func exactResolutionCanonicalPath(path string) string {
	path = strings.TrimSpace(strings.ReplaceAll(path, `\`, `/`))
	path = strings.TrimPrefix(path, "./")
	if path == "." {
		return ""
	}
	return filepath.ToSlash(path)
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
	return strings.ContainsAny(s, "._-")
}

func looksLikeBareConfigWord(s string) bool {
	s = strings.TrimSpace(strings.Trim(s, "`\"' "))
	if s == "" || strings.ContainsAny(s, " \t\r\n") {
		return false
	}
	if strings.ContainsAny(s, "/._-") || filepath.Ext(strings.ReplaceAll(s, `\`, `/`)) != "" {
		return false
	}
	hasLower := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			hasLower = true
		case r >= '0' && r <= '9':
			continue
		default:
			return false
		}
	}
	return hasLower
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
