package tool

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The ordinary query and reference assertion must share exact typed logical
// selection, not a second alias table. A logical identifier is never a path.
func TestTraceQueryBusinessRefLogicalAssertionsShareSourceResolution(t *testing.T) {
	for _, attached := range []bool{false, true} {
		name := "file"
		if attached {
			name = "attachment"
		}
		t.Run(name, func(t *testing.T) { traceQueryBusinessRefLogicalAssertions(t, attached) })
	}
}

func traceQueryBusinessRefLogicalAssertions(t *testing.T, attached bool) {
	ctx, ref := businessRefAssertionFixture(t)
	d := ref.Data()
	if !attached {
		ctx.AttachedTraceMaterial, ctx.AttachedHitrace = nil, ""
	}
	ctx.RuntimeArtifactPreflight = types.RuntimeArtifactPreflightProfile{Active: true, Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: d.Path, Carrier: "request_path"}}}
	item := logicalArtifactSelectionItemBySource(t, ctx, d.Path)
	ordinary := businessRefTestQuery(t, ctx, map[string]any{"view": "window_stats", "source": "path", "path": item.ID, "pid": d.TID, "time_start": d.StartTs, "time_end": d.EndTs})
	if !ordinary.Success {
		t.Fatalf("ordinary alias control failed: %s", ordinary.Summary)
	}
	got := businessRefTestQuery(t, ctx, map[string]any{"view": "window_stats", "business_span_ref": ref.Token(), "source": "path", "path": item.ID})
	if !got.Success {
		t.Fatalf("same typed alias rejected only with reference: %s", got.Summary)
	}
	payload := carrierNativePayload(t, got, ctx.WorkDir)
	if payload.WindowStats == nil || payload.WindowStats.Window.StartTs != 1 || payload.WindowStats.Window.EndTs != 1.05 {
		t.Fatal("logical assertion lost atomic window")
	}

	other := filepath.Join(ctx.WorkDir, "other.systrace")
	body, err := os.ReadFile(d.Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, body, 0600); err != nil {
		t.Fatal(err)
	}
	ctx.RuntimeArtifactPreflight.Artifacts = append(ctx.RuntimeArtifactPreflight.Artifacts, types.RuntimeArtifactPreflightArtifact{Kind: "trace", Source: other, Carrier: "request_path"})
	foreign := logicalArtifactSelectionItemBySource(t, ctx, other)
	for _, id := range []string{foreign.ID, "runtime_artifact:ffffffffffffffff"} {
		r := businessRefTestQuery(t, ctx, map[string]any{"view": "window_stats", "business_span_ref": ref.Token(), "path": id})
		if r.Success || len(r.Observations) != 0 {
			t.Fatalf("foreign/unknown alias gained reference identity: %s", r.Summary)
		}
	}
	if attached {
		r := businessRefTestQuery(t, ctx, map[string]any{"view": "window_stats", "business_span_ref": ref.Token(), "source": "attached_trace", "path": foreign.ID})
		if r.Success {
			t.Fatal("matching source hid foreign logical path")
		}
	}
}
