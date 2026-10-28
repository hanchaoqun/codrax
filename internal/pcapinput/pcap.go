// Package pcapinput reads classic PCAP records without materializing packet
// payloads or aligning their wall clock with a scheduler trace clock.
package pcapinput

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sort"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

const (
	DefaultLimit      = 32
	MaxLimit          = 64
	PacketPrefixBytes = 4096
)

type Header struct {
	ByteOrder    string `json:"byte_order"`
	Resolution   string `json:"timestamp_resolution"`
	Snaplen      uint32 `json:"snaplen_bytes"`
	LinkType     uint16 `json:"link_type"`
	LinkTypeWord uint32 `json:"link_type_word"`
}

type Timestamp struct {
	Seconds  uint32 `json:"seconds"`
	Fraction uint32 `json:"fraction"`
	Decimal  string `json:"epoch_seconds"`
}

type Packet struct {
	Ordinal              uint64    `json:"ordinal"`
	FileOffset           int64     `json:"file_offset"`
	Timestamp            Timestamp `json:"timestamp"`
	CapturedBytes        uint32    `json:"captured_bytes"`
	OriginalBytes        uint32    `json:"original_bytes"`
	Truncated            bool      `json:"capture_truncated"`
	DecoderPrefixLimited bool      `json:"decoder_prefix_limited"`
	InterfaceIndex       *uint32   `json:"interface_index,omitempty"`
	PacketType           *uint16   `json:"packet_type,omitempty"`
	Direction            string    `json:"direction"`
	LinkProtocol         *uint16   `json:"link_protocol,omitempty"`
	IPProtocol           *uint8    `json:"ip_protocol,omitempty"`
	Protocol             string    `json:"protocol"`
	DecodeStatus         string    `json:"decode_status"`
	SourceAddress        string    `json:"source_address,omitempty"`
	DestinationAddress   string    `json:"destination_address,omitempty"`
	SourcePort           *uint16   `json:"source_port,omitempty"`
	DestinationPort      *uint16   `json:"destination_port,omitempty"`
	Fragmented           bool      `json:"fragmented"`
}

type Count struct {
	Label string `json:"label"`
	Count uint64 `json:"count"`
}
type Failure struct {
	Code   string `json:"code"`
	Offset int64  `json:"file_offset"`
}
type Result struct {
	sourceIdentity    filegeneration.Identity
	View              string     `json:"view"`
	SourcePath        string     `json:"source_path"`
	Status            string     `json:"status"`
	TimeDomain        string     `json:"time_domain"`
	Header            Header     `json:"header"`
	SourceBytes       int64      `json:"source_bytes"`
	PacketCount       uint64     `json:"packet_count"`
	OmittedPackets    uint64     `json:"omitted_packets"`
	CapturedBytes     uint64     `json:"captured_bytes"`
	OriginalBytes     uint64     `json:"original_bytes"`
	TruncatedPackets  uint64     `json:"truncated_packets"`
	FirstTimestamp    *Timestamp `json:"first_record_timestamp,omitempty"`
	LastTimestamp     *Timestamp `json:"last_record_timestamp,omitempty"`
	MinTimestamp      *Timestamp `json:"min_timestamp,omitempty"`
	MaxTimestamp      *Timestamp `json:"max_timestamp,omitempty"`
	OutOfOrderPackets uint64     `json:"out_of_order_packets"`
	Protocols         []Count    `json:"protocols"`
	DecodeStatuses    []Count    `json:"decode_statuses"`
	Directions        []Count    `json:"directions"`
	Members           []Packet   `json:"members"`
	Failure           *Failure   `json:"failure,omitempty"`
}

// SourceVersionToken belongs to the descriptor actually scanned, not a later
// path lookup. A replacement must not assign its generation to old bytes.
func (r Result) SourceVersionToken() string { return r.sourceIdentity.CacheToken() }
func (r Result) ValidateCurrentSource() error {
	identity, err := filegeneration.FromPath(r.SourcePath)
	if err != nil {
		return err
	}
	if !r.sourceIdentity.SameVersion(identity) {
		return fmt.Errorf("pcap_source_changed_after_scan")
	}
	return nil
}

func HasMagic(b []byte) bool {
	if len(b) < 4 {
		return false
	}
	v := binary.BigEndian.Uint32(b)
	return v == 0xa1b2c3d4 || v == 0xd4c3b2a1 || v == 0xa1b23c4d || v == 0x4d3cb2a1 || v == 0x0a0d0d0a
}

func ParseHeader(b []byte) (Header, error) {
	h := Header{}
	if len(b) < 4 {
		return h, fmt.Errorf("pcap_short_global_header")
	}
	var order binary.ByteOrder
	switch binary.BigEndian.Uint32(b) {
	case 0xa1b2c3d4:
		order, h.ByteOrder, h.Resolution = binary.BigEndian, "big", "microsecond"
	case 0xd4c3b2a1:
		order, h.ByteOrder, h.Resolution = binary.LittleEndian, "little", "microsecond"
	case 0xa1b23c4d:
		order, h.ByteOrder, h.Resolution = binary.BigEndian, "big", "nanosecond"
	case 0x4d3cb2a1:
		order, h.ByteOrder, h.Resolution = binary.LittleEndian, "little", "nanosecond"
	case 0x0a0d0d0a:
		return h, fmt.Errorf("pcapng_unsupported: classic PCAP 2.4 required")
	default:
		return h, fmt.Errorf("pcap_magic_unsupported")
	}
	if len(b) < 24 {
		return h, fmt.Errorf("pcap_short_global_header")
	}
	if order.Uint16(b[4:6]) != 2 || order.Uint16(b[6:8]) != 4 {
		return h, fmt.Errorf("pcap_version_unsupported")
	}
	h.Snaplen = order.Uint32(b[16:20])
	h.LinkTypeWord = order.Uint32(b[20:24])
	h.LinkType = uint16(h.LinkTypeWord & 0xffff)
	if h.Snaplen == 0 {
		return h, fmt.Errorf("pcap_invalid_snaplen")
	}
	return h, nil
}

// Stream reads at most one bounded prefix per record. caplen is used only for
// checked file offsets; it never controls an allocation. The held descriptor
// and original path generation must remain identical throughout the scan.
func Stream(ctx context.Context, path string, limit int) (out Result, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	f, identity, err := filegeneration.OpenRegularReadOnly(path)
	if err != nil {
		return out, err
	}
	defer func() {
		current, e := filegeneration.FromFile(f)
		if e == nil && !identity.SameVersion(current) {
			e = fmt.Errorf("pcap_source_changed")
		}
		byPath, pe := filegeneration.FromPath(path)
		if pe == nil && !identity.SameVersion(byPath) {
			pe = fmt.Errorf("pcap_source_replaced")
		}
		err = errors.Join(err, e, pe, ctx.Err(), f.Close())
	}()
	var global [24]byte
	nGlobal, _ := io.ReadFull(f, global[:])
	h, err := ParseHeader(global[:nGlobal])
	if err != nil {
		return out, err
	}
	var order binary.ByteOrder = binary.LittleEndian
	if h.ByteOrder == "big" {
		order = binary.BigEndian
	}
	precision, scale := 6, uint32(1000000)
	if h.Resolution == "nanosecond" {
		precision, scale = 9, 1000000000
	}
	out = Result{sourceIdentity: identity, View: "packet_inventory", SourcePath: path, Status: "complete", TimeDomain: "pcap_epoch_seconds", Header: h, SourceBytes: identity.Size(), Members: []Packet{}}
	protocols, statuses, directions := map[string]uint64{}, map[string]uint64{}, map[string]uint64{}
	var prefix [PacketPrefixBytes]byte
	var record [16]byte
	partial := func(code string, offset int64) {
		out.Status = "partial"
		out.Failure = &Failure{Code: code, Offset: offset}
	}
	for offset := int64(24); offset < identity.Size(); {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		if identity.Size()-offset < 16 {
			partial("pcap_short_record_header", offset)
			break
		}
		if _, err := f.ReadAt(record[:], offset); err != nil {
			return out, err
		}
		sec, frac := order.Uint32(record[0:4]), order.Uint32(record[4:8])
		caplen, wirelen := order.Uint32(record[8:12]), order.Uint32(record[12:16])
		if frac >= scale {
			partial("pcap_invalid_timestamp_fraction", offset)
			break
		}
		if caplen > h.Snaplen || caplen > wirelen {
			partial("pcap_invalid_record_lengths", offset)
			break
		}
		if int64(caplen) > identity.Size()-offset-16 {
			partial("pcap_short_packet_data", offset)
			break
		}
		n := int64(caplen)
		if n > PacketPrefixBytes {
			n = PacketPrefixBytes
		}
		if _, err := f.ReadAt(prefix[:n], offset+16); err != nil {
			return out, err
		}
		p := Packet{Ordinal: out.PacketCount + 1, FileOffset: offset, Timestamp: Timestamp{Seconds: sec, Fraction: frac, Decimal: fmt.Sprintf("%d.%0*d", sec, precision, frac)}, CapturedBytes: caplen, OriginalBytes: wirelen, Truncated: caplen < wirelen, Direction: "unknown", Protocol: "unknown", DecodeStatus: "unknown_link_type"}
		p.DecoderPrefixLimited = int64(caplen) > n
		decodePacket(&p, h.LinkType, prefix[:n])
		if out.FirstTimestamp == nil {
			ts := p.Timestamp
			out.FirstTimestamp, out.MinTimestamp, out.MaxTimestamp = &ts, &ts, &ts
		}
		if out.LastTimestamp != nil && less(p.Timestamp, *out.LastTimestamp) {
			out.OutOfOrderPackets++
		}
		last := p.Timestamp
		out.LastTimestamp = &last
		if less(last, *out.MinTimestamp) {
			ts := last
			out.MinTimestamp = &ts
		}
		if less(*out.MaxTimestamp, last) {
			ts := last
			out.MaxTimestamp = &ts
		}
		out.PacketCount++
		out.CapturedBytes += uint64(caplen)
		if ^uint64(0)-out.OriginalBytes < uint64(wirelen) {
			return out, fmt.Errorf("pcap_original_length_total_overflow")
		}
		out.OriginalBytes += uint64(wirelen)
		if p.Truncated {
			out.TruncatedPackets++
		}
		protocols[p.Protocol]++
		statuses[p.DecodeStatus]++
		directions[p.Direction]++
		if len(out.Members) < limit {
			out.Members = append(out.Members, p)
		}
		offset += 16 + int64(caplen)
	}
	out.OmittedPackets = out.PacketCount - uint64(len(out.Members))
	out.Protocols, out.DecodeStatuses, out.Directions = counts(protocols), counts(statuses), counts(directions)
	return out, nil
}

func less(a, b Timestamp) bool {
	return a.Seconds < b.Seconds || a.Seconds == b.Seconds && a.Fraction < b.Fraction
}
func counts(m map[string]uint64) []Count {
	rows := make([]Count, 0, len(m))
	for k, v := range m {
		rows = append(rows, Count{k, v})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Count != rows[j].Count {
			return rows[i].Count > rows[j].Count
		}
		return rows[i].Label < rows[j].Label
	})
	return rows
}

func packetType(p *Packet, value uint16) {
	p.PacketType = &value
	switch value {
	case 0:
		p.Direction = "incoming_host"
	case 1:
		p.Direction = "incoming_broadcast"
	case 2:
		p.Direction = "incoming_multicast"
	case 3:
		p.Direction = "incoming_other_host"
	case 4:
		p.Direction = "outgoing"
	}
}

func decodePacket(p *Packet, link uint16, b []byte) {
	offset, protocol := 0, uint16(0)
	p.DecodeStatus = "truncated_link_header"
	switch link {
	case 1:
		if len(b) < 14 {
			return
		}
		offset, protocol = 14, binary.BigEndian.Uint16(b[12:14])
	case 113:
		if len(b) < 16 {
			return
		}
		packetType(p, binary.BigEndian.Uint16(b[0:2]))
		offset, protocol = 16, binary.BigEndian.Uint16(b[14:16])
	case 276:
		if len(b) < 20 {
			return
		}
		index := binary.BigEndian.Uint32(b[4:8])
		p.InterfaceIndex = &index
		packetType(p, uint16(b[10]))
		offset, protocol = 20, binary.BigEndian.Uint16(b[0:2])
	case 101:
		if len(b) == 0 {
			return
		}
		switch b[0] >> 4 {
		case 4:
			protocol = 0x800
		case 6:
			protocol = 0x86dd
		default:
			p.DecodeStatus = "unknown_ip_version"
			return
		}
	case 228:
		protocol = 0x800
	case 229:
		protocol = 0x86dd
	default:
		p.DecodeStatus = "unknown_link_type"
		return
	}
	for tags := 0; protocol == 0x8100 || protocol == 0x88a8; tags++ {
		if tags == 8 {
			p.DecodeStatus = "link_header_budget_exceeded"
			return
		}
		if len(b)-offset < 4 {
			return
		}
		protocol, offset = binary.BigEndian.Uint16(b[offset+2:offset+4]), offset+4
	}
	p.LinkProtocol = &protocol
	switch protocol {
	case 0x800:
		decodeIPv4(p, b[offset:])
	case 0x86dd:
		decodeIPv6(p, b[offset:])
	case 0x806:
		p.Protocol, p.DecodeStatus = "ARP", "network_protocol_only"
	default:
		p.Protocol, p.DecodeStatus = fmt.Sprintf("ethertype_0x%04x", protocol), "unknown_network_protocol"
	}
}

func setIPProtocol(p *Packet, protocol byte) {
	p.IPProtocol = &protocol
	switch protocol {
	case 1:
		p.Protocol = "ICMP"
	case 6:
		p.Protocol = "TCP"
	case 17:
		p.Protocol = "UDP"
	case 58:
		p.Protocol = "ICMPv6"
	case 50:
		p.Protocol = "ESP"
	case 59:
		p.Protocol = "IPv6_no_next_header"
	default:
		p.Protocol = fmt.Sprintf("ip_protocol_%d", protocol)
	}
}

func decodeIPv4(p *Packet, b []byte) {
	p.Protocol, p.DecodeStatus = "IPv4", "truncated_ip_header"
	if len(b) < 20 {
		return
	}
	ihl, total := int(b[0]&15)*4, int(binary.BigEndian.Uint16(b[2:4]))
	if b[0]>>4 != 4 || ihl < 20 || total < ihl {
		p.DecodeStatus = "invalid_ip_header"
		return
	}
	if len(b) < ihl {
		return
	}
	p.SourceAddress, p.DestinationAddress = net.IP(b[12:16]).String(), net.IP(b[16:20]).String()
	setIPProtocol(p, b[9])
	fragment := binary.BigEndian.Uint16(b[6:8])
	if fragment&0x3fff != 0 {
		p.Fragmented, p.DecodeStatus = true, "fragment_transport_unresolved"
		return
	}
	if len(b) > total {
		b = b[:total]
	}
	decodeTransport(p, b[ihl:])
}

func decodeIPv6(p *Packet, b []byte) {
	p.Protocol, p.DecodeStatus = "IPv6", "truncated_ip_header"
	if len(b) < 40 {
		return
	}
	if b[0]>>4 != 6 {
		p.DecodeStatus = "invalid_ip_header"
		return
	}
	p.SourceAddress, p.DestinationAddress = net.IP(b[8:24]).String(), net.IP(b[24:40]).String()
	payloadLength := int(binary.BigEndian.Uint16(b[4:6]))
	if payloadLength == 0 {
		if b[6] == 59 {
			setIPProtocol(p, b[6])
			p.DecodeStatus = "network_protocol_only"
			return
		}
		p.DecodeStatus = "ipv6_jumbo_or_empty_unresolved"
		return
	}
	if len(b) > 40+payloadLength {
		b = b[:40+payloadLength]
	}
	next, offset := b[6], 40
	for hops := 0; ; hops++ {
		if hops == 16 {
			p.DecodeStatus = "ipv6_extension_budget_exceeded"
			return
		}
		switch next {
		case 0, 43, 60, 51:
			if len(b)-offset < 2 {
				p.DecodeStatus = "truncated_ipv6_extension"
				return
			}
			length := (int(b[offset+1]) + 1) * 8
			if next == 51 {
				length = (int(b[offset+1]) + 2) * 4
			}
			if len(b)-offset < length {
				p.DecodeStatus = "truncated_ipv6_extension"
				return
			}
			next, offset = b[offset], offset+length
		case 44:
			p.Fragmented = true
			if len(b)-offset < 8 {
				p.DecodeStatus = "truncated_ipv6_fragment"
				return
			}
			setIPProtocol(p, b[offset])
			p.DecodeStatus = "fragment_transport_unresolved"
			return
		default:
			setIPProtocol(p, next)
			decodeTransport(p, b[offset:])
			return
		}
	}
}

func decodeTransport(p *Packet, b []byte) {
	p.DecodeStatus = "network_protocol_only"
	if p.IPProtocol == nil {
		return
	}
	switch *p.IPProtocol {
	case 6:
		p.DecodeStatus = "truncated_transport_header"
		if len(b) < 20 {
			return
		}
		length := int(b[12]>>4) * 4
		if length < 20 {
			p.DecodeStatus = "invalid_tcp_header"
			return
		}
		if len(b) < length {
			return
		}
	case 17:
		p.DecodeStatus = "truncated_transport_header"
		if len(b) < 8 {
			return
		}
		if binary.BigEndian.Uint16(b[4:6]) < 8 {
			p.DecodeStatus = "invalid_udp_header"
			return
		}
	default:
		return
	}
	src, dst := binary.BigEndian.Uint16(b[0:2]), binary.BigEndian.Uint16(b[2:4])
	p.SourcePort, p.DestinationPort, p.DecodeStatus = &src, &dst, "transport_header_decoded"
}
