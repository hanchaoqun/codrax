package tracewire

// PacketInventoryResult is a packet-record census, not a scheduler trace. Its
// timestamps preserve the capture's epoch clock and never imply clock alignment.
// Source-generation admission stays private to the reader that scanned it.
type PacketInventoryResult struct {
	View              string           `json:"view"`
	SourcePath        string           `json:"source_path"`
	Status            string           `json:"status"`
	TimeDomain        string           `json:"time_domain"`
	Header            PacketHeader     `json:"header"`
	SourceBytes       int64            `json:"source_bytes"`
	PacketCount       uint64           `json:"packet_count"`
	OmittedPackets    uint64           `json:"omitted_packets"`
	CapturedBytes     uint64           `json:"captured_bytes"`
	OriginalBytes     uint64           `json:"original_bytes"`
	TruncatedPackets  uint64           `json:"truncated_packets"`
	FirstTimestamp    *PacketTimestamp `json:"first_record_timestamp,omitempty"`
	LastTimestamp     *PacketTimestamp `json:"last_record_timestamp,omitempty"`
	MinTimestamp      *PacketTimestamp `json:"min_timestamp,omitempty"`
	MaxTimestamp      *PacketTimestamp `json:"max_timestamp,omitempty"`
	OutOfOrderPackets uint64           `json:"out_of_order_packets"`
	Protocols         []PacketCount    `json:"protocols"`
	DecodeStatuses    []PacketCount    `json:"decode_statuses"`
	Directions        []PacketCount    `json:"directions"`
	Members           []PacketRecord   `json:"members"`
	Failure           *PacketFailure   `json:"failure,omitempty"`
}

type PacketHeader struct {
	ByteOrder    string `json:"byte_order"`
	Resolution   string `json:"timestamp_resolution"`
	Snaplen      uint32 `json:"snaplen_bytes"`
	LinkType     uint16 `json:"link_type"`
	LinkTypeWord uint32 `json:"link_type_word"`
}

// Decimal avoids losing the native microsecond or nanosecond fraction to a
// floating-point conversion. Fraction uses Header.Resolution, not Trace units.
type PacketTimestamp struct {
	Seconds  uint32 `json:"seconds"`
	Fraction uint32 `json:"fraction"`
	Decimal  string `json:"epoch_seconds"`
}

type PacketRecord struct {
	Ordinal              uint64          `json:"ordinal"`
	FileOffset           int64           `json:"file_offset"`
	Timestamp            PacketTimestamp `json:"timestamp"`
	CapturedBytes        uint32          `json:"captured_bytes"`
	OriginalBytes        uint32          `json:"original_bytes"`
	Truncated            bool            `json:"capture_truncated"`
	DecoderPrefixLimited bool            `json:"decoder_prefix_limited"`
	InterfaceIndex       *uint32         `json:"interface_index,omitempty"`
	PacketType           *uint16         `json:"packet_type,omitempty"`
	Direction            string          `json:"direction"`
	LinkProtocol         *uint16         `json:"link_protocol,omitempty"`
	IPProtocol           *uint8          `json:"ip_protocol,omitempty"`
	Protocol             string          `json:"protocol"`
	DecodeStatus         string          `json:"decode_status"`
	SourceAddress        string          `json:"source_address,omitempty"`
	DestinationAddress   string          `json:"destination_address,omitempty"`
	SourcePort           *uint16         `json:"source_port,omitempty"`
	DestinationPort      *uint16         `json:"destination_port,omitempty"`
	Fragmented           bool            `json:"fragmented"`
}

type PacketCount struct {
	Label string `json:"label"`
	Count uint64 `json:"count"`
}

type PacketFailure struct {
	Code   string `json:"code"`
	Offset int64  `json:"file_offset"`
}
