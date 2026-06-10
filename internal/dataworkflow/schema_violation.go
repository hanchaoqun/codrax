package dataworkflow

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

type ArtifactFieldCandidate struct {
	Alias          string   `json:"alias,omitempty"`
	MatchingFields []string `json:"matching_fields,omitempty"`
	MatchesAll     bool     `json:"matches_all,omitempty"`
}

func FieldContractCandidateArtifacts(projections []ArtifactSchemaProjection, input string, missing []string, limit int) []ArtifactFieldCandidate {
	missing = cleanStrings(missing)
	if len(missing) == 0 {
		return nil
	}
	inputKey := normalizeAccessPath(input)
	type candidate struct {
		value     ArtifactFieldCandidate
		matchSize int
	}
	var candidates []candidate
	for _, projection := range projections {
		alias := firstArtifactSchemaAlias(projection)
		if alias == "" || normalizeAccessPath(alias) == inputKey || normalizeAccessPath(projection.ID) == inputKey {
			continue
		}
		fields := artifactFieldSet(projection.Fields)
		var matches []string
		for _, field := range missing {
			if fields[strings.ToLower(strings.TrimSpace(field))] != "" {
				matches = append(matches, field)
			}
		}
		matches = cleanStrings(matches)
		if len(matches) == 0 {
			continue
		}
		candidates = append(candidates, candidate{
			value: ArtifactFieldCandidate{
				Alias:          alias,
				MatchingFields: matches,
				MatchesAll:     len(matches) == len(missing),
			},
			matchSize: len(matches),
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].value.MatchesAll != candidates[j].value.MatchesAll {
			return candidates[i].value.MatchesAll
		}
		if candidates[i].matchSize != candidates[j].matchSize {
			return candidates[i].matchSize > candidates[j].matchSize
		}
		return candidates[i].value.Alias < candidates[j].value.Alias
	})
	if limit <= 0 {
		limit = len(candidates)
	}
	out := make([]ArtifactFieldCandidate, 0, min(limit, len(candidates)))
	for _, candidate := range candidates {
		out = append(out, candidate.value)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func FieldContractCandidateLabels(candidates []ArtifactFieldCandidate) []string {
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		alias := strings.TrimSpace(candidate.Alias)
		if alias == "" || len(candidate.MatchingFields) == 0 {
			continue
		}
		label := alias
		if !candidate.MatchesAll {
			label += " partial"
		}
		out = append(out, fmt.Sprintf("%s has [%s]", label, strings.Join(cleanStrings(candidate.MatchingFields), ", ")))
	}
	return out
}

func RecordInputCandidateArtifacts(projections []ArtifactSchemaProjection, rejected ArtifactSchemaProjection, input string, required []string, limit int) []ArtifactFieldCandidate {
	inputKey := normalizeAccessPath(input)
	required = cleanStrings(required)
	type candidate struct {
		value        ArtifactFieldCandidate
		matchSize    int
		lineageScore int
		rowCount     int
	}
	var candidates []candidate
	for _, projection := range projections {
		if !ArtifactUsableForRecordAction(projection) {
			continue
		}
		alias := firstArtifactSchemaAlias(projection)
		if alias == "" || normalizeAccessPath(alias) == inputKey || normalizeAccessPath(projection.ID) == inputKey {
			continue
		}
		fields := artifactFieldSet(projection.Fields)
		var matches []string
		for _, field := range required {
			if fields[strings.ToLower(strings.TrimSpace(field))] != "" {
				matches = append(matches, field)
			}
		}
		matches = cleanStrings(matches)
		lineageScore := artifactProjectionLineageOverlap(rejected, projection)
		if len(required) > 0 && len(matches) == 0 {
			continue
		}
		if len(required) == 0 && lineageScore == 0 {
			continue
		}
		candidates = append(candidates, candidate{
			value: ArtifactFieldCandidate{
				Alias:          alias,
				MatchingFields: matches,
				MatchesAll:     len(required) == 0 || len(matches) == len(required),
			},
			matchSize:    len(matches),
			lineageScore: lineageScore,
			rowCount:     projection.RowCount,
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].value.MatchesAll != candidates[j].value.MatchesAll {
			return candidates[i].value.MatchesAll
		}
		if candidates[i].lineageScore != candidates[j].lineageScore {
			return candidates[i].lineageScore > candidates[j].lineageScore
		}
		if candidates[i].matchSize != candidates[j].matchSize {
			return candidates[i].matchSize > candidates[j].matchSize
		}
		if candidates[i].rowCount != candidates[j].rowCount {
			return candidates[i].rowCount > candidates[j].rowCount
		}
		return candidates[i].value.Alias < candidates[j].value.Alias
	})
	if limit <= 0 {
		limit = len(candidates)
	}
	out := make([]ArtifactFieldCandidate, 0, min(limit, len(candidates)))
	for _, candidate := range candidates {
		out = append(out, candidate.value)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func artifactProjectionLineageOverlap(left, right ArtifactSchemaProjection) int {
	leftKeys := artifactProjectionLineageKeySet(left)
	if len(leftKeys) == 0 {
		return 0
	}
	var score int
	for key := range artifactProjectionLineageKeySet(right) {
		if leftKeys[key] {
			score++
		}
	}
	return score
}

func artifactProjectionLineageKeySet(projection ArtifactSchemaProjection) map[string]bool {
	out := map[string]bool{}
	add := func(value string) {
		for _, key := range artifactLineageKeys(value) {
			if key != "" {
				out[key] = true
			}
		}
	}
	add(projection.ID)
	for _, alias := range projection.Aliases {
		add(alias)
	}
	for _, value := range projection.SourcePaths {
		add(value)
	}
	for _, value := range projection.SourceRecordPaths {
		add(value)
	}
	for _, value := range projection.ReferencePaths {
		add(value)
	}
	return out
}

func FieldContractRepairHints(allowedActions []string) []string {
	allowed := map[string]bool{}
	for _, action := range allowedActions {
		action = strings.TrimSpace(action)
		if action != "" {
			allowed[action] = true
		}
	}
	var hints []string
	if allowed[string(dataquery.DataActionExtractFields)] {
		hints = append(hints, "use extract_fields when the missing fields are embedded in an existing text/record field")
	}
	if allowed[string(dataquery.DataActionGroupRecords)] {
		hints = append(hints, "use group_records before extract_fields when one logical record is split across multiple rows/spans sharing a key")
	}
	if allowed[string(dataquery.DataActionDeriveFields)] {
		hints = append(hints, "use derive_fields when the missing fields can be computed from fields already present on the same records")
	}
	if allowed[string(dataquery.DataActionEnrichRecords)] || allowed[string(dataquery.DataActionJoinRecords)] {
		hints = append(hints, "use enrich_records or a two-input join_records when the missing fields exist on another artifact")
	}
	if allowed[string(dataquery.DataActionFilterRecords)] || allowed[string(dataquery.DataActionComputeContribs)] {
		hints = append(hints, "do not repeat filter_records or compute_contributions until every named field exists on the selected input artifact")
	}
	return hints
}

func firstArtifactSchemaAlias(projection ArtifactSchemaProjection) string {
	for _, alias := range projection.Aliases {
		if alias = strings.TrimSpace(alias); alias != "" {
			return alias
		}
	}
	return strings.TrimSpace(projection.ID)
}
