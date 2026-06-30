package repomap

import (
	"strconv"
	"strings"

	ctypes "github.com/hanchaoqun/codrax/internal/types"
)

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
		}
	case "file_map":
		if tier < SizeTierMedium && viewItems < 80 && !repoMapExplicitTopNBroad(view, tier, p.TopN) {
			return nil
		}
		hint.ReasonCode = "repo_map_file_map_large_scope"
		hint.ResultTruncated = true
		if repoMapRuntimeCurrentSourceAvoidsSourceInventoryRefinement(ctx) {
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
	case "semantic_subgraph":
		if tier < SizeTierLarge && viewItems < 18 && !repoMapExplicitTopNBroad(view, tier, p.TopN) {
			return nil
		}
		hint.ReasonCode = "repo_map_semantic_subgraph_large_scope"
		hint.ResultTruncated = true
		hint.PreferredParams["view"] = "relation_map"
		hint.RequiredFields = []string{"scope", "sources"}
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
	return authority.CurrentSourceRequirement != ctypes.RuntimeSourceRequirementNone ||
		authority.CurrentSourceRequired ||
		authority.CanHardBlockCompletion ||
		authority.CanDowngradeToCaveat ||
		authority.CanUseRuntimeOnlyWithCaveat ||
		authority.CanCompleteWithCombinedProof ||
		authority.RuntimeOnlySufficient
}

func repoMapExplicitTopNBroad(view string, tier SizeTier, topN int) bool {
	if topN <= 0 {
		return false
	}
	def := DefaultTopN(view, tier)
	return def > 0 && topN > 2*def
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
