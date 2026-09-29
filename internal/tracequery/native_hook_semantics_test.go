package tracequery

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestNativeHookSemanticsExactNullableAndInvalid(t *testing.T) {
	for _, tc := range []struct{ name, metadata, key, value, status, reason string }{
		{"large", "source_heap_size=9007199254740993", "resource.size", "9007199254740993", "known", ""},
		{"negative_bits", "source_addr_i64=-1 source_addr_bits_hex=0xffffffffffffffff", "resource.address_i64", "-1", "known", ""},
		{"min_bits", "source_addr_i64=-9223372036854775808 source_addr_bits_hex=0x8000000000000000", "resource.address_bits_hex", "0x8000000000000000", "known", ""},
		{"zero", "source_sub_type_id=0", "resource.sub_type_id", "0", "known", ""},
		{"null", "source_sub_type_name=null", "resource.sub_type_name", "", "unavailable", "source_null"},
		{"empty", `source_sub_type_name=""`, "resource.sub_type_name", "", "known", ""},
		{"quoted", `source_sub_type_name="cache \u007c texture\nline"`, "resource.sub_type_name", "cache | texture\nline", "known", ""},
		{"absent", "source_sub_type_id=99", "resource.sub_type_name", "", "unavailable", "not_published"},
		{"bad_size", "source_heap_size=-1", "resource.size", "", "invalid", "invalid_source_value"},
		{"bad_integer", "source_sub_type_id=2.0", "resource.sub_type_id", "", "invalid", "invalid_source_value"},
		{"conflict", "source_addr_i64=-1 source_addr_bits_hex=0x0000000000000000", "resource.address_i64", "", "invalid", "address_representation_conflict"},
		{"null_conflict", "source_addr_i64=null source_addr_bits_hex=0x0000000000000000", "resource.address_bits_hex", "", "invalid", "address_representation_conflict"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event, ok := ParseLine(1, traceMarkTestLine("worker", 10, 1, "I|20|NativeHook:MmapEvent "+tc.metadata), nil)
			if !ok {
				t.Fatal("marker lost")
			}
			p := ProjectTraceEventSemantics(event)
			field := eventSemanticField(t, p, tc.key)
			if field.Status != tc.status || field.IssueReason != tc.reason || tc.status == "known" && (field.Value == nil || *field.Value != tc.value) {
				t.Fatalf("field: %+v", field)
			}
			if !types.ValidateTraceEventSemantics(p) {
				t.Fatal("invalid projection")
			}
			wire, _ := json.Marshal(event)
			var restored Event
			if json.Unmarshal(wire, &restored) != nil || !reflect.DeepEqual(p, ProjectTraceEventSemantics(restored)) {
				t.Fatal("cached event lost parser receipt")
			}
		})
	}
	for _, marker := range []string{
		"B|20|NativeHook:AllocEvent source_heap_size=9", "I|20|not NativeHook:AllocEvent source_heap_size=9",
		"I|20|NativeHook:AllocEvent source_heap_size=9 source_heap_size=1",
		`I|20|NativeHook:AllocEvent source_sub_type_name="broken`,
		"I|20|NativeHook:AllocEvent source_heap_size=9 extra=99",
	} {
		event, ok := ParseLine(1, traceMarkTestLine("worker", 10, 1, marker), nil)
		if !ok {
			t.Fatal("bad metadata erased original event")
		}
		if event.PluginFields != nil && event.PluginFields.NativeHookSemantics != nil {
			t.Fatal("wrong shape minted parsed metadata")
		}
	}
}

func TestNativeHookSemanticsPublicQueryAndBudget(t *testing.T) {
	label := strings.Repeat("buffer ", 100) + "end"
	encoded, _ := json.Marshal(label)
	path := writeTraceMarkIntegrityTrace(t, "resource.trace", traceMarkTestLine("worker", 10, 1, "I|20|NativeHook:NewResource source_addr_i64=-1 source_addr_bits_hex=0xffffffffffffffff source_sub_type_name="+string(encoded)))
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	q := Query{View: "event_search", TimeStart: .5, TimeEnd: 1.5, Limit: 10}
	a := Run(idx, q)
	b, err := StreamEventSearch(context.Background(), path, q)
	if err != nil || len(a.Events) != 1 || len(b.Events) != 1 {
		t.Fatalf("query failed: %v", err)
	}
	for _, r := range []Result{a, b} {
		p := ProjectTraceEventSemantics(r.Events[0].Event)
		expectEventSemanticValue(t, p, "resource.sub_type_name", label)
		expectEventSemanticValue(t, p, "resource.address_i64", "-1")
		if eventSemanticField(t, p, "resource.size").Unit != "" {
			t.Fatal("unknown resource unit became bytes")
		}
	}
	longName, _ := json.Marshal(strings.Repeat("x", 1200))
	event, _ := ParseLine(1, traceMarkTestLine("worker", 10, 1, "I|20|NativeHook:NewResource source_sub_type_name="+string(longName)), nil)
	p := ProjectTraceEventSemantics(event)
	if f := eventSemanticField(t, p, "resource.sub_type_name"); f.Status != "omitted" || f.Omitted == nil || f.Omitted.OriginalBytes != 1200 {
		t.Fatal("long identity silently shortened")
	}
}
