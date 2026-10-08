package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func catalogRequestWindowBus(t *testing.T, plans string) (*types.BusContext, string) {
	t.Helper()
	bus, root := catalogPublicFixture(t)
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{RuntimeArtifactScopeProfile: requestWindowTestProfile(10, 10.05)}}
	out, err := (&TraceCatalog{}).Execute(bus, json.RawMessage(plans))
	if err != nil || !out.Success {
		t.Fatalf("discover: %v %s", err, out.Summary)
	}
	return bus, root
}

func TestTraceCatalogRequestWindowPublicPreparedMembers(t *testing.T) {
	bus, root := catalogRequestWindowBus(t, `{"queries":[{"view":"resource_stack","pid":101,"time_start":10,"time_end":10.05}]}`)
	for _, name := range []string{"capture.data", "nested/capture.data"} {
		path := filepath.Join(root, name)
		for _, view := range []string{"resource_stack", "event_search"} {
			out := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": view, "pid": 101})
			payload := hmc17NamedPayload(t, out)
			if payload.TimeStart != 10 || payload.TimeEnd != 10.05 || !strings.Contains(out.Summary, "trace_query_request_window_inherited=") {
				t.Fatalf("%s/%s did not inherit exact plan window: %s", name, view, out.Summary)
			}
			if view == "resource_stack" && (payload.ResourceStack.MatchedEvents != 3 || payload.ResourceStack.Window.EndInclusive) {
				t.Fatalf("right boundary event admitted: %+v", payload.ResourceStack)
			}
		}
	}
	if len(bus.Mutable.TraceQueryCallWindows()) != 0 || bus.RuntimeArtifactPreflight.HasRuntimeArtifact() || bus.AttachedTraceMaterial != nil {
		t.Fatal("defaulting minted model-call/preflight/attachment authority")
	}
	// A later explicit scope must still win, independently of the planned one.
	out := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": filepath.Join(root, "capture.data"), "view": "resource_stack", "pid": 101, "time_start": 9, "time_end": 11})
	if p := hmc17NamedPayload(t, out); p.ResourceStack.MatchedEvents != 4 || p.TimeStart != 9 || p.TimeEnd != 11 {
		t.Fatal("catalog overrode explicit tool scope")
	}
}

func TestTraceCatalogRequestWindowPublicDeclarationToQuery(t *testing.T) {
	bus, root := catalogPublicFixture(t)
	request, payload := runtimeIntentPublicPayload(t)
	bus.Mutable = types.NewMutableState(request)
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	emitted, err := (&EmitAnalysis{}).Execute(bus, raw)
	if err != nil || !emitted.Success {
		t.Fatalf("emit before discovery: %v %s", err, emitted.Summary)
	}
	// Use the actual emitted mutable request, not a hand-built AnalysisIR.
	catalogPublicDiscover(t, bus)
	out := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": filepath.Join(root, "capture.data"), "view": "resource_stack", "pid": 101})
	result := hmc17NamedPayload(t, out)
	if result.ResourceStack.MatchedEvents != 3 || result.ResourceStack.Window.EndInclusive || result.TimeStart != 10 || result.TimeEnd != 10.05 {
		t.Fatalf("emitted directory scope did not survive prepare/query: %+v", result.ResourceStack)
	}
	if decided, full := types.RuntimeTraceReportShapeAuthority(bus.Mutable.RequestModel()); !decided || full {
		t.Fatal("successful query converted the finite question into a causal report")
	}
}

func TestTraceCatalogRequestWindowNeedsUnambiguousDeclaration(t *testing.T) {
	for name, plans := range map[string]string{
		"discovery_only":         `{}`,
		"unbounded":              `{"queries":[{"view":"resource_stack"}]}`,
		"one_endpoint":           `{"queries":[{"view":"resource_stack","time_start":10}]}`,
		"different_window":       `{"queries":[{"view":"resource_stack","time_start":9,"time_end":11}]}`,
		"conflicting_windows":    `{"queries":[{"view":"resource_stack","time_start":10,"time_end":10.05},{"view":"event_search","time_start":9,"time_end":11}]}`,
		"incomplete_second_plan": `{"queries":[{"view":"resource_stack","time_start":10,"time_end":10.05},{"view":"event_search"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			bus, root := catalogRequestWindowBus(t, plans)
			path := filepath.Join(root, "capture.data")
			// Successful normal query records must never become declarations.
			_ = catalogPublicQuery(t, bus, path, 101)
			out := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": "resource_stack", "pid": 101})
			if p := hmc17NamedPayload(t, out); p.ResourceStack.MatchedEvents != 4 || !p.ResourceStack.Window.EndInclusive || strings.Contains(out.Summary, "trace_query_request_window_inherited=") {
				t.Fatal("invented source/window binding")
			}
		})
	}
}

func TestTraceCatalogRequestWindowRejectsForeignStaleAndSubstitutedSources(t *testing.T) {
	bus, root := catalogRequestWindowBus(t, `{"queries":[{"view":"resource_stack","time_start":10,"time_end":10.05}]}`)
	path := filepath.Join(root, "capture.data")
	p := traceQueryParams{Source: "path", Path: path, View: "resource_stack"}
	if traceCatalogRequestWindowSourceMatches(bus, p, filepath.Join(root, "nested/capture.data"), 10, 10.05) {
		t.Fatal("same basename substituted source")
	}
	other := bus.ShallowClone()
	other.Mutable = types.NewMutableState("new turn")
	if traceCatalogRequestWindowSourceMatches(other, p, path, 10, 10.05) {
		t.Fatal("old catalog handle leaked into new turn")
	}
	bus.RuntimeArtifactPreflight.Artifacts = []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Carrier: "request_path", Source: "/another/capture"}}
	if traceCatalogRequestWindowSourceMatches(bus, p, path, 10, 10.05) {
		t.Fatal("catalog rescued unrelated preflight")
	}
	bus.RuntimeArtifactPreflight = types.RuntimeArtifactPreflightProfile{}
	m, err := bus.TraceInputPreparer.Prepare(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !traceCatalogRequestWindowSourceMatches(bus, p, m.QueryPath(), 10, 10.05) {
		t.Fatal("current prepared mapping lost")
	}
	if err := os.WriteFile(path, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if traceCatalogRequestWindowSourceMatches(bus, p, m.QueryPath(), 10, 10.05) {
		t.Fatal("stale source inherited request window")
	}
}
