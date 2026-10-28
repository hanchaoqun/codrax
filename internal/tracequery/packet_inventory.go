package tracequery

import (
	"context"
	"github.com/hanchaoqun/codrax/internal/pcapinput"
)

const ViewPacketInventory = "packet_inventory"
const PacketInventoryTeaching = "For a classic PCAP packet capture use view=packet_inventory with source/path and optional limit only. This bounded streaming view scans all complete records and publishes packet counts, exact epoch timestamps, captured/original frame lengths, protocol and source record identities; limit controls member preview only. PCAP time is not Trace time: time/line/PID/thread/other filters and cross-source clock alignment are not supported in this view. Unknown protocol, truncation and record failure remain explicit; lengths are recorded frame lengths, not transport payload, application traffic or physical wire bandwidth. Packet direction is available only from explicit Linux cooked packet type. Packet facts provide no thread, wait or causal relationship."

func StreamPacketInventory(ctx context.Context, path string, limit int) (pcapinput.Result, error) {
	return pcapinput.Stream(ctx, path, limit)
}
