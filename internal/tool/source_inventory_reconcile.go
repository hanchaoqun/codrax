package tool

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

type sourceInventoryCandidate struct {
	member     string
	key        string
	supportRef string
	note       string
	role       types.AnswerCandidateRole
	exported   bool
	file       string
	line       int
	language   string
	attributes []sourceInventoryCandidate
}

type sourceInventoryCandidateSet struct {
	role       types.AnswerCandidateRole
	candidates []sourceInventoryCandidate
	complete   bool
}

type sourceInventoryPackageBucket struct {
	dir       string
	files     int
	symbols   int
	languages map[string]int
	packages  map[string]int
}

func publishSourceInventoryAdvisory(ctx *types.BusContext, facts []types.AnswerAggregateFact, evidence []types.EvidenceItem) {
	if ctx == nil || ctx.Mutable == nil {
		return
	}
	advisory := buildSourceInventoryAdvisory(ctx, facts, evidence)
	if advisory.IsActive() {
		ctx.Mutable.SetSourceInventoryAdvisory(advisory)
		return
	}
	ctx.Mutable.ClearSourceInventoryAdvisory()
}

// PublishSourceInventoryAdvisoryFromTypedRequest publishes a pre-completion
// source-inventory checklist from the already-typed request model and repo-map
// graph. It is deliberately advisory-only: this is retrieval/navigation
// context for explorer lanes, not answer text and not a completion signal.
func PublishSourceInventoryAdvisoryFromTypedRequest(ctx *types.BusContext) bool {
	return publishSourceInventoryAdvisorySnapshot(ctx, "pre_explore_typed_request", true)
}

// PublishSourceInventoryAdvisoryFromToolObservation publishes the same
// advisory from a successful navigation observation. The returned string is a
// compact model-visible hint for the current ReAct dispatch. Empty means either
// no bounded source-inventory profile is active, no graph is available, or this
// active advisory lifecycle already attached the compact hint to a tool result.
func PublishSourceInventoryAdvisoryFromToolObservation(ctx *types.BusContext, result types.ToolResult) string {
	if ctx == nil || ctx.Mutable == nil || !result.Success || !sourceInventoryObservationTool(result.ToolName) {
		return ""
	}
	if !publishSourceInventoryAdvisorySnapshot(ctx, "tool:"+types.CanonicalToolName(result.ToolName), true) {
		return ""
	}
	if !ctx.Mutable.ClaimSourceInventoryAdvisoryHint() {
		return ""
	}
	return renderSourceInventoryAdvisoryToolHint(ctx.Mutable.SourceInventoryAdvisory())
}

func publishSourceInventoryAdvisorySnapshot(ctx *types.BusContext, provenance string, advisoryOnly bool) bool {
	if ctx == nil || ctx.Mutable == nil {
		return false
	}
	advisory := buildSourceInventoryAdvisory(ctx, nil, nil)
	if !advisory.IsActive() {
		return false
	}
	if advisoryOnly {
		advisory.AdvisoryOnly = true
	}
	advisory.Provenance = sourceInventoryAdvisoryAppendProvenance(advisory.Provenance, provenance)
	if current := ctx.Mutable.SourceInventoryAdvisory(); current.IsActive() {
		advisory = types.MergeSourceInventoryAdvisory(current, advisory)
	}
	ctx.Mutable.SetSourceInventoryAdvisory(advisory)
	return true
}

func sourceInventoryObservationTool(name string) bool {
	switch types.CanonicalToolName(name) {
	case "repo_map", "list_files":
		return true
	default:
		return false
	}
}

func sourceInventoryAdvisoryAppendProvenance(items []string, extra ...string) []string {
	out := append([]string(nil), items...)
	seen := make(map[string]bool, len(out)+len(extra))
	for _, item := range out {
		if key := strings.TrimSpace(item); key != "" {
			seen[key] = true
		}
	}
	for _, item := range extra {
		key := strings.TrimSpace(item)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
}

func renderSourceInventoryAdvisoryToolHint(advisory types.SourceInventoryAdvisory) string {
	if !advisory.IsActive() {
		return ""
	}
	const maxRows = 24
	var b strings.Builder
	b.WriteString("Structured source-inventory candidate checklist (advisory only, not final answer text):\n")
	if len(advisory.Scopes) > 0 {
		fmt.Fprintf(&b, "- scoped to: %s\n", strings.Join(advisory.Scopes, ", "))
	}
	b.WriteString("- reuse this checklist to avoid re-listing the same scope; verify/read unresolved rows before emitting evidence or aggregate_facts.\n")
	emitted := 0
	total := 0
	for _, set := range advisory.Sets {
		total += len(set.Candidates)
	}
	for _, set := range advisory.Sets {
		if emitted >= maxRows || len(set.Candidates) == 0 {
			continue
		}
		fmt.Fprintf(&b, "- %s candidates:", types.SourceInventoryAdvisoryRoleLabel(set.Role))
		for _, candidate := range set.Candidates {
			if emitted >= maxRows {
				break
			}
			member := strings.TrimSpace(candidate.Member)
			if member == "" {
				continue
			}
			if candidate.File != "" && candidate.Line > 0 {
				fmt.Fprintf(&b, " %s@%s:%d;", member, candidate.File, candidate.Line)
			} else if candidate.File != "" {
				fmt.Fprintf(&b, " %s@%s;", member, candidate.File)
			} else {
				fmt.Fprintf(&b, " %s;", member)
			}
			if attrs := renderSourceInventoryAdvisoryToolHintAttributes(candidate.Attributes); attrs != "" {
				fmt.Fprintf(&b, " {%s}", attrs)
			}
			emitted++
		}
		b.WriteByte('\n')
	}
	if total > emitted {
		fmt.Fprintf(&b, "- showing %d of %d candidates; keep using typed evidence for any hidden rows that matter.\n", emitted, total)
	}
	return strings.TrimSpace(b.String())
}

func renderSourceInventoryAdvisoryToolHintAttributes(attrs []types.SourceInventoryAdvisoryAttribute) string {
	if len(attrs) == 0 {
		return ""
	}
	const max = 3
	var parts []string
	for _, attr := range attrs {
		if len(parts) >= max {
			break
		}
		member := strings.TrimSpace(attr.Member)
		if member == "" {
			continue
		}
		loc := strings.TrimSpace(attr.File)
		if loc != "" && attr.Line > 0 {
			loc = loc + ":" + strconvItoa(attr.Line)
		}
		if loc != "" {
			parts = append(parts, member+"@"+loc)
		} else {
			parts = append(parts, member)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	if len(attrs) > len(parts) {
		parts = append(parts, fmt.Sprintf("+%d", len(attrs)-len(parts)))
	}
	return "related callables: " + strings.Join(parts, ", ")
}

func buildSourceInventoryAdvisory(ctx *types.BusContext, facts []types.AnswerAggregateFact, evidence []types.EvidenceItem) types.SourceInventoryAdvisory {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.Mutable == nil {
		return types.SourceInventoryAdvisory{}
	}
	graph, _ := ctx.Mutable.SearchGraph().(*repotypes.Graph)
	if graph == nil {
		return types.SourceInventoryAdvisory{}
	}
	profile, advisoryOnly, provenance := sourceInventoryAdvisoryProfile(ctx, graph)
	if !profile.Active() {
		return types.SourceInventoryAdvisory{}
	}
	scopes := sourceInventoryScopes(ctx, graph, facts)
	if len(scopes) == 0 {
		return types.SourceInventoryAdvisory{}
	}
	sets := sourceInventoryCandidateSets(ctx, graph, scopes, profile)
	if len(sets) == 0 {
		return types.SourceInventoryAdvisory{}
	}
	roles := make([]types.AnswerCandidateRole, 0, len(sets))
	for role := range sets {
		roles = append(roles, role)
	}
	sort.Slice(roles, func(i, j int) bool { return string(roles[i]) < string(roles[j]) })
	advisory := types.SourceInventoryAdvisory{
		Active:       true,
		AdvisoryOnly: advisoryOnly,
		Complete:     true,
		Scopes:       append([]string(nil), scopes...),
		Provenance:   sourceInventoryAdvisoryProvenance(ctx, provenance),
	}
	for _, role := range roles {
		set := sets[role]
		if len(set.candidates) == 0 {
			continue
		}
		if !set.complete {
			advisory.Complete = false
		}
		advisorySet := types.SourceInventoryAdvisorySet{
			Role:       set.role,
			Complete:   set.complete,
			Candidates: make([]types.SourceInventoryAdvisoryCandidate, 0, len(set.candidates)),
		}
		for _, candidate := range set.candidates {
			attrs := make([]types.SourceInventoryAdvisoryAttribute, 0, len(candidate.attributes))
			for _, attr := range candidate.attributes {
				attrs = append(attrs, types.SourceInventoryAdvisoryAttribute{
					Member:     attr.member,
					Key:        attr.key,
					SupportRef: attr.supportRef,
					Note:       sourceInventoryCandidateNote(attr),
					Role:       attr.role,
					Exported:   attr.exported,
					File:       attr.file,
					Line:       attr.line,
					Language:   attr.language,
				})
			}
			advisorySet.Candidates = append(advisorySet.Candidates, types.SourceInventoryAdvisoryCandidate{
				Member:     candidate.member,
				Key:        candidate.key,
				SupportRef: candidate.supportRef,
				Note:       sourceInventoryCandidateNote(candidate),
				Role:       candidate.role,
				Exported:   candidate.exported,
				File:       candidate.file,
				Line:       candidate.line,
				Language:   candidate.language,
				Attributes: attrs,
			})
		}
		advisory.Sets = append(advisory.Sets, advisorySet)
	}
	if len(advisory.Sets) == 0 {
		return types.SourceInventoryAdvisory{}
	}
	return advisory
}

func sourceInventoryAdvisoryProfile(ctx *types.BusContext, graph *repotypes.Graph) (*types.SourceInventoryProfile, bool, []string) {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil, false, nil
	}
	rm := ctx.AnalysisIR.RequestModel
	if profile := rm.SourceInventoryProfile; profile != nil && profile.Active() {
		clone := *profile
		clone.TargetRoles = append([]types.AnswerCandidateRole(nil), profile.TargetRoles...)
		clone.RequestedFields = append([]types.SourceInventoryRequestedField(nil), profile.RequestedFields...)
		clone.SourceQuotes = append([]string(nil), profile.SourceQuotes...)
		if profile.Confidence < 0.70 {
			return &clone, true, []string{"source_inventory_profile:low_confidence"}
		}
		return &clone, false, []string{"source_inventory_profile"}
	}
	var roleProfileRoles []types.AnswerCandidateRole
	if rm.AnswerRoleProfile != nil && rm.AnswerRoleProfile.Active() {
		roleProfileRoles = sourceInventoryAdvisoryPrincipalRolesFromRoleProfile(rm.AnswerRoleProfile)
		var provenance []string
		switch {
		case types.HasAttributeBearingEnumeration(rm):
			provenance = append(provenance, "request_traits:attribute_bearing_enumeration")
		case types.RequiresExhaustiveEnumerationMemberSetHandoff(rm):
			provenance = append(provenance, "request_traits:exhaustive_member_set")
		default:
			provenance = nil
		}
		if len(provenance) > 0 && len(roleProfileRoles) > 0 {
			provenance = append(provenance, "answer_role_profile")
			return &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles:       roleProfileRoles,
				RequestedFields: []types.SourceInventoryRequestedField{
					types.SourceInventoryFieldName,
					types.SourceInventoryFieldLocation,
					types.SourceInventoryFieldSummary,
				},
				Confidence: 0.50,
			}, true, provenance
		}
	}
	if types.HasBoundedSourceEnumerationScope(rm, ctx.AnalysisIR.EvidencePlan.RequiredFiles, ctx.RepoRoot) {
		roles := roleProfileRoles
		if len(roles) == 0 {
			roles = []types.AnswerCandidateRole{types.AnswerCandidateRolePackage}
		}
		return &types.SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       roles,
			RequestedFields: []types.SourceInventoryRequestedField{
				types.SourceInventoryFieldName,
				types.SourceInventoryFieldLocation,
				types.SourceInventoryFieldSummary,
			},
			Confidence: 0.45,
		}, true, []string{"request_traits:bounded_source_enumeration_scope"}
	}
	if sourceInventoryCanUseScopeEntityFallback(rm) && len(sourceInventoryRequestedScopes(ctx, graph)) > 0 {
		roles := roleProfileRoles
		if len(roles) == 0 {
			roles = []types.AnswerCandidateRole{types.AnswerCandidateRolePackage}
		}
		return &types.SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       roles,
			RequestedFields: []types.SourceInventoryRequestedField{
				types.SourceInventoryFieldName,
				types.SourceInventoryFieldLocation,
				types.SourceInventoryFieldSummary,
			},
			Confidence: 0.40,
		}, true, []string{"request_traits:source_scope_enumeration"}
	}
	return nil, false, nil
}

func sourceInventoryCanUseScopeEntityFallback(rm types.RequestModel) bool {
	if !types.IsTypedSourceEnumerationShape(rm) {
		return false
	}
	if rm.CompletenessObligation != nil && rm.CompletenessObligation.IsActive() {
		return false
	}
	return true
}

func sourceInventoryAdvisoryPrincipalRolesFromRoleProfile(profile *types.AnswerRoleProfile) []types.AnswerCandidateRole {
	if profile == nil || !profile.Active() {
		return nil
	}
	seen := map[types.AnswerCandidateRole]bool{}
	var roles []types.AnswerCandidateRole
	for _, role := range profile.RequiredCandidateRoles {
		if role == types.AnswerCandidateRoleUnknown || seen[role] {
			continue
		}
		switch role {
		case types.AnswerCandidateRoleFunction,
			types.AnswerCandidateRoleMethod,
			types.AnswerCandidateRoleType,
			types.AnswerCandidateRoleConstant,
			types.AnswerCandidateRoleVariable,
			types.AnswerCandidateRoleField,
			types.AnswerCandidateRolePackage,
			types.AnswerCandidateRoleFile,
			types.AnswerCandidateRoleConfigKey,
			types.AnswerCandidateRoleRoute,
			types.AnswerCandidateRoleImportPath:
			seen[role] = true
			roles = append(roles, role)
		}
	}
	return roles
}

func sourceInventoryAdvisoryProvenance(ctx *types.BusContext, base []string) []string {
	out := append([]string(nil), base...)
	out = append(out, "repomap_graph")
	if sourceInventoryAdvisoryHasListFilesScope(ctx) {
		out = append(out, "tool:list_files")
	}
	return out
}

func sourceInventoryAdvisoryHasListFilesScope(ctx *types.BusContext) bool {
	for _, result := range aggregateSupportToolResults(ctx) {
		if result.Success && result.ToolName == "list_files" && sourceInventoryListFilesScope(result.Summary) != "" {
			return true
		}
	}
	return false
}

func reconcileCompletionAggregateFactsWithSourceInventory(ctx *types.BusContext, facts []types.AnswerAggregateFact, evidence []types.EvidenceItem) []types.AnswerAggregateFact {
	if len(facts) == 0 || ctx == nil || ctx.AnalysisIR == nil || ctx.Mutable == nil {
		return facts
	}
	profile := ctx.AnalysisIR.RequestModel.SourceInventoryProfile
	if !profile.Active() || profile.Confidence < 0.70 {
		return facts
	}
	graph, _ := ctx.Mutable.SearchGraph().(*repotypes.Graph)
	if graph == nil {
		return facts
	}
	scopes := sourceInventoryScopes(ctx, graph, facts)
	if len(scopes) == 0 {
		return facts
	}
	sets := sourceInventoryCandidateSets(ctx, graph, scopes, profile)
	if len(sets) == 0 {
		return facts
	}
	families := newAggregateDeclarationFamilyLookup(ctx)
	out := cloneCompletionAggregateFacts(facts)
	changed := false
	for i := range out {
		if !types.AnswerAggregateFactCarriesCompleteMemberSet(out[i]) ||
			types.AnswerAggregateFactRoleForRequest(out[i], &ctx.AnalysisIR.RequestModel) != types.AnswerAggregateRolePrincipalAnswer ||
			len(out[i].Members) == 0 {
			continue
		}
		if sourceInventoryMustNotRewriteRelationMemberSet(ctx, out[i]) {
			continue
		}
		role, ok := sourceInventoryFactRole(ctx, out[i], evidence, profile, families)
		if !ok || !profile.RequiresPrincipalRole(role) {
			continue
		}
		set, ok := sets[role]
		if !ok || len(set.candidates) == 0 || !set.complete {
			continue
		}
		if !sourceInventoryMayRewriteMemberSet(profile, role) {
			continue
		}
		if !sourceInventoryShouldReplaceMemberSet(profile, role) {
			oldLen := len(out[i].Members)
			if sourceInventoryAppendMissingCandidates(&out[i], set.candidates) {
				reconcileAdjacentAggregateCountWithMemberSet(out, i, oldLen, len(out[i].Members))
				changed = true
			}
			continue
		}
		if sourceInventoryReplaceMemberSet(&out[i], set.candidates) {
			reconcileAdjacentAggregateCountWithMemberSet(out, i, len(facts[i].Members), len(out[i].Members))
			changed = true
		}
	}
	if !changed {
		return facts
	}
	return out
}

func sourceInventoryMustNotRewriteRelationMemberSet(ctx *types.BusContext, fact types.AnswerAggregateFact) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	if !types.HasTypedRelationMemberSetShape(ctx.AnalysisIR.RequestModel) {
		return false
	}
	// In relation-shaped questions the model-authored member_set is the
	// relation carrier. Source inventory remains useful evidence, but replacing
	// or appending principal members from "all symbols in the scoped file" can
	// turn precise implementers/callees into unrelated neighboring definitions.
	return types.AnswerAggregateFactRoleForRequest(fact, &ctx.AnalysisIR.RequestModel) == types.AnswerAggregateRolePrincipalAnswer
}

func sourceInventoryFactRole(ctx *types.BusContext, fact types.AnswerAggregateFact, evidence []types.EvidenceItem, profile *types.SourceInventoryProfile, families *aggregateDeclarationFamilyLookup) (types.AnswerCandidateRole, bool) {
	if profile != nil {
		principalRoles := profile.PrincipalTargetRoles()
		if len(principalRoles) == 1 {
			return principalRoles[0], true
		}
	}
	p, ok := aggregateMemberSetDefinitionProfile(ctx, fact, evidence, families)
	if !ok {
		return types.AnswerCandidateRoleUnknown, false
	}
	return p.role, true
}

func sourceInventoryMayRewriteMemberSet(profile *types.SourceInventoryProfile, role types.AnswerCandidateRole) bool {
	if profile == nil {
		return false
	}
	// The only source-inventory rewrite that is precise enough to alter the
	// model-authored member set is the language-structural "public string enum
	// type backed by const set" shape. Generic function/method/type inventories
	// can be useful evidence, but automatically replacing or appending their
	// full graph candidate set can broaden requests such as "one entry function
	// per package" into "all functions in scope". That violates the model/user
	// intent boundary, so those candidates stay as support unless the model
	// itself emits them.
	return role == types.AnswerCandidateRoleType &&
		profile.TypeUnderlying == types.SourceInventoryTypeUnderlyingString &&
		profile.RequiresConstSet
}

func sourceInventoryShouldReplaceMemberSet(profile *types.SourceInventoryProfile, role types.AnswerCandidateRole) bool {
	if profile == nil {
		return false
	}
	if role == types.AnswerCandidateRoleType &&
		profile.TypeUnderlying == types.SourceInventoryTypeUnderlyingString &&
		profile.RequiresConstSet {
		return true
	}
	return false
}

func sourceInventoryReplaceMemberSet(fact *types.AnswerAggregateFact, candidates []sourceInventoryCandidate) bool {
	if fact == nil || len(candidates) == 0 {
		return false
	}
	existingByKey := sourceInventoryExistingMemberCarriers(*fact)
	members := make([]string, 0, len(candidates))
	refs := make([]string, 0, len(candidates))
	notes := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		existing := existingByKey[candidate.key]
		supportRef := candidate.supportRef
		if strings.TrimSpace(supportRef) == "" {
			supportRef = existing.supportRef
		}
		note := sourceInventoryCandidateNote(candidate)
		if strings.TrimSpace(existing.note) != "" {
			note = types.MergeEvidenceSummaries(existing.note, note)
		}
		members = append(members, candidate.member)
		refs = append(refs, supportRef)
		notes = append(notes, note)
	}
	notes = trimTrailingEmptyStrings(notes)
	if stringSlicesEqual(fact.Members, members) && stringSlicesEqual(fact.SupportRefs, refs) && stringSlicesEqual(fact.MemberNotes, notes) {
		return false
	}
	fact.Members = members
	fact.SupportRefs = refs
	fact.MemberNotes = notes
	fact.Value = strconvItoa(len(members))
	fact.Provenance = sourceInventoryAppendProvenance(fact.Provenance)
	return true
}

type sourceInventoryExistingMemberCarrier struct {
	note       string
	supportRef string
}

func sourceInventoryExistingMemberCarriers(fact types.AnswerAggregateFact) map[string]sourceInventoryExistingMemberCarrier {
	out := map[string]sourceInventoryExistingMemberCarrier{}
	for idx, member := range fact.Members {
		key := aggregateMemberKey(member)
		if key == "" {
			continue
		}
		existing := out[key]
		if idx < len(fact.MemberNotes) {
			existing.note = types.MergeEvidenceSummaries(existing.note, fact.MemberNotes[idx])
		}
		if strings.TrimSpace(existing.supportRef) == "" && idx < len(fact.SupportRefs) {
			existing.supportRef = strings.TrimSpace(fact.SupportRefs[idx])
		}
		out[key] = existing
	}
	return out
}

func sourceInventoryAppendMissingCandidates(fact *types.AnswerAggregateFact, candidates []sourceInventoryCandidate) bool {
	if fact == nil || len(candidates) == 0 {
		return false
	}
	existing := make(map[string]bool, len(fact.Members))
	for _, member := range fact.Members {
		if key := aggregateMemberKey(member); key != "" {
			existing[key] = true
		}
	}
	changed := false
	for _, candidate := range candidates {
		if candidate.key == "" || existing[candidate.key] {
			continue
		}
		fact.Members = append(fact.Members, candidate.member)
		fact.SupportRefs = append(fact.SupportRefs, candidate.supportRef)
		if note := sourceInventoryCandidateNote(candidate); note != "" {
			fact.MemberNotes = appendStringAtIndex(fact.MemberNotes, len(fact.Members)-1, note)
		}
		existing[candidate.key] = true
		changed = true
	}
	if changed {
		fact.Value = strconvItoa(len(fact.Members))
		fact.Provenance = sourceInventoryAppendProvenance(fact.Provenance)
	}
	return changed
}

func sourceInventoryAppendProvenance(current string) string {
	current = strings.TrimSpace(current)
	const marker = "system:source_inventory"
	if current == "" {
		return marker
	}
	for _, part := range strings.Split(current, ",") {
		if strings.EqualFold(strings.TrimSpace(part), marker) {
			return current
		}
	}
	return current + ", " + marker
}

func sourceInventoryCandidateSets(ctx *types.BusContext, graph *repotypes.Graph, scopes []string, profile *types.SourceInventoryProfile) map[types.AnswerCandidateRole]sourceInventoryCandidateSet {
	out := map[types.AnswerCandidateRole]sourceInventoryCandidateSet{}
	for _, role := range profile.PrincipalTargetRoles() {
		switch {
		case role == types.AnswerCandidateRoleFile:
			out[role] = sourceInventoryFileCandidates(ctx, graph, scopes, profile)
		case role == types.AnswerCandidateRolePackage:
			out[role] = sourceInventoryPackageCandidates(ctx, graph, scopes, profile)
		case role == types.AnswerCandidateRoleType &&
			profile.TypeUnderlying == types.SourceInventoryTypeUnderlyingString &&
			profile.RequiresConstSet:
			out[role] = sourceInventoryGoStringEnumCandidates(ctx, graph, scopes, profile)
		default:
			out[role] = sourceInventoryGraphCandidates(ctx, graph, scopes, profile, role)
		}
		if len(out[role].candidates) == 0 {
			delete(out, role)
		}
	}
	return out
}

func sourceInventoryFileCandidates(ctx *types.BusContext, graph *repotypes.Graph, scopes []string, profile *types.SourceInventoryProfile) sourceInventoryCandidateSet {
	set := sourceInventoryCandidateSet{role: types.AnswerCandidateRoleFile, complete: sourceInventoryScopesHaveIndexedSourceFiles(graph, scopes)}
	if graph == nil {
		return set
	}
	seen := map[string]bool{}
	for _, fi := range sourceInventoryScopedGraphFiles(graph, scopes, "") {
		if fi == nil || fi.IsSpecial || strings.TrimSpace(fi.RelPath) == "" || strings.TrimSpace(fi.Language) == "" {
			continue
		}
		file := strings.Trim(strings.TrimSpace(strings.ReplaceAll(fi.RelPath, `\`, `/`)), "/")
		if file == "" || seen[file] || !aggregateEvidenceSourceInRequestedScope(ctx, file) {
			continue
		}
		seen[file] = true
		set.candidates = append(set.candidates, sourceInventoryCandidate{
			member:     file,
			key:        file,
			supportRef: file,
			note:       sourceInventoryFileCandidateNote(fi),
			role:       types.AnswerCandidateRoleFile,
			exported:   true,
			file:       file,
			language:   strings.TrimSpace(fi.Language),
		})
	}
	sourceInventorySortCandidates(set.candidates)
	return set
}

func sourceInventoryPackageCandidates(ctx *types.BusContext, graph *repotypes.Graph, scopes []string, profile *types.SourceInventoryProfile) sourceInventoryCandidateSet {
	set := sourceInventoryCandidateSet{role: types.AnswerCandidateRolePackage, complete: sourceInventoryScopesHaveIndexedSourceFiles(graph, scopes)}
	if graph == nil {
		return set
	}
	buckets := map[string]*sourceInventoryPackageBucket{}
	for _, fi := range sourceInventoryScopedGraphFiles(graph, scopes, "") {
		if fi == nil || fi.IsSpecial || strings.TrimSpace(fi.RelPath) == "" || strings.TrimSpace(fi.Language) == "" {
			continue
		}
		if !aggregateEvidenceSourceInRequestedScope(ctx, fi.RelPath) {
			continue
		}
		dir := strings.Trim(path.Dir(strings.ReplaceAll(fi.RelPath, `\`, `/`)), "/")
		if dir == "." || dir == "" {
			dir = strings.Trim(strings.TrimSpace(fi.Package), "/")
		}
		if dir == "" {
			continue
		}
		bucket := buckets[dir]
		if bucket == nil {
			bucket = &sourceInventoryPackageBucket{
				dir:       dir,
				languages: map[string]int{},
				packages:  map[string]int{},
			}
			buckets[dir] = bucket
		}
		bucket.files++
		bucket.symbols += len(fi.Symbols)
		if lang := strings.TrimSpace(fi.Language); lang != "" {
			bucket.languages[lang]++
		}
		if pkg := strings.TrimSpace(fi.Package); pkg != "" {
			bucket.packages[pkg]++
		}
	}
	for _, bucket := range buckets {
		member := sourceInventoryPackageBucketMember(bucket)
		key := strings.TrimSpace(bucket.dir)
		if member == "" || key == "" {
			continue
		}
		set.candidates = append(set.candidates, sourceInventoryCandidate{
			member:     member,
			key:        key,
			supportRef: key,
			note:       sourceInventoryPackageBucketNote(bucket),
			role:       types.AnswerCandidateRolePackage,
			exported:   true,
			file:       key,
			language:   sourceInventoryDominantMapKey(bucket.languages),
			attributes: sourceInventoryPackageBucketCallableAttributes(ctx, graph, key),
		})
	}
	sourceInventorySortCandidates(set.candidates)
	return set
}

const sourceInventoryMaxPackageAttributes = 4

func sourceInventoryPackageBucketCallableAttributes(ctx *types.BusContext, graph *repotypes.Graph, dir string) []sourceInventoryCandidate {
	if graph == nil {
		return nil
	}
	dir = strings.Trim(strings.TrimSpace(strings.ReplaceAll(dir, `\`, `/`)), "/")
	if dir == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []sourceInventoryCandidate
	for _, sym := range sourceInventoryGraphSymbols(graph) {
		if sym == nil || !sourceInventorySymbolInPackageDir(sym, dir) || !aggregateEvidenceSourceInRequestedScope(ctx, sym.File) {
			continue
		}
		role, ok := sourceInventoryCallableAttributeRole(sym)
		if !ok {
			continue
		}
		if ctx != nil && ctx.AnalysisIR != nil &&
			!sourceInventorySymbolMatchesVisibility(sym, ctx.AnalysisIR.RequestModel.AnswerVisibilityProfile) {
			continue
		}
		candidate := sourceInventoryCandidateForSymbol(sym, role, graph)
		key := string(role) + "\x00" + candidate.key + "\x00" + candidate.file + "\x00" + strconvItoa(candidate.line)
		if candidate.key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, candidate)
	}
	sourceInventorySortCallableAttributes(out)
	if len(out) > sourceInventoryMaxPackageAttributes {
		out = out[:sourceInventoryMaxPackageAttributes]
	}
	return out
}

func sourceInventorySymbolInPackageDir(sym *repotypes.Symbol, dir string) bool {
	if sym == nil {
		return false
	}
	file := strings.Trim(strings.TrimSpace(strings.ReplaceAll(sym.File, `\`, `/`)), "/")
	if file == "" || dir == "" {
		return false
	}
	return file == dir || strings.HasPrefix(file, dir+"/")
}

func sourceInventoryCallableAttributeRole(sym *repotypes.Symbol) (types.AnswerCandidateRole, bool) {
	if sym == nil {
		return types.AnswerCandidateRoleUnknown, false
	}
	switch strings.ToLower(strings.TrimSpace(sym.Kind)) {
	case "function", "foreign-func", "operator", "ui-entry", "builder", "rpc":
		return types.AnswerCandidateRoleFunction, true
	case "method", "ctor":
		return types.AnswerCandidateRoleMethod, true
	default:
		return types.AnswerCandidateRoleUnknown, false
	}
}

func sourceInventorySortCallableAttributes(candidates []sourceInventoryCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].exported != candidates[j].exported {
			return candidates[i].exported
		}
		if candidates[i].file != candidates[j].file {
			return candidates[i].file < candidates[j].file
		}
		if candidates[i].line != candidates[j].line {
			if candidates[i].line == 0 {
				return false
			}
			if candidates[j].line == 0 {
				return true
			}
			return candidates[i].line < candidates[j].line
		}
		return candidates[i].member < candidates[j].member
	})
}

func sourceInventoryPackageBucketMember(bucket *sourceInventoryPackageBucket) string {
	if bucket == nil {
		return ""
	}
	if pkg := sourceInventoryDominantMapKey(bucket.packages); pkg != "" {
		return pkg
	}
	return path.Base(strings.Trim(bucket.dir, "/"))
}

func sourceInventoryPackageBucketNote(bucket *sourceInventoryPackageBucket) string {
	if bucket == nil {
		return ""
	}
	parts := []string{"directory=" + bucket.dir}
	if lang := sourceInventoryDominantMapKey(bucket.languages); lang != "" {
		parts = append(parts, "language="+lang)
	}
	if bucket.files > 0 {
		parts = append(parts, fmt.Sprintf("files=%d", bucket.files))
	}
	if bucket.symbols > 0 {
		parts = append(parts, fmt.Sprintf("symbols=%d", bucket.symbols))
	}
	return strings.Join(parts, ", ")
}

func sourceInventoryDominantMapKey(counts map[string]int) string {
	if len(counts) == 0 {
		return ""
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		if strings.TrimSpace(key) != "" {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return ""
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] == counts[keys[j]] {
			return keys[i] < keys[j]
		}
		return counts[keys[i]] > counts[keys[j]]
	})
	return keys[0]
}

func sourceInventoryFileCandidateNote(fi *repotypes.FileInfo) string {
	if fi == nil {
		return ""
	}
	parts := make([]string, 0, 3)
	if language := strings.TrimSpace(fi.Language); language != "" {
		parts = append(parts, "language="+language)
	}
	if len(fi.Symbols) > 0 {
		parts = append(parts, fmt.Sprintf("symbols=%d", len(fi.Symbols)))
	}
	return strings.Join(parts, ", ")
}

func sourceInventoryGraphCandidates(ctx *types.BusContext, graph *repotypes.Graph, scopes []string, profile *types.SourceInventoryProfile, role types.AnswerCandidateRole) sourceInventoryCandidateSet {
	set := sourceInventoryCandidateSet{role: role, complete: sourceInventoryScopesHaveIndexedSourceFiles(graph, scopes)}
	if graph == nil {
		return set
	}
	excludedRoles := map[types.AnswerCandidateRole]bool{}
	if policy := ctx.AnalysisIR.RequestModel.AnswerExclusionPolicy; policy != nil && policy.Active() {
		excludedRoles = answerDocumentEffectiveExcludedRoleSet(ctx, policy, nil, nil)
	}
	seen := map[string]bool{}
	for _, sym := range sourceInventoryGraphSymbols(graph) {
		if sym == nil || !sourceInventoryFileInScopes(sym.File, scopes) || !aggregateEvidenceSourceInRequestedScope(ctx, sym.File) {
			continue
		}
		candidateRole, ok := aggregateAnswerCandidateRoleForSymbol(sym)
		if !ok || candidateRole != role || answerDocumentSymbolMatchesExcludedRole(sym, excludedRoles) {
			continue
		}
		if !sourceInventorySymbolMatchesVisibility(sym, ctx.AnalysisIR.RequestModel.AnswerVisibilityProfile) {
			continue
		}
		key := aggregateMemberKey(sym.Name)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		set.candidates = append(set.candidates, sourceInventoryCandidateForSymbol(sym, role, graph))
	}
	sourceInventorySortCandidates(set.candidates)
	return set
}

func sourceInventoryGoStringEnumCandidates(ctx *types.BusContext, graph *repotypes.Graph, scopes []string, profile *types.SourceInventoryProfile) sourceInventoryCandidateSet {
	set := sourceInventoryCandidateSet{role: types.AnswerCandidateRoleType, complete: true}
	if graph == nil || strings.TrimSpace(ctx.RepoRoot) == "" {
		set.complete = false
		return set
	}
	files := sourceInventoryScopedGraphFiles(graph, scopes, "go")
	if len(files) == 0 {
		set.complete = false
		return set
	}
	type typeInfo struct {
		file string
		line int
		note string
	}
	stringTypes := map[string]typeInfo{}
	constBacked := map[string]bool{}
	for _, fi := range files {
		if fi == nil || strings.TrimSpace(fi.RelPath) == "" {
			continue
		}
		full := filepath.Join(ctx.RepoRoot, filepath.FromSlash(fi.RelPath))
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, full, nil, parser.ParseComments)
		if err != nil {
			set.complete = false
			continue
		}
		for _, decl := range parsed.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			switch gen.Tok {
			case token.TYPE:
				for _, spec := range gen.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok || ts == nil || ts.Name == nil || !ts.Name.IsExported() {
						continue
					}
					if sourceInventoryGoExprSurface(ts.Type) != "string" {
						continue
					}
					pos := fset.Position(ts.Name.Pos())
					stringTypes[ts.Name.Name] = typeInfo{
						file: fi.RelPath,
						line: pos.Line,
						note: sourceInventoryGoCommentText(ts.Name.Name, ts.Doc, gen.Doc),
					}
				}
			case token.CONST:
				currentType := ""
				for _, spec := range gen.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok || vs == nil {
						continue
					}
					if vs.Type != nil {
						currentType = sourceInventoryGoExprSurface(vs.Type)
					}
					if _, ok := stringTypes[currentType]; !ok {
						continue
					}
					for _, name := range vs.Names {
						if name != nil {
							constBacked[currentType] = true
						}
					}
				}
			}
		}
	}
	seen := map[string]bool{}
	for name, info := range stringTypes {
		if !constBacked[name] {
			continue
		}
		sym := sourceInventoryGraphSymbolAt(graph, name, info.file, info.line)
		exported := ast.IsExported(name)
		if sym != nil {
			exported = sym.Exported
		}
		note := sourceInventoryCandidateNoteFromGraph(sym, sourceInventoryGraphLanguageForFile(graph, info.file))
		if note == "" {
			note = info.note
		}
		if !sourceInventoryVisibilityAllowsExported(exported, ctx.AnalysisIR.RequestModel.AnswerVisibilityProfile) {
			continue
		}
		key := aggregateMemberKey(name)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		set.candidates = append(set.candidates, sourceInventoryCandidate{
			member:     name,
			key:        key,
			supportRef: name + ": " + aggregateSupportLocationKey(info.file, info.line),
			note:       note,
			role:       types.AnswerCandidateRoleType,
			exported:   exported,
			file:       info.file,
			line:       info.line,
			language:   "go",
		})
	}
	sourceInventorySortCandidates(set.candidates)
	return set
}

func sourceInventoryGoExprSurface(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		if e.Sel != nil {
			return e.Sel.Name
		}
	}
	return ""
}

func sourceInventoryGoCommentText(symbolName string, groups ...*ast.CommentGroup) string {
	for _, group := range groups {
		if group == nil {
			continue
		}
		if note := sourceInventoryCompactNote(group.Text()); note != "" {
			if sourceInventoryCommentDescribesSymbol(note, symbolName) {
				return note
			}
		}
	}
	return ""
}

func sourceInventoryScopes(ctx *types.BusContext, graph *repotypes.Graph, facts []types.AnswerAggregateFact) []string {
	seen := map[string]bool{}
	var scopes []string
	add := func(raw string) {
		scope := sourceInventoryScopeForSurface(graph, raw)
		if scope == "" || seen[scope] {
			return
		}
		seen[scope] = true
		scopes = append(scopes, scope)
	}
	// Source-inventory recovery is allowed to fill holes inside the user's
	// requested package/path, but it must not promote analyzer-related context
	// files into principal inventory rows. Prefer path-like current-request
	// entity lanes; fall back to required file hints only when no request scope
	// was resolved. Subtopics and aggregate support refs remain legacy fallbacks.
	for _, scope := range sourceInventoryRequestedScopes(ctx, graph) {
		add(scope)
	}
	if len(scopes) > 0 {
		sort.Strings(scopes)
		return scopes
	}
	if ctx != nil && ctx.AnalysisIR != nil {
		for _, topic := range ctx.AnalysisIR.RequestModel.SubTopics {
			for _, entity := range topic.Entities {
				add(entity)
			}
		}
	}
	for _, fact := range facts {
		for _, ref := range fact.SupportRefs {
			_, loc, ok := aggregateMemberSupportRefParts(ref)
			if !ok {
				continue
			}
			file, _ := aggregateLocationParts(loc)
			add(file)
		}
	}
	sort.Strings(scopes)
	return scopes
}

func sourceInventoryRequestedScopes(ctx *types.BusContext, graph *repotypes.Graph) []string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	hints := ctx.AnalysisIR.RequestModel.AnalyzerHints
	seen := map[string]bool{}
	var out []string
	add := func(raw string) {
		scope := sourceInventoryScopeForSurface(graph, raw)
		if scope == "" || seen[scope] {
			return
		}
		seen[scope] = true
		out = append(out, scope)
	}
	for _, group := range [][]string{
		hints.ExactTargets,
		hints.MentionedEntities,
		hints.PrimaryEntities,
		hints.Entities,
	} {
		for _, raw := range group {
			add(raw)
		}
	}
	if len(out) > 0 {
		sort.Strings(out)
		return out
	}
	for _, result := range aggregateSupportToolResults(ctx) {
		if !result.Success || result.ToolName != "list_files" {
			continue
		}
		add(sourceInventoryListFilesScope(result.Summary))
	}
	if len(out) > 0 {
		sort.Strings(out)
		return out
	}
	for _, file := range ctx.AnalysisIR.EvidencePlan.RequiredFiles {
		add(file)
	}
	for _, hint := range hints.RequiredFileHints {
		add(hint.Path)
	}
	sort.Strings(out)
	return out
}

func sourceInventoryListFilesScope(summary string) string {
	if strings.TrimSpace(summary) == "" {
		return ""
	}
	firstLine := summary
	if idx := strings.Index(firstLine, "\n"); idx >= 0 {
		firstLine = firstLine[:idx]
	}
	firstLine = strings.TrimSpace(firstLine)
	if !strings.HasPrefix(firstLine, "[list_files: ") || !strings.HasSuffix(firstLine, "]") {
		return ""
	}
	const key = "path="
	idx := strings.Index(firstLine, key)
	if idx < 0 {
		return ""
	}
	value := strings.TrimSpace(firstLine[idx+len(key) : len(firstLine)-1])
	if split := strings.Index(value, " recursive="); split >= 0 {
		value = strings.TrimSpace(value[:split])
	}
	return value
}

func sourceInventoryScopeForSurface(graph *repotypes.Graph, raw string) string {
	surface := normalizeSourceInventoryScopeSurface(raw)
	if surface == "" {
		return ""
	}
	if graph != nil {
		if _, ok := graph.FileIndex[surface]; ok {
			return surface
		}
		if scope := sourceInventoryScopeFromAbsoluteSuffix(graph, surface); scope != "" {
			return scope
		}
		for file := range graph.FileIndex {
			file = strings.Trim(file, "/")
			if file == surface || strings.HasPrefix(file, surface+"/") {
				return surface
			}
			if aggregateReadFilePathMatchesQualifier(file, surface) {
				return file
			}
		}
	}
	return ""
}

func normalizeSourceInventoryScopeSurface(raw string) string {
	surface := strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`))
	if surface == "" {
		return ""
	}
	surface = filepath.ToSlash(filepath.Clean(surface))
	if surface == "." {
		return ""
	}
	surface = strings.TrimPrefix(surface, "./")
	return strings.Trim(surface, "/")
}

func sourceInventoryScopeFromAbsoluteSuffix(graph *repotypes.Graph, surface string) string {
	if graph == nil || len(graph.FileIndex) == 0 || surface == "" {
		return ""
	}
	best := ""
	consider := func(candidate string) {
		candidate = strings.Trim(candidate, "/")
		if candidate == "" {
			return
		}
		if surface == candidate || strings.HasSuffix(surface, "/"+candidate) {
			if len(candidate) > len(best) {
				best = candidate
			}
		}
	}
	for file := range graph.FileIndex {
		file = strings.Trim(file, "/")
		if file == "" {
			continue
		}
		consider(file)
		for dir := path.Dir(file); dir != "." && dir != "/" && dir != ""; dir = path.Dir(dir) {
			consider(dir)
		}
	}
	return best
}

func sourceInventoryScopedGraphFiles(graph *repotypes.Graph, scopes []string, language string) []*repotypes.FileInfo {
	if graph == nil {
		return nil
	}
	var files []*repotypes.FileInfo
	for _, fi := range graph.Files {
		if fi == nil || strings.TrimSpace(fi.RelPath) == "" {
			continue
		}
		if language != "" && strings.TrimSpace(fi.Language) != language {
			continue
		}
		if sourceInventoryFileInScopes(fi.RelPath, scopes) {
			files = append(files, fi)
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].RelPath < files[j].RelPath })
	return files
}

func sourceInventoryFileInScopes(file string, scopes []string) bool {
	file = strings.Trim(strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`)), "/")
	if file == "" {
		return false
	}
	for _, scope := range scopes {
		scope = strings.Trim(strings.TrimSpace(strings.ReplaceAll(scope, `\`, `/`)), "/")
		if scope == "" || scope == "." {
			return true
		}
		if file == scope || strings.HasPrefix(file, scope+"/") || aggregateReadFilePathMatchesQualifier(file, scope) {
			return true
		}
	}
	return false
}

func sourceInventoryScopesAllLanguage(graph *repotypes.Graph, scopes []string, language string) bool {
	files := sourceInventoryScopedGraphFiles(graph, scopes, "")
	if len(files) == 0 {
		return false
	}
	for _, fi := range files {
		if fi == nil || fi.IsSpecial {
			continue
		}
		if strings.TrimSpace(fi.Language) != language {
			return false
		}
	}
	return true
}

func sourceInventoryScopesHaveIndexedSourceFiles(graph *repotypes.Graph, scopes []string) bool {
	files := sourceInventoryScopedGraphFiles(graph, scopes, "")
	if len(files) == 0 {
		return false
	}
	for _, fi := range files {
		if fi == nil || fi.IsSpecial {
			continue
		}
		if strings.TrimSpace(fi.Language) != "" {
			return true
		}
	}
	return false
}

func sourceInventoryGraphSymbols(graph *repotypes.Graph) []*repotypes.Symbol {
	if graph == nil {
		return nil
	}
	seen := map[*repotypes.Symbol]bool{}
	var out []*repotypes.Symbol
	if len(graph.SymbolByID) > 0 {
		for _, sym := range graph.SymbolByID {
			if sym != nil && !seen[sym] {
				seen[sym] = true
				out = append(out, sym)
			}
		}
	}
	for _, defs := range graph.SymbolDefs {
		for _, sym := range defs {
			if sym != nil && !seen[sym] {
				seen[sym] = true
				out = append(out, sym)
			}
		}
	}
	return out
}

func sourceInventoryCandidateForSymbol(sym *repotypes.Symbol, role types.AnswerCandidateRole, graph *repotypes.Graph) sourceInventoryCandidate {
	language := sourceInventoryGraphLanguageForFile(graph, sym.File)
	return sourceInventoryCandidate{
		member:     strings.TrimSpace(sym.Name),
		key:        aggregateMemberKey(sym.Name),
		supportRef: strings.TrimSpace(sym.Name) + ": " + aggregateSupportLocationKey(sym.File, sym.Line),
		note:       sourceInventoryCandidateNoteFromGraph(sym, language),
		role:       role,
		exported:   sym.Exported,
		file:       sym.File,
		line:       sym.Line,
		language:   language,
	}
}

func sourceInventoryCandidateNote(candidate sourceInventoryCandidate) string {
	return sourceInventoryCompactNote(candidate.note)
}

func sourceInventoryCandidateNoteFromGraph(sym *repotypes.Symbol, language string) string {
	if sym == nil {
		return ""
	}
	note := sourceInventoryCompactNote(sym.Doc)
	if !sourceInventoryGraphDocDescribesSymbol(note, sym.Name, language) {
		return ""
	}
	return note
}

func sourceInventoryGraphLanguageForFile(graph *repotypes.Graph, file string) string {
	if graph == nil || graph.FileIndex == nil {
		return ""
	}
	if fi := graph.FileIndex[file]; fi != nil {
		return strings.TrimSpace(fi.Language)
	}
	return ""
}

func sourceInventoryCompactNote(raw string) string {
	note := strings.Join(strings.Fields(sourceInventoryCleanCommentText(raw)), " ")
	if note == "" {
		return ""
	}
	const max = 240
	if len([]rune(note)) <= max {
		return note
	}
	runes := []rune(note)
	return strings.TrimSpace(string(runes[:max]))
}

func sourceInventoryCleanCommentText(raw string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\r\n", "\n"))
	if raw == "" {
		return ""
	}
	lines := strings.Split(raw, "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimSpace(strings.TrimSuffix(line, "*/"))
		for {
			switch {
			case strings.HasPrefix(line, "//"):
				line = strings.TrimSpace(strings.TrimPrefix(line, "//"))
			case strings.HasPrefix(line, "/*"):
				line = strings.TrimSpace(strings.TrimPrefix(line, "/*"))
			case strings.HasPrefix(line, "*/"):
				line = strings.TrimSpace(strings.TrimPrefix(line, "*/"))
			case strings.HasPrefix(line, "*"):
				line = strings.TrimSpace(strings.TrimPrefix(line, "*"))
			default:
				goto donePrefix
			}
		}
	donePrefix:
		line = strings.TrimSpace(strings.TrimSuffix(line, "*/"))
		line = strings.TrimSpace(line)
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}
	return strings.Join(cleaned, " ")
}

func sourceInventoryCommentDescribesSymbol(note string, symbolName string) bool {
	note = strings.TrimSpace(note)
	symbolName = strings.TrimSpace(symbolName)
	if note == "" || symbolName == "" {
		return false
	}
	return sourceInventoryContainsSymbolToken(strings.ToLower(note), strings.ToLower(symbolName))
}

func sourceInventoryGraphDocDescribesSymbol(note string, symbolName string, language string) bool {
	note = strings.TrimSpace(note)
	if note == "" || sourceInventoryLooksStructuralDoc(note) {
		return false
	}
	if sourceInventoryCommentDescribesSymbol(note, symbolName) {
		return true
	}
	// Python docstrings are structurally inside the class/function node, so
	// they can be trusted as symbol docs even when the prose does not repeat
	// the symbol name. Adjacent comments in other languages remain stricter
	// until their extractors carry doc provenance separately.
	return language == repotypes.LangPython
}

func sourceInventoryLooksStructuralDoc(note string) bool {
	note = strings.TrimSpace(note)
	if note == "" {
		return false
	}
	lower := strings.ToLower(note)
	if strings.HasPrefix(note, "@") || strings.HasPrefix(lower, "extends ") {
		return true
	}
	fields := strings.Fields(lower)
	if len(fields) == 0 {
		return false
	}
	structural := map[string]bool{
		"public": true, "private": true, "protected": true, "internal": true,
		"open": true, "final": true, "abstract": true, "static": true,
		"const": true, "foreign": true, "operator": true, "func": true,
		"init": true, "main": true, "entry": true, "typealias": true,
		"extend": true,
	}
	for _, field := range fields {
		field = strings.Trim(field, " ,;:()[]{}")
		if field == "" {
			continue
		}
		if !structural[field] {
			return false
		}
	}
	return true
}

func sourceInventoryContainsSymbolToken(note string, symbol string) bool {
	start := 0
	for start < len(note) {
		idx := strings.Index(note[start:], symbol)
		if idx < 0 {
			return false
		}
		idx += start
		after := idx + len(symbol)
		if !sourceInventoryIdentifierRuneBefore(note, idx) && !sourceInventoryIdentifierRuneAt(note, after) {
			return true
		}
		start = after
	}
	return false
}

func sourceInventoryIdentifierRuneBefore(text string, idx int) bool {
	if idx <= 0 || idx > len(text) {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(text[:idx])
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func sourceInventoryIdentifierRuneAt(text string, idx int) bool {
	if idx < 0 || idx >= len(text) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(text[idx:])
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func appendStringAtIndex(values []string, idx int, value string) []string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	for len(values) < idx {
		values = append(values, "")
	}
	if len(values) == idx {
		return append(values, value)
	}
	if values[idx] == "" {
		values[idx] = value
	}
	return values
}

func trimTrailingEmptyStrings(values []string) []string {
	last := -1
	for i, value := range values {
		if strings.TrimSpace(value) != "" {
			last = i
		}
	}
	if last < 0 {
		return nil
	}
	return append([]string(nil), values[:last+1]...)
}

func sourceInventorySymbolMatchesVisibility(sym *repotypes.Symbol, profile *types.AnswerVisibilityProfile) bool {
	if sym == nil {
		return false
	}
	return sourceInventoryVisibilityAllowsExported(sym.Exported, profile)
}

func sourceInventoryVisibilityAllowsExported(exported bool, profile *types.AnswerVisibilityProfile) bool {
	if profile == nil || !profile.Active() {
		return true
	}
	switch profile.SymbolVisibility {
	case types.AnswerSymbolVisibilityPublicExported:
		return exported
	case types.AnswerSymbolVisibilityPrivateOnly:
		return !exported
	case types.AnswerSymbolVisibilityAll:
		return true
	default:
		return true
	}
}

func sourceInventoryGraphSymbolAt(graph *repotypes.Graph, name, file string, line int) *repotypes.Symbol {
	if graph == nil {
		return nil
	}
	for _, sym := range graph.SymbolDefs[name] {
		if aggregateGraphSymbolMatchesLocation(sym, name, file, line) {
			return sym
		}
	}
	return nil
}

func sourceInventorySortCandidates(candidates []sourceInventoryCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].file != candidates[j].file {
			return candidates[i].file < candidates[j].file
		}
		if candidates[i].line != candidates[j].line {
			return candidates[i].line < candidates[j].line
		}
		return candidates[i].member < candidates[j].member
	})
}

func strconvItoa(n int) string {
	return strconv.Itoa(n)
}
