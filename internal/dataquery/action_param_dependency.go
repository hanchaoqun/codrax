package dataquery

import (
	"fmt"
	"strings"
)

// ActionParamDependencyContract describes a typed cross-field dependency
// owned by an action executor. RequiredActionParamGroups is conjunctive across
// groups and disjunctive within a group: one non-empty parameter from every
// group is required unless one of the explicit typed alternatives applies.
// Planner schemas consume this registry; they must not reconstruct these
// dependencies from prompt or answer prose.
type ActionParamDependencyContract struct {
	Kind                               DataActionKind
	TriggerParam                       string
	TriggerValue                       string
	RequiredActionParamGroups          [][]string
	OutputCompleteReferenceAlternative bool
	ActionCompleteReferenceAlternative bool
}

// ActionParamExclusionContract describes a precise typed combination that an
// executor cannot consume. It is shared by runtime admission and planner
// schema projection so a model is not taught a combination that the same
// executor will deterministically reject after dispatch.
type ActionParamExclusionContract struct {
	Kind            DataActionKind
	TriggerParam    string
	TriggerValue    string
	ForbiddenParams []string
	Reason          string
}

var assembleReferenceOrderDependency = ActionParamDependencyContract{
	Kind:         DataActionAssembleAnswer,
	TriggerParam: "order_by",
	TriggerValue: "reference",
	RequiredActionParamGroups: [][]string{
		{"reference_paths", "reference_path"},
		{"reference_key_field", "key_field", "group_key_field"},
	},
	// A complete-reference output contract already carries the canonical
	// reference_path/reference_key_field pair. An action may instead declare
	// complete_reference=true and resolve the same typed authority from its
	// input/artifact scope, or provide the action-local pair above.
	OutputCompleteReferenceAlternative: true,
	ActionCompleteReferenceAlternative: true,
}

var actionParamExclusionContracts = []ActionParamExclusionContract{
	{
		Kind:            DataActionComputeContribs,
		TriggerParam:    "operation",
		TriggerValue:    "count",
		ForbiddenParams: []string{"value_field"},
		Reason:          "operation=count emits row-count value 1 and cannot also preserve value_field members",
	},
}

// DataActionParamDependencyContracts returns defensive copies of the
// executor-owned parameter dependencies projected into planner JSON schema.
func DataActionParamDependencyContracts() []ActionParamDependencyContract {
	return []ActionParamDependencyContract{cloneActionParamDependencyContract(assembleReferenceOrderDependency)}
}

// DataActionParamExclusionContracts returns defensive copies of the runtime
// executor exclusions projected into planner schema.
func DataActionParamExclusionContracts() []ActionParamExclusionContract {
	out := make([]ActionParamExclusionContract, 0, len(actionParamExclusionContracts))
	for _, contract := range actionParamExclusionContracts {
		copy := contract
		copy.ForbiddenParams = append([]string(nil), contract.ForbiddenParams...)
		out = append(out, copy)
	}
	return out
}

func validateActionParamExclusions(action DataAction) error {
	kind := normalizeDataActionKind(action.Kind)
	for _, contract := range actionParamExclusionContracts {
		if kind != contract.Kind {
			continue
		}
		trigger, _ := normalizeContributionOperation(action.Params[contract.TriggerParam])
		if trigger == "" {
			trigger = strings.ToLower(strings.TrimSpace(action.Params[contract.TriggerParam]))
		}
		if trigger != contract.TriggerValue {
			continue
		}
		for _, forbidden := range contract.ForbiddenParams {
			if strings.TrimSpace(action.Params[forbidden]) == "" {
				continue
			}
			return DataActionParamError{
				ActionKind:    action.Kind,
				Param:         forbidden,
				ExpectedShape: fmt.Sprintf("empty when %s=%s, or choose an operation that consumes %s", contract.TriggerParam, contract.TriggerValue, forbidden),
				ActualSnippet: strings.TrimSpace(action.Params[forbidden]),
				Message:       fmt.Sprintf("%s %s=%s ignores %s and forbids the combination: %s; remove %s or choose a member-preserving operation such as operation=include, operation=set, or operation=rank", action.Kind, contract.TriggerParam, contract.TriggerValue, forbidden, contract.Reason, forbidden),
			}
		}
	}
	return nil
}

func cloneActionParamDependencyContract(in ActionParamDependencyContract) ActionParamDependencyContract {
	out := in
	out.RequiredActionParamGroups = make([][]string, 0, len(in.RequiredActionParamGroups))
	for _, group := range in.RequiredActionParamGroups {
		out.RequiredActionParamGroups = append(out.RequiredActionParamGroups, append([]string(nil), group...))
	}
	return out
}

func actionParamsSatisfyDependencyGroups(params map[string]string, groups [][]string) bool {
	for _, group := range groups {
		matched := false
		for _, key := range group {
			if strings.TrimSpace(params[strings.TrimSpace(key)]) != "" {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return len(groups) > 0
}
