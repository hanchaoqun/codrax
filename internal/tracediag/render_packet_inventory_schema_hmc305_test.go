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

const packetInventorySchemaAddition = "PacketInventory|*tracewire.PacketInventoryResult|packet_inventory,omitempty"

// Invert only this reviewed optional addition. Historical witnesses still
// hash every prior field, type, tag and declaration position unchanged.
func resultSchemaBeforePacketInventory(t *testing.T, schema string) string {
	t.Helper()
	schema = resultSchemaBeforePacketFlows(t, schema)
	previous, err := stripPacketInventorySchemaAddition(schema)
	if err != nil {
		t.Fatal(err)
	}
	return previous
}

func stripPacketInventorySchemaAddition(schema string) (string, error) {
	var previous []string
	count := 0
	for _, field := range strings.Split(schema, ";") {
		if field == packetInventorySchemaAddition {
			count++
			continue
		}
		previous = append(previous, field)
	}
	if count != 1 {
		return "", fmt.Errorf("expected exactly one reviewed optional packet-inventory field, got %d", count)
	}
	return strings.Join(previous, ";"), nil
}

func TestHMC305PacketInventoryResultSchemaAddsOnlyReviewedOptionalField(t *testing.T) {
	typ := reflect.TypeOf(tracequery.Result{})
	_, schema := detailSchemaFingerprint(typ)
	beforeFlows := sha256.Sum256([]byte(resultSchemaBeforePacketFlows(t, schema)))
	current := hex.EncodeToString(beforeFlows[:])
	const currentFullResult = "068194140d8edb5f26727356721b8640eeb1f2a1cd2a2743f1b3ccedb194cd20"
	if current != currentFullResult {
		t.Fatalf("packet Result schema drift: got=%s want=%s\nschema=%s", current, currentFullResult, schema)
	}
	previous := resultSchemaBeforePacketInventory(t, schema)
	sum := sha256.Sum256([]byte(previous))
	const beforePacketInventory = "aefd0aaa1adde3b00685730bd355eb8f435e1b7d78d3ea416707a6bf542e0bd1"
	if got := hex.EncodeToString(sum[:]); got != beforePacketInventory {
		t.Fatalf("packet inventory changed an unrelated Result field: got=%s want=%s\nprevious=%s", got, beforePacketInventory, previous)
	}
	if policySkipsDetailField(&nonEventDetailPolicy, typ, "PacketInventory") || resultMetaFields["PacketInventory"] {
		t.Fatal("packet inventory must retain its dedicated detail surface")
	}
	for _, populated := range []bool{false, true} {
		var result tracequery.Result
		if populated {
			result.PacketInventory = &tracewire.PacketInventoryResult{Status: "complete", TimeDomain: "pcap_epoch_seconds"}
		}
		raw, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		if _, present := fields["packet_inventory"]; present != populated {
			t.Fatalf("packet optionality changed: populated=%t json=%s", populated, raw)
		}
	}
}

func TestHMC305PacketSchemaReverseIsExactAndCannotHideOldDrift(t *testing.T) {
	_, schema := detailSchemaFingerprint(reflect.TypeOf(tracequery.Result{}))
	for name, changed := range map[string]string{
		"missing":                      strings.Replace(schema, packetInventorySchemaAddition+";", "", 1),
		"duplicate":                    packetInventorySchemaAddition + ";" + schema,
		"wrong_type":                   strings.Replace(schema, "*tracewire.PacketInventoryResult", "*tracequery.Result", 1),
		"required_instead_of_optional": strings.Replace(schema, "packet_inventory,omitempty", "packet_inventory", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := stripPacketInventorySchemaAddition(changed); err == nil {
				t.Fatal("unreviewed optional field was stripped")
			}
		})
	}
	baseline := resultSchemaBeforePacketInventory(t, schema)
	for name, changed := range map[string]string{
		"old_type":             strings.Replace(schema, "View|string|view", "View|int|view", 1),
		"old_tag":              strings.Replace(schema, "SourcePath|string|source_path", "SourcePath|string|origin", 1),
		"old_order":            strings.Replace(schema, "View|string|view;SourcePath|string|source_path", "SourcePath|string|source_path;View|string|view", 1),
		"unreviewed_new_field": schema + ";Unreviewed|int|unreviewed,omitempty",
	} {
		t.Run(name, func(t *testing.T) {
			previous := resultSchemaBeforePacketInventory(t, changed)
			if previous == baseline || sha256.Sum256([]byte(previous)) == sha256.Sum256([]byte(baseline)) {
				t.Fatal("reverse transform concealed unrelated schema drift")
			}
		})
	}
}

// Each admitted leaf uses exact JSON presence, so later fields must receive
// the same explicit rendering review as the top-level optional face.
func TestHMC305PacketDetailSchemasAndDisposition(t *testing.T) {
	for _, tc := range []struct {
		typ  reflect.Type
		want string
	}{
		{reflect.TypeOf(tracewire.PacketInventoryResult{}), "View|string|view;SourcePath|string|source_path;Status|string|status;TimeDomain|string|time_domain;Header|tracewire.PacketHeader|header;SourceBytes|int64|source_bytes;PacketCount|uint64|packet_count;OmittedPackets|uint64|omitted_packets;CapturedBytes|uint64|captured_bytes;OriginalBytes|uint64|original_bytes;TruncatedPackets|uint64|truncated_packets;FirstTimestamp|*tracewire.PacketTimestamp|first_record_timestamp,omitempty;LastTimestamp|*tracewire.PacketTimestamp|last_record_timestamp,omitempty;MinTimestamp|*tracewire.PacketTimestamp|min_timestamp,omitempty;MaxTimestamp|*tracewire.PacketTimestamp|max_timestamp,omitempty;OutOfOrderPackets|uint64|out_of_order_packets;Protocols|[]tracewire.PacketCount|protocols;DecodeStatuses|[]tracewire.PacketCount|decode_statuses;Directions|[]tracewire.PacketCount|directions;Members|[]tracewire.PacketRecord|members;Failure|*tracewire.PacketFailure|failure,omitempty"},
		{reflect.TypeOf(tracewire.PacketHeader{}), "ByteOrder|string|byte_order;Resolution|string|timestamp_resolution;Snaplen|uint32|snaplen_bytes;LinkType|uint16|link_type;LinkTypeWord|uint32|link_type_word"},
		{reflect.TypeOf(tracewire.PacketTimestamp{}), "Seconds|uint32|seconds;Fraction|uint32|fraction;Decimal|string|epoch_seconds"},
		{reflect.TypeOf(tracewire.PacketRecord{}), "Ordinal|uint64|ordinal;FileOffset|int64|file_offset;Timestamp|tracewire.PacketTimestamp|timestamp;CapturedBytes|uint32|captured_bytes;OriginalBytes|uint32|original_bytes;Truncated|bool|capture_truncated;DecoderPrefixLimited|bool|decoder_prefix_limited;InterfaceIndex|*uint32|interface_index,omitempty;PacketType|*uint16|packet_type,omitempty;Direction|string|direction;LinkProtocol|*uint16|link_protocol,omitempty;IPProtocol|*uint8|ip_protocol,omitempty;Protocol|string|protocol;DecodeStatus|string|decode_status;SourceAddress|string|source_address,omitempty;DestinationAddress|string|destination_address,omitempty;SourcePort|*uint16|source_port,omitempty;DestinationPort|*uint16|destination_port,omitempty;Fragmented|bool|fragmented"},
		{reflect.TypeOf(tracewire.PacketCount{}), "Label|string|label;Count|uint64|count"},
		{reflect.TypeOf(tracewire.PacketFailure{}), "Code|string|code;Offset|int64|file_offset"},
	} {
		t.Run(tc.typ.Name(), func(t *testing.T) {
			_, schema := detailSchemaFingerprint(tc.typ)
			if schema != tc.want {
				t.Fatalf("unreviewed packet detail schema: got=%s want=%s", schema, tc.want)
			}
			for i := 0; i < tc.typ.NumField(); i++ {
				if policySkipsDetailField(&nonEventDetailPolicy, tc.typ, tc.typ.Field(i).Name) {
					t.Errorf("packet field silently hidden: %s.%s", tc.typ.Name(), tc.typ.Field(i).Name)
				}
			}
		})
	}
}
