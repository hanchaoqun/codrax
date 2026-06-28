package tool

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	sourceInventoryExactUniverseProvenanceListFilesDirect        = "tool:list_files:direct"
	sourceInventoryExactUniverseProvenanceRepoLensDirectChildren = "repo_lens:direct_children"

	// SourceInventoryCandidateUniverseSummaryLimit keeps candidate-universe
	// repair hints useful without turning navigation checklists into long
	// answer drafts. It is intentionally advisory-display only; coverage
	// checks still evaluate the full member set.
	SourceInventoryCandidateUniverseSummaryLimit = 12
)

type sourceInventoryExactUniverseSet struct {
	role    types.AnswerCandidateRole
	scope   string
	members []types.SourceInventoryObservationMember
}

// SourceInventoryCandidateUniverseGap describes a structural mismatch between
// an exact candidate universe observed by navigation tools and the model's
// own structured member_set/exclusion handoff. It never declares the observed
// candidates to be final-answer members; callers use it only to avoid treating
// a partial member_set as complete without an explicit model-authored
// exclusion/disclosure.
type SourceInventoryCandidateUniverseGap struct {
	Role      types.AnswerCandidateRole
	Scope     string
	Count     int
	Covered   int
	Excluded  int
	Missing   []types.SourceInventoryObservationMember
	Blocking  bool
	Disclosed bool
}

type SourceInventoryLensExecutionGap struct {
	Roles        []types.AnswerCandidateRole
	Scopes       []string
	HasAdvisory  bool
	HasListFiles bool
	Blocking     bool
}

func (g SourceInventoryCandidateUniverseGap) IsActive() bool {
	return g.Count > 0 && len(g.Missing) > 0
}

func (g SourceInventoryCandidateUniverseGap) Summary(maxMissing int) string {
	if !g.IsActive() {
		return ""
	}
	if maxMissing <= 0 {
		maxMissing = 8
	}
	role := strings.TrimSpace(string(g.Role))
	if role == "" {
		role = "candidate"
	}
	scope := strings.TrimSpace(g.Scope)
	if scope == "" {
		scope = "."
	}
	names := g.MissingNames(maxMissing)
	omitted := ""
	if len(g.Missing) > len(names) {
		omitted = fmt.Sprintf(" (+%d more)", len(g.Missing)-len(names))
	}
	return fmt.Sprintf("role=%s scope=%s exact_count=%d covered=%d excluded=%d missing=%d [%s]%s",
		role, scope, g.Count, g.Covered, g.Excluded, len(g.Missing), strings.Join(names, ", "), omitted)
}

func (g SourceInventoryCandidateUniverseGap) MissingNames(max int) []string {
	if max <= 0 || max > len(g.Missing) {
		max = len(g.Missing)
	}
	out := make([]string, 0, max)
	seen := make(map[string]bool, max)
	for _, member := range g.Missing {
		name := strings.TrimSpace(member.Name)
		if name == "" {
			name = strings.TrimSpace(member.Key)
		}
		if name == "" {
			name = strings.TrimSpace(member.File)
		}
		if name == "" {
			continue
		}
		key := sourceInventoryUniverseSurfaceKey(name)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, name)
		if len(out) >= max {
			break
		}
	}
	return out
}

// SourceInventoryLensExecutionGapForContext reports whether a principal
// source_inventory lane has reached the executable repo-map lens boundary.
func SourceInventoryLensExecutionGapForContext(ctx *types.BusContext) SourceInventoryLensExecutionGap {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.Mutable == nil {
		return SourceInventoryLensExecutionGap{}
	}
	rm := ctx.AnalysisIR.RequestModel
	profile := rm.SourceInventoryProfile
	advisory := ctx.Mutable.SourceInventoryAdvisory()
	var roles []types.AnswerCandidateRole
	if types.SourceInventoryPrincipalAuthorityActive(rm) {
		roles = profile.PrincipalTargetRoles()
		if len(roles) == 0 {
			roles = append([]types.AnswerCandidateRole(nil), profile.TargetRoles...)
		}
	} else if sourceInventoryAdvisoryIsTypedQueryLane(ctx, advisory) {
		roles = sourceInventoryLensExecutionRolesFromAdvisory(advisory)
	} else {
		return SourceInventoryLensExecutionGap{}
	}
	observation := types.SourceInventoryObservationFromMutable(ctx.Mutable)
	gap := SourceInventoryLensExecutionGap{
		Roles:       sourceInventoryLensExecutionRoles(roles),
		Scopes:      sourceInventoryLensExecutionScopes(advisory, observation),
		HasAdvisory: advisory.IsActive(),
	}
	if sourceInventoryObservationHasListFilesDirect(observation) {
		gap.HasListFiles = true
	}
	if sourceInventoryAdvisoryHasRepoLensToolQuery(advisory) ||
		sourceInventoryObservationHasRepoLensToolQuery(observation) ||
		sourceInventoryToolResultsHaveSourceInventoryLens(ctx.Mutable.DispatchToolResults()) ||
		sourceInventoryToolResultsHaveSourceInventoryLens(ctx.ToolResults) {
		return gap
	}
	gap.Blocking = true
	return gap
}

func sourceInventoryAdvisoryIsTypedQueryLane(ctx *types.BusContext, advisory types.SourceInventoryAdvisory) bool {
	if ctx == nil || ctx.AnalysisIR == nil || !advisory.IsActive() {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if !types.SourceInventoryPrincipalAuthorityActive(rm) &&
		(rm.SourceInventoryProfile != nil || !types.IsTypedSourceEnumerationShape(rm)) {
		return false
	}
	for _, provenance := range advisory.Provenance {
		if strings.TrimSpace(provenance) == "request_traits:typed_source_enumeration_query" {
			return true
		}
	}
	return false
}

func sourceInventoryLensExecutionRolesFromAdvisory(advisory types.SourceInventoryAdvisory) []types.AnswerCandidateRole {
	seen := map[types.AnswerCandidateRole]bool{}
	out := make([]types.AnswerCandidateRole, 0, len(advisory.Sets))
	for _, set := range advisory.Sets {
		if set.Role == "" || set.Role == types.AnswerCandidateRoleUnknown || seen[set.Role] {
			continue
		}
		seen[set.Role] = true
		out = append(out, set.Role)
	}
	return out
}

func sourceInventoryLensExecutionRoles(in []types.AnswerCandidateRole) []types.AnswerCandidateRole {
	seen := map[types.AnswerCandidateRole]bool{}
	out := make([]types.AnswerCandidateRole, 0, len(in))
	for _, role := range in {
		if role == "" || role == types.AnswerCandidateRoleUnknown || seen[role] {
			continue
		}
		seen[role] = true
		out = append(out, role)
	}
	return out
}

func sourceInventoryLensExecutionScopes(advisory types.SourceInventoryAdvisory, observation types.SourceInventoryObservation) []string {
	var out []string
	seen := map[string]bool{}
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" || seen[raw] {
			return
		}
		seen[raw] = true
		out = append(out, raw)
	}
	for _, scope := range advisory.Scopes {
		add(scope)
	}
	for _, scope := range observation.Scopes {
		add(scope)
	}
	return out
}

func sourceInventoryAdvisoryHasRepoLensToolQuery(advisory types.SourceInventoryAdvisory) bool {
	if !advisory.IsActive() {
		return false
	}
	for _, provenance := range advisory.Provenance {
		if strings.TrimSpace(provenance) == "repo_lens:tool_query" {
			return true
		}
	}
	return false
}

func sourceInventoryObservationHasRepoLensToolQuery(observation types.SourceInventoryObservation) bool {
	if !observation.IsActive() {
		return false
	}
	for _, provenance := range observation.Provenance {
		if strings.TrimSpace(provenance) == "repo_lens:tool_query" {
			return true
		}
	}
	return false
}

func sourceInventoryObservationHasListFilesDirect(observation types.SourceInventoryObservation) bool {
	if !observation.IsActive() {
		return false
	}
	for _, provenance := range observation.Provenance {
		if strings.TrimSpace(provenance) == sourceInventoryExactUniverseProvenanceListFilesDirect {
			return true
		}
	}
	for _, set := range observation.Sets {
		for _, member := range set.Members {
			for _, provenance := range member.Provenance {
				if strings.TrimSpace(provenance) == sourceInventoryExactUniverseProvenanceListFilesDirect {
					return true
				}
			}
		}
	}
	return false
}

func sourceInventoryToolResultsHaveSourceInventoryLens(results []types.ToolResult) bool {
	for _, result := range results {
		if !result.Success || types.CanonicalToolName(result.ToolName) != "repo_map" {
			continue
		}
		if result.SourceInventory != nil && types.SourceInventoryLensExecuted(*result.SourceInventory) {
			return true
		}
	}
	return false
}

// SourceInventoryCandidateUniverseCoverageGap compares exact observed
// candidate universes with model-authored aggregate facts. Exact universes are
// currently published by direct, non-recursive list_files results and by the
// model's explicit source_inventory lens requests when they ask for mechanical
// direct-child roles such as package/file/config_file. Future structured tools
// can opt in by attaching the same member-level exact provenance semantics.
// Advisory repo-map/source-inventory graph rows without exact provenance are
// intentionally ignored here.
func SourceInventoryCandidateUniverseCoverageGap(ctx *types.BusContext, facts []types.AnswerAggregateFact) SourceInventoryCandidateUniverseGap {
	if ctx == nil || ctx.Mutable == nil {
		return SourceInventoryCandidateUniverseGap{}
	}
	universes := sourceInventoryExactUniverseSets(ctx.Mutable.SourceInventoryObservation())
	if len(universes) == 0 {
		return SourceInventoryCandidateUniverseGap{}
	}
	var rm *types.RequestModel
	if ctx.AnalysisIR != nil {
		rm = &ctx.AnalysisIR.RequestModel
	}
	included, excluded := sourceInventoryAggregateCoverageKeys(facts, rm)
	best := SourceInventoryCandidateUniverseGap{}
	for _, universe := range universes {
		if len(universe.members) == 0 {
			continue
		}
		gap := sourceInventoryCoverageForUniverse(universe, included, excluded)
		if !gap.IsActive() {
			continue
		}
		if len(facts) > 0 && !sourceInventoryUniverseStronglyAligned(gap) {
			continue
		}
		if gap.Covered > 0 && sourceInventoryAggregateHasCountDisclosure(facts, len(gap.Missing)) {
			gap.Blocking = false
			gap.Disclosed = true
		} else {
			gap.Blocking = true
		}
		if sourceInventoryCandidateUniverseGapBetter(gap, best) {
			best = gap
		}
	}
	return best
}

// SourceInventoryAcceptedClosureCoversExactUniverse reports whether a
// model-authored aggregate handoff already covers at least one exact
// source-inventory universe observed by navigation tools.
//
// This is a positive proof helper for retry/closure policy, not an answer
// synthesizer. It only returns true when:
//   - an exact universe exists with member-level exact provenance;
//   - the model emitted a principal complete member_set/count-member carrier;
//   - every exact universe member is either included by that carrier or
//     explicitly excluded by the model; and
//   - at least one member is included, so an exclusion-only boundary cannot
//     masquerade as an answer slate.
//
// The comparison is intentionally language-neutral and reuses the same
// normalized member-key machinery as SourceInventoryCandidateUniverseCoverageGap.
// It never reads the raw user request, model thinking/prose, or language-specific
// source syntax.
func SourceInventoryAcceptedClosureCoversExactUniverse(ctx *types.BusContext, facts []types.AnswerAggregateFact) bool {
	if ctx == nil || ctx.Mutable == nil || len(facts) == 0 {
		return false
	}
	universes := sourceInventoryExactUniverseSets(ctx.Mutable.SourceInventoryObservation())
	if len(universes) == 0 {
		return false
	}
	var rm *types.RequestModel
	if ctx.AnalysisIR != nil {
		rm = &ctx.AnalysisIR.RequestModel
	}
	included, excluded := sourceInventoryAggregateCoverageKeys(facts, rm)
	if len(included) == 0 {
		return false
	}
	for _, universe := range universes {
		if len(universe.members) == 0 {
			continue
		}
		gap := sourceInventoryCoverageForUniverse(universe, included, excluded)
		if gap.Count > 0 && gap.Covered > 0 && gap.Covered+gap.Excluded == gap.Count {
			return true
		}
	}
	return false
}

func sourceInventoryExactUniverseSets(observation types.SourceInventoryObservation) []sourceInventoryExactUniverseSet {
	if !observation.IsActive() {
		return nil
	}
	groups := map[string]*sourceInventoryExactUniverseSet{}
	var order []string
	for _, set := range observation.Sets {
		if len(set.Members) == 0 {
			continue
		}
		for _, member := range set.Members {
			if !sourceInventoryObservationMemberIsExactUniverse(member) {
				continue
			}
			scope := sourceInventoryObservationMemberScope(member)
			key := string(member.Role) + "\x00" + scope
			group := groups[key]
			if group == nil {
				group = &sourceInventoryExactUniverseSet{role: member.Role, scope: scope}
				groups[key] = group
				order = append(order, key)
			}
			group.members = append(group.members, member)
		}
	}
	out := make([]sourceInventoryExactUniverseSet, 0, len(order))
	for _, key := range order {
		group := groups[key]
		if group == nil || len(group.members) == 0 {
			continue
		}
		out = append(out, *group)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if len(out[i].members) != len(out[j].members) {
			return len(out[i].members) > len(out[j].members)
		}
		if out[i].scope != out[j].scope {
			return out[i].scope < out[j].scope
		}
		return string(out[i].role) < string(out[j].role)
	})
	return out
}

func sourceInventoryObservationMemberIsExactUniverse(member types.SourceInventoryObservationMember) bool {
	for _, provenance := range member.Provenance {
		switch strings.TrimSpace(provenance) {
		case sourceInventoryExactUniverseProvenanceListFilesDirect,
			sourceInventoryExactUniverseProvenanceRepoLensDirectChildren:
			return true
		}
	}
	return false
}

func sourceInventoryObservationMemberScope(member types.SourceInventoryObservationMember) string {
	raw := strings.TrimSpace(member.Key)
	if raw == "" {
		raw = strings.TrimSpace(member.File)
	}
	raw = strings.Trim(strings.ReplaceAll(raw, `\`, `/`), "/")
	if raw == "" {
		return "."
	}
	dir := path.Dir(raw)
	if dir == "." || dir == "/" || dir == "" {
		return "."
	}
	return dir
}

func sourceInventoryCoverageForUniverse(universe sourceInventoryExactUniverseSet, included, excluded map[string]bool) SourceInventoryCandidateUniverseGap {
	gap := SourceInventoryCandidateUniverseGap{
		Role:  universe.role,
		Scope: universe.scope,
		Count: len(universe.members),
	}
	for _, member := range universe.members {
		keys := sourceInventoryUniverseMemberKeys(member)
		if sourceInventoryUniverseAnyKey(keys, included) {
			gap.Covered++
			continue
		}
		if sourceInventoryUniverseAnyKey(keys, excluded) {
			gap.Excluded++
			continue
		}
		gap.Missing = append(gap.Missing, member)
	}
	return gap
}

func sourceInventoryUniverseStronglyAligned(gap SourceInventoryCandidateUniverseGap) bool {
	aligned := gap.Covered + gap.Excluded
	if aligned <= 0 || gap.Count <= 0 || aligned >= gap.Count {
		return false
	}
	if gap.Count <= 3 {
		return aligned >= 2 && aligned >= gap.Count-1
	}
	return aligned >= 3 && aligned*5 >= gap.Count*3
}

func sourceInventoryAggregateCoverageKeys(facts []types.AnswerAggregateFact, rm *types.RequestModel) (map[string]bool, map[string]bool) {
	included := map[string]bool{}
	excluded := map[string]bool{}
	addMany := func(target map[string]bool, raw string) {
		for _, key := range sourceInventoryUniverseAggregateMemberKeys(raw) {
			target[key] = true
		}
	}
	for _, fact := range facts {
		role := types.AnswerAggregateFactRoleForRequest(fact, rm)
		if role == types.AnswerAggregateRolePrincipalAnswer && types.AnswerAggregateFactCarriesCompleteMemberSet(fact) {
			for _, member := range fact.Members {
				addMany(included, member)
			}
			for _, ref := range fact.SupportRefs {
				included = sourceInventoryUniverseAppendSupportRefKeys(included, ref)
			}
		}
		for _, member := range fact.Excluded {
			addMany(excluded, member)
		}
		if fact.Kind == types.AnswerAggregateExcluded {
			for _, member := range fact.Members {
				addMany(excluded, member)
			}
		}
	}
	return included, excluded
}

func sourceInventoryAggregateHasCountDisclosure(facts []types.AnswerAggregateFact, missingCount int) bool {
	if missingCount <= 0 {
		return true
	}
	for _, fact := range facts {
		if fact.Kind != types.AnswerAggregateExcluded {
			continue
		}
		value := strings.TrimSpace(fact.Value)
		if value == "" {
			continue
		}
		n, err := strconv.Atoi(value)
		if err == nil && n == missingCount {
			return true
		}
	}
	return false
}

func sourceInventoryCandidateUniverseGapBetter(candidate, current SourceInventoryCandidateUniverseGap) bool {
	if !candidate.IsActive() {
		return false
	}
	if !current.IsActive() {
		return true
	}
	if candidate.Blocking != current.Blocking {
		return candidate.Blocking
	}
	cAligned := candidate.Covered + candidate.Excluded
	curAligned := current.Covered + current.Excluded
	if cAligned != curAligned {
		return cAligned > curAligned
	}
	if candidate.Count != current.Count {
		return candidate.Count > current.Count
	}
	return len(candidate.Missing) > len(current.Missing)
}

func sourceInventoryUniverseAnyKey(keys []string, target map[string]bool) bool {
	for _, key := range keys {
		if target[key] {
			return true
		}
	}
	return false
}

func sourceInventoryUniverseMemberKeys(member types.SourceInventoryObservationMember) []string {
	var out []string
	add := func(raw string) {
		out = sourceInventoryUniverseAppendSurfaceKeys(out, raw)
	}
	add(member.Name)
	add(member.Key)
	add(member.SupportRef)
	add(member.File)
	return sourceInventoryUniverseDedupKeys(out)
}

func sourceInventoryUniverseAggregateMemberKeys(member string) []string {
	var out []string
	add := func(raw string) {
		out = sourceInventoryUniverseAppendSurfaceKeys(out, raw)
	}
	add(member)
	for _, candidate := range types.AnswerAggregateMemberDisplayCandidates(member) {
		add(candidate)
	}
	if label, loc, ok := types.ParseAnswerSupportRefMemberLocation(member); ok {
		add(label)
		add(loc.File)
	}
	if left, right, ok := types.AnswerAggregateMemberRelationParts(member); ok {
		add(left)
		add(right)
	}
	if base, qualifier, ok := types.AnswerAggregateDecoratedLabelParts(member); ok {
		add(base)
		add(qualifier)
	}
	for _, sep := range []string{" @ ", "\t", " | "} {
		if idx := strings.Index(member, sep); idx > 0 {
			add(member[:idx])
		}
	}
	return sourceInventoryUniverseDedupKeys(out)
}

func sourceInventoryUniverseAppendSurfaceKeys(out []string, raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out
	}
	add := func(candidate string) {
		if key := sourceInventoryUniverseSurfaceKey(candidate); key != "" {
			out = append(out, key)
		}
	}
	add(raw)
	if label, loc, ok := types.ParseAnswerSupportRefMemberLocation(raw); ok {
		add(label)
		add(loc.File)
	}
	normalizedPath := strings.Trim(strings.ReplaceAll(raw, `\`, `/`), "/")
	if normalizedPath != "" && strings.Contains(normalizedPath, "/") {
		add(path.Base(normalizedPath))
	}
	if base, qualifier, ok := types.AnswerAggregateDecoratedLabelParts(raw); ok {
		add(base)
		add(qualifier)
	}
	if left, right, ok := types.AnswerAggregateMemberRelationParts(raw); ok {
		add(left)
		add(right)
	}
	return out
}

func sourceInventoryUniverseSurfaceKey(raw string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`))
	raw = strings.Trim(raw, "` \t\r\n")
	if raw == "" {
		return ""
	}
	return strings.ToLower(strings.Join(strings.Fields(raw), " "))
}

func sourceInventoryUniverseDedupKeys(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, key := range in {
		key = strings.TrimSpace(key)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
}
