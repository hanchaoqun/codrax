package hitraceconv

import (
	"github.com/hanchaoqun/codrax/internal/tracewire"
	"testing"
)

func TestRawPerfSamplingFramesPreserveContextAndRecursion(t *testing.T) {
	user := uint64(0xfffffffffffffe00)
	sample := rawPerfSample{IP: 0x10, Callchain: []uint64{user, 0x10, 0x20, 0x10}}
	p, ok := tracewire.DecodePerfFrames(rawPerfFramesV1(&rawPerfData{}, sample))
	if !ok || len(p.Frames) != 4 {
		t.Fatalf("context or recursion lost/duplicate leaf invented: %+v", p)
	}
	if p.Frames[0].Kind != "context_marker" || p.Frames[0].IP != "0xfffffffffffffe00" || p.Frames[0].SourceDepth != 0 || p.Frames[1].IP != "0x10" || p.Frames[2].IP != "0x20" || p.Frames[3].IP != "0x10" || p.Frames[3].SourceRef != "PERF_SAMPLE_CALLCHAIN[1]" {
		t.Fatalf("source block ordering/provenance: %+v", p)
	}
	for _, f := range p.Frames {
		if f.SourceRef == "PERF_SAMPLE_IP" {
			t.Fatal("duplicated separately captured leaf")
		}
	}
}

func TestProtoPerfSamplingUnknownMetadataUnavailable(t *testing.T) {
	for _, raw := range []string{simpleperfProtoFramesV1(simpleperfProtoSample{Frames: []simpleperfProtoFrame{{}}}, nil), hiperfFramesV1(hiperfProtoSample{Frames: []hiperfProtoFrame{{}}}, nil)} {
		p, ok := tracewire.DecodePerfFrames(raw)
		if !ok || p.Status != "frame_budget_or_metadata_unavailable" || len(p.Frames) != 0 {
			t.Fatalf("unproved synthesized labels entered prefix identity: %+v", p)
		}
	}
}

func TestRawPerfSamplingZeroAddressAndMarkerOnly(t *testing.T) {
	for _, sample := range []rawPerfSample{{IP: 1, Callchain: []uint64{1, 0}}, {IP: ^uint64(511)}} {
		p, ok := tracewire.DecodePerfFrames(rawPerfFramesV1(&rawPerfData{}, sample))
		if !ok || p.Status != "frame_budget_or_metadata_unavailable" || len(p.Frames) != 0 {
			t.Fatalf("unknown address became frame: %+v", p)
		}
	}
	p, ok := tracewire.DecodePerfFrames(rawPerfFramesV1(&rawPerfData{}, rawPerfSample{Callchain: []uint64{^uint64(511)}}))
	if !ok || p.Status != "missing" || len(p.Frames) != 0 {
		t.Fatalf("marker-only stack became function: %+v", p)
	}
}
