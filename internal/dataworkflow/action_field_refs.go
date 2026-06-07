package dataworkflow

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

// SingleRecordSetActionFieldRefs returns the input-field contract for typed
// actions that consume exactly one record-shaped artifact. It is intentionally
// domain-neutral: it reads only action kind and structured params.
func SingleRecordSetActionFieldRefs(kind dataquery.DataActionKind, action dataquery.DataAction) []string {
	switch NormalizeActionKind(kind) {
	case dataquery.DataActionDeriveFields, dataquery.DataActionExtractFields:
		return DeriveActionSourceFieldRefs(action)
	case dataquery.DataActionGroupRecords:
		return GroupRecordsActionFieldRefs(action)
	case dataquery.DataActionExpandRecords:
		return parseActionStringList(firstNonEmpty(
			action.Params["source_field"],
			action.Params["input_field"],
			action.Params["field"],
			action.Params["value_field"],
		))
	case dataquery.DataActionFilterRecords:
		return FilterActionFieldRefs(action)
	case dataquery.DataActionQualifyRecords:
		return QualifyActionFieldRefs(action)
	default:
		return nil
	}
}

func DeriveActionSourceFieldRefs(action dataquery.DataAction) []string {
	var fields []string
	for _, spec := range actionObjectListParam(action.Params, "field_specs_json", "extract_specs_json", "derive_specs_json", "transforms_json", "field_specs", "extract_specs", "derive_specs", "transforms") {
		op := strings.ToLower(strings.TrimSpace(firstNonEmpty(
			mapStringValue(spec, "operation"),
			mapStringValue(spec, "op"),
			mapStringValue(spec, "transform"),
		)))
		sources := fieldRefsFromSpec(spec, "source_field", "input_field", "field", "value_field")
		sources = append(sources, fieldRefsFromSpec(spec, "source_fields", "input_fields", "fields")...)
		if len(sources) == 0 && op != "constant" {
			sources = append(sources, fieldRefsFromSpec(spec, "left_field", "right_field")...)
		}
		fields = append(fields, sources...)
	}
	fields = append(fields, parseActionStringList(firstNonEmpty(
		action.Params["source_field"],
		action.Params["input_field"],
		action.Params["field"],
		action.Params["value_field"],
	))...)
	return cleanStrings(fields)
}

func GroupRecordsActionFieldRefs(action dataquery.DataAction) []string {
	var fields []string
	fields = append(fields, parseActionStringList(firstNonEmpty(
		action.Params["group_fields"],
		action.Params["group_by_fields"],
		action.Params["key_fields"],
		action.Params["group_field"],
		action.Params["group_by"],
		action.Params["key_field"],
	))...)
	fields = append(fields, parseActionStringList(firstNonEmpty(
		action.Params["text_fields"],
		action.Params["concat_fields"],
		action.Params["aggregate_fields"],
		action.Params["source_fields"],
		action.Params["text_field"],
		action.Params["source_field"],
		action.Params["field"],
	))...)
	fields = append(fields, parseActionStringList(firstNonEmpty(
		action.Params["first_fields"],
		action.Params["copy_fields"],
		action.Params["preserve_fields"],
		action.Params["keep_fields"],
	))...)
	return cleanStrings(fields)
}

func FilterActionFieldRefs(action dataquery.DataAction) []string {
	var fields []string
	for _, spec := range actionObjectListParam(action.Params, "filters_json", "filters") {
		fields = append(fields, fieldRefsFromSpec(spec, "field", "source_field", "input_field")...)
	}
	fields = append(fields, parseActionStringList(action.Params["filter_field"])...)
	return cleanStrings(fields)
}

func QualifyActionFieldRefs(action dataquery.DataAction) []string {
	var fields []string
	fields = append(fields, FilterActionFieldRefs(action)...)
	for _, spec := range actionObjectListParam(action.Params, "reject_filters_json", "exclude_filters_json", "block_filters_json", "reject_filters", "exclude_filters", "block_filters") {
		fields = append(fields, fieldRefsFromSpec(spec, "field", "source_field", "input_field")...)
	}
	fields = append(fields, parseActionStringList(firstNonEmpty(action.Params["required_fields"], action.Params["non_empty_fields"]))...)
	fields = append(fields, parseActionStringList(firstNonEmpty(action.Params["evidence_fields"], action.Params["required_evidence_fields"]))...)
	fields = append(fields, parseActionStringList(firstNonEmpty(action.Params["status_fields"], action.Params["generated_status_fields"]))...)
	return cleanStrings(fields)
}

func ComputeContributionActionFieldRefs(action dataquery.DataAction) []string {
	var fields []string
	fields = append(fields, parseActionStringList(action.Params["value_field"])...)
	fields = append(fields, parseActionStringList(action.Params["group_key_field"])...)
	fields = append(fields, FilterActionFieldRefs(action)...)
	return cleanStrings(fields)
}

func fieldRefsFromSpec(spec map[string]any, keys ...string) []string {
	var out []string
	for _, key := range keys {
		value, ok := spec[key]
		if !ok {
			continue
		}
		out = append(out, anyStringList(value)...)
	}
	return cleanStrings(out)
}

func anyStringList(value any) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		return parseActionStringList(typed)
	case []string:
		return cleanStrings(typed)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, anyStringList(item)...)
		}
		return cleanStrings(out)
	default:
		return cleanStrings([]string{fmt.Sprint(typed)})
	}
}

func parseActionStringList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(raw, "[") {
		var stringsValue []string
		if err := json.Unmarshal([]byte(raw), &stringsValue); err == nil {
			return cleanStrings(stringsValue)
		}
		var anyValue []any
		if err := json.Unmarshal([]byte(raw), &anyValue); err == nil {
			out := make([]string, 0, len(anyValue))
			for _, item := range anyValue {
				if item == nil {
					continue
				}
				out = append(out, strings.TrimSpace(fmt.Sprint(item)))
			}
			return cleanStrings(out)
		}
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case ',', '，', ';', '；', '|', '\n', '\r', '\t':
			return true
		default:
			return false
		}
	})
	return cleanStrings(fields)
}
