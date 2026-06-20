package tool

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/tool/sourceinventory"
	"github.com/hanchaoqun/codrax/internal/types"
)

type sourceInventoryObservationScopeGroup struct {
	Scope      string
	order      int
	Candidates []types.SourceInventoryObservationMember
	RoleCounts map[types.AnswerCandidateRole]int
	Languages  map[string]int
}

type sourceInventorySuggestedFileGroup struct {
	Scope          string
	order          int
	Files          []sourceInventorySuggestedFile
	candidateCount int
}

type sourceInventorySuggestedFile struct {
	File       string
	order      int
	RoleCounts map[types.AnswerCandidateRole]int
	Languages  map[string]int
	Candidates []sourceInventorySuggestedFileCandidate
	seen       map[string]bool
}

type sourceInventorySuggestedFileCandidate struct {
	Name string
	Role types.AnswerCandidateRole
	Line int
}

func renderSourceInventoryAdvisoryToolHint(advisory types.SourceInventoryAdvisory) string {
	if !advisory.IsActive() {
		return ""
	}
	const maxRows = 24
	var b strings.Builder
	b.WriteString("Structured source-inventory candidate checklist (verified navigation/count facts, not final answer text):\n")
	if len(advisory.Scopes) > 0 {
		fmt.Fprintf(&b, "- scoped to: %s\n", strings.Join(advisory.Scopes, ", "))
	}
	b.WriteString("- reuse this checklist to avoid re-listing the same scope; verify/read selected semantic claims before emitting source-cited evidence or aggregate_facts.\n")
	b.WriteString("- for a compact scoped member/count checklist, call repo_map with view=\"source_inventory\", roles=[...], include_attributes=false instead of reading every candidate file; add attribute_roles only after choosing a narrow scope/member.\n")
	if cascadeView := renderSourceInventoryCascadeGuideView(
		types.SourceInventoryObservationFromAdvisory(advisory),
		types.SourceInventoryLensQuery{Scopes: append([]string(nil), advisory.Scopes...), IncludeAttributes: false, IncludeCounts: true},
		8,
	); cascadeView != "" {
		b.WriteByte('\n')
		b.WriteString(cascadeView)
		b.WriteString("\n\n")
	}
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

// RenderSourceInventoryCascadeGuideView exposes the same low-noise repo-lens
// navigation guide used by repo_map(view="source_inventory") tool results.
// It is intentionally a navigation/candidate-universe guide: callers may render
// it in prompts to help models choose narrower follow-up repo_map calls and
// preserve count/member invariants, but it must not be treated as a semantic
// source citation or as system-written answer text.
func RenderSourceInventoryCascadeGuideView(observation types.SourceInventoryObservation, query types.SourceInventoryLensQuery, maxGroups int) string {
	return renderSourceInventoryCascadeGuideView(observation, query, maxGroups)
}

func renderSourceInventoryCascadeGuideView(observation types.SourceInventoryObservation, query types.SourceInventoryLensQuery, maxGroups int) string {
	if !observation.IsActive() || sourceInventoryLensQueryOffset(query) > 0 || maxGroups <= 0 {
		return ""
	}
	groups := sourceInventorySuggestedFileGroups(observation, query)
	totalMembers := 0
	roleCounts := map[types.AnswerCandidateRole]int{}
	languageCounts := map[string]int{}
	attrRoleCounts := map[types.AnswerCandidateRole]int{}
	for _, set := range observation.Sets {
		totalMembers += len(set.Members)
		if set.Role != types.AnswerCandidateRoleUnknown && len(set.Members) > 0 {
			roleCounts[set.Role] += len(set.Members)
		}
		for _, member := range set.Members {
			if lang := strings.TrimSpace(member.Language); lang != "" {
				languageCounts[lang]++
			}
			for _, attr := range member.Attributes {
				if attr.Role != types.AnswerCandidateRoleUnknown {
					attrRoleCounts[attr.Role]++
				}
				if lang := strings.TrimSpace(attr.Language); lang != "" {
					languageCounts[lang]++
				}
			}
		}
	}
	totalFiles := 0
	totalCandidateItems := 0
	ambiguousGroups := 0
	for _, group := range groups {
		totalFiles += len(group.Files)
		totalCandidateItems += group.candidateCount
		if len(group.Files) != 1 || group.candidateCount != 1 {
			ambiguousGroups++
		}
	}

	roles := sourceInventoryLensQueryRoles(query)
	if len(roles) == 0 {
		roles = sourceInventoryObservationRoles(observation)
	}
	attributeRoles := sourceInventoryLensQueryAttributeRoles(query)
	if len(attributeRoles) == 0 {
		attributeRoles = sourceInventoryObservationAttributeRoles(observation)
	}
	var b strings.Builder
	b.WriteString("## Cascaded Repo Lens Guide (advisory)\n\n")
	b.WriteString("Use this first as a navigation summary. It verifies candidate scopes/files/symbols/counts and helps you choose the next narrower `repo_map(view=\"source_inventory\")` call; it does not decide the final answer and is not a semantic source citation.\n\n")
	fmt.Fprintf(&b, "- summary: member_rows=%d", totalMembers)
	if len(groups) > 0 {
		fmt.Fprintf(&b, " scope_groups=%d candidate_files=%d candidate_items=%d ambiguous_groups=%d",
			len(groups), totalFiles, totalCandidateItems, ambiguousGroups)
	}
	if rolesText := renderSourceInventoryRoleCounts(roleCounts); rolesText != "" {
		fmt.Fprintf(&b, " roles=%s", rolesText)
	}
	if attrRolesText := renderSourceInventoryRoleCounts(attrRoleCounts); attrRolesText != "" {
		fmt.Fprintf(&b, " attribute_roles=%s", attrRolesText)
	}
	if languages := renderSourceInventoryLanguageCounts(languageCounts); languages != "" {
		fmt.Fprintf(&b, " languages=%s", languages)
	}
	b.WriteByte('\n')
	if len(groups) > 0 {
		fmt.Fprintf(&b, "- expand a scope before reading many files: `%s`\n",
			sourceInventoryCascadeRepoMapCall(sourceInventoryLensQueryPath(query), "<scope>", nil, roles, nil, false, 24, ""))
	}
	if totalMembers > 0 {
		fmt.Fprintf(&b, "- page the current checklist instead of widening blindly: `%s`\n",
			sourceInventoryCascadeRepoMapCall(sourceInventoryLensQueryPath(query), "", sourceInventoryGroupScopesFromQuery(query, observation), roles, attributeRoles, query.IncludeAttributes, 24, "<next_cursor>"))
	}
	b.WriteString("- after you choose a candidate from a narrower lens, verify with `read_file` or `grep` before citing implementation behavior or exact source text.\n")
	if len(groups) == 0 {
		return strings.TrimSpace(b.String())
	}
	b.WriteString("\nSuggested cascade expansions (model chooses which ones match the user's intent):\n")
	rendered := 0
	for _, group := range groups {
		if rendered >= maxGroups {
			break
		}
		if len(group.Files) == 0 && group.candidateCount == 0 {
			continue
		}
		rendered++
		fmt.Fprintf(&b, "- `%s` — files=%d candidates=%d",
			group.Scope, len(group.Files), group.candidateCount)
		if rolesText := renderSourceInventoryFileGroupRoleCounts(group); rolesText != "" {
			fmt.Fprintf(&b, " roles=%s", rolesText)
		}
		if languages := renderSourceInventoryFileGroupLanguageCounts(group); languages != "" {
			fmt.Fprintf(&b, " languages=%s", languages)
		}
		fmt.Fprintf(&b, " — next `%s`\n",
			sourceInventoryCascadeRepoMapCall(sourceInventoryLensQueryPath(query), group.Scope, nil, roles, nil, false, 24, ""))
	}
	if len(groups) > rendered {
		fmt.Fprintf(&b, "\nshowing %d of %d expandable scope groups; use `cursor`/narrower `scope` to continue without reading every file.\n", rendered, len(groups))
	}
	return strings.TrimSpace(b.String())
}

func sourceInventoryObservationRoles(observation types.SourceInventoryObservation) []types.AnswerCandidateRole {
	seen := map[types.AnswerCandidateRole]bool{}
	var roles []types.AnswerCandidateRole
	for _, set := range observation.Sets {
		if set.Role == types.AnswerCandidateRoleUnknown || seen[set.Role] || len(set.Members) == 0 {
			continue
		}
		seen[set.Role] = true
		roles = append(roles, set.Role)
	}
	return roles
}

func sourceInventoryObservationAttributeRoles(observation types.SourceInventoryObservation) []types.AnswerCandidateRole {
	seen := map[types.AnswerCandidateRole]bool{}
	var roles []types.AnswerCandidateRole
	for _, set := range observation.Sets {
		for _, member := range set.Members {
			for _, attr := range member.Attributes {
				if attr.Role == types.AnswerCandidateRoleUnknown || seen[attr.Role] {
					continue
				}
				seen[attr.Role] = true
				roles = append(roles, attr.Role)
			}
		}
	}
	return roles
}

func sourceInventoryLensQueryPath(query types.SourceInventoryLensQuery) string {
	p := strings.TrimSpace(strings.ReplaceAll(query.Path, `\`, `/`))
	if p == "" {
		return "<same repo path>"
	}
	return p
}

func sourceInventoryCascadeRepoMapCall(toolPath, scope string, scopes []string, roles, attributeRoles []types.AnswerCandidateRole, includeAttributes bool, topN int, cursor string) string {
	parts := []string{
		`"path": ` + strconv.Quote(toolPath),
		`"view": "source_inventory"`,
	}
	scope = strings.TrimSpace(scope)
	if scope != "" {
		parts = append(parts, `"scope": `+strconv.Quote(scope))
	} else if scopesText := sourceInventoryStringJSONArray(scopes); scopesText != "" {
		parts = append(parts, `"scopes": `+scopesText)
	}
	if rolesText := sourceInventoryRoleJSONArray(roles); rolesText != "" {
		parts = append(parts, `"roles": `+rolesText)
	}
	if attrRolesText := sourceInventoryRoleJSONArray(attributeRoles); attrRolesText != "" {
		parts = append(parts, `"attribute_roles": `+attrRolesText)
	}
	if includeAttributes {
		parts = append(parts, `"include_counts": true`, `"include_attributes": true`)
	} else {
		parts = append(parts, `"include_counts": true`, `"include_attributes": false`)
	}
	if topN > 0 {
		parts = append(parts, `"top_n": `+strconvItoa(topN))
	}
	if cursor = strings.TrimSpace(cursor); cursor != "" {
		parts = append(parts, `"cursor": `+strconv.Quote(cursor))
	}
	return "repo_map {" + strings.Join(parts, ", ") + "}"
}

func sourceInventoryStringJSONArray(items []string) string {
	if len(items) == 0 {
		return ""
	}
	seen := map[string]bool{}
	var parts []string
	for _, item := range items {
		item = strings.TrimSpace(strings.ReplaceAll(item, `\`, `/`))
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		parts = append(parts, strconv.Quote(item))
	}
	if len(parts) == 0 {
		return ""
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func sourceInventoryRoleJSONArray(roles []types.AnswerCandidateRole) string {
	if len(roles) == 0 {
		return ""
	}
	seen := map[types.AnswerCandidateRole]bool{}
	var parts []string
	for _, role := range roles {
		if role == types.AnswerCandidateRoleUnknown || seen[role] {
			continue
		}
		seen[role] = true
		parts = append(parts, strconv.Quote(string(role)))
	}
	if len(parts) == 0 {
		return ""
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func renderSourceInventoryFileGroupRoleCounts(group sourceInventorySuggestedFileGroup) string {
	counts := map[types.AnswerCandidateRole]int{}
	for _, file := range group.Files {
		for role, count := range file.RoleCounts {
			counts[role] += count
		}
	}
	return renderSourceInventoryRoleCounts(counts)
}

func renderSourceInventoryFileGroupLanguageCounts(group sourceInventorySuggestedFileGroup) string {
	counts := map[string]int{}
	for _, file := range group.Files {
		for language, count := range file.Languages {
			counts[language] += count
		}
	}
	return renderSourceInventoryLanguageCounts(counts)
}

// RenderSourceInventoryObservationView renders a compact model-facing repo lens
// view. The underlying observation remains structured in MutableState; this
// markdown is only a readable checklist for the current tool result.
func RenderSourceInventoryObservationView(observation types.SourceInventoryObservation, query types.SourceInventoryLensQuery) string {
	if !observation.IsActive() {
		return "Repo Lens: no source-inventory observation is available for the requested typed scope/role slice. Try a narrower `scope` or provide `roles` that match repo-map candidate roles."
	}
	cascadeView := renderSourceInventoryCascadeGuideView(observation, query, 16)
	groupedView := renderSourceInventoryGroupedObservationView(observation, query)
	suggestedFilesView := renderSourceInventoryCandidateFileSamplesView(observation, query)
	topN := query.TopN
	if topN <= 0 {
		topN = 60
		if query.RepoFileCount > 0 {
			if tiered := repotypes.DefaultTopN("source_inventory", repotypes.RepoSizeTier(query.RepoFileCount)); tiered > 0 {
				topN = tiered
			}
		}
	}
	if groupedView != "" && query.TopN <= 0 {
		topN = 24
	}
	offset := sourceInventoryLensQueryOffset(query)
	renderOffset := offset
	pageAlreadyApplied := sourceInventoryObservationPageAlreadyApplied(observation, offset)
	if pageAlreadyApplied {
		renderOffset = 0
	}
	includeCounts := query.IncludeCounts
	includeAttributes := query.IncludeAttributes
	var b strings.Builder
	b.WriteString("# Repo Lens: Source Inventory\n\n")
	b.WriteString("This is a repo-map observation checklist for navigation and mechanical member/count checks. It is not final answer text; semantic conclusions still need grounded evidence or explicit model-authored aggregate facts.\n\n")
	if len(observation.Scopes) > 0 {
		fmt.Fprintf(&b, "- scopes: `%s`\n", strings.Join(observation.Scopes, "`, `"))
	}
	if len(observation.Provenance) > 0 {
		fmt.Fprintf(&b, "- provenance: `%s`\n", strings.Join(observation.Provenance, "`, `"))
	}
	if sourceInventoryStringSliceContains(observation.Provenance, "repo_lens:attributes_deferred_broad_scope") {
		b.WriteString("- row-local attributes were deferred because this source_inventory lens is broad; choose a narrower `scope` or selected member, then rerun with `attribute_roles` and `include_attributes=true` for details.\n")
	}
	if sourceInventoryStringSliceContains(observation.Provenance, "repo_lens:candidate_budget_truncated") {
		b.WriteString("- candidate materialization was budget-truncated; treat visible rows as a bounded navigation sample and rerun with narrower `scope`, `roles`, or `query` before claiming exhaustive coverage.\n")
	}
	if len(observation.Lens) > 0 {
		fmt.Fprintf(&b, "- lens: `%s`\n", strings.Join(observation.Lens, "`, `"))
	}
	if sourceClasses := renderSourceInventorySourceClassCounts(observation.SourceClasses); sourceClasses != "" {
		fmt.Fprintf(&b, "- source_classes: %s\n", sourceClasses)
	}
	if census := renderSourceInventoryRepoLanguageCensus(observation); census != "" {
		b.WriteString(census)
		b.WriteByte('\n')
	}
	if roles := sourceInventoryLensQueryAttributeRoles(query); len(roles) > 0 {
		labels := make([]string, 0, len(roles))
		for _, role := range roles {
			labels = append(labels, string(role))
		}
		fmt.Fprintf(&b, "- attribute_roles: `%s`\n", strings.Join(labels, "`, `"))
	}
	if includeCounts {
		totalMembers := 0
		for _, set := range observation.Sets {
			totalMembers += len(set.Members)
		}
		fmt.Fprintf(&b, "- total_visible_member_count: %d\n", totalMembers)
	}
	if offset > 0 {
		fmt.Fprintf(&b, "- page_offset: %d\n", offset)
	}
	b.WriteByte('\n')
	if len(observation.Sets) == 0 {
		b.WriteString("No candidate member rows matched this source_inventory lens. Do not treat this as repository-wide absence unless the typed source-class universe above covers the requested source classes and the final answer carries a bounded negative citation.\n")
		return strings.TrimSpace(b.String())
	}
	if cascadeView != "" {
		b.WriteString(cascadeView)
		b.WriteString("\n\n")
	}
	if groupedView != "" {
		b.WriteString(groupedView)
		b.WriteString("\n\n")
	}
	if suggestedFilesView != "" {
		b.WriteString(suggestedFilesView)
		b.WriteString("\n\n")
	}
	emitted := 0
	visited := 0
	visibleTotal := 0
	fullTotal := 0
	for _, set := range observation.Sets {
		visibleTotal += len(set.Members)
		if set.Total > len(set.Members) {
			fullTotal += set.Total
			continue
		}
		fullTotal += len(set.Members)
	}
	for _, set := range observation.Sets {
		if len(set.Members) == 0 || emitted >= topN {
			continue
		}
		setRowsRemaining := len(set.Members)
		if renderOffset > visited {
			skipInSet := minInt(renderOffset-visited, len(set.Members))
			setRowsRemaining -= skipInSet
		}
		if setRowsRemaining <= 0 {
			visited += len(set.Members)
			continue
		}
		if includeCounts {
			fmt.Fprintf(&b, "## %s (%s)\n\ncount=%d complete=%t\n\n",
				types.SourceInventoryAdvisoryRoleLabel(set.Role), set.Role, set.Count, set.Complete)
		} else {
			fmt.Fprintf(&b, "## %s (%s)\n\ncomplete=%t\n\n",
				types.SourceInventoryAdvisoryRoleLabel(set.Role), set.Role, set.Complete)
		}
		for _, member := range set.Members {
			if visited < renderOffset {
				visited++
				continue
			}
			if emitted >= topN {
				break
			}
			line := renderSourceInventoryObservationMember(member, includeAttributes)
			visited++
			if line == "" {
				continue
			}
			b.WriteString("- ")
			b.WriteString(line)
			b.WriteByte('\n')
			emitted++
		}
		b.WriteByte('\n')
	}
	if pageAlreadyApplied && observation.Page != nil {
		fullTotal = observation.Page.Total
	}
	if fullTotal > emitted || visibleTotal > emitted {
		footerOffset := offset
		if !pageAlreadyApplied {
			footerOffset = renderOffset
		}
		end := footerOffset + emitted
		if end > fullTotal {
			end = fullTotal
		}
		fmt.Fprintf(&b, "---\nshowing rows [%d,%d) of %d visible member rows in this tool result", footerOffset, end, fullTotal)
		if sourceInventoryStringSliceContains(observation.Provenance, "repo_lens:candidate_budget_truncated") {
			b.WriteString("; the structured observation is budget-truncated, so rerun a narrower source_inventory lens before exhaustive claims.\n")
		} else {
			b.WriteString("; the structured observation stored in run state preserves the full set.\n")
		}
		if observation.Page != nil && observation.Page.NextCursor != "" {
			fmt.Fprintf(&b, "next_cursor=%s\n", observation.Page.NextCursor)
		} else if end < fullTotal {
			fmt.Fprintf(&b, "next_cursor=%d\n", end)
		}
	}
	return strings.TrimSpace(b.String())
}

func renderSourceInventoryCandidateFileSamplesView(observation types.SourceInventoryObservation, query types.SourceInventoryLensQuery) string {
	return renderSourceInventoryCandidateFileSamplesViewWithLimits(observation, query, 16, 3, 3)
}

func renderSourceInventoryCandidateFileSamplesViewWithLimits(observation types.SourceInventoryObservation, query types.SourceInventoryLensQuery, maxGroups, maxFilesPerGroup, maxItemsPerFile int) string {
	if !observation.IsActive() || sourceInventoryLensQueryOffset(query) > 0 {
		return ""
	}
	if maxGroups <= 0 || maxFilesPerGroup <= 0 || maxItemsPerFile <= 0 {
		return ""
	}
	groups := sourceInventorySuggestedFileGroups(observation, query)
	if len(groups) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Candidate File Samples to Verify (advisory)\n\n")
	b.WriteString("These are bounded examples from the same structured rows, useful after choosing a scope from the cascade guide. They are not a hard whitelist, not semantic source citations, and not a final answer slate.\n\n")
	renderedGroups := 0
	for _, group := range groups {
		if renderedGroups >= maxGroups {
			break
		}
		if len(group.Files) == 0 {
			continue
		}
		renderedGroups++
		fmt.Fprintf(&b, "- `%s` — file_count=%d candidate_count=%d — files: ",
			group.Scope, len(group.Files), group.candidateCount)
		parts := make([]string, 0, minInt(len(group.Files), maxFilesPerGroup))
		for _, file := range group.Files {
			if len(parts) >= maxFilesPerGroup {
				break
			}
			if part := renderSourceInventorySuggestedFile(file, maxItemsPerFile); part != "" {
				parts = append(parts, part)
			}
		}
		if len(parts) == 0 {
			b.WriteString("(none)")
		} else {
			b.WriteString(strings.Join(parts, "; "))
		}
		if len(group.Files) > len(parts) {
			fmt.Fprintf(&b, " (+%d more files)", len(group.Files)-len(parts))
		}
		b.WriteByte('\n')
	}
	if len(groups) > renderedGroups {
		fmt.Fprintf(&b, "\nshowing %d of %d file groups; the structured observation stored in run state preserves all candidate files.\n", renderedGroups, len(groups))
	}
	return strings.TrimSpace(b.String())
}

func renderSourceInventorySuggestedFile(file sourceInventorySuggestedFile, maxItems int) string {
	if strings.TrimSpace(file.File) == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("`")
	b.WriteString(file.File)
	b.WriteString("`")
	var meta []string
	if languages := renderSourceInventoryLanguageCounts(file.Languages); languages != "" {
		meta = append(meta, "languages="+languages)
	}
	if roles := renderSourceInventoryRoleCounts(file.RoleCounts); roles != "" {
		meta = append(meta, "roles="+roles)
	}
	candidates := renderSourceInventorySuggestedFileCandidates(file.Candidates, maxItems)
	if candidates != "" {
		meta = append(meta, "candidates: "+candidates)
	}
	if len(meta) > 0 {
		b.WriteString(" (")
		b.WriteString(strings.Join(meta, "; "))
		b.WriteString(")")
	}
	return b.String()
}

func renderSourceInventorySuggestedFileCandidates(candidates []sourceInventorySuggestedFileCandidate, maxItems int) string {
	if len(candidates) == 0 || maxItems <= 0 {
		return ""
	}
	var parts []string
	for _, candidate := range candidates {
		if len(parts) >= maxItems {
			break
		}
		name := strings.TrimSpace(candidate.Name)
		if name == "" {
			continue
		}
		role := strings.TrimSpace(string(candidate.Role))
		if role == "" {
			role = "candidate"
		}
		item := role + " `" + name + "`"
		if candidate.Line > 0 {
			item += "@" + strconvItoa(candidate.Line)
		}
		parts = append(parts, item)
	}
	if len(parts) == 0 {
		return ""
	}
	if len(candidates) > len(parts) {
		parts = append(parts, fmt.Sprintf("+%d", len(candidates)-len(parts)))
	}
	return strings.Join(parts, ", ")
}

func sourceInventorySuggestedFileGroups(observation types.SourceInventoryObservation, query types.SourceInventoryLensQuery) []sourceInventorySuggestedFileGroup {
	if !observation.IsActive() {
		return nil
	}
	groupScopes := sourceInventoryGroupScopesFromQuery(query, observation)
	groupOrder := map[string]int{}
	for i, scope := range groupScopes {
		groupOrder[scope] = i
	}
	groupsByScope := map[string]*sourceInventorySuggestedFileGroup{}
	filesByScope := map[string]map[string]*sourceInventorySuggestedFile{}
	add := func(scope, file, language string, role types.AnswerCandidateRole, name string, line int) {
		file = strings.Trim(strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`)), "/")
		if file == "" {
			return
		}
		scope = strings.Trim(strings.TrimSpace(strings.ReplaceAll(scope, `\`, `/`)), "/")
		if scope == "" {
			scope = "."
		}
		group := groupsByScope[scope]
		if group == nil {
			order := len(groupOrder) + len(groupsByScope)
			if explicit, ok := groupOrder[scope]; ok {
				order = explicit
			}
			group = &sourceInventorySuggestedFileGroup{Scope: scope, order: order}
			groupsByScope[scope] = group
			filesByScope[scope] = map[string]*sourceInventorySuggestedFile{}
		}
		files := filesByScope[scope]
		item := files[file]
		if item == nil {
			item = &sourceInventorySuggestedFile{
				File:       file,
				order:      len(files),
				RoleCounts: map[types.AnswerCandidateRole]int{},
				Languages:  map[string]int{},
				seen:       map[string]bool{},
			}
			files[file] = item
			group.Files = append(group.Files, *item)
		}
		key := sourceInventorySuggestedCandidateKey(role, name, line)
		if key == "" || item.seen[key] {
			return
		}
		item.seen[key] = true
		item.Candidates = append(item.Candidates, sourceInventorySuggestedFileCandidate{Name: name, Role: role, Line: line})
		if role != types.AnswerCandidateRoleUnknown {
			item.RoleCounts[role]++
		}
		if language = strings.TrimSpace(language); language != "" {
			item.Languages[language]++
		}
		group.candidateCount++
		group.Files[item.order] = *item
	}
	addMember := func(member types.SourceInventoryObservationMember) {
		file := sourceInventoryObservationMemberFile(member)
		if file != "" {
			scope := sourceInventorySuggestedFileScope(file, groupScopes, observation.Scopes)
			add(scope, file, member.Language, member.Role, member.Name, member.Line)
		}
		for _, attr := range member.Attributes {
			attrFile := strings.Trim(strings.TrimSpace(strings.ReplaceAll(attr.File, `\`, `/`)), "/")
			if attrFile == "" {
				continue
			}
			scope := sourceInventorySuggestedFileScope(attrFile, groupScopes, observation.Scopes)
			add(scope, attrFile, attr.Language, attr.Role, attr.Name, attr.Line)
		}
	}
	for _, set := range observation.Sets {
		for _, member := range set.Members {
			addMember(member)
		}
	}
	groups := make([]sourceInventorySuggestedFileGroup, 0, len(groupsByScope))
	for _, group := range groupsByScope {
		sort.SliceStable(group.Files, func(i, j int) bool {
			if group.Files[i].order != group.Files[j].order {
				return group.Files[i].order < group.Files[j].order
			}
			return group.Files[i].File < group.Files[j].File
		})
		groups = append(groups, *group)
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].order != groups[j].order {
			return groups[i].order < groups[j].order
		}
		return groups[i].Scope < groups[j].Scope
	})
	return groups
}

func sourceInventorySuggestedCandidateKey(role types.AnswerCandidateRole, name string, line int) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	return string(role) + "\x00" + name + "\x00" + strconvItoa(line)
}

func sourceInventorySuggestedFileScope(file string, groupScopes []string, observationScopes []string) string {
	file = strings.Trim(strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`)), "/")
	if file == "" {
		return ""
	}
	if len(groupScopes) > 1 {
		if scope := sourceInventoryBestMatchingScope(file, groupScopes); scope != "" {
			return scope
		}
	}
	baseScopes := groupScopes
	if len(baseScopes) == 0 {
		baseScopes = observationScopes
	}
	base := "."
	if len(baseScopes) == 1 {
		base = strings.Trim(strings.TrimSpace(strings.ReplaceAll(baseScopes[0], `\`, `/`)), "/")
		if base == "" {
			base = "."
		}
	}
	return sourceInventoryChildScopeForFile(file, base)
}

func sourceInventorySuggestedFilesForRequestedScope(groups []sourceInventorySuggestedFileGroup, requestedDir, requestedRel string) []sourceInventorySuggestedFile {
	requestedDir = strings.Trim(strings.TrimSpace(strings.ReplaceAll(requestedDir, `\`, `/`)), "/")
	if requestedDir == "" {
		requestedDir = "."
	}
	requestedRel = strings.Trim(strings.TrimSpace(strings.ReplaceAll(requestedRel, `\`, `/`)), "/")
	var files []sourceInventorySuggestedFile
	seen := map[string]bool{}
	addFile := func(file sourceInventorySuggestedFile) {
		if file.File == "" || file.File == requestedRel || seen[file.File] {
			return
		}
		seen[file.File] = true
		files = append(files, file)
	}
	for _, group := range groups {
		scope := strings.Trim(strings.TrimSpace(strings.ReplaceAll(group.Scope, `\`, `/`)), "/")
		if scope == "" {
			scope = "."
		}
		if scope != requestedDir {
			continue
		}
		for _, file := range group.Files {
			addFile(file)
		}
	}
	if len(files) > 0 {
		return files
	}
	for _, group := range groups {
		for _, file := range group.Files {
			if path.Dir(file.File) == requestedDir {
				addFile(file)
			}
		}
	}
	return files
}

func renderSourceInventoryGroupedObservationView(observation types.SourceInventoryObservation, query types.SourceInventoryLensQuery) string {
	if !observation.IsActive() || sourceInventoryLensQueryOffset(query) > 0 {
		return ""
	}
	groups := sourceInventoryObservationScopeGroups(observation, query)
	if len(groups) < 2 {
		return ""
	}
	const (
		maxGroups          = 48
		maxCandidatesInRow = 5
	)
	var b strings.Builder
	b.WriteString("## Scope-grouped Candidate View (advisory)\n\n")
	b.WriteString("Use this navigation view to choose what to verify next. It groups the same structured rows by scope; it does not assert final answer membership or entry-point identity.\n\n")
	b.WriteString("If the downstream task needs one candidate per scope, treat groups with `candidate_count` other than 1 as ambiguous and verify the listed files before selecting or excluding a candidate.\n\n")
	renderedGroups := 0
	for _, group := range groups {
		if renderedGroups >= maxGroups {
			break
		}
		if len(group.Candidates) == 0 {
			continue
		}
		renderedGroups++
		fmt.Fprintf(&b, "- `%s` — candidate_count=%d", group.Scope, len(group.Candidates))
		if roles := renderSourceInventoryRoleCounts(group.RoleCounts); roles != "" {
			fmt.Fprintf(&b, " roles=%s", roles)
		}
		if languages := renderSourceInventoryLanguageCounts(group.Languages); languages != "" {
			fmt.Fprintf(&b, " languages=%s", languages)
		}
		b.WriteString(" — top_candidates: ")
		parts := make([]string, 0, maxCandidatesInRow)
		for _, candidate := range group.Candidates {
			if len(parts) >= maxCandidatesInRow {
				break
			}
			if part := renderSourceInventoryGroupedCandidate(candidate); part != "" {
				parts = append(parts, part)
			}
		}
		if len(parts) == 0 {
			b.WriteString("(none)")
		} else {
			b.WriteString(strings.Join(parts, "; "))
		}
		if len(group.Candidates) > len(parts) {
			fmt.Fprintf(&b, " (+%d more)", len(group.Candidates)-len(parts))
		}
		b.WriteByte('\n')
	}
	if len(groups) > renderedGroups {
		fmt.Fprintf(&b, "\nshowing %d of %d scope groups; the structured observation stored in run state preserves all grouped rows.\n", renderedGroups, len(groups))
	}
	return strings.TrimSpace(b.String())
}

func sourceInventoryObservationScopeGroups(observation types.SourceInventoryObservation, query types.SourceInventoryLensQuery) []sourceInventoryObservationScopeGroup {
	groupScopes := sourceInventoryGroupScopesFromQuery(query, observation)
	groupOrder := map[string]int{}
	for i, scope := range groupScopes {
		groupOrder[scope] = i
	}
	groupsByScope := map[string]*sourceInventoryObservationScopeGroup{}
	addCandidate := func(member types.SourceInventoryObservationMember) {
		if strings.TrimSpace(member.Name) == "" || !sourceInventoryObservationMemberGroupable(member) {
			return
		}
		scope := sourceInventoryObservationMemberGroupScope(member, groupScopes, observation.Scopes)
		if scope == "" {
			return
		}
		group := groupsByScope[scope]
		if group == nil {
			order := len(groupOrder) + len(groupsByScope)
			if explicit, ok := groupOrder[scope]; ok {
				order = explicit
			}
			group = &sourceInventoryObservationScopeGroup{
				Scope:      scope,
				order:      order,
				RoleCounts: map[types.AnswerCandidateRole]int{},
				Languages:  map[string]int{},
			}
			groupsByScope[scope] = group
		}
		group.Candidates = append(group.Candidates, member)
		group.RoleCounts[member.Role]++
		if lang := strings.TrimSpace(member.Language); lang != "" {
			group.Languages[lang]++
		}
		for _, attr := range member.Attributes {
			if lang := strings.TrimSpace(attr.Language); lang != "" {
				group.Languages[lang]++
			}
			if attr.Role != types.AnswerCandidateRoleUnknown {
				group.RoleCounts[attr.Role]++
			}
		}
	}
	for _, set := range observation.Sets {
		for _, member := range set.Members {
			addCandidate(member)
		}
	}
	groups := make([]sourceInventoryObservationScopeGroup, 0, len(groupsByScope))
	for _, group := range groupsByScope {
		sourceInventorySortObservationGroupCandidates(group.Candidates)
		groups = append(groups, *group)
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].order != groups[j].order {
			return groups[i].order < groups[j].order
		}
		return groups[i].Scope < groups[j].Scope
	})
	return groups
}

func sourceInventoryObservationMemberGroupable(member types.SourceInventoryObservationMember) bool {
	switch member.Role {
	case types.AnswerCandidateRoleFunction,
		types.AnswerCandidateRoleMethod,
		types.AnswerCandidateRoleType,
		types.AnswerCandidateRoleConstant,
		types.AnswerCandidateRoleVariable,
		types.AnswerCandidateRoleField,
		types.AnswerCandidateRolePackage,
		types.AnswerCandidateRoleFile,
		types.AnswerCandidateRoleConfigFile,
		types.AnswerCandidateRoleConfigKey,
		types.AnswerCandidateRoleRoute,
		types.AnswerCandidateRoleImportPath,
		types.AnswerCandidateRoleLiteralValue:
		return true
	default:
		return len(member.Attributes) > 0
	}
}

func sourceInventoryGroupScopesFromQuery(query types.SourceInventoryLensQuery, observation types.SourceInventoryObservation) []string {
	seen := map[string]bool{}
	var scopes []string
	add := func(raw string) {
		scope := strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`))
		if scope == "" {
			return
		}
		if scope == "." || scope == "./" || scope == "/" {
			scope = "."
		} else {
			scope = normalizeSourceInventoryScopeSurface(scope)
		}
		if scope == "" {
			scope = "."
		}
		if seen[scope] {
			return
		}
		seen[scope] = true
		scopes = append(scopes, scope)
	}
	for _, scope := range query.Scopes {
		add(scope)
	}
	if len(scopes) == 0 {
		for _, scope := range observation.Scopes {
			add(scope)
		}
	}
	return scopes
}

func sourceInventoryObservationMemberGroupScope(member types.SourceInventoryObservationMember, groupScopes []string, observationScopes []string) string {
	file := sourceInventoryObservationMemberFile(member)
	if len(groupScopes) > 1 {
		if scope := sourceInventoryBestMatchingScope(file, groupScopes); scope != "" {
			return scope
		}
	}
	baseScopes := groupScopes
	if len(baseScopes) == 0 {
		baseScopes = observationScopes
	}
	base := "."
	if len(baseScopes) == 1 {
		base = strings.Trim(strings.TrimSpace(strings.ReplaceAll(baseScopes[0], `\`, `/`)), "/")
		if base == "" {
			base = "."
		}
	}
	if file == "" {
		if base != "." {
			return base
		}
		return strings.TrimSpace(member.Name)
	}
	return sourceInventoryChildScopeForFile(file, base)
}

func sourceInventoryObservationMemberFile(member types.SourceInventoryObservationMember) string {
	file := strings.Trim(strings.TrimSpace(strings.ReplaceAll(member.File, `\`, `/`)), "/")
	if file != "" {
		return file
	}
	ref := strings.TrimSpace(member.SupportRef)
	if ref == "" {
		return ""
	}
	if _, loc, ok := aggregateMemberSupportRefParts(ref); ok {
		file, _ := aggregateLocationParts(loc)
		return strings.Trim(strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`)), "/")
	}
	return ""
}

func sourceInventoryBestMatchingScope(file string, scopes []string) string {
	file = strings.Trim(strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`)), "/")
	best := ""
	for _, raw := range scopes {
		scope := strings.Trim(strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`)), "/")
		if scope == "" || scope == "." {
			if best == "" {
				best = "."
			}
			continue
		}
		if file == scope || strings.HasPrefix(file, scope+"/") {
			if len(scope) > len(best) {
				best = scope
			}
		}
	}
	return best
}

func sourceInventoryChildScopeForFile(file, base string) string {
	file = strings.Trim(strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`)), "/")
	base = strings.Trim(strings.TrimSpace(strings.ReplaceAll(base, `\`, `/`)), "/")
	if file == "" {
		return ""
	}
	if base == "" || base == "." {
		parts := strings.Split(file, "/")
		if len(parts) <= 1 {
			return file
		}
		return parts[0]
	}
	if file == base {
		return base
	}
	if strings.HasPrefix(file, base+"/") {
		rest := strings.TrimPrefix(file, base+"/")
		parts := strings.Split(rest, "/")
		if len(parts) == 0 || parts[0] == "" {
			return base
		}
		return strings.Trim(base+"/"+parts[0], "/")
	}
	return path.Dir(file)
}

func sourceInventorySortObservationGroupCandidates(candidates []types.SourceInventoryObservationMember) {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Exported != candidates[j].Exported {
			return candidates[i].Exported
		}
		if candidates[i].Role != candidates[j].Role {
			return string(candidates[i].Role) < string(candidates[j].Role)
		}
		if candidates[i].File != candidates[j].File {
			return candidates[i].File < candidates[j].File
		}
		if candidates[i].Line != candidates[j].Line {
			if candidates[i].Line == 0 {
				return false
			}
			if candidates[j].Line == 0 {
				return true
			}
			return candidates[i].Line < candidates[j].Line
		}
		return candidates[i].Name < candidates[j].Name
	})
}

func renderSourceInventoryGroupedCandidate(member types.SourceInventoryObservationMember) string {
	name := strings.TrimSpace(member.Name)
	if name == "" {
		return ""
	}
	role := strings.TrimSpace(string(member.Role))
	if role == "" {
		role = "candidate"
	}
	item := role + " `" + name + "`"
	loc := sourceInventoryObservationMemberFile(member)
	if loc != "" && member.Line > 0 {
		loc = loc + ":" + strconvItoa(member.Line)
	}
	if loc != "" {
		item += " @ " + loc
	}
	if len(member.Attributes) > 0 {
		if attrs := renderSourceInventoryObservationAttributes(member.Attributes); attrs != "" {
			item += " {" + attrs + "}"
		}
	}
	return item
}

func renderSourceInventoryRoleCounts(counts map[types.AnswerCandidateRole]int) string {
	if len(counts) == 0 {
		return ""
	}
	var keys []types.AnswerCandidateRole
	for role, count := range counts {
		if role == types.AnswerCandidateRoleUnknown || count <= 0 {
			continue
		}
		keys = append(keys, role)
	}
	sort.Slice(keys, func(i, j int) bool { return string(keys[i]) < string(keys[j]) })
	parts := make([]string, 0, len(keys))
	for _, role := range keys {
		parts = append(parts, fmt.Sprintf("%s:%d", role, counts[role]))
	}
	return strings.Join(parts, ",")
}

func renderSourceInventoryLanguageCounts(counts map[string]int) string {
	if len(counts) == 0 {
		return ""
	}
	var keys []string
	for lang, count := range counts {
		if strings.TrimSpace(lang) == "" || count <= 0 {
			continue
		}
		keys = append(keys, lang)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, lang := range keys {
		parts = append(parts, fmt.Sprintf("%s:%d", lang, counts[lang]))
	}
	return strings.Join(parts, ",")
}

func renderSourceInventorySourceClassCounts(classes []types.SourceInventorySourceClassCount) string {
	if len(classes) == 0 {
		return ""
	}
	order := []types.SourcePathRole{
		types.SourcePathRoleProduction,
		types.SourcePathRoleTest,
		types.SourcePathRoleFixture,
		types.SourcePathRoleExample,
		types.SourcePathRoleDocumentation,
		types.SourcePathRolePromptSupport,
		types.SourcePathRoleThirdParty,
		types.SourcePathRoleVendor,
		types.SourcePathRoleGenerated,
	}
	byRole := make(map[types.SourcePathRole]types.SourceInventorySourceClassCount, len(classes))
	for _, class := range classes {
		if class.Role == types.SourcePathRoleUnknown || class.Count <= 0 {
			continue
		}
		byRole[class.Role] = class
	}
	var parts []string
	for _, role := range order {
		class, ok := byRole[role]
		if !ok {
			continue
		}
		delete(byRole, role)
		parts = append(parts, renderSourceInventorySourceClassCount(class))
	}
	if len(byRole) > 0 {
		var rest []string
		for role := range byRole {
			rest = append(rest, string(role))
		}
		sort.Strings(rest)
		for _, role := range rest {
			parts = append(parts, renderSourceInventorySourceClassCount(byRole[types.SourcePathRole(role)]))
		}
	}
	return strings.Join(parts, ",")
}

func renderSourceInventorySourceClassCount(class types.SourceInventorySourceClassCount) string {
	item := fmt.Sprintf("%s:%d", class.Role, class.Count)
	if !class.Complete {
		item += "(partial)"
	}
	return item
}

func sourceInventoryLensQueryOffset(query types.SourceInventoryLensQuery) int {
	return sourceinventory.CursorOffset(query.Offset, query.Cursor)
}

func renderSourceInventoryObservationMember(member types.SourceInventoryObservationMember, includeAttributes bool) string {
	name := strings.TrimSpace(member.Name)
	if name == "" {
		return ""
	}
	parts := []string{"`" + name + "`"}
	loc := strings.TrimSpace(member.File)
	if loc != "" && member.Line > 0 {
		loc = loc + ":" + strconvItoa(member.Line)
	}
	if loc != "" {
		parts = append(parts, "@ "+loc)
	}
	if member.Language != "" {
		parts = append(parts, "language="+member.Language)
	}
	if member.SupportRef != "" {
		parts = append(parts, "support_ref=`"+member.SupportRef+"`")
	}
	if member.CoverageState != "" {
		parts = append(parts, "coverage="+string(member.CoverageState))
	}
	if note := strings.TrimSpace(member.Note); note != "" {
		parts = append(parts, "note: "+note)
	}
	if includeAttributes {
		if attrs := renderSourceInventoryObservationAttributes(member.Attributes); attrs != "" {
			parts = append(parts, attrs)
		}
	}
	return strings.Join(parts, " — ")
}

func renderSourceInventoryObservationAttributes(attrs []types.SourceInventoryObservationAttribute) string {
	if len(attrs) == 0 {
		return ""
	}
	const max = 4
	var parts []string
	for _, attr := range attrs {
		if len(parts) >= max {
			break
		}
		name := strings.TrimSpace(attr.Name)
		if name == "" {
			continue
		}
		loc := strings.TrimSpace(attr.File)
		if loc != "" && attr.Line > 0 {
			loc = loc + ":" + strconvItoa(attr.Line)
		}
		role := strings.TrimSpace(string(attr.Role))
		if role == "" {
			role = "candidate"
		}
		item := role + " `" + name + "`"
		if loc != "" {
			item += " @ " + loc
		}
		if attr.Ambiguity != "" {
			item += " ambiguity=" + attr.Ambiguity
		}
		parts = append(parts, item)
	}
	if len(parts) == 0 {
		return ""
	}
	suffix := ""
	if len(attrs) > len(parts) {
		suffix = fmt.Sprintf(" (+%d more)", len(attrs)-len(parts))
	}
	return "attributes: " + strings.Join(parts, "; ") + suffix
}
