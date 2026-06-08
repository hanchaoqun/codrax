package dataworkflow

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

type FieldContractIssueInput struct {
	Records            []WorkflowRecord
	ArtifactAccess     []ArtifactAccessView
	AllowedNextActions []string
	Limit              int
}

type FieldContractIssue struct {
	Round                int      `json:"round,omitempty"`
	ActionIndex          int      `json:"action_index,omitempty"`
	ActionID             string   `json:"action_id,omitempty"`
	ArtifactID           string   `json:"artifact_id,omitempty"`
	ActionKind           string   `json:"action_kind,omitempty"`
	InputAlias           string   `json:"input_alias,omitempty"`
	InputAliases         []string `json:"input_aliases,omitempty"`
	OutputAlias          string   `json:"output_alias,omitempty"`
	IdempotencyKey       string   `json:"idempotency_key,omitempty"`
	DependencyRank       int      `json:"dependency_rank,omitempty"`
	MissingFields        []string `json:"missing_fields,omitempty"`
	AvailableFieldSample []string `json:"available_field_sample,omitempty"`
	CandidateArtifacts   []string `json:"candidate_artifacts,omitempty"`
	RepairActionHints    []string `json:"repair_action_hints,omitempty"`
	Reason               string   `json:"reason,omitempty"`
}

var planningMissingFieldRe = regexp.MustCompile(`data planning incomplete: action (\d+) \(([^)]*)\) references field\(s\) \[([^\]]*)\] that are not present on input ([^ ]+) fields \[([^\]]*)\]`)
var numericFieldContractRe = regexp.MustCompile(`numeric field contract failed: (filter_records|compute_contributions) input ([^ ]+) (?:field|value_field) "([^"]+)"`)

func DiscoverFieldContractIssues(input FieldContractIssueInput) []FieldContractIssue {
	limit := input.Limit
	if limit <= 0 || len(input.Records) == 0 || len(input.ArtifactAccess) == 0 {
		return nil
	}
	var out []FieldContractIssue
	for i := len(input.Records) - 1; i >= 0 && len(out) < limit; i-- {
		rec := input.Records[i]
		for _, issue := range fieldContractIssuesFromRecord(i+1, rec, input.ArtifactAccess, input.AllowedNextActions) {
			out = append(out, issue)
			if len(out) >= limit {
				break
			}
		}
		if rec.Result != nil {
			for j := len(rec.Result.Artifacts) - 1; j >= 0 && len(out) < limit; j-- {
				for _, issue := range extractFieldContractIssuesFromArtifact(i+1, rec.Result.Artifacts[j], input.ArtifactAccess) {
					out = append(out, issue)
					if len(out) >= limit {
						break
					}
				}
			}
		}
	}
	return out
}

func ArtifactAccessSchemaProjections(access []ArtifactAccessView) []ArtifactSchemaProjection {
	out := make([]ArtifactSchemaProjection, 0, len(access))
	for _, artifact := range access {
		out = append(out, ArtifactAccessViewToSchemaProjection(artifact))
	}
	return out
}

func ArtifactAccessViewToSchemaProjection(artifact ArtifactAccessView) ArtifactSchemaProjection {
	return ArtifactSchemaProjection{
		ID:                strings.TrimSpace(artifact.ID),
		Kind:              strings.TrimSpace(artifact.Kind),
		NodeClass:         strings.TrimSpace(artifact.NodeClass),
		Aliases:           cleanStrings(artifact.Aliases),
		JSONShape:         strings.TrimSpace(artifact.JSONShape),
		Fields:            cleanStrings(artifact.Fields),
		AccessHint:        strings.TrimSpace(artifact.AccessHint),
		SourcePaths:       cleanStrings(artifact.SourcePaths),
		SourceRecordPaths: cleanStrings(artifact.SourceRecordPaths),
		ReferencePaths:    cleanStrings(artifact.ReferencePaths),
		EvidencePaths:     cleanStrings(artifact.EvidencePaths),
	}
}

func fieldContractIssuesFromRecord(round int, rec WorkflowRecord, access []ArtifactAccessView, allowedActions []string) []FieldContractIssue {
	errText := strings.TrimSpace(rec.Err)
	if errText == "" {
		return nil
	}
	projections := ArtifactAccessSchemaProjections(access)
	var out []FieldContractIssue
	for _, match := range planningMissingFieldRe.FindAllStringSubmatch(errText, -1) {
		actionIndex := 0
		fmt.Sscanf(match[1], "%d", &actionIndex)
		actionID := strings.TrimSpace(match[2])
		actionKind := ""
		var action dataquery.DataAction
		hasAction := false
		if actionIndex > 0 && actionIndex <= len(rec.Plan.Actions) {
			action = rec.Plan.Actions[actionIndex-1]
			hasAction = true
			actionID = firstIssueText(strings.TrimSpace(action.ID), actionID)
			actionKind = string(NormalizeActionKind(action.Kind))
		}
		missing := parseIssueList(match[3])
		inputAlias := strings.TrimSpace(match[4])
		available := ClampRecordViewStringSlice(parseIssueList(match[5]), 24)
		if hasAction {
			violation := NewFieldContractViolation(FieldContractViolationInput{
				Action:             action,
				InputAlias:         inputAlias,
				MissingFields:      missing,
				AvailableFields:    available,
				SchemaProjections:  projections,
				AllowedNextActions: allowedActions,
			})
			issue := fieldContractIssueFromWorkflowViolation(round, actionIndex, violation)
			issue.ActionID = firstIssueText(issue.ActionID, actionID)
			issue.Reason = ClampRecordViewText(match[0], 700)
			out = append(out, issue)
			continue
		}
		out = append(out, FieldContractIssue{
			Round:                round,
			ActionIndex:          actionIndex,
			ActionID:             actionID,
			ActionKind:           actionKind,
			InputAlias:           inputAlias,
			MissingFields:        missing,
			AvailableFieldSample: available,
			CandidateArtifacts:   fieldContractCandidateLabels(access, inputAlias, missing),
			RepairActionHints:    FieldContractRepairHints(allowedActions),
			Reason:               ClampRecordViewText(match[0], 700),
		})
	}
	for _, match := range numericFieldContractRe.FindAllStringSubmatch(errText, -1) {
		inputAlias := strings.TrimSpace(match[2])
		field := strings.TrimSpace(match[3])
		var available []string
		if artifact, ok := artifactAccessByAlias(access, inputAlias); ok {
			available = ClampRecordViewStringSlice(artifact.Fields, 24)
		}
		out = append(out, FieldContractIssue{
			Round:                round,
			ActionKind:           strings.TrimSpace(match[1]),
			InputAlias:           inputAlias,
			MissingFields:        []string{field},
			AvailableFieldSample: available,
			CandidateArtifacts:   numericFieldCandidateArtifacts(access, inputAlias),
			RepairActionHints: []string{
				"use derive_fields operation=parse_number to materialize a numeric field from an existing numeric-looking source field",
				"do not repeat numeric filter_records or compute_contributions with a flag/status/text field as the numeric field",
				"use flag/status fields only for eq/in/contains/decision filters, then use a separate numeric value_field for contribution/reconcile",
			},
			Reason: ClampRecordViewText(match[0], 700),
		})
	}
	return out
}

func fieldContractIssueFromWorkflowViolation(round, actionIndex int, violation WorkflowViolation) FieldContractIssue {
	return FieldContractIssue{
		Round:                round,
		ActionIndex:          actionIndex,
		ActionID:             strings.TrimSpace(violation.ActionID),
		ActionKind:           strings.TrimSpace(violation.ActionKind),
		InputAlias:           strings.TrimSpace(violation.InputAlias),
		InputAliases:         cleanStrings(violation.InputAliases),
		OutputAlias:          strings.TrimSpace(violation.OutputAlias),
		IdempotencyKey:       strings.TrimSpace(violation.IdempotencyKey),
		DependencyRank:       violation.DependencyRank,
		MissingFields:        append([]string(nil), violation.MissingFields...),
		AvailableFieldSample: append([]string(nil), violation.AvailableFieldSample...),
		CandidateArtifacts:   append([]string(nil), violation.CandidateArtifacts...),
		RepairActionHints:    append([]string(nil), violation.RepairActionHints...),
		Reason:               strings.TrimSpace(violation.Reason),
	}
}

func extractFieldContractIssuesFromArtifact(round int, artifact dataquery.DataArtifact, access []ArtifactAccessView) []FieldContractIssue {
	var out []FieldContractIssue
	var walk func(dataquery.DataArtifact)
	walk = func(current dataquery.DataArtifact) {
		if artifactKindHasPrefix(current.Kind, dataquery.DataActionExtractFields) {
			inputRows := artifactIntField(current, "input_rows")
			outputRows := artifactIntField(current, "output_rows")
			required := cleanStrings(strings.Split(artifactField(current, "required_fields"), ","))
			extracted := cleanStrings(strings.Split(artifactField(current, "extracted_fields"), ","))
			if inputRows > 0 && outputRows == 0 && (len(required) > 0 || len(extracted) > 0) {
				missing := required
				if len(missing) == 0 {
					missing = extracted
				}
				inputAlias := firstIssueText(strings.TrimSpace(artifactField(current, "input_path")), strings.TrimSpace(artifactField(current, "input_paths")))
				available := cleanStrings(strings.Split(firstIssueText(artifactField(current, "source_fields"), strings.Join(current.Headers, ",")), ","))
				candidateFields := cleanStrings(strings.Split(firstIssueText(artifactField(current, "source_fields"), strings.Join(current.Headers, ",")), ","))
				candidates := fieldContractCandidateLabels(access, inputAlias, candidateFields)
				if len(candidates) == 0 {
					candidates = fieldContractCandidateLabels(access, inputAlias, missing)
				}
				if len(candidates) == 0 {
					candidates = ClampRecordViewStringSlice(ArtifactAliases(current), 8)
				}
				reasonParts := []string{"extract_fields produced zero matched records from a non-empty input"}
				if diag := strings.TrimSpace(artifactField(current, "zero_match_diagnostics")); diag != "" {
					reasonParts = append(reasonParts, "diagnostics="+diag)
				}
				if samples := strings.TrimSpace(artifactField(current, "source_field_samples")); samples != "" {
					reasonParts = append(reasonParts, "source_field_samples="+samples)
				}
				out = append(out, FieldContractIssue{
					Round:                round,
					ArtifactID:           strings.TrimSpace(current.ID),
					ActionKind:           string(dataquery.DataActionExtractFields),
					InputAlias:           inputAlias,
					MissingFields:        ClampRecordViewStringSlice(missing, 16),
					AvailableFieldSample: ClampRecordViewStringSlice(available, 24),
					CandidateArtifacts:   ClampRecordViewStringSlice(candidates, 8),
					RepairActionHints: []string{
						"inspect source_field_samples and current artifact fields before retrying extraction",
						"repair extract_fields by adjusting source_field/source_fields, regex/pattern, document scope, grouping, or required_fields",
						"do not compute contributions or assemble a final answer from this zero-match extraction while contribution/reconcile/projection remains required",
					},
					Reason: ClampRecordViewText(strings.Join(reasonParts, "; "), 1200),
				})
			}
		}
		for _, child := range current.Children {
			walk(child)
		}
	}
	walk(artifact)
	return out
}

func fieldContractCandidateLabels(access []ArtifactAccessView, input string, missing []string) []string {
	candidates := FieldContractCandidateArtifacts(ArtifactAccessSchemaProjections(access), input, missing, 4)
	return FieldContractCandidateLabels(candidates)
}

func numericFieldCandidateArtifacts(access []ArtifactAccessView, input string) []string {
	inputKey := normalizeAccessPath(input)
	type candidate struct {
		label string
		score int
	}
	var candidates []candidate
	for _, artifact := range access {
		alias := firstArtifactAccessAlias(artifact)
		if alias == "" {
			alias = artifact.ID
		}
		if strings.TrimSpace(alias) == "" {
			continue
		}
		fields := numericLookingSampleFields(artifact.FieldSamples, 6)
		if len(fields) == 0 {
			continue
		}
		score := len(fields)
		if normalizeAccessPath(alias) == inputKey || normalizeAccessPath(artifact.ID) == inputKey {
			score += 10
		}
		candidates = append(candidates, candidate{
			label: fmt.Sprintf("%s numeric-like fields [%s]", alias, strings.Join(fields, ", ")),
			score: score,
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].label < candidates[j].label
	})
	var out []string
	for _, candidate := range candidates {
		out = append(out, candidate.label)
		if len(out) >= 4 {
			break
		}
	}
	return out
}

func numericLookingSampleFields(samples map[string][]string, limit int) []string {
	if limit <= 0 {
		limit = 1
	}
	var out []string
	for field, values := range samples {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		for _, value := range values {
			if stringLooksNumeric(value) {
				out = append(out, field)
				break
			}
		}
	}
	sort.Strings(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func artifactAccessByAlias(access []ArtifactAccessView, alias string) (ArtifactAccessView, bool) {
	want := normalizeAccessPath(alias)
	if want == "" {
		return ArtifactAccessView{}, false
	}
	for _, artifact := range access {
		if normalizeAccessPath(artifact.ID) == want {
			return artifact, true
		}
		for _, candidate := range artifact.Aliases {
			if normalizeAccessPath(candidate) == want {
				return artifact, true
			}
		}
	}
	return ArtifactAccessView{}, false
}

func firstArtifactAccessAlias(artifact ArtifactAccessView) string {
	for _, alias := range artifact.Aliases {
		if alias = strings.TrimSpace(alias); alias != "" {
			return alias
		}
	}
	return strings.TrimSpace(artifact.ID)
}

func parseIssueList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	raw = strings.TrimPrefix(strings.TrimSuffix(raw, "]"), "[")
	parts := strings.Split(raw, ",")
	return cleanStrings(parts)
}

func stringLooksNumeric(value string) bool {
	value = strings.TrimSpace(strings.ReplaceAll(value, ",", ""))
	if value == "" {
		return false
	}
	_, err := strconv.ParseFloat(value, 64)
	return err == nil
}

func firstIssueText(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
