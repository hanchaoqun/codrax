package repomap

import (
	"path"
	"sort"
	"strconv"
	"strings"

	ctypes "github.com/hanchaoqun/codrax/internal/types"
)

// repoMapSourceInventoryRefinementWithNarrowing wraps the historical
// source_inventory refinement and attaches typed ParamNarrowingSuggestions
// derived from the typed observation only (member file paths, per-role set
// counts, page limits — never rendered prose). The new suggestions route
// through the SAME runtime-source authority gate as the navigation
// refinements (repoMapRuntimeCurrentSourceAvoidsSourceInventoryRefinement):
// when runtime-source authority is active, no source-inventory scope/roles
// narrowing is suggested, preserving the d0bb5bc2/5d176481 shared-carrier
// routing.
func repoMapSourceInventoryRefinementWithNarrowing(ctx *ctypes.BusContext, observation ctypes.SourceInventoryObservation, query ctypes.SourceInventoryLensQuery) *ctypes.ToolRefinementHint {
	hint := repoMapSourceInventoryRefinement(observation, query)
	if hint == nil || repoMapRuntimeCurrentSourceAvoidsSourceInventoryRefinement(ctx) {
		return hint
	}
	suggestions := repoMapSourceInventoryNarrowingSuggestions(observation, query, *hint)
	if len(suggestions) == 0 {
		return hint
	}
	out := *hint
	out.ParamNarrowingSuggestions = suggestions
	normalized := ctypes.NormalizeToolRefinementHint(out)
	return &normalized
}

func repoMapSourceInventoryNarrowingSuggestions(observation ctypes.SourceInventoryObservation, query ctypes.SourceInventoryLensQuery, hint ctypes.ToolRefinementHint) []ctypes.ToolParamNarrowingSuggestion {
	var out []ctypes.ToolParamNarrowingSuggestion
	if hint.CandidateBudgetTruncated {
		if scopes := repoMapSourceInventoryScopeCandidates(observation); len(scopes) > 0 {
			out = append(out, repoMapScopeNarrowingSuggestion(scopes, ctypes.ToolParamNarrowReasonCandidateBudgetTruncated))
		}
		if len(query.Roles) == 0 || len(query.Roles) > 2 {
			if roles := repoMapSourceInventoryTopRoles(observation); len(roles) > 0 {
				out = append(out, ctypes.ToolParamNarrowingSuggestion{
					Param:      "roles",
					Priority:   2,
					Suggested:  strings.Join(roles, ","),
					ReasonCode: ctypes.ToolParamNarrowReasonCandidateBudgetTruncated,
				})
			}
		}
		if observation.Page != nil && observation.Page.Limit > 0 {
			out = append(out, ctypes.ToolParamNarrowingSuggestion{
				Param:      "top_n",
				Priority:   3,
				Suggested:  strconv.Itoa(observation.Page.Limit),
				ReasonCode: ctypes.ToolParamNarrowReasonCandidateBudgetTruncated,
			})
		}
	}
	if cursor := strings.TrimSpace(hint.NextCursor); cursor != "" {
		out = append(out, ctypes.ToolParamNarrowingSuggestion{
			Param:      "cursor",
			Priority:   4,
			Suggested:  cursor,
			ReasonCode: ctypes.ToolParamNarrowReasonPageIncomplete,
		})
	}
	return out
}

// repoMapSourceInventoryScopeCandidates derives concrete top directory-scope
// candidates from the typed observation: member file paths first, source-class
// samples as fallback. Ordering is count-desc then name-asc (the
// formatCategoryCounts discipline), bounded to 3 candidates.
func repoMapSourceInventoryScopeCandidates(observation ctypes.SourceInventoryObservation) []string {
	counts := map[string]int{}
	add := func(raw string) {
		if scope := repoMapNarrowingScopeFromPath(raw); scope != "" {
			counts[scope]++
		}
	}
	for _, set := range observation.Sets {
		for _, member := range set.Members {
			add(member.File)
		}
	}
	if len(counts) == 0 {
		for _, class := range observation.SourceClasses {
			for _, sample := range class.Samples {
				add(sample)
			}
		}
	}
	return repoMapTopCountedNames(counts, 3)
}

// repoMapSourceInventoryTopRoles picks the highest-count roles from the
// observation's typed per-role member sets (count-desc then name-asc, top 2).
func repoMapSourceInventoryTopRoles(observation ctypes.SourceInventoryObservation) []string {
	counts := map[string]int{}
	for _, set := range observation.Sets {
		role := strings.TrimSpace(string(set.Role))
		if role == "" {
			continue
		}
		count := set.Total
		if count < set.Count {
			count = set.Count
		}
		if count <= 0 {
			continue
		}
		counts[role] += count
	}
	return repoMapTopCountedNames(counts, 2)
}

func repoMapTopCountedNames(counts map[string]int, limit int) []string {
	if len(counts) == 0 {
		return nil
	}
	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if counts[names[i]] != counts[names[j]] {
			return counts[names[i]] > counts[names[j]]
		}
		return names[i] < names[j]
	})
	if limit > 0 && len(names) > limit {
		names = names[:limit]
	}
	return names
}

// repoMapNarrowingScopeFromPath reduces a repo-relative file path to a
// directory-scope candidate bounded to two segments (e.g.
// internal/tool/builtin.go -> internal/tool).
func repoMapNarrowingScopeFromPath(raw string) string {
	p := strings.Trim(strings.ReplaceAll(strings.TrimSpace(raw), `\`, `/`), "/")
	if p == "" || p == "." {
		return ""
	}
	dir := path.Dir(p)
	if dir == "" || dir == "." || dir == "/" {
		return ""
	}
	parts := strings.Split(dir, "/")
	if len(parts) > 2 {
		parts = parts[:2]
	}
	return strings.Join(parts, "/")
}

func repoMapNavigationRefinement(ctx *ctypes.BusContext, graph *Graph, p repoMapParams, params ViewParams, data *ViewData) *ctypes.ToolRefinementHint {
	if graph == nil {
		return nil
	}
	view := strings.TrimSpace(p.View)
	if view == "" {
		view = "overview"
	}
	switch view {
	case "source_inventory", "call_path", "edit_impact", "implementers":
		return nil
	}

	hint := ctypes.ToolRefinementHint{
		PreferredNextTool: "repo_map",
		PreferredParams: map[string]string{
			"view": view,
		},
	}
	if path := strings.TrimSpace(p.Path); path != "" {
		hint.PreferredParams["path"] = path
	}
	if topN := p.TopN; topN > 0 {
		hint.PreferredParams["top_n"] = strconv.Itoa(topN)
	}
	if query := strings.TrimSpace(params.Query); query != "" {
		hint.PreferredParams["query"] = query
	}

	tier := GraphSizeTier(graph)
	viewItems := repoMapViewDataItemCount(data)
	switch view {
	case "overview":
		if tier < SizeTierLarge {
			return nil
		}
		hint.ReasonCode = "repo_map_overview_large_scope"
		hint.ResultTruncated = true
		hint.PreferredParams["view"] = "task_map"
		hint.RequiredFields = []string{"query"}
	case "task_map":
		if strings.TrimSpace(params.Query) == "" {
			hint.ReasonCode = "repo_map_task_map_missing_query"
			hint.ResultTruncated = true
			hint.RequiredFields = []string{"query"}
			break
		}
		if repoMapExplicitTopNBroad(view, tier, p.TopN) {
			hint.ReasonCode = "repo_map_task_map_large_top_n"
			hint.ResultTruncated = true
			hint.PreferredParams["view"] = "relation_map"
			hint.RequiredFields = []string{"scope", "sources", "relation_kinds"}
		}
	case "relation_map":
		scopes := repoMapRelationScopes(p)
		if len(params.Sources) == 0 && strings.TrimSpace(params.Query) == "" && len(scopes) == 0 {
			hint.ReasonCode = "repo_map_relation_map_broad_fallback"
			hint.ResultTruncated = true
			hint.RequiredFields = []string{"query", "scope", "sources"}
			hint.ParamNarrowingSuggestions = repoMapViewDataScopeNarrowingSuggestions(data)
		}
	case "file_map":
		if tier < SizeTierMedium && viewItems < 80 && !repoMapExplicitTopNBroad(view, tier, p.TopN) {
			return nil
		}
		hint.ReasonCode = "repo_map_file_map_large_scope"
		hint.ResultTruncated = true
		if repoMapRuntimeCurrentSourceAvoidsSourceInventoryRefinement(ctx) {
			// Runtime-source authority active: no source-inventory scope/roles
			// suggestions here either — the narrowing rows are gated identically
			// to the preferred-view routing (2184de6c/5d176481 discipline).
			hint.PreferredParams["view"] = "task_map"
			if _, ok := hint.PreferredParams["top_n"]; !ok {
				hint.PreferredParams["top_n"] = "12"
			}
			if strings.TrimSpace(params.Query) == "" {
				hint.RequiredFields = []string{"query"}
			}
			break
		}
		hint.PreferredParams["view"] = "source_inventory"
		hint.PreferredParams["include_attributes"] = "false"
		hint.RequiredFields = []string{"scope", "roles"}
		hint.ParamNarrowingSuggestions = repoMapViewDataScopeNarrowingSuggestions(data)
	case "semantic_subgraph":
		if tier < SizeTierLarge && viewItems < 18 && !repoMapExplicitTopNBroad(view, tier, p.TopN) {
			return nil
		}
		hint.ReasonCode = "repo_map_semantic_subgraph_large_scope"
		hint.ResultTruncated = true
		hint.PreferredParams["view"] = "relation_map"
		hint.RequiredFields = []string{"scope", "sources"}
		hint.ParamNarrowingSuggestions = repoMapViewDataScopeNarrowingSuggestions(data)
	}

	out := ctypes.NormalizeToolRefinementHint(hint)
	if out.Empty() {
		return nil
	}
	return &out
}

func repoMapRuntimeCurrentSourceAvoidsSourceInventoryRefinement(ctx *ctypes.BusContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	rm := ctypes.RuntimeSourceAuthorityRequestModelFromBusContext(ctx)
	if rm == nil {
		return false
	}
	if ctypes.SourceInventoryPrincipalNavigationActive(*rm) {
		return false
	}
	authority := ctypes.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(ctx, ctypes.ObservationLedger{})
	if !authority.Active {
		return false
	}
	if !ctypes.RuntimeSourceAuthorityRequestCarrierActive(ctx.TurnRouteHint, rm, authority) {
		return false
	}
	return authority.HasRuntimeSourceHandoffSurface()
}

func repoMapExplicitTopNBroad(view string, tier SizeTier, topN int) bool {
	if topN <= 0 {
		return false
	}
	def := DefaultTopN(view, tier)
	return def > 0 && topN > 2*def
}

// repoMapViewDataScopeNarrowingSuggestions derives a scope(1) narrowing row
// from the typed ViewData item file paths (tool-computed counts, count-desc
// then name-asc). Returns nil when the view exposes no file-backed items.
func repoMapViewDataScopeNarrowingSuggestions(data *ViewData) []ctypes.ToolParamNarrowingSuggestion {
	counts := map[string]int{}
	var walk func(sections []ViewSection)
	walk = func(sections []ViewSection) {
		for i := range sections {
			for _, item := range sections[i].Items {
				if scope := repoMapNarrowingScopeFromPath(item.File); scope != "" {
					counts[scope]++
				}
			}
			walk(sections[i].Subsections)
		}
	}
	if data != nil {
		walk(data.Sections)
	}
	scopes := repoMapTopCountedNames(counts, 3)
	if len(scopes) == 0 {
		return nil
	}
	return []ctypes.ToolParamNarrowingSuggestion{repoMapScopeNarrowingSuggestion(scopes, ctypes.ToolParamNarrowReasonEntriesOverThreshold)}
}

// repoMapScopeNarrowingSuggestion mirrors the PreferredParams convention at
// the source-inventory refinement site: `scope` is a SINGLE-path param
// (exact-or-prefix matching, nothing splits it on commas), so multiple
// directory candidates ride the plural `scopes` param, whose schema is a
// split-string array and therefore verbatim-adoptable as a comma list.
func repoMapScopeNarrowingSuggestion(scopes []string, reason string) ctypes.ToolParamNarrowingSuggestion {
	param := "scope"
	if len(scopes) > 1 {
		param = "scopes"
	}
	return ctypes.ToolParamNarrowingSuggestion{
		Param:      param,
		Priority:   1,
		Suggested:  strings.Join(scopes, ","),
		ReasonCode: reason,
	}
}

func repoMapViewDataItemCount(data *ViewData) int {
	if data == nil {
		return 0
	}
	count := 0
	for i := range data.Sections {
		count += repoMapViewSectionItemCount(&data.Sections[i])
	}
	return count
}

func repoMapViewSectionItemCount(section *ViewSection) int {
	if section == nil {
		return 0
	}
	count := len(section.Items)
	for i := range section.Subsections {
		count += repoMapViewSectionItemCount(&section.Subsections[i])
	}
	return count
}
