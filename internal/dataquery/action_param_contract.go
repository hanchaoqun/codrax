package dataquery

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// dataActionParamAliasGroup declares names that have exactly the same typed
// meaning for one action family. Aliases are normalized before execution so a
// planner cannot successfully publish a parameter that the executor then
// silently ignores. Different non-empty values in one group fail closed: the
// runner must not guess which declaration owns the action semantics.
type dataActionParamAliasGroup struct {
	Canonical string
	Aliases   []string
}

type dataActionParamContract struct {
	Allowed     map[string]bool
	AliasGroups []dataActionParamAliasGroup
}

var dataActionParamContracts = map[DataActionKind]dataActionParamContract{
	DataActionFilterRecords: {
		Allowed: actionParamSet(
			"input_path",
			"filters_json", "filter_field", "filter_op", "filter_value", "filter_equals", "filter_not_equals",
			"rule_refs", "item_id_field", "reason", "max_records", "max_output_records",
		),
		AliasGroups: []dataActionParamAliasGroup{
			{Canonical: "input_path", Aliases: []string{"record_path", "base_path"}},
			{
				Canonical: "filters_json",
				Aliases: []string{
					"filters",
					"source_filters", "source_filters_json",
					"base_filters", "base_filters_json",
					"record_filters", "record_filters_json",
				},
			},
		},
	},
	DataActionValueDistribution: {
		Allowed: actionParamSet(
			"input_path", "fields", "top_n", "max_fields", "max_records",
		),
		AliasGroups: []dataActionParamAliasGroup{
			{Canonical: "input_path", Aliases: []string{"record_path", "base_path"}},
			{Canonical: "fields", Aliases: []string{"fields_json", "field", "target_fields"}},
		},
	},
	DataActionQualifyRecords: {
		Allowed: actionParamSet(
			"input_path",
			"filters_json", "filter_field", "filter_op", "filter_value", "filter_equals", "filter_not_equals",
			"reject_filters_json",
			"required_fields", "evidence_fields", "status_fields", "accepted_statuses", "blocked_statuses",
			"auto_status_fields", "pass_field", "reason_field", "item_id_field", "output_mode", "rule_refs",
			"max_records", "max_output_records",
		),
		AliasGroups: []dataActionParamAliasGroup{
			{Canonical: "input_path", Aliases: []string{"record_path", "base_path"}},
			{
				Canonical: "filters_json",
				Aliases: []string{
					"filters",
					"source_filters", "source_filters_json",
					"base_filters", "base_filters_json",
					"record_filters", "record_filters_json",
				},
			},
			{
				Canonical: "reject_filters_json",
				Aliases: []string{
					"reject_filters",
					"exclude_filters", "exclude_filters_json",
					"block_filters", "block_filters_json",
				},
			},
			{Canonical: "required_fields", Aliases: []string{"non_empty_fields"}},
			{Canonical: "evidence_fields", Aliases: []string{"required_evidence_fields"}},
			{Canonical: "status_fields", Aliases: []string{"generated_status_fields"}},
			{Canonical: "pass_field", Aliases: []string{"eligible_field"}},
			{Canonical: "output_mode", Aliases: []string{"mode"}},
		},
	},
}

func actionParamSet(keys ...string) map[string]bool {
	out := make(map[string]bool, len(keys))
	for _, key := range keys {
		if key = strings.TrimSpace(key); key != "" {
			out[key] = true
		}
	}
	return out
}

func actionParamKeys(params map[string]string) []string {
	keys := make([]string, 0, len(params))
	for key := range params {
		key = strings.TrimSpace(key)
		if key != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

// applyDataActionParamContract is the common admission-to-execution boundary
// for action families that have published a parameter contract. It uses only
// the typed action kind and parameter keys; question, rule, and answer prose do
// not participate in normalization or rejection.
func applyDataActionParamContract(action DataAction) (DataAction, error) {
	contract, ok := dataActionParamContracts[normalizeDataActionKind(action.Kind)]
	if !ok {
		return action, nil
	}
	params := make(map[string]string, len(action.Params))
	for key, value := range action.Params {
		params[key] = value
	}
	for _, group := range contract.AliasGroups {
		canonical := strings.TrimSpace(group.Canonical)
		keys := append([]string{canonical}, group.Aliases...)
		selectedKey := ""
		selectedValue := ""
		for _, key := range keys {
			value, exists := params[key]
			if !exists || strings.TrimSpace(value) == "" {
				continue
			}
			if selectedKey == "" {
				selectedKey = key
				selectedValue = value
				continue
			}
			if !equivalentActionParamValues(selectedValue, value) {
				return action, DataActionParamError{
					ActionKind:    action.Kind,
					Param:         selectedKey + "/" + key,
					ExpectedShape: "one unambiguous value for the canonical " + canonical + " parameter",
					ActualSnippet: selectedKey + "," + key,
					Message: fmt.Sprintf(
						"%s parameters %q and %q are aliases for %q but carry different values; emit one canonical declaration instead of asking the executor to guess",
						action.Kind, selectedKey, key, canonical,
					),
				}
			}
		}
		for _, alias := range group.Aliases {
			delete(params, alias)
		}
		if selectedKey != "" {
			params[canonical] = selectedValue
		}
	}

	unknown := make([]string, 0)
	for key := range params {
		if key != "" && !contract.Allowed[key] {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		allowed := make([]string, 0, len(contract.Allowed))
		for key := range contract.Allowed {
			allowed = append(allowed, key)
		}
		sort.Strings(allowed)
		return action, DataActionParamError{
			ActionKind:    action.Kind,
			Param:         strings.Join(unknown, ","),
			ExpectedShape: "only parameters consumed by the typed " + string(action.Kind) + " executor",
			ActualSnippet: strings.Join(unknown, ","),
			Message: fmt.Sprintf(
				"%s has unsupported parameter(s) [%s]; allowed canonical parameters are [%s]. Unknown parameters are rejected because silently ignoring a typed declaration would change the requested data semantics",
				action.Kind, strings.Join(unknown, ", "), strings.Join(allowed, ", "),
			),
		}
	}
	action.Params = params
	return action, nil
}

func equivalentActionParamValues(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == right {
		return true
	}
	leftJSON, leftOK := canonicalActionParamJSON(left)
	rightJSON, rightOK := canonicalActionParamJSON(right)
	return leftOK && rightOK && leftJSON == rightJSON
}

func canonicalActionParamJSON(raw string) (string, bool) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return "", false
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", false
	}
	return string(encoded), true
}
