package tracequery

const ViewPacketFlows = "packet_flows"

const PacketFlowsTeaching = "For classic PCAP endpoint traffic use view=packet_flows with source/path and optional limit only. It scans all records before previewing TCP/UDP bidirectional endpoint groups, ordered by captured frame bytes. Identity includes the physical source generation, link/interface/VLAN observation domain, IP family, protocol and canonical endpoint pair. A-to-B/B-to-A/same-endpoint counts and captured/original frame bytes are independent from Linux incoming/outgoing direction. Unknown endpoints, fragments and unsupported decoding remain reasoned ungrouped records. The 4096-group memory bound is independent of preview limit: exceeded or malformed input returns partial facts, never a complete census or global Top N. Native epoch min/max are observed record bounds, not a query window or response latency. Endpoint groups are not proven connections, sessions, requests, encryption, application bytes, process identity or causal links. No Trace time/line/thread filters or clock alignment are supported."

// IsPacketView is an exact operation-family contract, never a filename or
// request-text heuristic. Packet observations cannot inherit Trace coordinates.
func IsPacketView(view string) bool {
	switch CanonicalViewName(view) {
	case ViewPacketInventory, ViewPacketFlows:
		return true
	}
	return false
}
