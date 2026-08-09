package dataquery

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

var computeContributionAllowedParams = map[string]bool{
	"allow_unqualified_records": true,
	"expected_count":            true,
	"expected_values_json":      true,
	"filter_equals":             true,
	"filter_field":              true,
	"filter_not_equals":         true,
	"filter_op":                 true,
	"filter_value":              true,
	"filters":                   true,
	"filters_json":              true,
	"generated_status_fields":   true,
	"group_by":                  true,
	"group_by_fields":           true,
	"group_field":               true,
	"group_fields":              true,
	"group_key":                 true,
	"group_key_field":           true,
	"group_key_fields":          true,
	"group_key_literal":         true,
	"item_id_field":             true,
	"key_field":                 true,
	"max_contributions":         true,
	"max_records":               true,
	"metric":                    true,
	"operation":                 true,
	"reason":                    true,
	"replace_contributions":     true,
	"role":                      true,
	"rule_refs":                 true,
	"scope":                     true,
	"status_fields":             true,
	"value_field":               true,
}

// validateComputeContributionActionParams prevents a typed action from
// silently accepting configuration that the executor never consumes.  A
// phantom key is a precise structural error: without this check a planner can
// believe it requested member collection (for example via an invented key)
// while the runner executes an unrelated count.  The allowlist covers the
// action family rather than any business/case vocabulary.
func validateComputeContributionActionParams(action DataAction) error {
	var unknown []string
	for key := range action.Params {
		key = strings.TrimSpace(key)
		if key != "" && !computeContributionAllowedParams[key] {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		allowed := make([]string, 0, len(computeContributionAllowedParams))
		for key := range computeContributionAllowedParams {
			allowed = append(allowed, key)
		}
		sort.Strings(allowed)
		return DataActionParamError{
			ActionKind:    DataActionComputeContribs,
			Param:         strings.Join(unknown, ","),
			ExpectedShape: "only supported compute_contributions parameters",
			ActualSnippet: strings.Join(unknown, ","),
			Message: fmt.Sprintf(
				"compute_contributions has unsupported parameter(s) [%s]; allowed parameters are [%s]. For member/id/label lists use operation=include, set, or rank with value_field=<existing field>; operation=count counts rows and does not emit member values",
				strings.Join(unknown, ", "), strings.Join(allowed, ", ")),
		}
	}

	// count never consumes value_field: each selected row contributes the
	// literal value 1. Accepting both declarations lets a structurally valid
	// plan say "collect these member values" while the executor silently
	// counts rows instead. Reject that typed self-contradiction at the common
	// admission boundary; no goal/rule/answer prose participates.
	operation, _ := normalizeContributionOperation(action.Params["operation"])
	if operation == "count" && strings.TrimSpace(action.Params["value_field"]) != "" {
		return DataActionParamError{
			ActionKind:    DataActionComputeContribs,
			Param:         "value_field",
			ExpectedShape: "empty when operation=count, or operation=include|set|rank when value_field members must be preserved",
			ActualSnippet: strings.TrimSpace(action.Params["value_field"]),
			Message:       "compute_contributions operation=count ignores value_field and only emits row-count value 1; remove value_field for a count, or use operation=include, set, or rank to preserve the declared member values",
		}
	}
	return nil
}

// validateComputeContributionExpectedClosure is the deterministic completion
// plan's runtime value gate. Artifact schema and RowCount can admit a
// candidate plan, but only the materialized values may close the existing
// answer; equal-sized yet different member sets fail closed here.
func validateComputeContributionExpectedClosure(action DataAction, contributions []ContributionRecord) error {
	expectedCountText := strings.TrimSpace(action.Params["expected_count"])
	if expectedCountText != "" {
		expectedCount, err := strconv.Atoi(expectedCountText)
		if err != nil || expectedCount < 0 {
			return dataActionParamError(DataActionComputeContribs, "expected_count", "non-negative integer", expectedCountText, nil)
		}
		if len(contributions) != expectedCount {
			return DataActionDependencyError{
				ActionKind:    DataActionComputeContribs,
				Role:          "answer_closure",
				Operation:     "compute_contributions",
				InputAliases:  normalizeMaterialPaths(action.InputPaths),
				ExpectedShape: fmt.Sprintf("exactly %d sourced target contribution(s) matching the existing answer", expectedCount),
				ActualSnippet: fmt.Sprintf("computed_contributions=%d", len(contributions)),
				RepairAction:  DataActionQualifyRecords,
				Message:       fmt.Sprintf("compute_contributions answer closure failed: existing answer requires %d selected record(s), but the action materialized %d contribution(s); re-qualify the record set or compute contributions from the real answer fields", expectedCount, len(contributions)),
			}
		}
	}
	expectedValuesRaw := strings.TrimSpace(action.Params["expected_values_json"])
	if expectedValuesRaw == "" {
		return nil
	}
	var expectedValues []string
	if err := json.Unmarshal([]byte(expectedValuesRaw), &expectedValues); err != nil {
		return dataActionParamError(DataActionComputeContribs, "expected_values_json", "JSON string array", expectedValuesRaw, nil)
	}
	actualValues := make([]string, 0, len(contributions))
	for _, contribution := range contributions {
		value := strings.TrimSpace(contribution.Value.String())
		if value == "" {
			value = strings.TrimSpace(contribution.ItemID.String())
		}
		actualValues = append(actualValues, value)
	}
	for i := range expectedValues {
		expectedValues[i] = strings.TrimSpace(expectedValues[i])
	}
	sort.Strings(expectedValues)
	sort.Strings(actualValues)
	if stringSlicesEqual(expectedValues, actualValues) {
		return nil
	}
	return DataActionDependencyError{
		ActionKind:    DataActionComputeContribs,
		Role:          "answer_closure",
		Operation:     "compute_contributions",
		InputAliases:  normalizeMaterialPaths(action.InputPaths),
		ExpectedShape: "target contribution member values exactly matching the existing answer member set",
		ActualSnippet: fmt.Sprintf("expected=%s actual=%s", strings.Join(expectedValues, ","), strings.Join(actualValues, ",")),
		RepairAction:  DataActionQualifyRecords,
		Message:       fmt.Sprintf("compute_contributions answer closure failed: selected record values [%s] do not match existing answer members [%s]; re-qualify the selected artifact or compute the real contribution field", strings.Join(actualValues, ","), strings.Join(expectedValues, ",")),
	}
}

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func computeContributionGroupKeyFields(action DataAction) []string {
	if field := strings.TrimSpace(action.Params["group_key_field"]); field != "" {
		return []string{field}
	}
	return cleanStringList(parseActionStringListParam(firstNonEmptyString(
		action.Params["group_key_fields"],
		action.Params["group_by_fields"],
		action.Params["group_fields"],
		action.Params["group_by"],
		action.Params["group_field"],
		action.Params["key_field"],
	)))
}
