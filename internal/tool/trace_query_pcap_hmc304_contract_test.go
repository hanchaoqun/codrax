package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Test the declared paths against a real registered query's persisted JSON,
// not a synthetic Result literal. Metadata cannot borrow an unrelated Trace
// metric merely to pass discovery or claim a different clock than the producer.
func TestHMC304PacketCatalogMatchesPublicEnvelope(t *testing.T) {
	catalog := capabilityPublicCall(t, nil, `{"view":"packet_inventory","detail":true}`)
	views := catalog["views"].([]any)
	if len(views) != 1 || !reflect.DeepEqual(views[0].(map[string]any)["metric_refs"], []any{"packet_inventory"}) {
		t.Fatalf("packet view borrowed unrelated metrics: %+v", views)
	}
	metrics := catalog["metrics"].([]any)
	if len(metrics) != 1 || metrics[0].(map[string]any)["id"] != "packet_inventory" {
		t.Fatalf("packet metadata does not describe its own measurement: %+v", metrics)
	}
	path := hmc303PCAP(t)
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	result, payload := hmc304PacketPublicPayload(t, path, 1)
	if !result.Success || len(result.Observations) != 1 || result.TraceEvidenceAuthority != nil {
		t.Fatalf("complete packet query did not preserve its own authority: %+v", result)
	}
	if len(payload) != 3 || payload["view"] != "packet_inventory" || payload["source_path"] != path {
		t.Fatalf("packet result gained Trace fields or lost source: %+v", payload)
	}
	packet := payload["packet_inventory"].(map[string]any)
	if packet["status"] != "complete" || packet["time_domain"] != "pcap_epoch_seconds" || packet["packet_count"] != float64(2) || packet["omitted_packets"] != float64(1) || len(packet["members"].([]any)) != 1 {
		t.Fatalf("record census confused with preview: %+v", packet)
	}
	if packet["max_timestamp"].(map[string]any)["epoch_seconds"] != "1789020987.035119" || packet["members"].([]any)[0].(map[string]any)["timestamp"].(map[string]any)["epoch_seconds"] != "1789020987.035118" {
		t.Fatal("native precision or aggregate extent lost to preview", packet)
	}
	for _, raw := range metrics[0].(map[string]any)["outputs"].([]any) {
		output := raw.(map[string]any)
		section := output["section"].(string)
		if section != "packet_inventory" && !strings.HasPrefix(section, "packet_inventory.") {
			t.Fatalf("packet metric points outside dedicated envelope: %+v", output)
		}
		for _, rawField := range output["fields"].([]any) {
			field := rawField.(string)
			if !hmc304JSONPathExists(payload, strings.Split(section+"."+field, ".")) {
				t.Errorf("published field absent in actual query JSON: %s.%s", section, field)
			}
			if field == "epoch_seconds" && output["unit"] != "epoch seconds" {
				t.Errorf("packet timestamp acquired Trace clock unit: %+v", output)
			}
		}
	}
}

func TestHMC304PacketEnvelopeEmptyAndPartialAreDistinct(t *testing.T) {
	for _, kind := range []string{"empty", "partial"} {
		t.Run(kind, func(t *testing.T) {
			path := hmc303PCAP(t)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "empty" {
				data = data[:24]
			} else {
				data = append(data, 1, 2, 3)
			}
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			result, payload := hmc304PacketPublicPayload(t, path, 1)
			if len(payload) != 3 || result.TraceEvidenceAuthority != nil {
				t.Fatalf("packet status acquired Trace surface: %+v", payload)
			}
			packet := payload["packet_inventory"].(map[string]any)
			if kind == "empty" {
				if !result.Success || packet["status"] != "complete" || packet["packet_count"] != float64(0) || len(packet["members"].([]any)) != 0 || packet["failure"] != nil {
					t.Fatalf("empty capture not a successful zero census: %+v", packet)
				}
				for _, field := range []string{"first_record_timestamp", "last_record_timestamp", "min_timestamp", "max_timestamp"} {
					if _, ok := packet[field]; ok {
						t.Errorf("empty capture invented timestamp %s", field)
					}
				}
			} else if result.Success || len(result.Observations) != 0 || packet["status"] != "partial" || packet["packet_count"] != float64(2) || packet["failure"].(map[string]any)["code"] != "pcap_short_record_header" {
				t.Fatalf("failed source prefix became complete or empty: %+v", packet)
			}
		})
	}
}

func hmc304PacketPublicPayload(t *testing.T, path string, limit int) (types.ToolResult, map[string]any) {
	t.Helper()
	m, err := traceinput.Prepare(t.Context(), traceinput.Options{InputPath: path, PreviewBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	bus := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), AttachedTraceMaterial: m, AttachedHitrace: m.Preview(), Mutable: types.NewMutableState("packet inventory")}
	registry := NewRegistry()
	RegisterDefaults(registry)
	params, _ := json.Marshal(map[string]any{"view": tracequery.ViewPacketInventory, "source": "attached_trace", "limit": limit})
	result, err := registry.Execute(bus, "trace_query", params)
	if err != nil {
		t.Fatal(err)
	}
	payloadRef := result.RawRef
	if result.Success && len(result.Observations) > 0 {
		payloadRef = result.Observations[0].SourceRef.PayloadRef
	}
	data, err := os.ReadFile(payloadRef)
	if err != nil {
		t.Fatalf("missing raw result: %+v: %v", result, err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	return result, payload
}

func hmc304JSONPathExists(value any, parts []string) bool {
	if len(parts) == 0 {
		return value != nil
	}
	if rows, ok := value.([]any); ok {
		if len(rows) == 0 {
			return false
		}
		for _, row := range rows {
			if !hmc304JSONPathExists(row, parts) {
				return false
			}
		}
		return true
	}
	object, ok := value.(map[string]any)
	if !ok {
		return false
	}
	next, ok := object[parts[0]]
	return ok && hmc304JSONPathExists(next, parts[1:])
}
