package types

import (
	"strings"
	"unicode"
)

// HasNonEmptyAmbiguity reports whether the request carries at least one
// analyzer-emitted ambiguity clause with real content. Shared by
// scenario reconcile and compiler template selection so both stages do
// not drift on what counts as "this question still needs branch
// collapse/reconciliation".
func HasNonEmptyAmbiguity(rm RequestModel) bool {
	for _, a := range rm.Ambiguities {
		if strings.TrimSpace(a.Clause) != "" {
			return true
		}
	}
	return false
}

// HasAttributeBearingEnumeration reports whether the request asks for
// an exhaustive / bounded set of principal members AND also asks for a
// related per-member attribute. Example shape: "list all X and, for
// each X, name its Y". The completeness axes must stay separate:
// membership completeness is about X, while attribute completeness is
// about whether every X has a grounded Y.
//
// The signal is intentionally typed-only. It consumes analyzer
// predicates, PredicateAxis, AnalyzerHints entity cardinality, and
// QuestionStructure fields; it never scans raw request text, so
// downstream prompts and validators do not learn new keyword tables.
// A relational lookup is directly two-axis. A non-relational category
// enumeration becomes two-axis-risky when the analyzer emitted an
// exhaustive / bounded multi-member set plus any typed sign that the
// answer has structure beyond "just the names" (predicate axis or
// sub-topic split). That shape means "member set" and "member facet"
// must not share one completeness bit.
func HasAttributeBearingEnumeration(rm RequestModel) bool {
	if !rm.Predicates.IsCategoryEnumeration {
		return false
	}
	if rm.Predicates.IsRelationalLookup {
		return true
	}
	if !hasExhaustiveMultiMemberSet(rm) {
		return false
	}
	return rm.PredicateAxis != AxisUnknown || len(rm.SubTopics) > 0
}

// HasBoundedCategoryEnumerationMembers reports whether the analyzer
// produced a typed, multi-member category-enumeration lane that is
// bounded by a declared count, an active completeness obligation, or
// multiple required file hints. Downstream hard gates use this only to
// avoid over-interpreting package/module/directory member names as
// missing symbols; it is not proof that every member is correct.
func HasBoundedCategoryEnumerationMembers(rm RequestModel) bool {
	if !rm.Predicates.IsCategoryEnumeration || rm.Predicates.IsRelationalLookup {
		return false
	}
	return hasExhaustiveMultiMemberSet(rm)
}

// HasPrincipalCategoryEnumerationMemberLane reports whether the analyzer has
// already emitted a typed principal member lane for a non-relation category
// enumeration.
//
// This is narrower than "the request is an enumeration" and broader than
// "the user stated an explicit all/count boundary": the analyzer contract says
// that for non-relational category enumerations, entities are the enumerated
// members themselves, while relational enumerations keep the relation target and
// helper surfaces in the same entity list and must wait for exploration. The
// helper therefore consumes only typed analyzer fields and a multi-entity
// cardinality check; it does not scan RawRequest for localized list/all words.
func HasPrincipalCategoryEnumerationMemberLane(rm RequestModel) bool {
	if !CanUseAnalyzerEntitiesAsHardPrincipalMembers(rm) {
		return false
	}
	if distinctNonEmptyStrings(rm.AnalyzerHints.Entities) <= 1 {
		return false
	}
	if rm.Intent == IntentEnumerate {
		return true
	}
	switch NormalizeRequirementKind(rm.AnalyzerHints.Kind) {
	case ReqEnumeration, ReqRegistration:
		return true
	default:
		return false
	}
}

// ShouldSurfaceTypedRelationHints reports whether downstream agents should see
// typed graph relation rows such as interface→implementer membership.
//
// This is intentionally broader than enumeration: the same precise relation
// fact can be needed by a list, count, comparison, architecture explanation, or
// diagram. The signal remains schema-only. It consumes analyzer predicates and
// PredicateAxis, never localized raw request text, so relation facts stay
// language-neutral across every repomap-supported language.
func ShouldSurfaceTypedRelationHints(rm RequestModel) bool {
	if rm.Predicates.IsCategoryEnumeration ||
		rm.Predicates.IsRelationalLookup ||
		rm.Predicates.IsCountQuestion {
		return true
	}
	return rm.PredicateAxis == AxisImplement
}

// HasTypedRelationMemberSetShape reports whether a principal member_set, when
// present, should be interpreted as relation membership rather than a generic
// source inventory.
//
// Source inventory repair can safely fill package/file symbol lists, but it
// must not rewrite a model-authored relation set such as "interface
// implementers" into "all types in the interface file". The signal is
// schema-only: PredicateAxis and relational predicates, never request prose or
// localized keywords. This keeps the rule language-neutral across all
// repomap-supported languages.
func HasTypedRelationMemberSetShape(rm RequestModel) bool {
	return rm.PredicateAxis == AxisImplement || rm.Predicates.IsRelationalLookup
}

// RequiresExhaustiveEnumerationMemberSetHandoff reports whether a
// set-valued enumeration answer must be carried downstream as a
// model-authored aggregate_facts.member_set before later stages are
// allowed to treat the exploration as complete.
//
// This is the shared typed boundary for closed principal member lists:
// the explorer may discover rich candidates through grep/read/repomap,
// but a complete answer set must be emitted through a structured
// handoff so extractor/finalizer do not reconstruct it from thinking,
// tool prose, or chain-ranking leftovers. The predicate is deliberately
// schema-only. It consumes analyzer intent, predicates, question
// structure, and requirement kind; it never scans raw request keywords,
// so the rule applies equally to Go, Python, JavaScript/TypeScript,
// Java/Kotlin, Rust, C/C++, Ruby, Swift, Lua, Proto, ArkTS, Cangjie,
// and mixed-language repositories.
func RequiresExhaustiveEnumerationMemberSetHandoff(rm RequestModel) bool {
	if !IsCategoryEnumerationAnswerShape(rm) {
		return false
	}
	if rm.Predicates.IsScalarAnswer ||
		rm.Predicates.IsRoleLocateLookup ||
		rm.Predicates.IsCountQuestion ||
		rm.Predicates.IsHistoryLookup ||
		rm.Predicates.IsDiagnosticQuestion {
		return false
	}
	if IsArchitectureNarrativeExplanation(rm) {
		return false
	}
	if rm.ChangeImpactProfile != nil && rm.ChangeImpactProfile.Active() {
		return false
	}
	if rm.CompletenessObligation.IsActive() {
		return true
	}
	if rm.EnumerationBoundary != nil && rm.EnumerationBoundary.DeclaredCount > 0 {
		return true
	}
	if rm.Predicates.IsRelationalLookup {
		return rm.Intent == IntentEnumerate && rm.Predicates.IsCategoryEnumeration
	}
	if !rm.Predicates.IsCategoryEnumeration {
		return false
	}
	if HasPrincipalCategoryEnumerationMemberLane(rm) {
		return true
	}
	if !hasExhaustiveMultiMemberSet(rm) {
		return false
	}
	switch NormalizeRequirementKind(rm.AnalyzerHints.Kind) {
	case ReqEnumeration, ReqRegistration:
		return rm.Intent == IntentEnumerate
	default:
		return false
	}
}

// RequiresRelationMemberSetHandoff reports whether a relation lookup needs a
// model-authored principal member_set before exploration may close.
//
// This is the relation-shaped counterpart to
// RequiresExhaustiveEnumerationMemberSetHandoff. It only consumes typed answer
// shape signals: relational lookup + set/count/enumerate shape. It intentionally
// skips scalar role-location relations ("which function handles X?") and pure
// architecture explanations ("how A talks to B?"), because those are answered
// by a resolved literal or mechanism narrative rather than a qualifying-member
// set.
func RequiresRelationMemberSetHandoff(rm RequestModel) bool {
	if !rm.Predicates.IsRelationalLookup {
		return false
	}
	if rm.Predicates.IsScalarAnswer || rm.Predicates.IsRoleLocateLookup {
		return false
	}
	if rm.ChangeImpactProfile != nil && rm.ChangeImpactProfile.Active() {
		return false
	}
	return rm.Intent == IntentEnumerate ||
		rm.Predicates.IsCategoryEnumeration ||
		rm.Predicates.IsCountQuestion
}

// CompletenessObligationIsMechanismCoverageOnly reports whether an active
// completeness obligation should be interpreted as "cover these mechanism
// facets in the explanation" rather than "emit a closed principal member set".
//
// The distinction is intentionally typed-only. Mechanism explanations often
// contain wording like "must explain A, B, and their relationship"; analyzer
// records that as CompletenessObligation because the quoted phrase is real, but
// downstream stages must not turn those participants into an enumeration table.
// True set boundaries still win through category/count/relation/bucket/count
// signals.
func CompletenessObligationIsMechanismCoverageOnly(rm RequestModel) bool {
	if !rm.CompletenessObligation.IsActive() {
		return false
	}
	if rm.Intent != IntentExplain {
		return false
	}
	if rm.EnumerationBoundary != nil && rm.EnumerationBoundary.DeclaredCount > 0 {
		return false
	}
	if len(rm.QuestionStructure().Buckets) >= 2 {
		return false
	}
	if rm.Predicates.IsScalarAnswer ||
		rm.Predicates.IsRelationalLookup ||
		rm.Predicates.IsCategoryEnumeration ||
		rm.Predicates.IsCountQuestion ||
		rm.Predicates.IsHistoryLookup ||
		rm.Predicates.IsDiagnosticQuestion {
		return false
	}
	switch rm.PredicateAxis {
	case AxisCondition, AxisCall, AxisRegister:
		return true
	}
	switch NormalizeRequirementKind(rm.AnalyzerHints.Kind) {
	case ReqMechanism, ReqConditional, ReqRegistration:
		return true
	default:
		return false
	}
}

// HasPrincipalAnswerSetObligation reports whether the request carries a typed
// obligation that should shape the final answer as a closed principal set.
// Coverage-only mechanism obligations remain advisory coverage requirements:
// they must be preserved for finalizer context, but they must not activate
// enumeration family routing, answer-symbol slates, or deterministic row
// compilers.
func HasPrincipalAnswerSetObligation(rm RequestModel) bool {
	view := rm.QuestionStructure()
	if view.EnumerationBoundary != nil && view.EnumerationBoundary.DeclaredCount > 0 {
		return true
	}
	if len(view.Buckets) >= 2 {
		return true
	}
	if view.CompletenessObligation.IsActive() {
		return !CompletenessObligationIsMechanismCoverageOnly(rm)
	}
	return false
}

// HistoryLookupPrefersVCSNarrativePrincipal reports whether repository history
// metadata should be the principal evidence lane and current source files should
// remain optional support.
//
// The rule is intentionally typed-only. It does not scan RawRequest for words
// like "diff", "current", or localized equivalents. Mixed history+current-code
// questions must surface through analyzer predicates / profiles / diagram or
// change-impact contracts; when they do, this helper returns false so VCS and
// current-source evidence can both stay principal.
func HistoryLookupPrefersVCSNarrativePrincipal(rm RequestModel, contract *AnswerContract) bool {
	if !rm.Predicates.IsHistoryLookup {
		return false
	}
	intentContract := CompileAnswerIntentContract(rm, contract)
	if intentContract.HasOrigin(AnswerEvidenceOriginCurrentSource) {
		return false
	}
	kind := NormalizeRequirementKind(rm.AnalyzerHints.Kind)
	if rm.Predicates.IsScalarAnswer ||
		rm.Predicates.IsCountQuestion ||
		rm.Predicates.IsRelationalLookup ||
		rm.Predicates.IsDiagnosticQuestion {
		return false
	}
	if rm.Predicates.IsCrossComponent && kind != ReqHistory {
		return false
	}
	if rm.DiagnosticProfile.RequiresDiagnosticRootCause() ||
		rm.DiagnosticProfile.RequiresCurrentStatusDiagnostic() {
		return false
	}
	if rm.ChangeImpactProfile != nil && rm.ChangeImpactProfile.Active() {
		return false
	}
	if rm.FieldValueProfile != nil && rm.FieldValueProfile.Active() {
		return false
	}
	if rm.Scenario == ScenarioArchitectureExplain && kind != ReqHistory {
		return false
	}
	if rm.DiagramHint != nil && rm.DiagramHint.Kind != "" {
		return false
	}
	if contract != nil && contract.Diagram != nil && contract.Diagram.Required {
		return false
	}
	return true
}

// HistoryLookupScalarTargetCount returns the typed number of independent
// history/search targets carried by the analyzer. It deliberately consumes only
// structured lanes; raw request prose is not used to infer answer shape.
func HistoryLookupScalarTargetCount(rm RequestModel) int {
	var targets []string
	switch {
	case len(rm.AnalyzerHints.MentionedEntities) > 0:
		targets = append(targets, rm.AnalyzerHints.MentionedEntities...)
	case len(rm.AnalyzerHints.PrimaryEntities) > 0:
		targets = append(targets, rm.AnalyzerHints.PrimaryEntities...)
	default:
		targets = append(targets, rm.AnalyzerHints.Entities...)
	}
	for _, topic := range rm.SubTopics {
		targets = append(targets, topic.Entities...)
	}
	for _, bucket := range rm.QuestionStructure().Buckets {
		targets = append(targets, bucket.Label)
	}
	seen := map[string]bool{}
	for _, target := range targets {
		key := strings.ToLower(strings.TrimSpace(target))
		if key == "" {
			continue
		}
		seen[key] = true
	}
	return len(seen)
}

// IsHistoryBackedCurrentCodeExplanation reports whether the request combines a
// history/diff lookup with a current-code mechanism explanation. In this shape
// VCS facts are provenance/support for "what changed", while current source
// evidence explains "how it works now"; neither axis is a principal
// enumeration by itself.
//
// This intentionally differs from HistoryLookupPrefersVCSNarrativePrincipal:
// pure history narratives may skip current-source preselection, but mixed
// history+code explanations must keep current-source evidence active. The
// helper is schema-only: it consumes analyzer intent/predicates/scenario/kind
// and typed bucket partitions, never raw prompt prose or model free text.
func IsHistoryBackedCurrentCodeExplanation(rm RequestModel) bool {
	if !rm.Predicates.IsHistoryLookup {
		return false
	}
	if rm.Intent != IntentExplain && rm.Intent != IntentTrace {
		return false
	}
	if rm.Predicates.IsScalarAnswer ||
		rm.Predicates.IsRoleLocateLookup ||
		rm.Predicates.IsCountQuestion ||
		rm.Predicates.IsRelationalLookup ||
		rm.Predicates.IsDiagnosticQuestion {
		return false
	}
	if rm.Intent == IntentEnumerate {
		return false
	}
	if rm.DiagnosticProfile.RequiresDiagnosticRootCause() ||
		rm.DiagnosticProfile.RequiresCurrentStatusDiagnostic() {
		return false
	}
	if rm.ChangeImpactProfile != nil && rm.ChangeImpactProfile.Active() {
		return false
	}
	if rm.FieldValueProfile != nil && rm.FieldValueProfile.Active() {
		return false
	}
	if rm.DiagramHint != nil && rm.DiagramHint.Kind != "" {
		return false
	}
	if len(rm.QuestionStructure().Buckets) >= 2 {
		return false
	}
	kind := NormalizeRequirementKind(rm.AnalyzerHints.Kind)
	if kind == ReqHistory {
		return false
	}
	if rm.Intent == IntentTrace && (rm.PredicateAxis == AxisCall || kind == ReqCallChain) &&
		historyBackedTraceHasExplicitEndpoints(rm) {
		return false
	}
	if rm.Scenario == ScenarioArchitectureExplain {
		return true
	}
	switch kind {
	case ReqMechanism, ReqHistory:
		return true
	default:
		return false
	}
}

func historyBackedTraceHasExplicitEndpoints(rm RequestModel) bool {
	if len(rm.AnalyzerHints.ExactTargets) >= 2 {
		return true
	}
	mentioned := MentionedEntitiesFromRawRequest(rm.RawRequest, rm.AnalyzerHints.MentionedEntities)
	if len(mentioned) == 2 && len(mentioned) == len(rm.AnalyzerHints.MentionedEntities) {
		return true
	}
	return false
}

// IsCategoryEnumerationAnswerShape reports whether the user's answer
// shape is a set of principal members, as opposed to a single scalar
// role/literal that happens to be phrased with "which".
//
// The strongest signal is Predicates.IsCategoryEnumeration. Intent
// Enumerate is accepted as a fallback only when the scalar/role/count
// predicates do not contradict it. This keeps "which function handles
// X?" in the role-lookup lane when the analyzer marks it as a scalar
// role lookup, while preserving true set-valued questions even if the
// analyzer also emitted a function-like AnswerSubject.Kind.
func IsCategoryEnumerationAnswerShape(rm RequestModel) bool {
	if rm.Predicates.IsCategoryEnumeration {
		return true
	}
	if rm.Intent != IntentEnumerate {
		return false
	}
	if rm.Predicates.IsScalarAnswer ||
		rm.Predicates.IsRoleLocateLookup ||
		rm.Predicates.IsCountQuestion {
		return false
	}
	return true
}

// CanUseAnalyzerEntitiesAsHardPrincipalMembers reports whether
// AnalyzerHints.Entities may seed hard principal-member obligations
// such as AnswerContract.MustInclude.
//
// This is intentionally stricter than "entities are useful". Entities
// are always useful as soft search / exploration hints, but hard gates
// need a positive principal-member lane. Relation-shaped enumerations
// ("which callers of X", "which modules import Y", "which agents can
// invoke Z") are two-axis by definition: AnalyzerHints.Entities can
// contain the answer member candidates, the relation target, runtime
// helpers, tool names, and nearby anchors. Until the analyzer exposes a
// separate typed member field for that shape, those mixed entities must
// not become hard answer-member floors.
//
// Exhaustive, bounded, or bucketed enumerations are deliberately
// excluded: their answer members must come from exploration-time
// structured handoff (`aggregate_facts.member_set`) or grounded
// evidence, not from the analyzer's early entity shortlist. That
// shortlist can contain package names, scope anchors, helper concepts,
// and category labels even when it is useful for search.
//
// The signal is schema-only and language-neutral. It does not scan
// RawRequest text, so it applies uniformly to Go, C/C++, ArkTS,
// Cangjie, Java/Kotlin, JS/TS, Python, Rust, and path/module surfaces.
func CanUseAnalyzerEntitiesAsHardPrincipalMembers(rm RequestModel) bool {
	if IsArchitectureNarrativeExplanation(rm) {
		return false
	}
	if rm.QuestionStructure().HasAnyObligation() {
		return false
	}
	if !IsCategoryEnumerationAnswerShape(rm) {
		return false
	}
	if len(rm.AnalyzerHints.Entities) == 0 {
		return false
	}
	if rm.Predicates.IsRelationalLookup {
		return false
	}
	return true
}

// StructuralRelationScopeCandidates returns the narrow entity lane that may
// seed structural relation probes / hard divergence oracles.
//
// The important boundary is provenance, not spelling: broad
// AnalyzerHints.Entities can later be widened with repo-map candidates,
// helper names, and context objects. Those are useful search hints but are too
// noisy for hard relation gates. This helper therefore prefers the
// request-mentioned lane, exact targets, and the analyzer's original primary
// shortlist. It only falls back to Entities for legacy callers that have no
// provenance split at all and no known derived lane.
func StructuralRelationScopeCandidates(rm RequestModel) []string {
	var out []string
	add := func(values []string) {
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			dup := false
			for _, existing := range out {
				if strings.EqualFold(existing, value) {
					dup = true
					break
				}
			}
			if !dup {
				out = append(out, value)
			}
		}
	}
	add(rm.AnalyzerHints.MentionedEntities)
	add(rm.AnalyzerHints.ExactTargets)
	add(rm.AnalyzerHints.PrimaryEntities)
	if len(out) == 0 && len(rm.AnalyzerHints.DerivedEntities) == 0 {
		add(rm.AnalyzerHints.Entities)
	}
	return out
}

// IsArchitectureNarrativeExplanation reports whether the request is
// asking for an architecture / logical-view / diagram narrative rather
// than a principal member set.
//
// Architecture explanations often name many components because the
// answer is a decomposition plus relationships between components.
// Those names are excellent search hints, but they are not necessarily
// the answer members. This trait is the shared typed boundary that
// keeps analyzer amplification, family routing, and must-include
// pinning from treating component participants as a bounded
// enumeration unless the request also carries an explicit structural
// enumeration obligation.
//
// The signal is intentionally schema-only: Scenario, Intent,
// DiagramHint, SubTopics, predicates, and QuestionStructure. No raw
// request keyword matching is used, so the rule is language-neutral
// across Go, C/C++, ArkTS, Cangjie, Java/Kotlin, JS/TS, Python, Rust,
// and mixed-language repositories.
func IsArchitectureNarrativeExplanation(rm RequestModel) bool {
	if rm.Intent != IntentExplain || rm.Scenario != ScenarioArchitectureExplain {
		return false
	}
	if rm.Predicates.IsScalarAnswer ||
		rm.Predicates.IsRelationalLookup ||
		rm.Predicates.IsCountQuestion ||
		rm.Predicates.IsHistoryLookup ||
		rm.Predicates.IsDiagnosticQuestion {
		return false
	}
	if IsSingleTopicMechanismExplanation(rm) {
		return false
	}
	if rm.EnumerationBoundary != nil && rm.EnumerationBoundary.DeclaredCount > 0 {
		return false
	}
	if rm.CompletenessObligation.IsActive() {
		return false
	}
	if len(rm.QuestionStructure().Buckets) >= 2 {
		return false
	}
	return rm.DiagramHint != nil ||
		len(rm.SubTopics) > 1 ||
		rm.Predicates.IsCrossComponent ||
		rm.Complexity == ComplexityComplex
}

// IsSingleTopicMechanismExplanation reports whether the request asks
// for one mechanism / condition / registration explanation rather than
// an answer-member set, scalar lookup, or architecture decomposition.
//
// This trait intentionally consumes only typed analyzer fields. It is
// shared by the analyzer amplifier and QuestionFamily routing so the
// system does not upgrade mechanism participants (tool names, fields,
// helpers, routes, macros, spans, config keys, etc.) into hard
// principal members or architecture components merely because more
// than one identifier is involved. A bare IsCrossComponent=true flag
// is not enough to disqualify this lane: without multiple subtopics,
// buckets, or a relational lookup, it may only mean the mechanism
// crosses files during implementation while still remaining one user
// question.
func IsSingleTopicMechanismExplanation(rm RequestModel) bool {
	if rm.Intent != IntentExplain {
		return false
	}
	if rm.Predicates.IsScalarAnswer ||
		rm.Predicates.IsRelationalLookup ||
		rm.Predicates.IsCategoryEnumeration ||
		rm.Predicates.IsCountQuestion ||
		rm.Predicates.IsHistoryLookup ||
		rm.Predicates.IsDiagnosticQuestion {
		return false
	}
	if HasPrincipalAnswerSetObligation(rm) ||
		(rm.EnumerationBoundary != nil && rm.EnumerationBoundary.DeclaredCount > 0) {
		return false
	}
	if HasNonEmptyAmbiguity(rm) {
		return false
	}
	switch rm.PredicateAxis {
	case AxisCondition, AxisCall, AxisRegister:
		return true
	}
	switch NormalizeRequirementKind(rm.AnalyzerHints.Kind) {
	case ReqMechanism, ReqConditional, ReqRegistration:
		return true
	default:
		return false
	}
}

func hasExhaustiveMultiMemberSet(rm RequestModel) bool {
	if len(rm.AnalyzerHints.Entities) <= 1 {
		return false
	}
	if IsArchitectureNarrativeExplanation(rm) {
		return false
	}
	if rm.EnumerationBoundary != nil && rm.EnumerationBoundary.DeclaredCount > 0 {
		return true
	}
	if len(rm.AnalyzerHints.RequiredFileHints) > 1 {
		return true
	}
	return rm.CompletenessObligation.IsActive()
}

func distinctNonEmptyStrings(values []string) int {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		key := strings.ToLower(strings.TrimSpace(value))
		if key == "" {
			continue
		}
		seen[key] = struct{}{}
	}
	return len(seen)
}

// IsCodeIdentitySurface accepts cross-language code identity surfaces
// that are single tokens but may not be Go-style identifiers: Python
// modules (foo_bar), Java/Kotlin packages (com.example.foo), Rust /
// npm packages (foo-bar), scoped JS packages (@scope/pkg), C++/C#
// namespaces (foo::bar), and path-like package names. It rejects
// whitespace and prose punctuation so free-form phrases cannot become
// code identities by accident.
func IsCodeIdentitySurface(s string) bool {
	t := strings.TrimSpace(s)
	if t == "" {
		return false
	}
	hasAlphaNum := false
	for _, r := range t {
		// Non-ASCII characters (CJK / Cyrillic / Greek / emoji …) are
		// display prose in this codebase's identifier conventions —
		// Go / TS / Rust / Java / Cangjie / ArkTS code uses ASCII
		// identifiers in practice, so a token containing any non-ASCII
		// rune is a user-facing label, not a code identity. Without
		// this guard, Go's unicode.IsLetter mis-classifies CJK display
		// labels ("引用锚定", "自审查机制") as code-identity-shaped,
		// which then triggers the citation-alignment oracle to demand
		// the label name a symbol at the cited file:line — a
		// requirement no Chinese-only label can satisfy.
		if r > unicode.MaxASCII {
			return false
		}
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			hasAlphaNum = true
		case r == '_' || r == '.' || r == '-' || r == '/' || r == ':' || r == '@':
			continue
		default:
			return false
		}
	}
	return hasAlphaNum
}

// IsScalarSourceLiteralLookup reports whether the request resolves to
// one named source-code literal rather than a mechanism walkthrough or
// a set-valued answer. Shared by analyzer reconcile, prompt shaping,
// and answer-surface compilation so those stages do not re-derive the
// same scalar-lookup policy independently.
func IsScalarSourceLiteralLookup(rm RequestModel) bool {
	if len(rm.SubTopics) > 1 {
		return false
	}
	if rm.Predicates.IsCountQuestion ||
		rm.Predicates.IsCategoryEnumeration ||
		rm.Predicates.IsCrossComponent ||
		rm.Predicates.IsRelationalLookup ||
		rm.Predicates.IsHistoryLookup ||
		rm.Predicates.IsDiagnosticQuestion {
		return false
	}
	if !isScalarSourceLiteralSubjectKind(rm.AnswerSubject.Kind) {
		if !(rm.Predicates.IsRoleLocateLookup && rm.AnswerSubject.Kind == SubjectConfigKey) {
			return false
		}
	}
	// 2026-05-02 — the LLM's role-locate signal is a "where is the
	// thing that plays this role" judgment, locally plausible for any
	// "X 来自哪里" wording. But when the user attached a multi-frame
	// log/perf artifact, that artifact is OBJECTIVE evidence the
	// answer surface is a multi-step mechanism, not a single source
	// literal. In that case the role-locate short-circuit is
	// contradicted by user-supplied artifact data and must yield to
	// the regular IsScalarAnswer / structural-fallback path below.
	// Pre-fix the short-circuit fired unconditionally and routed
	// panic / OOM / perf-trace root-cause requests into the scalar
	// lane, breaking the answer contract downstream.
	if rm.Predicates.IsRoleLocateLookup && !hasMultiFrameArtifactEvidence(rm) {
		return true
	}
	if rm.Predicates.IsScalarAnswer {
		return true
	}
	// Fallback for role-locate lookups: the analyzer sometimes
	// correctly identifies the subject kind (function / type / route /
	// file path) but still emits a prose/list carrier. For a single,
	// simple, non-relational request over one source literal, keep the
	// answer in the scalar lane even when is_scalar_answer=false.
	//
	// 2026-05-02 — same multi-frame artifact guard as the explicit
	// short-circuit above: when an attached log/perf bundle resolves
	// 2+ frames, the request is by definition NOT "over one source
	// literal" and must not enter the unnamed-fallback scalar lane.
	if hasMultiFrameArtifactEvidence(rm) {
		return false
	}
	if rm.Complexity != ComplexitySimple {
		return false
	}
	// Only activate this fallback when the user did NOT already name
	// a concrete source literal in the request. If there are
	// analyzer-detected entities / primary entities, the question is
	// more likely "explain Foo" than "locate the thing that plays this
	// role", and the strict scalar_answer=true path should decide.
	if len(rm.AnalyzerHints.PrimaryEntities) > 0 || len(rm.AnalyzerHints.Entities) > 0 {
		return false
	}
	switch rm.Intent {
	case IntentExplain, IntentUnknown, IntentReturnValue:
	default:
		return false
	}
	switch NormalizeRequirementKind(rm.AnalyzerHints.Kind) {
	case ReqMechanism, ReqRegistration, ReqUnknown:
		return true
	default:
		return false
	}
}

// hasMultiFrameArtifactEvidence reports whether the request arrived
// bundled with an external artifact (attached log / htrace / atrace)
// that resolved 2+ frames or stalls. Such artifacts are objective
// proof that the answer surface is a multi-step mechanism rather
// than a single source literal, and therefore should temper the
// IsRoleLocateLookup scalar short-circuit in
// IsScalarSourceLiteralLookup. Returns false when no bundle is
// attached (preserving pre-2026-05-02 behaviour for plain text-only
// questions).
//
// Threshold rationale: 2 frames / janks / stalls is the same lower
// bound logBundleAuthoritativeFrames + renderLogCallChain already
// use to distinguish "real call chain" from "single sample" — keeps
// the multi-frame definition consistent across the codebase.
//
// Read-only on RequestModel; safe to call with a zero-value rm.
func hasMultiFrameArtifactEvidence(rm RequestModel) bool {
	if rm.LogTriage != nil {
		for _, e := range rm.LogTriage.Errors {
			if len(e.Frames) >= 2 {
				return true
			}
		}
	}
	if rm.PerfTrace != nil {
		if len(rm.PerfTrace.Frames) >= 2 ||
			len(rm.PerfTrace.Janks) >= 2 ||
			len(rm.PerfTrace.Stalls) >= 2 {
			return true
		}
	}
	return false
}

// HasExternalOnlyRuntimeArtifact reports whether the current request
// carries a structured runtime artifact whose facts are observation-
// only for this checkout: the log / trace has answer-grade events, but
// none of its frames resolved to current repository files. Downstream
// answer-shape compilers use this precise typed signal to avoid turning
// artifact observations into current-code path obligations.
func (rm RequestModel) HasExternalOnlyRuntimeArtifact() bool {
	if rm.LogTriage != nil && rm.LogTriage.IsExternalSource() {
		return true
	}
	if rm.PerfTrace != nil && rm.PerfTrace.IsExternalSource() {
		return true
	}
	return false
}

// HasObservationOnlyRuntimeArtifact reports the narrower external-
// runtime shape where the user's current request can be answered from
// the attached artifact itself and does NOT carry a separate typed
// current-checkout target. Downstream hard-focus and enrichment
// surfaces use this to keep external observations from being polluted
// by unrelated repo symbols that merely resemble artifact labels.
func (rm RequestModel) HasObservationOnlyRuntimeArtifact() bool {
	return rm.HasExternalOnlyRuntimeArtifact() &&
		!rm.HasRuntimeArtifactCurrentVerificationAnchor()
}

// HasRuntimeArtifactCurrentVerificationAnchor reports whether an external
// runtime artifact has a separate, typed current-checkout target strong enough
// to justify opening the current-source lane. The signal must come from
// structured analyzer fields; stack-frame labels from an unresolved external
// log are not enough by themselves.
func (rm RequestModel) HasRuntimeArtifactCurrentVerificationAnchor() bool {
	if !rm.HasExternalOnlyRuntimeArtifact() {
		return false
	}
	if rm.CurrentSourceExplanationProfile != nil && rm.CurrentSourceExplanationProfile.Active() {
		return true
	}
	if rm.LogTriage != nil && len(rm.LogTriage.ResolvedFiles) > 0 {
		return true
	}
	if rm.PerfTrace != nil && len(rm.PerfTrace.ResolvedFiles) > 0 {
		return true
	}
	for _, target := range rm.AnalyzerHints.ExactTargets {
		if strings.TrimSpace(target) != "" {
			return true
		}
	}
	for _, hint := range rm.AnalyzerHints.RequiredFileHints {
		if strings.TrimSpace(hint.Path) != "" {
			return true
		}
	}
	if rm.RequestedAnswerDimensions != nil && rm.RequestedAnswerDimensions.Active() {
		for _, dim := range rm.RequestedAnswerDimensions.Dimensions {
			if dim.Role == RequestedAnswerDimensionCurrentKeyCode {
				return true
			}
		}
	}
	return false
}

func isScalarSourceLiteralSubjectKind(kind AnswerSubjectKind) bool {
	switch kind {
	case SubjectFunctionName,
		SubjectTypeName,
		SubjectInterface,
		SubjectHandlerRoute,
		SubjectFilePath,
		SubjectStringLiteral,
		SubjectEnumValue,
		SubjectStructField:
		return true
	}
	return false
}

// IsScalarRoleLocateLookup is the narrower subset of scalar source-
// literal lookups where the user describes a role/clue ("the entry
// function that ...", "the file that ...") and wants the located
// literal itself. These answers should keep summary surface tight and
// avoid expanding into surrounding mechanism prose unless the user
// explicitly asked for it.
func IsScalarRoleLocateLookup(rm RequestModel) bool {
	if !IsScalarSourceLiteralLookup(rm) {
		return false
	}
	if rm.Predicates.IsRoleLocateLookup {
		return true
	}
	if rm.AnswerSubject.Kind == SubjectReturnValue {
		return false
	}
	if !rm.Predicates.IsScalarAnswer {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(rm.AnalyzerHints.Kind), "return_value") {
		return true
	}
	return rm.PredicateAxis == AxisReturn
}

// IsProjectOrientationQuestion reports whether the request is a
// project-orientation ask ("what does this repo do?", "summarise the
// project", "give me a tour"). Detection is structured-signal-only:
// no substring matching on RawRequest, language-neutral, driven
// entirely by the analyzer LLM's existing classification.
//
// Conditions (all must hold):
//
//   - intent: explain or unknown (root_cause / trace / etc. always
//     fall through to deep investigation)
//   - complexity: simple — the analyzer's own assessment that scope
//     is single-entity / 1-2 files
//   - predicates: is_cross_component / is_count_question /
//     is_history_lookup / is_diagnostic_question / is_scalar_answer ALL false
//   - len(PrimaryEntities) == 0 — the user didn't pin to specific code
//   - len(Entities) == 0 — no identifier-shaped tokens in the request
//
// Shared by:
//
//   - internal/tool/emit_investigation_complete.go:
//     applyMultiPathAnchorChecks (the multi-path symbol-anchored
//     gate) skips orientation questions because they don't need
//     cross-component depth.
//   - internal/analysis/budget/budget.go: tightens the EvidenceBudget
//     base so the explorer's existing MaxFiles / MaxReactIters caps
//     enforce a smaller ceiling — README + manifest + entry-point
//     answer needs ~5 files, not the moderate-default 30.
//
// Returns false on a zero-value RequestModel — callers default to
// "fire the gate / use full budget" which preserves pre-2026-04-29
// behaviour for any path that didn't run the analyzer.
func IsProjectOrientationQuestion(rm RequestModel) bool {
	switch rm.Intent {
	case IntentExplain, IntentUnknown:
		// continue
	default:
		return false
	}
	if rm.Complexity != ComplexitySimple {
		return false
	}
	if rm.Predicates.IsCrossComponent ||
		rm.Predicates.IsCountQuestion ||
		rm.Predicates.IsHistoryLookup ||
		rm.Predicates.IsDiagnosticQuestion ||
		rm.Predicates.IsScalarAnswer {
		return false
	}
	if len(rm.AnalyzerHints.PrimaryEntities) > 0 {
		return false
	}
	if len(rm.AnalyzerHints.Entities) > 0 {
		return false
	}
	return true
}

// IsSingleTopicStructuralTrace reports whether the request is a
// single-topic structural walkthrough (call-chain / flow / dispatch)
// that benefits from a lighter trace-oriented DAG rather than the
// heavier architecture-explain template with a dedicated reconcile
// window.
//
// Important distinction: a single ordered trace may legitimately cross
// files / packages / components without becoming a multi-topic
// architecture survey. What disqualifies the lighter lane is not
// "crossing modules" by itself, but structurally independent topics
// (multiple sub-topics), ambiguity that still needs reconciliation, or
// set-style / relational asks that are not one source-to-sink chain.
//
// The signals stay typed and language-neutral: trace intent +
// structural axis / question kind, while explicitly excluding
// multi-topic, ambiguity-bearing, relational, enumerative, and
// history-style questions that genuinely need broader orchestration.
func IsSingleTopicStructuralTrace(rm RequestModel) bool {
	if rm.Intent != IntentTrace {
		return false
	}
	if len(rm.SubTopics) > 1 || HasNonEmptyAmbiguity(rm) {
		return false
	}
	if rm.Predicates.IsRelationalLookup ||
		rm.Predicates.IsCategoryEnumeration ||
		rm.Predicates.IsCountQuestion ||
		rm.Predicates.IsHistoryLookup ||
		rm.Predicates.IsDiagnosticQuestion {
		return false
	}
	switch rm.PredicateAxis {
	case AxisCall, AxisCondition, AxisRegister:
		return true
	}
	switch NormalizeRequirementKind(rm.AnalyzerHints.Kind) {
	case ReqCallChain, ReqConditional, ReqMechanism, ReqRegistration:
		return true
	}
	return false
}
