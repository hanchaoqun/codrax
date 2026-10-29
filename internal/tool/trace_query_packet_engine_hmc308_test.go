package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/pcapinput"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Moving execution to the shared engine must not change the native payload,
// the publication identity, or the distinction between complete and partial.
// The independent low-level scan is the pre-migration envelope oracle, not a
// copy of the new adapter's rendering or completion decision.
func TestHMC308PacketEngineAndPublicToolPreserveEnvelope(t *testing.T) {
	for _, view := range []string{tracequery.ViewPacketInventory, tracequery.ViewPacketFlows} {
		for _, kind := range []string{"complete", "empty", "partial"} {
			for _, limit := range []int{1, 64} {
				t.Run(fmt.Sprintf("%s/%s/%d", view, kind, limit), func(t *testing.T) {
					path := hmc306PublicPCAP(t)
					original, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					switch kind {
					case "empty":
						original = original[:24]
					case "partial":
						original = append(original, 1, 2, 3)
					}
					if err := os.WriteFile(path, original, 0600); err != nil {
						t.Fatal(err)
					}
					path, err = filepath.EvalSymlinks(path)
					if err != nil {
						t.Fatal(err)
					}
					want := tracequery.Result{View: view, SourcePath: path}
					var version string
					if view == tracequery.ViewPacketInventory {
						scan, err := pcapinput.Stream(t.Context(), path, limit)
						if err != nil {
							t.Fatal(err)
						}
						want.PacketInventory, version = &scan.PacketInventoryResult, scan.SourceVersionToken()
					} else {
						scan, err := pcapinput.StreamFlows(t.Context(), path, limit)
						if err != nil {
							t.Fatal(err)
						}
						want.PacketFlows, version = &scan.PacketFlowsResult, scan.SourceVersionToken()
					}
					wantJSON, err := json.Marshal(want)
					if err != nil {
						t.Fatal(err)
					}
					shared, err := tracequery.StreamPacketQuery(t.Context(), path, view, limit)
					if err != nil || shared.ValidateCurrentSource() != nil || shared.SourceVersionToken() != version {
						t.Fatalf("shared scan lost source generation: %v", err)
					}
					sharedJSON, err := json.Marshal(shared.Payload)
					if err != nil || !bytes.Equal(sharedJSON, wantJSON) {
						t.Fatalf("shared envelope differs from native scan: %v\nwant=%s\ngot=%s", err, wantJSON, sharedJSON)
					}
					material, err := traceinput.Prepare(t.Context(), traceinput.Options{InputPath: path, PreviewBytes: 1024})
					if err != nil {
						t.Fatal(err)
					}
					bus := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), AttachedTraceMaterial: material,
						AttachedHitrace: material.Preview(), Mutable: types.NewMutableState("packet execution parity"),
						AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{RuntimeArtifactScopeProfile: requestWindowTestProfile(10, 11)}}}
					registry := NewRegistry()
					RegisterDefaults(registry)
					params, _ := json.Marshal(map[string]any{"view": view, "source": "attached_trace", "limit": limit})
					result, err := registry.Execute(bus, "trace_query", params)
					if err != nil || result.Success != (kind != "partial") || result.TraceEvidenceAuthority != nil {
						t.Fatalf("execution status/Trace authority changed: %+v %v", result, err)
					}
					ref := result.RawRef
					if kind == "partial" {
						if len(result.Observations) != 0 {
							t.Fatal("partial capture published complete native facts")
						}
					} else {
						if len(result.Observations) != 1 || result.Observations[0].SourceRef.QueryWindowKnown {
							t.Fatalf("packet source lost or inherited Trace window: %+v", result.Observations)
						}
						ref = result.Observations[0].SourceRef.PayloadRef
					}
					payload, err := os.ReadFile(ref)
					if err != nil || !bytes.Equal(payload, wantJSON) {
						t.Fatalf("public native payload changed: %v\nwant=%s\ngot=%s", err, wantJSON, payload)
					}
					current, err := os.ReadFile(path)
					if err != nil || !bytes.Equal(current, original) || shared.ValidateCurrentSource() != nil {
						t.Fatalf("read execution mutated source or invalidated original scan: %v", err)
					}
				})
			}
		}
	}
}
