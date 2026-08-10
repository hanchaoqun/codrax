package dataquery

import (
	"encoding/json"
	"fmt"
	"strings"
)

func defaultGeneratedBlockingStatusValues() map[string]bool {
	return map[string]bool{
		"ambiguous":          true,
		"matched_ambiguous":  true,
		"missing":            true,
		"missing_source":     true,
		"not_applicable":     true,
		"not_matched":        true,
		"unmatched":          true,
		"unmatched_required": true,
		"unresolved":         true,
		"conflict":           true,
		"invalid":            true,
	}
}

// contributionBlockedGeneratedStatusFacts preserves the exact observed value
// and its source location as separate fields. Human-readable error text is a
// projection of these facts, never the input to repair planning.
func contributionBlockedGeneratedStatusFacts(record actionRecord, statusFields []string, statusFilterFields map[string]bool, blockedStatuses map[string]bool) []ObservedFieldValue {
	var facts []ObservedFieldValue
	for _, field := range statusFields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		value := strings.ToLower(strings.TrimSpace(recordField(record.Fields, field)))
		if value == "" || !blockedStatuses[value] {
			continue
		}
		facts = append(facts, ObservedFieldValue{
			Field:               field,
			Value:               value,
			SourceLocator:       fmt.Sprintf("line:%d", record.Line),
			RecordID:            firstNonEmptyString(recordField(record.Fields, "id"), recordField(record.Fields, "item_id"), recordField(record.Fields, "row_id")),
			SelectedAfterFilter: statusFilterFields[strings.ToLower(field)],
		})
	}
	return facts
}

func renderObservedStatusFacts(facts []ObservedFieldValue) []string {
	out := make([]string, 0, len(facts))
	for _, fact := range facts {
		locator := firstNonEmptyString(strings.TrimSpace(fact.RecordID), strings.TrimSpace(fact.SourceLocator), "unknown")
		suffix := ""
		if fact.SelectedAfterFilter {
			suffix = ":after_filter"
		}
		out = append(out, fmt.Sprintf("%s=%s@%s%s", strings.TrimSpace(fact.Field), strings.TrimSpace(fact.Value), locator, suffix))
	}
	return out
}

// generatedStatusRepairParams uses the canonical qualify_records parameter
// schema. It is a typed proposal fragment only; the workflow still requires a
// model-authored action and runs the ordinary guard/runner/validator gates.
func generatedStatusRepairParams(facts []ObservedFieldValue) map[string]string {
	var fields []string
	var values []string
	for _, fact := range facts {
		fields = append(fields, strings.TrimSpace(fact.Field))
		values = append(values, strings.TrimSpace(fact.Value))
	}
	fields = uniqueTrimmedStrings(fields)
	values = uniqueTrimmedStrings(values)
	if len(fields) == 0 || len(values) == 0 {
		return nil
	}
	fieldJSON, fieldErr := json.Marshal(fields)
	valueJSON, valueErr := json.Marshal(values)
	if fieldErr != nil || valueErr != nil {
		return nil
	}
	return map[string]string{
		"status_fields":      string(fieldJSON),
		"blocked_statuses":   string(valueJSON),
		"auto_status_fields": "false",
		"output_mode":        "filter",
	}
}

func uniqueTrimmedStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, value := range in {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if value == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}
