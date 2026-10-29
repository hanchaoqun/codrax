package tracediag

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/tracewire"
)

const packetFlowsSchemaAddition = "PacketFlows|*tracewire.PacketFlowsResult|packet_flows,omitempty"

// Remove exactly the reviewed optional addition. Historical types, tags and
// declaration order are still checked by every prior complete fingerprint.
func resultSchemaBeforePacketFlows(t *testing.T, schema string) string {
	t.Helper()
	previous, err := stripPacketFlowsSchemaAddition(schema)
	if err != nil {
		t.Fatal(err)
	}
	return previous
}

func stripPacketFlowsSchemaAddition(schema string) (string, error) {
	var previous []string
	count := 0
	for _, field := range strings.Split(schema, ";") {
		if field == packetFlowsSchemaAddition {
			count++
			continue
		}
		previous = append(previous, field)
	}
	if count != 1 {
		return "", fmt.Errorf("expected exactly one reviewed optional packet-flows field, got %d", count)
	}
	return strings.Join(previous, ";"), nil
}

func TestHMC306PacketFlowsResultSchemaAddsOnlyReviewedOptionalField(t *testing.T) {
	typ := reflect.TypeOf(tracequery.Result{})
	current, schema := detailSchemaFingerprint(typ)
	const currentFullResult = "8a6e67555e8d211b42a85a74170fbb011a02b5d08e8472440d0d413e00abd6bb"
	if current != currentFullResult {
		t.Fatalf("packet-flows Result schema drift: got=%s want=%s\nschema=%s", current, currentFullResult, schema)
	}
	previous := resultSchemaBeforePacketFlows(t, schema)
	sum := sha256.Sum256([]byte(previous))
	const previousFullResult = "068194140d8edb5f26727356721b8640eeb1f2a1cd2a2743f1b3ccedb194cd20"
	if got := hex.EncodeToString(sum[:]); got != previousFullResult {
		t.Fatalf("packet flows changed another Result field: got=%s want=%s\nprevious=%s", got, previousFullResult, previous)
	}
	if schema != packetFlowsSchemaAddition+";"+previous {
		t.Fatal("reviewed packet-flows field declaration position changed")
	}
	if policySkipsDetailField(&nonEventDetailPolicy, typ, "PacketFlows") || resultMetaFields["PacketFlows"] {
		t.Fatal("packet flows must retain their independent detail surface")
	}
	t.Logf("current_full_result=%s previous_full_result=%s", current, previousFullResult)
	for _, present := range []bool{false, true} {
		var result tracequery.Result
		if present {
			result.PacketFlows = &tracewire.PacketFlowsResult{Status: "partial"}
		}
		raw, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		if _, found := fields["packet_flows"]; found != present {
			t.Fatal("packet flows lost optionality", string(raw))
		}
	}
}

func TestHMC306PacketFlowDTOExactSchemaAndDisposition(t *testing.T) {
	for _, tc := range []struct {
		typ  reflect.Type
		want string
	}{
		{reflect.TypeOf(tracewire.PacketFlowsResult{}), "View|string|view;SourcePath|string|source_path;Status|string|status;TimeDomain|string|time_domain;Header|tracewire.PacketHeader|header;SourceBytes|int64|source_bytes;PacketCount|uint64|packet_count;CapturedBytes|uint64|captured_bytes;OriginalBytes|uint64|original_bytes;TruncatedPackets|uint64|truncated_packets;GroupedPackets|uint64|grouped_packets;UngroupedPackets|uint64|ungrouped_packets;FlowCount|*uint64|flow_count,omitempty;TrackedFlowCount|uint64|tracked_flow_count;OmittedTrackedFlows|uint64|omitted_tracked_flows;GroupLimit|int|group_limit;GroupBudgetExceeded|bool|group_budget_exceeded;FirstTimestamp|*tracewire.PacketTimestamp|first_record_timestamp,omitempty;LastTimestamp|*tracewire.PacketTimestamp|last_record_timestamp,omitempty;MinTimestamp|*tracewire.PacketTimestamp|min_timestamp,omitempty;MaxTimestamp|*tracewire.PacketTimestamp|max_timestamp,omitempty;OutOfOrderPackets|uint64|out_of_order_packets;Members|[]tracewire.PacketFlow|members;Ungrouped|[]tracewire.PacketFlowUngrouped|ungrouped;Failure|*tracewire.PacketFailure|failure,omitempty"},
		{reflect.TypeOf(tracewire.PacketFlow{}), "ID|string|id;LinkType|uint16|link_type;InterfaceIndex|*uint32|interface_index,omitempty;VLANs|[]tracewire.PacketFlowVLAN|vlans;IPVersion|uint8|ip_version;IPProtocol|uint8|ip_protocol;Protocol|string|protocol;EndpointA|tracewire.PacketFlowEndpoint|endpoint_a;EndpointB|tracewire.PacketFlowEndpoint|endpoint_b;Stats|tracewire.PacketFlowStats|stats;AToB|tracewire.PacketFlowStats|a_to_b;BToA|tracewire.PacketFlowStats|b_to_a;SameEndpoint|tracewire.PacketFlowStats|same_endpoint;Directions|[]tracewire.PacketCount|directions"},
		{reflect.TypeOf(tracewire.PacketFlowStats{}), "Packets|uint64|packets;CapturedBytes|uint64|captured_bytes;OriginalBytes|uint64|original_bytes;TruncatedPackets|uint64|truncated_packets;DecoderPrefixLimitedPackets|uint64|decoder_prefix_limited_packets;MinTimestamp|*tracewire.PacketTimestamp|min_timestamp,omitempty;MaxTimestamp|*tracewire.PacketTimestamp|max_timestamp,omitempty"},
		{reflect.TypeOf(tracewire.PacketFlowEndpoint{}), "Address|string|address;Port|uint16|port"},
		{reflect.TypeOf(tracewire.PacketFlowVLAN{}), "TPID|uint16|tpid;ID|uint16|id"},
		{reflect.TypeOf(tracewire.PacketFlowUngrouped{}), "Reason|string|reason;Stats|tracewire.PacketFlowStats|stats"},
	} {
		t.Run(tc.typ.Name(), func(t *testing.T) {
			_, schema := detailSchemaFingerprint(tc.typ)
			if schema != tc.want {
				t.Fatalf("unreviewed flow DTO schema: got=%s want=%s", schema, tc.want)
			}
			if !packetFlowsExactDetailTypes[tc.typ] {
				t.Fatal("native DTO is not admitted by its reviewed result field path")
			}
			for i := 0; i < tc.typ.NumField(); i++ {
				if policySkipsDetailField(&nonEventDetailPolicy, tc.typ, tc.typ.Field(i).Name) {
					t.Errorf("flow field silently skipped: %s", tc.typ.Field(i).Name)
				}
			}
		})
	}
}

func TestHMC306PacketFlowDTOFieldPathsAreClosed(t *testing.T) {
	root := reflect.TypeOf(tracequery.Result{}.PacketFlows).Elem()
	paths := []struct {
		names []string
		want  reflect.Type
	}{
		{nil, reflect.TypeOf(tracewire.PacketFlowsResult{})},
		{[]string{"Members"}, reflect.TypeOf(tracewire.PacketFlow{})},
		{[]string{"Ungrouped"}, reflect.TypeOf(tracewire.PacketFlowUngrouped{})},
		{[]string{"Members", "Stats"}, reflect.TypeOf(tracewire.PacketFlowStats{})},
		{[]string{"Members", "EndpointA"}, reflect.TypeOf(tracewire.PacketFlowEndpoint{})},
		{[]string{"Members", "VLANs"}, reflect.TypeOf(tracewire.PacketFlowVLAN{})},
	}
	want := map[reflect.Type]bool{}
	for _, path := range paths {
		typ := root
		for _, name := range path.names {
			field, found := typ.FieldByName(name)
			if !found {
				t.Fatalf("reviewed path missing: %v", path.names)
			}
			typ = field.Type
			if typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice {
				typ = typ.Elem()
			}
		}
		if typ != path.want {
			t.Errorf("reviewed path changed type: %v got=%v want=%v", path.names, typ, path.want)
		}
		want[path.want] = true
	}
	if len(want) != 6 || !reflect.DeepEqual(packetFlowsExactDetailTypes, want) {
		t.Fatal("flow DTO binding escaped the six reviewed types")
	}
	type futureStats tracewire.PacketFlowStats
	type futureFlow tracewire.PacketFlow
	for _, value := range []any{futureStats{}, futureFlow{}, struct {
		Packets uint64 `json:"packets"`
	}{}} {
		typ := reflect.TypeOf(value)
		if packetFlowsExactDetailTypes[typ] || packetInventoryExactDetailTypes[typ] {
			t.Fatalf("future/shape-compatible DTO granted native policy: %T", value)
		}
	}
}

func TestHMC306PacketFlowsSchemaReverseIsExactAndPreservesOldDrift(t *testing.T) {
	_, schema := detailSchemaFingerprint(reflect.TypeOf(tracequery.Result{}))
	for name, changed := range map[string]string{
		"missing":    strings.Replace(schema, packetFlowsSchemaAddition+";", "", 1),
		"duplicate":  packetFlowsSchemaAddition + ";" + schema,
		"wrong_type": strings.Replace(schema, "*tracewire.PacketFlowsResult", "*tracequery.Result", 1),
		"wrong_tag":  strings.Replace(schema, "packet_flows,omitempty", "packet_flows_wrong,omitempty", 1),
		"required":   strings.Replace(schema, "packet_flows,omitempty", "packet_flows", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := stripPacketFlowsSchemaAddition(changed); err == nil {
				t.Fatal("unreviewed addition was silently stripped")
			}
		})
	}
	baseline := resultSchemaBeforePacketFlows(t, schema)
	for name, changed := range map[string]string{
		"old_type":         strings.Replace(schema, "View|string|view", "View|int|view", 1),
		"old_tag":          strings.Replace(schema, "SourcePath|string|source_path", "SourcePath|string|origin", 1),
		"old_order":        strings.Replace(schema, "View|string|view;SourcePath|string|source_path", "SourcePath|string|source_path;View|string|view", 1),
		"unknown_addition": schema + ";Unreviewed|int|unreviewed,omitempty",
	} {
		t.Run(name, func(t *testing.T) {
			previous := resultSchemaBeforePacketFlows(t, changed)
			if previous == baseline || sha256.Sum256([]byte(previous)) == sha256.Sum256([]byte(baseline)) {
				t.Fatal("reverse transform concealed unrelated schema drift")
			}
		})
	}
}
