package dataworkflow

import (
	"encoding/json"
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
			baseFields, lookupFields, evidence := relationFieldsFromTypedMetadata(base, lookup, 4)
			score := len(baseFields) + 6
			if len(baseFields) == 0 || len(lookupFields) == 0 {
				baseFields, lookupFields = structuralRelationFields(base.Fields, lookup.Fields, "", "", 4)
				evidence = []string{"common schema key fields"}
				score = len(baseFields)
			}
			if len(baseFields) == 0 || len(lookupFields) == 0 {
				continue
			}
			valueFields := relationLookupValueFields(lookup.Fields, lookupFields, 8)
			if len(valueFields) == 0 {
				continue
			}
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

func relationFieldsFromTypedMetadata(base, lookup ArtifactSchemaProjection, limit int) ([]string, []string, []string) {
	if limit <= 0 || !ArtifactLineageContains(lookup, firstArtifactAlias(base)) {
		return nil, nil, nil
	}
	baseFields := relationMetadataFieldList(lookup, "base_fields", "source_fields", "source_field")
	if len(baseFields) == 0 {
		return nil, nil, nil
	}
	lookupFields := relationMetadataFieldList(lookup, "lookup_fields", "mapping_source_fields", "reference_fields")
	if len(lookupFields) == 0 && ExistingField(lookup.Fields, "source_value") != "" {
		lookupFields = []string{ExistingField(lookup.Fields, "source_value")}
	}
	baseFields = relationExistingFields(base.Fields, baseFields, limit)
	lookupFields = relationExistingFields(lookup.Fields, lookupFields, limit)
	if len(baseFields) == 0 || len(lookupFields) == 0 {
		return nil, nil, nil
	}
	if len(baseFields) > len(lookupFields) {
		baseFields = baseFields[:len(lookupFields)]
	} else if len(lookupFields) > len(baseFields) {
		lookupFields = lookupFields[:len(baseFields)]
	}
	return baseFields, lookupFields, []string{"typed relation metadata"}
}

func relationMetadataFieldList(projection ArtifactSchemaProjection, keys ...string) []string {
	for _, key := range keys {
		if projection.Diagnostics == nil {
			continue
		}
		if fields := parseRelationFieldList(projection.Diagnostics[key]); len(fields) > 0 {
			return fields
		}
	}
	return nil
}

func parseRelationFieldList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var values []string
	if strings.HasPrefix(raw, "[") {
		if err := json.Unmarshal([]byte(raw), &values); err == nil {
			return cleanStrings(values)
		}
	}
	return cleanStrings(strings.Split(raw, ","))
}

func relationExistingFields(available, candidates []string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	var out []string
	for _, candidate := range cleanStrings(candidates) {
		field := ExistingField(available, candidate)
		if field == "" || !FieldUsableForRecordJoin(field) {
			continue
		}
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
