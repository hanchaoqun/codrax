package pcapinput

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/netip"
	"path/filepath"
	"sort"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/tracewire"
)

const MaxFlowGroups = 4096

type FlowResult struct {
	tracewire.PacketFlowsResult
	sourceIdentity filegeneration.Identity
}

func (r FlowResult) SourceVersionToken() string { return r.sourceIdentity.CacheToken() }
func (r FlowResult) ValidateCurrentSource() error {
	identity, err := filegeneration.FromPath(r.SourcePath)
	if err != nil {
		return err
	}
	if !r.sourceIdentity.SameVersion(identity) {
		return fmt.Errorf("pcap_source_changed_after_scan")
	}
	return nil
}

type flowKey struct {
	domain         packetDomain
	interfaceKnown bool
	interfaceIndex uint32
	protocol       uint8
	a, b           netip.AddrPort
}

type flowAccumulator struct {
	row        tracewire.PacketFlow
	directions map[string]uint64
}

type flowCollector struct {
	groups    map[flowKey]*flowAccumulator
	ungrouped map[string]*tracewire.PacketFlowStats
	grouped   uint64
	excluded  uint64
	failure   *Failure
}

// StreamFlows scans a held source generation once. The fixed group budget is
// independent of limit, which controls only the sorted result preview. Budget
// exhaustion retains exact tracked groups and excluded-record counts, but the
// all-flow count remains unknown and no complete result is claimed.
func StreamFlows(ctx context.Context, path string, limit int) (FlowResult, error) {
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	c := flowCollector{groups: make(map[flowKey]*flowAccumulator), ungrouped: make(map[string]*tracewire.PacketFlowStats)}
	inventory, err := stream(ctx, path, 1, c.consume)
	if err != nil {
		return FlowResult{}, err
	}
	p := inventory.PacketInventoryResult
	out := FlowResult{sourceIdentity: inventory.sourceIdentity, PacketFlowsResult: tracewire.PacketFlowsResult{
		View: "packet_flows", SourcePath: p.SourcePath, Status: p.Status, TimeDomain: p.TimeDomain,
		Header: p.Header, SourceBytes: p.SourceBytes, PacketCount: p.PacketCount, CapturedBytes: p.CapturedBytes,
		OriginalBytes: p.OriginalBytes, TruncatedPackets: p.TruncatedPackets, GroupedPackets: c.grouped,
		UngroupedPackets: c.excluded, TrackedFlowCount: uint64(len(c.groups)), GroupLimit: MaxFlowGroups,
		GroupBudgetExceeded: c.failure != nil, FirstTimestamp: p.FirstTimestamp, LastTimestamp: p.LastTimestamp,
		MinTimestamp: p.MinTimestamp, MaxTimestamp: p.MaxTimestamp, OutOfOrderPackets: p.OutOfOrderPackets,
		Members: []tracewire.PacketFlow{}, Ungrouped: []tracewire.PacketFlowUngrouped{}, Failure: p.Failure,
	}}
	if c.failure != nil {
		out.Status = "partial"
		if out.Failure == nil {
			out.Failure = c.failure
		}
	}
	if out.Status == "complete" {
		count := uint64(len(c.groups))
		out.FlowCount = &count
	}
	// Identity carries no measured values or preview coordinates, and cannot
	// silently merge a copy/replacement file with the originally scanned source.
	for _, group := range c.groups {
		group.row.Directions = counts(group.directions)
		key := struct {
			LinkType       uint16
			InterfaceIndex *uint32
			VLANs          []tracewire.PacketFlowVLAN
			IPVersion      uint8
			IPProtocol     uint8
			EndpointA      tracewire.PacketFlowEndpoint
			EndpointB      tracewire.PacketFlowEndpoint
		}{group.row.LinkType, group.row.InterfaceIndex, group.row.VLANs, group.row.IPVersion,
			group.row.IPProtocol, group.row.EndpointA, group.row.EndpointB}
		encoded, marshalErr := json.Marshal(key)
		if marshalErr != nil {
			return FlowResult{}, marshalErr
		}
		group.row.ID = fmt.Sprintf("pcap-flow:%x", sha256.Sum256(append([]byte(filepath.Clean(path)+"\x00"+out.SourceVersionToken()+"\x00"), encoded...)))
		out.Members = append(out.Members, group.row)
	}
	sort.Slice(out.Members, func(i, j int) bool {
		a, b := out.Members[i], out.Members[j]
		if a.Stats.CapturedBytes != b.Stats.CapturedBytes {
			return a.Stats.CapturedBytes > b.Stats.CapturedBytes
		}
		return a.ID < b.ID
	})
	if len(out.Members) > limit {
		out.OmittedTrackedFlows = uint64(len(out.Members) - limit)
		// Do not return a preview that retains the entire aggregate slice.
		out.Members = append([]tracewire.PacketFlow(nil), out.Members[:limit]...)
	}
	for reason, stats := range c.ungrouped {
		out.Ungrouped = append(out.Ungrouped, tracewire.PacketFlowUngrouped{Reason: reason, Stats: *stats})
	}
	sort.Slice(out.Ungrouped, func(i, j int) bool { return out.Ungrouped[i].Reason < out.Ungrouped[j].Reason })
	return out, nil
}

func (c *flowCollector) consume(p Packet, domain packetDomain) error {
	key, direction, reason := packetFlowKey(p, domain)
	if reason != "" {
		return c.exclude(reason, p)
	}
	group := c.groups[key]
	if group == nil {
		if len(c.groups) == MaxFlowGroups {
			if c.failure == nil {
				c.failure = &Failure{Code: "pcap_flow_group_budget_exceeded", Offset: p.FileOffset}
			}
			return c.exclude("flow_group_budget_exceeded", p)
		}
		group = &flowAccumulator{row: tracewire.PacketFlow{
			LinkType: domain.link, VLANs: append([]tracewire.PacketFlowVLAN{}, domain.vlans[:domain.vlanCount]...),
			IPVersion: domain.ipVersion, IPProtocol: key.protocol, Protocol: p.Protocol,
			EndpointA: tracewire.PacketFlowEndpoint{Address: key.a.Addr().String(), Port: key.a.Port()},
			EndpointB: tracewire.PacketFlowEndpoint{Address: key.b.Addr().String(), Port: key.b.Port()},
		}, directions: map[string]uint64{}}
		if key.interfaceKnown {
			index := key.interfaceIndex
			group.row.InterfaceIndex = &index
		}
		c.groups[key] = group
	}
	if err := addFlowStats(&group.row.Stats, p); err != nil {
		return err
	}
	stats := &group.row.AToB
	if direction < 0 {
		stats = &group.row.BToA
	} else if direction == 0 {
		stats = &group.row.SameEndpoint
	}
	if err := addFlowStats(stats, p); err != nil {
		return err
	}
	group.directions[p.Direction]++
	c.grouped++
	return nil
}

func (c *flowCollector) exclude(reason string, p Packet) error {
	stats := c.ungrouped[reason]
	if stats == nil {
		stats = &tracewire.PacketFlowStats{}
		c.ungrouped[reason] = stats
	}
	if err := addFlowStats(stats, p); err != nil {
		return err
	}
	c.excluded++
	return nil
}

func packetFlowKey(p Packet, domain packetDomain) (flowKey, int, string) {
	if p.Fragmented {
		return flowKey{}, 0, "fragment_transport_unresolved"
	}
	if !domain.source.IsValid() || !domain.target.IsValid() || p.IPProtocol == nil {
		return flowKey{}, 0, "network_identity_unresolved"
	}
	if *p.IPProtocol != 6 && *p.IPProtocol != 17 {
		return flowKey{}, 0, "unsupported_transport_protocol"
	}
	if p.SourcePort == nil || p.DestinationPort == nil || p.DecodeStatus != "transport_header_decoded" {
		return flowKey{}, 0, "transport_identity_unresolved"
	}
	a, b := netip.AddrPortFrom(domain.source, *p.SourcePort), netip.AddrPortFrom(domain.target, *p.DestinationPort)
	direction := 1
	if a == b {
		direction = 0
	} else if a.Compare(b) > 0 {
		a, b, direction = b, a, -1
	}
	// Source/destination order must not survive in the bidirectional map key.
	domain.source, domain.target = netip.Addr{}, netip.Addr{}
	key := flowKey{domain: domain, protocol: *p.IPProtocol, a: a, b: b}
	if p.InterfaceIndex != nil {
		key.interfaceKnown, key.interfaceIndex = true, *p.InterfaceIndex
	}
	return key, direction, ""
}

func addFlowStats(s *tracewire.PacketFlowStats, p Packet) error {
	if s.Packets == ^uint64(0) || ^uint64(0)-s.CapturedBytes < uint64(p.CapturedBytes) || ^uint64(0)-s.OriginalBytes < uint64(p.OriginalBytes) {
		return fmt.Errorf("pcap_flow_statistics_overflow")
	}
	s.Packets++
	s.CapturedBytes += uint64(p.CapturedBytes)
	s.OriginalBytes += uint64(p.OriginalBytes)
	if p.Truncated {
		s.TruncatedPackets++
	}
	if p.DecoderPrefixLimited {
		s.DecoderPrefixLimitedPackets++
	}
	if s.MinTimestamp == nil || less(p.Timestamp, *s.MinTimestamp) {
		ts := p.Timestamp
		s.MinTimestamp = &ts
	}
	if s.MaxTimestamp == nil || less(*s.MaxTimestamp, p.Timestamp) {
		ts := p.Timestamp
		s.MaxTimestamp = &ts
	}
	return nil
}
