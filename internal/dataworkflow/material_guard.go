package dataworkflow

import (
	"fmt"
	"strings"
)

type RequiredMaterialSchedulingGuardInput struct {
	ContinueAfter  bool
	RequiredPaths  []string
	ScheduledPaths []string
}

func RequiredMaterialSchedulingGuardResult(input RequiredMaterialSchedulingGuardInput) GuardResult {
	if input.ContinueAfter {
		return GuardResult{}
	}
	required := cleanStrings(input.RequiredPaths)
	if len(required) == 0 {
		return GuardResult{}
	}
	scheduled := map[string]bool{}
	for _, p := range cleanStrings(input.ScheduledPaths) {
		scheduled[p] = true
	}
	var missing []string
	for _, p := range required {
		if !scheduled[p] {
			missing = append(missing, p)
		}
	}
	if len(missing) == 0 {
		return GuardResult{}
	}
	message := fmt.Sprintf("data planning incomplete: terminal batch declares %d required material(s) that are not scheduled for script/typed-action consumption: %s. Add focused actions that read these materials, change their usage_mode to planner_distilled/text_evidence_consumed when appropriate, or keep continue_after=true until the required materials are covered.",
		len(missing), strings.Join(missing, ", "))
	violation := NewGenericViolation(GenericViolationInput{
		Code:              "required_material_scheduling",
		Severity:          "error",
		Repairability:     RepairNeedsTypedAction,
		InputAliases:      missing,
		RepairActionHints: []string{"inspect_material", "extract_records", "derive_rules", "derive_fields", "custom_transform"},
		Reason:            message,
	})
	return NewGuardResult("required_material_scheduling", "error", RepairNeedsTypedAction, message, violation)
}
