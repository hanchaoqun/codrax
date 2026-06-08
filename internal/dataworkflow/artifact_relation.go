package dataworkflow

import (
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func ArtifactRelationsFromProjections(projections []ArtifactSchemaProjection, limit int) []ArtifactRelation {
	if len(projections) == 0 || limit == 0 {
		return nil
	}
	if limit < 0 {
		limit = len(projections) * len(projections)
	}
	var candidates []ArtifactRelation
	for _, base := range projections {
		baseAlias := firstArtifactAlias(base)
		if baseAlias == "" || len(base.Fields) == 0 || !ArtifactUsableForRecordAction(base) {
			continue
		}
		for _, lookup := range projections {
			lookupAlias := firstArtifactAlias(lookup)
			if lookupAlias == "" || normalizeAccessPath(lookupAlias) == normalizeAccessPath(baseAlias) || len(lookup.Fields) == 0 {
				continue
			}
			if lookup.NodeClass == ArtifactNodeClassDiagnosticChild || lookup.NodeClass == ArtifactNodeClassWorkflowLedger {
				continue
			}
			baseFields, lookupFields := structuralRelationFields(base.Fields, lookup.Fields, "", "", 4)
			if len(baseFields) == 0 || len(lookupFields) == 0 {
				continue
			}
			valueFields := relationLookupValueFields(lookup.Fields, lookupFields, 8)
			if len(valueFields) == 0 {
				continue
			}
			score := len(baseFields)
			evidence := []string{"common schema key fields"}
			if ArtifactLineageContains(lookup, baseAlias) || ArtifactLineageContains(base, lookupAlias) {
				score += 4
				evidence = append(evidence, "compatible artifact lineage")
			}
			if artifactKindHasPrefix(lookup.Kind, dataquery.DataActionNormalizeEntities, dataquery.DataActionEnrichRecords, dataquery.DataActionApplyResolutions) ||
				strings.Contains(strings.ToLower(strings.TrimSpace(lookup.Kind)), "entity_resolution") {
				score += 2
				evidence = append(evidence, "relation-producing action kind")
			}
			candidates = append(candidates, ArtifactRelation{
				BaseAlias:         baseAlias,
				LookupAlias:       lookupAlias,
				BaseNodeID:        strings.TrimSpace(base.ID),
				LookupNodeID:      strings.TrimSpace(lookup.ID),
				BaseFields:        baseFields,
				LookupFields:      lookupFields,
				LookupValueFields: valueFields,
				MatchMode:         "exact",
				Evidence:          cleanStrings(evidence),
				Score:             score,
			})
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		if candidates[i].BaseAlias != candidates[j].BaseAlias {
			return candidates[i].BaseAlias < candidates[j].BaseAlias
		}
		return candidates[i].LookupAlias < candidates[j].LookupAlias
	})
	if limit > 0 && len(candidates) > limit {
		candidates = candidates[:limit]
	}
	return candidates
}

func relationLookupValueFields(fields, keyFields []string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	keySet := map[string]bool{}
	for _, field := range cleanStrings(keyFields) {
		keySet[strings.ToLower(strings.TrimSpace(field))] = true
	}
	var out []string
	seen := map[string]bool{}
	for _, field := range cleanStrings(fields) {
		key := strings.ToLower(strings.TrimSpace(field))
		if key == "" || seen[key] || keySet[key] || !FieldUsableForRecordJoin(field) {
			continue
		}
		seen[key] = true
		out = append(out, field)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func relationLimitForArtifactGraph(nodeLimit, projectionCount int) int {
	if projectionCount <= 1 {
		return 0
	}
	if nodeLimit <= 0 {
		nodeLimit = 16
	}
	limit := nodeLimit * 2
	if limit < 8 {
		limit = 8
	}
	if limit > 48 {
		limit = 48
	}
	return limit
}
