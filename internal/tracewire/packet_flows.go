package tracewire

// PacketFlowsResult groups observed TCP/UDP endpoints within one capture
// generation. A group is not a connection, session, request or causal edge.
type PacketFlowsResult struct {
	View                string                `json:"view"`
	SourcePath          string                `json:"source_path"`
	Status              string                `json:"status"`
	TimeDomain          string                `json:"time_domain"`
	Header              PacketHeader          `json:"header"`
	SourceBytes         int64                 `json:"source_bytes"`
	PacketCount         uint64                `json:"packet_count"`
	CapturedBytes       uint64                `json:"captured_bytes"`
	OriginalBytes       uint64                `json:"original_bytes"`
	TruncatedPackets    uint64                `json:"truncated_packets"`
	GroupedPackets      uint64                `json:"grouped_packets"`
	UngroupedPackets    uint64                `json:"ungrouped_packets"`
	FlowCount           *uint64               `json:"flow_count,omitempty"`
	TrackedFlowCount    uint64                `json:"tracked_flow_count"`
	OmittedTrackedFlows uint64                `json:"omitted_tracked_flows"`
	GroupLimit          int                   `json:"group_limit"`
	GroupBudgetExceeded bool                  `json:"group_budget_exceeded"`
	FirstTimestamp      *PacketTimestamp      `json:"first_record_timestamp,omitempty"`
	LastTimestamp       *PacketTimestamp      `json:"last_record_timestamp,omitempty"`
	MinTimestamp        *PacketTimestamp      `json:"min_timestamp,omitempty"`
	MaxTimestamp        *PacketTimestamp      `json:"max_timestamp,omitempty"`
	OutOfOrderPackets   uint64                `json:"out_of_order_packets"`
	Members             []PacketFlow          `json:"members"`
	Ungrouped           []PacketFlowUngrouped `json:"ungrouped"`
	Failure             *PacketFailure        `json:"failure,omitempty"`
}

// PacketFlowStats counts original records, including duplicate bytes. The
// two byte measures are independent frame lengths, never payload or bandwidth.
// Missing timestamps mean no records in this direction, not epoch zero.
type PacketFlowStats struct {
	Packets                     uint64           `json:"packets"`
	CapturedBytes               uint64           `json:"captured_bytes"`
	OriginalBytes               uint64           `json:"original_bytes"`
	TruncatedPackets            uint64           `json:"truncated_packets"`
	DecoderPrefixLimitedPackets uint64           `json:"decoder_prefix_limited_packets"`
	MinTimestamp                *PacketTimestamp `json:"min_timestamp,omitempty"`
	MaxTimestamp                *PacketTimestamp `json:"max_timestamp,omitempty"`
}

type PacketFlowEndpoint struct {
	Address string `json:"address"`
	Port    uint16 `json:"port"`
}

// A VLAN's priority/drop-eligibility bits are not network identity. Preserve
// the ordered tag protocol and VLAN ID, including a valid ID of zero.
type PacketFlowVLAN struct {
	TPID uint16 `json:"tpid"`
	ID   uint16 `json:"id"`
}

type PacketFlow struct {
	ID             string             `json:"id"`
	LinkType       uint16             `json:"link_type"`
	InterfaceIndex *uint32            `json:"interface_index,omitempty"`
	VLANs          []PacketFlowVLAN   `json:"vlans"`
	IPVersion      uint8              `json:"ip_version"`
	IPProtocol     uint8              `json:"ip_protocol"`
	Protocol       string             `json:"protocol"`
	EndpointA      PacketFlowEndpoint `json:"endpoint_a"`
	EndpointB      PacketFlowEndpoint `json:"endpoint_b"`
	Stats          PacketFlowStats    `json:"stats"`
	AToB           PacketFlowStats    `json:"a_to_b"`
	BToA           PacketFlowStats    `json:"b_to_a"`
	SameEndpoint   PacketFlowStats    `json:"same_endpoint"`
	Directions     []PacketCount      `json:"directions"`
}

// Ungrouped is a disjoint record classification, not an unknown endpoint group.
// In particular a missing port must never join a real port zero or other
// missing identities. Budget exclusions are explicit and not network absence.
type PacketFlowUngrouped struct {
	Reason string          `json:"reason"`
	Stats  PacketFlowStats `json:"stats"`
}
