package tool

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/tracewire"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc306PublicPCAP(t *testing.T) string {
	t.Helper()
	path := hmc303PCAP(t)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	reverse := append([]byte(nil), data[24:88]...)
	copy(reverse[16+32:16+40], append(append([]byte(nil), reverse[16+36:16+40]...), reverse[16+32:16+36]...))
	copy(reverse[16+40:16+44], []byte{0x10, 0xe1, 0x04, 0xd2})
	binary.LittleEndian.PutUint32(reverse[4:8], 35120)
	zero := append([]byte(nil), data[24:88]...)
	binary.BigEndian.PutUint16(zero[16+40:16+42], 0)
	binary.LittleEndian.PutUint32(zero[4:8], 35121)
	data = append(append(data, reverse...), zero...)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func hmc306FlowCall(t *testing.T, path string, limit int) (*types.BusContext, types.ToolResult, tracequery.Result) {
	t.Helper()
	m, err := traceinput.Prepare(t.Context(), traceinput.Options{InputPath: path, PreviewBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.Preview(), "packet_flows") {
		t.Fatal("prepared source did not teach endpoint query")
	}
	bus := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), AttachedTraceMaterial: m, AttachedHitrace: m.Preview(), Mutable: types.NewMutableState("packet endpoints"), AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{RuntimeArtifactScopeProfile: requestWindowTestProfile(10, 11)}}}
	registry := NewRegistry()
	RegisterDefaults(registry)
	for _, wrong := range []string{`{"view":"packet_flows","time_start":0}`, `{"view":"packet_flows","pid":1}`, `{"view":"packet_flows","limit":65}`, `{"view":"packet_flows","line_end":1}`} {
		r, e := registry.Execute(bus, "trace_query", json.RawMessage(wrong))
		if e != nil || r.Success || r.Repair == nil || r.Repair.Metadata["stage"] != types.ToolRepairStageTraceSourceSelection || len(r.Observations) != 0 {
			t.Fatalf("selector failure became completed/bad source: %+v %v", r, e)
		}
	}
	params, _ := json.Marshal(map[string]any{"view": "packet_flows", "source": "attached_trace", "limit": limit})
	r, err := registry.Execute(bus, "trace_query", params)
	if err != nil {
		t.Fatal(err)
	}
	ref := r.RawRef
	if len(r.Observations) > 0 {
		ref = r.Observations[0].SourceRef.PayloadRef
	}
	data, err := os.ReadFile(ref)
	if err != nil {
		t.Fatalf("payload absent: %+v %v", r, err)
	}
	var result tracequery.Result
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope) != 3 || envelope["view"] != "packet_flows" || result.PacketInventory != nil || result.PacketFlows == nil {
		t.Fatalf("flow gained unrelated Trace fields: %+v", envelope)
	}
	return bus, r, result
}

func TestHMC306PacketFlowsPublicRetryRawReceiptAndDirectionalFacts(t *testing.T) {
	path := hmc306PublicPCAP(t)
	before, _ := os.ReadFile(path)
	bus, r, payload := hmc306FlowCall(t, path, 1)
	p := payload.PacketFlows
	if !r.Success || len(r.Observations) != 1 || r.TraceEvidenceAuthority != nil || p.Status != "complete" || p.FlowCount == nil || *p.FlowCount != 2 || p.PacketCount != 4 || p.GroupedPackets != 4 || p.UngroupedPackets != 0 || p.OmittedTrackedFlows != 1 || len(p.Members) != 1 {
		t.Fatalf("wrong complete/preview census: %+v %+v", r, p)
	}
	f := p.Members[0]
	if f.Stats.Packets != 3 || f.AToB.Packets != 2 || f.BToA.Packets != 1 || f.AToB.CapturedBytes != 96 || f.BToA.OriginalBytes != 48 || f.Stats.MinTimestamp.Decimal != "1789020987.035118" || f.Stats.MaxTimestamp.Decimal != "1789020987.035120" {
		t.Fatalf("directional frame facts lost: %+v", f)
	}
	if r.Observations[0].SourceRef.QueryWindowKnown || len(bus.Mutable.TraceQueryCallWindows()) != 0 {
		t.Fatal("PCAP inherited Trace window")
	}
	if _, _, ok := bus.Mutable.ResolveTraceQueryWindowReplay(r.TraceQueryWindowReplay); ok {
		t.Fatal("PCAP minted Trace replay")
	}
	bus.AnalysisIR = nil
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{r}})
	contract := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInputFromBusContext(bus, 0))
	if contract == nil || len(contract.Choices()) != 3 {
		t.Fatalf("missing native flow tables: %+v", contract)
	}
	for _, table := range contract.Choices() {
		if !table.HasNativeRawFacts() {
			t.Fatalf("flow lost raw seal: %+v", table)
		}
		if table.View == types.RuntimeMeasurementMembers && (len(table.Rows) != 2 || table.Rows[0][6] != "2" || table.Rows[1][6] != "1") {
			t.Fatalf("native direction rows changed: %+v", table)
		}
		changed := table.Clone()
		changed.Notes = append(changed.Notes, "a connection was established")
		if changed.HasNativeRawFacts() {
			t.Fatal("copy acquired relationship authority")
		}
	}
	// Explicit path and attached input must query the same complete file, not
	// the prepared textual preview or the last selected Trace window.
	r2, err := (&TraceQuery{}).Execute(bus, json.RawMessage(fmtJSONFlowPath(path, 64)))
	if err != nil || !r2.Success || len(r2.Observations) != 1 {
		t.Fatalf("path route failed: %+v %v", r2, err)
	}
	data, _ := os.ReadFile(r2.Observations[0].SourceRef.PayloadRef)
	var all tracequery.Result
	if err := json.Unmarshal(data, &all); err != nil {
		t.Fatal(err)
	}
	if all.PacketFlows.PacketCount != p.PacketCount || *all.PacketFlows.FlowCount != *p.FlowCount || all.PacketFlows.Members[0].ID != f.ID || all.PacketFlows.OmittedTrackedFlows != 0 {
		t.Fatal("preview limit changed aggregate/identity")
	}
	if all.PacketFlows.Members[1].EndpointA.Port != 0 {
		t.Fatal("valid port zero lost")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("read query rewrote input")
	}
	serialized, _ := json.Marshal(r)
	var historical types.ToolResult
	if err := json.Unmarshal(serialized, &historical); err != nil {
		t.Fatal(err)
	}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{historical}})
	for _, table := range types.BuildRuntimeMeasurementContract(types.ObservationLedgerInputFromBusContext(bus, 0)).Choices() {
		if table.HasNativeRawFacts() {
			t.Fatal("JSON revived raw authority")
		}
	}
}

func fmtJSONFlowPath(path string, limit int) string {
	b, _ := json.Marshal(map[string]any{"view": "packet_flows", "source": "path", "path": path, "limit": limit})
	return string(b)
}

func TestHMC306PacketFlowsPublicEmptyPartialAndCatalog(t *testing.T) {
	c := capabilityPublicCall(t, nil, `{"view":"packet_flows","detail":true}`)
	view := c["views"].([]any)[0].(map[string]any)
	if !reflect.DeepEqual(view["metric_refs"], []any{"packet_flows"}) || len(c["metrics"].([]any)) != 1 {
		t.Fatalf("borrowed metric contract: %+v", c)
	}
	for _, mode := range []string{"empty", "partial"} {
		t.Run(mode, func(t *testing.T) {
			path := hmc306PublicPCAP(t)
			data, _ := os.ReadFile(path)
			if mode == "empty" {
				data = data[:24]
			} else {
				data = append(data, 1)
			}
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			_, r, result := hmc306FlowCall(t, path, 1)
			p := result.PacketFlows
			if mode == "empty" {
				if !r.Success || p.FlowCount == nil || *p.FlowCount != 0 || p.PacketCount != 0 || p.MinTimestamp != nil || p.MaxTimestamp != nil {
					t.Fatalf("real zero treated unknown: %+v %+v", r, p)
				}
			} else if r.Success || len(r.Observations) != 0 || p.FlowCount != nil || p.Status != "partial" || p.PacketCount != 4 {
				t.Fatalf("partial prefix authorized as complete: %+v %+v", r, p)
			}
		})
	}
}

func TestHMC306FlowPublicationMaxIPv6RowsFitsNativeReceipt(t *testing.T) {
	count := uint64(64)
	p := tracewire.PacketFlowsResult{SourcePath: filepath.Join(t.TempDir(), "source.pcap"), FlowCount: &count, TrackedFlowCount: count}
	for i := 0; i < 64; i++ {
		f := tracewire.PacketFlow{IPVersion: 6, Protocol: "TCP", EndpointA: tracewire.PacketFlowEndpoint{Address: "ffff:ffff:ffff:ffff:ffff:ffff:ffff:fffe", Port: 65535}, EndpointB: tracewire.PacketFlowEndpoint{Address: "ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff", Port: 65535}}
		for j := 0; j < 8; j++ {
			f.VLANs = append(f.VLANs, tracewire.PacketFlowVLAN{TPID: 0x88a8, ID: 4095})
		}
		index := ^uint32(0)
		f.InterfaceIndex = &index
		ts := &tracewire.PacketTimestamp{Seconds: ^uint32(0), Fraction: 999999999, Decimal: "4294967295.999999999"}
		f.AToB = tracewire.PacketFlowStats{Packets: ^uint64(0), CapturedBytes: ^uint64(0), OriginalBytes: ^uint64(0), TruncatedPackets: ^uint64(0), DecoderPrefixLimitedPackets: ^uint64(0), MinTimestamp: ts, MaxTimestamp: ts}
		f.BToA = f.AToB
		p.Members = append(p.Members, f)
	}
	b, err := json.Marshal(packetFlowsPublication(types.ObservationRecord{ID: "native"}, p))
	if err != nil || len(b) > 64<<10 {
		t.Fatalf("legal max display cannot fit native receipt: %d %v", len(b), err)
	}
}
