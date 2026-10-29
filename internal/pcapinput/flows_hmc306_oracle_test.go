package pcapinput

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

// This opt-in acceptance test consumes an independently produced byte-walk
// oracle, never a production query result. The portable adversarial fixtures in
// flows_hmc306_test.go remain mandatory when this external capture is absent.
func TestHMC306FlowRealCaptureIndependentByteOracle(t *testing.T) {
	path := os.Getenv("CODRAX_PCAP_FLOW_ORACLE")
	if path == "" {
		t.Skip("set CODRAX_PCAP_FLOW_ORACLE to the independent real-capture byte oracle")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var oracle struct {
		Source           string                       `json:"source"`
		SourceSHA256     string                       `json:"source_sha256"`
		SourceBytes      int64                        `json:"source_bytes"`
		PacketCount      uint64                       `json:"complete_record_count"`
		Global           hmc306OracleStats            `json:"global"`
		GroupedPackets   uint64                       `json:"grouped_packet_count"`
		UngroupedPackets uint64                       `json:"ungrouped_packet_count"`
		FlowCount        uint64                       `json:"flow_count"`
		Members          []hmc306OracleFlow           `json:"members"`
		Ungrouped        map[string]hmc306OracleStats `json:"ungrouped"`
	}
	if err := json.Unmarshal(raw, &oracle); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(oracle.Source)
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(source)) != oracle.SourceBytes || fmt.Sprintf("%x", sha256.Sum256(source)) != oracle.SourceSHA256 {
		t.Fatal("oracle belongs to a different source generation")
	}
	if oracle.FlowCount != uint64(len(oracle.Members)) || oracle.PacketCount != oracle.Global.Packets || oracle.GroupedPackets+oracle.UngroupedPackets != oracle.PacketCount {
		t.Fatal("independent oracle census is internally inconsistent")
	}
	want := make(map[string]hmc306OracleFlow, len(oracle.Members))
	var rankedBytes []uint64
	for _, member := range oracle.Members {
		if member.Identity.SourceSHA256 != oracle.SourceSHA256 {
			t.Fatal("member identity belongs to another source")
		}
		key := member.Identity.key()
		if _, exists := want[key]; exists {
			t.Fatal("independent oracle duplicates a complete flow identity")
		}
		want[key] = member
		rankedBytes = append(rankedBytes, member.Stats.CapturedBytes)
	}
	sort.Slice(rankedBytes, func(i, j int) bool { return rankedBytes[i] > rankedBytes[j] })
	checkRow := func(t *testing.T, row tracewire.PacketFlow) {
		t.Helper()
		key := hmc306OracleIdentityFor(row).key()
		member, exists := want[key]
		if !exists {
			t.Fatalf("unobserved flow identity: %s", key)
		}
		if row.Protocol != map[uint8]string{6: "TCP", 17: "UDP"}[row.IPProtocol] {
			t.Fatal("transport label differs from the native protocol number")
		}
		hmc306CheckOracleStats(t, "total", row.Stats, member.Stats)
		hmc306CheckOracleStats(t, "A to B", row.AToB, member.AToB)
		hmc306CheckOracleStats(t, "B to A", row.BToA, member.BToA)
		hmc306CheckOracleStats(t, "same endpoint", row.SameEndpoint, member.SameEndpoint)
	}
	t.Run("public_preview_and_complete_census", func(t *testing.T) {
		got, err := StreamFlows(t.Context(), oracle.Source, MaxLimit)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != "complete" || got.Failure != nil || got.GroupBudgetExceeded || got.FlowCount == nil || *got.FlowCount != oracle.FlowCount || got.TrackedFlowCount != oracle.FlowCount || got.PacketCount != oracle.PacketCount || got.GroupedPackets != oracle.GroupedPackets || got.UngroupedPackets != oracle.UngroupedPackets || got.SourceBytes != oracle.SourceBytes || got.TimeDomain != "pcap_epoch_seconds" || got.Header.Resolution != "microsecond" || got.ValidateCurrentSource() != nil {
			t.Fatalf("public census/source/clock mismatch: %+v", got.PacketFlowsResult)
		}
		hmc306CheckOracleStats(t, "public all-record aggregate", tracewire.PacketFlowStats{Packets: got.PacketCount, CapturedBytes: got.CapturedBytes, OriginalBytes: got.OriginalBytes, TruncatedPackets: got.TruncatedPackets, MinTimestamp: got.MinTimestamp, MaxTimestamp: got.MaxTimestamp}, oracle.Global)
		previewSize := min(MaxLimit, len(want))
		if len(got.Members) != previewSize || got.OmittedTrackedFlows != oracle.FlowCount-uint64(previewSize) || len(got.Ungrouped) != len(oracle.Ungrouped) {
			t.Fatal("preview promoted to the full population or ungrouped reasons omitted")
		}
		seen := make(map[string]bool, previewSize)
		for i, row := range got.Members {
			checkRow(t, row)
			if row.ID == "" || seen[row.ID] || row.Stats.CapturedBytes != rankedBytes[i] {
				t.Fatal("public preview is duplicated or not ranked by captured frame bytes")
			}
			seen[row.ID] = true
		}
		for _, row := range got.Ungrouped {
			stats, exists := oracle.Ungrouped[row.Reason]
			if !exists {
				t.Fatalf("unexpected ungrouped reason %s", row.Reason)
			}
			hmc306CheckOracleStats(t, row.Reason, row.Stats, stats)
		}
	})
	t.Run("all_private_groups_not_just_preview", func(t *testing.T) {
		collector := flowCollector{groups: make(map[flowKey]*flowAccumulator), ungrouped: make(map[string]*tracewire.PacketFlowStats)}
		inventory, err := stream(t.Context(), oracle.Source, 1, collector.consume)
		if err != nil || inventory.Status != "complete" || collector.failure != nil || uint64(len(collector.groups)) != oracle.FlowCount || collector.grouped != oracle.GroupedPackets || collector.excluded != oracle.UngroupedPackets {
			t.Fatalf("complete low-level scan differs from independent census: %v", err)
		}
		seen := make(map[string]bool, len(want))
		for _, group := range collector.groups {
			key := hmc306OracleIdentityFor(group.row).key()
			if seen[key] {
				t.Fatal("private identity split became a duplicate public flow")
			}
			seen[key] = true
			checkRow(t, group.row)
		}
		if len(seen) != len(want) {
			t.Fatal("complete private census omitted an oracle identity")
		}
	})
	t.Logf("independent source=%s bytes=%d packets=%d grouped=%d ungrouped=%d all_groups=%d public_preview=%d", oracle.SourceSHA256, oracle.SourceBytes, oracle.PacketCount, oracle.GroupedPackets, oracle.UngroupedPackets, oracle.FlowCount, min(MaxLimit, len(want)))
}

type hmc306OracleStats struct {
	Packets          uint64  `json:"packets"`
	CapturedBytes    uint64  `json:"captured_bytes"`
	OriginalBytes    uint64  `json:"original_bytes"`
	TruncatedPackets uint64  `json:"truncated_packets"`
	MinEpochUS       *uint64 `json:"min_epoch_us"`
	MaxEpochUS       *uint64 `json:"max_epoch_us"`
}

type hmc306OracleIdentity struct {
	SourceSHA256   string                       `json:"source_sha256"`
	LinkType       uint16                       `json:"linktype"`
	InterfaceKnown bool                         `json:"interface_known"`
	InterfaceIndex uint32                       `json:"interface_index"`
	VLANs          []tracewire.PacketFlowVLAN   `json:"vlan_stack"`
	IPFamily       string                       `json:"ip_family"`
	IPProtocol     uint8                        `json:"ip_protocol"`
	EndpointA      tracewire.PacketFlowEndpoint `json:"endpoint_a"`
	EndpointB      tracewire.PacketFlowEndpoint `json:"endpoint_b"`
}

func (i hmc306OracleIdentity) key() string {
	i.SourceSHA256 = "" // Checked independently against the scanned source bytes.
	b, _ := json.Marshal(i)
	return string(b)
}

func hmc306OracleIdentityFor(row tracewire.PacketFlow) hmc306OracleIdentity {
	i := hmc306OracleIdentity{LinkType: row.LinkType, VLANs: row.VLANs, IPFamily: fmt.Sprintf("IPv%d", row.IPVersion), IPProtocol: row.IPProtocol, EndpointA: row.EndpointA, EndpointB: row.EndpointB}
	if row.InterfaceIndex != nil {
		i.InterfaceKnown, i.InterfaceIndex = true, *row.InterfaceIndex
	}
	return i
}

type hmc306OracleFlow struct {
	Identity     hmc306OracleIdentity `json:"identity"`
	Stats        hmc306OracleStats    `json:"stats"`
	AToB         hmc306OracleStats    `json:"a_to_b"`
	BToA         hmc306OracleStats    `json:"b_to_a"`
	SameEndpoint hmc306OracleStats    `json:"same_endpoint"`
}

func hmc306CheckOracleStats(t *testing.T, label string, got tracewire.PacketFlowStats, want hmc306OracleStats) {
	t.Helper()
	toUS := func(ts *tracewire.PacketTimestamp) *uint64 {
		if ts == nil {
			return nil
		}
		value := uint64(ts.Seconds)*1_000_000 + uint64(ts.Fraction)
		if ts.Decimal != fmt.Sprintf("%d.%06d", ts.Seconds, ts.Fraction) {
			t.Fatalf("%s lost exact epoch text: %+v", label, ts)
		}
		return &value
	}
	actual := hmc306OracleStats{got.Packets, got.CapturedBytes, got.OriginalBytes, got.TruncatedPackets, toUS(got.MinTimestamp), toUS(got.MaxTimestamp)}
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("%s differs from independent byte oracle: got=%+v want=%+v", label, actual, want)
	}
}
