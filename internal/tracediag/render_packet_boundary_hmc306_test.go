package tracediag

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/tracewire"
)

func TestHMC306PacketExactTypesAreBoundToSixReviewedResultPaths(t *testing.T) {
	root := reflect.TypeOf(tracequery.Result{}.PacketInventory).Elem()
	paths := []struct {
		field string
		kind  reflect.Kind
		want  reflect.Type
	}{
		{"", reflect.Struct, reflect.TypeOf(tracewire.PacketInventoryResult{})},
		{"Header", reflect.Struct, reflect.TypeOf(tracewire.PacketHeader{})},
		{"FirstTimestamp", reflect.Pointer, reflect.TypeOf(tracewire.PacketTimestamp{})},
		{"Members", reflect.Slice, reflect.TypeOf(tracewire.PacketRecord{})},
		{"Protocols", reflect.Slice, reflect.TypeOf(tracewire.PacketCount{})},
		{"Failure", reflect.Pointer, reflect.TypeOf(tracewire.PacketFailure{})},
	}
	wantTypes := map[reflect.Type]bool{}
	for _, path := range paths {
		got := root
		if path.field != "" {
			field, ok := root.FieldByName(path.field)
			if !ok || field.Type.Kind() != path.kind {
				t.Fatalf("reviewed Result.PacketInventory.%s shape changed", path.field)
			}
			got = field.Type
			if path.kind == reflect.Pointer || path.kind == reflect.Slice {
				got = got.Elem()
			}
		}
		if got != path.want || !packetInventoryExactDetailTypes[got] {
			t.Errorf("Result.PacketInventory.%s lost exact DTO binding: got=%v want=%v", path.field, got, path.want)
		}
		wantTypes[path.want] = true
	}
	if len(wantTypes) != 6 || !reflect.DeepEqual(packetInventoryExactDetailTypes, wantTypes) {
		t.Fatalf("packet exact policy is not the six reviewed types: got=%v want=%v", packetInventoryExactDetailTypes, wantTypes)
	}
}

func TestHMC306PacketExactPolicyDoesNotAdmitUnknownOrFutureDTO(t *testing.T) {
	// Identical underlying fields, naming and report paths do not confer the
	// native family's JSON-presence policy on a newly defined DTO.
	type futurePacketCount tracewire.PacketCount
	type futurePacketTimestamp tracewire.PacketTimestamp
	type futurePacketInventory tracewire.PacketInventoryResult
	for _, value := range []any{
		futurePacketCount{Label: "same protocol"},
		futurePacketTimestamp{Decimal: "0.000000007"},
		futurePacketInventory{Status: "future"},
		struct {
			Label string `json:"label"`
			Count uint64 `json:"count"`
		}{Label: "same shape"},
	} {
		if packetInventoryExactDetailTypes[reflect.TypeOf(value)] {
			t.Fatalf("unreviewed DTO was granted exact policy: %T", value)
		}
	}
	for _, tc := range []struct {
		value any
		path  string
		zero  string
	}{
		{futurePacketCount{Label: "same protocol"}, "packet_inventory.protocols[0]", "count=0"},
		{futurePacketTimestamp{Decimal: "0.000000007"}, "packet_inventory.first_record_timestamp", "seconds=0"},
	} {
		var lines []string
		walkDetail(reflect.ValueOf(tc.value), tc.path, func(line string) { lines = append(lines, line) }, 0)
		if len(lines) == 0 || strings.Contains(strings.Join(lines, "\n"), " "+tc.zero) {
			t.Fatalf("unknown DTO inherited exact native rendering from its path: %T %v", tc.value, lines)
		}
	}
	var native []string
	walkDetail(reflect.ValueOf(tracewire.PacketCount{Label: "native"}), "unrelated display path", func(line string) { native = append(native, line) }, 0)
	if !strings.Contains(strings.Join(native, "\n"), "count=0") {
		t.Fatal("exact native type lost known zero because its display path changed", native)
	}
}
