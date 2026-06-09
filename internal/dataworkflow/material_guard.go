package dataworkflow

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

type TextConstraintCoverageGuardInput struct {
	ContinueAfter               bool
	RuleCoverageRequired        bool
	ValidationLedgerCount       int
	CustomInputPaths            []string
	ScriptConsumedMaterialPaths []string
}

type RuleCoveragePrerequisiteGuardInput struct {
	RuleCoverageRequired bool
	RuleCoverageRecords  int
	PostRuleProgress     bool
	Actions              []dataquery.DataAction
	RuleMaterialPaths    []string
}

type RequiredMaterialSchedulingGuardInput struct {
	ContinueAfter  bool
	RequiredPaths  []string
	ScheduledPaths []string
}

func TextConstraintCoverageGuardResult(input TextConstraintCoverageGuardInput) GuardResult {
	if input.ContinueAfter || input.RuleCoverageRequired || input.ValidationLedgerCount < 2 {
		return GuardResult{}
	}
	inputs := map[string]bool{}
	for _, p := range cleanStrings(input.CustomInputPaths) {
		inputs[normalizeCoveragePath(p)] = true
	}
	if len(inputs) == 0 {
		return GuardResult{}
	}
	var materials []string
	for _, material := range cleanStrings(input.ScriptConsumedMaterialPaths) {
		p := normalizeCoveragePath(material)
		if p == "" || !inputs[p] || !PathLooksLikeTextConstraintMaterial(p) {
			continue
		}
		materials = append(materials, p)
	}
	if len(materials) == 0 {
		return GuardResult{}
	}
	sort.Strings(materials)
	message := fmt.Sprintf("data planning incomplete: terminal data calculation consumes text/rule material(s) %s with decision/contribution/reconcile ledgers but coverage_contract.rule_coverage_required=false. Add a derive_rules action or emit result.rule_coverage records linked from decisions/contributions/entity_resolutions before completing.",
		strings.Join(materials, ", "))
	violation := NewGenericViolation(GenericViolationInput{
		Code:              "text_constraint_coverage_required",
		Severity:          "error",
		Repairability:     RepairNeedsTypedAction,
		InputAliases:      materials,
		RepairActionHints: []string{string(dataquery.DataActionDeriveRules)},
		Reason:            message,
	})
	return NewGuardResult("text_constraint_coverage_required", "error", RepairNeedsTypedAction, message, violation)
}

func RuleCoveragePrerequisiteGuardResult(input RuleCoveragePrerequisiteGuardInput) GuardResult {
	if !input.RuleCoverageRequired || input.RuleCoverageRecords > 0 || input.PostRuleProgress {
		return GuardResult{}
	}
	var blocked []dataquery.DataAction
	for _, action := range input.Actions {
		kind := NormalizeActionKind(action.Kind)
		switch kind {
		case dataquery.DataActionDeriveRules,
			dataquery.DataActionMaterialInventory,
			dataquery.DataActionInspectMaterial,
			dataquery.DataActionExtractRecords:
			continue
		default:
			if DependencyRank(kind) <= 1 && !IsLeafFallback(kind) {
				continue
			}
			blocked = append(blocked, action)
		}
	}
	if len(blocked) == 0 {
		return GuardResult{}
	}
	materials := cleanStrings(input.RuleMaterialPaths)
	if len(materials) == 0 {
		materials = []string{"coverage_contract.rule_coverage_required"}
	}
	sort.Strings(materials)
	first := blocked[0]
	kind := NormalizeActionKind(first.Kind)
	message := fmt.Sprintf("data planning incomplete: rule_coverage_required=true but no rule_coverage records are materialized before action %s (%s). First emit derive_rules from rule/constraint materials [%s], then continue with downstream typed data actions.",
		firstNonEmptyGuardText(strings.TrimSpace(first.ID), "action"),
		kind,
		strings.Join(materials, ", "))
	violation := NewGenericViolation(GenericViolationInput{
		Code:              "rule_coverage_prerequisite_missing",
		Severity:          "error",
		Repairability:     RepairNeedsTypedAction,
		Action:            first,
		InputAliases:      materials,
		RepairActionHints: []string{string(dataquery.DataActionDeriveRules)},
		Reason:            message,
	})
	return NewGuardResult("rule_coverage_prerequisite_missing", "error", RepairNeedsTypedAction, message, violation)
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
