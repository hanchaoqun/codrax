package dataworkflow

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
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

type BroadCustomPrerequisiteGuardInput struct {
	Action      dataquery.DataAction
	ActionIndex int
	IsBroad     bool
	Missing     []string
}

func BroadCustomPrerequisiteGuardResult(input BroadCustomPrerequisiteGuardInput) GuardResult {
	action := input.Action
	kind := NormalizeActionKind(action.Kind)
	missing := cleanStrings(input.Missing)
	if kind != dataquery.DataActionCustomTransform ||
		strings.TrimSpace(action.Script) == "" ||
		!input.IsBroad ||
		len(missing) == 0 {
		return GuardResult{}
	}
	actionNumber := input.ActionIndex + 1
	if actionNumber <= 0 {
		actionNumber = 1
	}
	label := firstNonEmptyGuardText(strings.TrimSpace(action.ID), strings.TrimSpace(string(kind)))
	message := fmt.Sprintf("data planning incomplete: broad custom_transform action %d (%s) reads or depends on %d material(s) that were not covered by prior typed actions/results: %s. First add smaller atomic actions such as inspect_material, derive_rules, derive_fields, normalize_entities, enrich_records, join_records, compute_contributions, or extract_records for the missing inputs, then use custom_transform only as a bounded transform over known materials.",
		actionNumber, label, len(missing), strings.Join(missing, ", "))
	violation := NewGenericViolation(GenericViolationInput{
		Code:              "broad_custom_prerequisite_missing",
		Severity:          "error",
		Repairability:     RepairNeedsTypedAction,
		Action:            action,
		InputAliases:      missing,
		RepairActionHints: []string{"inspect_material", "extract_records", "derive_rules", "derive_fields", "normalize_entities", "enrich_records", "join_records", "compute_contributions"},
		Reason:            message,
	})
	return NewGuardResult("broad_custom_prerequisite_missing", "error", RepairNeedsTypedAction, message, violation)
}
