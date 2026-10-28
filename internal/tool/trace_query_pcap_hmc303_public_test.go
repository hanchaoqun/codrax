package tool

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	promptctx "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc303PCAP(t *testing.T) string {
	t.Helper()
	var b bytes.Buffer
	for _, v := range []any{uint32(0xa1b2c3d4), uint16(2), uint16(4), int32(0), uint32(0), uint32(65535), uint32(276)} {
		if err := binary.Write(&b, binary.LittleEndian, v); err != nil {
			t.Fatal(err)
		}
	}
	// Linux cooked v2 + IPv4 + UDP. Duplicate bytes are independent records.
	packet := make([]byte, 48)
	binary.BigEndian.PutUint16(packet[0:2], 0x0800)
	binary.BigEndian.PutUint32(packet[4:8], 7)
	packet[10], packet[20], packet[29] = 4, 0x45, 17
	binary.BigEndian.PutUint16(packet[22:24], 28)
	copy(packet[32:36], []byte{192, 0, 2, 1})
	copy(packet[36:40], []byte{192, 0, 2, 2})
	binary.BigEndian.PutUint16(packet[40:42], 1234)
	binary.BigEndian.PutUint16(packet[42:44], 4321)
	binary.BigEndian.PutUint16(packet[44:46], 8)
	for i := uint32(0); i < 2; i++ {
		for _, v := range []uint32{1789020987, 35118 + i, uint32(len(packet)), uint32(len(packet))} {
			if err := binary.Write(&b, binary.LittleEndian, v); err != nil {
				t.Fatal(err)
			}
		}
		b.Write(packet)
	}
	path := filepath.Join(t.TempDir(), "ordinary-capture.data")
	if err := os.WriteFile(path, b.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHMC303PacketInventoryRepairRetryAndDefaultIsolation(t *testing.T) {
	path := hmc303PCAP(t)
	m, err := traceinput.Prepare(t.Context(), traceinput.Options{InputPath: path, PreviewBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	bus := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), AttachedTraceMaterial: m, AttachedHitrace: m.Preview(), Mutable: types.NewMutableState("packet facts"), AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{RuntimeArtifactScopeProfile: requestWindowTestProfile(10, 11)}}}
	for _, raw := range []string{`{"view":"packet_inventory","time_start":0}`, `{"view":"packet_inventory","pid":42}`, `{"view":"packet_inventory","line_start":0}`, `{"view":"packet_inventory","limit":65}`, `{"view":"event_search"}`} {
		r, err := (&TraceQuery{}).Execute(bus, json.RawMessage(raw))
		if err != nil || r.Success || r.Repair == nil || r.Repair.Metadata["stage"] != types.ToolRepairStageTraceSourceSelection || len(r.Observations) != 0 {
			t.Fatalf("wrong selector became success/terminal source failure: %+v %v", r, err)
		}
	}
	r, err := (&TraceQuery{}).Execute(bus, json.RawMessage(`{"view":"packet_inventory","limit":2}`))
	if err != nil || !r.Success || len(r.Observations) != 1 || r.Observations[0].SourceRef.QueryWindowKnown || len(bus.Mutable.TraceQueryCallWindows()) != 0 {
		t.Fatalf("repair or inherited Trace window blocked PCAP: %+v %v", r, err)
	}
	if _, _, _, ok := bus.Mutable.ResolveTraceIntervalNavigation(t.Context(), r.TraceQueryWindowReplay); ok {
		t.Fatal("packet facts acquired interval navigation")
	}
	if _, _, ok := bus.Mutable.ResolveTraceQueryWindowReplay(r.TraceQueryWindowReplay); ok {
		t.Fatal("packet facts acquired scheduler window replay")
	}
	bus.AnalysisIR = nil
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{r}})
	for _, table := range types.BuildRuntimeMeasurementContract(types.ObservationLedgerInputFromBusContext(bus, 0)).Choices() {
		if !table.HasNativeRawFacts() {
			t.Fatal("current prepared-source facts missing")
		}
		mutated := table.Clone()
		mutated.Notes = append(mutated.Notes, "invented relationship")
		if mutated.HasNativeRawFacts() {
			t.Fatal("mutated notes retained trusted display")
		}
	}
	serialized, _ := json.Marshal(r)
	var historical types.ToolResult
	if err := json.Unmarshal(serialized, &historical); err != nil {
		t.Fatal(err)
	}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{historical}})
	for _, table := range types.BuildRuntimeMeasurementContract(types.ObservationLedgerInputFromBusContext(bus, 0)).Choices() {
		if table.HasNativeRawFacts() {
			t.Fatal("serialized result reconstructed private facts")
		}
	}
	if err := os.WriteFile(path, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	r, err = (&TraceQuery{}).Execute(bus, json.RawMessage(`{"view":"packet_inventory"}`))
	if err != nil || r.Success || len(r.Observations) != 0 {
		t.Fatalf("changed original retained old facts: %+v %v", r, err)
	}
}

func TestHMC303PacketInventoryPartialIsNotEmptySuccess(t *testing.T) {
	path := hmc303PCAP(t)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	m, err := traceinput.Prepare(t.Context(), traceinput.Options{InputPath: path, PreviewBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	bus := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), AttachedTraceMaterial: m, AttachedHitrace: m.Preview(), Mutable: types.NewMutableState("packet facts")}
	r, err := (&TraceQuery{}).Execute(bus, json.RawMessage(`{"view":"packet_inventory"}`))
	if err != nil || r.Success || len(r.Observations) != 0 || r.RawRef == "" || !strings.Contains(r.Summary, "pcap_short_record_header") {
		t.Fatalf("partial packet source falsely successful: %+v %v", r, err)
	}
	data, err := os.ReadFile(r.RawRef)
	if err != nil {
		t.Fatal(err)
	}
	var payload tracequery.Result
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	p := payload.PacketInventory
	if p == nil || p.Status != "partial" || p.PacketCount != 2 || p.Failure == nil {
		t.Fatalf("partial raw evidence lost: %s", data)
	}
}

// This opt-in test exercises the same public producer against independently
// audited real bytes; normal deterministic tests never require external data.
func TestHMC303PacketInventoryRealCapture(t *testing.T) {
	path := os.Getenv("CODRAX_HMC303_PCAP_FIXTURE")
	if path == "" {
		t.Skip("real capture path not supplied")
	}
	m, err := traceinput.Prepare(t.Context(), traceinput.Options{InputPath: path, PreviewBytes: 2048})
	if err != nil {
		t.Fatal(err)
	}
	bus := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), AttachedTraceMaterial: m, AttachedHitrace: m.Preview(), Mutable: types.NewMutableState("packet facts")}
	r, err := (&TraceQuery{}).Execute(bus, json.RawMessage(`{"view":"packet_inventory","limit":12}`))
	if err != nil || !r.Success {
		t.Fatalf("real query: %+v %v", r, err)
	}
	data, err := os.ReadFile(r.Observations[0].SourceRef.PayloadRef)
	if err != nil {
		t.Fatal(err)
	}
	var payload tracequery.Result
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	p := payload.PacketInventory
	if p == nil {
		t.Fatal("missing dedicated packet inventory", string(data))
	}
	if p.PacketCount != 6101 || p.OmittedPackets != 6089 || p.SourceBytes != 3499182 || p.CapturedBytes != 3401542 || p.OriginalBytes != 3401542 || p.FirstTimestamp.Decimal != "1789020987.035118" || p.LastTimestamp.Decimal != "1789021016.416211" || p.Header.LinkType != 276 || p.TruncatedPackets != 0 {
		t.Fatalf("real counts/clock disagree with independent oracle: %+v", p)
	}
	want := map[string]uint64{"TCP": 4336, "UDP": 1627, "ICMP": 138}
	if len(p.Protocols) != len(want) {
		t.Fatal("protocol classes wrong", p.Protocols)
	}
	for _, row := range p.Protocols {
		if want[row.Label] != row.Count {
			t.Fatal("protocol count wrong", row)
		}
	}
	first := p.Members[0]
	if first.SourceAddress != "7.250.117.156" || first.DestinationAddress != "113.142.220.210" || first.SourcePort == nil || *first.SourcePort != 49479 || first.DestinationPort == nil || *first.DestinationPort != 443 || first.Direction != "outgoing" {
		t.Fatalf("first record differs from tcpdump: %+v", first)
	}
	t.Logf("real PCAP records=%d source_bytes=%d captured/original=%d/%d time=%s..%s protocols=%+v", p.PacketCount, p.SourceBytes, p.CapturedBytes, p.OriginalBytes, p.FirstTimestamp.Decimal, p.LastTimestamp.Decimal, p.Protocols)
}

func TestHMC303PacketInventoryPublicRoute(t *testing.T) {
	path := hmc303PCAP(t)
	before, _ := os.ReadFile(path)
	m, err := traceinput.Prepare(context.Background(), traceinput.Options{InputPath: path, PreviewBytes: 1024})
	if err != nil {
		t.Fatalf("public preparation rejected classic PCAP: %v", err)
	}
	bus := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), AttachedHitrace: m.Preview(), AttachedTraceMaterial: m, Mutable: types.NewMutableState("inspect capture")}
	projected := types.ToolBusContext(promptctx.BuildAgentContext(bus, types.AgentExplorer, types.StageExplore), types.AgentExplorer)
	r, err := (&TraceQuery{}).Execute(projected, json.RawMessage(`{"source":"attached_trace","view":"packet_inventory","limit":1}`))
	if err != nil || !r.Success {
		t.Fatalf("native packet query failed: %+v, %v", r, err)
	}
	if len(r.Observations) != 1 {
		t.Fatalf("missing native packet facts: %+v", r.Observations)
	}
	o := r.Observations[0]
	if o.Predicate != "packet_inventory_observation" || o.SourceRef.TimeDomain != "pcap_epoch_seconds" || o.SourceRef.QueryWindowKnown || o.Span.StartTs != 0 || o.Span.EndTs != 0 || r.TraceEvidenceAuthority != nil || len(projected.Mutable.TraceQueryCallWindows()) != 0 {
		t.Fatalf("packet facts acquired Trace coordinates or authority: %+v", r)
	}
	data, err := os.ReadFile(o.SourceRef.PayloadRef)
	if err != nil {
		t.Fatal(err)
	}
	var payload tracequery.Result
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	p := payload.PacketInventory
	if p == nil || p.PacketCount != 2 || p.OmittedPackets != 1 || len(p.Members) != 1 {
		t.Fatalf("preview replaced complete population: %s", data)
	}
	publication, ok := types.DecodeRuntimeMeasurementPublication(o)
	if !ok || len(publication.Tables) != 3 {
		t.Fatalf("native display handoff missing: %+v", o)
	}
	projected.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{r}})
	contract := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInputFromBusContext(projected, 0))
	if len(contract.Choices()) != 3 {
		t.Fatalf("native selectors lost: %+v", contract.Choices())
	}
	for _, table := range contract.Tables {
		if !table.HasNativeRawFacts() {
			t.Fatalf("table lost raw-fact seal: %+v", table)
		}
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("capture modified")
	}
}
