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
	Allowed      map[string]bool
	AliasGroups  []dataActionParamAliasGroup
	Descriptions map[string]string
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
	DataActionMappingCandidate: {
		Allowed: actionParamSet(
			"source_path", "reference_path",
			"source_field", "name_field", "value_field", "source_fields", "name_fields", "value_fields",
			"reference_field", "reference_fields", "reference_name_field", "reference_name_fields",
			"mapping_source_field", "mapping_source_fields", "lookup_field", "lookup_fields",
			"reference_fields_json", "reference_name_fields_json",
			"canonical_id_field", "canonical_label_field", "match_mode",
			"max_records", "max_output_records", "max_candidates_per_source",
		),
		AliasGroups: []dataActionParamAliasGroup{
			{Canonical: "source_path", Aliases: []string{"base_path", "record_path"}},
			{Canonical: "reference_path", Aliases: []string{"mapping_path", "lookup_path"}},
			{Canonical: "canonical_id_field", Aliases: []string{"id_field", "reference_value_field", "lookup_value_field"}},
			{Canonical: "canonical_label_field", Aliases: []string{"label_field"}},
			{Canonical: "match_mode", Aliases: []string{"mode"}},
		},
	},
	DataActionJoinRecords: {
		Allowed: actionParamSet(
			"left_path", "right_path", "left_fields", "right_fields",
			"join_type", "left_prefix", "right_prefix", "collision", "max_records", "max_output_records",
		),
		AliasGroups: []dataActionParamAliasGroup{
			{Canonical: "left_path", Aliases: []string{"left"}},
			{Canonical: "right_path", Aliases: []string{"right"}},
			{
				Canonical: "left_fields",
				Aliases: []string{
					"left_fields_json", "left_key_fields", "left_key_fields_json",
					"base_fields", "base_fields_json", "base_key_fields", "base_key_fields_json",
					"left_keys", "left_keys_json", "left_key",
					"join_fields", "join_fields_json", "join_key",
				},
			},
			{
				Canonical: "right_fields",
				Aliases: []string{
					"right_fields_json", "right_key_fields", "right_key_fields_json",
					"lookup_fields", "lookup_fields_json", "lookup_key_fields", "lookup_key_fields_json",
					"reference_fields", "reference_fields_json", "reference_key_fields", "reference_key_fields_json",
					"right_keys", "right_keys_json", "right_key",
				},
			},
			{Canonical: "join_type", Aliases: []string{"type"}},
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
	DataActionComputeContribs: {
		Allowed: computeContributionAllowedParams,
	},
	DataActionAssembleAnswer: {
		Allowed: actionParamSet(
			"projection", "order_by", "delimiter", "value_field", "include_keys",
			"output_field",
			"complete_reference", "reference_path", "reference_paths", "reference_key_field", "metric",
		),
		AliasGroups: []dataActionParamAliasGroup{
			{Canonical: "output_field", Aliases: []string{"output_key", "json_field", "target_field", "field"}},
			{Canonical: "reference_key_field", Aliases: []string{"key_field", "group_key_field"}},
		},
		Descriptions: map[string]string{
			"output_field":        "External JSON object field name for projection=json_object. This renames only the published object key; it never changes contribution or reconcile group_key identity.",
			"reference_key_field": "Field in a declared complete-reference source whose values share the internal contribution group_key domain. It selects reference members; it does not rename a JSON output field.",
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

// DataActionAcceptedParamKeys returns the exact parameter-key vocabulary
// accepted at the typed action admission boundary. The returned vocabulary
// includes compatibility aliases because they are valid input declarations;
// the executor subsequently normalizes them to the canonical keys in the same
// contract. Action families without a published fail-closed contract return
// ok=false rather than acquiring a guessed planner-only allowlist.
func DataActionAcceptedParamKeys(kind DataActionKind) (keys []string, ok bool) {
	contract, ok := dataActionParamContracts[normalizeDataActionKind(kind)]
	if !ok {
		return nil, false
	}
	seen := make(map[string]struct{}, len(contract.Allowed))
	for key := range contract.Allowed {
		if key = strings.TrimSpace(key); key != "" {
			seen[key] = struct{}{}
		}
	}
	for _, group := range contract.AliasGroups {
		if key := strings.TrimSpace(group.Canonical); key != "" {
			seen[key] = struct{}{}
		}
		for _, alias := range group.Aliases {
			if alias = strings.TrimSpace(alias); alias != "" {
				seen[alias] = struct{}{}
			}
		}
	}
	keys = make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, true
}

// DataActionParamDescription returns executor-owned teaching for one accepted
// parameter. Planner schemas consume this registry so semantic distinctions
// such as assemble_answer's external output_field versus internal/reference
// group identity cannot drift into a second prompt-only contract.
func DataActionParamDescription(kind DataActionKind, key string) string {
	contract, ok := dataActionParamContracts[normalizeDataActionKind(kind)]
	if !ok {
		return ""
	}
	key = strings.TrimSpace(key)
	if description := strings.TrimSpace(contract.Descriptions[key]); description != "" {
		return description
	}
	for _, group := range contract.AliasGroups {
		if key == group.Canonical {
			return strings.TrimSpace(contract.Descriptions[group.Canonical])
		}
		for _, alias := range group.Aliases {
			if key == alias {
				return strings.TrimSpace(firstNonEmptyString(contract.Descriptions[alias], contract.Descriptions[group.Canonical]))
			}
		}
	}
	return ""
}

// DataActionKindsWithParamContracts enumerates only action families whose
// executor rejects unknown parameter keys. It is deliberately derived from
// the owning runtime registry so planner schemas cannot drift into a second
// source of truth.
func DataActionKindsWithParamContracts() []DataActionKind {
	kinds := make([]DataActionKind, 0, len(dataActionParamContracts))
	for kind := range dataActionParamContracts {
		kinds = append(kinds, kind)
	}
	sort.Slice(kinds, func(i, j int) bool { return kinds[i] < kinds[j] })
	return kinds
}

// applyDataActionParamContract is the common admission-to-execution boundary
// for action families that have published a parameter contract. It uses only
// the typed action kind and parameter keys; question, rule, and answer prose do
// not participate in normalization or rejection.
func applyDataActionParamContract(action DataAction, outputContract OutputContract) (DataAction, error) {
	contract, ok := dataActionParamContracts[normalizeDataActionKind(action.Kind)]
	if !ok {
		return action, nil
	}
	// Keep the compute family's established typed repair guidance while its
	// accepted key set is now owned by the common registry used by planner
	// schema projection. This remains the only admission call site.
	if normalizeDataActionKind(action.Kind) == DataActionComputeContribs {
		return action, validateComputeContributionActionParams(action)
	}
	if normalizeDataActionKind(action.Kind) == DataActionAssembleAnswer {
		if groupKey, present := action.Params["group_key"]; present {
			return action, DataActionParamError{
				ActionKind:    action.Kind,
				Param:         "group_key",
				ExpectedShape: "output_field for the external JSON object key, or reference_key_field for a declared complete-reference source",
				ActualSnippet: strings.TrimSpace(groupKey),
				Message:       "assemble_answer does not accept the overloaded group_key carrier: use output_field for the external JSON object key or reference_key_field for the reference-member domain",
			}
		}
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
	if normalizeDataActionKind(action.Kind) == DataActionAssembleAnswer {
		projection, err := normalizeActionOutputProjection(params["projection"], outputContract.Format)
		if err != nil {
			return action, err
		}
		if strings.TrimSpace(params["projection"]) != "" {
			params["projection"] = projection
		}
		outputField := strings.TrimSpace(params["output_field"])
		if outputField != "" && projection == "" && outputContract.Normalize().Format == OutputJSONOnly {
			// An explicit external object key is already a typed shape choice.
			// Make it executable instead of allowing a shape-dependent default
			// to ignore output_field for some reconcile groups.
			projection = "json_object"
			params["projection"] = projection
		}
		if outputField != "" && projection != "json_object" {
			// An omitted projection is contract-dependent: under json_only
			// the same declaration defaults to json_object above. An
			// explicit non-object projection is rejected under every format.
			contractFormat := OutputFormat("")
			if projection == "" {
				contractFormat = outputContract.Normalize().Format
			}
			return action, DataActionParamError{
				ActionKind:    action.Kind,
				Param:         "output_field/projection",
				ExpectedShape: "output_field with projection=json_object, or an omitted projection under output_contract.format=json_only",
				ActualSnippet: "projection=" + projection,
				Message:       "assemble_answer output_field names an external JSON object key and would be ignored by the selected non-object projection",
				OutputFormat:  contractFormat,
			}
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

// NormalizeDataActionForOutputContract is the shared plan/admission boundary
// for one typed action. Planner-side pre-dispatch repair, workflow admission
// (dataworkflow.ActionOutputContractGuardResult) and ActionRunner must call
// this same implementation rather than maintaining separate semantic
// matrices. Every caller must pass the output contract the plan will EXECUTE
// under (V9-4, colleague_merge_audit §40.56): the verdict for
// assemble_answer depends on outputContract.Format, so judging a draft
// contract here and a carried contract at execution is two snapshots of one
// identifier space.
func NormalizeDataActionForOutputContract(action DataAction, outputContract OutputContract) (DataAction, error) {
	return applyDataActionParamContract(action, outputContract)
}

// DataActionParamError is the executor-owned typed rejection for one action
// parameter. OutputFormat is non-empty only when the verdict was judged
// against the plan's output contract (assemble_answer projection × format,
// or the output_field default that exists only under json_only): such a
// rejection is a function of the contract the action executes under, which
// is what lets workflow admission separate "this action cannot satisfy the
// contract in effect" from contract-independent parameter defects without
// scanning message prose.
type DataActionParamError struct {
	ActionKind    DataActionKind `json:"action_kind,omitempty"`
	Param         string         `json:"param,omitempty"`
	ExpectedShape string         `json:"expected_shape,omitempty"`
	ActualSnippet string         `json:"actual_snippet,omitempty"`
	Message       string         `json:"message,omitempty"`
	OutputFormat  OutputFormat   `json:"output_format,omitempty"`
	Cause         error          `json:"-"`
}

// OutputContractDependent reports whether the rejection would change under a
// different output_contract.format — the precise signal for the admission
// drift guard.
func (e DataActionParamError) OutputContractDependent() bool {
	return strings.TrimSpace(string(e.OutputFormat)) != ""
}

func (e DataActionParamError) Error() string {
	if text := strings.TrimSpace(e.Message); text != "" {
		return text
	}
	kind := strings.TrimSpace(string(e.ActionKind))
	if kind == "" {
		kind = "data action"
	}
	param := strings.TrimSpace(e.Param)
	if param == "" {
		param = "parameter"
	}
	expected := strings.TrimSpace(e.ExpectedShape)
	if expected == "" {
		expected = "schema-compatible value"
	}
	msg := fmt.Sprintf("%s parameter %q does not match %s", kind, param, expected)
	if e.Cause != nil {
		msg += ": " + e.Cause.Error()
	}
	return msg
}

func (e DataActionParamError) Unwrap() error {
	return e.Cause
}

func (e DataActionParamError) Violation() DataTaskViolation {
	return DataTaskViolation{
		Code:          "action_param_violation",
		Summary:       clampViolationText(e.Error(), 500),
		ActionKind:    strings.TrimSpace(string(e.ActionKind)),
		Param:         strings.TrimSpace(e.Param),
		ExpectedShape: firstNonEmptyString(strings.TrimSpace(e.ExpectedShape), "schema-compatible action parameter"),
		ActualSnippet: clampViolationText(e.ActualSnippet, 300),
		Repairability: RepairabilityNeedsRecompute,
		RepairHint:    "Use the typed action schema for this parameter. Keep arrays as JSON arrays and objects as JSON objects; split the batch if the parameter belongs to a later action.",
	}
}

func dataActionParamError(kind DataActionKind, param, expectedShape, actualSnippet string, cause error) DataActionParamError {
	return DataActionParamError{
		ActionKind:    kind,
		Param:         strings.TrimSpace(param),
		ExpectedShape: strings.TrimSpace(expectedShape),
		ActualSnippet: strings.TrimSpace(actualSnippet),
		Cause:         cause,
	}
}

func dataActionMissingParamError(kind DataActionKind, param, expectedShape string, actual any) DataActionParamError {
	return dataActionParamError(kind, param, expectedShape, actionParamActualSnippet(actual), nil)
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
