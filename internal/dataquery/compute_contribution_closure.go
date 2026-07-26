package dataquery

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

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
