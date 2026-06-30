package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func enrichSourceInventoryProfileFromAnalyzerPrescan(ctx *types.BusContext, rm *types.RequestModel, raw string) string {
	if ctx == nil || ctx.Mutable == nil || rm == nil {
		return ""
	}
	if types.SourceInventoryLaneConflictsWithRoleBinding(*rm) ||
		types.HasTypedRelationMemberSetShape(*rm) ||
		types.SourceInventoryLaneConflictsWithRelationFlow(*rm) ||
		rm.HasObservationOnlyRuntimeArtifact() {
		return ""
	}
	observation, ok := sourceInventoryAnalyzerPrescanObservation(ctx)
	if !ok {
		return ""
	}
	if rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		if !types.IsTypedSourceEnumerationShape(*rm) {
			return ""
		}
		rm.SourceInventoryProfile = &types.SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       sourceInventoryDefaultQueryEnumerationRoles(),
			RequestedFields: []types.SourceInventoryRequestedField{
				types.SourceInventoryFieldName,
				types.SourceInventoryFieldLocation,
				types.SourceInventoryFieldSummary,
			},
			Confidence: 0.50,
			Rationale:  "synthesized from typed analyzer source-inventory prescan",
		}
	}
	profile := rm.SourceInventoryProfile
	changedQuotes := mergeSourceInventoryProfileQuotes(profile, sourceInventoryAnalyzerPrescanQuotes(raw, *rm, observation))
	if changedQuotes {
		if profile.Confidence < 0.55 {
			profile.Confidence = 0.55
		}
		if strings.TrimSpace(profile.Rationale) == "" {
			profile.Rationale = "enriched from typed analyzer source-inventory prescan"
		}
		return "source_inventory_profile enriched from typed analyzer source-inventory prescan"
	}
	return ""
}

func sourceInventoryAnalyzerPrescanObservation(ctx *types.BusContext) (types.SourceInventoryObservation, bool) {
	if ctx == nil || ctx.Mutable == nil {
		return types.SourceInventoryObservation{}, false
	}
	var out types.SourceInventoryObservation
	active := false
	for _, result := range ctx.Mutable.DispatchToolResults() {
		if !result.Success || types.CanonicalToolName(result.ToolName) != "repo_map" || result.SourceInventory == nil ||
			!result.SourceInventory.IsActive() {
			continue
		}
		current := types.CloneSourceInventoryObservation(*result.SourceInventory)
		if out.IsActive() {
			out = types.MergeSourceInventoryObservation(out, current)
		} else {
			out = current
		}
		active = true
	}
	if active {
		return out, true
	}
	current := ctx.Mutable.SourceInventoryObservation()
	if current.IsActive() && sourceInventoryObservationProvenanceContains(current, types.SourceInventoryProvenanceStageAnalyze) {
		return current, true
	}
	return types.SourceInventoryObservation{}, false
}

func sourceInventoryAnalyzerPrescanQuotes(raw string, rm types.RequestModel, observation types.SourceInventoryObservation) []string {
	seen := map[string]bool{}
	var out []string
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || !sourceQuotePresentInCurrentRequest(raw, value) {
			return
		}
		key := strings.ToLower(value)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, value)
	}
	if rm.SourceInventoryProfile != nil {
		for _, quote := range rm.SourceInventoryProfile.SourceQuotes {
			add(quote)
		}
	}
	for _, entity := range rm.AnalyzerHints.Entities {
		add(entity)
	}
	for _, set := range observation.Sets {
		for _, member := range set.Members {
			for _, term := range member.SurfaceTerms {
				add(term)
			}
			for _, attr := range member.Attributes {
				for _, term := range attr.SurfaceTerms {
					add(term)
				}
			}
		}
	}
	return out
}

func mergeSourceInventoryProfileQuotes(profile *types.SourceInventoryProfile, quotes []string) bool {
	if profile == nil || len(quotes) == 0 {
		return false
	}
	seen := map[string]bool{}
	var out []string
	for _, quote := range profile.SourceQuotes {
		quote = strings.TrimSpace(quote)
		if quote == "" {
			continue
		}
		key := strings.ToLower(quote)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, quote)
	}
	changed := len(out) != len(profile.SourceQuotes)
	for _, quote := range quotes {
		quote = strings.TrimSpace(quote)
		if quote == "" {
			continue
		}
		key := strings.ToLower(quote)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, quote)
		changed = true
	}
	if changed {
		profile.SourceQuotes = out
	}
	return changed
}

func sourceInventoryObservationProvenanceContains(observation types.SourceInventoryObservation, want string) bool {
	for _, provenance := range observation.Provenance {
		if strings.TrimSpace(provenance) == want {
			return true
		}
	}
	return false
}
