package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestEmitPerfTrace_Execute_TagsRuntimeArtifactOrigin(t *testing.T) {
	bus := &types.BusContext{
		Mutable:         types.NewMutableState("test"),
		AttachedHitrace: "Frame 1 duration=24.5ms",
	}
	params, err := json.Marshal(emitPerfTraceParams{
		Meta: emitPerfTraceMeta{
			Source:     "hitrace",
			DurationMs: 24.5,
			Signals:    []string{"jank"},
			Summary:    "one slow frame",
		},
		Frames: []emitPerfTraceFrame{{
			FrameNo:    1,
			DurationMs: 24.5,
			Janky:      true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	tool := &EmitPerfTrace{}
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("success=false, summary=%s", res.Summary)
	}
	if !strings.Contains(res.Summary, "evidence_origin=runtime_artifact") {
		t.Fatalf("summary should tag runtime artifact origin: %q", res.Summary)
	}
	if bundle := bus.Mutable.PerfTrace(); bundle == nil || len(bundle.Frames) != 1 {
		t.Fatalf("perf bundle missing frame: %+v", bundle)
	}
}
