package tracediag

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/pcapinput"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/tracewire"
)

func hmc305PacketRenderResult(t *testing.T, kind string) tracequery.Result {
	t.Helper()
	var data bytes.Buffer
	for _, value := range []any{uint32(0xa1b23c4d), uint16(2), uint16(4), int32(0), uint32(0), uint32(65535), uint32(147)} {
		if err := binary.Write(&data, binary.LittleEndian, value); err != nil {
			t.Fatal(err)
		}
	}
	if kind != "empty" {
		for _, fraction := range []uint32{7, 9, 8} {
			for _, value := range []uint32{1789020987, fraction, 2, 5} {
				if err := binary.Write(&data, binary.LittleEndian, value); err != nil {
					t.Fatal(err)
				}
			}
			data.Write([]byte{1, 2})
		}
	}
	if kind == "partial" {
		data.Write([]byte{1, 2, 3})
	}
	path := filepath.Join(t.TempDir(), "ordinary.capture")
	if err := os.WriteFile(path, data.Bytes(), 0400); err != nil {
		t.Fatal(err)
	}
	packet, err := pcapinput.Stream(t.Context(), path, 2)
	if err != nil {
		t.Fatal(err)
	}
	return tracequery.Result{View: tracequery.ViewPacketInventory, SourcePath: path, PacketInventory: &packet.PacketInventoryResult}
}

func TestHMC305PacketActualRenderPreservesOrderBudgetAndEpoch(t *testing.T) {
	result := hmc305PacketRenderResult(t, "complete")
	before, _ := json.Marshal(result)
	full := renderStepBody(&Step{View: result.View, effMaxLines: 1000}, stepOutcome{result: &result})
	report := strings.Join(full.lines, "\n")
	for _, wanted := range []string{"status=complete", "time_domain=pcap_epoch_seconds", "packet_count=3", "omitted_packets=1", "captured_bytes=6", "original_bytes=15", "truncated_packets=3", "out_of_order_packets=1", "timestamp_resolution=nanosecond", "capture_truncated=true", "decoder_prefix_limited=false", "fragmented=false"} {
		if !strings.Contains(report, wanted) {
			t.Errorf("missing native packet token %s:\n%s", wanted, report)
		}
	}
	last := -1
	for _, prefix := range []string{"- packet_inventory:", "- packet_inventory.header:", "- packet_inventory.first_record_timestamp:", "- packet_inventory.last_record_timestamp:", "- packet_inventory.min_timestamp:", "- packet_inventory.max_timestamp:", "- packet_inventory.protocols[0]:", "- packet_inventory.decode_statuses[0]:", "- packet_inventory.directions[0]:", "- packet_inventory.members[0]:", "- packet_inventory.members[0].timestamp:", "- packet_inventory.members[1]:"} {
		if count := strings.Count(report, prefix); count != 1 {
			t.Errorf("native path %s rendered %d times", prefix, count)
		}
		pos := strings.Index(report, prefix)
		if pos <= last {
			t.Errorf("native aggregate/header/clock/member order changed at %s", prefix)
		}
		last = pos
	}
	for _, pair := range []struct{ path, epoch string }{
		{"first_record_timestamp", "1789020987.000000007"},
		{"last_record_timestamp", "1789020987.000000008"},
		{"min_timestamp", "1789020987.000000007"},
		{"max_timestamp", "1789020987.000000009"},
		{"members[0].timestamp", "1789020987.000000007"},
		{"members[1].timestamp", "1789020987.000000009"},
	} {
		found := false
		for _, line := range full.lines {
			// A complete packet member now occupies one physical line; its
			// exact timestamp path/value remains uniquely addressable within it.
			if strings.Contains(line, "- packet_inventory."+pair.path+":") && strings.Contains(line, "epoch_seconds="+pair.epoch) {
				found = true
			}
		}
		if !found {
			t.Errorf("exact native epoch lost at %s", pair.path)
		}
	}
	for _, forbidden := range []string{"window:", "query=[", "event_count=", "clock_regressions=", "line_count=", "source_port=", "destination_port=", "interface_index=", "packet_inventory.members[2]", filepath.Dir(result.SourcePath)} {
		if strings.Contains(report, forbidden) {
			t.Errorf("packet rendering invented Trace/unknown/hidden facts or exposed host path %q", forbidden)
		}
	}
	for _, cap := range []int{1, 3, 8, len(full.lines) - 1, len(full.lines)} {
		bounded := renderStepBody(&Step{View: result.View, effMaxLines: cap}, stepOutcome{result: &result})
		if bounded.total != full.total || len(bounded.lines) != cap || !reflect.DeepEqual(bounded.lines, full.lines[:cap]) {
			t.Errorf("cap=%d changed native output census or prefix", cap)
		}
	}
	after, _ := json.Marshal(result)
	if !bytes.Equal(before, after) || full.total != len(full.lines) || strings.Count(report, "明细 packet_inventory:") != 1 {
		t.Fatal("packet display mutated, duplicated or silently omitted facts")
	}
}

func TestHMC305PacketActualRenderZeroUnknownAndPartial(t *testing.T) {
	for _, kind := range []string{"empty", "partial"} {
		t.Run(kind, func(t *testing.T) {
			result := hmc305PacketRenderResult(t, kind)
			body := renderStepBody(&Step{View: result.View, effMaxLines: 1000}, stepOutcome{result: &result})
			report := strings.Join(body.lines, "\n")
			if kind == "empty" {
				for _, token := range []string{"status=complete", "packet_count=0", "omitted_packets=0", "captured_bytes=0", "original_bytes=0", "truncated_packets=0", "out_of_order_packets=0"} {
					if !strings.Contains(report, token) {
						t.Errorf("empty capture lost measured zero %s:\n%s", token, report)
					}
				}
				if strings.Contains(report, "epoch_seconds=") || strings.Contains(report, "packet_inventory.members[") || strings.Contains(report, "packet_inventory.failure:") {
					t.Fatal("empty capture acquired invented time, member or failure")
				}
			} else {
				if !strings.Contains(report, "status=partial") || strings.Contains(report, "status=complete") || !strings.Contains(report, "code=pcap_short_record_header") || !strings.Contains(report, "packet_count=3") {
					t.Fatal("partial prefix became empty or complete", report)
				}
				if strings.Index(report, "packet_inventory.failure:") > strings.Index(report, "packet_inventory.members[") {
					t.Fatal("failure boundary displaced by member preview")
				}
			}
		})
	}
}

func TestHMC305PacketZeroPointersRemainDistinctFromAbsentIdentity(t *testing.T) {
	zero := uint16(0)
	packet := tracewire.PacketRecord{SourcePort: &zero, Direction: "unknown", Protocol: "unknown", DecodeStatus: "unknown"}
	var lines []string
	walkDetail(reflect.ValueOf(packet), "packet", func(line string) { lines = append(lines, line) }, 0)
	report := strings.Join(lines, "\n")
	if strings.Count(report, "source_port=0") != 1 || strings.Contains(report, "destination_port=") || strings.Contains(report, "interface_index=") || !strings.Contains(report, "fragmented=false") {
		t.Fatal("known-zero pointer was hidden, repeated or confused with absent identity", report)
	}
}
