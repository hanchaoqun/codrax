package dataworkflow

import (
	"encoding/json"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func NormalizeRolePathActionInputs(plan *dataquery.TaskPlan) bool {
	if plan == nil || len(plan.Actions) == 0 {
		return false
	}
	changed := false
	for i := range plan.Actions {
		action, actionChanged := NormalizeRolePathAction(plan.Actions[i])
		if actionChanged {
			plan.Actions[i] = action
			changed = true
		}
	}
	return changed
}

func NormalizeRolePathAction(action dataquery.DataAction) (dataquery.DataAction, bool) {
	roleInputs := ActionRoleInputPaths(action)
	if len(roleInputs) == 0 {
		return action, false
	}
	next := mergeActionInputPaths(action.InputPaths, roleInputs)
	if strings.Join(next, "\x00") == strings.Join(cleanStrings(action.InputPaths), "\x00") {
		return action, false
	}
	action.InputPaths = next
	return action, true
}

func ActionRoleInputPaths(action dataquery.DataAction) []string {
	params := action.Params
	if len(params) == 0 {
		return nil
	}
	switch NormalizeActionKind(action.Kind) {
	case dataquery.DataActionNormalizeEntities:
		return cleanStrings([]string{
			params["source_path"],
			params["base_path"],
			params["record_path"],
			params["reference_path"],
			params["lookup_path"],
			params["mapping_path"],
		})
	case dataquery.DataActionMappingCandidate:
		return cleanStrings([]string{
			params["source_path"],
			params["base_path"],
			params["record_path"],
			params["reference_path"],
			params["lookup_path"],
			params["mapping_path"],
		})
	case dataquery.DataActionEnrichRecords:
		return cleanStrings([]string{
			params["base_path"],
			params["source_path"],
			params["record_path"],
			params["reference_path"],
			params["lookup_path"],
			params["mapping_path"],
		})
	case dataquery.DataActionJoinRecords:
		return cleanStrings([]string{
			params["left_path"],
			params["right_path"],
			params["base_path"],
			params["reference_path"],
		})
	case dataquery.DataActionApplyResolutions:
		paths := []string{
			params["base_path"],
			params["record_path"],
			params["source_path"],
			params["resolution_path"],
			params["mapping_path"],
			params["lookup_path"],
			params["reference_path"],
		}
		for _, spec := range actionObjectListParam(params, "resolution_specs_json", "resolution_specs") {
			paths = append(paths,
				mapStringValue(spec, "base_path"),
				mapStringValue(spec, "record_path"),
				mapStringValue(spec, "source_path"),
				mapStringValue(spec, "resolution_path"),
				mapStringValue(spec, "mapping_path"),
				mapStringValue(spec, "lookup_path"),
				mapStringValue(spec, "reference_path"),
			)
		}
		return cleanStrings(paths)
	default:
		return nil
	}
}

func mergeActionInputPaths(paths, required []string) []string {
	out := make([]string, 0, len(paths)+len(required))
	seen := map[string]bool{}
	for _, list := range [][]string{paths, required} {
		for _, value := range list {
			value = strings.TrimSpace(value)
			if value == "" || seen[value] {
				continue
			}
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

func actionObjectListParam(params map[string]string, keys ...string) []map[string]any {
	for _, key := range keys {
		raw := strings.TrimSpace(params[key])
		if raw == "" {
			continue
		}
		var list []map[string]any
		if err := json.Unmarshal([]byte(raw), &list); err == nil {
			return list
		}
		var one map[string]any
		if err := json.Unmarshal([]byte(raw), &one); err == nil {
			return []map[string]any{one}
		}
	}
	return nil
}

func mapStringValue(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	switch value := values[key].(type) {
	case string:
		return strings.TrimSpace(value)
	default:
		return ""
	}
}
