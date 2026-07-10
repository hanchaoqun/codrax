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

// TraceMarkActions mirrors TraceEventTypes' small-model compatibility at the
// JSON boundary while keeping a distinct semantic type. The schema remains a
// closed uppercase enum; validation against tracequery's canonical registry
// happens after decode so unknown or duplicate tokens fail loud.
type TraceMarkActions []string

func (actions TraceMarkActions) Strings() []string { return []string(actions) }

func (actions *TraceMarkActions) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*actions = nil
		return nil
	}
	if trimmed[0] == '"' {
		var raw string
		if err := json.Unmarshal(trimmed, &raw); err != nil {
			return fmt.Errorf("trace-mark-actions: %w", err)
		}
		items := splitTraceStringList(raw)
		for i := range items {
			items[i] = normalizeTraceMarkActionToken(items[i])
		}
		*actions = TraceMarkActions(items)
		return nil
	}
	var raw []string
	if err := json.Unmarshal(trimmed, &raw); err != nil {
		return fmt.Errorf("trace-mark-actions: %w", err)
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, normalizeTraceMarkActionToken(item))
		}
	}
	*actions = TraceMarkActions(out)
	return nil
}

func normalizeTraceMarkActionToken(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) == 1 && raw[0] >= 'a' && raw[0] <= 'z' {
		return string(raw[0] - ('a' - 'A'))
	}
	return raw
}

func splitTraceStringList(raw string) []string {
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
		if field = strings.Trim(strings.TrimSpace(field), `"'`); field != "" {
			out = append(out, field)
		}
	}
	return out
}
