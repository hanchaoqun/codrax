package dataworkflow

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

type ArtifactDiagnosticBatch struct {
	Round     int
	Artifacts []ArtifactProjectionSource
}

type ZeroMatchFilterIssue struct {
	Round             int      `json:"round,omitempty"`
	ArtifactID        string   `json:"artifact_id,omitempty"`
	Aliases           []string `json:"aliases,omitempty"`
	InputPath         string   `json:"input_path,omitempty"`
	InputRows         int      `json:"input_rows,omitempty"`
	OutputRows        int      `json:"output_rows,omitempty"`
	FilterFields      []string `json:"filter_fields,omitempty"`
	FilterDiagnostics string   `json:"filter_diagnostics,omitempty"`
	RepairActionHints []string `json:"repair_action_hints,omitempty"`
	Reason            string   `json:"reason,omitempty"`
}

type UnmatchedResolutionIssue struct {
	Round             int      `json:"round,omitempty"`
	ArtifactID        string   `json:"artifact_id,omitempty"`
	Aliases           []string `json:"aliases,omitempty"`
	BasePath          string   `json:"base_path,omitempty"`
	BaseRows          int      `json:"base_rows,omitempty"`
	OutputRows        int      `json:"output_rows,omitempty"`
	TargetFields      []string `json:"target_fields,omitempty"`
	MatchedCounts     []string `json:"matched_counts,omitempty"`
	UnmatchedCounts   []string `json:"unmatched_counts,omitempty"`
	RepairActionHints []string `json:"repair_action_hints,omitempty"`
	Reason            string   `json:"reason,omitempty"`
}

type ZeroEligibleIssue struct {
	Round                    int      `json:"round,omitempty"`
	ArtifactID               string   `json:"artifact_id,omitempty"`
	Aliases                  []string `json:"aliases,omitempty"`
	InputPath                string   `json:"input_path,omitempty"`
	InputRows                int      `json:"input_rows,omitempty"`
	EligibleRows             int      `json:"eligible_rows,omitempty"`
	IncludeFilterDiagnostics string   `json:"include_filter_diagnostics,omitempty"`
	QualificationReasons     string   `json:"qualification_reasons,omitempty"`
	RepairActionHints        []string `json:"repair_action_hints,omitempty"`
	Reason                   string   `json:"reason,omitempty"`
}

func ZeroMatchFilterIssuesFromBatches(batches []ArtifactDiagnosticBatch, limit int) []ZeroMatchFilterIssue {
	if limit <= 0 || len(batches) == 0 {
		return nil
	}
	var out []ZeroMatchFilterIssue
	clearedAliases := map[string]bool{}
	for _, batch := range batches {
		for i := len(batch.Artifacts) - 1; i >= 0 && len(out) < limit; i-- {
			markPositiveFilterArtifactAliases(batch.Artifacts[i], clearedAliases)
			for _, issue := range ZeroMatchFilterIssuesFromArtifact(batch.Round, batch.Artifacts[i]) {
				if zeroMatchFilterIssueCleared(issue, clearedAliases) {
					continue
				}
				out = append(out, issue)
				if len(out) >= limit {
					break
				}
			}
		}
	}
	return out
}

func ZeroMatchFilterIssuesFromArtifact(round int, artifact ArtifactProjectionSource) []ZeroMatchFilterIssue {
	var out []ZeroMatchFilterIssue
	var walk func(ArtifactProjectionSource)
	walk = func(current ArtifactProjectionSource) {
		if artifactKindHasPrefix(current.Kind, dataquery.DataActionFilterRecords) {
			inputRows := artifactIntField(current, "input_rows")
			outputRows := artifactIntField(current, "output_rows", "filter_matched")
			if inputRows > 0 && outputRows == 0 {
				out = append(out, ZeroMatchFilterIssue{
					Round:             round,
					ArtifactID:        strings.TrimSpace(current.ID),
					Aliases:           clampStrings(ArtifactAliases(current), 8),
					InputPath:         artifactField(current, "input_path"),
					InputRows:         inputRows,
					OutputRows:        outputRows,
					FilterFields:      cleanStrings(strings.Split(artifactField(current, "filter_fields"), ",")),
					FilterDiagnostics: clampArtifactDiagnostic(artifactField(current, "filter_diagnostics"), 1200),
					RepairActionHints: []string{
						"inspect the original input artifact or current filter artifact to compare actual field values against every filter",
						"rerun filter_records against the non-empty input artifact with corrected filters before join_records or compute_contributions",
						"do not join or compute contributions from this zero-row artifact while contribution/reconcile is still required",
					},
					Reason: "filter_records produced zero rows from a non-empty input while contribution/reconcile remains required",
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

func UnmatchedResolutionIssuesFromBatches(batches []ArtifactDiagnosticBatch, limit int) []UnmatchedResolutionIssue {
	if limit <= 0 || len(batches) == 0 {
		return nil
	}
	var out []UnmatchedResolutionIssue
	for _, batch := range batches {
		for i := len(batch.Artifacts) - 1; i >= 0 && len(out) < limit; i-- {
			for _, issue := range UnmatchedResolutionIssuesFromArtifact(batch.Round, batch.Artifacts[i]) {
				out = append(out, issue)
				if len(out) >= limit {
					break
				}
			}
		}
	}
	return out
}

func UnmatchedResolutionIssuesFromArtifact(round int, artifact ArtifactProjectionSource) []UnmatchedResolutionIssue {
	var out []UnmatchedResolutionIssue
	var walk func(ArtifactProjectionSource)
	walk = func(current ArtifactProjectionSource) {
		if artifactKindHasPrefix(current.Kind, dataquery.DataActionApplyResolutions) {
			baseRows := artifactIntField(current, "base_rows", "input_rows")
			outputRows := artifactIntField(current, "output_rows", "row_count")
			if outputRows == 0 {
				outputRows = current.RowCount
			}
			targets := cleanStrings(strings.Split(artifactField(current, "target_fields"), ","))
			if len(targets) > 0 && baseRows > 0 {
				var unmatchedTargets []string
				var matchedCounts []string
				var unmatchedCounts []string
				for _, target := range targets {
					matched := artifactIntField(current, "matched_"+target)
					unmatched := artifactIntField(current, "unmatched_"+target)
					if matched == 0 && unmatched > 0 {
						unmatchedTargets = append(unmatchedTargets, target)
						matchedCounts = append(matchedCounts, target+"=0")
						unmatchedCounts = append(unmatchedCounts, fmt.Sprintf("%s=%d", target, unmatched))
					}
				}
				if len(unmatchedTargets) == len(targets) {
					out = append(out, UnmatchedResolutionIssue{
						Round:           round,
						ArtifactID:      strings.TrimSpace(current.ID),
						Aliases:         clampStrings(ArtifactAliases(current), 8),
						BasePath:        artifactField(current, "base_path"),
						BaseRows:        baseRows,
						OutputRows:      outputRows,
						TargetFields:    unmatchedTargets,
						MatchedCounts:   matchedCounts,
						UnmatchedCounts: unmatchedCounts,
						RepairActionHints: []string{
							"repair apply_entity_resolutions before filtering, qualification, join, or contribution calculation",
							"use locator-compatible key fields: when one side uses item_id/source_locator and the other side uses _source_locator/_source_index, match them through the row locator/index contract",
							"do not consume this all-unmatched resolution artifact for downstream contribution work",
						},
						Reason: "apply_entity_resolutions produced no matched canonical target values for a non-empty base record set",
					})
				}
			}
		}
		for _, child := range current.Children {
			walk(child)
		}
	}
	walk(artifact)
	return out
}

func ZeroEligibleIssuesFromBatches(batches []ArtifactDiagnosticBatch, limit int) []ZeroEligibleIssue {
	if limit <= 0 || len(batches) == 0 {
		return nil
	}
	var out []ZeroEligibleIssue
	for _, batch := range batches {
		for i := len(batch.Artifacts) - 1; i >= 0 && len(out) < limit; i-- {
			for _, issue := range ZeroEligibleIssuesFromArtifact(batch.Round, batch.Artifacts[i]) {
				out = append(out, issue)
				if len(out) >= limit {
					break
				}
			}
		}
	}
	return out
}

func ZeroEligibleIssuesFromArtifact(round int, artifact ArtifactProjectionSource) []ZeroEligibleIssue {
	var out []ZeroEligibleIssue
	var walk func(ArtifactProjectionSource)
	walk = func(current ArtifactProjectionSource) {
		if artifactKindHasPrefix(current.Kind, dataquery.DataActionQualifyRecords) {
			inputRows := artifactIntField(current, "input_rows")
			eligibleRows := artifactIntField(current, "eligible_rows", "output_rows")
			if inputRows > 0 && eligibleRows == 0 {
				out = append(out, ZeroEligibleIssue{
					Round:                    round,
					ArtifactID:               strings.TrimSpace(current.ID),
					Aliases:                  clampStrings(ArtifactAliases(current), 8),
					InputPath:                artifactField(current, "input_path"),
					InputRows:                inputRows,
					EligibleRows:             eligibleRows,
					IncludeFilterDiagnostics: clampArtifactDiagnostic(artifactField(current, "include_filters"), 1200),
					QualificationReasons:     clampArtifactDiagnostic(artifactField(current, "qualification_reasons"), 800),
					RepairActionHints: []string{
						"repair the upstream field materialization, resolution application, or qualification filters before joining or computing contributions",
						"inspect actual field values when every row failed an include/status/evidence condition",
						"do not join or compute contributions from this zero-eligible artifact while contribution/reconcile remains required",
					},
					Reason: "qualify_records produced zero eligible rows from a non-empty input while contribution/reconcile remains required",
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

func markPositiveFilterArtifactAliases(artifact ArtifactProjectionSource, cleared map[string]bool) {
	if cleared == nil {
		return
	}
	var walk func(ArtifactProjectionSource)
	walk = func(current ArtifactProjectionSource) {
		if artifactKindHasPrefix(current.Kind, dataquery.DataActionFilterRecords) {
			outputRows := artifactIntField(current, "output_rows", "filter_matched")
			if outputRows == 0 {
				outputRows = current.RowCount
			}
			if outputRows > 0 {
				for _, alias := range ArtifactAliases(current) {
					if key := normalizeAccessPath(alias); key != "" {
						cleared[key] = true
					}
				}
				if key := normalizeAccessPath(current.ID); key != "" {
					cleared[key] = true
				}
			}
		}
		for _, child := range current.Children {
			walk(child)
		}
	}
	walk(artifact)
}

func zeroMatchFilterIssueCleared(issue ZeroMatchFilterIssue, cleared map[string]bool) bool {
	if len(cleared) == 0 {
		return false
	}
	for _, alias := range issue.Aliases {
		if cleared[normalizeAccessPath(alias)] {
			return true
		}
	}
	if issue.ArtifactID != "" && cleared[normalizeAccessPath(issue.ArtifactID)] {
		return true
	}
	return false
}

func artifactIntField(artifact ArtifactProjectionSource, keys ...string) int {
	for _, key := range keys {
		raw := artifactField(artifact, key)
		if raw == "" {
			continue
		}
		value, err := strconv.Atoi(raw)
		if err == nil {
			return value
		}
	}
	return 0
}
