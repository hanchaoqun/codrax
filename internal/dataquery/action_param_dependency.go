package dataquery

import "strings"

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

// DataActionParamDependencyContracts returns defensive copies of the
// executor-owned parameter dependencies projected into planner JSON schema.
func DataActionParamDependencyContracts() []ActionParamDependencyContract {
	return []ActionParamDependencyContract{cloneActionParamDependencyContract(assembleReferenceOrderDependency)}
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
