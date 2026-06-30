package types

import (
	"regexp"
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
// diagram. The signal remains schema-only. It consumes analyzer predicates,
// AnswerSubject, DiagramHint, and PredicateAxis, never localized raw request
// text, so relation facts stay language-neutral across every repomap-supported
// language.
func ShouldSurfaceTypedRelationHints(rm RequestModel) bool {
	return HasTypedRelationQueryShape(rm, TypedRelationPurposePromptHint)
}

// HasInterfaceTypedRelationDiagramShape reports a typed shape where the user
// needs a structural diagram of an interface / trait / protocol relation even
// if the analyzer chose a broad predicate axis such as "define". This helper is
// deliberately schema-only: it allows exact graph relation probes to run, but
// the probe still emits nothing unless repomap resolves the entity as an
// interface-like symbol with concrete relation members.
func HasInterfaceTypedRelationDiagramShape(rm RequestModel) bool {
	return rm.AnswerSubject.Kind == SubjectInterface &&
		rm.DiagramHint != nil &&
		rm.DiagramHint.Kind != DiagramNone
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
	if !HasTypedRelationQueryShape(rm, TypedRelationPurposeCoverageGate) {
		return false
	}
	return rm.PredicateAxis == AxisImplement ||
		rm.Predicates.IsRelationalLookup ||
		HasInterfaceTypedRelationDiagramShape(rm)
}

// SourceInventoryLaneConflictsWithPrincipalAnswer reports whether source
// inventory may assist navigation but must not own completion authority.
func SourceInventoryLaneConflictsWithPrincipalAnswer(rm RequestModel) bool {
	return SourceInventoryLaneConflictsWithRoleBinding(rm) ||
		SourceInventoryLaneConflictsWithRelationFlow(rm) ||
		HasTypedRelationMemberSetShape(rm)
}

// SourceInventoryProfileConflictsWithRelationFlow reports whether an analyzer
// emitted source_inventory_profile for a request whose principal answer shape
// is a structural flow/trace rather than a bounded source member inventory.
//
// The helper is deliberately schema-only. It consumes intent, question kind,
// predicate axis, and semantic predicates; it never scans RawRequest or model
// prose. This keeps the rule language-neutral across all repomap-supported
// languages and avoids the historical bug class where a call-chain / dispatch
// walkthrough was flattened into a dry "all functions" inventory because the
// user also asked for key files/functions.
func SourceInventoryProfileConflictsWithRelationFlow(rm RequestModel) bool {
	if rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		return false
	}
	return SourceInventoryLaneConflictsWithRelationFlow(rm)
}

// SourceInventoryProfileConflictsWithRoleBinding reports whether an analyzer
// emitted source_inventory_profile for a registry/catalog/binding member-set
// answer. These questions may use source inventory as navigation, but the
// principal universe is the binding relation proven by registration/call
// evidence, not every source type/function in the repository.
func SourceInventoryProfileConflictsWithRoleBinding(rm RequestModel) bool {
	if rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		return false
	}
	return SourceInventoryLaneConflictsWithRoleBinding(rm)
}

// SourceInventoryLaneConflictsWithRoleBinding is the profile-independent form
// used before synthesis and by completion authority. It is intentionally typed
// only: PredicateAxis and answer-shape predicates decide the boundary. No user
// keywords, model prose, or tool summaries are inspected.
//
// The answer-role profile is helpful when present, but it is model-authored and
// may be omitted or set false while the more stable PredicateAxis still says
// "register". In that shape, source_inventory remains useful navigation, but
// registry/catalog/binding membership must be proven by registration/call
// evidence and a structured member_set, not by exhausting every source
// function/type in the repository.
func SourceInventoryLaneConflictsWithRoleBinding(rm RequestModel) bool {
	roleBindingRequested := rm.AnswerRoleProfile != nil && rm.AnswerRoleProfile.IsRoleBindingRequested
	if rm.PredicateAxis != AxisRegister && !roleBindingRequested {
		return false
	}
	if rm.PredicateAxis != AxisRegister && sourceInventoryPrincipalLaneOverridesRoleBindingProfile(rm) {
		return false
	}
	return IsCategoryEnumerationAnswerShape(rm) ||
		rm.Predicates.IsCountQuestion ||
		rm.CompletenessObligation.IsActive() ||
		RequiresExhaustiveEnumerationMemberSetHandoff(rm) ||
		RequiresRelationMemberSetHandoff(rm)
}

func sourceInventoryPrincipalLaneOverridesRoleBindingProfile(rm RequestModel) bool {
	if rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		return false
	}
	if !IsCategoryEnumerationAnswerShape(rm) || rm.Intent != IntentEnumerate {
		return false
	}
	return SourceInventoryProfileHasPrincipalPrecision(rm)
}

// SourceInventoryLaneConflictsWithRelationFlow reports whether synthesizing or
// preserving a source-inventory lane would fight the typed principal-answer
// shape. Unlike SourceInventoryProfileConflictsWithRelationFlow, this can be
// used before a SourceInventoryProfile exists, so analyzer normalization can
// safely recover missing inventory lanes without upgrading call-chain / trace
// walkthroughs into member lists.
func SourceInventoryLaneConflictsWithRelationFlow(rm RequestModel) bool {
	if rm.Predicates.IsCategoryEnumeration ||
		rm.Predicates.IsRelationalLookup ||
		rm.Predicates.IsCountQuestion {
		return false
	}
	if IsSingleTopicStructuralTrace(rm) {
		return true
	}
	kind := NormalizeRequirementKind(rm.AnalyzerHints.Kind)
	if kind == ReqCallChain {
		return true
	}
	if rm.Intent != IntentTrace {
		return false
	}
	switch rm.PredicateAxis {
	case AxisCall, AxisCondition, AxisRegister:
		return true
	default:
		return false
	}
}

// SourceInventoryPrincipalNavigationActive reports whether source_inventory is
// the principal navigation lane for tool refinements. It is intentionally
// schema-only and softer than completion authority: callers use it to choose
// guidance order, not to prove answer completeness.
func SourceInventoryPrincipalNavigationActive(rm RequestModel) bool {
	return SourceInventoryPrincipalAuthorityActive(rm)
}

// SourceInventoryPrincipalAuthorityActive reports whether a source-inventory
// profile is precise enough to carry scheduling / completion authority.
//
// A model-emitted `source_inventory_profile` by itself is not enough: broad
// category/count questions about registries, bindings, roles, or architecture
// can accidentally look like "list all types/functions" even though the answer
// is proven by owner/relationship evidence. The authority boundary therefore
// consumes only typed, structural precision signals: an explicit source scope,
// language-neutral structural facets such as type-underlying / const-set, or a
// bounded same-directory source universe. Model rationale, raw request prose,
// and localized keyword tables never participate.
func SourceInventoryPrincipalAuthorityActive(rm RequestModel) bool {
	if rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		return false
	}
	if SourceInventoryCompletionIsSupportOnly(rm) {
		return false
	}
	if SourceInventoryLaneConflictsWithRoleBinding(rm) {
		return false
	}
	if SourceInventoryLaneConflictsWithRelationFlow(rm) {
		return false
	}
	return SourceInventoryProfileHasPrincipalPrecision(rm)
}

// SourceInventoryProfileHasPrincipalPrecision is the structural precision
// check behind SourceInventoryPrincipalAuthorityActive. It deliberately does
// not read profile rationale. A model confidence value, when present and low,
// is allowed to demote an otherwise unscoped profile because that is a
// fail-open-to-ordinary-navigation choice: low-confidence source_inventory
// remains available as advisory context, but it cannot own the loop.
func SourceInventoryProfileHasPrincipalPrecision(rm RequestModel) bool {
	profile := rm.SourceInventoryProfile
	if profile == nil || !profile.Active() {
		return false
	}
	if profile.RequiresConstSet ||
		(profile.TypeUnderlying != "" && profile.TypeUnderlying != SourceInventoryTypeUnderlyingUnknown) {
		return true
	}
	if sourceInventoryHasExplicitTypedScope(rm.SourceScopeProfile) {
		return true
	}
	files := BoundedSourceEnumerationScopeFiles(rm, nil, "")
	if len(files) >= BoundedSourceEnumerationMinFiles &&
		BoundedSourceEnumerationCommonScope(files) != "" {
		return true
	}
	return profile.Confidence <= 0 || profile.Confidence >= 0.8
}

func sourceInventoryHasExplicitTypedScope(profile *SourceScopeProfile) bool {
	if profile == nil {
		return false
	}
	scope := profile.RequestedScope
	return scope != "" && scope != SourceScopeUnknown && scope.IsValid()
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

// RequiresSourceOperationSiteMemberSetHandoff reports whether a
// current-source mechanism/root-cause request asks for a principal set of
// operation sites such as write points, call sites, registration points, or
// entry points.
//
// Most mechanism questions are narrative: discovered helpers and branches are
// supporting context, not an answer-member slate. This helper is intentionally
// narrower and typed-only: the analyzer must provide both a set-boundary signal
// and an operation-site surface signal. No RawRequest keyword table, repo-map
// rank, grep count, evidence label, or model prose can activate this trait.
func RequiresSourceOperationSiteMemberSetHandoff(rm RequestModel) bool {
	if rm.Predicates.IsScalarAnswer ||
		rm.Predicates.IsCountQuestion ||
		rm.Predicates.IsHistoryLookup ||
		rm.Predicates.IsRoleLocateLookup {
		return false
	}
	if rm.Scenario == ScenarioArchitectureExplain &&
		(rm.Predicates.IsCrossComponent || rm.DiagramHint != nil || len(rm.SubTopics) > 1) {
		return false
	}
	if IsArchitectureNarrativeExplanation(rm) {
		return false
	}
	if !sourceOperationSiteHasTypedSetBoundary(rm) {
		return false
	}
	return sourceOperationSiteHasTypedSurface(rm)
}

func sourceOperationSiteHasTypedSetBoundary(rm RequestModel) bool {
	if rm.Intent == IntentEnumerate ||
		rm.Predicates.IsCategoryEnumeration ||
		rm.Predicates.IsRelationalLookup ||
		rm.Predicates.HasPerMemberTable {
		return true
	}
	if rm.EnumerationBoundary != nil && rm.EnumerationBoundary.DeclaredCount > 0 {
		return true
	}
	if rm.CompletenessObligation.IsActive() {
		return true
	}
	return len(rm.QuestionStructure().Buckets) >= 2
}

func sourceOperationSiteHasTypedSurface(rm RequestModel) bool {
	if sourceOperationSiteInventoryProfileActive(rm) {
		return true
	}
	switch rm.PredicateAxis {
	case AxisCall, AxisRegister, AxisConfigure, AxisCondition:
		return true
	}
	switch NormalizeRequirementKind(rm.AnalyzerHints.Kind) {
	case ReqCallChain, ReqRegistration, ReqConfigMapping:
		return true
	}
	return false
}

func sourceOperationSiteInventoryProfileActive(rm RequestModel) bool {
	if rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		return false
	}
	for _, role := range rm.SourceInventoryProfile.PrincipalTargetRoles() {
		switch role {
		case AnswerCandidateRoleFunction,
			AnswerCandidateRoleMethod,
			AnswerCandidateRoleRoute,
			AnswerCandidateRoleFile:
			return true
		}
	}
	return false
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
	if rm.RuntimeArtifactValueProfile != nil && rm.RuntimeArtifactValueProfile.Active() {
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
	if rm.RuntimeArtifactValueProfile != nil && rm.RuntimeArtifactValueProfile.Active() {
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

// StructuralRelationCoverageScopeCandidates is the hard-gate sibling of
// StructuralRelationScopeCandidates. It deliberately omits the legacy
// AnalyzerHints.Entities fallback: broad entity lists are useful prompt/search
// hints, but they can contain helper concepts, generic role names, and relation
// targets mixed with answer members. Hard relation coverage may only start from
// provenance-bearing lanes the analyzer marked as request-mentioned, exact, or
// primary.
func StructuralRelationCoverageScopeCandidates(rm RequestModel) []string {
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
		rm.Predicates.IsRoleLocateLookup ||
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

// IsArchitectureInventoryShape reports whether an architecture_explain request
// asks for a bounded inventory/decomposition surface: principal members,
// per-member roles, counts, or relationship edges.
//
// This differs from IsArchitectureNarrativeExplanation. Narrative questions
// need broad explanatory context, while inventory questions need a compact
// entity/edge ledger. The signal is typed-only: it consumes analyzer scenario,
// intent, predicates, source-inventory profile, diagram/subtopic structure, and
// predicate axis. It must not inspect RawRequest or model prose, so it remains
// language-neutral across Go, C/C++, ArkTS, Cangjie, Java/Kotlin, JS/TS,
// Python, Rust, Ruby, Swift, Lua, Proto, and mixed-language repositories.
func IsArchitectureInventoryShape(rm RequestModel) bool {
	if rm.Scenario != ScenarioArchitectureExplain {
		return false
	}
	if rm.Predicates.IsScalarAnswer && !rm.Predicates.IsCountQuestion {
		return false
	}
	if rm.Predicates.IsRoleLocateLookup ||
		rm.Predicates.IsHistoryLookup ||
		rm.Predicates.IsDiagnosticQuestion {
		return false
	}
	if rm.FieldValueProfile != nil && rm.FieldValueProfile.Active() {
		return false
	}
	if rm.RuntimeArtifactValueProfile != nil && rm.RuntimeArtifactValueProfile.Active() {
		return false
	}
	if rm.ChangeImpactProfile != nil && rm.ChangeImpactProfile.Active() {
		return false
	}
	if rm.SourceInventoryProfile != nil && rm.SourceInventoryProfile.Active() {
		return true
	}
	if rm.Intent == IntentEnumerate ||
		rm.Predicates.IsCategoryEnumeration ||
		rm.Predicates.IsCountQuestion ||
		rm.Predicates.HasPerMemberTable {
		return true
	}
	if rm.Predicates.IsRelationalLookup {
		return true
	}
	return len(rm.SubTopics) > 1 &&
		(rm.Predicates.IsCrossComponent || rm.DiagramHint != nil)
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
		rm.Predicates.IsRoleLocateLookup ||
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

// HasRuntimeArtifactPathReference reports whether the analyzer preserved an
// explicit log/trace artifact path in structured request fields. This covers
// user-provided paths in the question (absolute or relative) where no
// log_triage/perf_triage bundle exists because the artifact was not attached
// via --log/--htrace. The signal is deliberately typed: callers inspect only
// analyzer-emitted path carriers plus the external-artifact citation policy,
// never final-answer prose.
func (rm RequestModel) HasRuntimeArtifactPathReference() bool {
	if rm.ExternalObservationPolicy == nil ||
		(!rm.ExternalObservationPolicy.ArtifactCitationsExternalOnly() &&
			!rm.ExternalObservationPolicy.ExcludesCurrentSource()) {
		return false
	}
	for _, raw := range rm.runtimeArtifactPathReferenceCandidates() {
		if RuntimeArtifactPathKindInText(raw) != "" {
			return true
		}
	}
	return false
}

// RuntimeArtifactPathReferenceKind returns the first explicit runtime artifact
// path family preserved in structured analyzer fields. Empty means no typed
// runtime path reference is active. Callers use this for origin-specific
// handling (log vs trace) without parsing model prose.
func (rm RequestModel) RuntimeArtifactPathReferenceKind() string {
	if !rm.HasRuntimeArtifactPathReference() {
		return ""
	}
	for _, hint := range rm.AnalyzerHints.RequiredFileHints {
		if kind := RuntimeArtifactPathKind(hint.Path); kind != "" {
			return kind
		}
	}
	for _, raw := range rm.runtimeArtifactPathReferenceCandidates() {
		if kind := RuntimeArtifactPathKindInText(raw); kind != "" {
			return kind
		}
	}
	return ""
}

func (rm RequestModel) runtimeArtifactPathReferenceCandidates() []string {
	var out []string
	out = append(out, rm.AnalyzerHints.ExactTargets...)
	out = append(out, rm.AnalyzerHints.MentionedEntities...)
	out = append(out, rm.AnalyzerHints.PrimaryEntities...)
	out = append(out, rm.AnalyzerHints.Entities...)
	for _, hint := range rm.AnalyzerHints.RequiredFileHints {
		out = append(out, hint.Path)
	}
	if rm.CurrentSourceExplanationProfile != nil {
		out = append(out, rm.CurrentSourceExplanationProfile.SourceQuotes...)
	}
	if rm.ExternalObservationPolicy != nil {
		out = append(out, rm.ExternalObservationPolicy.SourceQuotes...)
	}
	if rm.RequestedAnswerDimensions != nil {
		for _, dim := range rm.RequestedAnswerDimensions.Dimensions {
			out = append(out, dim.SourceQuote, dim.Label)
		}
	}
	return out
}

// HasExternalObservationArtifactReference reports whether the analyzer has
// explicitly classified visible line/row references as external-observation
// citations. This covers MCP/resources/web/docs where there may be no attached
// runtime artifact bundle, but the coordinates still must not become
// current-source line citations.
func (rm RequestModel) HasExternalObservationArtifactReference() bool {
	if rm.ExternalObservationPolicy == nil || !rm.ExternalObservationPolicy.ArtifactCitationsExternalOnly() {
		return false
	}
	for _, target := range rm.AnalyzerHints.ExactTargets {
		if looksLikeExternalObservationReference(target) || looksLikeExternalArtifactCoordinate(target) {
			return true
		}
	}
	for _, entity := range rm.AnalyzerHints.Entities {
		if looksLikeExternalObservationReference(entity) || looksLikeExternalArtifactCoordinate(entity) {
			return true
		}
	}
	for _, entity := range rm.AnalyzerHints.PrimaryEntities {
		if looksLikeExternalObservationReference(entity) || looksLikeExternalArtifactCoordinate(entity) {
			return true
		}
	}
	for _, entity := range rm.AnalyzerHints.MentionedEntities {
		if looksLikeExternalObservationReference(entity) || looksLikeExternalArtifactCoordinate(entity) {
			return true
		}
	}
	for _, hint := range rm.AnalyzerHints.RequiredFileHints {
		if looksLikeExternalObservationReference(hint.Path) || looksLikeExternalArtifactCoordinate(hint.Path) {
			return true
		}
	}
	if rm.CurrentSourceExplanationProfile != nil {
		for _, quote := range rm.CurrentSourceExplanationProfile.SourceQuotes {
			if looksLikeExternalObservationReference(quote) || looksLikeExternalArtifactCoordinate(quote) {
				return true
			}
		}
	}
	if rm.RequestedAnswerDimensions != nil {
		for _, dim := range rm.RequestedAnswerDimensions.Dimensions {
			if looksLikeExternalObservationReference(dim.SourceQuote) ||
				looksLikeExternalArtifactCoordinate(dim.SourceQuote) ||
				looksLikeExternalArtifactCoordinate(dim.Label) {
				return true
			}
		}
	}
	return false
}

// HasObservationOnlyRuntimeArtifact reports the narrower external-
// runtime shape where the user's current request explicitly excludes current
// checkout evidence. Omitted policy defaults to mixed external-observation plus
// current-source analysis; unresolved artifact frames alone are not a source
// exclusion signal.
func (rm RequestModel) HasObservationOnlyRuntimeArtifact() bool {
	return rm.HasExternalOnlyRuntimeArtifact() &&
		!rm.HasRuntimeArtifactCurrentVerificationAnchor() &&
		rm.ExternalObservationPolicy != nil &&
		rm.ExternalObservationPolicy.ExcludesCurrentSource()
}

// ExternalObservationAllowsCurrentSource reports whether the analyzer emitted
// an explicit typed mixed-lane request. This is stronger than the default
// posture: default means current source is permitted when useful, while allow
// means downstream must not collapse the turn into runtime-observation-only.
func (rm RequestModel) ExternalObservationAllowsCurrentSource() bool {
	return rm.ExternalObservationPolicy != nil &&
		rm.ExternalObservationPolicy.CurrentSourceMode == ExternalObservationCurrentSourceAllow
}

// HasRuntimeArtifactWithoutRequiredCurrentSource reports that runtime artifact
// observations are answer-grade and current-checkout source evidence is not a
// hard requirement for this request. This is the shared precise signal for
// suppressing current-source citation/read/review hard gates while preserving the
// default policy that source exploration remains allowed when useful.
func (rm RequestModel) HasRuntimeArtifactWithoutRequiredCurrentSource() bool {
	return (rm.HasExternalOnlyRuntimeArtifact() || rm.HasRuntimeArtifactPathReference()) &&
		!rm.CurrentSourceLaneDecision().RequiresCurrentSource()
}

// HasRuntimeArtifactWithoutRequiredCurrentSourceInArtifactContext is the
// runtime-artifact-aware sibling of HasRuntimeArtifactWithoutRequiredCurrentSource.
// It covers attached logs/traces and trace_query path reads where runtime
// observations are present, but the pre-stage did not materialize a structured
// LogBundle/PerfBundle on the RequestModel. The synthetic observation is used
// only for source-lane decisioning; callers must not treat it as evidence.
func (rm RequestModel) HasRuntimeArtifactWithoutRequiredCurrentSourceInArtifactContext(attachedRuntimeArtifact bool) bool {
	if rm.HasRuntimeArtifactWithoutRequiredCurrentSource() {
		return true
	}
	if !attachedRuntimeArtifact {
		return false
	}
	withRuntime := rm.withAttachedRuntimeArtifact()
	return withRuntime.HasRuntimeArtifactWithoutRequiredCurrentSource()
}

// HasRuntimeArtifactWithoutRequiredCurrentSourceInTraceContext is kept for
// older trace-specific call sites. New code should pass the broader
// runtime-artifact context so logs and trace_query path observations share the
// same hard-gate boundary.
func (rm RequestModel) HasRuntimeArtifactWithoutRequiredCurrentSourceInTraceContext(attachedTrace bool) bool {
	return rm.HasRuntimeArtifactWithoutRequiredCurrentSourceInArtifactContext(attachedTrace)
}

// HasRuntimeArtifactObservationOnlySurface is the narrower source-optional
// runtime shape that may render observation-only start/completion guidance.
// Explicit current_source_mode=allow keeps a mixed lane available even when
// current-source evidence is not otherwise hard-required.
func (rm RequestModel) HasRuntimeArtifactObservationOnlySurface() bool {
	return rm.HasRuntimeArtifactWithoutRequiredCurrentSource() &&
		!rm.ExternalObservationAllowsCurrentSource()
}

func (rm RequestModel) HasRuntimeArtifactObservationOnlySurfaceInArtifactContext(attachedRuntimeArtifact bool) bool {
	if rm.HasRuntimeArtifactObservationOnlySurface() {
		return true
	}
	if !attachedRuntimeArtifact {
		return false
	}
	withRuntime := rm.withAttachedRuntimeArtifact()
	return withRuntime.HasRuntimeArtifactObservationOnlySurface()
}

func (rm RequestModel) HasRuntimeArtifactObservationOnlySurfaceInTraceContext(attachedTrace bool) bool {
	return rm.HasRuntimeArtifactObservationOnlySurfaceInArtifactContext(attachedTrace)
}

// RuntimeArtifactReadSourceNavigationNotRequired reports whether read-mode
// source navigation/localizer debt is advisory for this request. It is used by
// hard pre-finalize floors and final-answer system supplement stamping. The
// signal is typed: runtime artifact presence comes from structured bundles or
// an attached trace flag, and the current-source posture comes from
// CurrentSourceLaneDecision.
func RuntimeArtifactReadSourceNavigationNotRequired(ir *AnalysisIR, attachedRuntimeArtifact bool) bool {
	if ir == nil {
		return false
	}
	return RuntimeArtifactRequestSourceNavigationNotRequired(ir.RequestModel, attachedRuntimeArtifact)
}

// RuntimeArtifactReadSourceNavigationNotRequiredForBusContext is the
// answer/report-stage companion to RuntimeArtifactReadSourceNavigationNotRequired.
// It prefers RuntimeSourceAnswerAuthoritySnapshot when the run has accepted
// runtime/source evidence so final report consumers do not reinterpret soft
// route-backed source obligations independently.
func RuntimeArtifactReadSourceNavigationNotRequiredForBusContext(ctx *BusContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	if ctx.AnalysisIR.RequestModel.HasCurrentSourceObligationSignal() {
		return false
	}
	authority := BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(ctx, ObservationLedger{})
	if runtimeSourceAuthorityAppliesToReadSourceAudit(ctx, authority) {
		return runtimeSourceAuthoritySuppressesReadSourceAudit(authority)
	}
	return RuntimeArtifactReadSourceNavigationNotRequired(ctx.AnalysisIR, RuntimeArtifactContextActiveFromBus(ctx))
}

// RuntimeArtifactRequestSourceNavigationNotRequired is the pre-IR companion to
// RuntimeArtifactReadSourceNavigationNotRequired. Analyzer post-processing uses
// it before AnalysisIR exists to avoid eager source graph construction for
// answer-grade runtime artifacts whose current-source lane is typed optional.
func RuntimeArtifactRequestSourceNavigationNotRequired(rm RequestModel, attachedRuntimeArtifact bool) bool {
	return rm.HasRuntimeArtifactObservationOnlySurfaceInArtifactContext(attachedRuntimeArtifact)
}

// RuntimeArtifactReadSourceSupplementsNotRequired reports whether final answer
// source-localization / repo_map audit supplements should stay out of the
// user-facing answer surface. Source exploration may still have happened and
// remains available in TurnA / reasoning artifacts; this only prevents optional
// source-side audit tables from crowding runtime-artifact answers when current
// checkout source was not a typed requirement.
func RuntimeArtifactReadSourceSupplementsNotRequired(ir *AnalysisIR, attachedRuntimeArtifact bool) bool {
	if ir == nil {
		return false
	}
	return ir.RequestModel.HasRuntimeArtifactWithoutRequiredCurrentSourceInArtifactContext(attachedRuntimeArtifact)
}

// RuntimeArtifactReadSourceSupplementsNotRequiredForBusContext uses the shared
// runtime/source authority view for final-answer source audit suppression. It
// keeps source audit available once current-source proof is accepted, even if the
// original request was only a soft route-backed runtime/source mix.
func RuntimeArtifactReadSourceSupplementsNotRequiredForBusContext(ctx *BusContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	if ctx.AnalysisIR.RequestModel.HasCurrentSourceObligationSignal() {
		return false
	}
	authority := BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(ctx, ObservationLedger{})
	if runtimeSourceAuthorityAppliesToReadSourceAudit(ctx, authority) {
		return runtimeSourceAuthoritySuppressesReadSourceAudit(authority)
	}
	return RuntimeArtifactReadSourceSupplementsNotRequired(ctx.AnalysisIR, RuntimeArtifactContextActiveFromBus(ctx))
}

func runtimeSourceAuthorityAppliesToReadSourceAudit(ctx *BusContext, authority RuntimeSourceAnswerAuthoritySnapshot) bool {
	if !authority.Active {
		return false
	}
	if authority.RuntimeObservationCount > 0 ||
		authority.DeterministicRuntimeQueryCount > 0 ||
		authority.RuntimeOnlySufficient ||
		authority.CanHardBlockCompletion {
		return true
	}
	return authority.ExactCurrentSourceSupportCount > 0 &&
		ctx != nil &&
		RuntimeArtifactContextActiveFromBus(ctx)
}

func runtimeSourceAuthoritySuppressesReadSourceAudit(authority RuntimeSourceAnswerAuthoritySnapshot) bool {
	if !authority.Active {
		return false
	}
	if authority.ExactCurrentSourceSupportCount > 0 || authority.CanHardBlockCompletion {
		return false
	}
	return authority.CanUseRuntimeOnlyWithCaveat ||
		authority.CanDowngradeToCaveat ||
		(authority.RuntimeObservationCount > 0 && !authority.CurrentSourceRequired)
}

// HasRuntimeArtifactSourceOptionalMixedSurface is the counterpart of
// HasRuntimeArtifactObservationOnlySurface: runtime observations may answer the
// artifact lane, but the analyzer explicitly kept current-source analysis in
// scope, so prompts and closure gates must not say "leave source out".
func (rm RequestModel) HasRuntimeArtifactSourceOptionalMixedSurface() bool {
	return rm.HasRuntimeArtifactWithoutRequiredCurrentSource() &&
		rm.ExternalObservationAllowsCurrentSource()
}

func (rm RequestModel) HasRuntimeArtifactSourceOptionalMixedSurfaceInArtifactContext(attachedRuntimeArtifact bool) bool {
	if rm.HasRuntimeArtifactSourceOptionalMixedSurface() {
		return true
	}
	if !attachedRuntimeArtifact {
		return false
	}
	withRuntime := rm.withAttachedRuntimeArtifact()
	return withRuntime.HasRuntimeArtifactSourceOptionalMixedSurface()
}

func (rm RequestModel) HasRuntimeArtifactSourceOptionalMixedSurfaceInTraceContext(attachedTrace bool) bool {
	return rm.HasRuntimeArtifactSourceOptionalMixedSurfaceInArtifactContext(attachedTrace)
}

func (rm RequestModel) withAttachedTraceRuntimeArtifact() RequestModel {
	return rm.withAttachedRuntimeArtifact()
}

func (rm RequestModel) withAttachedRuntimeArtifact() RequestModel {
	if rm.LogTriage != nil && rm.LogTriage.HasStructuredObservations() {
		return rm
	}
	if rm.PerfTrace != nil && rm.PerfTrace.HasStructuredObservations() {
		return rm
	}
	clone := rm
	synthetic := PerfObservation{
		Kind:    "attached_runtime_artifact",
		Subject: "attached runtime artifact",
		Summary: "attached runtime log or trace artifact is present",
	}
	if clone.PerfTrace == nil {
		clone.PerfTrace = &PerfBundle{Observations: []PerfObservation{synthetic}}
		return clone
	}
	perf := *clone.PerfTrace
	perf.Observations = append(append([]PerfObservation(nil), perf.Observations...), synthetic)
	clone.PerfTrace = &perf
	return clone
}

// RuntimeArtifactContextActiveFromBus reports whether the current run has a
// precise runtime-artifact carrier outside ordinary repo source evidence. This
// intentionally consumes only typed context fields and deterministic runtime
// tool observations; it never scans request prose.
func RuntimeArtifactContextActiveFromBus(ctx *BusContext) bool {
	if ctx == nil {
		return false
	}
	if strings.TrimSpace(ctx.AttachedLog) != "" || strings.TrimSpace(ctx.AttachedHitrace) != "" {
		return true
	}
	if ctx.Mutable == nil {
		return false
	}
	if ctx.Mutable.LogTriage() != nil || ctx.Mutable.PerfTrace() != nil {
		return true
	}
	return ctx.Mutable.TraceQueryRuntimeObservationCount() > 0
}

// RuntimeArtifactContextActiveFromAgent is the AgentContext equivalent of
// RuntimeArtifactContextActiveFromBus. It exists so prompt/policy helpers do
// not drift between BusContext and AgentContext plumbing.
func RuntimeArtifactContextActiveFromAgent(ctx *AgentContext) bool {
	if ctx == nil {
		return false
	}
	if strings.TrimSpace(ctx.AttachedLog) != "" || strings.TrimSpace(ctx.AttachedHitrace) != "" {
		return true
	}
	if ctx.LogTriage != nil || ctx.PerfTrace != nil {
		return true
	}
	if ctx.Mutable == nil {
		return false
	}
	if ctx.Mutable.LogTriage() != nil || ctx.Mutable.PerfTrace() != nil {
		return true
	}
	return ctx.Mutable.TraceQueryRuntimeObservationCount() > 0
}

// HasTypedCurrentSourceScopeRequest reports that an external-observation turn
// also carries a typed source-scope contract for current checkout evidence. The
// signal is intentionally narrow: it consumes only analyzer-emitted structured
// scope fields plus artifact/source policy state, never raw request prose,
// analyzer rationale text, or model-authored narrative.
func (rm RequestModel) HasTypedCurrentSourceScopeRequest() bool {
	if !rm.HasExternalOnlyRuntimeArtifact() && !rm.HasExternalObservationArtifactReference() {
		return false
	}
	if rm.ExternalObservationPolicy != nil && rm.ExternalObservationPolicy.ExcludesCurrentSource() {
		return false
	}
	if rm.SourceScopeProfile == nil {
		return false
	}
	scope := rm.SourceScopeProfile.RequestedScope
	if scope == "" || scope == SourceScopeUnknown || !scope.IsValid() {
		return false
	}
	if rm.ExternalObservationPolicy != nil &&
		rm.ExternalObservationPolicy.CurrentSourceMode == ExternalObservationCurrentSourceAllow {
		return true
	}
	if rm.sourceScopeHasCurrentRequestAnchor() {
		return true
	}
	return rm.hasRequiredCurrentKeyCodeDimension()
}

// HasCurrentSourceObligationSignal reports that the analyzer emitted a typed
// current-source/mechanism obligation that was later dropped from the soft
// presentation profile. Runtime/log/trace gates may consume this as a source
// lane obligation, but it carries no answer content and creates no citations.
func (rm RequestModel) HasCurrentSourceObligationSignal() bool {
	if rm.ExternalObservationPolicy != nil && rm.ExternalObservationPolicy.ExcludesCurrentSource() {
		return false
	}
	for _, signal := range rm.CurrentSourceObligationSignals {
		if signal.Active() {
			return true
		}
	}
	return false
}

// CurrentSourceLaneDecision is the typed, non-prose decision used by hard
// current-source gates. It separates "source analysis is allowed by default"
// from "source evidence is required before completion"; external observations
// can still be explored together with source, but runtime artifacts must not be
// mistaken for implementation files when no current-source anchor exists.
type CurrentSourceLaneDecision string

const (
	CurrentSourceLaneRequired        CurrentSourceLaneDecision = "required"
	CurrentSourceLaneAllowedOptional CurrentSourceLaneDecision = "allowed_optional"
	CurrentSourceLaneExcluded        CurrentSourceLaneDecision = "excluded"
	CurrentSourceLaneSatisfiedAbsent CurrentSourceLaneDecision = "satisfied_absent"
)

func (d CurrentSourceLaneDecision) RequiresCurrentSource() bool {
	return d == CurrentSourceLaneRequired
}

// CurrentSourceLaneDecision returns the precise source-lane posture for hard
// gates. It consumes only typed analyzer/runtime fields; it does not inspect
// raw user prose.
func (rm RequestModel) CurrentSourceLaneDecision() CurrentSourceLaneDecision {
	if rm.HasRuntimeArtifactCurrentVerificationAnchor() {
		return CurrentSourceLaneRequired
	}
	if rm.HasTypedCurrentSourceScopeRequest() {
		return CurrentSourceLaneRequired
	}
	if (rm.HasExternalOnlyRuntimeArtifact() || rm.HasExternalObservationArtifactReference()) &&
		rm.ExternalObservationAllowsCurrentSource() {
		return CurrentSourceLaneRequired
	}
	if rm.ExternalObservationPolicy != nil && rm.ExternalObservationPolicy.ExcludesCurrentSource() {
		return CurrentSourceLaneExcluded
	}
	if !rm.HasExternalOnlyRuntimeArtifact() && !rm.HasExternalObservationArtifactReference() {
		return CurrentSourceLaneRequired
	}
	return CurrentSourceLaneAllowedOptional
}

// HasRuntimeArtifactCurrentVerificationAnchor reports whether an external
// runtime artifact has a separate, typed current-checkout target strong enough
// to justify opening the current-source lane. The signal must come from
// structured analyzer fields; stack-frame labels from an unresolved external
// log are not enough by themselves.
func (rm RequestModel) HasRuntimeArtifactCurrentVerificationAnchor() bool {
	if !rm.HasExternalOnlyRuntimeArtifact() && !rm.HasExternalObservationArtifactReference() {
		return false
	}
	if rm.hasRuntimeArtifactDiagnosticMechanismBridge() {
		return true
	}
	if rm.CurrentSourceExplanationProfile != nil &&
		rm.CurrentSourceExplanationProfile.Active() &&
		rm.currentSourceExplanationHasCurrentSourceQuote() {
		return true
	}
	if rm.hasRequiredRuntimeCurrentSourceMechanismDimension() {
		return true
	}
	if rm.HasCurrentSourceObligationSignal() {
		return true
	}
	if rm.LogTriage != nil && len(rm.LogTriage.ResolvedFiles) > 0 {
		return true
	}
	if rm.PerfTrace != nil && len(rm.PerfTrace.ResolvedFiles) > 0 {
		return true
	}
	if rm.DiagnosticProfile.CurrentVersionCheck {
		for _, target := range rm.AnalyzerHints.ExactTargets {
			if strings.TrimSpace(target) != "" && !LooksLikeRuntimeArtifactPath(target) {
				return true
			}
		}
	}
	for _, target := range rm.AnalyzerHints.ExactTargets {
		if targetLooksLikeCurrentSourceAnchor(target) {
			return true
		}
	}
	for _, hint := range rm.AnalyzerHints.RequiredFileHints {
		if targetLooksLikeCurrentSourceAnchor(hint.Path) {
			return true
		}
	}
	if rm.RequestedAnswerDimensions != nil && rm.RequestedAnswerDimensions.Active() {
		for _, dim := range rm.RequestedAnswerDimensions.Dimensions {
			if dim.Role != RequestedAnswerDimensionCurrentKeyCode {
				continue
			}
			if rm.dimensionHasCurrentSourceAnchor(dim) {
				return true
			}
		}
	}
	return false
}

func (rm RequestModel) hasRuntimeArtifactDiagnosticMechanismBridge() bool {
	if rm.ExternalObservationPolicy != nil && rm.ExternalObservationPolicy.ExcludesCurrentSource() {
		return false
	}
	if !rm.DiagnosticProfile.CurrentVersionCheck ||
		!rm.Predicates.IsDiagnosticQuestion ||
		!rm.Predicates.IsCrossComponent {
		return false
	}
	switch rm.Intent {
	case IntentRootCause, IntentExplain:
		return true
	default:
		return false
	}
}

func (rm RequestModel) hasRequiredCurrentKeyCodeDimension() bool {
	if rm.RequestedAnswerDimensions == nil || !rm.RequestedAnswerDimensions.Active() {
		return false
	}
	for _, dim := range rm.RequestedAnswerDimensions.Dimensions {
		if dim.Required && dim.Role == RequestedAnswerDimensionCurrentKeyCode {
			return true
		}
	}
	return false
}

func (rm RequestModel) hasRequiredRuntimeCurrentSourceMechanismDimension() bool {
	if rm.RequestedAnswerDimensions == nil || !rm.RequestedAnswerDimensions.Active() {
		return false
	}
	if rm.ExternalObservationPolicy != nil && rm.ExternalObservationPolicy.ExcludesCurrentSource() {
		return false
	}
	for _, dim := range rm.RequestedAnswerDimensions.Dimensions {
		if !dim.Required {
			continue
		}
		switch dim.Role {
		case RequestedAnswerDimensionFunctionOrPurpose:
			if rm.dimensionHasCurrentSourceAnchor(dim) {
				return true
			}
		}
	}
	return false
}

func (rm RequestModel) sourceScopeHasCurrentRequestAnchor() bool {
	if rm.SourceScopeProfile == nil {
		return false
	}
	return len(rm.SourceScopeProfile.SourceQuotes) > 0
}

func (rm RequestModel) externalObservationDefaultArtifactOnly() bool {
	return rm.ExternalObservationPolicy != nil &&
		rm.ExternalObservationPolicy.ArtifactCitationsExternalOnly() &&
		rm.ExternalObservationPolicy.CurrentSourceMode != ExternalObservationCurrentSourceAllow
}

func (rm RequestModel) currentSourceExplanationHasCurrentSourceQuote() bool {
	artifactExternalOnly := rm.ExternalObservationPolicy != nil && rm.ExternalObservationPolicy.ArtifactCitationsExternalOnly()
	for _, quote := range rm.CurrentSourceExplanationProfile.SourceQuotes {
		if textCanRepresentCurrentSourceAnchor(quote, artifactExternalOnly) {
			return true
		}
	}
	return false
}

func (rm RequestModel) currentSourceExplanationHasPreciseCurrentSourceQuote() bool {
	if rm.CurrentSourceExplanationProfile == nil || !rm.CurrentSourceExplanationProfile.Active() {
		return false
	}
	for _, quote := range rm.CurrentSourceExplanationProfile.SourceQuotes {
		if targetLooksLikeCurrentSourceAnchor(quote) {
			return true
		}
		if loc, ok := ParseAnswerSourceLocationSurface(quote); ok && strings.TrimSpace(loc.File) != "" {
			return true
		}
	}
	return false
}

func (rm RequestModel) dimensionHasCurrentSourceAnchor(dim RequestedAnswerDimension) bool {
	artifactExternalOnly := rm.ExternalObservationPolicy != nil && rm.ExternalObservationPolicy.ArtifactCitationsExternalOnly()
	return textCanRepresentCurrentSourceAnchor(dim.SourceQuote, artifactExternalOnly) ||
		textCanRepresentCurrentSourceAnchor(dim.Label, artifactExternalOnly)
}

func textCanRepresentCurrentSourceAnchor(raw string, artifactExternalOnly bool) bool {
	s := strings.TrimSpace(raw)
	if s == "" {
		return false
	}
	if targetLooksLikeCurrentSourceAnchor(s) {
		return true
	}
	if artifactExternalOnly &&
		(looksLikeExternalObservationReference(s) || looksLikeExternalArtifactCoordinate(s)) {
		return false
	}
	return !LooksLikeRuntimeArtifactPath(s)
}

func targetLooksLikeCurrentSourceAnchor(raw string) bool {
	s := strings.TrimSpace(raw)
	if s == "" || LooksLikeRuntimeArtifactPath(s) {
		return false
	}
	return HasCodeOrConfigPathSuffix(s)
}

func looksLikeExternalObservationReference(raw string) bool {
	s := strings.TrimSpace(strings.ToLower(raw))
	if s == "" {
		return false
	}
	if LooksLikeRuntimeArtifactPath(s) {
		return true
	}
	return strings.Contains(s, "://")
}

var externalArtifactCoordinatePattern = regexp.MustCompile(`(?i)^\s*(?:line|row)\s*[:#]?\s*\d+\s*$|^\s*(?:第\s*)?\d+\s*行\s*$`)

func looksLikeExternalArtifactCoordinate(raw string) bool {
	return externalArtifactCoordinatePattern.MatchString(strings.TrimSpace(raw))
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
