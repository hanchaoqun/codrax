package tracequery

import (
	"strings"
	"testing"
	"unsafe"
)

func TestFrameMapRelationWireRoundTrip(t *testing.T) {
	relation := FrameMapRelation{
		TimestampNS:       2_000_000,
		RelationID:        9,
		SourceRow:         77,
		DestinationRow:    78,
		SourceTimestampNS: 1_500_000,
	}
	line, err := FormatFrameMapRelation(relation)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(line, "[000]") || !strings.HasPrefix(line, frameMapRelationPrefix+" ") {
		t.Fatalf("frame relation fabricated a physical envelope: %q", line)
	}
	if ts, ok := ParseLineTimestampNS(line); !ok || ts != relation.TimestampNS {
		t.Fatalf("frame relation timestamp=(%d,%t), want (%d,true)", ts, ok, relation.TimestampNS)
	}
	event, ok := ParseLine(7, line, newStringInterner())
	if !ok || event.Type != EventFrameMap || event.CPU != -1 || event.Line != 7 ||
		event.Name != "codrax_frame_map" || event.PluginFields == nil ||
		event.PluginFields.FrameMap == nil ||
		event.PluginFields.FrameMap.RelationID != relation.RelationID ||
		event.PluginFields.FrameMap.SourceRow != relation.SourceRow ||
		event.PluginFields.FrameMap.DestinationRow != relation.DestinationRow ||
		event.PluginFields.FrameMap.SourceTimestampNS != relation.SourceTimestampNS ||
		event.PluginFields.FrameMap.DestinationTimestampNS != relation.TimestampNS {
		t.Fatalf("typed frame relation round-trip drifted: %+v", event)
	}
	found := EventSearch(&Index{Events: []Event{event}}, Query{
		EventTypes: []EventType{EventFrameMap}, Limit: 4,
	})
	if len(found) != 1 || found[0].PluginFields == nil || found[0].PluginFields.FrameMap == nil {
		t.Fatalf("frame relation unavailable to event search: %+v", found)
	}
	wantSideBytes := int64(unsafe.Sizeof(PluginFields{})) + int64(unsafe.Sizeof(FrameMapFields{}))
	if got := eventSideTableBytes(&event); got != wantSideBytes {
		t.Fatalf("frame relation side-table bytes=%d, want %d", got, wantSideBytes)
	}
}

func TestFrameMapRelationClosedWire(t *testing.T) {
	base, err := FormatFrameMapRelation(FrameMapRelation{
		TimestampNS: 2, RelationID: 0, SourceRow: 0, DestinationRow: ^uint32(0), SourceTimestampNS: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{
		strings.Replace(base, "dst_row=4294967295", "dst_row=4294967296", 1),
		strings.Replace(base, "dst_row=4294967295", "dst_row=0", 1),
		strings.Replace(base, "relation_id=0", "relation_id=-1", 1),
		strings.Replace(base, "src_ts_ns=1", "src_ts_ns=1.0", 1),
		strings.Replace(base, " codrax_frame_map/v1 ", " codrax_frame_map/v2 ", 1),
		base + " extra=x",
	} {
		if _, ok := ParseLine(1, line, newStringInterner()); ok {
			t.Fatalf("accepted non-canonical frame relation: %q", line)
		}
	}
}
