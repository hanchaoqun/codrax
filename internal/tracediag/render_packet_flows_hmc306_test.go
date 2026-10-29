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

func hmc306FlowRenderResult(t *testing.T, mode string) tracequery.Result {
	t.Helper()
	a := make([]byte, 28)
	a[0], a[9] = 0x45, 17
	binary.BigEndian.PutUint16(a[2:4], 28)
	copy(a[12:20], []byte{192, 0, 2, 1, 192, 0, 2, 2})
	binary.BigEndian.PutUint16(a[20:22], 0)
	binary.BigEndian.PutUint16(a[22:24], 4321)
	binary.BigEndian.PutUint16(a[24:26], 8)
	b := append([]byte(nil), a...)
	copy(b[12:16], a[16:20])
	copy(b[16:20], a[12:16])
	copy(b[20:22], a[22:24])
	copy(b[22:24], a[20:22])
	c := append([]byte(nil), a...)
	binary.BigEndian.PutUint16(c[20:22], 999)
	var data bytes.Buffer
	for _, value := range []any{uint32(0xa1b23c4d), uint16(2), uint16(4), int32(0), uint32(0), uint32(65535), uint32(101)} {
		if err := binary.Write(&data, binary.LittleEndian, value); err != nil {
			t.Fatal(err)
		}
	}
	if mode != "empty" {
		for i, packet := range [][]byte{a, b, a, c} {
			for _, value := range []uint32{1789020987, uint32(7 + i), uint32(len(packet)), uint32(len(packet))} {
				if err := binary.Write(&data, binary.LittleEndian, value); err != nil {
					t.Fatal(err)
				}
			}
			data.Write(packet)
		}
	}
	if mode == "partial" {
		data.Write([]byte{1, 2, 3})
	}
	path := filepath.Join(t.TempDir(), "ordinary.capture")
	if err := os.WriteFile(path, data.Bytes(), 0400); err != nil {
		t.Fatal(err)
	}
	flows, err := pcapinput.StreamFlows(t.Context(), path, 1)
	if err != nil {
		t.Fatal(err)
	}
	return tracequery.Result{View: tracequery.ViewPacketFlows, SourcePath: path, PacketFlows: &flows.PacketFlowsResult}
}

func TestHMC306PacketFlowsActualRenderPreservesNativeScopeAndBudget(t *testing.T) {
	result := hmc306FlowRenderResult(t, "complete")
	before, _ := json.Marshal(result)
	full := renderStepBody(&Step{View: result.View, effMaxLines: 1000}, stepOutcome{result: &result})
	report := strings.Join(full.lines, "\n")
	for _, wanted := range []string{"status=complete", "time_domain=pcap_epoch_seconds", " packet_count=4", " flow_count=2", " tracked_flow_count=2", " omitted_tracked_flows=1", " grouped_packets=4", " ungrouped_packets=0", " group_budget_exceeded=false", " port=0", "epoch_seconds=1789020987.000000007", "packet_flows.members[0].same_endpoint: packets=0 captured_bytes=0 original_bytes=0"} {
		if !strings.Contains(report, wanted) {
			t.Errorf("missing native flow token %q:\n%s", wanted, report)
		}
	}
	for _, forbidden := range []string{"event_count=", "line_count=", "clock_regressions=", "query=[", "window:", "interface_index=", "packet_flows.members[1]", "same_endpoint.min_timestamp", filepath.Dir(result.SourcePath)} {
		if strings.Contains(report, forbidden) {
			t.Errorf("invented Trace/unknown/hidden fact %q", forbidden)
		}
	}
	for _, cap := range []int{1, 3, 8, len(full.lines) - 1, len(full.lines)} {
		bounded := renderStepBody(&Step{View: result.View, effMaxLines: cap}, stepOutcome{result: &result})
		if bounded.total != full.total || len(bounded.lines) != cap || !reflect.DeepEqual(bounded.lines, full.lines[:cap]) {
			t.Errorf("cap %d changed full census or native prefix", cap)
		}
	}
	after, _ := json.Marshal(result)
	if !bytes.Equal(before, after) || full.total != len(full.lines) || strings.Count(report, "明细 packet_flows:") != 1 {
		t.Fatal("flow rendering mutated native facts, repeated surface or lost cap accounting")
	}
}

func TestHMC306PacketFlowsActualRenderEmptyAndPartialStayDistinct(t *testing.T) {
	for _, mode := range []string{"empty", "partial"} {
		t.Run(mode, func(t *testing.T) {
			result := hmc306FlowRenderResult(t, mode)
			body := renderStepBody(&Step{View: result.View, effMaxLines: 1000}, stepOutcome{result: &result})
			report := strings.Join(body.lines, "\n")
			if mode == "empty" {
				for _, wanted := range []string{"status=complete", " flow_count=0", " packet_count=0", " captured_bytes=0", " original_bytes=0"} {
					if !strings.Contains(report, wanted) {
						t.Errorf("true empty flow census lost %s: %s", wanted, report)
					}
				}
				if strings.Contains(report, "epoch_seconds=") || strings.Contains(report, "packet_flows.members[") {
					t.Fatal("empty capture acquired timestamp or member")
				}
			} else {
				if !strings.Contains(report, "status=partial") || strings.Contains(report, " flow_count=") || strings.Contains(report, "status=complete") || !strings.Contains(report, "code=pcap_short_record_header") {
					t.Fatal("partial flow prefix became complete/zero or lost its failure", report)
				}
				if strings.Index(report, "packet_flows.failure:") > strings.Index(report, "packet_flows.members[") {
					t.Fatal("failure displaced by member preview")
				}
			}
		})
	}
}

func TestHMC306PacketFlowsKnownZeroIdentityIsNotMissing(t *testing.T) {
	zero := uint32(0)
	flow := tracewire.PacketFlow{InterfaceIndex: &zero, VLANs: []tracewire.PacketFlowVLAN{{TPID: 0x8100, ID: 0}}, EndpointA: tracewire.PacketFlowEndpoint{Address: "192.0.2.1", Port: 0}}
	var lines []string
	walkDetail(reflect.ValueOf(flow), "flow", func(line string) { lines = append(lines, line) }, 0)
	report := strings.Join(lines, "\n")
	for _, token := range []string{"interface_index=0", "id=0", "port=0", "packets=0", "decoder_prefix_limited_packets=0"} {
		if !strings.Contains(report, token) {
			t.Errorf("lost known zero %s: %s", token, report)
		}
	}
	if strings.Contains(report, "epoch_seconds=") {
		t.Fatal("absent direction timestamp became epoch zero")
	}
}
