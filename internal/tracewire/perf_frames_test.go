package tracewire

import (
	"reflect"
	"strings"
	"testing"
)

func TestPerfFramesLosslessBoundaries(t *testing.T) {
	frames := []PerfFrame{{Symbol: " sys;root@literal \"quoted\" ", DSO: "/system/a;b.so", IP: "0xffffffffffffffff", SourceRef: "frame:0", SourceDepth: 7}, {Symbol: "work", DSO: "/app/b.so", IP: "0x2", SourceRef: "frame:1", SourceDepth: 0}}
	raw := EncodePerfFrames(frames, "available")
	p, ok := DecodePerfFrames(raw)
	if !ok || !reflect.DeepEqual(frames, p.Frames) {
		t.Fatalf("frame identity changed: %s %+v", raw, p)
	}
	for _, bad := range []string{strings.Replace(raw, `"version":1`, `"version":1,"version":1`, 1), raw + "x", strings.Replace(raw, `"status":"available"`, `"status":"unknown"`, 1)} {
		if _, ok := DecodePerfFrames(bad); ok {
			t.Fatal("accepted malformed frame carrier")
		}
	}
	oversized := append([]PerfFrame(nil), frames...)
	oversized[0].Symbol = strings.Repeat("x", MaxPerfMetadataBytes+1)
	p, ok = DecodePerfFrames(EncodePerfFrames(oversized, "available"))
	if !ok || p.Status != "frame_budget_or_metadata_unavailable" || len(p.Frames) != 0 {
		t.Fatal("budget truncated a frame")
	}
}
