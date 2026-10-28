package pcapinput

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func captureBytes(order binary.ByteOrder, nano bool, link, snap uint32, packets ...[]byte) []byte {
	var b bytes.Buffer
	magic := uint32(0xa1b2c3d4)
	if nano {
		magic = 0xa1b23c4d
	}
	for _, v := range []any{magic, uint16(2), uint16(4), int32(0), uint32(0), snap, link} {
		_ = binary.Write(&b, order, v)
	}
	for i, packet := range packets {
		for _, v := range []uint32{1789020987, uint32(35118 + i), uint32(len(packet)), uint32(len(packet))} {
			_ = binary.Write(&b, order, v)
		}
		b.Write(packet)
	}
	return b.Bytes()
}
func putCapture(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "capture.bin")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
func ipv4UDP() []byte {
	b := make([]byte, 28)
	b[0], b[9] = 0x45, 17
	binary.BigEndian.PutUint16(b[2:4], 28)
	copy(b[12:20], []byte{192, 0, 2, 1, 192, 0, 2, 2})
	binary.BigEndian.PutUint16(b[20:22], 1234)
	binary.BigEndian.PutUint16(b[22:24], 4321)
	binary.BigEndian.PutUint16(b[24:26], 8)
	return b
}

func TestPCAPEndianResolutionIdentityAndPreview(t *testing.T) {
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		for _, nano := range []bool{false, true} {
			t.Run(order.String()+"/"+map[bool]string{false: "micro", true: "nano"}[nano], func(t *testing.T) {
				path := putCapture(t, captureBytes(order, nano, 0x24000065, 65535, ipv4UDP(), ipv4UDP()))
				got, err := Stream(t.Context(), path, 1)
				if err != nil {
					t.Fatal(err)
				}
				want := "1789020987.035118"
				if nano {
					want = "1789020987.000035118"
				}
				if got.Status != "complete" || got.PacketCount != 2 || got.OmittedPackets != 1 || got.CapturedBytes != 56 || len(got.Members) != 1 || got.Members[0].FileOffset != 24 || got.FirstTimestamp.Decimal != want || got.Header.LinkType != 101 || got.Header.LinkTypeWord != 0x24000065 || got.Protocols[0] != (Count{"UDP", 2}) {
					t.Fatalf("lost native facts: %+v", got)
				}
				full, err := Stream(t.Context(), path, 64)
				if err != nil || full.Members[0].Ordinal == full.Members[1].Ordinal || full.Members[1].FileOffset != 68 {
					t.Fatalf("duplicates collapsed: %+v %v", full, err)
				}
				if got.ValidateCurrentSource() != nil {
					t.Fatal("fresh source rejected")
				}
				replacement := putCapture(t, captureBytes(order, nano, 101, 65535, ipv4UDP()))
				if err := os.Rename(replacement, path); err != nil {
					t.Fatal(err)
				}
				if got.ValidateCurrentSource() == nil {
					t.Fatal("old payload accepted replacement generation")
				}
			})
		}
	}
}

func TestPCAPMalformedAndEmptyAreDistinct(t *testing.T) {
	base := captureBytes(binary.LittleEndian, false, 101, 65535, ipv4UDP())
	cases := []struct {
		name  string
		data  []byte
		code  string
		count uint64
	}{
		{"short_tail", append(append([]byte(nil), base...), 1, 2, 3), "pcap_short_record_header", 1},
		{"short_packet", append([]byte(nil), base[:len(base)-1]...), "pcap_short_packet_data", 0},
		{"invalid_lengths", append([]byte(nil), base...), "pcap_invalid_record_lengths", 0},
		{"invalid_fraction", append([]byte(nil), base...), "pcap_invalid_timestamp_fraction", 0},
		{"huge_caplen", append([]byte(nil), base...), "pcap_short_packet_data", 0},
	}
	binary.LittleEndian.PutUint32(cases[2].data[32:36], 29)
	binary.LittleEndian.PutUint32(cases[3].data[28:32], 1000000)
	for _, span := range [][2]int{{16, 20}, {32, 36}, {36, 40}} {
		binary.LittleEndian.PutUint32(cases[4].data[span[0]:span[1]], ^uint32(0))
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Stream(t.Context(), putCapture(t, c.data), 2)
			if err != nil || got.Status != "partial" || got.Failure == nil || got.Failure.Code != c.code || got.PacketCount != c.count {
				t.Fatalf("bad record became complete/zero: %+v %v", got, err)
			}
		})
	}
	empty, err := Stream(t.Context(), putCapture(t, base[:24]), 2)
	if err != nil || empty.Status != "complete" || empty.PacketCount != 0 || empty.MinTimestamp != nil || empty.FirstTimestamp != nil {
		t.Fatalf("valid empty inventory wrong: %+v %v", empty, err)
	}
	for _, data := range [][]byte{{0x0a, 0x0d, 0x0d, 0x0a}, base[:10], []byte("not a capture")} {
		if _, err := Stream(t.Context(), putCapture(t, data), 2); err == nil {
			t.Fatal("bad header accepted")
		}
	}
	if _, err := Stream(t.Context(), putCapture(t, []byte{0x0a, 0x0d, 0x0d, 0x0a}), 2); err == nil || !strings.Contains(err.Error(), "pcapng_unsupported") {
		t.Fatalf("unsupported PCAPNG not explicit: %v", err)
	}
}

func TestPCAPLinkDirectionFragmentAndIPv6Extension(t *testing.T) {
	ip := ipv4UDP()
	ether := append(make([]byte, 14), ip...)
	binary.BigEndian.PutUint16(ether[12:14], 0x800)
	sll := append(make([]byte, 16), ip...)
	binary.BigEndian.PutUint16(sll[0:2], 4)
	binary.BigEndian.PutUint16(sll[14:16], 0x800)
	sll2 := append(make([]byte, 20), ip...)
	binary.BigEndian.PutUint16(sll2[0:2], 0x800)
	binary.BigEndian.PutUint32(sll2[4:8], 9)
	sll2[10] = 0
	unknownType := append([]byte(nil), sll2...)
	unknownType[10] = 255
	fragment := append([]byte(nil), ip...)
	binary.BigEndian.PutUint16(fragment[6:8], 0x2000)
	ipv6 := make([]byte, 56)
	ipv6[0], ipv6[6], ipv6[40] = 0x60, 0, 17
	binary.BigEndian.PutUint16(ipv6[4:6], 16)
	copy(ipv6[48:], ip[20:])
	ipv6Fragment := append([]byte(nil), ipv6...)
	ipv6Fragment[6] = 44
	for _, c := range []struct {
		name                        string
		link                        uint32
		packet                      []byte
		protocol, status, direction string
		ports                       bool
	}{
		{"ethernet", 1, ether, "UDP", "transport_header_decoded", "unknown", true},
		{"sll", 113, sll, "UDP", "transport_header_decoded", "outgoing", true},
		{"sll2", 276, sll2, "UDP", "transport_header_decoded", "incoming_host", true},
		{"unknown_packet_type", 276, unknownType, "UDP", "transport_header_decoded", "unknown", true},
		{"unknown_link", 147, ip, "unknown", "unknown_link_type", "unknown", false},
		{"ipv4_fragment", 101, fragment, "UDP", "fragment_transport_unresolved", "unknown", false},
		{"ipv6_extension", 101, ipv6, "UDP", "transport_header_decoded", "unknown", true},
		{"ipv6_fragment", 101, ipv6Fragment, "UDP", "fragment_transport_unresolved", "unknown", false},
		{"ipv6_short_extension", 101, ipv6[:41], "IPv6", "truncated_ipv6_extension", "unknown", false},
		{"short_transport", 101, ip[:23], "UDP", "truncated_transport_header", "unknown", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := Stream(t.Context(), putCapture(t, captureBytes(binary.LittleEndian, false, c.link, 65535, c.packet)), 1)
			if err != nil {
				t.Fatal(err)
			}
			p := got.Members[0]
			if p.Protocol != c.protocol || p.DecodeStatus != c.status || p.Direction != c.direction || (p.SourcePort != nil) != c.ports {
				t.Fatalf("unsafe decode: %+v", p)
			}
			if c.link == 276 && (p.InterfaceIndex == nil || *p.InterfaceIndex != 9) {
				t.Fatal("interface index lost")
			}
		})
	}
}

func TestPCAPCaptureTruncationOrderingAndCancellation(t *testing.T) {
	b := captureBytes(binary.LittleEndian, false, 101, 65535, ipv4UDP(), ipv4UDP())
	binary.LittleEndian.PutUint32(b[36:40], 100)
	binary.LittleEndian.PutUint32(b[72:76], 1)
	got, err := Stream(t.Context(), putCapture(t, b), 2)
	if err != nil || got.TruncatedPackets != 1 || !got.Members[0].Truncated || got.CapturedBytes != 56 || got.OriginalBytes != 128 || got.OutOfOrderPackets != 1 || got.MinTimestamp.Decimal != "1789020987.000001" || got.FirstTimestamp.Decimal != "1789020987.035118" {
		t.Fatalf("length/time accounting: %+v %v", got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Stream(ctx, putCapture(t, b), 2); err != context.Canceled {
		t.Fatalf("cancellation=%v", err)
	}
}

func TestPCAPLargeCaplenUsesBoundedPrefix(t *testing.T) {
	const size = uint32(1 << 30)
	b := captureBytes(binary.LittleEndian, false, 147, size, []byte{})
	binary.LittleEndian.PutUint32(b[32:36], size)
	binary.LittleEndian.PutUint32(b[36:40], size)
	path := putCapture(t, b)
	if err := os.Truncate(path, 40+int64(size)); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	got, err := Stream(t.Context(), path, 1)
	runtime.ReadMemStats(&after)
	allocated := after.TotalAlloc - before.TotalAlloc
	t.Logf("logical_input_bytes=%d decoder_prefix_bytes=%d allocated_bytes=%d retained_members=%d", 40+int64(size), PacketPrefixBytes, allocated, len(got.Members))
	if err != nil || got.PacketCount != 1 || got.CapturedBytes != uint64(size) || !got.Members[0].DecoderPrefixLimited || allocated > 32<<20 {
		t.Fatalf("caplen allocated/unbounded or wrong result: %+v %v allocated=%d", got, err, allocated)
	}
}
