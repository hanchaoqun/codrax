package pcapinput

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net/netip"
	"os"
	"reflect"
	"runtime"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

func TestHMC306FlowPublicEntrySeparatesCompleteTotalsFromPreview(t *testing.T) {
	a := ipv4UDP()
	b := append([]byte(nil), a...)
	copy(b[12:16], a[16:20])
	copy(b[16:20], a[12:16])
	copy(b[20:22], a[22:24])
	copy(b[22:24], a[20:22])
	c := append([]byte(nil), a...)
	binary.BigEndian.PutUint16(c[20:22], 999)
	path := putCapture(t, captureBytes(binary.LittleEndian, false, 101, 65535, a, b, a, c))
	got, err := StreamFlows(t.Context(), path, 1)
	if err != nil || got.Status != "complete" || got.PacketCount != 4 || got.FlowCount == nil || *got.FlowCount != 2 || got.GroupedPackets != 4 || got.UngroupedPackets != 0 || got.OmittedTrackedFlows != 1 || len(got.Members) != 1 {
		t.Fatalf("complete flow census or preview changed: %+v err=%v", got, err)
	}
	f := got.Members[0]
	if f.AToB.Packets != 2 || f.BToA.Packets != 1 || f.SameEndpoint.Packets != 0 || f.Stats.Packets != 3 || f.Stats.CapturedBytes != 84 || f.Protocol != "UDP" || f.IPVersion != 4 || f.InterfaceIndex != nil {
		t.Fatalf("bidirectional identity/measure mismatch: %+v", f)
	}
	full, err := StreamFlows(t.Context(), path, 64)
	if err != nil || full.Members[0].ID != f.ID || full.CapturedBytes != got.CapturedBytes || full.FlowCount == nil || *full.FlowCount != *got.FlowCount || got.ValidateCurrentSource() != nil {
		t.Fatalf("preview granted a different source/flow census: %+v %v", full, err)
	}
}

func hmc306Reverse(packet []byte, version int) []byte {
	b := append([]byte(nil), packet...)
	start, width, transport := 12, 4, int(packet[0]&15)*4
	if version == 6 {
		start, width, transport = 8, 16, 40
	}
	copy(b[start:start+width], packet[start+width:start+2*width])
	copy(b[start+width:start+2*width], packet[start:start+width])
	copy(b[transport:transport+2], packet[transport+2:transport+4])
	copy(b[transport+2:transport+4], packet[transport:transport+2])
	return b
}

func hmc306IPv6(protocol byte, source, target string, sourcePort, targetPort uint16) []byte {
	length := 8
	if protocol == 6 {
		length = 20
	}
	b := make([]byte, 40+length)
	b[0], b[6] = 0x60, protocol
	binary.BigEndian.PutUint16(b[4:6], uint16(length))
	s, d := netip.MustParseAddr(source).As16(), netip.MustParseAddr(target).As16()
	copy(b[8:24], s[:])
	copy(b[24:40], d[:])
	binary.BigEndian.PutUint16(b[40:42], sourcePort)
	binary.BigEndian.PutUint16(b[42:44], targetPort)
	if protocol == 6 {
		b[52] = 5 << 4
	} else {
		binary.BigEndian.PutUint16(b[44:46], uint16(length))
	}
	return b
}

func hmc306Cooked(ip []byte, iface uint32, packetType byte, tags ...uint16) []byte {
	b := make([]byte, 20)
	binary.BigEndian.PutUint16(b[0:2], 0x800)
	binary.BigEndian.PutUint32(b[4:8], iface)
	b[10] = packetType
	if len(tags) > 0 {
		binary.BigEndian.PutUint16(b[0:2], 0x8100)
		for i, tag := range tags {
			protocol := uint16(0x8100)
			if i == len(tags)-1 {
				protocol = 0x800
			}
			b = binary.BigEndian.AppendUint16(b, tag)
			b = binary.BigEndian.AppendUint16(b, protocol)
		}
	}
	return append(b, ip...)
}

func TestHMC306FlowPublicVLANInterfaceAndDirectionIsolation(t *testing.T) {
	ip := ipv4UDP()
	packets := [][]byte{
		hmc306Cooked(ip, 0, 4), hmc306Cooked(hmc306Reverse(ip, 4), 0, 0),
		hmc306Cooked(ip, 7, 255), hmc306Cooked(ip, 0, 4, 0),
		hmc306Cooked(ip, 0, 4, 9), hmc306Cooked(ip, 0, 4, 0xa009),
		hmc306Cooked(ip, 0, 4, 9, 10), hmc306Cooked(ip, 0, 4, 10, 9),
	}
	got, err := StreamFlows(t.Context(), putCapture(t, captureBytes(binary.LittleEndian, false, 276, 65535, packets...)), 64)
	if err != nil || got.FlowCount == nil || *got.FlowCount != 6 || got.GroupedPackets != 8 {
		t.Fatalf("interface/VLAN identity collapsed: %+v %v", got, err)
	}
	var foundUntagged, foundPriority bool
	for _, f := range got.Members {
		if f.InterfaceIndex == nil {
			t.Fatal("observed interface including zero became unknown")
		}
		if *f.InterfaceIndex == 0 && len(f.VLANs) == 0 {
			foundUntagged = true
			if f.AToB.Packets != 1 || f.BToA.Packets != 1 || !reflect.DeepEqual(f.Directions, []Count{{"incoming_host", 1}, {"outgoing", 1}}) {
				t.Fatalf("A/B direction confused with cooked type: %+v", f)
			}
		}
		if len(f.VLANs) == 1 && f.VLANs[0].ID == 9 {
			foundPriority = true
			if f.Stats.Packets != 2 || f.VLANs[0].TPID != 0x8100 {
				t.Fatalf("VLAN priority minted another observed network: %+v", f)
			}
		}
		if *f.InterfaceIndex == 7 && (len(f.Directions) != 1 || f.Directions[0].Label != "unknown") {
			t.Fatalf("unknown cooked direction invented: %+v", f)
		}
	}
	if !foundUntagged || !foundPriority {
		t.Fatal("expected VLAN groups absent")
	}
	// Unknown and a genuine zero interface are different keys, even though
	// classic PCAP cannot switch its link type between individual records.
	p := Packet{IPProtocol: new(uint8), SourcePort: new(uint16), DestinationPort: new(uint16), DecodeStatus: "transport_header_decoded"}
	*p.IPProtocol = 17
	d := packetDomain{link: 276, ipVersion: 4, source: netip.MustParseAddr("192.0.2.1"), target: netip.MustParseAddr("192.0.2.2")}
	unknown, _, _ := packetFlowKey(p, d)
	p.InterfaceIndex = new(uint32)
	zero, _, _ := packetFlowKey(p, d)
	if unknown == zero {
		t.Fatal("missing interface treated as zero")
	}
}

func TestHMC306FlowPublicIPv6MappedAddressesProtocolAndZeroPorts(t *testing.T) {
	v6 := hmc306IPv6(17, "2001:db8::1", "2001:db8::2", 0, 443)
	tcp := hmc306IPv6(6, "2001:db8::1", "2001:db8::2", 0, 443)
	mapped := hmc306IPv6(17, "::ffff:192.0.2.1", "::ffff:192.0.2.2", 1234, 4321)
	self := hmc306IPv6(17, "2001:db8::1", "2001:db8::1", 0, 0)
	packets := [][]byte{v6, hmc306Reverse(v6, 6), tcp, mapped, ipv4UDP(), self}
	got, err := StreamFlows(t.Context(), putCapture(t, captureBytes(binary.BigEndian, true, 101, 65535, packets...)), 64)
	if err != nil || got.FlowCount == nil || *got.FlowCount != 5 || got.GroupedPackets != 6 || got.Header.Resolution != "nanosecond" {
		t.Fatalf("IPv4/IPv6/protocol aliasing: %+v %v", got, err)
	}
	var seenZero, seenMapped, seenSelf bool
	for _, f := range got.Members {
		if f.EndpointA.Address == "::ffff:192.0.2.1" {
			seenMapped = f.IPVersion == 6
		}
		if f.EndpointA.Port == 0 && f.EndpointB.Port == 443 && f.Protocol == "UDP" {
			seenZero = true
			if f.AToB.Packets != 1 || f.BToA.Packets != 1 || f.AToB.MinTimestamp.Decimal != "1789020987.000035118" {
				t.Fatalf("port zero/nanosecond/reciprocal stats lost: %+v", f)
			}
		}
		if f.EndpointA == f.EndpointB {
			seenSelf = true
			if f.SameEndpoint.Packets != 1 || f.AToB.Packets != 0 || f.BToA.Packets != 0 || f.AToB.MinTimestamp != nil || f.BToA.MaxTimestamp != nil {
				t.Fatalf("same endpoint falsely assigned a direction: %+v", f)
			}
		}
	}
	if !seenZero || !seenMapped || !seenSelf {
		t.Fatal("required endpoint kind lost")
	}
}

func TestHMC306FlowPublicUnknownFragmentAndCaptureTruncation(t *testing.T) {
	fragment := ipv4UDP()
	binary.BigEndian.PutUint16(fragment[6:8], 0x2000)
	icmp := ipv4UDP()
	icmp[9] = 1
	v6fragment := hmc306IPv6(17, "2001:db8::1", "2001:db8::2", 1, 2)
	v6fragment[6], v6fragment[40] = 44, 17
	packets := [][]byte{ipv4UDP(), fragment, ipv4UDP()[:22], icmp, {0xff}, v6fragment}
	b := captureBytes(binary.LittleEndian, false, 101, 65535, packets...)
	binary.LittleEndian.PutUint32(b[36:40], 100)
	got, err := StreamFlows(t.Context(), putCapture(t, b), 64)
	if err != nil || got.Status != "complete" || got.FlowCount == nil || *got.FlowCount != 1 || got.GroupedPackets != 1 || got.UngroupedPackets != 5 || got.PacketCount != 6 || got.TruncatedPackets != 1 {
		t.Fatalf("unresolved records omitted or falsely grouped: %+v %v", got, err)
	}
	if got.Members[0].Stats.CapturedBytes != 28 || got.Members[0].Stats.OriginalBytes != 100 || got.Members[0].Stats.TruncatedPackets != 1 {
		t.Fatal("capture truncation dropped a known endpoint or lost length caliber")
	}
	reasons := map[string]uint64{}
	var excludedBytes uint64
	for _, u := range got.Ungrouped {
		reasons[u.Reason] = u.Stats.Packets
		excludedBytes += u.Stats.CapturedBytes
	}
	if !reflect.DeepEqual(reasons, map[string]uint64{"fragment_transport_unresolved": 2, "transport_identity_unresolved": 1, "unsupported_transport_protocol": 1, "network_identity_unresolved": 1}) || excludedBytes+28 != got.CapturedBytes {
		t.Fatalf("reason buckets do not partition source records: %+v", got)
	}
}

func TestHMC306FlowPublicBudgetDoesNotDependOnPreview(t *testing.T) {
	var packets [][]byte
	for i := 0; i <= MaxFlowGroups; i++ {
		p := ipv4UDP()
		binary.BigEndian.PutUint16(p[20:22], uint16(i))
		packets = append(packets, p)
	}
	packets = append(packets, packets[0], packets[MaxFlowGroups])
	path := putCapture(t, captureBytes(binary.LittleEndian, false, 101, 65535, packets...))
	for _, limit := range []int{1, MaxLimit} {
		got, err := StreamFlows(t.Context(), path, limit)
		if err != nil || got.Status != "partial" || !got.GroupBudgetExceeded || got.FlowCount != nil || got.TrackedFlowCount != MaxFlowGroups || got.PacketCount != MaxFlowGroups+3 || got.GroupedPackets != MaxFlowGroups+1 || got.UngroupedPackets != 2 || got.OmittedTrackedFlows != uint64(MaxFlowGroups-limit) || len(got.Members) != limit || got.Failure == nil || got.Failure.Code != "pcap_flow_group_budget_exceeded" {
			t.Fatalf("budget manufactured complete TopN: %+v %v", got, err)
		}
		if got.Members[0].Stats.Packets != 2 || len(got.Ungrouped) != 1 || got.Ungrouped[0].Reason != "flow_group_budget_exceeded" || got.Ungrouped[0].Stats.Packets != 2 {
			t.Fatal("budget stopped tracking existing exact groups or lost exclusions")
		}
	}
}

func TestHMC306FlowPublicEmptyPartialCancelAndSourceGeneration(t *testing.T) {
	empty, err := StreamFlows(t.Context(), putCapture(t, captureBytes(binary.LittleEndian, false, 101, 65535)), 0)
	if err != nil || empty.Status != "complete" || empty.FlowCount == nil || *empty.FlowCount != 0 || empty.MinTimestamp != nil || empty.GroupBudgetExceeded {
		t.Fatalf("empty and unknown conflated: %+v %v", empty, err)
	}
	b := captureBytes(binary.LittleEndian, false, 101, 65535, ipv4UDP())
	partial, err := StreamFlows(t.Context(), putCapture(t, append(append([]byte(nil), b...), 1)), 1)
	if err != nil || partial.Status != "partial" || partial.FlowCount != nil || partial.PacketCount != 1 || partial.GroupBudgetExceeded || partial.Failure == nil || partial.Failure.Code != "pcap_short_record_header" {
		t.Fatalf("partial prefix promoted to full capture: %+v %v", partial, err)
	}
	path := putCapture(t, b)
	live, err := StreamFlows(t.Context(), path, 1)
	if err != nil || live.ValidateCurrentSource() != nil {
		t.Fatal(err)
	}
	copyResult, err := StreamFlows(t.Context(), putCapture(t, b), 1)
	if err != nil || copyResult.Members[0].ID == live.Members[0].ID {
		t.Fatal("same-byte sources share flow identity")
	}
	raw, err := json.Marshal(live.PacketFlowsResult)
	if err != nil {
		t.Fatal(err)
	}
	var restored FlowResult
	if json.Unmarshal(raw, &restored) != nil || restored.ValidateCurrentSource() == nil {
		t.Fatal("wire JSON reconstructed live source authority")
	}
	replacement := putCapture(t, b)
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	if live.ValidateCurrentSource() == nil {
		t.Fatal("replacement did not invalidate live source receipt")
	}
	current, err := StreamFlows(t.Context(), path, 1)
	if err != nil || current.Members[0].ID == live.Members[0].ID {
		t.Fatal("replaced generation reused old flow identity")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := StreamFlows(ctx, path, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled scan=%v", err)
	}
	if _, err := StreamFlows(t.Context(), putCapture(t, []byte{0x0a, 0x0d, 0x0d, 0x0a}), 1); err == nil {
		t.Fatal("unsupported PCAPNG turned into zero flows")
	}
}

func TestHMC306FlowPublicRankUsesFrameBytesAndKeepsExactEpoch(t *testing.T) {
	a := ipv4UDP()
	b := append(ipv4UDP(), make([]byte, 200)...)
	binary.BigEndian.PutUint16(b[20:22], 999)
	data := captureBytes(binary.LittleEndian, false, 101, 65535, a, a, b, b)
	// A later file record has an earlier native epoch fraction. Identity and
	// first/last positions are independent of the observed min/max times.
	lastOffset := 24 + 16 + len(a) + 16 + len(a) + 16 + len(b)
	binary.LittleEndian.PutUint32(data[lastOffset+4:lastOffset+8], 1)
	path := putCapture(t, data)
	got, err := StreamFlows(t.Context(), path, 1)
	if err != nil || got.Members[0].EndpointA.Port != 999 || got.Members[0].Stats.CapturedBytes != 456 || got.Members[0].Stats.MinTimestamp.Decimal != "1789020987.000001" || got.OutOfOrderPackets != 1 || got.MinTimestamp.Decimal != "1789020987.000001" || got.FirstTimestamp.Decimal != "1789020987.035118" {
		t.Fatalf("rank or epoch changed: %+v %v", got, err)
	}
	again, err := StreamFlows(t.Context(), path, 1)
	if err != nil || !reflect.DeepEqual(got.PacketFlowsResult, again.PacketFlowsResult) {
		t.Fatal("same-generation flow result is nondeterministic")
	}
}

func TestHMC306FlowPublicLargeCaplenKeepsBoundedPrefix(t *testing.T) {
	const size = uint32(1 << 30)
	b := captureBytes(binary.LittleEndian, false, 101, size, ipv4UDP())
	binary.LittleEndian.PutUint32(b[32:36], size)
	binary.LittleEndian.PutUint32(b[36:40], size)
	path := putCapture(t, b)
	if err := os.Truncate(path, 40+int64(size)); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	got, err := StreamFlows(t.Context(), path, 1)
	runtime.ReadMemStats(&after)
	allocated := after.TotalAlloc - before.TotalAlloc
	t.Logf("logical_input_bytes=%d prefix_bytes=%d allocation_bytes=%d", 40+int64(size), PacketPrefixBytes, allocated)
	if err != nil || got.PacketCount != 1 || got.GroupedPackets != 1 || got.Members[0].Stats.CapturedBytes != uint64(size) || got.Members[0].Stats.DecoderPrefixLimitedPackets != 1 || allocated > 32<<20 {
		t.Fatalf("unbounded allocation or frame/payload conflation: %+v err=%v allocated=%d", got, err, allocated)
	}
}

func TestHMC306FlowStatisticOverflowIsNotWrapped(t *testing.T) {
	s := tracewire.PacketFlowStats{OriginalBytes: ^uint64(0)}
	if err := addFlowStats(&s, Packet{OriginalBytes: 1}); err == nil || s.Packets != 0 || s.OriginalBytes != ^uint64(0) {
		t.Fatalf("overflow accepted/mutated accumulator: %+v %v", s, err)
	}
}
