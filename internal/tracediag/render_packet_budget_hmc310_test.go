package tracediag

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/tracewire"
)

// A real classic-PCAP byte fixture, not a Result mock: 70 distinct endpoint
// groups and one ungrouped ICMP record. This exercises both preview and report
// limits without relying on the private real-capture path.
func hmc310PacketBudgetSource(t *testing.T, partial bool) string {
	t.Helper()
	b := make([]byte, 24)
	binary.LittleEndian.PutUint32(b, 0xa1b2c3d4)
	binary.LittleEndian.PutUint16(b[4:], 2)
	binary.LittleEndian.PutUint16(b[6:], 4)
	binary.LittleEndian.PutUint32(b[16:], 65535)
	binary.LittleEndian.PutUint32(b[20:], 101)
	for i := 0; i < 71; i++ {
		h := make([]byte, 16)
		binary.LittleEndian.PutUint32(h, uint32(1700000000+i))
		binary.LittleEndian.PutUint32(h[8:], 28)
		binary.LittleEndian.PutUint32(h[12:], 28)
		p := make([]byte, 28)
		p[0], p[9] = 0x45, 17
		if i == 70 {
			p[9] = 1
		}
		binary.BigEndian.PutUint16(p[2:], 28)
		copy(p[12:], []byte{192, 0, 2, 1, 192, 0, 2, 2})
		binary.BigEndian.PutUint16(p[20:], uint16(i))
		binary.BigEndian.PutUint16(p[22:], 4321)
		binary.BigEndian.PutUint16(p[24:], 8)
		b = append(append(b, h...), p...)
	}
	if partial {
		b = append(b, 1)
	}
	p := filepath.Join(t.TempDir(), "ordinary.capture")
	if err := os.WriteFile(p, b, 0400); err != nil {
		t.Fatal(err)
	}
	physical, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return physical
}

func hmc310PacketBudgetScript(t *testing.T, version, capLines int, view string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "collect.yaml")
	text := fmt.Sprintf("version: %d\nsteps:\n - {label: native, view: %s, max_lines: %d}\n", version, view, capLines)
	if err := os.WriteFile(p, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

func hmc310AssertWholePacketMembers(t *testing.T, result *tracequery.Result, report string) int {
	t.Helper()
	var members reflect.Value
	if result.PacketFlows != nil {
		members = reflect.ValueOf(result.PacketFlows.Members)
	} else {
		members = reflect.ValueOf(result.PacketInventory.Members)
	}
	shown := 0
	for i := 0; i < members.Len(); i++ {
		path := fmt.Sprintf("%s.members[%d]", result.View, i)
		var exact []string
		walkDetail(members.Index(i), path, func(s string) { exact = append(exact, s) }, 1)
		if !strings.Contains(report, "- "+path+":") {
			for _, line := range exact {
				if strings.Contains(report, line) {
					t.Fatalf("orphan member field without its identity: %s", line)
				}
			}
			continue
		}
		shown++
		for _, line := range exact {
			if strings.Count(report, line) != 1 {
				t.Errorf("visible member %d must contain each exact field once; missing/repeated %q", i, line)
			}
		}
	}
	return shown
}

func TestHMC310PacketPublicReportWholeGroupsAndExclusions(t *testing.T) {
	path := hmc310PacketBudgetSource(t, false)
	query, err := tracequery.StreamPacketQuery(t.Context(), path, tracequery.ViewPacketFlows, 64)
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []int{1, 2} {
		t.Run(fmt.Sprintf("v%d", version), func(t *testing.T) {
			var out bytes.Buffer
			failed, err := Run(context.Background(), Options{ScriptPath: hmc310PacketBudgetScript(t, version, 450, tracequery.ViewPacketFlows), TracePath: path, TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{})}, &out)
			if err != nil || failed != 0 {
				t.Fatalf("failed=%d err=%v\n%s", failed, err, out.String())
			}
			shown := hmc310AssertWholePacketMembers(t, &query.Payload, out.String())
			if shown == 0 || !strings.Contains(out.String(), "reason=unsupported_transport_protocol") {
				t.Errorf("member list displaced compact exclusion facts: shown=%d\n%s", shown, out.String())
			}
			for _, token := range []string{"packet_count=71", "flow_count=70", "omitted_tracked_flows=6", "grouped_packets=70", "ungrouped_packets=1", fmt.Sprintf("native_preview_members=64 displayed_members=%d", shown)} {
				if !strings.Contains(out.String(), token) {
					t.Errorf("lost native aggregate/report scope %q", token)
				}
			}
		})
	}
}

func TestHMC310PacketEveryBudgetPreservesWholeMembers(t *testing.T) {
	path := hmc310PacketBudgetSource(t, false)
	for _, view := range []string{tracequery.ViewPacketInventory, tracequery.ViewPacketFlows} {
		t.Run(view, func(t *testing.T) {
			query, err := tracequery.StreamPacketQuery(t.Context(), path, view, 3)
			if err != nil {
				t.Fatal(err)
			}
			full := renderStepBody(&Step{View: view, effMaxLines: 1000}, stepOutcome{result: &query.Payload})
			original, _ := json.Marshal(query.Payload)
			for capLines := 1; capLines <= full.total; capLines++ {
				body := renderStepBody(&Step{View: view, effMaxLines: capLines}, stepOutcome{result: &query.Payload})
				if body.total != full.total || len(body.lines) != capLines || !reflect.DeepEqual(body.lines, full.lines[:capLines]) {
					t.Fatalf("cap=%d changed census or existing prefix contract", capLines)
				}
				hmc310AssertWholePacketMembers(t, &query.Payload, strings.Join(body.lines, "\n"))
			}
			after, _ := json.Marshal(query.Payload)
			if !bytes.Equal(original, after) {
				t.Fatal("display schedule mutated native query authority")
			}
		})
	}
}

func TestHMC310PacketPublicOuterCapCountsOnlyWrittenMembers(t *testing.T) {
	path := hmc310PacketBudgetSource(t, false)
	for _, view := range []string{tracequery.ViewPacketInventory, tracequery.ViewPacketFlows} {
		query, err := tracequery.StreamPacketQuery(t.Context(), path, view, 64)
		if err != nil {
			t.Fatal(err)
		}
		for _, capLines := range []int{1, 15, 24, 50} {
			t.Run(fmt.Sprintf("%s/%d", view, capLines), func(t *testing.T) {
				var out bytes.Buffer
				failed, err := Run(t.Context(), Options{ScriptPath: hmc310PacketBudgetScript(t, 1, 450, view), TracePath: path, TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{}), TotalMaxLines: capLines}, &out)
				if failed != 0 || err != nil {
					t.Fatalf("cap changed complete query state: failed=%d err=%v", failed, err)
				}
				shown := hmc310AssertWholePacketMembers(t, &query.Payload, out.String())
				if !strings.Contains(out.String(), fmt.Sprintf("native_preview_members=64 displayed_members=%d undisplayed_preview_members=%d", shown, 64-shown)) {
					t.Fatalf("report cap overstated displayed members: shown=%d\n%s", shown, out.String())
				}
				if shown == 64 || !strings.Contains(out.String(), "native_status=complete") {
					t.Fatal("outer cap fixture did not isolate display from complete scan")
				}
			})
		}
	}
}

func TestHMC310PacketPublicTinyBudgetKeepsFailureAndUnknownDistinct(t *testing.T) {
	for _, view := range []string{tracequery.ViewPacketInventory, tracequery.ViewPacketFlows} {
		t.Run(view, func(t *testing.T) {
			path := hmc310PacketBudgetSource(t, true)
			var out bytes.Buffer
			failed, err := Run(t.Context(), Options{ScriptPath: hmc310PacketBudgetScript(t, 1, 1, view), TracePath: path, TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{}), TotalMaxLines: 1}, &out)
			if err != nil || failed != 1 {
				t.Fatalf("partial lost failure: failed=%d err=%v\n%s", failed, err, out.String())
			}
			for _, token := range []string{"native_status=partial", "native_preview_members=64 displayed_members=0", "code=pcap_short_record_header", "file_offset=3148", "displayed_classification_entries=0"} {
				if !strings.Contains(out.String(), token) {
					t.Errorf("tiny report lost %q:\n%s", token, out.String())
				}
			}
			if strings.Contains(out.String(), "状态=成功") || strings.Contains(out.String(), "flow_count=0") {
				t.Fatal("partial became success or unknown became zero")
			}
		})
	}
}

func TestHMC310PacketMemberPhysicalLinesKeepAllNativePaths(t *testing.T) {
	zero := uint32(0)
	stamp := &tracewire.PacketTimestamp{Seconds: 0, Fraction: 7, Decimal: "0.000000007"}
	result := tracequery.Result{View: tracequery.ViewPacketFlows, PacketFlows: &tracewire.PacketFlowsResult{
		Status: "complete", TimeDomain: "pcap_epoch_seconds", Members: []tracewire.PacketFlow{{
			ID: "row\r\nidentity", InterfaceIndex: &zero,
			VLANs:        []tracewire.PacketFlowVLAN{{TPID: 0x8100, ID: 0}},
			EndpointA:    tracewire.PacketFlowEndpoint{Address: "::1", Port: 0},
			EndpointB:    tracewire.PacketFlowEndpoint{Address: "::1", Port: 0},
			Stats:        tracewire.PacketFlowStats{Packets: 2, MinTimestamp: stamp, MaxTimestamp: stamp},
			SameEndpoint: tracewire.PacketFlowStats{Packets: 2, MinTimestamp: stamp, MaxTimestamp: stamp},
		}},
	}}
	body := renderStepBody(&Step{View: result.View, effMaxLines: 1000}, stepOutcome{result: &result})
	memberLines := 0
	for _, line := range body.lines {
		if strings.ContainsAny(line, "\r\n") {
			t.Fatal("logical line contains an uncounted physical break")
		}
		if strings.Contains(line, "- packet_flows.members[0]:") {
			memberLines++
			for _, token := range []string{`id=row\r\nidentity`, "interface_index=0", "vlans[0]: tpid=33024 id=0", "endpoint_a: address=::1 port=0", "endpoint_b: address=::1 port=0", "same_endpoint: packets=2", "same_endpoint.min_timestamp: seconds=0 fraction=7 epoch_seconds=0.000000007", "a_to_b: packets=0", "b_to_a: packets=0"} {
				if !strings.Contains(line, token) {
					t.Errorf("whole member lost exact token %q: %s", token, line)
				}
			}
		}
	}
	if memberLines != 1 {
		t.Fatalf("member occupies %d physical lines", memberLines)
	}
}

func TestHMC310PacketV2DisplayAccountingFitsValidatedWorstBudget(t *testing.T) {
	const steps, maxLines = HardV2MaxExpandedSteps, 10
	const worst = 66 + steps*(maxLines+12)
	var script strings.Builder
	fmt.Fprintf(&script, "version: 2\nlimits: {max_expanded_steps: %d, max_report_lines: %d}\nsteps:\n", steps, worst)
	for i := 0; i < steps; i++ {
		view := tracequery.ViewPacketInventory
		if i%2 != 0 {
			view = tracequery.ViewPacketFlows
		}
		fmt.Fprintf(&script, " - {label: native-%d, view: %s, max_lines: %d}\n", i, view, maxLines)
	}
	scriptPath := filepath.Join(t.TempDir(), "max-steps.yaml")
	if err := os.WriteFile(scriptPath, []byte(script.String()), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadScript(scriptPath)
	if err != nil || loaded.v2WorstReportLines != worst {
		t.Fatalf("fixture changed existing budget contract: %v %+v", err, loaded)
	}
	var out bytes.Buffer
	failed, err := Run(t.Context(), Options{ScriptPath: scriptPath, TracePath: hmc310PacketBudgetSource(t, false), TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{}), TotalMaxLines: worst}, &out)
	if failed != 0 || err != nil {
		t.Fatalf("valid v2 budget rejected: failed=%d err=%v", failed, err)
	}
	if lines := strings.Count(out.String(), "\n"); lines > worst {
		t.Fatalf("actual report has %d physical lines above validated %d", lines, worst)
	}
	if strings.Count(out.String(), "\n- 步骤 ") != steps || strings.Count(out.String(), "展示范围:") != steps {
		t.Fatal("display accounting must share the existing status line, once per step")
	}
}
