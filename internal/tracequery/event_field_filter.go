package tracequery

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const EventFieldFilterLimit = 16

// EventFieldValue never traverses float64: native nanosecond counters routinely
// exceed the exact-integer range of a JSON double. Canonical output is a string.
type EventFieldValue string

func (v *EventFieldValue) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	raw := string(data)
	if len(data) > 0 && data[0] == '"' {
		if err := json.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("event_field_filters value: %w", err)
		}
	}
	if _, err := parseEventFieldInteger(raw); err != nil {
		return fmt.Errorf("event_field_filters value: %w", err)
	}
	*v = EventFieldValue(raw)
	return nil
}

type EventFieldFilter struct {
	Field string          `json:"field" yaml:"field"`
	Op    string          `json:"op" yaml:"op"`
	Value EventFieldValue `json:"value" yaml:"value"`
}

func EventFieldFilterFields() []string { return []string{"start_ts", "end_ts", "jank_frames", "appid"} }
func EventFieldFilterOps() []string    { return []string{"eq", "ne", "gt", "gte", "lt", "lte"} }

func parseEventFieldInteger(raw string) (int64, error) {
	start := 0
	if strings.HasPrefix(raw, "-") {
		start = 1
	}
	if len(raw) == start {
		return 0, fmt.Errorf("event field value %q must be a decimal int64", raw)
	}
	for i := start; i < len(raw); i++ {
		if raw[i] < '0' || raw[i] > '9' {
			return 0, fmt.Errorf("event field value %q must be a decimal int64", raw)
		}
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("event field value %q is outside int64", raw)
	}
	return n, nil
}

func ValidateEventFieldFilters(view string, filters []EventFieldFilter) error {
	if len(filters) == 0 {
		return nil
	}
	if CanonicalViewName(view) != FallbackViewEventSearch {
		return fmt.Errorf("event_field_filters is only valid for view=event_search")
	}
	if len(filters) > EventFieldFilterLimit {
		return fmt.Errorf("event_field_filters received %d predicates; maximum is %d", len(filters), EventFieldFilterLimit)
	}
	for i, filter := range filters {
		switch filter.Field {
		case "start_ts", "end_ts", "jank_frames", "appid":
		default:
			return fmt.Errorf("event_field_filters[%d]: unknown field %q", i, filter.Field)
		}
		switch filter.Op {
		case "eq", "ne", "gt", "gte", "lt", "lte":
		default:
			return fmt.Errorf("event_field_filters[%d]: unknown operator %q", i, filter.Op)
		}
		if _, err := parseEventFieldInteger(string(filter.Value)); err != nil {
			return fmt.Errorf("event_field_filters[%d]: %w", i, err)
		}
	}
	return nil
}

// eventMatchesFieldFilters is shared by indexed display, its exhaustive census,
// and streaming. A missing/invalid typed field is not equal OR unequal to zero.
func eventMatchesFieldFilters(ev Event, filters []EventFieldFilter) bool {
	if len(filters) == 0 {
		return true
	}
	plugin := ev.PluginFields
	if plugin == nil || plugin.JankEvent == nil || plugin.JankEvent.Values == nil {
		return false
	}
	values := plugin.JankEvent.Values
	for _, filter := range filters {
		var value int64
		switch filter.Field {
		case "start_ts":
			value = values.StartTSNS
		case "end_ts":
			value = values.EndTSNS
		case "jank_frames":
			value = values.JankFrames
		case "appid":
			value = values.AppID
		default:
			return false
		}
		threshold, err := parseEventFieldInteger(string(filter.Value))
		if err != nil {
			return false
		}
		matched := false
		switch filter.Op {
		case "eq":
			matched = value == threshold
		case "ne":
			matched = value != threshold
		case "gt":
			matched = value > threshold
		case "gte":
			matched = value >= threshold
		case "lt":
			matched = value < threshold
		case "lte":
			matched = value <= threshold
		}
		if !matched {
			return false
		}
	}
	return true
}

func eventFieldFilterInvalidResult(idx *Index, q Query, err error) Result {
	result := Result{View: q.View, Caveats: []string{"event_field_filters_invalid=true; " + err.Error()}}
	if idx != nil {
		result.SourcePath = idx.Path
	}
	return result
}
