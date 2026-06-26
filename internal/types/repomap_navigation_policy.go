package types

import (
	"fmt"
	"strings"
	"unicode"
)

// RepoMapNavigationRoute names a model-visible repo_map navigation route.
// It is advisory prompt data only. Hard gates must continue to read precise
// evidence/citation contracts, not these route hints.
type RepoMapNavigationRoute string

const (
	RepoMapNavigationRouteOverview        RepoMapNavigationRoute = "overview"
	RepoMapNavigationRouteTaskMap         RepoMapNavigationRoute = "task_map"
	RepoMapNavigationRouteFileMap         RepoMapNavigationRoute = "file_map"
	RepoMapNavigationRouteSourceInventory RepoMapNavigationRoute = "source_inventory"
	RepoMapNavigationRouteImplementers    RepoMapNavigationRoute = "implementers"
	RepoMapNavigationRouteRelationMap     RepoMapNavigationRoute = "relation_map"
	RepoMapNavigationRouteEditImpact      RepoMapNavigationRoute = "edit_impact"
	RepoMapNavigationRouteSemanticGraph   RepoMapNavigationRoute = "semantic_subgraph"
	RepoMapNavigationRouteCallPath        RepoMapNavigationRoute = "call_path"
)

// RepoMapNavigationPurpose describes why a route is useful for the typed
// request shape. The values are intentionally cross-language and do not encode
// Go-specific concepts.
type RepoMapNavigationPurpose string

const (
	RepoMapNavigationPurposeOrientation     RepoMapNavigationPurpose = "orientation"
	RepoMapNavigationPurposeInventory       RepoMapNavigationPurpose = "inventory"
	RepoMapNavigationPurposeRelation        RepoMapNavigationPurpose = "relation"
	RepoMapNavigationPurposeChangeImpact    RepoMapNavigationPurpose = "change_impact"
	RepoMapNavigationPurposeExternalCurrent RepoMapNavigationPurpose = "external_current_source"
	RepoMapNavigationPurposeScalarLookup    RepoMapNavigationPurpose = "scalar_lookup"
	RepoMapNavigationPurposeEntrypoint      RepoMapNavigationPurpose = "entrypoint_lookup"
	RepoMapNavigationPurposeLocalizationGap RepoMapNavigationPurpose = "localization_gap"
)

// RepoMapNavigationStep is one soft navigation recommendation. It is compiled
// from typed analyzer/request contracts and may be rendered to prompts or tool
// results. It is not an obligation and must not be used as a hard validator.
type RepoMapNavigationStep struct {
	Route        RepoMapNavigationRoute
	Purpose      RepoMapNavigationPurpose
	When         string
	Params       []string
	FollowUps    []RepoMapNavigationRoute
	RelationHint []TypedRelationKind
}

// RepoMapNavigationPolicy is the shared typed route plan for repo_map usage in
// explore-like stages. It consumes RequestModel / AnswerContract /
// ExploreLanePlan and never scans user prose or model prose.
type RepoMapNavigationPolicy struct {
	QueryTerms []string
	Partitions []RepoMapNavigationPartition
	Steps      []RepoMapNavigationStep
}

// RepoMapNavigationPartition preserves analyzer-authored sub-topic / bucket /
// lane boundaries for soft navigation. It helps parallel explorers avoid
// collapsing unrelated questions into one broad repo_map call.
type RepoMapNavigationPartition struct {
	Label      string
	QueryTerms []string
	Scopes     []string
}

func (p RepoMapNavigationPolicy) Empty() bool {
	return len(p.QueryTerms) == 0 && len(p.Partitions) == 0 && len(p.Steps) == 0
}

func (p RepoMapNavigationPolicy) HasRoute(route RepoMapNavigationRoute) bool {
	for _, step := range p.Steps {
		if step.Route == route {
			return true
		}
	}
	return false
}

func (p RepoMapNavigationPolicy) HasPurpose(purpose RepoMapNavigationPurpose) bool {
	for _, step := range p.Steps {
		if step.Purpose == purpose {
			return true
		}
	}
	return false
}

func (p RepoMapNavigationPolicy) QueryTermList(limit int) []string {
	if limit <= 0 || len(p.QueryTerms) <= limit {
		return append([]string(nil), p.QueryTerms...)
	}
	return append([]string(nil), p.QueryTerms[:limit]...)
}

// FirstHopStep returns the first repo_map lens that should lead a source-code
// navigation pass. It is prompt guidance only; source-inventory lanes have a
// separate bounded tool surface because their completion semantics are stricter.
func (p RepoMapNavigationPolicy) FirstHopStep() (RepoMapNavigationStep, bool) {
	if len(p.Steps) > 0 &&
		p.Steps[0].Route == RepoMapNavigationRouteSourceInventory &&
		p.Steps[0].Purpose == RepoMapNavigationPurposeInventory {
		return RepoMapNavigationStep{}, false
	}
	for _, step := range p.Steps {
		if step.Route == RepoMapNavigationRouteSourceInventory {
			continue
		}
		return step, true
	}
	return RepoMapNavigationStep{}, false
}

// RenderMarkdownHint renders the shared model-facing policy block. It is kept
// with the policy data type so explorer prompts and repo_map tool output do not
// drift into separate route taxonomies.
func (p RepoMapNavigationPolicy) RenderMarkdownHint(title, lead string) string {
	if p.Empty() {
		return ""
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Typed Repo Map Route Hints"
	}
	lead = strings.TrimSpace(lead)
	if lead == "" {
		lead = "These are soft navigation hints from the structured analysis result and exploration lane plan. They help choose the efficient repo_map lens; they are not read obligations and do not replace your judgment."
	}
	var b strings.Builder
	b.WriteString("### ")
	b.WriteString(title)
	b.WriteString("\n\n")
	b.WriteString(lead)
	b.WriteString("\n")
	if terms := p.QueryTermList(8); len(terms) > 0 {
		b.WriteString("- Prefer concise exact code surfaces as `query` before broad root maps: identifiers, files/modules/packages, routes, or config keys such as `")
		b.WriteString(strings.Join(terms, "`, `"))
		if len(p.QueryTerms) > len(terms) {
			fmt.Fprintf(&b, "` (+%d more)", len(p.QueryTerms)-len(terms))
		} else {
			b.WriteString("`")
		}
		b.WriteString(". Do not paste a natural-language sentence; omit broad glue words unless no more precise terms are available.\n")
	}
	if len(p.Partitions) > 0 {
		b.WriteString("- When sub-topics, user buckets, or evidence lanes are separate, keep repo_map calls partitioned instead of merging unrelated topics into one broad map:\n")
		hasScopedPartition := false
		for i, part := range p.Partitions {
			if i >= 5 {
				fmt.Fprintf(&b, "  - ... (+%d more partition(s))\n", len(p.Partitions)-i)
				break
			}
			b.WriteString("  - ")
			label := strings.TrimSpace(part.Label)
			if label == "" {
				label = "partition"
			}
			b.WriteString(label)
			if len(part.QueryTerms) > 0 {
				b.WriteString(" — query `")
				b.WriteString(strings.Join(part.QueryTerms, "`, `"))
				b.WriteString("`")
			}
			if len(part.Scopes) > 0 {
				hasScopedPartition = true
				b.WriteString(" — scope `")
				b.WriteString(strings.Join(part.Scopes, "`, `"))
				b.WriteString("`")
			}
			b.WriteString("\n")
		}
		if hasScopedPartition {
			b.WriteString("- For partition scopes that name active sub-repos, set repo_map `path` to that sub-repo first; keep `sources`, `scope`, `scopes`, `target_file`, and `entry_point` relative to the selected sub-repo.\n")
		}
	}
	if len(p.Steps) > 0 {
		b.WriteString("- Recommended repo_map lens order for this typed shape:\n")
		for _, step := range p.Steps {
			fmt.Fprintf(&b, "  - `view=\"%s\"`", step.Route)
			if step.When != "" {
				b.WriteString(" — ")
				b.WriteString(step.When)
			}
			if len(step.Params) > 0 {
				b.WriteString("; params: ")
				b.WriteString(strings.Join(step.Params, ", "))
			}
			if len(step.RelationHint) > 0 {
				b.WriteString("; relation_kinds candidates: ")
				var kinds []string
				for _, kind := range step.RelationHint {
					kinds = append(kinds, string(kind))
				}
				b.WriteString(strings.Join(kinds, ", "))
			}
			b.WriteString(".\n")
		}
	}
	b.WriteString("\n")
	return b.String()
}

// CompileRepoMapNavigationPolicy derives soft repo_map guidance from existing
// typed request lanes. It deliberately avoids RawRequest and model-authored
// prose so localized wording cannot become control flow.
func CompileRepoMapNavigationPolicy(rm RequestModel, contract *AnswerContract, lanes ExploreLanePlan) RepoMapNavigationPolicy {
	var p RepoMapNavigationPolicy
	p.QueryTerms = repoMapNavigationQueryTerms(rm)
	p.Partitions = repoMapNavigationPartitions(rm, lanes)
	add := func(step RepoMapNavigationStep) {
		if step.Route == "" {
			return
		}
		for _, existing := range p.Steps {
			if existing.Route == step.Route && existing.Purpose == step.Purpose {
				return
			}
		}
		p.Steps = append(p.Steps, step)
	}

	sourceInventoryFirst := repoMapNavigationNeedsSourceInventory(rm)
	if sourceInventoryFirst {
		add(RepoMapNavigationStep{
			Route:   RepoMapNavigationRouteSourceInventory,
			Purpose: RepoMapNavigationPurposeInventory,
			When: "for scoped member inventories, count/member checklists, routes, config keys, or candidate attributes; " +
				"when the requested members are the implementers of a named interface/trait/protocol, `view=\"" + string(RepoMapNavigationRouteImplementers) + "\"` with the interface name as `query` is the exhaustive alternative lens for this same first hop",
			Params: []string{"scope", "scopes", "roles", "include_attributes=false", "attribute_roles after narrowing", "cursor/offset for paging"},
		})
	}

	if len(p.QueryTerms) > 0 && !sourceInventoryFirst {
		add(RepoMapNavigationStep{
			Route:   RepoMapNavigationRouteTaskMap,
			Purpose: RepoMapNavigationPurposeOrientation,
			When:    "when typed entities or target terms are already known",
			Params:  []string{"query", "top_n"},
		})
		add(RepoMapNavigationStep{
			Route:   RepoMapNavigationRouteFileMap,
			Purpose: RepoMapNavigationPurposeOrientation,
			When:    "when you need symbols grouped by selected files/directories",
			Params:  []string{"path", "top_n"},
		})
	}

	relationKinds := TypedRelationKindsForRequest(rm, TypedRelationPurposePromptHint)
	if repoMapNavigationNeedsRelationMap(rm, relationKinds) {
		callChain := repoMapNavigationCallChainShape(rm)
		followUps := []RepoMapNavigationRoute{RepoMapNavigationRouteTaskMap, RepoMapNavigationRouteFileMap}
		if callChain {
			followUps = append(followUps, RepoMapNavigationRouteCallPath)
		}
		add(RepoMapNavigationStep{
			Route:        RepoMapNavigationRouteRelationMap,
			Purpose:      RepoMapNavigationPurposeRelation,
			When:         "for calls, imports, inheritance, implements, references, registrations, routes, or flow edges around chosen sources/scopes",
			Params:       []string{"sources", "scope", "scopes", "relation_kinds", "query"},
			FollowUps:    followUps,
			RelationHint: relationKinds,
		})
		if callChain {
			add(repoMapNavigationCallPathStep())
		}
	}

	if rm.ChangeImpactProfile != nil && rm.ChangeImpactProfile.Active() {
		add(RepoMapNavigationStep{
			Route:   RepoMapNavigationRouteEditImpact,
			Purpose: RepoMapNavigationPurposeChangeImpact,
			When:    "when the typed request asks what a target change would affect",
			Params:  []string{"target_file", "query"},
		})
	}

	if repoMapNavigationNeedsExternalCurrentSource(rm, contract, lanes) {
		add(RepoMapNavigationStep{
			Route:   RepoMapNavigationRouteTaskMap,
			Purpose: RepoMapNavigationPurposeExternalCurrent,
			When:    "after VCS/log/trace/command/MCP/web evidence identifies current-source terms that need code verification",
			Params:  []string{"query"},
		})
	}

	if repoMapNavigationNeedsScalarLookup(rm) {
		add(RepoMapNavigationStep{
			Route:   RepoMapNavigationRouteTaskMap,
			Purpose: RepoMapNavigationPurposeScalarLookup,
			When:    "for exact scalar/config/return-value or role-location lookups before reading the selected file",
			Params:  []string{"query"},
		})
	}

	if repoMapNavigationNeedsEntrypointLookup(rm) {
		add(RepoMapNavigationStep{
			Route:   RepoMapNavigationRouteFileMap,
			Purpose: RepoMapNavigationPurposeEntrypoint,
			When:    "for entry/startup/handler role-location questions; discover framework routes, manifests, registrations, lifecycle callbacks, command roots, and generated-adjacent entry surfaces before assuming a literal main function",
			Params:  []string{"path", "query", "top_n"},
			FollowUps: []RepoMapNavigationRoute{
				RepoMapNavigationRouteTaskMap,
				RepoMapNavigationRouteRelationMap,
			},
		})
	}

	if !sourceInventoryFirst && (rm.Scenario == ScenarioArchitectureExplain ||
		(rm.DiagramHint != nil && rm.DiagramHint.Kind == DiagramArchitecture)) {
		add(RepoMapNavigationStep{
			Route:   RepoMapNavigationRouteSemanticGraph,
			Purpose: RepoMapNavigationPurposeOrientation,
			When:    "for module/component topology context — hub files, bridges, linear chains — when an architecture overview is requested",
			Params:  []string{"path", "top_n"},
		})
	}

	if rm.DiagramHint != nil && rm.DiagramHint.Kind == DiagramCallDAG {
		add(repoMapNavigationCallPathStep())
	}

	return p
}

// RepoMapNavigationPolicyWithLocalizationReview adds repo_map lenses required
// by typed source-localization gaps observed after initial analysis. It reads
// SourceLocalizationReview/LocalizationRequirementSet only; user prose, model
// rationale, and rendered summaries never participate.
func RepoMapNavigationPolicyWithLocalizationReview(policy RepoMapNavigationPolicy, review *SourceLocalizationReview) RepoMapNavigationPolicy {
	if review == nil {
		return policy
	}
	requirements := LocalizationRequirementsFromSourceLocalizationReview(*review, 0)
	if requirements.OpenItems == 0 || len(requirements.MissingOwnerAnchorPaths) == 0 {
		return policy
	}
	add := func(step RepoMapNavigationStep) {
		if step.Route == "" {
			return
		}
		for _, existing := range policy.Steps {
			if existing.Route == step.Route && existing.Purpose == step.Purpose {
				return
			}
		}
		policy.Steps = append(policy.Steps, step)
	}
	add(RepoMapNavigationStep{
		Route:   RepoMapNavigationRouteFileMap,
		Purpose: RepoMapNavigationPurposeLocalizationGap,
		When:    "for typed source-localization gaps where a production path was observed but no owner anchor has been established",
		Params:  []string{"path", "top_n"},
	})
	add(RepoMapNavigationStep{
		Route:     RepoMapNavigationRouteRelationMap,
		Purpose:   RepoMapNavigationPurposeLocalizationGap,
		When:      "for typed owner-localization gaps around observed production paths; use it to find nearby callers, implementers, registrations, or ownership edges before planning",
		Params:    []string{"sources", "scope", "scopes", "relation_kinds", "query"},
		FollowUps: []RepoMapNavigationRoute{RepoMapNavigationRouteFileMap},
	})
	return policy
}

// repoMapNavigationCallPathStep is the shared call_path soft route. It is
// added both for call-chain-shaped requests (as the follow-up to relation_map)
// and for call-DAG diagram hints; the add() dedupe keeps it single.
func repoMapNavigationCallPathStep() RepoMapNavigationStep {
	return RepoMapNavigationStep{
		Route:   RepoMapNavigationRouteCallPath,
		Purpose: RepoMapNavigationPurposeRelation,
		When:    "only after a concrete entry point is known and a path trace is needed",
		Params:  []string{"entry_point"},
	}
}

// repoMapNavigationCallChainShape reports whether the typed request signals
// describe a call-chain shape. It reads only the already-computed precise
// fields that drive the relation_map trigger — never user or model prose.
func repoMapNavigationCallChainShape(rm RequestModel) bool {
	if NormalizeRequirementKind(rm.AnalyzerHints.Kind) == ReqCallChain {
		return true
	}
	return rm.Intent == IntentTrace || rm.PredicateAxis == AxisCall
}

func repoMapNavigationNeedsSourceInventory(rm RequestModel) bool {
	if rm.SourceInventoryProfile != nil && rm.SourceInventoryProfile.Active() {
		return true
	}
	if RequiresExhaustiveEnumerationMemberSetHandoff(rm) ||
		HasBoundedCategoryEnumerationMembers(rm) ||
		rm.Predicates.IsCategoryEnumeration ||
		rm.Predicates.IsCountQuestion ||
		rm.Intent == IntentEnumerate {
		return true
	}
	return false
}

func repoMapNavigationNeedsRelationMap(rm RequestModel, relationKinds []TypedRelationKind) bool {
	if len(relationKinds) > 0 {
		return true
	}
	kind := NormalizeRequirementKind(rm.AnalyzerHints.Kind)
	if kind == ReqCallChain || kind == ReqRegistration {
		return true
	}
	if rm.Intent == IntentTrace || rm.PredicateAxis == AxisCall || rm.PredicateAxis == AxisRegister || rm.PredicateAxis == AxisImplement {
		return true
	}
	if rm.DiagramHint != nil {
		switch rm.DiagramHint.Kind {
		case DiagramFlow, DiagramSequence, DiagramCallDAG:
			return true
		}
	}
	if SourceInventoryProfileConflictsWithRelationFlow(rm) {
		return true
	}
	return false
}

func repoMapNavigationNeedsExternalCurrentSource(rm RequestModel, contract *AnswerContract, lanes ExploreLanePlan) bool {
	if rm.CurrentSourceExplanationProfile != nil && rm.CurrentSourceExplanationProfile.Active() {
		return true
	}
	if rm.HasRuntimeArtifactCurrentVerificationAnchor() {
		return true
	}
	if !lanes.Empty() {
		hasExternal := false
		hasCurrent := false
		for _, lane := range lanes.Lanes {
			if lane.Origin == AnswerEvidenceOriginCurrentSource {
				hasCurrent = true
			} else if AnswerEvidenceOriginCarriesOriginSpecificSupport(lane.Origin) {
				hasExternal = true
			}
		}
		if hasExternal && hasCurrent {
			return true
		}
	}
	if contract == nil {
		return false
	}
	intent := CompileAnswerIntentContract(rm, contract)
	if !intent.HasOrigin(AnswerEvidenceOriginCurrentSource) {
		return false
	}
	for _, origin := range intent.Origins {
		if origin != AnswerEvidenceOriginCurrentSource && AnswerEvidenceOriginCarriesOriginSpecificSupport(origin) {
			return true
		}
	}
	return false
}

func repoMapNavigationNeedsScalarLookup(rm RequestModel) bool {
	if rm.Predicates.IsCountQuestion {
		return false
	}
	if repoMapNavigationNeedsEntrypointLookup(rm) {
		return true
	}
	if rm.Intent == IntentReturnValue || rm.PredicateAxis == AxisReturn || rm.PredicateAxis == AxisConfigure {
		return true
	}
	switch rm.AnswerSubject.Kind {
	case SubjectReturnValue, SubjectStringLiteral, SubjectNumeric, SubjectEnumValue, SubjectConfigKey, SubjectFilePath:
		return true
	default:
		return false
	}
}

func repoMapNavigationNeedsEntrypointLookup(rm RequestModel) bool {
	if !rm.Predicates.IsRoleLocateLookup {
		return false
	}
	switch rm.AnswerSubject.Kind {
	case SubjectFunctionName, SubjectHandlerRoute:
		return true
	default:
		return false
	}
}

func repoMapNavigationQueryTerms(rm RequestModel) []string {
	var preferred []string
	var fallback []string
	add := func(value string, trusted bool) {
		addRepoMapNavigationTerm(&fallback, value)
		if trusted || repoMapNavigationTermLooksPrecise(value) {
			addRepoMapNavigationTerm(&preferred, value)
		}
	}
	addAll := func(values []string, trusted bool) {
		for _, value := range values {
			add(value, trusted)
		}
	}
	addAll(rm.AnalyzerHints.ExactTargets, true)
	addAll(rm.AnalyzerHints.MentionedEntities, false)
	addAll(rm.AnalyzerHints.PrimaryEntities, false)
	addAll(rm.AnalyzerHints.Entities, false)
	addAll(rm.AnalyzerHints.Keywords, false)
	if rm.CurrentSourceExplanationProfile != nil && rm.CurrentSourceExplanationProfile.Active() {
		addAll(rm.CurrentSourceExplanationProfile.TargetTerms, true)
	}
	if rm.ChangeImpactProfile != nil && rm.ChangeImpactProfile.Active() {
		add(rm.ChangeImpactProfile.Target, true)
	}
	if rm.FieldValueProfile != nil && rm.FieldValueProfile.Active() {
		add(rm.FieldValueProfile.Target, true)
		add(rm.FieldValueProfile.Owner, true)
		add(rm.FieldValueProfile.Field, true)
		add(rm.FieldValueProfile.Literal, true)
	}
	out := preferred
	if len(out) == 0 {
		out = fallback
	}
	if len(out) > 16 {
		out = out[:16]
	}
	return out
}

func repoMapNavigationPartitions(rm RequestModel, lanes ExploreLanePlan) []RepoMapNavigationPartition {
	var out []RepoMapNavigationPartition
	if scopes := repoMapNavigationTermsFromValues(rm.AnalyzerHints.PrimaryScopes); len(scopes) > 0 {
		out = append(out, RepoMapNavigationPartition{Label: "active sub-repo scope", Scopes: scopes})
	}
	for _, topic := range rm.SubTopics {
		label := strings.TrimSpace(topic.Summary)
		terms := repoMapNavigationTermsFromValues(topic.Entities)
		scopes := repoMapNavigationTermsFromValues(topic.Scopes)
		if label == "" && len(terms) == 0 && len(scopes) == 0 {
			continue
		}
		out = append(out, RepoMapNavigationPartition{Label: label, QueryTerms: terms, Scopes: scopes})
	}
	for _, bucket := range rm.QuestionStructure().Buckets {
		label := strings.TrimSpace(bucket.Label)
		terms := repoMapNavigationTermsFromValues(bucket.Anchors)
		if label == "" && len(terms) == 0 {
			continue
		}
		out = append(out, RepoMapNavigationPartition{Label: label, QueryTerms: terms})
	}
	for _, lane := range lanes.Lanes {
		label := strings.TrimSpace(lane.Label)
		terms := append([]string{}, lane.DimensionLabels...)
		if lane.InvestigationUnitLabel != "" {
			terms = append(terms, lane.InvestigationUnitLabel)
		}
		terms = repoMapNavigationTermsFromValues(terms)
		if label == "" && len(terms) == 0 {
			continue
		}
		out = append(out, RepoMapNavigationPartition{Label: label, QueryTerms: terms})
	}
	return dedupeRepoMapNavigationPartitions(out)
}

func repoMapNavigationTermsFromValues(values []string) []string {
	var preferred []string
	var fallback []string
	for _, value := range values {
		addRepoMapNavigationTerm(&fallback, value)
		if repoMapNavigationTermLooksPrecise(value) {
			addRepoMapNavigationTerm(&preferred, value)
		}
	}
	if len(preferred) > 0 {
		return preferred
	}
	return fallback
}

func dedupeRepoMapNavigationPartitions(in []RepoMapNavigationPartition) []RepoMapNavigationPartition {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]RepoMapNavigationPartition, 0, len(in))
	for _, part := range in {
		key := strings.ToLower(strings.TrimSpace(part.Label)) + "\x00" +
			strings.ToLower(strings.Join(part.QueryTerms, "|")) + "\x00" +
			strings.ToLower(strings.Join(part.Scopes, "|"))
		if key == "\x00\x00" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, part)
	}
	if len(out) > 8 {
		out = out[:8]
	}
	return out
}

func addRepoMapNavigationTerm(out *[]string, raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return
	}
	key := strings.ToLower(raw)
	for _, existing := range *out {
		if strings.ToLower(existing) == key {
			return
		}
	}
	*out = append(*out, raw)
}

// repoMapNavigationTermLooksPrecise is a soft prompt-shaping helper only. It
// decides which analyzer-provided terms are worth showing first as repo_map
// query suggestions; it must not become an evidence gate or proof of relevance.
func repoMapNavigationTermLooksPrecise(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.Contains(raw, " ") || strings.Contains(raw, "\t") || strings.Contains(raw, "\n") {
		return false
	}
	if strings.ContainsAny(raw, "_./:\\-#@$[]{}()<>") {
		return true
	}
	hasLower := false
	hasUpper := false
	letterCount := 0
	for _, r := range raw {
		switch {
		case unicode.IsDigit(r):
			return true
		case unicode.IsLetter(r):
			letterCount++
			if unicode.IsLower(r) {
				hasLower = true
			}
			if unicode.IsUpper(r) {
				hasUpper = true
			}
		}
	}
	if hasLower && hasUpper {
		return true
	}
	return hasUpper && letterCount >= 2
}
