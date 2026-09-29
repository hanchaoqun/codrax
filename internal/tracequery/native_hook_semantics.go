package tracequery

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// This is the native parser for the exporter's closed NativeHook instant
// metadata grammar. It is a display receipt, not an execution, pairing, stack,
// unit or source-authenticity authority. Other marker names are left untouched.
func attachNativeHookSemantics(event *Event) {
	if event.Type != EventTraceMark || event.SpanAction != "I" || !strings.HasPrefix(event.SpanName, "NativeHook:") {
		return
	}
	operation, tail, _ := strings.Cut(strings.TrimPrefix(event.SpanName, "NativeHook:"), " ")
	if operation == "" {
		return
	}
	fields := map[string]string{}
	for tail != "" {
		tail = strings.TrimLeft(tail, " ")
		if tail == "" {
			break
		}
		key, value, ok := strings.Cut(tail, "=")
		if !ok || !nativeHookMetadataKey(key) {
			return
		}
		if _, duplicate := fields[key]; duplicate {
			return
		}
		end := strings.IndexByte(value, ' ')
		if strings.HasPrefix(value, `"`) {
			decoder := json.NewDecoder(strings.NewReader(value))
			var text string
			if err := decoder.Decode(&text); err != nil {
				return
			}
			end = int(decoder.InputOffset())
			if end < len(value) && value[end] != ' ' {
				return
			}
		} else if end < 0 {
			end = len(value)
		}
		fields[key], tail = value[:end], value[end:]
	}
	p := traceEventSemanticProjector{value: types.TraceEventSemantics{SchemaVersion: types.TraceEventSemanticsVersion}}
	p.known("resource.operation", operation)
	for _, field := range nativeHookMetadataFields {
		value, present := fields[field.wire]
		if !present {
			p.unknown(field.key, "unavailable", "not_published")
			continue
		}
		if value == "null" {
			p.unknown(field.key, "unavailable", "source_null")
			continue
		}
		if field.wire == "source_sub_type_name" {
			var text string
			if !strings.HasPrefix(value, `"`) || json.Unmarshal([]byte(value), &text) != nil {
				p.unknown(field.key, "invalid", "invalid_source_value")
				continue
			}
			p.known(field.key, text)
			continue
		}
		if field.wire == "source_addr_bits_hex" {
			bits, err := strconv.ParseUint(strings.TrimPrefix(value, "0x"), 16, 64)
			if err != nil || fmt.Sprintf("0x%016x", bits) != value {
				p.unknown(field.key, "invalid", "invalid_source_value")
				continue
			}
		} else {
			number, err := strconv.ParseInt(value, 10, 64)
			if err != nil || strconv.FormatInt(number, 10) != value || field.wire == "source_heap_size" && number < 0 {
				p.unknown(field.key, "invalid", "invalid_source_value")
				continue
			}
		}
		p.known(field.key, value)
	}
	// Never choose a winning representation when two explicit address views
	// disagree. The original marker remains available for diagnosis.
	if raw, ok := fields["source_addr_i64"]; ok {
		if bits, exists := fields["source_addr_bits_hex"]; exists {
			number, err := strconv.ParseInt(raw, 10, 64)
			consistent := raw == "null" && bits == "null" || err == nil && fmt.Sprintf("0x%016x", uint64(number)) == bits
			if !consistent {
				for n, field := range p.value.Fields {
					if field.Key == "resource.address_i64" || field.Key == "resource.address_bits_hex" {
						p.value.Fields[n].Status, p.value.Fields[n].IssueReason, p.value.Fields[n].Value = "invalid", "address_representation_conflict", nil
					}
				}
			}
		}
	}
	semantics := p.finish()
	if semantics == nil {
		return
	}
	if event.PluginFields == nil {
		event.PluginFields = &PluginFields{}
	}
	event.PluginFields.NativeHookSemantics = semantics
}

var nativeHookMetadataFields = []struct{ wire, key string }{
	{"resource_end_ts_ns", "resource.end_ts_ns"},
	{"source_heap_size", "resource.size"},
	{"source_callchain_id", "resource.callchain_id"},
	{"source_addr_i64", "resource.address_i64"},
	{"source_addr_bits_hex", "resource.address_bits_hex"},
	{"source_sub_type_id", "resource.sub_type_id"},
	{"source_sub_type_name", "resource.sub_type_name"},
}

func nativeHookMetadataKey(key string) bool {
	for _, field := range nativeHookMetadataFields {
		if field.wire == key {
			return true
		}
	}
	return false
}
