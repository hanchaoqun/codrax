package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// TraceEventTypes accepts either a normal JSON string array or a delimited
// string. The schema still teaches models to emit an array; this fallback keeps
// trace_query robust when a model writes "sched_switch,sched_wakeup".
type TraceEventTypes []string

func (et TraceEventTypes) Strings() []string { return []string(et) }

func (et *TraceEventTypes) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*et = nil
		return nil
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return fmt.Errorf("trace-event-types: %w", err)
		}
		*et = splitTraceEventTypesLiteral(s)
		return nil
	}
	var arr []string
	if err := json.Unmarshal(trimmed, &arr); err != nil {
		return fmt.Errorf("trace-event-types: %w", err)
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	*et = out
	return nil
}

func splitTraceEventTypesLiteral(raw string) TraceEventTypes {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case ',', ';', '|', '\n', '\r', '\t', ' ':
			return true
		default:
			return false
		}
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.Trim(strings.TrimSpace(field), `"'`)
		if field != "" {
			out = append(out, field)
		}
	}
	return TraceEventTypes(out)
}
